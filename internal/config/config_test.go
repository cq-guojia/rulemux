package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad 校验自写的 TOML 子集解析器：
// 顶层 workspace、注释、[[source]]、path 数组、单引号 path、agents。
func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `# rulemux 配置
[[source]]
path = ["C:/rules/a.md", "C:/rules/style.md"]
agents = ["claude", "codex"]

[[source]]
path = 'D:/notes/b.txt'
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) != 2 {
		t.Fatalf("期望 2 条 source，实得 %d", len(c.Sources))
	}
	if len(c.Sources[0].Paths) != 2 || c.Sources[0].Paths[0] != "C:/rules/a.md" {
		t.Fatalf("path 数组解析错误：%v", c.Sources[0].Paths)
	}
	if len(c.Sources[0].Agents) != 2 || c.Sources[0].Agents[0] != "claude" {
		t.Fatalf("agents 解析错误：%v", c.Sources[0].Agents)
	}
	if len(c.Sources[1].Paths) != 1 || c.Sources[1].Paths[0] != "D:/notes/b.txt" {
		t.Fatalf("单引号 path 解析错误：%v", c.Sources[1].Paths)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate 不该报错：%v", err)
	}
}

// TestTrailingComment 校验行尾注释（TOML 标准语法）：键值与表头都能带尾注释，
// 引号内的 "#" 是值的一部分、不能被当注释剥掉；注释里的引号（don't）不误伤。
// 回归保护：修复前 ["base"] # 尾注释 会被整体当成组名，报 undefined file group。
func TestTrailingComment(t *testing.T) {
	c := writeCfg(t, `[[file_group]]          # 组头也能带尾注释
name = "base"              # 组名
path = ["/rules/a.md", "/rules/b.md"]   # 组内文件

