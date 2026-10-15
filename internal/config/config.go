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
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cq-guojia/rulemux/internal/agents"
)

// 作用域（scope）：规则 / skill 落在哪个域。
const (
	// ScopeWorkspace 工作区级（默认）：落到各工作区的 agent 目录（如 .codebuddy/rules）。
	ScopeWorkspace = "workspace"
	// ScopeUser 用户级（全局）：落到 agent 的用户级目录（如 ~/.codebuddy/rules），
	// 与工作区无关 ⇒ 查询时忽略 workspace 匹配。
	ScopeUser = "user"
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
	// 注意：Scope 为 user 时本字段被忽略（用户级目录本就与工作区无关）。
	Workspaces []string
	// Groups 引用哪些文件组（按 [[file_group]].name），解析阶段展开成具体文件并入 Paths。
	Groups []string
	// WorkspaceGroups 引用哪些工作区分组（按 [[workspace_group]].name），解析阶段展开成具体路径并入 Workspaces。
	WorkspaceGroups []string
	// Scope 投放域："" / "workspace"（默认）= 工作区级；"user" = 用户级（全局）。
	Scope string
}

// IsUserScope 报告该条是否投放到用户级（全局）域。
func (s Source) IsUserScope() bool { return strings.EqualFold(s.Scope, ScopeUser) }

// SkillSource 是配置里列出的一条 skill 源（v1：仅本地目录）。
//
// 语义与 [[source]] 平行，但源是**目录**而非文件：path 指向一个 skill 根目录，
// 其下每个含 SKILL.md（大小写不敏感）的子目录就是一个 skill；若根目录自身含
// SKILL.md，则根本身也是一个 skill（单技能目录布局同样支持）。
type SkillSource struct {
	// Paths 是 skill 根目录列表（单个或数组均可）。
	Paths []string
	// Agents 指定投递给哪些 agent；为空表示全部支持的 agent。
	Agents []string
	// Workspaces 适用工作区（仅 workspace 域生效；user 域忽略）。
	Workspaces []string
	// Groups 引用哪些 skill 组（按 [[skill_group]].name），解析阶段展开成具体
	// skill 根目录并入 Paths —— 与 [[source]].Groups 同构，只是装的是目录。
	Groups []string
	// WorkspaceGroups 引用哪些工作区分组（按 [[workspace_group]].name），解析阶段
	// 展开成具体路径并入 Workspaces（工作区在两个载体间共用，不分规则 / skill）。
	WorkspaceGroups []string
	// Scope 投放域："" / "workspace"（默认）| "user"。
	Scope string
}

// IsUserScope 报告该条是否投放到用户级（全局）域。
func (s SkillSource) IsUserScope() bool { return strings.EqualFold(s.Scope, ScopeUser) }

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

// SkillGroup 是配置中的一个“skill 组”，把若干 skill 根目录打包，供 [[skill_source]] 用 groups 引用。
// 与 FileGroup 同构，但装的是“目录”（skill 根）而非文件：展开后并入 SkillSource.Paths。
type SkillGroup struct {
	Name  string   // 组名，供 groups 按名引用
	Paths []string // 组内 skill 根目录；单个或数组写法均可
	Uses  []string // 引用的其它 skill_group 名称（可多个，支持嵌套）
}

// Config 是 rulemux 的完整配置。
type Config struct {
	Sources         []Source
	SkillSources    []SkillSource
	FileGroups      []FileGroup
	WorkspaceGroups []WorkspaceGroup
	SkillGroups     []SkillGroup
	// FetchTimeout 是远程 skill 源（git/npm）抓取超时，形如 "30s"；空 = 默认 30s。
	FetchTimeout string
	// File 是本次实际读取的配置文件路径。
	File string
}

// DefaultFetchTimeout 是远程 skill 源抓取的默认超时（config.fetch_timeout 未配 / 非法时回退）。
const DefaultFetchTimeout = 30 * time.Second

// FetchTimeoutDuration 返回远程抓取超时；未配 / 解析失败 / 非正 ⇒ 回退默认值。
func (c *Config) FetchTimeoutDuration() time.Duration {
	if c.FetchTimeout == "" {
		return DefaultFetchTimeout
	}
	d, err := time.ParseDuration(c.FetchTimeout)
	if err != nil || d <= 0 {
		return DefaultFetchTimeout
	}
	return d
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// 剥 UTF-8 BOM：否则首行（常是第一张表头）匹配不上，整张表静默丢失。
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	c := &Config{File: path}
	p := &tomlParser{path: path, src: string(data), row: 1}
	if err := p.parse(c); err != nil {
		return nil, err
	}

	// 顺序要紧：先把 "~" 展开成绝对路径，再展开组引用 —— 组引用合并时按**同一形态的
	// 绝对路径**去重，才不会出现 "/a.md" 与 "~/a.md" 被当成两个文件而重复投递。
	c.expandHomes()

	if err := c.resolveGroups(); err != nil {
		return nil, err
	}
	c.canonicalizeAgents()
	c.normalizeScopes()
	return c, nil
}

