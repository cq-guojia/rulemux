package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/state"
)

// TestSyncSelfHealsOutdatedHook：不依赖任何 npm 安装脚本 —— 一次普通 sync 就应把
// 「我们自己那条旧格式钩子」就地升级，且不碰别人的条目与文件里其它键。
// 见 docs/design/features/agent-onboarding.md §二「升级刷新」。
func TestSyncSelfHealsOutdatedHook(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(filepath.Join(home, ".codebuddy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	hookPath := filepath.Join(home, ".codebuddy", "settings.json")
	seed := `{"theme":"dark","hooks":{"SessionStart":[` +
		`{"hooks":[{"type":"command","command":"node \"/other.js\""}]},` +
		`{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent codebuddy"}]}]}}`
	if err := os.WriteFile(hookPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	_ = captureStdout(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws, "--agent", "codebuddy"})
	})

	b, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "rulemux sync --hook --agent codebuddy") {
		t.Fatalf("旧格式钩子未被自愈: %s", got)
	}
	if !strings.Contains(got, `node \"/other.js\"`) {
		t.Fatalf("别家的钩子被改动: %s", got)
	}
	if !strings.Contains(got, `"theme": "dark"`) {
		t.Fatalf("文件里其它键丢失: %s", got)
	}
}

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

// TestSyncHookResolvesWorkspaceFromPayload 校验：hook 调用且未给 --workspace 时，
// sync --hook 应从 stdin 载荷读 cwd / workspace_roots 定位工作区（而非依赖 os.Getwd()）。
// 这是 2026-10-09 Trae canary 的直接受益：Trae/Claude Code 的 hook 事件会透传 cwd。
func TestSyncHookResolvesWorkspaceFromPayload(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	stateDir := filepath.Join(tmp, "state")
	t.Setenv("RULEMUX_STATE_DIR", stateDir)

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 不写 workspace ⇒ 适用于所有工作区；关键是不传 --workspace，全靠载荷 cwd 定位。
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\nagents = [\"trae\"]\n")

	// 把 hook 载荷写进 pipe，接到 os.Stdin（模拟 Trae/Claude Code 的 hook 事件输入）。
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	payload := `{"cwd": "` + ws + `", "workspace_roots": ["` + ws + `"], "hook_event_name":"SessionStart"}`
	if _, err := w.WriteString(payload); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	_ = captureStdout(t, func() {
		if code := Sync([]string{"--config", cfgPath, "--hook", "--agent", "trae"}); code != 0 {
			t.Fatalf("sync --hook 应成功，实得退出码 %d", code)
		}
	})

	// 引擎会把源文件按规则同步进工作区规则目录（文件名带 __rulemux__ 前缀）；
	// 关键是确认它确实落进了「载荷 cwd 指向的」ws/.trae/rules，而非别处。
	dir := filepath.Join(ws, ".trae", "rules")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("应从载荷 cwd 把规则同步进 %s: %v", dir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("载荷 cwd 定位的工作区规则目录 %s 不应为空", dir)
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

	// 关键：预先把这两个目录建出来。否则「目录不存在 + create=false ⇒ 不写」会让断言
	// 恒真（无论跳过逻辑在不在都通过）——目录先存在，才验证得到「确实没去处理它们」。
	for _, rel := range []string{".claude/rules", ".ghost/rules"} {
		if err := os.MkdirAll(filepath.Join(ws, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if code := Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"}); code != 0 {
		t.Fatalf("hook sync 应成功，实得 %d", code)
	}

	for _, rel := range []string{".claude/rules", ".ghost/rules"} {
		if _, err := os.Stat(filepath.Join(ws, rel, "__rulemux__a.md")); !os.IsNotExist(err) {
			t.Fatalf("顺手同步不该处理未验证 / 未知 agent，但 %s 被写了文件（stat err=%v）", rel, err)
		}
	}
}

// installHook 在假 HOME 下为指定 agent 写一条 rulemux SessionStart 钩子，
// 使 hookInstalled(...) 判定为「已装」。返回写入的文件路径。
func installHook(t *testing.T, home, dirName, agentID string) string {
	t.Helper()
	p := filepath.Join(home, dirName, "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --hook --agent ` + agentID + `"}]}]}}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("期望存在 %s: %v", p, err)
	}
}

