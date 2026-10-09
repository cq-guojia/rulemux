package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
