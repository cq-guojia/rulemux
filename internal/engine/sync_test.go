package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cq-guojia/rulemux/internal/config"
)

func TestBuildPlanNaming(t *testing.T) {
	p := BuildPlan("/dir", []config.Source{
		{Paths: []string{"/x/a.md"}},
		{Paths: []string{"/y/b.txt"}},
	})
	if p.Items[0].DstName != "__rulemux__a.md" {
		t.Fatalf("期望 __rulemux__a.md，实得 %s", p.Items[0].DstName)
	}
	if p.Items[1].DstName != "__rulemux__b.txt" {
		t.Fatalf("期望 __rulemux__b.txt，实得 %s", p.Items[1].DstName)
	}
}

// TestBuildPlanMultiplePaths 校验一条 source 里 path 写数组时，每个文件都会展开。
func TestBuildPlanMultiplePaths(t *testing.T) {
	p := BuildPlan("/dir", []config.Source{
		{Paths: []string{"/x/a.md", "/x/b.md", "/x/c.md"}},
	})
	if len(p.Items) != 3 {
		t.Fatalf("path 数组应展开成 3 个文件，实得 %d", len(p.Items))
	}
}

func TestBuildPlanDedupe(t *testing.T) {
	p := BuildPlan("/dir", []config.Source{
		{Paths: []string{"/x/a.md"}},
		{Paths: []string{"/y/a.md"}},
	})
	if p.Items[0].DstName == p.Items[1].DstName {
		t.Fatalf("同名 basename 未去重：%s", p.Items[0].DstName)
	}
	for _, it := range p.Items {
		if !strings.HasPrefix(it.DstName, Prefix) {
			t.Fatalf("缺少前缀：%s", it.DstName)
		}
	}
}

// TestBuildPlanSamePathAcrossSources 校验：同一源文件路径被多条 source 引用时，
// 只在最终列表保留一次（按源路径去重），不会因命中多条策略而重复注入。
func TestBuildPlanSamePathAcrossSources(t *testing.T) {
	p := BuildPlan("/dir", []config.Source{
		{Paths: []string{"/x/a.md"}},
		{Paths: []string{"/x/a.md"}}, // 同一条源被另一条 source 再次引用
	})
	if len(p.Items) != 1 {
		t.Fatalf("同一源路径跨多条 source 应只注入一次，实得 %d 个 item", len(p.Items))
	}
}