[[source]]
groups = ["base"]          # 尾注释
path = '/rules/a#b.md'     # 引号内的 # 属于路径，不是注释
agents = ["codebuddy"]     # 只投一家
scope = "user"# 无空格紧跟也要剥
`)
	if len(c.FileGroups) != 1 || c.FileGroups[0].Name != "base" {
		t.Fatalf("带尾注释的 [[file_group]] 表头/键值解析错误：%+v", c.FileGroups)
	}
	if got := c.FileGroups[0].Paths; !equalSlice(got, []string{"/rules/a.md", "/rules/b.md"}) {
		t.Fatalf("file_group.path 带尾注释解析错误：%v", got)
	}
	if len(c.Sources) != 1 {
		t.Fatalf("带尾注释的 [[source]] 应解析出 1 条，实得 %d", len(c.Sources))
	}
	s := c.Sources[0]
	if !equalSlice(s.Groups, []string{"base"}) {
		t.Fatalf("groups 带尾注释解析错误：%v", s.Groups)
	}
	// 组展开的文件在前，source 自身显式 path 在后；单引号值内的 # 必须保留
	want := []string{"/rules/a.md", "/rules/b.md", "/rules/a#b.md"}
	if !equalSlice(s.Paths, want) {
		t.Fatalf("展开后 Paths 应为 %v，实得 %v", want, s.Paths)
	}
	if !s.IsUserScope() {
		t.Fatalf("scope 带尾注释应归一为 user，实得 %q", s.Scope)
	}
	if !equalSlice(s.Agents, []string{"codebuddy"}) {
		t.Fatalf("agents 带尾注释解析错误：%v", s.Agents)
	}
}

// TestStripTrailingComment 单测剥注释的引号状态机：引号内的 # 保留、
// 注释里的引号（don't）不误伤、无 # 原样返回。
func TestStripTrailingComment(t *testing.T) {
	cases := []struct{ in, want string }{
		{`path = ["a.md"] # 尾注释`, `path = ["a.md"]`},
		{`path = "/rules/a#b.md"`, `path = "/rules/a#b.md"`},
		{`path = '/a#b.md' # c`, `path = '/a#b.md'`},
		{`[[source]] # 注释`, `[[source]]`},
		{`# 整行注释`, ``},
		{`path = "a.md"# 无空格`, `path = "a.md"`},
		{`path = ["a.md"] # don't panic`, `path = ["a.md"]`},
		{`path = "a.md"`, `path = "a.md"`},
	}
	for _, c := range cases {
		if got := stripTrailingComment(c.in); got != c.want {
			t.Errorf("stripTrailingComment(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSinglePath 校验 path 仍可写单个字符串（向后兼容）。
func TestSinglePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte("[[source]]\npath = \"a.md\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) != 1 || len(c.Sources[0].Paths) != 1 || c.Sources[0].Paths[0] != "a.md" {
		t.Fatalf("单字符串 path 解析错误：%+v", c.Sources)
	}
}

// TestSourcesFor 校验 agents 过滤：显式列出命中，省略（空）表示投递给全部。
func TestSourcesFor(t *testing.T) {
	c := &Config{Sources: []Source{
		{Paths: []string{"a"}, Agents: []string{"claude"}},
		{Paths: []string{"b"}},
	}}
	if got := c.SourcesFor("claude", "/any"); len(got) != 2 {
		t.Fatalf("claude 应命中 2 条（显式 + 全部），实得 %d", len(got))
	}
	if got := c.SourcesFor("trae", "/any"); len(got) != 1 {
		t.Fatalf("trae 应只命中「全部」那 1 条，实得 %d", len(got))
	}
}

// TestWorkspaceMatching 校验 workspace 过滤：
// 省略 / "*" / "all" = 所有工作区；单个或数组按路径匹配。
func TestWorkspaceMatching(t *testing.T) {
	c := &Config{Sources: []Source{
		{Paths: []string{"omit"}},                              // 省略 = 所有
		{Paths: []string{"star"}, Workspaces: []string{"*"}},   // * = 所有
		{Paths: []string{"word"}, Workspaces: []string{"all"}}, // all = 所有
		{Paths: []string{"single"}, Workspaces: []string{"/tmp/proj1"}},
		{Paths: []string{"multi"}, Workspaces: []string{"/tmp/p2", "/tmp/p3"}},
	}}

	// /tmp/proj1：omit + star + word + single = 4 条，multi 不命中
	if got := c.SourcesFor("claude", "/tmp/proj1"); len(got) != 4 {
		t.Fatalf("/tmp/proj1 应命中 4 条，实得 %d", len(got))
	}
	// /tmp/p3：omit + star + word + multi = 4 条，single 不命中
	if got := c.SourcesFor("claude", "/tmp/p3"); len(got) != 4 {
		t.Fatalf("/tmp/p3 应命中 4 条，实得 %d", len(got))
	}
	// /tmp/other：只有 omit + star + word = 3 条
	if got := c.SourcesFor("claude", "/tmp/other"); len(got) != 3 {
		t.Fatalf("/tmp/other 应命中 3 条，实得 %d", len(got))
	}
}

// TestWorkspaceConfigParsing 校验 workspace 单值与数组都能解析。
func TestWorkspaceConfigParsing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `[[source]]
path = "a.md"
workspace = "/one/path"

[[source]]
path = "b.md"
workspace = ["/two/path", "/three/path"]
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources[0].Workspaces) != 1 || c.Sources[0].Workspaces[0] != "/one/path" {
		t.Fatalf("单值 workspace 解析错误：%v", c.Sources[0].Workspaces)
	}
	if len(c.Sources[1].Workspaces) != 2 || c.Sources[1].Workspaces[1] != "/three/path" {
		t.Fatalf("数组 workspace 解析错误：%v", c.Sources[1].Workspaces)
	}
}

func TestValidateEmpty(t *testing.T) {
	c := &Config{File: "x.toml"}
	if err := c.Validate(); err == nil {
		t.Fatal("空配置应校验失败")
	}
}

// TestWorkspaceGlob 校验业界标准 glob：* 单段、** 跨段递归、可出现在中间段。
func TestWorkspaceGlob(t *testing.T) {
	c := &Config{Sources: []Source{
		{Paths: []string{"recursive"}, Workspaces: []string{"/abs/**/B"}},
		{Paths: []string{"single"}, Workspaces: []string{"/abs/*/B"}},
	}}
	// /abs/A/B：同时命中 ** 与 *
	if got := c.SourcesFor("claude", "/abs/A/B"); len(got) != 2 {
		t.Fatalf("/abs/A/B 应命中 2 条（** 与 *），实得 %d", len(got))
	}
	// /abs/A/deep/B：只命中 **（* 不跨段）
	if got := c.SourcesFor("claude", "/abs/A/deep/B"); len(got) != 1 {
		t.Fatalf("/abs/A/deep/B 应只命中 1 条（**），实得 %d", len(got))
	}
	// /other/B：都不命中
	if got := c.SourcesFor("claude", "/other/B"); len(got) != 0 {
		t.Fatalf("/other/B 应命中 0 条，实得 %d", len(got))
	}
}

// equalSlice 比较两个字符串切片是否逐项相等（顺序敏感）。
func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestFileGroupExpand 校验文件组展开：嵌套引用会把组内所有文件合并进 source.Paths。
func TestFileGroupExpand(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `
[[file_group]]
name = "base"
path = ["/rules/a.md", "/rules/b.md"]

[[file_group]]
name = "proj"
use = ["base"]
path = ["/rules/c.md"]

[[source]]
groups = ["base", "proj"]
agents = ["codebuddy"]
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Sources) != 1 {
		t.Fatalf("期望 1 条 source，实得 %d", len(c.Sources))
	}
	got := c.Sources[0].Paths
	want := []string{"/rules/a.md", "/rules/b.md", "/rules/c.md"}
	if !equalSlice(got, want) {
		t.Fatalf("展开后 Paths 应为 %v，实得 %v", want, got)
	}
}

