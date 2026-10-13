package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
)

// TestInstallMethod 表驱动校验安装方式判定（纯函数，不碰真文件系统）。
func TestInstallMethod(t *testing.T) {
	cases := []struct{ bin, method, pkg string }{
		{
			"/root/.rulemux-npm/lib/node_modules/rulemux/dist/rulemux-linux-amd64",
			"npm", "/root/.rulemux-npm/lib/node_modules/rulemux",
		},
		{
			"/proj/node_modules/rulemux/dist/rulemux-darwin-arm64",
			"npm", "/proj/node_modules/rulemux",
		},
		{
			"/home/u/.local/share/pnpm/global/5/node_modules/.pnpm/rulemux@0.4.2/node_modules/rulemux/dist/rulemux-linux-amd64",
			"pnpm",
			"/home/u/.local/share/pnpm/global/5/node_modules/.pnpm/rulemux@0.4.2/node_modules/rulemux",
		},
		{
			"/home/u/.config/yarn/global/node_modules/rulemux/dist/rulemux-linux-amd64",
			"yarn", "/home/u/.config/yarn/global/node_modules/rulemux",
		},
		{"/home/u/go/bin/rulemux", "go install", ""},
		{"/usr/local/bin/rulemux", "standalone binary (manual / GitHub Release)", ""},
	}
	for _, c := range cases {
		m, p := installMethod(c.bin)
		if m != c.method || p != c.pkg {
			t.Errorf("installMethod(%q) = (%q, %q), want (%q, %q)", c.bin, m, p, c.method, c.pkg)
		}
	}
}

// TestDoctorOutput 结构冒烟：四段式齐全（Install / Config / Agents / 汇总），
// 不断言具体环境值（随机器而变）。
func TestDoctorOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // 隔离：不读真实 ~/.rulemux
	out := captureStdout(t, func() { Doctor(nil) })
	for _, want := range []string{
		"rulemux doctor — environment check",
		"Install",
		"Config",
		"Agents —",
		"not initialized:",
		"next: rulemux verify",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor 输出应包含 %q，实得:\n%s", want, out)
		}
	}
}

// TestAgentStateLine 校验已初始化 agent 的行内状态（2026-10-13 用户拍版二版）：
// 只报钩子健康度，不再罗列 rules/skills 目录存在性——sync 已经报过，doctor
// 不重复刷屏。
func TestAgentStateLine(t *testing.T) {
	ws := t.TempDir()
	a, ok := agents.Get("codebuddy")
	if !ok {
		t.Skip("codebuddy 未注册")
	}
	line := agentStateLine(a, ws)
	if strings.Contains(line, "rules") || strings.Contains(line, "skills") {
		t.Errorf("agent 行不应再罗列规则/skill 目录，实得 %q", line)
	}
	if !strings.Contains(line, "hook") {
		t.Errorf("agent 行应报钩子状态，实得 %q", line)
	}
}

// TestPrintConfig 校验 Config 段新版式：计数行（rule files / skill dirs）+
// rules/skills 分组列文件 + 被 workspace 范围跳过的源折叠成一行暗色说明；
// 不再出现 agents / workspace 过滤标注（health check 不承担配置教程）。
func TestPrintConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	other := filepath.Join(tmp, "other")
	for _, d := range []string{home, ws, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	rule1 := filepath.Join(tmp, "a.md")
	rule2 := filepath.Join(tmp, "b.md")
	for _, p := range []string{rule1, rule2} {
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	skillDir := filepath.Join(tmp, "sk")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 规则源限定在 other 工作区 ⇒ 当前 ws 下应被记为 skipped
	cfgPath := writeConfig(t, tmp,
		"[[source]]\npath = [\""+rule1+"\", \""+rule2+"\"]\nworkspace = [\""+other+"\"]\n\n[[skill_source]]\npath = [\""+skillDir+"\"]\n")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() { printConfig(cfgPath, cfg, nil, ws) })
	for _, want := range []string{
		"2 rule files · 1 skill dir",
		"    rules\n",
		"    skills\n",
		"skipped in this workspace",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Config 输出应包含 %q，实得:\n%s", want, out)
		}
	}
	for _, bad := range []string{"all agents", "[skills]", "→"} {
		if strings.Contains(out, bad) {
			t.Errorf("Config 输出不应再出现 %q，实得:\n%s", bad, out)
		}
	}
}
