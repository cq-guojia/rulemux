package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cq-guojia/rulemux/internal/engine"
	"github.com/cq-guojia/rulemux/internal/state"
)

// verifySetup 搭一个「三个 Tier-1 规则目录都在、账本只记 codebuddy」的现场：
// 裸 verify 只该给 codebuddy 种 canary（R4）。
func verifySetup(t *testing.T) (cfgPath, ws string) {
	t.Helper()
	tmp := t.TempDir()
	ws = filepath.Join(tmp, "ws")
	for _, d := range []string{".codebuddy/rules", ".dsh/rules", ".trae/rules"} {
		if err := os.MkdirAll(filepath.Join(ws, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n"), ws
}

// TestVerifyBareOnlyInUse 校验 R4 主路径：裸 verify 只给「在用」的 Tier-1 agent
// 种 canary；不在用的（账本没记、也没被配置点名）一个字节都不写——否则一次裸
// verify 就给没装 / 已卸的 agent 建出规则目录，成为回填跳板。
func TestVerifyBareOnlyInUse(t *testing.T) {
	cfgPath, ws := verifySetup(t)

	if code := Verify([]string{"--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("裸 verify 应成功，实得 %d", code)
	}
	mustExist(t, filepath.Join(ws, ".codebuddy", "rules", engine.CanaryName))
	for _, d := range []string{".dsh", ".trae"} {
		if _, err := os.Stat(filepath.Join(ws, d, "rules", engine.CanaryName)); !os.IsNotExist(err) {
			t.Fatalf("不在用的 %s 不该被种 canary（stat err=%v）", d, err)
		}
	}
}

// TestVerifyExplicitAgentOverridesFilter 校验 R4 例外：显式 --agent 永远允许
// （onboarding 是 verify 的存在理由，新 agent 靠它验收）。用 trae 当样本：
// 它不在账本里（不在用），但显式点名就照做。
func TestVerifyExplicitAgentOverridesFilter(t *testing.T) {
	cfgPath, ws := verifySetup(t)

	if code := Verify([]string{"--agent", "trae", "--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("显式 verify 应成功，实得 %d", code)
	}
	mustExist(t, filepath.Join(ws, ".trae", "rules", engine.CanaryName))
}

// TestVerifyTier2AgentWritesNothing 锁住 dsh 改注入后的 verify 行为：Tier-2 没有规则目录，
// 显式 `--agent dsh` 也不许建目录、不许种 canary —— 否则一次 verify 就又给不在用的 dsh
// 造出一个目录，正是本次要消灭的跳板；它只打印「去会话里直接问」。
func TestVerifyTier2AgentWritesNothing(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	out := captureStdout(t, func() {
		if code := Verify([]string{"--agent", "dsh", "--config", cfgPath, "--workspace", ws}); code != 0 {
			t.Fatalf("显式 verify --agent dsh 应成功，实得 %d", code)
		}
	})
	if _, err := os.Stat(filepath.Join(ws, ".dsh")); !os.IsNotExist(err) {
		t.Fatalf("dsh 走注入交付，verify 不该建出 .dsh 目录（stat err=%v）", err)
	}
	if !strings.Contains(out, "no rules dir") {
		t.Fatalf("应说明 Tier-2 没有规则目录、去会话里直接问，实得:\n%s", out)
	}
}

// TestVerifyCleanRemovesAllCanaries 校验 R4 的超集语义：裸 --clean 清本工作区
// **所有** Tier-1 的 canary——含显式 --agent 种给不在用 agent 的那颗（否则它
// 永远不在任何「在用」集合里，没人清）。
func TestVerifyCleanRemovesAllCanaries(t *testing.T) {
	cfgPath, ws := verifySetup(t)

	if code := Verify([]string{"--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("裸 verify 应成功，实得 %d", code)
	}
	if code := Verify([]string{"--agent", "trae", "--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("显式 verify 应成功，实得 %d", code)
	}
	if code := Verify([]string{"--clean", "--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("裸 --clean 应成功，实得 %d", code)
	}
	for _, d := range []string{".codebuddy", ".trae"} {
		if _, err := os.Stat(filepath.Join(ws, d, "rules", engine.CanaryName)); !os.IsNotExist(err) {
			t.Fatalf("%s 的 canary 应被清掉（stat err=%v）", d, err)
		}
	}
}

// TestVerifyDegradedConfigUsesLedgerOnly 校验配置读不了时的降级：退化为仅账本，
// 不崩、也不扩大范围（不给不在用的 agent 种 canary）。
func TestVerifyDegradedConfigUsesLedgerOnly(t *testing.T) {
	_, ws := verifySetup(t)

	if code := Verify([]string{"--config", filepath.Join(t.TempDir(), "nope.toml"), "--workspace", ws}); code != 0 {
		t.Fatalf("配置读不了时裸 verify 应降级而非失败，实得 %d", code)
	}
	mustExist(t, filepath.Join(ws, ".codebuddy", "rules", engine.CanaryName))
	for _, d := range []string{".dsh", ".trae"} {
		if _, err := os.Stat(filepath.Join(ws, d, "rules", engine.CanaryName)); !os.IsNotExist(err) {
			t.Fatalf("降级路径不该给不在用的 %s 种 canary（stat err=%v）", d, err)
		}
	}
}

// TestVerifyCleanRemovesCanaryAndEmptiedDir 锁住裸 --clean 的两件事：① 清掉 canary；
// ② 顺带删掉因此变空的规则目录（与 uninstall 的 D3 同规）。同时锁住「在用集合为空时
// 裸 --clean 必须照样工作」——早退短路曾是本修复自己引入的回归。
func TestVerifyCleanRemovesCanaryAndEmptiedDir(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	traeRules := filepath.Join(ws, ".trae", "rules")
	if err := os.MkdirAll(traeRules, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state")) // 空账本 ⇒ 在用集合为空
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	// 显式给不在用的 trae 种一颗 canary（onboarding 场景）
	if code := Verify([]string{"--agent", "trae", "--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("显式 verify 应成功，实得 %d", code)
	}
	mustExist(t, filepath.Join(traeRules, engine.CanaryName))

	if code := Verify([]string{"--clean", "--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("裸 --clean 应成功，实得 %d", code)
	}
	if _, err := os.Stat(filepath.Join(traeRules, engine.CanaryName)); !os.IsNotExist(err) {
		t.Fatalf("裸 --clean 必须清掉 canary（在用集合为空也不例外）（stat err=%v）", err)
	}
	if _, err := os.Stat(traeRules); !os.IsNotExist(err) {
		t.Fatalf("清空后的规则目录应被删掉（stat err=%v）", err)
	}
	mustExist(t, filepath.Join(ws, ".trae")) // 不上溯
}

// TestVerifyBareNothingInUse 校验裸 verify 的空场：账本与配置都没点名任何 agent
// ⇒ 提示怎么开始 + exit 0，不种任何 canary。
func TestVerifyBareNothingInUse(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	for _, d := range []string{".codebuddy/rules", ".dsh/rules", ".trae/rules"} {
		if err := os.MkdirAll(filepath.Join(ws, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state")) // 空账本
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	if code := Verify([]string{"--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("空场裸 verify 应安静退出（0），实得 %d", code)
	}
	for _, d := range []string{".codebuddy", ".dsh", ".trae"} {
		if _, err := os.Stat(filepath.Join(ws, d, "rules", engine.CanaryName)); !os.IsNotExist(err) {
			t.Fatalf("空场裸 verify 不该给 %s 种 canary（stat err=%v）", d, err)
		}
	}
}