// TestGroupDedup 校验组间重复文件按源路径去重，每个文件只出现一次。
func TestGroupDedup(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `
[[file_group]]
name = "base"
path = ["/rules/a.md", "/rules/b.md"]

[[file_group]]
name = "proj"
use = ["base"]
path = ["/rules/a.md", "/rules/c.md"]

[[source]]
groups = ["base", "proj"]
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Sources[0].Paths
	want := []string{"/rules/a.md", "/rules/b.md", "/rules/c.md"}
	if !equalSlice(got, want) {
		t.Fatalf("去重后 Paths 应为 %v，实得 %v", want, got)
	}
}

// TestWorkspaceGroupExpandAndGlob 校验工作区分组展开，且 glob 原样保留。
func TestWorkspaceGroupExpandAndGlob(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `
[[workspace_group]]
name = "dev"
workspace = ["/proj/1", "/proj/2"]

[[workspace_group]]
name = "qa"
use = ["dev"]
workspace = ["/proj/1/**"]

[[source]]
path = ["/rules/a.md"]
workspace_groups = ["dev", "qa"]
workspace = ["/standalone"]
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Sources[0].Workspaces
	want := []string{"/proj/1", "/proj/2", "/proj/1/**", "/standalone"}
	if !equalSlice(got, want) {
		t.Fatalf("工作区展开后应为 %v，实得 %v", want, got)
	}
}

// TestMixedGroupsAndPaths 校验 [[source]] 可同时引用组与混列单个文件。
func TestMixedGroupsAndPaths(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `
[[file_group]]
name = "base"
path = ["/rules/a.md"]

[[source]]
groups = ["base"]
path = ["/rules/extra.md"]
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Sources[0].Paths
	want := []string{"/rules/a.md", "/rules/extra.md"}
	if !equalSlice(got, want) {
		t.Fatalf("混合引用展开后应为 %v，实得 %v", want, got)
	}
}

// TestGroupUndefinedError 校验引用未定义的组应报错。
func TestGroupUndefinedError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `[[source]]
groups = ["nope"]
path = ["/x.md"]
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("引用未定义组应报错")
	}
}

// TestGroupCycleError 校验文件组循环引用应报错。
func TestGroupCycleError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `
[[file_group]]
name = "A"
use = ["B"]

[[file_group]]
name = "B"
use = ["A"]

[[source]]
groups = ["A"]
path = ["/x.md"]
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("循环引用应报错")
	}
}

// TestDuplicateGroupNameError 校验同类表重复定义组名应报错。
func TestDuplicateGroupNameError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `
[[file_group]]
name = "x"
path = ["/a.md"]