func TestSyncCopySkipUpdateDelete(t *testing.T) {
	srcDir := t.TempDir()
	target := t.TempDir()
	src := filepath.Join(srcDir, "src.md")

	if err := os.WriteFile(src, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcs := []config.Source{{Paths: []string{src}}}

	// 首次：复制
	r, err := Sync(target, srcs, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Copied) != 1 {
		t.Fatalf("首次应复制 1 个，实得 %d", len(r.Copied))
	}
	if !r.HasChanges() {
		t.Fatal("首次复制应算作有变化")
	}

	// 再次：内容一致 ⇒ 跳过（幂等、不累积）
	r, err = Sync(target, srcs, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Skipped) != 1 || len(r.Copied) != 0 {
		t.Fatalf("二次应跳过 1 个且不复制，实得 skipped=%d copied=%d", len(r.Skipped), len(r.Copied))
	}
	if r.HasChanges() {
		t.Fatal("全部跳过时不该算作有变化")
	}

	// 源变更 ⇒ 覆盖
	if err := os.WriteFile(src, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err = Sync(target, srcs, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Updated) != 1 {
		t.Fatalf("变更应更新 1 个，实得 %d", len(r.Updated))
	}
	got, err := os.ReadFile(filepath.Join(target, Prefix+"src.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2" {
		t.Fatalf("覆盖后内容应为 v2，实得 %q", string(got))
	}

	// 源从配置移除 ⇒ 目标残留被清
	r, err = Sync(target, nil, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Deleted) != 1 {
		t.Fatalf("应删残留 1 个，实得 %d", len(r.Deleted))
	}
	if !r.HasChanges() {
		t.Fatal("删除残留也应算作有变化（供变化提示使用）")
	}
	if _, err := os.Stat(filepath.Join(target, Prefix+"src.md")); !os.IsNotExist(err) {
		t.Fatal("残留未被删除")
	}
}

// TestSyncInjectsFrontmatter 校验：需要 frontmatter 的 agent（CodeBuddy/WorkBuddy）落盘文件
// 带 alwaysApply:true 头，且比对与写入都基于「加头后」内容（二次同步应跳过，不反复覆盖）。
func TestSyncInjectsFrontmatter(t *testing.T) {
	srcDir := t.TempDir()
	target := t.TempDir()
	src := filepath.Join(srcDir, "src.md")
	if err := os.WriteFile(src, []byte("# body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcs := []config.Source{{Paths: []string{src}}}

	r, err := Sync(target, srcs, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Copied) != 1 {
		t.Fatalf("首次应复制 1 个，实得 %d", len(r.Copied))
	}
	got, err := os.ReadFile(filepath.Join(target, Prefix+"src.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := AutoApplyFrontmatter + "# body\n"
	if string(got) != want {
		t.Fatalf("注入 frontmatter 后内容应为 %q，实得 %q", want, string(got))
	}

	r, err = Sync(target, srcs, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Skipped) != 1 {
		t.Fatalf("二次应跳过 1 个（比对基于加头后内容），实得 skipped=%d updated=%d", len(r.Skipped), len(r.Updated))
	}
}

func TestSyncIgnoresUserFiles(t *testing.T) {
	target := t.TempDir()
	userFile := filepath.Join(target, "my-own.md")
	if err := os.WriteFile(userFile, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(target, nil, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Fatal("用户自己的文件被删了")
	}
}

// TestSyncMissingSource 校验：源文件不存在时只记录 Missing，且**不算作「有变化」**
// —— 否则「变化提示」会在每次会话误报，而用户重开一百次也没用。
func TestSyncMissingSource(t *testing.T) {
	target := t.TempDir()
	r, err := Sync(target, []config.Source{{Paths: []string{"/definitely/not/here.md"}}}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Missing) != 1 {
		t.Fatalf("应记录 1 个缺失源，实得 %d", len(r.Missing))
	}
	if r.HasChanges() {
		t.Fatal("源文件缺失时磁盘并没有变，不该算作有变化")
	}
}

// TestSyncCreateFalseSkipsMissingDir 校验原则 2：不允许创建时，目录不存在 ⇒ 整次跳过
// （不建、不读、不删），并标记 SkippedNoDir。
func TestSyncCreateFalseSkipsMissingDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "rules") // 故意不存在
	src := filepath.Join(base, "s.md")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Sync(dir, []config.Source{{Paths: []string{src}}}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.SkippedNoDir {
		t.Fatal("目录不存在且 create=false 时应标记 SkippedNoDir")
	}
	if r.HasChanges() {
		t.Fatal("跳过不该算作变化")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("create=false 时不该创建目录")
	}
}

// TestSyncCreateTrueCreatesDir 校验原则 1：允许创建时（该 agent 自己的 hook），目录会被建出来。
func TestSyncCreateTrueCreatesDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "rules")
	src := filepath.Join(base, "s.md")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Sync(dir, []config.Source{{Paths: []string{src}}}, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Copied) != 1 {
		t.Fatalf("应复制 1 个，实得 %d", len(r.Copied))
	}
	if _, err := os.Stat(filepath.Join(dir, Prefix+"s.md")); err != nil {
		t.Fatal("create=true 时应建目录并落文件")
	}
}

// TestSyncPartialFailureKeepsCompleted 校验分文件容错：一个文件写失败不中断其余文件，
// 且已完成的结果照常返回（变化提示据此仍能触发）。用「目标名是个目录」制造写失败，
// 与运行用户权限无关，跨平台稳定。
func TestSyncPartialFailureKeepsCompleted(t *testing.T) {
	base := t.TempDir()
	target := t.TempDir()
	blocked := filepath.Join(base, "a.md")
	good := filepath.Join(base, "b.md")
	if err := os.WriteFile(blocked, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(good, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(target, Prefix+"a.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	r, err := Sync(target, []config.Source{{Paths: []string{blocked, good}}}, false, true)
	if err != nil {
		t.Fatalf("单文件失败不该让整次同步报错：%v", err)
	}
	if len(r.Errors) != 1 {
		t.Fatalf("应记录 1 个错误，实得 %d (%v)", len(r.Errors), r.Errors)
	}
	if len(r.Copied) != 1 {
		t.Fatalf("另一个文件应照常复制，实得 copied=%d (%v)", len(r.Copied), r.Copied)
	}
	if !r.HasChanges() {
		t.Fatal("已完成的复制应算作变化")
	}
}
