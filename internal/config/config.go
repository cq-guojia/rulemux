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
)

// Source 是配置里列出的一条源规则（可含一个或多个文件）。
type Source struct {
	// Paths 是源文件路径列表，每个都可以是磁盘上的任意位置。
	// 配置里既可写单个：path = "a.md"
	// 也可写数组：path = ["a.md", "b.md"]  —— 同一批文件共享下面的 agents/workspace。
	Paths []string
	// Agents 指定这批文件投递给哪些 agent；为空表示投递给全部 agent。
	Agents []string
	// Workspaces 指定这条规则适用于哪些工作区；为空、或含 "*"/"all" 表示所有工作区。
	// 既支持单个：workspace = "/path/to/proj"
	// 也支持数组：workspace = ["/a", "/b"]
	Workspaces []string
}

// Config 是 rulemux 的完整配置。
type Config struct {
	Sources []Source
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
	cur := -1 // 当前正在填充的 [[source]] 下标，-1 表示不在 source 表内

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// [[source]] 数组表
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
			if name == "source" {
				c.Sources = append(c.Sources, Source{})
				cur = len(c.Sources) - 1
			} else {
				cur = -1
			}
			continue
		}

		// 其它普通表（如 [features]），其键值不属于 source，忽略
		if strings.HasPrefix(line, "[") {
			cur = -1
			continue
		}

		idx := strings.Index(line, "=")
		if idx < 0 || cur < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "path":
			// 既支持单个字符串，也支持数组
			c.Sources[cur].Paths = parsePaths(val)
		case "agents":
			c.Sources[cur].Agents = parseArray(val)
		case "workspace":
			// 既支持单个字符串，也支持数组
			c.Sources[cur].Workspaces = parsePaths(val)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return c, nil
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
		return fmt.Errorf("配置 %s 中没有 [[source]]，请先用 rulemux init 生成示例配置并填写源文件", c.File)
	}
	for i, s := range c.Sources {
		if len(s.Paths) == 0 {
			return fmt.Errorf("配置 %s 第 %d 条 [[source]] 缺少 path", c.File, i+1)
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
//   "*"  匹配单个路径段（不含 "/"）
//   "**" 匹配零个或多个路径段（含 "/"），可出现在中间或末尾，用于"中间段统一"等场景
//   "?"、"[...]" 由 path/filepath.Match 处理
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