// tomlParser 是按本配置 schema 裁剪的 TOML 递归下降解析器（零第三方依赖）。
//
// 覆盖标准写法：字符串（基本/字面/多行基本/多行字面）、字符串数组（可跨行、
// 可含注释与尾逗号）、裸键与引号键、[[数组表]] 与 [普通表] 头、注释与空行。
//
// 原则「宁报错不静默」：旧的行级解析器把认不出的东西静默吞掉 —— 多行数组被读成
// 空 ⇒ workspace 变成「所有工作区」；数组元素里的逗号把路径切碎；引号键不匹配
// 被整条忽略 ⇒ agents 空 = 投全部 agent；BOM 让首张表整表丢失。本解析器对一切
// 认不出的语法（其余值类型、裸值、点键、同表重复键、已知表内未知键）一律带
// 文件:行号 报错。
type tomlParser struct {
	path string // 配置文件路径，仅用于错误信息
	src  string
	pos  int
	row  int
}

// parse 遍历整份配置：表头切换当前表，键值分发进对应字段。
func (p *tomlParser) parse(c *Config) error {
	// curKind 跟踪当前正在填充的数组表类型；普通表（如 [features]）或未知数组表均为 none。
	const (
		curNone = iota
		curSource
		curSkillSource
		curFileGroup
		curWSGroup
		curSkillGroup
	)
	cur := curNone
	srcIdx, skillIdx, fgIdx, wgIdx, sgIdx := -1, -1, -1, -1, -1
	// seen 记录当前表内已出现的键 —— 同表重复键按 TOML 规定非法，报错而非静默覆盖。
	seen := map[string]bool{}

	for {
		p.skipBlank()
		if p.eof() {
			return nil
		}

		// 表头：[[name]]（数组表）或 [name]（普通表，忽略其键值）
		if p.peek() == '[' {
			hdrRow := p.row
			name, isArray, err := p.parseTableHeader()
			if err != nil {
				return err
			}
			seen = map[string]bool{}
			// 已知数组表单写成 [name] 是常见 typo：按未知表静默丢掉整张表太危险，显式报错。
			if !isArray {
				switch name {
				case "source", "skill_source", "file_group", "workspace_group", "skill_group":
					return p.errAt(hdrRow, "table [%s] should be an array of tables: write [[%s]]", name, name)
				}
				cur = curNone
				continue
			}
			switch name {
			case "source":
				c.Sources = append(c.Sources, Source{})
				srcIdx = len(c.Sources) - 1
				cur = curSource
			case "skill_source":
				c.SkillSources = append(c.SkillSources, SkillSource{})
				skillIdx = len(c.SkillSources) - 1
				cur = curSkillSource
			case "file_group":
				c.FileGroups = append(c.FileGroups, FileGroup{})
				fgIdx = len(c.FileGroups) - 1
				cur = curFileGroup
			case "workspace_group":
				c.WorkspaceGroups = append(c.WorkspaceGroups, WorkspaceGroup{})
				wgIdx = len(c.WorkspaceGroups) - 1
				cur = curWSGroup
			case "skill_group":
				c.SkillGroups = append(c.SkillGroups, SkillGroup{})
				sgIdx = len(c.SkillGroups) - 1
				cur = curSkillGroup
			default:
				cur = curNone
			}
			continue
		}

		// key = value
		key, err := p.parseKey()
		if err != nil {
			return err
		}
		p.skipWS()
		if p.peek() != '=' {
			return p.errf("expected '=' after key %q", key)
		}
		p.advance()
		p.skipWS()
		items, isArray, err := p.parseValue()
		if err != nil {
			return err
		}
		if err := p.expectLineEnd(); err != nil {
			return err
		}
		if seen[key] {
			return p.errf("duplicate key %q in the same table", key)
		}
		seen[key] = true
		// name / scope 是单值字段，数组写法是用户错误，显式报错。
		if isArray && (key == "name" || key == "scope") {
			return p.errf("key %q expects a single string, not an array", key)
		}

		switch cur {
		case curSource:
			switch key {
			case "path":
				c.Sources[srcIdx].Paths = items
			case "agents":
				c.Sources[srcIdx].Agents = items
			case "workspace":
				c.Sources[srcIdx].Workspaces = items
			case "groups":
				c.Sources[srcIdx].Groups = items
			case "workspace_groups":
				c.Sources[srcIdx].WorkspaceGroups = items
			case "scope":
				c.Sources[srcIdx].Scope = normalizeScope(items[0])
			default:
				return p.errf("unknown key %q in [[source]]", key)
			}
		case curSkillSource:
			// groups / workspace_groups 与 [[source]] 同义：分别是 [[skill_group]] /
			// [[workspace_group]] 的按名引用，解析阶段展开进 Paths / Workspaces。
			switch key {
			case "path":
				c.SkillSources[skillIdx].Paths = items
			case "agents":
				c.SkillSources[skillIdx].Agents = items
			case "workspace":
				c.SkillSources[skillIdx].Workspaces = items
			case "groups":
				c.SkillSources[skillIdx].Groups = items
			case "workspace_groups":
				c.SkillSources[skillIdx].WorkspaceGroups = items
			case "scope":
				c.SkillSources[skillIdx].Scope = normalizeScope(items[0])
			default:
				return p.errf("unknown key %q in [[skill_source]]", key)
			}
		case curFileGroup:
			switch key {
			case "name":
				c.FileGroups[fgIdx].Name = items[0]
			case "path":
				c.FileGroups[fgIdx].Paths = items
			case "use":
				c.FileGroups[fgIdx].Uses = items
			default:
				return p.errf("unknown key %q in [[file_group]]", key)
			}
		case curWSGroup:
			switch key {
			case "name":
				c.WorkspaceGroups[wgIdx].Name = items[0]
			case "workspace":
				c.WorkspaceGroups[wgIdx].Workspaces = items
			case "use":
				c.WorkspaceGroups[wgIdx].Uses = items
			default:
				return p.errf("unknown key %q in [[workspace_group]]", key)
			}
		case curSkillGroup:
			switch key {
			case "name":
				c.SkillGroups[sgIdx].Name = items[0]
			case "path":
				c.SkillGroups[sgIdx].Paths = items
			case "use":
				c.SkillGroups[sgIdx].Uses = items
			default:
				return p.errf("unknown key %q in [[skill_group]]", key)
			}
		case curNone:
			// 顶层（任何表之前 / 未知普通表）键：只认 fetch_timeout，其余静默忽略（向后兼容）。
			if key == "fetch_timeout" && !isArray {
				c.FetchTimeout = items[0]
			}
		}
	}
}

