package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cq-guojia/rulemux/internal/agents"
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

// TestAgentStateLine 校验已初始化 agent 的行内状态：钩子状态 + 目录存在性
// （缺失暗色 (missing)），一行承载，不出目录刷屏。
func TestAgentStateLine(t *testing.T) {
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".codebuddy", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, ok := agents.Get("codebuddy")
	if !ok {
		t.Skip("codebuddy 未注册")
	}
	line := agentStateLine(a, ws)
	if !strings.Contains(line, "rules ") {
		t.Errorf("行内状态应含 rules 目录，实得 %q", line)
	}
	// 工作区目录相对 ws 显示：.codebuddy/rules 已建 ⇒ ✓；skills 未建 ⇒ (missing)
	if !strings.Contains(line, ".codebuddy/rules ✓") {
		t.Errorf("已建的 rules 目录应标 ✓（相对路径），实得 %q", line)
	}
	if !strings.Contains(line, "(missing)") {
		t.Errorf("未建的 skills 目录应标 (missing)，实得 %q", line)
	}
}