[[file_group]]
name = "x"
path = ["/b.md"]

[[source]]
groups = ["x"]
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("重复定义组名应报错")
	}
}

// TestDeclaresWorkspace 校验 sync 守卫：该工作区是否被配置里任何一条 source 覆盖。
func TestDeclaresWorkspace(t *testing.T) {
	c := &Config{Sources: []Source{
		{Paths: []string{"a"}, Workspaces: []string{"/proj/a"}},
		{Paths: []string{"b"}, Workspaces: []string{"/proj/**"}},
	}}
	if !c.DeclaresWorkspace("/proj/a") {
		t.Fatal("/proj/a 应被声明")
	}
	if !c.DeclaresWorkspace("/proj/a/deep") {
		t.Fatal("/proj/** 应覆盖任意深度")
	}
	if c.DeclaresWorkspace("/tmp/other") {
		t.Fatal("/tmp/other 不该被声明")
	}

	// 省略 workspace 的全局 source ⇒ 任意路径都算已声明。
	global := &Config{Sources: []Source{{Paths: []string{"x"}}}}
	if !global.DeclaresWorkspace("/whatever") {
		t.Fatal("全局 source 应使任意路径都被声明")
	}

	// 空配置 ⇒ 谁都不声明。
	empty := &Config{}
	if empty.DeclaresWorkspace("/proj/a") {
		t.Fatal("空配置不该声明任何工作区")
	}
}