// ---- 字符级原语 ----

func (p *tomlParser) eof() bool { return p.pos >= len(p.src) }

// peek 看当前字符；文件尾返回 0。
func (p *tomlParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

// at 看当前起第 i 个字符；越界返回 0。
func (p *tomlParser) at(i int) byte {
	if p.pos+i >= len(p.src) {
		return 0
	}
	return p.src[p.pos+i]
}

// advance 消费一个字符并维护行号。
func (p *tomlParser) advance() byte {
	ch := p.src[p.pos]
	p.pos++
	if ch == '\n' {
		p.row++
	}
	return ch
}

func (p *tomlParser) hasPrefix(s string) bool {
	return strings.HasPrefix(p.src[p.pos:], s)
}

func (p *tomlParser) errf(format string, args ...any) error {
	return p.errAt(p.row, format, args...)
}

// errAt 在指定行报错 —— 「未闭合」类错误报构造起始行（如多行数组的 '[' 所在行），
// 比报 EOF 行更能定位。
func (p *tomlParser) errAt(row int, format string, args ...any) error {
	return fmt.Errorf("config %s:%d: %s", p.path, row, fmt.Sprintf(format, args...))
}

// skipWS 跳过空格、制表符与 CR（CRLF 行尾的 \r 在此消化）。
func (p *tomlParser) skipWS() {
	for !p.eof() {
		switch p.peek() {
		case ' ', '\t', '\r':
			p.advance()
		default:
			return
		}
	}
}

// skipBlank 跳过语句间的空白、换行与注释（数组内同样合法）。
func (p *tomlParser) skipBlank() {
	for {
		p.skipWS()
		if p.peek() == '\n' {
			p.advance()
			continue
		}
		if p.peek() == '#' {
			for !p.eof() && p.peek() != '\n' {
				p.advance()
			}
			continue
		}
		return
	}
}

// expectLineEnd 值之后只允许空白与注释，然后是换行或文件尾。
func (p *tomlParser) expectLineEnd() error {
	p.skipWS()
	if p.peek() == '#' {
		for !p.eof() && p.peek() != '\n' {
			p.advance()
		}
	}
	if p.eof() || p.peek() == '\n' {
		return nil
	}
	return p.errf("unexpected content after value")
}

func isBareKeyByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' || c == '_' || c == '-'
}