// TestSyncSharedRulesDir_NoMutualDeletion：codebuddy 与 workbuddy 共享 .codebuddy/rules。
// 两个 agent 各自只投一部分 source，若不同步取并集，后跑的那个会把先跑的文件当残留删掉
// ⇒ 两个钩子轮流触发时文件来回消失。本用例锁住「不互删」。
func TestSyncSharedRulesDir_NoMutualDeletion(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := os.MkdirAll(filepath.Join(ws, ".codebuddy", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}

	a := filepath.Join(tmp, "a.md") // 只投 workbuddy
	b := filepath.Join(tmp, "b.md") // 只投 codebuddy
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfgPath := writeConfig(t, tmp,
		"[[source]]\npath = \""+a+"\"\nagents = [\"workbuddy\"]\n\n"+
			"[[source]]\npath = \""+b+"\"\nagents = [\"codebuddy\"]\n")

	installHook(t, home, ".workbuddy", "workbuddy")
	installHook(t, home, ".codebuddy", "codebuddy")

	dir := filepath.Join(ws, ".codebuddy", "rules")
	fa := filepath.Join(dir, "__rulemux__a.md")
	fb := filepath.Join(dir, "__rulemux__b.md")

	// workbuddy 的钩子先跑：并集 ⇒ 两个文件都落
	_ = captureStdout(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "workbuddy"})
	})
	mustExist(t, fa)
	mustExist(t, fb)

	// codebuddy 的钩子再跑：不得把 workbuddy 的 a.md 当残留删掉
	_ = captureStdout(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"})
	})
	mustExist(t, fa)
	mustExist(t, fb)
}

// TestSyncSharedRulesDir_FallsBackToWholeGroupWhenNoHook：共享组里一个钩子都没装时
// （纯手动 sync 场景）必须退回整组并集，否则手动跑什么都不落。
func TestSyncSharedRulesDir_FallsBackToWholeGroupWhenNoHook(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := os.MkdirAll(filepath.Join(ws, ".codebuddy", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}

	a := filepath.Join(tmp, "a.md")
	b := filepath.Join(tmp, "b.md")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfgPath := writeConfig(t, tmp,
		"[[source]]\npath = \""+a+"\"\nagents = [\"workbuddy\"]\n\n"+
			"[[source]]\npath = \""+b+"\"\nagents = [\"codebuddy\"]\n")

	// 一个钩子都不装
	_ = captureStdout(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws})
	})

	dir := filepath.Join(ws, ".codebuddy", "rules")
	mustExist(t, filepath.Join(dir, "__rulemux__a.md"))
	mustExist(t, filepath.Join(dir, "__rulemux__b.md"))
}

// TestSyncBareEmptyLedgerWritesNothing 校验 D1 第一半：裸 sync（无 --agent）且账本
// 为空 ⇒ 不写任何规则目录、不建账本文件，只给一句可执行提示，退出码 0。
func TestSyncBareEmptyLedgerWritesNothing(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	stateDir := filepath.Join(tmp, "state")
	t.Setenv("RULEMUX_STATE_DIR", stateDir)

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	code := 0
	errOut := captureStderr(t, func() { code = Sync([]string{"--config", cfgPath, "--workspace", ws}) })
	if code != 0 {
		t.Fatalf("空账本裸 sync 应安静退出（0），实得 %d", code)
	}
	if !strings.Contains(errOut, "sync --agent") {
		t.Fatalf("应提示怎么开始（sync --agent），实得 %q", errOut)
	}
	for _, d := range []string{".codebuddy", ".dsh", ".trae", ".workbuddy"} {
		if _, err := os.Stat(filepath.Join(ws, d)); !os.IsNotExist(err) {
			t.Fatalf("空账本裸 sync 不该创建 %s（stat err=%v）", d, err)
		}
	}
	if _, err := os.Stat(filepath.Join(stateDir, "workspaces.json")); !os.IsNotExist(err) {
		t.Fatal("空账本裸 sync 不该创建账本文件")
	}
}

