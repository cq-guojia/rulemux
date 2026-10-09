// Package config 负责读取 rulemux 的 TOML 配置。
//
// 配置形态（docs/design/implementation.md #6）：
//
//	[[source]]
//	path = ["C:/rules/a.md", "C:/rules/b.md"]
//	agents = ["claude", "codex"]
//	workspace = ["/path/to/proj", "/path/to/other"]
//
// 本包只解析 rulemux 用到的 TOML 子集（[[source]] / path / agents / workspace / 注释），
// 目的是保持零第三方依赖，使将来在离线环境（如 NAS）也能 go build。
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
)

// Source 是配置里列出的一条源规则（可含一个或多个文件）。
type Source struct {
	// Paths 是源文件路径列表，每个都可以是磁盘上的任意位置。
	// 配置里既可写单个：path = "a.md"
	// 也可写数组：path = ["a.md", "b.md"]  —— 同一批文件共享下面的 agents/workspace。
	// 也可通过 groups 引用“文件组”，解析阶段会把组内所有文件合并进来。
	Paths []string
	// Agents 指定这批文件投递给哪些 agent；为空表示投递给全部 agent。
	Agents []string
	// Workspaces 指定这条规则适用于哪些工作区；为空、或含 "*"/"all" 表示所有工作区。
	// 既支持单个：workspace = "/path/to/proj"
	// 也支持数组：workspace = ["/a", "/b"]
	// 也可通过 workspace_groups 引用“工作区分组”，解析阶段会把组内所有路径合并进来。
	Workspaces []string
	// Groups 引用哪些文件组（按 [[file_group]].name），解析阶段展开成具体文件并入 Paths。
	Groups []string
	// WorkspaceGroups 引用哪些工作区分组（按 [[workspace_group]].name），解析阶段展开成具体路径并入 Workspaces。
	WorkspaceGroups []string
}

// FileGroup 是配置中的一个“文件组”，把若干规则文件打包，供 [[source]] 用 groups 引用。
// 组与组之间可互相引用（use），形成嵌套。
type FileGroup struct {
	Name  string   // 组名，供 groups 按名引用
	Paths []string // 组内文件；单个或数组写法均可
	Uses  []string // 引用的其它 file_group 名称（可多个，支持嵌套）
}

// WorkspaceGroup 是配置中的一个“工作区分组”，把若干工作区打包，供 [[source]] 用 workspace_groups 引用。
// 组与组之间可互相引用（use），形成嵌套；其 workspace 既可是具体路径，也可是 glob（如 /proj/**）。
type WorkspaceGroup struct {
	Name       string   // 组名，供 workspace_groups 按名引用
	Workspaces []string // 组内工作区；单个或数组写法均可，支持 glob
	Uses       []string // 引用的其它 workspace_group 名称（可多个，支持嵌套）
}

// Config 是 rulemux 的完整配置。
type Config struct {
	Sources         []Source
	FileGroups      []FileGroup
	WorkspaceGroups []WorkspaceGroup
	// File 是本次实际读取的配置文件路径。
	File string
}

// DefaultPath 返回默认配置路径 ~/.rulemux/config.toml。
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".rulemux", "config.toml")
	}
	return filepath.Join(home, ".rulemux", "config.toml")
}

