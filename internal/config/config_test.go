package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad 校验自写的 TOML 子集解析器（注释、[[source]]、path、agents、单引号）。
func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	content := `# rulemux 配置
[[source]]
path = "C:/rules/a.md"
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
	if c.Sources[0].Path != "C:/rules/a.md" {
		t.Fatalf("path 解析错误：%q", c.Sources[0].Path)
	}
	if len(c.Sources[0].Agents) != 2 || c.Sources[0].Agents[0] != "claude" {
		t.Fatalf("agents 解析错误：%v", c.Sources[0].Agents)
	}
	if c.Sources[1].Path != "D:/notes/b.txt" {
		t.Fatalf("单引号 path 解析错误：%q", c.Sources[1].Path)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate 不该报错：%v", err)
	}
}

// TestSourcesFor 校验 agents 过滤：显式列出命中，省略（空）表示投递给全部。
func TestSourcesFor(t *testing.T) {
	c := &Config{Sources: []Source{
		{Path: "a", Agents: []string{"claude"}},
		{Path: "b"},
	}}
	if got := c.SourcesFor("claude"); len(got) != 2 {
		t.Fatalf("claude 应命中 2 条（显式 + 全部），实得 %d", len(got))
	}
	if got := c.SourcesFor("trae"); len(got) != 1 {
		t.Fatalf("trae 应只命中「全部」那 1 条，实得 %d", len(got))
	}
}

func TestValidateEmpty(t *testing.T) {
	c := &Config{File: "x.toml"}
	if err := c.Validate(); err == nil {
		t.Fatal("空配置应校验失败")
	}
}
