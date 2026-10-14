package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/hooks"
	"github.com/cq-guojia/rulemux/internal/state"
)

// hasStr 报告 list 是否含 s（测试辅助；state.containsStr 是包私有的）。
func hasStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

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

// TestUninstallAgentRemovesOnlyThatLedgerEntry 校验 D2：uninstall --agent X 只把 X
// 从账本移除（off += X + 从该工作区列表删 X），Workspaces 条目保留，其它 agent 不动。
func TestUninstallAgentRemovesOnlyThatLedgerEntry(t *testing.T) {
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
	if err := state.Record(ws, "workbuddy"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	_ = captureStdout(t, func() {
		Uninstall([]string{"--agent", "workbuddy", "--config", cfgPath, "--workspace", ws, "--yes"})
	})

	l := state.Load()
	// AgentsFor 按 resolveWS 归一查询：还能查到即证明 Workspaces 条目没被删。
	if got := l.AgentsFor(ws); len(got) != 1 || got[0] != "codebuddy" {
		t.Fatalf("账本应只剩 codebuddy，实得 %v", got)
	}
	if !hasStr(l.Off, "workbuddy") {
		t.Fatalf("off 应含 workbuddy，实得 %v", l.Off)
	}
}

// TestUninstallHookFailureLeavesLedger 校验「先清理后删账本」的失败侧：钩子没摘掉
// ⇒ 该 agent 整个跳过，账本与 off 原样（它还在用，不能假装卸干净了）。
func TestUninstallHookFailureLeavesLedger(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "workbuddy"); err != nil {
		t.Fatal(err)
	}
	// 把 workbuddy 的钩子文件变成一个目录 ⇒ os.ReadFile 报 EISDIR ⇒ Uninstall 失败
	if err := os.MkdirAll(filepath.Join(home, ".workbuddy", "settings.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	_ = captureStdout(t, func() {
		Uninstall([]string{"--agent", "workbuddy", "--config", cfgPath, "--workspace", ws, "--yes"})
	})

	l := state.Load()
	if got := l.AgentsFor(ws); len(got) != 1 || got[0] != "workbuddy" {
		t.Fatalf("钩子没摘掉时账本必须原样，实得 %v", got)
	}
	if len(l.Off) != 0 {
		t.Fatalf("钩子没摘掉时 off 必须为空，实得 %v", l.Off)
	}
}