// Load 读取并解析指定路径的配置文件。
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := &Config{File: path}
	sc := bufio.NewScanner(f)

	// curKind 跟踪当前正在填充的数组表类型；普通表（如 [features]）或未知数组表均为 none。
	const (
		curNone = iota
		curSource
		curFileGroup
		curWSGroup
	)
	cur := curNone
	srcIdx, fgIdx, wgIdx := -1, -1, -1

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 数组表 [[name]]
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
			switch name {
			case "source":
				c.Sources = append(c.Sources, Source{})
				srcIdx = len(c.Sources) - 1
				cur = curSource
			case "file_group":
				c.FileGroups = append(c.FileGroups, FileGroup{})
				fgIdx = len(c.FileGroups) - 1
				cur = curFileGroup
			case "workspace_group":
				c.WorkspaceGroups = append(c.WorkspaceGroups, WorkspaceGroup{})
				wgIdx = len(c.WorkspaceGroups) - 1
				cur = curWSGroup
			default:
				cur = curNone
			}
			continue
		}

		// 其它普通表（如 [features]），其键值不属于任何已知数组表，忽略
		if strings.HasPrefix(line, "[") {
			cur = curNone
			continue
		}

		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		switch cur {
		case curSource:
			switch key {
			case "path":
				c.Sources[srcIdx].Paths = parsePaths(val)
			case "agents":
				c.Sources[srcIdx].Agents = parseArray(val)
			case "workspace":
				c.Sources[srcIdx].Workspaces = parsePaths(val)
			case "groups":
				c.Sources[srcIdx].Groups = parseArray(val)
			case "workspace_groups":
				c.Sources[srcIdx].WorkspaceGroups = parseArray(val)
			}
		case curFileGroup:
			switch key {
			case "name":
				c.FileGroups[fgIdx].Name = unquote(val)
			case "path":
				c.FileGroups[fgIdx].Paths = parsePaths(val)
			case "use":
				c.FileGroups[fgIdx].Uses = parseArray(val)
			}
		case curWSGroup:
			switch key {
			case "name":
				c.WorkspaceGroups[wgIdx].Name = unquote(val)
			case "workspace":
				c.WorkspaceGroups[wgIdx].Workspaces = parsePaths(val)
			case "use":
				c.WorkspaceGroups[wgIdx].Uses = parseArray(val)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	// 顺序要紧：先把 "~" 展开成绝对路径，再展开组引用 —— 组引用合并时按**同一形态的
	// 绝对路径**去重，才不会出现 "/a.md" 与 "~/a.md" 被当成两个文件而重复投递。
	c.expandHomes()

	if err := c.resolveGroups(); err != nil {
		return nil, err
	}
	c.canonicalizeAgents()
	return c, nil
}

// expandHomes 把配置里所有路径值中的 "~" / "~/" 展开为当前用户的 HOME，
// 使同一份配置能跨机器、跨用户复用（不必硬编码 /Users/xxx 这类前缀）。
//
// 覆盖四处：Source.Paths、Source.Workspaces、FileGroup.Paths、WorkspaceGroup.Workspaces。
// 含 glob 的值（如 "~/proj/*"）同样先展开再匹配，单段 "*" / 跨段 "**" 的语义不变。
//
// 展开逻辑复用 agents.ExpandHome（全项目 "~" 展开的单一真源），此处不做第二份实现；
// 取不到 HOME 时它会原样返回，绝不静默改坏用户写的路径。
func (c *Config) expandHomes() {
	for i := range c.Sources {
		c.Sources[i].Paths = expandHomesAll(c.Sources[i].Paths)
		c.Sources[i].Workspaces = expandHomesAll(c.Sources[i].Workspaces)
	}
	for i := range c.FileGroups {
		c.FileGroups[i].Paths = expandHomesAll(c.FileGroups[i].Paths)
	}
	for i := range c.WorkspaceGroups {
		c.WorkspaceGroups[i].Workspaces = expandHomesAll(c.WorkspaceGroups[i].Workspaces)
	}
}

// expandHomesAll 把一组路径里的 "~" / "~/" 展开为 HOME；空切片原样返回。
func expandHomesAll(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, len(in))
	for i, p := range in {
		out[i] = agents.ExpandHome(p)
	}
	return out
}