// TestSyncBareLedgerRestrictsTargets 校验 D1 第二半：账本只记 codebuddy 时，裸 sync
// 只落 codebuddy；.dsh/rules、.trae/rules 一个字节都不写（用户机元凶的回归锁）。
func TestSyncBareLedgerRestrictsTargets(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	rules := filepath.Join(ws, ".codebuddy", "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
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
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	if code := Sync([]string{"--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("裸 sync 应成功，实得 %d", code)
	}
	mustExist(t, filepath.Join(rules, "__rulemux__a.md"))
	for _, d := range []string{".dsh", ".trae"} {
		if _, err := os.Stat(filepath.Join(ws, d)); !os.IsNotExist(err) {
			t.Fatalf("账本没记 %s，裸 sync 不该碰它（stat err=%v）", d, err)
		}
	}
}

// TestSyncBareHealsHookWhenLedgerEmpty 校验 healHooks 与同步范围解耦：账本为空时
// 裸 sync 什么都不落，但已存在的旧格式钩子仍被自愈（否则空账本会丧失升级路径）。
func TestSyncBareHealsHookWhenLedgerEmpty(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(filepath.Join(home, ".codebuddy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	hookPath := filepath.Join(home, ".codebuddy", "settings.json")
	seed := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent codebuddy"}]}]}}`
	if err := os.WriteFile(hookPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := Sync([]string{"--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("裸 sync 应成功，实得 %d", code)
	}
	b, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "rulemux sync --hook --agent codebuddy") {
		t.Fatalf("空账本裸 sync 仍应自愈旧格式钩子，实得 %s", b)
	}
	if _, err := os.Stat(filepath.Join(ws, ".codebuddy", "rules")); !os.IsNotExist(err) {
		t.Fatal("空账本裸 sync 不该落规则文件")
	}
}

// TestSyncBareConfigExplicitAgentStillSyncs 校验 M3：配置里 agents 非空且点名的
// agent，即使账本没有它，裸 sync 也照样落盘——显式配置意图优先于账本收窄。
func TestSyncBareConfigExplicitAgentStillSyncs(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	traeRules := filepath.Join(ws, ".trae", "rules")
	if err := os.MkdirAll(traeRules, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state")) // 空账本

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\nagents = [\"trae\"]\n")

	if code := Sync([]string{"--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("裸 sync 应成功，实得 %d", code)
	}
	mustExist(t, filepath.Join(traeRules, "__rulemux__a.md"))
}

// TestAgentsFromLedger 表驱动校验账本 → agent 的解析语义（D1/D2 暗雷）：
// nil/空 = 无 agent（不再是「全部」）；显式列表跳过未知/未验证，同样减 off。
// （哨兵 `*` 的收紧见 TestAgentsFromLedgerSentinel —— 它依赖钩子状态。）
func TestAgentsFromLedger(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
		off  []string
		want []string
	}{
		{"nil 不是全部", nil, nil, nil},
		{"空不是全部", []string{}, nil, nil},
		{"显式跳过未知与未验证", []string{"codebuddy", "ghost", "claude"}, nil, []string{"codebuddy"}},
		{"显式也减 off", []string{"codebuddy", "trae"}, []string{"trae"}, []string{"codebuddy"}},
		{"Tier-2 注入型不进表", []string{"codebuddy", "dsh", "codex"}, nil, []string{"codebuddy"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := agentIDs(agentsFromLedger(c.ids, c.off, ""))
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("want %v, got %v", c.want, got)
			}
		})
	}
}

// TestAgentsFromLedgerSentinel 校验哨兵 `*`（v1 账本占位）的收紧：只取**有在用证据**
// 的 agent —— 钩子型要求钩子已装；external 型（dsh）与没装钩子的一律不纳入。否则
// `*` 会把裸 sync / cascade / sync --all 重新放大成「全部 Tier-1」，把遗留的
// `.dsh/rules` 又写一遍（正是本次修复要消灭的「新的写入」）。
func TestAgentsFromLedgerSentinel(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	// dsh 的规则目录存在（历史残留 / 插件建的）也不构成「在用」证据
	if err := os.MkdirAll(filepath.Join(ws, ".dsh", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := agentIDs(agentsFromLedger([]string{"*"}, nil, ws)); len(got) != 0 {
		t.Fatalf("没有任何钩子时哨兵不该展开出任何 agent（含 dsh），实得 %v", got)
	}

	installHook(t, home, ".codebuddy", "codebuddy")
	if got := agentIDs(agentsFromLedger([]string{"*"}, nil, ws)); strings.Join(got, ",") != "codebuddy" {
		t.Fatalf("哨兵应只展开有钩子在用的 agent，实得 %v", got)
	}
	if got := agentIDs(agentsFromLedger([]string{"*"}, []string{"codebuddy"}, ws)); len(got) != 0 {
		t.Fatalf("哨兵展开后仍要减 off，实得 %v", got)
	}
}

// agentIDs 提取 agent 的规范 ID（测试辅助）。
func agentIDs(list []agents.Agent) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.ID)
	}
	return out
}

// TestSyncBareConfigExplicitAgentSkipsOff 校验 off 的减法覆盖**配置点名**那一路
// （不只是账本那一路）：配置里显式点名了一个已卸载的 agent，裸 sync 也不许写它。
func TestSyncBareConfigExplicitAgentSkipsOff(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	dshRules := filepath.Join(ws, ".dsh", "rules")
	if err := os.MkdirAll(dshRules, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.UpdateLedger(func(l *state.Ledger) error { l.AddOff("dsh"); return nil }); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\nagents = [\"dsh\"]\n")

	if code := Sync([]string{"--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("裸 sync 应安静退出（0），实得 %d", code)
	}
	if _, err := os.Stat(filepath.Join(dshRules, "__rulemux__a.md")); !os.IsNotExist(err) {
		t.Fatalf("已卸载 agent（off 里）即使被配置点名也不该同步（stat err=%v）", err)
	}
}

// TestSharedGroupFallbackRespectsOff 校验共享组「一个钩子都没装 ⇒ 退回整组」的兜底
// 也要减 off：否则「两家钩子都不在 + off 含其中一家」时，手动 sync 会把已卸那家的
// 源并回共享目录（已卸载的 agent 被复活）。
func TestSharedGroupFallbackRespectsOff(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	dir := filepath.Join(ws, ".codebuddy", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	// 账本记过 codebuddy + workbuddy；两家钩子都没装；workbuddy 已卸载（off）
	if err := state.Record(ws, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	if err := state.Record(ws, "workbuddy"); err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateLedger(func(l *state.Ledger) error {
		l.ForgetAgent(ws, "workbuddy")
		l.AddOff("workbuddy")
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	a := filepath.Join(tmp, "a.md") // 两家都要
	b := filepath.Join(tmp, "b.md") // 只给 workbuddy
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfgPath := writeConfig(t, tmp,
		"[[source]]\npath = \""+a+"\"\n\n"+
			"[[source]]\npath = \""+b+"\"\nagents = [\"workbuddy\"]\n")

	_ = captureStdout(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws, "--agent", "codebuddy"})
	})

	mustExist(t, filepath.Join(dir, "__rulemux__a.md"))
	if _, err := os.Stat(filepath.Join(dir, "__rulemux__b.md")); !os.IsNotExist(err) {
		t.Fatalf("已卸载的 workbuddy 不该被整组兜底并回来（stat err=%v）", err)
	}
}

// TestExplicitSyncOfOffAgentStillExecutes 校验「显式 --agent 按意图执行」：点名一个
// 已卸载（off）的 agent 时，若其规则目录已存在，仍按它的源同步 —— 不能被并集兜底
// 把它自己剔掉（那样 want 为空，反倒把它的残留删了）。
func TestExplicitSyncOfOffAgentStillExecutes(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	dir := filepath.Join(ws, ".trae", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "trae"); err != nil {
		t.Fatal(err)
	}
	if err := state.UpdateLedger(func(l *state.Ledger) error {
		l.ForgetAgent(ws, "trae")
		l.AddOff("trae")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	_ = captureStdout(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws, "--agent", "trae"})
	})

	mustExist(t, filepath.Join(dir, "__rulemux__a.md"))
}

// TestSyncBareReportListsOnlyInUseUninitialized 校验未初始化名单的新口径
// （2026-10-14 用户拍板「没用过的别出现，没有意义」）：名单 = 这个工作区**在用**
// （账本记录 ∪ 配置点名 − off）却**没有规则目录**的 agent。
//   - 阶段一：codebuddy 在用但没目录、trae 只用配置点名也没目录 ⇒ 两者都列；
//     workbuddy 从没用过、dsh 已卸载（off）⇒ 都不列；
//   - 阶段二：把两者的目录建出来（都初始化的状态）⇒ 整行消失，包括从没用过的
//     workbuddy —— 这正是老口径（全部 Tier-1 减 off）做不到的：它会把 workbuddy 一直挂着。
func TestSyncBareReportListsOnlyInUseUninitialized(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	if err := state.Record(ws, "dsh"); err != nil {
		t.Fatal(err) // 账本里记着 dsh，但它在 off 里（已卸载）
	}
	if err := state.UpdateLedger(func(l *state.Ledger) error { l.AddOff("dsh"); return nil }); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\nagents = [\"trae\"]\n")

	errOut := captureStderr(t, func() { Sync([]string{"--config", cfgPath, "--workspace", ws}) })
	line := notInitLine(errOut)
	for _, id := range []string{"codebuddy", "trae"} {
		if !strings.Contains(line, id) {
			t.Fatalf("在用但没目录的 %s 应出现在未初始化名单：%q", id, line)
		}
	}
	for _, id := range []string{"workbuddy", "dsh"} {
		if strings.Contains(line, id) {
			t.Fatalf("%s 不该出现在未初始化名单（从没用过 / 已卸载）：%s", id, line)
		}
	}

	// 阶段二：把两个在用 agent 的目录建出来 ⇒ 名单整行消失。
	for _, d := range []string{".codebuddy/rules", ".trae/rules"} {
		if err := os.MkdirAll(filepath.Join(ws, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	errOut = captureStderr(t, func() { Sync([]string{"--config", cfgPath, "--workspace", ws}) })
	if line := notInitLine(errOut); line != "" {
		t.Fatalf("都初始化后不该再有未初始化名单（含从没用过的 workbuddy），实得 %q", line)
	}
}

// TestSyncHookTier2AgentWritesNothing 锁住 dsh 改注入后最关键的一条：`sync --hook --agent dsh`
// 不再往工作区写任何东西——旧的 .dsh/rules 残留既不被改写也不被删（那段设计已作废，
// 残留不强制清除），账本也不会记它（它不再靠目录型交付）。
func TestSyncHookTier2AgentWritesNothing(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	legacy := filepath.Join(ws, ".dsh", "rules")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(legacy, "__rulemux__a.md")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	_ = captureStdout(t, func() {
		if code := Sync([]string{"--hook", "--agent", "dsh", "--config", cfgPath, "--workspace", ws}); code != 0 {
			t.Fatalf("sync --hook --agent dsh 应安静成功（什么也不做），实得 %d", code)
		}
	})
	b, err := os.ReadFile(stale)
	if err != nil {
		t.Fatalf("旧残留文件不该被删：%v", err)
	}
	if strings.TrimSpace(string(b)) != "stale" {
		t.Fatalf("旧残留文件不该被改写，实得 %q", b)
	}
	if got := state.Load().AgentsFor(ws); len(got) != 0 {
		t.Fatalf("Tier-2 注入型不该进账本，实得 %v", got)
	}
}

// notInitLine 从 sync 报告里抽出「not initialized:」那一行（测试辅助）。
func notInitLine(report string) string {
	for _, l := range strings.Split(report, "\n") {
		if strings.Contains(l, "not initialized:") {
			return l
		}
	}
	return ""
}
