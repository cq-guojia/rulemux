package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout 临时接管 os.Stdout，返回 fn 执行期间写出的全部内容。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

// writeConfig 在 tmp 下写一份配置，返回其路径。
func writeConfig(t *testing.T, tmp, body string) string {
	t.Helper()
	p := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestSyncSkipsUndeclaredWorkspace 校验 sync 的守卫：当目标工作区未被配置里任何
// 一条 [[source]] 声明时，sync 必须整体跳过 —— 既不创建规则目录，也不写账本。
// 这是「空 want ⇒ 删残留」会清空无关目录这一安全事故的防线。
// 见 docs/design/features/sync-all-and-change-notice.md §二「清空的正确姿势」。
func TestSyncSkipsUndeclaredWorkspace(t *testing.T) {
	tmp := t.TempDir()

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 配置只声明 /some/other，不包含我们将要同步的工作区。
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\nworkspace = \"/some/other\"\n")

	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	// 隔离账本，避免污染真实的 ~/.rulemux。
	stateDir := filepath.Join(tmp, "state")
	t.Setenv("RULEMUX_STATE_DIR", stateDir)

	if code := Sync([]string{"--config", cfgPath, "--workspace", ws, "--agent", "codebuddy"}); code != 0 {
		t.Fatalf("未声明的工作区应安静跳过（退出码 0），实得 %d", code)
	}

	if _, err := os.Stat(filepath.Join(ws, ".codebuddy", "rules")); !os.IsNotExist(err) {
		t.Fatalf("未声明的工作区不该被创建规则目录（stat err=%v）", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "workspaces.json")); !os.IsNotExist(err) {
		t.Fatalf("未声明的工作区不该写账本（stat err=%v）", err)
	}
}

// TestSyncHookRequiresAgent 校验：--hook 由 hook 调用，必然点名 agent，缺了应报错。
func TestSyncHookRequiresAgent(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+filepath.Join(tmp, "a.md")+"\"\n")
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	if code := Sync([]string{"--config", cfgPath, "--hook"}); code != 2 {
		t.Fatalf("--hook 缺 --agent 应以退出码 2 报错，实得 %d", code)
	}
}

// TestSyncStdoutContract 校验输出的硬契约：
//   - hook 路径 + 确有变化 ⇒ stdout 输出**恰好一条**协议 JSON；
//   - hook 路径 + 无变化   ⇒ stdout **一个字节都没有**（hook 绝不注入未经用户同意的内容）。
func TestSyncStdoutContract(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// workspace 省略 ⇒ 全局 source ⇒ 任意 cwd 都算「已声明」。
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	hookArgs := []string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"}

	first := captureStdout(t, func() {
		if code := Sync(hookArgs); code != 0 {
			t.Fatalf("hook sync 应成功，实得 %d", code)
		}
	})
	if !strings.Contains(first, `"hookSpecificOutput"`) || !strings.Contains(first, `"SessionStart"`) {
		t.Fatalf("有变化时 stdout 应输出协议 JSON，实得 %q", first)
	}
	if strings.Count(strings.TrimSpace(first), "\n") != 0 {
		t.Fatalf("协议应恰好一条，实得 %q", first)
	}

	second := captureStdout(t, func() {
		if code := Sync(hookArgs); code != 0 {
			t.Fatalf("第二次 hook sync 应成功，实得 %d", code)
		}
	})
	if second != "" {
		t.Fatalf("无变化时 stdout 必须为空，实得 %q", second)
	}

	// 手动（非 hook）路径：即使确有变化，也绝不吐协议。
	if err := os.WriteFile(src, []byte("# a v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third := captureStdout(t, func() {
		if code := Sync([]string{"--config", cfgPath, "--workspace", ws, "--agent", "codebuddy"}); code != 0 {
			t.Fatalf("手动 sync 应成功，实得 %d", code)
		}
	})
	if third != "" {
		t.Fatalf("手动路径不该输出协议，实得 %q", third)
	}
}

// TestSyncAllIteratesLedgerAndGCSMissing 校验 `sync --all`：
//   - 与 --workspace / --hook / --agent 互斥；
//   - 只走账本、只写已存在目录；
//   - 路径**确实不存在**（ENOENT）的账本条目被顺手 GC，存在的工作区不受影响。
func TestSyncAllIteratesLedgerAndGCSMissing(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(tmp, "state")
	t.Setenv("RULEMUX_STATE_DIR", stateDir)

	// 先经 hook 同步一次，建立账本与规则目录。
	if code := Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"}); code != 0 {
		t.Fatalf("hook sync 应成功，实得 %d", code)
	}

	// 互斥校验。
	if code := Sync([]string{"--config", cfgPath, "--all", "--workspace", ws}); code != 2 {
		t.Fatalf("--all 与 --workspace 应互斥（退出码 2），实得 %d", code)
	}
	if code := Sync([]string{"--config", cfgPath, "--all", "--hook"}); code != 2 {
		t.Fatalf("--all 与 --hook 应互斥（退出码 2），实得 %d", code)
	}

	// 账本里掺一个不存在的目录 ⇒ --all 应顺手 GC 掉它。
	lb := `{"version":2,"workspaces":["` + ws + `","/definitely/gone"],` +
		`"agents":{"` + ws + `":["codebuddy"],"/definitely/gone":["codebuddy"]}}`
	if err := os.WriteFile(filepath.Join(stateDir, "workspaces.json"), []byte(lb), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := Sync([]string{"--config", cfgPath, "--all"}); code != 0 {
		t.Fatalf("--all 应成功，实得 %d", code)
	}
	b, err := os.ReadFile(filepath.Join(stateDir, "workspaces.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "/definitely/gone") {
		t.Fatalf("不存在的账本条目应被 GC，实得 %s", string(b))
	}
	if !strings.Contains(string(b), ws) {
		t.Fatalf("存在的工作区不该被误删，实得 %s", string(b))
	}
}

// TestCascadeNeverCreatesDirs 校验顺手同步的安全边界：账本里记录的其它 agent
// （未知 id / 未验证 / 用同一个目录的别名）都不会导致创建任何目录，也不报错。
func TestCascadeNeverCreatesDirs(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(tmp, "state")
	t.Setenv("RULEMUX_STATE_DIR", stateDir)

	// 账本里塞入：同一 agent 的别名、未验证的、以及完全不存在的 id。
	lb := `{"version":2,"workspaces":["` + ws + `"],` +
		`"agents":{"` + ws + `":["workbuddy","claude","ghost"]}}`
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "workspaces.json"), []byte(lb), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"}); code != 0 {
		t.Fatalf("hook sync 应成功，实得 %d", code)
	}

	for _, rel := range []string{".claude/rules", ".ghost/rules"} {
		if _, err := os.Stat(filepath.Join(ws, rel)); !os.IsNotExist(err) {
			t.Fatalf("顺手同步绝不创建目录，但出现了 %s（stat err=%v）", rel, err)
		}
	}
}