// SourcesFor 返回在指定工作区下、应投递给指定 agent 的源规则。
//   - agents 为空 ⇒ 该条投递给全部 agent
//   - workspace 为空 / 含 "*" / "all" ⇒ 该条适用于全部工作区
func (c *Config) SourcesFor(agentID, workspace string) []Source {
	out := make([]Source, 0, len(c.Sources))
	for _, s := range c.Sources {
		if len(s.Agents) != 0 && !containsFold(s.Agents, agentID) {
			continue
		}
		if !s.MatchesWorkspace(workspace) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// DeclaresWorkspace 报告「该工作区是否被配置里任何一条 [[source]] 覆盖」。
//
// 用途：sync 的守卫 —— 若 cwd 不属于配置声明的工作区，则整体跳过（不建、不删、
// 不记账）。否则「空 want ⇒ 删残留」会清空一个我们无权管辖的目录。
// 见 docs/design/features/sync-all-and-change-notice.md §二「清空的正确姿势」/ §八。
//
// 注意：省略 workspace、或写 "*"/"**"/"all" 的 source 适用于所有工作区，
// 因此只要配置里有这类全局 source，任意路径都会被判为「已声明」。
func (c *Config) DeclaresWorkspace(ws string) bool {
	for _, s := range c.Sources {
		if s.MatchesWorkspace(ws) {
			return true
		}
	}
	return false
}

// MatchesWorkspace 报告该条 source 是否适用于给定工作区。
//   - Workspaces 为空           ⇒ 适用于所有工作区
//   - 含 "*" / "**" / "all"     ⇒ 适用于所有工作区（全局通配）
//   - 含 "*" / "?" / "[" 等 glob ⇒ 按业界标准 glob 匹配（"*" 单段，"**" 跨段递归）
//   - 其余                      ⇒ 与当前工作区路径完全相等（规范化 + 解析符号链接后）
func (s Source) MatchesWorkspace(ws string) bool {
	if len(s.Workspaces) == 0 {
		return true
	}
	for _, w := range s.Workspaces {
		if w == "*" || w == "**" || strings.EqualFold(w, "all") {
			return true // 全局通配
		}
		if isGlob(w) {
			// 通配模式保持原样（仅 Clean，不加 cwd 前缀），只对当前工作区解析符号链接
			if globMatch(filepath.Clean(w), resolvePath(ws)) {
				return true
			}
			continue
		}
		if samePath(w, ws) {
			return true
		}
	}
	return false
}

// Validate 校验配置是否可用。
func (c *Config) Validate() error {
	if len(c.Sources) == 0 {
		return fmt.Errorf("config %s has no [[source]]; run rulemux init first to generate a sample config and fill in your source files", c.File)
	}
	for i, s := range c.Sources {
		if len(s.Paths) == 0 {
			return fmt.Errorf("config %s: [[source]] #%d is missing path", c.File, i+1)
		}
	}
	return nil
}

// parsePaths 解析 path / workspace：单个字符串或数组都支持。
func parsePaths(val string) []string {
	if strings.HasPrefix(strings.TrimSpace(val), "[") {
		return parseArray(val)
	}
	if v := strings.TrimSpace(unquote(val)); v != "" {
		return []string{v}
	}
	return nil
}

// ResolvePath 把路径转绝对并规范化；能解析符号链接就解析（失败回退原规范化值）。
//
// 导出供 state（账本入库）等包复用同一口径 —— 否则同一物理工作区经软链打开时会被
// 记成两个不同的工作区。见 docs/design/features/sync-all-and-change-notice.md §四。
func ResolvePath(p string) string { return resolvePath(p) }

// resolvePath 把路径转绝对并规范化；能解析符号链接就解析（失败回退原规范化值），
// 避免工作区经软链接打开时与配置里的真实路径对不上而漏配。
func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	abs = filepath.Clean(abs)
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	return abs
}

// isGlob 报告一个 workspace 模式是否包含 glob 元字符（单独的 "*" / "**" / "all"
// 已在 MatchesWorkspace 中作为全局通配先行处理，不会落到这里）。
func isGlob(w string) bool {
	return strings.Contains(w, "*")
}

// samePath 比较两个工作区路径是否指向同一个目录。
// 先规范化并尽量解析符号链接（resolvePath），再比较；Windows 忽略大小写。
func samePath(a, b string) bool {
	aa, bb := resolvePath(a), resolvePath(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(aa, bb) // Windows 路径大小写不敏感
	}
	return aa == bb
}

// globMatch 按业界公认的 glob 语义做整路径匹配：
//
//	"*"  匹配单个路径段（不含 "/"）
//	"**" 匹配零个或多个路径段（含 "/"），可出现在中间或末尾，用于"中间段统一"等场景
//	"?"、"[...]" 由 path/filepath.Match 处理
//
// 匹配前统一去掉前导 "/"，使绝对/相对写法都能对齐。
func globMatch(pattern, name string) bool {
	pattern = strings.TrimLeft(pattern, "/")
	name = strings.TrimLeft(name, "/")
	pp := strings.Split(pattern, "/")
	np := strings.Split(name, "/")
	var rec func(pi, ni int) bool
	rec = func(pi, ni int) bool {
		for pi < len(pp) {
			p := pp[pi]
			if p == "**" {
				if pi == len(pp)-1 {
					return true // 末尾 ** 匹配剩余所有段
				}
				for k := ni; k <= len(np); k++ { // ** 吞掉 0..剩余 段，回溯尝试
					if rec(pi+1, k) {
						return true
					}
				}
				return false
			}
			if ni >= len(np) {
				return false
			}
			if !segMatch(p, np[ni]) {
				return false
			}
			pi++
			ni++
		}
		return ni == len(np)
	}
	return rec(0, 0)
}

// segMatch 匹配单个路径段：无元字符则直接相等；否则交给 path/filepath.Match。
func segMatch(pat, name string) bool {
	if !strings.ContainsAny(pat, "*?[") {
		return pat == name
	}
	ok, err := filepath.Match(pat, name)
	if err != nil {
		return pat == name
	}
	return ok
}

// unquote 去掉字符串两侧的成对引号。
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseArray 解析 ["a", "b"] 形式的字符串数组。
func parseArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, unquote(p))
	}
	return out
}

// containsFold 忽略大小写判断切片是否包含某字符串。
func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), v) {
			return true
		}
	}
	return false
}

