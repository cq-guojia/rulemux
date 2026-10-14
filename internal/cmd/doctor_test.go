package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/state"
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
		"next: rulemux verify",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor 输出应包含 %q，实得:\n%s", want, out)
		}
	}
}

// TestDoctorHidesUninstalledAgent 校验「卸载即断联」（用户 2026-10-14 拍板）：
// uninstall 后 off 含该 agent ⇒ doctor 输出不再提它——不出现、不 nag、不建议重装。
// 也顺带锁住名单新口径：从没用过的 agent 同样不出现，于是卸载完这个工作区就没人可列。
func TestDoctorHidesUninstalledAgent(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	traeRules := filepath.Join(ws, ".trae", "rules")
	if err := os.MkdirAll(traeRules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(traeRules, "__rulemux__a.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "trae"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	_ = captureStdout(t, func() {
		Uninstall([]string{"--agent", "trae", "--config", cfgPath, "--workspace", ws, "--yes"})
	})
	out := captureStdout(t, func() { Doctor([]string{"--config", cfgPath, "--workspace", ws}) })
	if strings.Contains(out, "trae") {
		t.Fatalf("已卸载 agent 不该出现在 doctor 输出里，实得:\n%s", out)
	}
	if !strings.Contains(out, "Agents — none in use in this workspace") {
		t.Fatalf("卸载后没人在用，Agents 段应这么说（而不是 0 of 0），实得:\n%s", out)
	}
}

// TestDoctorShowsInjectionOnlyAgent 校验 dsh 改注入后 doctor 说人话：它由宿主侧插件
// 现取现注入，工作区里没有可初始化的东西——既不报「未初始化」（那会把用户支去跑一个
// 修不好它的 init），也不静默消失（那会让用户以为 doctor 没看见它），而是单列一行说明。
func TestDoctorShowsInjectionOnlyAgent(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "dsh"); err != nil {
		t.Fatal(err) // dsh 在用（账本记着）
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	out := captureStdout(t, func() { Doctor([]string{"--config", cfgPath, "--workspace", ws}) })
	if !strings.Contains(out, "dsh") || !strings.Contains(out, "host-side plugin") {
		t.Fatalf("在用 dsh 应有一行说明它由宿主侧插件注入，实得:\n%s", out)
	}
	if strings.Contains(out, "not initialized: dsh") {
		t.Fatalf("dsh 不该被报成「未初始化」：\n%s", out)
	}
	if !strings.Contains(out, "Agents — 0 of 1 initialized") {
		t.Fatalf("分母只算在用的 agent（这里只有 dsh），实得:\n%s", out)
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
	// 规则源限定在 other 工作区 ⇒ 当前 ws 下应被记为 skipped；
	// 第二个 source 重复声明 rule1（file_group 继承的常态）⇒ 去重后只列一次
	cfgPath := writeConfig(t, tmp,
		"[[source]]\npath = [\""+rule1+"\", \""+rule2+"\"]\nworkspace = [\""+other+"\"]\n\n"+
			"[[source]]\npath = [\""+rule1+"\"]\nworkspace = [\""+other+"\"]\n\n"+
			"[[skill_source]]\npath = [\""+skillDir+"\"]\n")
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
	if n := strings.Count(out, rule1); n != 1 {
		t.Errorf("被多个 source 声明的文件应只出现一次，实得 %d 次:\n%s", n, out)
	}
	for _, bad := range []string{"all agents", "[skills]", "→"} {
		if strings.Contains(out, bad) {
			t.Errorf("Config 输出不应再出现 %q，实得:\n%s", bad, out)
		}
	}
}