// tildeHome 造一个假的 HOME 并让 os.UserHomeDir() 认它，返回其绝对路径。
//
// 先 EvalSymlinks 是因为 MatchesWorkspace 会解析符号链接：macOS 上 t.TempDir() 常是
// /var/...（真实路径 /private/var/...），不先解析会让「配置的 ~/」与「解析后的工作区」
// 对不上而误判失败。
func tildeHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		home = t.TempDir()
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows 走这个变量
	return home
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestExpandHomes_Paths 校验配置里的 "~" / "~/" 被展开为 HOME，
// 而非 "~" 开头的绝对路径必须原样保留（回归保护）。
func TestExpandHomes_Paths(t *testing.T) {
	home := tildeHome(t)
	p := writeConfig(t, `[[file_group]]
name = "base"
path = ["~/00.RULES/100.BASE.md"]

[[source]]
path = "~/00.RULES/direct.md"

[[source]]
path = ["/abs/no-tilde.md", "~"]
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := c.FileGroups[0].Paths[0], filepath.Join(home, "00.RULES", "100.BASE.md"); got != want {
		t.Errorf("file_group.path 未展开: got %q, want %q", got, want)
	}
	if got, want := c.Sources[0].Paths[0], filepath.Join(home, "00.RULES", "direct.md"); got != want {
		t.Errorf("source.path 未展开: got %q, want %q", got, want)
	}
	// 绝对路径不许被动过
	if got := c.Sources[1].Paths[0]; got != "/abs/no-tilde.md" {
		t.Errorf("非 ~ 开头的绝对路径被改动: %q", got)
	}
	// 裸 "~" = HOME 本身
	if got := c.Sources[1].Paths[1]; got != home {
		t.Errorf("裸 ~ 应展开为 HOME 本身: got %q, want %q", got, home)
	}
}

// TestExpandHomes_WorkspaceGlob 校验含 glob 的 workspace 先展开再匹配，
// 单段 "*" 只匹配直接子目录一级的既有语义不变。
func TestExpandHomes_WorkspaceGlob(t *testing.T) {
	home := tildeHome(t)
	p := writeConfig(t, `[[source]]
path = ["~/00.RULES/a.md"]
workspace = ["~/Agent.Workspace/*"]
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := c.Sources[0].Workspaces[0], filepath.Join(home, "Agent.Workspace", "*"); got != want {
		t.Fatalf("workspace 未展开: got %q, want %q", got, want)
	}
	// 直接子目录一级 ⇒ 命中
	if !c.Sources[0].MatchesWorkspace(filepath.Join(home, "Agent.Workspace", "Explore")) {
		t.Error("~/Agent.Workspace/* 应匹配其直接子目录")
	}
	// 更深一层 ⇒ 不命中（单段 * 不递归）
	if c.Sources[0].MatchesWorkspace(filepath.Join(home, "Agent.Workspace", "a", "b")) {
		t.Error("单段 * 不该递归匹配更深层")
	}
	// 别的目录 ⇒ 不命中
	if c.Sources[0].MatchesWorkspace(filepath.Join(home, "Other", "x")) {
		t.Error("不该匹配 ~/Agent.Workspace 之外的目录")
	}
}

// TestExpandHomes_GroupInheritance 校验组引用（use）展开后拿到的同样是已展开的绝对路径
// —— 展开必须在 resolveGroups 之前完成，否则 "/a.md" 与 "~/a.md" 会被当成两个文件。
func TestExpandHomes_GroupInheritance(t *testing.T) {
	home := tildeHome(t)
	p := writeConfig(t, `[[file_group]]
name = "base"
path = ["~/00.RULES/100.BASE.md"]

[[file_group]]
name = "work"
use = ["base"]
path = ["~/00.RULES/200.WORKSPACE.md"]

[[source]]
groups = ["work"]
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		filepath.Join(home, "00.RULES", "100.BASE.md"),
		filepath.Join(home, "00.RULES", "200.WORKSPACE.md"),
	}
	got := c.Sources[0].Paths
	if len(got) != len(want) {
		t.Fatalf("继承后应有 %d 个文件, got %d: %v", len(want), len(got), got)
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("继承来的路径未展开或缺失: want %q in %v", w, got)
		}
	}
	for _, g := range got {
		if strings.HasPrefix(g, "~") {
			t.Errorf("组展开后仍残留 ~ 路径: %q", g)
		}
	}
}

// writeCfg 把内容写进临时目录并 Load，返回配置。
func writeCfg(t *testing.T, content string) *Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestSkillSourceParsing 校验 [[skill_source]] 解析：path 单值/数组、agents、workspace、scope。
func TestSkillSourceParsing(t *testing.T) {
	c := writeCfg(t, `[[skill_source]]
path = ["/s/skills-a", "/s/skills-b"]
agents = ["codebuddy"]
workspace = ["/w/one", "/w/two"]
scope = "user"

[[skill_source]]
path = "/s/other"
`)
	if len(c.SkillSources) != 2 {
		t.Fatalf("期望 2 条 skill_source，实得 %d", len(c.SkillSources))
	}
	s := c.SkillSources[0]
	if len(s.Paths) != 2 || s.Paths[0] != "/s/skills-a" {
		t.Fatalf("skill path 数组解析错误：%v", s.Paths)
	}
	if len(s.Agents) != 1 || s.Agents[0] != "codebuddy" {
		t.Fatalf("skill agents 解析错误：%v", s.Agents)
	}
	if len(s.Workspaces) != 2 || s.Workspaces[1] != "/w/two" {
		t.Fatalf("skill workspace 解析错误：%v", s.Workspaces)
	}
	if s.Scope != ScopeUser {
		t.Fatalf("scope 应归一为 %q，实得 %q", ScopeUser, s.Scope)
	}
	if !s.IsUserScope() {
		t.Fatal("IsUserScope 应为 true")
	}
	// 省略 scope ⇒ 默认 workspace
	if c.SkillSources[1].Scope != ScopeWorkspace || c.SkillSources[1].IsUserScope() {
		t.Fatalf("省略 scope 应默认 %q，实得 %q", ScopeWorkspace, c.SkillSources[1].Scope)
	}
	if len(c.SkillSources[1].Paths) != 1 || c.SkillSources[1].Paths[0] != "/s/other" {
		t.Fatalf("单字符串 skill path 解析错误：%v", c.SkillSources[1].Paths)
	}
}

// TestScopeDomainSplit 校验四个域的查询互不串门：
// workspace 查询只见 workspace 域，user 查询只见 user 域。
func TestScopeDomainSplit(t *testing.T) {
	c := writeCfg(t, `[[source]]
path = ["/r/ws.md"]

[[source]]
path = ["/r/user.md"]
scope = "user"

[[skill_source]]
path = ["/s/ws-skills"]

[[skill_source]]
path = ["/s/user-skills"]
scope = "user"
`)
	// 规则：workspace 域
	wsSrcs := c.SourcesForAny([]string{"codebuddy"}, "/w")
	if len(wsSrcs) != 1 || wsSrcs[0].Paths[0] != "/r/ws.md" {
		t.Fatalf("workspace 规则域应只有 ws.md，实得 %+v", wsSrcs)
	}
	// 规则：user 域（忽略 workspace）
	userSrcs := c.UserSourcesForAny([]string{"codebuddy"})
	if len(userSrcs) != 1 || userSrcs[0].Paths[0] != "/r/user.md" {
		t.Fatalf("user 规则域应只有 user.md，实得 %+v", userSrcs)
	}
	// skill：workspace 域
	wsSkills := c.SkillSourcesForAny([]string{"codebuddy"}, "/w")
	if len(wsSkills) != 1 || wsSkills[0].Paths[0] != "/s/ws-skills" {
		t.Fatalf("workspace skill 域应只有 ws-skills，实得 %+v", wsSkills)
	}
	// skill：user 域
	userSkills := c.UserSkillSourcesForAny([]string{"codebuddy"})
	if len(userSkills) != 1 || userSkills[0].Paths[0] != "/s/user-skills" {
		t.Fatalf("user skill 域应只有 user-skills，实得 %+v", userSkills)
	}
	// 空 agentIDs ⇒ 一律空（卸载收敛的「无人使用」场景不能被 agents 为空的源命中）
	if got := c.UserSourcesForAny(nil); got != nil {
		t.Fatalf("空 agentIDs 的 user 规则查询应为 nil，实得 %+v", got)
	}
	if got := c.SkillSourcesForAny(nil, "/w"); got != nil {
		t.Fatalf("空 agentIDs 的 workspace skill 查询应为 nil，实得 %+v", got)
	}
}

// TestValidateSkillOnly 校验：只有 [[skill_source]] 的配置合法；
// skill_source 缺 path / scope 非法都要报错。
func TestValidateSkillOnly(t *testing.T) {
	ok := writeCfg(t, `[[skill_source]]
path = ["/s/skills"]
`)
	if err := ok.Validate(); err != nil {
		t.Fatalf("只有 skill_source 的配置应合法：%v", err)
	}
	if got := ok.HasAnyFor([]string{"codebuddy"}, "/w"); !got {
		t.Fatal("HasAnyFor 应报告有源")
	}

	noPath := writeCfg(t, `[[skill_source]]
agents = ["codebuddy"]
`)
	if err := noPath.Validate(); err == nil {
		t.Fatal("skill_source 缺 path 应报错")
	}

	badScope := writeCfg(t, `[[skill_source]]
path = ["/s/skills"]
scope = "galaxy"
`)
	if err := badScope.Validate(); err == nil {
		t.Fatal("非法 scope 应报错")
	}

	badScopeSrc := writeCfg(t, `[[source]]
path = ["/r/a.md"]
scope = "galaxy"
`)
	if err := badScopeSrc.Validate(); err == nil {
		t.Fatal("source 的非法 scope 应报错")
	}
}

// TestDeclaresWorkspaceUserScope 校验：任一 user 域条目 ⇒ 任意工作区都算「已声明」
// （用户级目录本就与工作区无关）；纯 workspace 域条目仍按原语义。
func TestDeclaresWorkspaceUserScope(t *testing.T) {
	c := writeCfg(t, `[[source]]
path = ["/r/a.md"]
scope = "user"
`)
	if !c.DeclaresWorkspace("/any/where") {
		t.Fatal("含 user 域 source 时任意工作区都应判为已声明")
	}

	c2 := writeCfg(t, `[[skill_source]]
path = ["/s/skills"]
scope = "user"
`)
	if !c2.DeclaresWorkspace("/any/where") {
		t.Fatal("含 user 域 skill_source 时任意工作区都应判为已声明")
	}

	c3 := writeCfg(t, `[[skill_source]]
path = ["/s/skills"]
workspace = ["/w/only"]
`)
	if c3.DeclaresWorkspace("/w/other") {
		t.Fatal("未声明的工作区不应被 workspace 域 skill_source 判为已声明")
	}
	if !c3.DeclaresWorkspace("/w/only") {
		t.Fatal("声明过的工作区应判为已声明")
	}
}

// TestSkillGroupExpand 校验 skill 组展开：嵌套引用把组内所有 skill 根目录合并进
// SkillSource.Paths，且可同时混列自身 path（组在前、显式 path 在后）。
func TestSkillGroupExpand(t *testing.T) {
	c := writeCfg(t, `
[[skill_group]]
name = "base"
path = ["/s/react", "/s/css"]

[[skill_group]]
name = "all"
use = ["base"]
path = ["/s/backend"]

[[skill_source]]
groups = ["all"]
path = ["/s/extra"]
agents = ["codebuddy"]
`)
	if len(c.SkillSources) != 1 {
		t.Fatalf("期望 1 条 skill_source，实得 %d", len(c.SkillSources))
	}
	got := c.SkillSources[0].Paths
	want := []string{"/s/react", "/s/css", "/s/backend", "/s/extra"}
	if !equalSlice(got, want) {
		t.Fatalf("展开后 Paths 应为 %v，实得 %v", want, got)
	}
}

// TestSkillSourceWorkspaceGroups 校验 skill_source 可引用 [[workspace_group]] 展开工作区，
// 且与自身显式 workspace 混合（组在前）。
func TestSkillSourceWorkspaceGroups(t *testing.T) {
	c := writeCfg(t, `
[[workspace_group]]
name = "dev"
workspace = ["/proj/1", "/proj/2"]

[[skill_source]]
path = ["/s/skills"]
workspace_groups = ["dev"]
workspace = ["/standalone"]
`)
	got := c.SkillSources[0].Workspaces
	want := []string{"/proj/1", "/proj/2", "/standalone"}
	if !equalSlice(got, want) {
		t.Fatalf("工作区展开后应为 %v，实得 %v", want, got)
	}
}

// TestSkillGroupErrors 校验 skill 组三类错误：引用未定义 / 循环引用 / 同类重名。
func TestSkillGroupErrors(t *testing.T) {
	undefined := `[[skill_source]]
groups = ["nope"]
path = ["/s/a"]
`
	if _, err := Load(writeConfig(t, undefined)); err == nil {
		t.Fatal("引用未定义 skill 组应报错")
	}

	cycle := `
[[skill_group]]
name = "A"
use = ["B"]

[[skill_group]]
name = "B"
use = ["A"]

[[skill_source]]
groups = ["A"]
path = ["/s/a"]
`
	if _, err := Load(writeConfig(t, cycle)); err == nil {
		t.Fatal("skill 组循环引用应报错")
	}

	dup := `
[[skill_group]]
name = "x"
path = ["/s/a"]

[[skill_group]]
name = "x"
path = ["/s/b"]

[[skill_source]]
groups = ["x"]
`
	if _, err := Load(writeConfig(t, dup)); err == nil {
		t.Fatal("skill 组重名应报错")
	}
}

// TestExpandHomes_SkillGroup 校验 skill_group.path 里的 "~" 在组展开前已被展开
// （否则组内根目录会残留 ~ 而永远匹配不到）。
func TestExpandHomes_SkillGroup(t *testing.T) {
	home := tildeHome(t)
	c := writeCfg(t, `[[skill_group]]
name = "g"
path = ["~/skills/a"]

[[skill_source]]
groups = ["g"]
`)
	got := c.SkillSources[0].Paths[0]
	want := filepath.Join(home, "skills", "a")
	if got != want {
		t.Fatalf("skill_group.path 未展开: got %q, want %q", got, want)
	}
}
