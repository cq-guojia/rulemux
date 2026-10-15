package state

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadMigratesV1WithoutLosingEntries 校验 v1 → v2 迁移**绝不丢条目**：
// 老条目（只有工作区、没有 agents）必须以哨兵 AnyAgent 占位保留。
// 丢条目会让卸载再也回访不到那些工作区，残留永远清不掉。
func TestLoadMigratesV1WithoutLosingEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvStateDir, dir)
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"workspaces":["/a","/b"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	l := Load()
	if len(l.Workspaces) != 2 {
		t.Fatalf("v1 迁移不得丢条目，实得 %v", l.Workspaces)
	}
	for _, ws := range []string{"/a", "/b"} {
		got := l.AgentsFor(ws)
		if len(got) != 1 || got[0] != AnyAgent {
			t.Fatalf("%s 的老条目应以哨兵 %q 占位，实得 %v", ws, AnyAgent, got)
		}
	}
	if l.Version != Version {
		t.Fatalf("迁移后 Version 应为 %d，实得 %d", Version, l.Version)
	}
}

// TestRecordAccumulatesAndIsIdempotent 校验「工作区 × agent」：同一对幂等，
// 不同 agent 累积（同一个工作区可能被多个 agent 用过）。
func TestRecordAccumulatesAndIsIdempotent(t *testing.T) {
	t.Setenv(EnvStateDir, t.TempDir())
	ws := "/proj/a"

	if err := Record(ws, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	if err := Record(ws, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	if err := Record(ws, "claude"); err != nil {
		t.Fatal(err)
	}

	l := Load()
	if got := l.AgentsFor(ws); len(got) != 2 {
		t.Fatalf("应记录 2 个 agent（且幂等），实得 %v", got)
	}
	if len(l.Workspaces) != 1 {
		t.Fatalf("工作区应只记一条，实得 %v", l.Workspaces)
	}
}

// TestRecordKeepsSentinel 校验：哨兵表示「全部已支持 agent」，不该被具体 agent 覆盖
// （否则会把「未知」错误地收窄成「只用过这一个」）。
func TestRecordKeepsSentinel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvStateDir, dir)
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"workspaces":["/a"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Record("/a", "codebuddy"); err != nil {
		t.Fatal(err)
	}
	if got := Load().AgentsFor("/a"); len(got) != 1 || got[0] != AnyAgent {
		t.Fatalf("哨兵应保留，实得 %v", got)
	}
}

// TestSaveIsAtomicAndValid 校验落盘：原子替换不留 .tmp 残留，且是合法 JSON + 带版本号。
func TestSaveIsAtomicAndValid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvStateDir, dir)
	if err := Record("/x", "codebuddy"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, FileName+".tmp")); !os.IsNotExist(err) {
		t.Fatal("原子写不该留下 .tmp 残留")
	}
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	var l Ledger
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatalf("账本应是合法 JSON：%v", err)
	}
	if l.Version != Version {
		t.Fatalf("落盘应带 version=%d，实得 %d", Version, l.Version)
	}
}

// TestLedgerSeenTouchStale 校验 lastSeen 的写入规则：
//   - Record 刷新 Seen；刚记录的不是 stale；
//   - Touch 只保成员、不刷新 Seen（sync --all 用它避免续命）；
//   - 回拨超过 StaleAfter 后 IsStale 为真；未知 agent 永远不 stale（保守）。
func TestLedgerSeenTouchStale(t *testing.T) {
	t.Setenv(EnvStateDir, t.TempDir())
	ws := "/proj/a"
	key := resolveWS(ws)

	if err := Record(ws, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load().Seen[key]["codebuddy"]; !ok {
		t.Fatal("Record 应写入 Seen")
	}
	if Load().IsStale(ws, "codebuddy") {
		t.Fatal("刚记录的条目不应是 stale")
	}

	// 回拨 Seen，Touch 不应刷新
	back := time.Now().Add(-(StaleAfter + time.Hour)).Unix()
	if err := UpdateLedger(func(l *Ledger) error {
		l.Seen[key]["codebuddy"] = back
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := Touch(ws, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	if got := Load().Seen[key]["codebuddy"]; got != back {
		t.Fatalf("Touch 不应刷新 Seen，got %d want %d", got, back)
	}

	// Record 重新刷新
	if err := Record(ws, "codebuddy"); err != nil {
		t.Fatal(err)
	}
	if got := Load().Seen[key]["codebuddy"]; got <= back {
		t.Fatalf("Record 应刷新 Seen，got %d", got)
	}

	if Load().IsStale(ws, "ghost") {
		t.Fatal("未知 agent 不应是 stale（保守）")
	}
}

// TestOthers 校验回访口径：返回除当前工作区之外的其它工作区。
func TestOthers(t *testing.T) {
	t.Setenv(EnvStateDir, t.TempDir())
	for _, ws := range []string{"/a", "/b", "/c"} {
		if err := Record(ws, "codebuddy"); err != nil {
			t.Fatal(err)
		}
	}
	got := Others("/b")
	if len(got) != 2 {
		t.Fatalf("应返回除当前外的 2 个，实得 %v", got)
	}
	for _, ws := range got {
		if ws == "/b" {
			t.Fatal("Others 不该包含当前工作区")
		}
	}
}

// TestLoadRespectsEmptyAgentsEntry 校验「空列表 = 无 agent」：v2 文件里某工作区是
// 空列表（uninstall 清空后的终态）时，Load 不得回填哨兵 *——那会把卸载成果冲掉（B1）。
func TestLoadRespectsEmptyAgentsEntry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvStateDir, dir)
	body := `{"version":2,"workspaces":["/a"],"agents":{"/a":[]}}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load().AgentsFor("/a"); len(got) != 0 {
		t.Fatalf("空 agents 条目应表示「无 agent」，实得 %v", got)
	}
}

// TestSaveDoesNotDegradeToV1AfterOff 校验：清空 agents + 写 off 之后，文件必须保持
// v2 形态（agents 键在），再次 Load 不得把工作区还原成「全部 agent」。
func TestSaveDoesNotDegradeToV1AfterOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvStateDir, dir)
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"workspaces":["/a"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateLedger(func(l *Ledger) error {
		l.Agents["/a"] = []string{}
		l.AddOff("dsh")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"agents"`)) {
		t.Fatalf("Save 后必须保留 agents 键（不得退化成 v1 形态），实得 %s", b)
	}
	l := Load()
	if got := l.AgentsFor("/a"); len(got) != 0 {
		t.Fatalf("清空后应是「无 agent」，实得 %v", got)
	}
	if !containsStr(l.Off, "dsh") {
		t.Fatalf("off 应持久化，实得 %v", l.Off)
	}
}

