package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cq-guojia/rulemux/internal/config"
)

// mkSkill 在 root 下造一个 skill 目录：<name>/SKILL.md + <name>/references/ref.md。
func mkSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "ref.md"), []byte("ref-"+name), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillSrcs(paths ...string) []config.SkillSource {
	return []config.SkillSource{{Paths: paths}}
}

// TestSyncSkillsCopyThenIdempotent：首拷整树，二次同步全跳过。
func TestSyncSkillsCopyThenIdempotent(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mkSkill(t, src, "alpha", "alpha skill")
	mkSkill(t, src, "beta", "beta skill")

	r, err := SyncSkills(dst, skillSrcs(src), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Copied) != 2 {
		t.Fatalf("首拷应复制 2 个 skill，实得 %d（%v）", len(r.Copied), r.Copied)
	}
	if !r.HasChanges() {
		t.Fatal("首拷应有变化")
	}
	for _, name := range []string{"__rulemux__alpha", "__rulemux__beta"} {
		if _, err := os.Stat(filepath.Join(dst, name, "SKILL.md")); err != nil {
			t.Fatalf("缺少 %s/SKILL.md：%v", name, err)
		}
		if _, err := os.Stat(filepath.Join(dst, name, "references", "ref.md")); err != nil {
			t.Fatalf("缺少 %s/references/ref.md：%v", name, err)
		}
	}

	r2, err := SyncSkills(dst, skillSrcs(src), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Copied)+len(r2.Updated)+len(r2.Deleted) != 0 {
		t.Fatalf("二次同步应全部跳过，实得 %+v", r2)
	}
	if len(r2.Skipped) != 2 {
		t.Fatalf("二次同步应跳过 2 个，实得 %d", len(r2.Skipped))
	}
}

// TestSyncSkillsUpdateAndTreeCleanup：SKILL.md 变更⇒覆盖；源里增删文件⇒副本跟着变。
func TestSyncSkillsUpdateAndTreeCleanup(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mkSkill(t, src, "alpha", "v1")

	if _, err := SyncSkills(dst, skillSrcs(src), true); err != nil {
		t.Fatal(err)
	}
	// 改内容
	if err := os.WriteFile(filepath.Join(src, "alpha", "SKILL.md"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 源里加一个文件、删一个文件
	if err := os.WriteFile(filepath.Join(src, "alpha", "extra.md"), []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(src, "alpha", "references", "ref.md")); err != nil {
		t.Fatal(err)
	}

	r, err := SyncSkills(dst, skillSrcs(src), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Updated) != 1 || r.Updated[0] != "__rulemux__alpha" {
		t.Fatalf("应更新 __rulemux__alpha，实得 %v", r.Updated)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "__rulemux__alpha", "SKILL.md")); string(got) != "v2" {
		t.Fatalf("SKILL.md 未更新为 v2：%q", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "__rulemux__alpha", "extra.md")); err != nil {
		t.Fatal("新增的 extra.md 未同步")
	}
	if _, err := os.Stat(filepath.Join(dst, "__rulemux__alpha", "references", "ref.md")); !os.IsNotExist(err) {
		t.Fatal("源里删掉的 references/ref.md 未被清掉")
	}
	if _, err := os.Stat(filepath.Join(dst, "__rulemux__alpha", "references")); !os.IsNotExist(err) {
		t.Fatal("变空的 references 目录未被清掉")
	}
}

// TestSyncSkillsResidueAndUserSkills：删源⇒清残留；用户自带 skill（无前缀）不碰。
func TestSyncSkillsResidueAndUserSkills(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mkSkill(t, src, "alpha", "a")
	mkSkill(t, src, "beta", "b")

	if _, err := SyncSkills(dst, skillSrcs(src), true); err != nil {
		t.Fatal(err)
	}
	// 用户自己的 skill（无前缀）
	if err := os.MkdirAll(filepath.Join(dst, "my-own"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "my-own", "SKILL.md"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 源里删掉 beta
	if err := os.RemoveAll(filepath.Join(src, "beta")); err != nil {
		t.Fatal(err)
	}
	r, err := SyncSkills(dst, skillSrcs(src), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Deleted) != 1 || r.Deleted[0] != "__rulemux__beta" {
		t.Fatalf("应整树删除 __rulemux__beta，实得 %v", r.Deleted)
	}
	if _, err := os.Stat(filepath.Join(dst, "__rulemux__beta")); !os.IsNotExist(err) {
		t.Fatal("__rulemux__beta 残留未清")
	}
	if _, err := os.Stat(filepath.Join(dst, "__rulemux__alpha", "SKILL.md")); err != nil {
		t.Fatal("__rulemux__alpha 被误删")
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "my-own", "SKILL.md")); string(got) != "mine" {
		t.Fatal("用户自己的 skill 被碰了")
	}
}

// TestSyncSkillsNameCollision：跨源同名 skill ⇒ 第二个追加短 hash，两个都保留。
func TestSyncSkillsNameCollision(t *testing.T) {
	srcA, srcB := t.TempDir(), t.TempDir()
	dst := t.TempDir()
	mkSkill(t, srcA, "dup", "from A")
	mkSkill(t, srcB, "dup", "from B")

	if _, err := SyncSkills(dst, skillSrcs(srcA, srcB), true); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), Prefix) {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) != 2 {
		t.Fatalf("同名 skill 应去重成两个目标目录，实得 %v", dirs)
	}
	bodies := map[string]bool{}
	for _, d := range dirs {
		b, _ := os.ReadFile(filepath.Join(dst, d, "SKILL.md"))
		bodies[string(b)] = true
	}
	if !bodies["from A"] || !bodies["from B"] {
		t.Fatalf("两个同名 skill 的内容都应保留，实得 %v", bodies)
	}
}