// TestUninstallSkipsGoneWorkspaceKeepsLedger 校验 sweep 的边界：不存在的工作区
// 无从清理 ⇒ 跳过且条目保留（路径重现时还要回访）；存在且清过的工作区才动账本。
func TestUninstallSkipsGoneWorkspaceKeepsLedger(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	gone := filepath.Join(tmp, "gone") // 不创建 ⇒ 不存在
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	if err := state.Record(gone, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	_ = captureStdout(t, func() {
		Uninstall([]string{"--agent", "codebuddy", "--config", cfgPath, "--workspace", ws, "--yes"})
	})

	l := state.Load()
	if got := l.AgentsFor(gone); len(got) != 1 || got[0] != "codebuddy" {
		t.Fatalf("不存在的工作区条目必须保留，实得 %v", got)
	}
	if got := l.AgentsFor(ws); len(got) != 0 {
		t.Fatalf("存在且清过的工作区应移除该 agent，实得 %v", got)
	}
	if !hasStr(l.Off, "codebuddy") {
		t.Fatalf("off 应含 codebuddy，实得 %v", l.Off)
	}
}

// TestUninstallOffClearsAgentsAndSetsOff 校验 --off / --all（同一分支）：全部 agent
// 入 off、各工作区 agents 清空（B1 的「无 agent」终态，文件不得退化成 v1 形态）。
func TestUninstallOffClearsAgentsAndSetsOff(t *testing.T) {
	for _, flag := range []string{"--off", "--all"} {
		t.Run(flag, func(t *testing.T) {
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
			if err := state.Record(ws, "workbuddy"); err != nil {
				t.Fatal(err)
			}
			src := filepath.Join(tmp, "a.md")
			if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

			_ = captureStdout(t, func() {
				Uninstall([]string{flag, "--config", cfgPath, "--workspace", ws, "--yes"})
			})

			l := state.Load()
			if got := l.AgentsFor(ws); len(got) != 0 {
				t.Fatalf("%s 后 agents 应清空，实得 %v", flag, got)
			}
			for _, id := range []string{"codebuddy", "workbuddy", "trae", "dsh"} {
				if !hasStr(l.Off, id) {
					t.Fatalf("%s 后 off 应含 %s，实得 %v", flag, id, l.Off)
				}
			}
		})
	}
}

// TestUninstallSkippedPruneKeepsLedgerButOffBlocks 校验「先清理后删账本」的 skipped
// 侧 + off 的兜底作用：配置读不了 ⇒ 清理被跳过（账本条目保留、off 仍写入），但此后的
// 裸 sync 依旧不重写它 —— off 是这条路径上唯一的防线，而它确实在工作。
func TestUninstallSkippedPruneKeepsLedgerButOffBlocks(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	rules := filepath.Join(ws, ".trae", "rules")
	if err := os.MkdirAll(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "trae"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")
	if code := Sync([]string{"--hook", "--agent", "trae", "--config", cfgPath, "--workspace", ws}); code != 0 {
		t.Fatalf("seed sync 应成功，实得 %d", code)
	}
	target := filepath.Join(rules, "__rulemux__a.md")
	mustExist(t, target)
	if err := os.WriteFile(src, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 配置路径不存在 ⇒ pruneSkipped：文件原样、账本条目保留，但 off 写入
	_ = captureStdout(t, func() {
		Uninstall([]string{"--agent", "trae", "--config", filepath.Join(tmp, "nope.toml"), "--workspace", ws, "--yes"})
	})
	l := state.Load()
	if got := l.AgentsFor(ws); len(got) != 1 || got[0] != "trae" {
		t.Fatalf("清理被跳过时账本条目必须保留，实得 %v", got)
	}
	if !hasStr(l.Off, "trae") {
		t.Fatalf("清理跳过也要记住「已卸载」（off），实得 %v", l.Off)
	}
	mustExist(t, target) // 清理确实被跳过：文件还在

	// 裸 sync：off 挡住它 ⇒ 内容仍是 v1（没被重写成 v2）
	_ = captureStderr(t, func() { Sync([]string{"--config", cfgPath, "--workspace", ws}) })
	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "v1" {
		t.Fatalf("off 应挡住已卸载 agent 的同步，实得内容 %q", b)
	}
}

// TestRemoveEmptyLeaf 表驱动校验删空目录 helper 的四条安全边界。
func TestRemoveEmptyLeaf(t *testing.T) {
	tmp := t.TempDir()

	empty := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	nonEmpty := filepath.Join(tmp, "nonempty")
	if err := os.MkdirAll(nonEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonEmpty, "my-own.md"), []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withSubdir := filepath.Join(tmp, "withsub")
	if err := os.MkdirAll(filepath.Join(withSubdir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "realrules")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink 不可用")
	}

	cases := []struct {
		name string
		dir  string
		want bool
	}{
		{"空目录删掉", empty, true},
		{"有用户文件不删", nonEmpty, false},
		{"有子目录条目不删", withSubdir, false},
		{"软链不碰", link, false},
		{"不存在当跳过", filepath.Join(tmp, "nope"), false},
		{"空路径当跳过", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := removeEmptyLeaf(c.dir); got != c.want {
				t.Fatalf("removeEmptyLeaf(%s) = %v, want %v", c.dir, got, c.want)
			}
		})
	}
	// 软链与目标都必须原样
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("软链必须原样保留")
	}
	mustExist(t, target)
}

// TestUninstallRemovesEmptyRulesDir 校验 D3：收敛后空的 rules 目录被删、父目录不上溯。
func TestUninstallRemovesEmptyRulesDir(t *testing.T) {
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

	if _, err := os.Stat(traeRules); !os.IsNotExist(err) {
		t.Fatalf("清空后的 rules 目录应被删除（stat err=%v）", err)
	}
	mustExist(t, filepath.Join(ws, ".trae")) // 不上溯：父目录保留
}

// TestUninstallExternalAgentWarnsAndReportsLegacyResidue 校验 R1（2026-10-14 按纯注入重写）
// 与旧交接目录的报告：dsh 改注入后卸载**不碰任何文件**，但必须说清两件事——
// ① 宿主侧插件停不掉（它的每个会话会继续调用 `rulemux inject --agent dsh` 注入规则）；
// ② 旧的 .dsh/rules 残留还在哪（只报不删，残留不强制清除）。
func TestUninstallExternalAgentWarnsAndReportsLegacyResidue(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	legacy := filepath.Join(ws, ".dsh", "rules")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(legacy, "__rulemux__a.md")
	if err := os.WriteFile(stale, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "dsh"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	out := captureStdout(t, func() {
		Uninstall([]string{"--agent", "dsh", "--config", cfgPath, "--workspace", ws, "--yes"})
	})

	for _, want := range []string{"inject --agent dsh", "disabled: true", "leftover"} {
		if !strings.Contains(out, want) {
			t.Fatalf("uninstall dsh 的输出应含 %q，实得:\n%s", want, out)
		}
	}
	mustExist(t, stale)  // 残留只报不删
	mustExist(t, legacy) // 目录本身也不动
	if l := state.Load(); !hasStr(l.Off, "dsh") {
		t.Fatalf("卸载后 off 应含 dsh，实得 %v", l.Off)
	}
}

// TestUninstallKeepsNonEmptyRulesDir 校验 D3 的边界：目录里还有用户自己的文件
// ⇒ 非空保留（只删 __rulemux__ 前缀内容）。
func TestUninstallKeepsNonEmptyRulesDir(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	traeRules := filepath.Join(ws, ".trae", "rules")
	if err := os.MkdirAll(traeRules, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(traeRules, "__rulemux__a.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(traeRules, "my-own.md"), []byte("# mine\n"), 0o644); err != nil {
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

	if _, err := os.Stat(filepath.Join(traeRules, "__rulemux__a.md")); !os.IsNotExist(err) {
		t.Error("__rulemux__a.md 应被删除")
	}
	mustExist(t, filepath.Join(traeRules, "my-own.md"))
	mustExist(t, traeRules) // 非空即留
}

// TestUninstallKeepsSymlinkedRulesDir 校验 D3 不碰软链：软链本身与目标目录都保留。
func TestUninstallKeepsSymlinkedRulesDir(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	target := filepath.Join(tmp, "realrules")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "__rulemux__a.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ws, ".dsh", "rules")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink 不可用")
	}
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))
	if err := state.Record(ws, "dsh"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	_ = captureStdout(t, func() {
		Uninstall([]string{"--agent", "dsh", "--config", cfgPath, "--workspace", ws, "--yes"})
	})

	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("软链必须原样保留")
	}
	mustExist(t, target) // 目标目录不删（链里的前缀文件照常被收敛）
}

