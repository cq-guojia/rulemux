package config

import (
	"os"
	"path/filepath"
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
		{Paths: []string{"omit"}},                                 // 省略 = 所有
		{Paths: []string{"star"}, Workspaces: []string{"*"}},       // * = 所有
		{Paths: []string{"word"}, Workspaces: []string{"all"}},     // all = 所有
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