// TestSyncSkillsMissingRoot：源根不存在 ⇒ 记 Missing，不报错、不动目标。
func TestSyncSkillsMissingRoot(t *testing.T) {
	dst := t.TempDir()
	gone := filepath.Join(t.TempDir(), "gone")
	r, err := SyncSkills(dst, skillSrcs(gone), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Missing) != 1 || r.Missing[0] != gone {
		t.Fatalf("应记录缺失源根，实得 %v", r.Missing)
	}
}

// TestSyncSkillsCreateGate：create=false 且目录不存在 ⇒ 整次跳过（不建不读不删）。
func TestSyncSkillsCreateGate(t *testing.T) {
	src := t.TempDir()
	mkSkill(t, src, "alpha", "a")
	dst := filepath.Join(t.TempDir(), "skills")

	r, err := SyncSkills(dst, skillSrcs(src), false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.SkippedNoDir {
		t.Fatal("create=false 且目录不存在时应 SkippedNoDir")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("SkippedNoDir 时不该创建目录")
	}

	// create=true ⇒ 建目录并落盘
	if _, err := SyncSkills(dst, skillSrcs(src), true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "__rulemux__alpha", "SKILL.md")); err != nil {
		t.Fatal("create=true 时应创建目录并同步")
	}
}

// TestSyncSkillsLowerCaseAndRootItself：skill.md 小写认；根目录自身是 skill 也认。
func TestSyncSkillsLowerCaseAndRootItself(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	// 小写 skill.md
	lower := filepath.Join(src, "lower")
	if err := os.MkdirAll(lower, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lower, "skill.md"), []byte("lower"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 根本身是 skill（单技能目录布局）
	single := filepath.Join(t.TempDir(), "solo")
	if err := os.MkdirAll(single, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(single, "SKILL.md"), []byte("solo"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := SyncSkills(dst, skillSrcs(src, single), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Copied) != 2 {
		t.Fatalf("应同步 2 个 skill（小写 skill.md + 根自身），实得 %v", r.Copied)
	}
	if _, err := os.Stat(filepath.Join(dst, "__rulemux__lower", "skill.md")); err != nil {
		t.Fatal("小写 skill.md 的 skill 未同步")
	}
	if _, err := os.Stat(filepath.Join(dst, "__rulemux__solo", "SKILL.md")); err != nil {
		t.Fatal("根目录自身的 skill 未同步")
	}
}

// TestSyncSkillsNonSkillSubdirIgnored：不含 SKILL.md 的子目录被静默跳过。
func TestSyncSkillsNonSkillSubdirIgnored(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mkSkill(t, src, "real", "r")
	if err := os.MkdirAll(filepath.Join(src, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "not-a-skill", "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := SyncSkills(dst, skillSrcs(src), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Copied) != 1 || r.Copied[0] != "__rulemux__real" {
		t.Fatalf("只应同步 real，实得 %v", r.Copied)
	}
	if _, err := os.Stat(filepath.Join(dst, "__rulemux__not-a-skill")); !os.IsNotExist(err) {
		t.Fatal("非 skill 子目录不该被同步")
	}
}

// TestSyncSkillsEmptyWantClearsResidue：配置里 skill 源清空 ⇒ 前缀残留全清（加删自由）。
func TestSyncSkillsEmptyWantClearsResidue(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	mkSkill(t, src, "alpha", "a")
	if _, err := SyncSkills(dst, skillSrcs(src), true); err != nil {
		t.Fatal(err)
	}

	r, err := SyncSkills(dst, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Deleted) != 1 {
		t.Fatalf("空 want 应清掉全部前缀残留，实得 %v", r.Deleted)
	}
	entries, _ := os.ReadDir(dst)
	if len(entries) != 0 {
		t.Fatalf("清残后目标目录应为空，实得 %v", entries)
	}
}