// TestForgetAgentSemantics 校验 ForgetAgent：显式列表移除该 agent、哨兵 * 列表不动
// （排除交给 off）、Workspaces 条目保留、幂等。
func TestForgetAgentSemantics(t *testing.T) {
	t.Setenv(EnvStateDir, t.TempDir())
	if err := UpdateLedger(func(l *Ledger) error {
		l.Workspaces = []string{"/ws", "/star"}
		l.Agents = map[string][]string{
			"/ws":   {"codebuddy", "dsh"},
			"/star": {AnyAgent},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateLedger(func(l *Ledger) error {
		l.ForgetAgent("/ws", "dsh")
		l.ForgetAgent("/star", "dsh") // 哨兵列表不动
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	l := Load()
	if got := l.AgentsFor("/ws"); len(got) != 1 || got[0] != "codebuddy" {
		t.Fatalf("显式列表应移除 dsh，实得 %v", got)
	}
	if got := l.AgentsFor("/star"); len(got) != 1 || got[0] != AnyAgent {
		t.Fatalf("哨兵列表应保持 *（排除交给 off），实得 %v", got)
	}
	if !containsStr(l.Workspaces, "/ws") || !containsStr(l.Workspaces, "/star") {
		t.Fatalf("Workspaces 条目必须保留，实得 %v", l.Workspaces)
	}
	if err := UpdateLedger(func(l *Ledger) error { l.ForgetAgent("/ws", "dsh"); return nil }); err != nil {
		t.Fatal(err)
	}
	if got := Load().AgentsFor("/ws"); len(got) != 1 || got[0] != "codebuddy" {
		t.Fatalf("ForgetAgent 应幂等，实得 %v", got)
	}
	// 不存在的工作区：安静无操作（不 panic、不新增条目）
	if err := UpdateLedger(func(l *Ledger) error { l.ForgetAgent("/nope", "dsh"); return nil }); err != nil {
		t.Fatal(err)
	}
	if got := Load().Workspaces; len(got) != 2 {
		t.Fatalf("对不存在的工作区 ForgetAgent 不该改动账本，实得 %v", got)
	}
}

// TestOffRoundTrip 校验 off 的增删与去重持久化。
func TestOffRoundTrip(t *testing.T) {
	t.Setenv(EnvStateDir, t.TempDir())
	if err := UpdateLedger(func(l *Ledger) error { l.AddOff("dsh", "dsh", "trae"); return nil }); err != nil {
		t.Fatal(err)
	}
	if got := Load().Off; len(got) != 2 {
		t.Fatalf("off 应去重持久化，实得 %v", got)
	}
	if err := UpdateLedger(func(l *Ledger) error { l.ClearOff("dsh"); return nil }); err != nil {
		t.Fatal(err)
	}
	if got := Load().Off; len(got) != 1 || got[0] != "trae" {
		t.Fatalf("ClearOff 后应只剩 trae，实得 %v", got)
	}
}

// TestRecordDoesNotTouchOff 校验 Record 不碰 off：自愈已删（全局自愈会跨工作区
// 复活已卸 agent），「重新在用」只由 init 安装模式清 off 表达。
func TestRecordDoesNotTouchOff(t *testing.T) {
	t.Setenv(EnvStateDir, t.TempDir())
	if err := UpdateLedger(func(l *Ledger) error { l.AddOff("dsh"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := Record("/ws", "dsh"); err != nil {
		t.Fatal(err)
	}
	if !containsStr(Load().Off, "dsh") {
		t.Fatal("Record 不得把 agent 从 off 移除")
	}
}