// isWSOrNL 报告是否为空白或换行（多行串行尾续行要连空行一起吞掉）。
func isWSOrNL(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// ---- 键 ----

// parseKey 解析键：裸键（A-Za-z0-9_-）或引号键（"..." / '...'，单行）。
// 点键（a.b）显式报错 —— 本配置 schema 无嵌套表，不能静默忽略。
func (p *tomlParser) parseKey() (string, error) {
	if ch := p.peek(); ch == '"' || ch == '\'' {
		if p.hasPrefix(`"""`) || p.hasPrefix(`'''`) {
			return "", p.errf("multi-line strings are not allowed in keys")
		}
		return p.parseSingleLineString()
	}
	start := p.pos
	for !p.eof() && isBareKeyByte(p.peek()) {
		p.advance()
	}
	if p.pos == start {
		return "", p.errf("expected a key")
	}
	if p.peek() == '.' {
		return "", p.errf("dotted keys (a.b) are not supported")
	}
	return p.src[start:p.pos], nil
}

// ---- 表头 ----

// parseTableHeader 解析表头 [[name]] / [name]（当前位置在第一个 '['）。
// name 支持裸键与引号写法；点号表名显式报错。
func (p *tomlParser) parseTableHeader() (name string, isArray bool, err error) {
	isArray = p.at(1) == '['
	p.advance()
	if isArray {
		p.advance()
	}
	p.skipWS()
	if ch := p.peek(); ch == '"' || ch == '\'' {
		if p.hasPrefix(`"""`) || p.hasPrefix(`'''`) {
			return "", false, p.errf("multi-line strings are not allowed in table names")
		}
		name, err = p.parseSingleLineString()
		if err != nil {
			return "", false, err
		}
	} else {
		start := p.pos
		for !p.eof() && isBareKeyByte(p.peek()) {
			p.advance()
		}
		name = p.src[start:p.pos]
		if name == "" {
			return "", false, p.errf("expected a table name")
		}
	}
	p.skipWS()
	if p.peek() == '.' {
		return "", false, p.errf("dotted table names (a.b) are not supported")
	}
	if isArray {
		if !p.hasPrefix("]]") {
			return "", false, p.errf("expected ']]' to close the table header")
		}
		p.advance()
		p.advance()
	} else {
		if p.peek() != ']' {
			return "", false, p.errf("expected ']' to close the table header")
		}
		p.advance()
	}
	return name, isArray, nil
}

// ---- 值 ----

// parseValue 解析 = 右侧的值。本配置 schema 只用字符串与字符串数组：
// 数组可跨行、可含注释与尾逗号，元素须为字符串（嵌套数组显式报错）。
// 返回的 items 对单串是单元素切片，对数组是各元素 —— 两种写法对所有字段等价。
func (p *tomlParser) parseValue() (items []string, isArray bool, err error) {
	if p.peek() != '[' {
		s, err := p.parseString()
		if err != nil {
			return nil, false, err
		}
		return []string{s}, false, nil
	}
	p.advance()       // '['
	startRow := p.row // advance 单字符不跨行，p.row 即 '[' 所在行
	for {
		p.skipBlank() // 数组内允许换行、空行与注释
		if p.eof() {
			return nil, false, p.errAt(startRow, "unterminated array (missing ']')")
		}
		if p.peek() == ']' {
			p.advance()
			return items, true, nil
		}
		if p.peek() == '[' {
			return nil, false, p.errf("nested arrays are not supported")
		}
		s, err := p.parseString()
		if err != nil {
			return nil, false, err
		}
		items = append(items, s)
		p.skipBlank()
		if p.peek() == ',' {
			p.advance()
			continue
		}
		if p.peek() == ']' {
			p.advance()
			return items, true, nil
		}
		if p.eof() {
			return nil, false, p.errAt(startRow, "unterminated array (missing ']')")
		}
		return nil, false, p.errf("expected ',' or ']' in array")
	}
}

// parseString 解析一个字符串值：基本串（含转义）、字面串、多行基本串、多行字面串。
func (p *tomlParser) parseString() (string, error) {
	if p.hasPrefix(`"""`) {
		return p.parseMultiLine('"')
	}
	if p.hasPrefix(`'''`) {
		return p.parseMultiLine('\'')
	}
	return p.parseSingleLineString()
}

func (p *tomlParser) parseSingleLineString() (string, error) {
	quote := p.peek()
	if quote != '"' && quote != '\'' {
		if p.eof() {
			return "", p.errf("expected a value after '='")
		}
		return "", p.errf("expected a quoted string value (this config only takes strings and arrays of strings)")
	}
	p.advance()
	startRow := p.row
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errAt(startRow, "unterminated string")
		}
		ch := p.peek()
		if ch == '\n' {
			return "", p.errAt(startRow, "unterminated string (a string must be closed on the same line)")
		}
		if ch == quote {
			p.advance()
			return b.String(), nil
		}
		if quote == '"' && ch == '\\' {
			p.advance()
			if err := p.readEscape(&b); err != nil {
				return "", err
			}
			continue
		}
		b.WriteByte(p.advance())
	}
}