// resolveGroups 把配置中的“组”引用展开成具体的文件 / 工作区，
// 并写回每条 Source 的 Paths / Workspaces。展开过程做环检测与按源路径去重，
// 因此组间重复文件最终只出现一次（满足“每个文件只做一次”）。
func (c *Config) resolveGroups() error {
	fgMap := make(map[string]*FileGroup, len(c.FileGroups))
	for i := range c.FileGroups {
		g := &c.FileGroups[i]
		if g.Name == "" {
			return fmt.Errorf("config %s has an unnamed [[file_group]]", c.File)
		}
		if _, dup := fgMap[g.Name]; dup {
			return fmt.Errorf("config %s: duplicate [[file_group]] name %q", c.File, g.Name)
		}
		fgMap[g.Name] = g
	}
	wgMap := make(map[string]*WorkspaceGroup, len(c.WorkspaceGroups))
	for i := range c.WorkspaceGroups {
		g := &c.WorkspaceGroups[i]
		if g.Name == "" {
			return fmt.Errorf("config %s has an unnamed [[workspace_group]]", c.File)
		}
		if _, dup := wgMap[g.Name]; dup {
			return fmt.Errorf("config %s: duplicate [[workspace_group]] name %q", c.File, g.Name)
		}
		wgMap[g.Name] = g
	}

	// 递归展开文件组，visiting 用于环检测。
	var expandFG func(name string, visiting map[string]bool, out *[]string) error
	expandFG = func(name string, visiting map[string]bool, out *[]string) error {
		g, ok := fgMap[name]
		if !ok {
			return fmt.Errorf("config %s references undefined file group %q", c.File, name)
		}
		if visiting[name] {
			return fmt.Errorf("config %s has a circular reference in file groups: %s", c.File, name)
		}
		visiting[name] = true
		defer delete(visiting, name)
		for _, u := range g.Uses {
			if err := expandFG(u, visiting, out); err != nil {
				return err
			}
		}
		appendUnique(out, g.Paths)
		return nil
	}

	// 递归展开工作区分组（workspace 值可能是 glob，原样保留，交给 MatchesWorkspace 处理）。
	var expandWG func(name string, visiting map[string]bool, out *[]string) error
	expandWG = func(name string, visiting map[string]bool, out *[]string) error {
		g, ok := wgMap[name]
		if !ok {
			return fmt.Errorf("config %s references undefined workspace group %q", c.File, name)
		}
		if visiting[name] {
			return fmt.Errorf("config %s has a circular reference in workspace groups: %s", c.File, name)
		}
		visiting[name] = true
		defer delete(visiting, name)
		for _, u := range g.Uses {
			if err := expandWG(u, visiting, out); err != nil {
				return err
			}
		}
		appendUnique(out, g.Workspaces)
		return nil
	}

	for i := range c.Sources {
		s := &c.Sources[i]
		var files []string
		for _, gname := range s.Groups {
			if err := expandFG(gname, map[string]bool{}, &files); err != nil {
				return err
			}
		}
		// 组展开的文件在前，source 自身显式 path 在后（书写顺序直观：groups 写在前）。
		s.Paths = mergeUnique(files, s.Paths)

		var wss []string
		for _, gname := range s.WorkspaceGroups {
			if err := expandWG(gname, map[string]bool{}, &wss); err != nil {
				return err
			}
		}
		// 工作区分组展开在前，source 自身显式 workspace 在后。
		s.Workspaces = mergeUnique(wss, s.Workspaces)
	}
	return nil
}

// appendUnique 把 vals 中尚未出现在 *out 的元素追加进去（保持首次出现顺序，按精确字符串去重）。
func appendUnique(out *[]string, vals []string) {
	for _, v := range vals {
		if !containsStr(*out, v) {
			*out = append(*out, v)
		}
	}
}

// mergeUnique 合并 a 与 b，去掉重复（b 中已存在于 a 的元素忽略，保持 a 顺序后接 b 剩余）。
func mergeUnique(a, b []string) []string {
	out := make([]string, len(a))
	copy(out, a)
	appendUnique(&out, b)
	return out
}

// containsStr 报告切片是否包含 s（精确匹配）。
func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// canonicalizeAgents 把每条 source 的 agents 值归一为注册表里的规范 ID（别名 → 规范 ID）。
//
// 否则用户在配置里写别名（如 agents = ["workbuddy"]）时，SourcesFor 用规范 ID 去匹配
// 会**静默漏投**。这是「别名归一」四个入口之一（见设计 §十 #4）。
func (c *Config) canonicalizeAgents() {
	for i := range c.Sources {
		c.Sources[i].Agents = canonicalAgents(c.Sources[i].Agents)
	}
}

// canonicalAgents 把一组 agent 名归一为规范 ID 并去重（保持首次出现顺序）。
// 注册表里查不到的名字原样保留 —— 它不会命中任何 agent，语义等价于「没有这个 agent」。
func canonicalAgents(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, x := range in {
		id := x
		if a, ok := agents.Get(x); ok {
			id = a.ID
		}
		if id == "" || seen[strings.ToLower(id)] {
			continue
		}
		seen[strings.ToLower(id)] = true
		out = append(out, id)
	}
	return out
}
