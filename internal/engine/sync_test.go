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
	if p.Items[0].DstName != ".rulemux__a.md" {
		t.Fatalf("期望 .rulemux__a.md，实得 %s", p.Items[0].DstName)
	}
	if p.Items[1].DstName != ".rulemux__b.txt" {
		t.Fatalf("期望 .rulemux__b.txt，实得 %s", p.Items[1].DstName)
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
	r, err := Sync(target, srcs)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Copied) != 1 {
		t.Fatalf("首次应复制 1 个，实得 %d", len(r.Copied))
	}

	// 再次：内容一致 ⇒ 跳过（幂等、不累积）
	r, err = Sync(target, srcs)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Skipped) != 1 || len(r.Copied) != 0 {
		t.Fatalf("二次应跳过 1 个且不复制，实得 skipped=%d copied=%d", len(r.Skipped), len(r.Copied))
	}

	// 源变更 ⇒ 覆盖
	if err := os.WriteFile(src, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err = Sync(target, srcs)
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
	r, err = Sync(target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Deleted) != 1 {
		t.Fatalf("应删残留 1 个，实得 %d", len(r.Deleted))
	}
	if _, err := os.Stat(filepath.Join(target, Prefix+"src.md")); !os.IsNotExist(err) {
		t.Fatal("残留未被删除")
	}
}

func TestSyncIgnoresUserFiles(t *testing.T) {
	target := t.TempDir()
	userFile := filepath.Join(target, "my-own.md")
	if err := os.WriteFile(userFile, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(target, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Fatal("用户自己的文件被删了")
	}
}

func TestSyncMissingSource(t *testing.T) {
	target := t.TempDir()
	r, err := Sync(target, []config.Source{{Paths: []string{"/definitely/not/here.md"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Missing) != 1 {
		t.Fatalf("应记录 1 个缺失源，实得 %d", len(r.Missing))
	}
}