// TestUninstallKeepsDirWithPrefixedSubdir 校验 D3 的判空口径：engine 删残留跳过
// 子目录 ⇒ 残留的前缀子目录让目录「非空」⇒ 保留（不硬删）。
func TestUninstallKeepsDirWithPrefixedSubdir(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	traeRules := filepath.Join(ws, ".trae", "rules")
	if err := os.MkdirAll(filepath.Join(traeRules, "__rulemux__x"), 0o755); err != nil {
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

	mustExist(t, filepath.Join(traeRules, "__rulemux__x"))
	mustExist(t, traeRules) // 有子目录条目不删
}

// TestUninstallConvergesCurrentWorkspaceSkills 校验 H3：当前工作区的 skills 目录
// 以前只在 sweep 里收、主循环漏了 ⇒ 残留永驻。现在 uninstall 必须收敛它，空则连目录删掉。
func TestUninstallConvergesCurrentWorkspaceSkills(t *testing.T) {
	tmp := t.TempDir()
	ws := filepath.Join(tmp, "ws")
	skills := filepath.Join(ws, ".codebuddy", "skills")
	if err := os.MkdirAll(filepath.Join(skills, "__rulemux__alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skills, "__rulemux__alpha", "SKILL.md"), []byte("# alpha\n"), 0o644); err != nil {
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
	// 只有 rules 源：声明工作区即可（DeclaresWorkspace 认它），skills want 为空 ⇒ 全清。
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	_ = captureStdout(t, func() {
		Uninstall([]string{"--agent", "codebuddy", "--config", cfgPath, "--workspace", ws, "--yes"})
	})

	if _, err := os.Stat(filepath.Join(skills, "__rulemux__alpha")); !os.IsNotExist(err) {
		t.Error("skills 残留应被收敛")
	}
	if _, err := os.Stat(skills); !os.IsNotExist(err) {
		t.Fatalf("清空后的 skills 目录应被删除（stat err=%v）", err)
	}
	mustExist(t, filepath.Join(ws, ".codebuddy")) // 不上溯
}