// parseMultiLine 解析多行字符串（三引号包裹，quote 为基本串 " 或字面串 ' 两种）：
// 首个紧邻换行被修剪；基本多行串支持转义与行尾 \ 续行（吞掉后续所有空白与换行）；
// 收尾分隔符前允许最多两个同引号字符（TOML："""" 表示内容以一个引号结尾）。
func (p *tomlParser) parseMultiLine(quote byte) (string, error) {
	startRow := p.row
	p.advance()
	p.advance()
	p.advance()
	if p.peek() == '\r' && p.at(1) == '\n' {
		p.advance()
		p.advance()
	} else if p.peek() == '\n' {
		p.advance()
	}
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errAt(startRow, "unterminated multi-line string")
		}
		if p.peek() == quote && p.at(1) == quote && p.at(2) == quote {
			p.advance()
			p.advance()
			p.advance()
			for extra := 0; extra < 2 && p.peek() == quote; extra++ {
				b.WriteByte(p.advance())
			}
			return b.String(), nil
		}
		ch := p.peek()
		if quote == '"' && ch == '\\' {
			// 行尾续行：\ 后仅剩空白与换行 ⇒ 全部吞掉（含后续所有空行）
			j := p.pos + 1
			for j < len(p.src) && (p.src[j] == ' ' || p.src[j] == '\t' || p.src[j] == '\r') {
				j++
			}
			if j < len(p.src) && p.src[j] == '\n' {
				p.pos = j
				p.advance() // 过 '\n'（row++）
				for !p.eof() && isWSOrNL(p.peek()) {
					p.advance()
				}
				continue
			}
			p.advance() // 过 '\'
			if err := p.readEscape(&b); err != nil {
				return "", err
			}
			continue
		}
		b.WriteByte(p.advance())
	}
}

// readEscape 读入一个转义序列（调用时已消费 '\'）并按 TOML 标准解码。
func (p *tomlParser) readEscape(b *strings.Builder) error {
	if p.eof() {
		return p.errf("unterminated escape sequence")
	}
	ch := p.advance()
	switch ch {
	case 'b':
		b.WriteByte('\b')
	case 't':
		b.WriteByte('\t')
	case 'n':
		b.WriteByte('\n')
	case 'f':
		b.WriteByte('\f')
	case 'r':
		b.WriteByte('\r')
	case '"':
		b.WriteByte('"')
	case '\\':
		b.WriteByte('\\')
	case 'u', 'U':
		n := 4
		if ch == 'U' {
			n = 8
		}
		if p.pos+n > len(p.src) {
			return p.errf("invalid \\%c escape: too short", ch)
		}
		hex := p.src[p.pos : p.pos+n]
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return p.errf("invalid \\%c escape %q", ch, hex)
		}
		p.pos += n
		b.WriteRune(rune(v))
	default:
		return p.errf("invalid escape sequence \\%c (Windows paths: use a literal string '...' or forward slashes)", ch)
	}
	return nil
}

