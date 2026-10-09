package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/hooks"
)

// 共享规则目录（codebuddy / workbuddy 都是 .codebuddy/rules）下，卸载一个 agent 不能
// 把仍在使用的另一个 agent 的文件一起删掉。本文件锁住这条语义。
//
// 三个 source 的归属：
//
//	a.md → 只投 workbuddy（卸载 workbuddy 后应消失）
//	b.md → 只投 codebuddy（应保留）
//	c.md → 省略 agents（两个都要，应保留）
func setupSharedDir(t *testing.T) (cfgPath, ws, dir string) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws = filepath.Join(tmp, "ws")
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	dir = filepath.Join(ws, ".codebuddy", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	paths := map[string]string{}
	for _, n := range []string{"a", "b", "c"} {
		p := filepath.Join(tmp, n+".md")
		if err := os.WriteFile(p, []byte("# "+n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[n] = p
	}
	cfgPath = writeConfig(t, tmp,
		"[[source]]\npath = \""+paths["a"]+"\"\nagents = [\"workbuddy\"]\n\n"+
			"[[source]]\npath = \""+paths["b"]+"\"\nagents = [\"codebuddy\"]\n\n"+
			"[[source]]\npath = \""+paths["c"]+"\"\n")

	// 两个 agent 都装着钩子
	installHook(t, home, ".workbuddy", "workbuddy")
	installHook(t, home, ".codebuddy", "codebuddy")

	// 先同步一次，三个文件都落盘
	_ = captureStdout(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "workbuddy"})
	})
	for _, n := range []string{"a", "b", "c"} {
		mustExist(t, filepath.Join(dir, "__rulemux__"+n+".md"))
	}
	return cfgPath, ws, dir
}

// TestUninstallKeepsSharedDirFiles：卸掉 workbuddy 后，codebuddy 仍在用 ⇒
// 只有 workbuddy 独有的 a.md 被删，b/c 必须留下。
func TestUninstallKeepsSharedDirFiles(t *testing.T) {
	cfgPath, ws, dir := setupSharedDir(t)

	_ = captureStdout(t, func() {
		Uninstall([]string{"--agent", "workbuddy", "--config", cfgPath, "--workspace", ws, "--yes"})
	})

	if _, err := os.Stat(filepath.Join(dir, "__rulemux__a.md")); !os.IsNotExist(err) {
		t.Error("workbuddy 独有的 a.md 应被删除")
	}
	mustExist(t, filepath.Join(dir, "__rulemux__b.md"))
	mustExist(t, filepath.Join(dir, "__rulemux__c.md"))

	// workbuddy 的钩子也必须被摘掉
	wb, _ := agents.Get("workbuddy")
	installed, _, err := hooks.Inspect(wb.HookFileAbs(ws), wb)
	if err != nil || installed {
		t.Errorf("workbuddy 的钩子应已被摘掉: installed=%v err=%v", installed, err)
	}
}

// TestUninstallAllClearsSharedDir：--all 时两个 agent 都不再用 ⇒ want 为空 ⇒ 全清。
func TestUninstallAllClearsSharedDir(t *testing.T) {
	cfgPath, ws, dir := setupSharedDir(t)

	_ = captureStdout(t, func() {
		Uninstall([]string{"--all", "--config", cfgPath, "--workspace", ws, "--yes"})
	})

	for _, n := range []string{"a", "b", "c"} {
		if _, err := os.Stat(filepath.Join(dir, "__rulemux__"+n+".md")); !os.IsNotExist(err) {
			t.Errorf("--all 后 %s.md 应被删除", n)
		}
	}
}