// expandHomes 把配置里所有路径值中的 "~" / "~/" 展开为当前用户的 HOME，
// 使同一份配置能跨机器、跨用户复用（不必硬编码 /Users/xxx 这类前缀）。
//
// 覆盖七处：Source.Paths、Source.Workspaces、SkillSource.Paths、SkillSource.Workspaces、
// FileGroup.Paths、WorkspaceGroup.Workspaces、SkillGroup.Paths。
// 含 glob 的值（如 "~/proj/*"）同样先展开再匹配，单段 "*" / 跨段 "**" 的语义不变。
//
// 展开逻辑复用 agents.ExpandHome（全项目 "~" 展开的单一真源），此处不做第二份实现；
// 取不到 HOME 时它会原样返回，绝不静默改坏用户写的路径。
func (c *Config) expandHomes() {
	for i := range c.Sources {
		c.Sources[i].Paths = expandHomesAll(c.Sources[i].Paths)
		c.Sources[i].Workspaces = expandHomesAll(c.Sources[i].Workspaces)
	}
	for i := range c.SkillSources {
		c.SkillSources[i].Paths = expandHomesAll(c.SkillSources[i].Paths)
		c.SkillSources[i].Workspaces = expandHomesAll(c.SkillSources[i].Workspaces)
	}
	for i := range c.FileGroups {
		c.FileGroups[i].Paths = expandHomesAll(c.FileGroups[i].Paths)
	}
	for i := range c.WorkspaceGroups {
		c.WorkspaceGroups[i].Workspaces = expandHomesAll(c.WorkspaceGroups[i].Workspaces)
	}
	for i := range c.SkillGroups {
		c.SkillGroups[i].Paths = expandHomesAll(c.SkillGroups[i].Paths)
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

// SourcesFor 返回在指定工作区下、应投递给指定 agent 的源规则（**仅 workspace 域**）。
//   - agents 为空 ⇒ 该条投递给全部 agent
//   - workspace 为空 / 含 "*" / "all" ⇒ 该条适用于全部工作区
//   - scope 为 user 的条目不在此列（走 UserSourcesFor）
func (c *Config) SourcesFor(agentID, workspace string) []Source {
	out := make([]Source, 0, len(c.Sources))
	for _, s := range c.Sources {
		if s.IsUserScope() {
			continue
		}
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

// SourcesForAny 返回在指定工作区下、应投递给 agentIDs 中**任一** agent 的源规则的**并集**
// （**仅 workspace 域**）。
//
// 与 SourcesFor 的差别只在 agent 那一维：命中任一 id 即收录（agents 为空视为命中全部），
// 且每条 source 只收录一次、保持 c.Sources 原序 —— 所以对共享规则目录的多个 agent
// 传进它们的 ID 列表，得到的就是该目录「应有文件」的完整集合。
//
// 用途：共享同一工作区规则目录的 agent（如 codebuddy 与 workbuddy 都落 .codebuddy/rules）
// 必须按并集同步，否则 engine 的删残留会把另一方的文件当残留删掉。
//
// ⚠ 不要拿 SourcesFor("", ws) 当「全量」用：那只匹配 agents 字段为空的 source，语义不同。
func (c *Config) SourcesForAny(agentIDs []string, workspace string) []Source {
	if len(agentIDs) == 0 {
		// 没有任何 agent 在要 ⇒ 该目录不该有任何文件（卸载收敛到"无人使用"时的全清场景）。
		// 不能落到下面的循环：agents 字段为空的 source 会被误判为命中，文件反而被留下。
		return nil
	}
	out := make([]Source, 0, len(c.Sources))
	for _, s := range c.Sources {
		if s.IsUserScope() {
			continue
		}
		if len(s.Agents) != 0 && !containsAnyFold(s.Agents, agentIDs) {
			continue
		}
		if !s.MatchesWorkspace(workspace) {
			continue
		}
		out = append(out, s) // 一条只收录一次：命中任一 id 即 break 出循环
	}
	return out
}

// UserSourcesFor 返回应投递给指定 agent 的用户级（全局）规则源。
// 用户级目录与工作区无关 ⇒ 忽略 workspace 匹配。
func (c *Config) UserSourcesFor(agentID string) []Source {
	return c.UserSourcesForAny([]string{agentID})
}

// UserSourcesForAny 返回应投递给 agentIDs 中任一 agent 的用户级规则源并集（忽略 workspace）。
func (c *Config) UserSourcesForAny(agentIDs []string) []Source {
	if len(agentIDs) == 0 {
		return nil
	}
	out := make([]Source, 0, len(c.Sources))
	for _, s := range c.Sources {
		if !s.IsUserScope() {
			continue
		}
		if len(s.Agents) != 0 && !containsAnyFold(s.Agents, agentIDs) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// SkillSourcesFor 返回在指定工作区下、应投递给指定 agent 的 skill 源（**仅 workspace 域**）。
func (c *Config) SkillSourcesFor(agentID, workspace string) []SkillSource {
	out := make([]SkillSource, 0, len(c.SkillSources))
	for _, s := range c.SkillSources {
		if s.IsUserScope() {
			continue
		}
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

// SkillSourcesForAny 返回在指定工作区下、应投递给 agentIDs 中任一 agent 的 skill 源并集
// （**仅 workspace 域**）。共享 skills 目录（如 .codebuddy/skills）按并集同步，同 rules。
func (c *Config) SkillSourcesForAny(agentIDs []string, workspace string) []SkillSource {
	if len(agentIDs) == 0 {
		return nil
	}
	out := make([]SkillSource, 0, len(c.SkillSources))
	for _, s := range c.SkillSources {
		if s.IsUserScope() {
			continue
		}
		if len(s.Agents) != 0 && !containsAnyFold(s.Agents, agentIDs) {
			continue
		}
		if !s.MatchesWorkspace(workspace) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// UserSkillSourcesForAny 返回应投递给 agentIDs 中任一 agent 的用户级 skill 源并集（忽略 workspace）。
func (c *Config) UserSkillSourcesForAny(agentIDs []string) []SkillSource {
	if len(agentIDs) == 0 {
		return nil
	}
	out := make([]SkillSource, 0, len(c.SkillSources))
	for _, s := range c.SkillSources {
		if !s.IsUserScope() {
			continue
		}
		if len(s.Agents) != 0 && !containsAnyFold(s.Agents, agentIDs) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// HasAnyFor 报告任一域（工作区/用户级 × 规则/skill）是否有投给 agentIDs 任一的源。
// 供 cascade 判断「这个 agent 还有没有可同步的东西」。
func (c *Config) HasAnyFor(agentIDs []string, workspace string) bool {
	return len(c.SourcesForAny(agentIDs, workspace)) > 0 ||
		len(c.SkillSourcesForAny(agentIDs, workspace)) > 0 ||
		len(c.UserSourcesForAny(agentIDs)) > 0 ||
		len(c.UserSkillSourcesForAny(agentIDs)) > 0
}

// containsAnyFold 报告 list 与 ids 是否存在交集（忽略大小写）。
func containsAnyFold(list, ids []string) bool {
	for _, id := range ids {
		if containsFold(list, id) {
			return true
		}
	}
	return false
}

// DeclaresWorkspace 报告「该工作区是否被配置里任何一条 [[source]] / [[skill_source]] 覆盖」。
//
// 用途：sync 的守卫 —— 若 cwd 不属于配置声明的工作区，则整体跳过（不建、不删、
// 不记账）。否则「空 want ⇒ 删残留」会清空一个我们无权管辖的目录。
// 见 docs/design/features/sync-all-and-change-notice.md §二「清空的正确姿势」/ §八。
//
// 注意：省略 workspace、或写 "*"/"**"/"all" 的条目适用于所有工作区；scope=user 的条目
// 本就与工作区无关 ⇒ 同样视为「已声明」。因此只要配置里有这类条目，任意路径都会被判为已声明。
func (c *Config) DeclaresWorkspace(ws string) bool {
	for _, s := range c.Sources {
		if s.IsUserScope() || s.MatchesWorkspace(ws) {
			return true
		}
	}
	for _, s := range c.SkillSources {
		if s.IsUserScope() || s.MatchesWorkspace(ws) {
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
	return matchesWorkspace(s.Workspaces, ws)
}

// MatchesWorkspace 与 Source.MatchesWorkspace 同语义（skill 源复用同一匹配器）。
func (s SkillSource) MatchesWorkspace(ws string) bool {
	return matchesWorkspace(s.Workspaces, ws)
}

// matchesWorkspace 是 workspace 匹配的唯一实现（Source 与 SkillSource 共用）。
func matchesWorkspace(workspaces []string, ws string) bool {
	if len(workspaces) == 0 {
		return true
	}
	for _, w := range workspaces {
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
	if len(c.Sources) == 0 && len(c.SkillSources) == 0 {
		return fmt.Errorf("config %s has no [[source]] or [[skill_source]]; run rulemux init first to generate a sample config and fill in your source files", c.File)
	}
	for i, s := range c.Sources {
		if len(s.Paths) == 0 {
			return fmt.Errorf("config %s: [[source]] #%d is missing path", c.File, i+1)
		}
		for _, pth := range s.Paths {
			if strings.TrimSpace(pth) == "" {
				return fmt.Errorf("config %s: [[source]] #%d has an empty path", c.File, i+1)
			}
		}
		if !validScope(s.Scope) {
			return fmt.Errorf("config %s: [[source]] #%d has unknown scope %q (expected \"workspace\" or \"user\")", c.File, i+1, s.Scope)
		}
	}
	for i, s := range c.SkillSources {
		if len(s.Paths) == 0 {
			return fmt.Errorf("config %s: [[skill_source]] #%d is missing path", c.File, i+1)
		}
		for _, pth := range s.Paths {
			if strings.TrimSpace(pth) == "" {
				return fmt.Errorf("config %s: [[skill_source]] #%d has an empty path", c.File, i+1)
			}
		}
		if !validScope(s.Scope) {
			return fmt.Errorf("config %s: [[skill_source]] #%d has unknown scope %q (expected \"workspace\" or \"user\")", c.File, i+1, s.Scope)
		}
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

// containsFold 忽略大小写判断切片是否包含某字符串。
func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), v) {
			return true
		}
	}
	return false
}

// resolveGroups 把配置中的“组”引用展开成具体的文件 / 工作区 / skill 根目录，
// 并写回每条 Source / SkillSource 的 Paths / Workspaces。展开过程做环检测与按值去重，
// 因此组间重复项最终只出现一次（满足“每个条目只处理一次”）。
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
	sgMap := make(map[string]*SkillGroup, len(c.SkillGroups))
	for i := range c.SkillGroups {
		g := &c.SkillGroups[i]
		if g.Name == "" {
			return fmt.Errorf("config %s has an unnamed [[skill_group]]", c.File)
		}
		if _, dup := sgMap[g.Name]; dup {
			return fmt.Errorf("config %s: duplicate [[skill_group]] name %q", c.File, g.Name)
		}
		sgMap[g.Name] = g
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

	// 递归展开 skill 组（与文件组同构：嵌套 + 环检测；值是 skill 根目录）。
	var expandSG func(name string, visiting map[string]bool, out *[]string) error
	expandSG = func(name string, visiting map[string]bool, out *[]string) error {
		g, ok := sgMap[name]
		if !ok {
			return fmt.Errorf("config %s references undefined skill group %q", c.File, name)
		}
		if visiting[name] {
			return fmt.Errorf("config %s has a circular reference in skill groups: %s", c.File, name)
		}
		visiting[name] = true
		defer delete(visiting, name)
		for _, u := range g.Uses {
			if err := expandSG(u, visiting, out); err != nil {
				return err
			}
		}
		appendUnique(out, g.Paths)
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

	for i := range c.SkillSources {
		s := &c.SkillSources[i]
		var roots []string
		for _, gname := range s.Groups {
			if err := expandSG(gname, map[string]bool{}, &roots); err != nil {
				return err
			}
		}
		// skill 组展开的根在前，skill_source 自身显式 path 在后（同 [[source]] 口径）。
		s.Paths = mergeUnique(roots, s.Paths)

		var wss []string
		for _, gname := range s.WorkspaceGroups {
			if err := expandWG(gname, map[string]bool{}, &wss); err != nil {
				return err
			}
		}
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

// canonicalizeAgents 把每条 source / skill_source 的 agents 值归一为注册表里的规范 ID（别名 → 规范 ID）。
//
// 否则用户在配置里写别名（如 agents = ["workbuddy"]）时，SourcesFor 用规范 ID 去匹配
// 会**静默漏投**。这是「别名归一」四个入口之一（见设计 §十 #4）。
func (c *Config) canonicalizeAgents() {
	for i := range c.Sources {
		c.Sources[i].Agents = canonicalAgents(c.Sources[i].Agents)
	}
	for i := range c.SkillSources {
		c.SkillSources[i].Agents = canonicalAgents(c.SkillSources[i].Agents)
	}
}

// normalizeScope 归一化作用域值：空串按 workspace（默认）处理；其余原样返回，
// 由 Validate 拒绝无法识别的值（宁可报错也不静默按默认域投放）。
func normalizeScope(s string) string {
	if s == "" {
		return ScopeWorkspace
	}
	return s
}

// normalizeScopes 把省略 scope 的条目补默认值（解析期只有显式写出 scope 才会赋值）。
func (c *Config) normalizeScopes() {
	for i := range c.Sources {
		c.Sources[i].Scope = normalizeScope(c.Sources[i].Scope)
	}
	for i := range c.SkillSources {
		c.SkillSources[i].Scope = normalizeScope(c.SkillSources[i].Scope)
	}
}

// validScope 报告作用域值是否可识别。
func validScope(s string) bool {
	return s == "" || strings.EqualFold(s, ScopeWorkspace) || strings.EqualFold(s, ScopeUser)
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
