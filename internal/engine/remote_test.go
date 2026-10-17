package engine

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestClassifySource(t *testing.T) {
	cases := []struct {
		spec     string
		wantKind SourceKind
		wantKey  string
		wantSub  string
	}{
		{"~/my-skills", SourceLocal, "", ""},
		{"./x", SourceLocal, "", ""},
		{"/abs/dir", SourceLocal, "", ""},
		{"gh:owner/repo", SourceGitHub, "owner-repo", ""},
		{"github:owner/repo", SourceGitHub, "owner-repo", ""},
		{"owner/repo", SourceGitHub, "owner-repo", ""},
		{"git@github.com:owner/repo.git", SourceGit, "owner-repo", ""},
		{"https://github.com/owner/repo.git", SourceGit, "owner-repo", ""},
		{"https://gitlab.com/grp/sub/proj.git", SourceGit, "sub-proj", ""},
		{"npm:pkg", SourceNPM, "npm-pkg", ""},
		{"npm:@scope/name", SourceNPM, "npm-scope-name", ""},
		// 子路径（#）：base 决定 kind/key，sub 单列；staging key 忽略 sub ⇒ 同 repo 多子路径共享 clone
		{"gh:anthropics/skills#skills", SourceGitHub, "anthropics-skills", "skills"},
		{"gh:anthropics/skills#skills/docx", SourceGitHub, "anthropics-skills", "skills/docx"},
		{"git@github.com:o/r.git#sub", SourceGit, "o-r", "sub"},
		{"npm:pkg#x", SourceNPM, "npm-pkg", "x"},
		{"/abs/dir#sub", SourceLocal, "", "sub"},
	}
	for _, c := range cases {
		kind, key, _, sub := ClassifySource(c.spec)
		if kind != c.wantKind || key != c.wantKey || sub != c.wantSub {
			t.Errorf("ClassifySource(%q) = (%v, %q, sub=%q), want (%v, %q, sub=%q)",
				c.spec, kind, key, sub, c.wantKind, c.wantKey, c.wantSub)
		}
	}
}

func TestWithinDir(t *testing.T) {
	root := "/a/b"
	if !withinDir(root, "/a/b") || !withinDir(root, "/a/b/c") {
		t.Error("root and its children must be within")
	}
	if withinDir(root, "/a/bc") {
		t.Error("sibling sharing a name prefix must NOT be within")
	}
	if withinDir(root, "/a") {
		t.Error("parent must NOT be within")
	}
}

// gitRepo 建一个带一个 skill 的本地 git 仓库，返回其路径。
func gitRepo(t *testing.T, body string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "srcrepo")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("-c", "user.email=a@b.c", "-c", "user.name=t", "commit", "-q", "-m", "init")
	return repo
}

func TestEnsureStagedGitCloneAndCache(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("HOME", t.TempDir()) // staging 落临时 HOME
	repo := gitRepo(t, "name: x\ndescription: d\n")
	spec := "file://" + repo

	root, err := EnsureStaged(spec, 30*time.Second)
	if err != nil {
		t.Fatalf("EnsureStaged: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "SKILL.md")); err != nil || !bytes.Contains(b, []byte("name: x")) {
		t.Fatalf("staged SKILL.md wrong: %v %q", err, b)
	}
	// 幂等：改源后再次 EnsureStaged 不重新 clone（缓存原样）。
	if err := os.WriteFile(filepath.Join(repo, "SKILL.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root2, err := EnsureStaged(spec, 30*time.Second)
	if err != nil || root2 != root {
		t.Fatalf("second EnsureStaged: %v %q", err, root2)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "SKILL.md")); bytes.Contains(b, []byte("changed")) {
		t.Error("cached clone should NOT change without an explicit update")
	}
}

func TestUpdateStagedGitPull(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("HOME", t.TempDir())
	repo := gitRepo(t, "v1\n")
	spec := "file://" + repo
	root, err := EnsureStaged(spec, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// 源前进一个 commit，再 UpdateStaged ⇒ 原地 pull 跟上。
	if err := os.WriteFile(filepath.Join(repo, "SKILL.md"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"-c", "user.email=a@b.c", "-c", "user.name=t", "commit", "-q", "-m", "v2"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	before, after, err := UpdateStaged(spec, 30*time.Second)
	if err != nil {
		t.Fatalf("UpdateStaged: %v", err)
	}
	if before == after {
		t.Errorf("expected version to change after pull, before==after==%q", before)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "SKILL.md")); !bytes.Contains(b, []byte("v2")) {
		t.Errorf("git pull did not propagate upstream change, got %q", b)
	}
}

// TestUpdateStagedAutoFetch 验证：从未抓取过的源，update 应惰性 clone 而非报「not staged」。
func TestUpdateStagedAutoFetch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("HOME", t.TempDir())
	repo := gitRepo(t, "v1\n")
	spec := "file://" + repo
	before, after, err := UpdateStaged(spec, 30*time.Second)
	if err != nil {
		t.Fatalf("UpdateStaged auto-fetch: %v", err)
	}
	if before != "" {
		t.Errorf("first fetch should report empty before, got %q", before)
	}
	if after == "" {
		t.Error("after-fetch version should be non-empty (git describe / short hash)")
	}
}

// TestNpmVersionReadsPackageJSON 验证 npm 版本取自 package.json 的 version 字段。
func TestNpmVersionReadsPackageJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"),
		[]byte(`{"name":"x","version":"1.2.3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := npmVersion(dir); got != "1.2.3" {
		t.Errorf("npmVersion = %q, want 1.2.3", got)
	}
}

// TestGitDescribe 验证给一个有效 repo 能拿到非空版本标识。
func TestGitDescribe(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("HOME", t.TempDir())
	repo := gitRepo(t, "x\n")
	if got := gitDescribe(repo); got == "" {
		t.Error("gitDescribe returned empty for a valid repo")
	}
}

// makeTarGz 构造一个 tar.gz：files 为普通文件、links 为符号链接、escapes 为穿越条目。
func makeTarGz(t *testing.T, files, escapes map[string]string, links map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pkg.tgz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	reg := func(name, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	for n, b := range files {
		reg(n, b)
	}
	for n, b := range escapes {
		reg(n, b)
	}
	for n, target := range links {
		if err := tw.WriteHeader(&tar.Header{Name: n, Typeflag: tar.TypeSymlink, Linkname: target}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractTarGz(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	tgz := makeTarGz(t,
		map[string]string{"package/SKILL.md": "hi", "package/sub/a.txt": "x"},
		nil,
		map[string]string{"package/link": "/etc/passwd"},
	)
	if err := extractTarGz(tgz, dest); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "SKILL.md")); err != nil || string(b) != "hi" {
		t.Errorf("package/ prefix not stripped or content wrong: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(dest, "sub", "a.txt")); err != nil {
		t.Errorf("nested file missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "link")); err == nil {
		t.Error("symlink entry should have been skipped")
	}
}

func TestExtractTarGzRejectsEscape(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	tgz := makeTarGz(t, map[string]string{"package/ok.txt": "x"}, map[string]string{"package/../../evil.txt": "evil"}, nil)
	if err := extractTarGz(tgz, dest); err == nil {
		t.Fatal("expected path-traversal entry to be rejected")
	}
}

func TestDiscoverSkillsNested(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("name: x\ndescription: d\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("template/SKILL.md")
	mk("template/references/notes.md") // skill 内部资源，不是 skill
	mk("skills/docx/SKILL.md")
	mk("skills/docx/scripts/run.py") // skill 内部资源
	mk("skills/pdf/SKILL.md")
	mk(".git/objects/ab/SKILL.md")  // .git 内，须跳过
	mk("node_modules/foo/SKILL.md") // 依赖，须跳过
	mk("a/b/c/d/e/SKILL.md")        // 第 5 层，超 maxSkillScanDepth，不探

	got := map[string]bool{}
	for _, sk := range discoverSkills(root) {
		got[sk.Name] = true
	}
	for _, w := range []string{"template", "docx", "pdf"} {
		if !got[w] {
			t.Errorf("missing skill %q (got %v)", w, got)
		}
	}
	for _, bad := range []string{"references", "scripts", "ab", "foo", "e", "a", "node_modules"} {
		if got[bad] {
			t.Errorf("unexpectedly discovered %q", bad)
		}
	}
}

func TestDiscoverSkillsRootIsSkill(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 根自身是 skill 时不再下钻：即便子目录也有 SKILL.md 也不算独立 skill。
	if err := os.MkdirAll(filepath.Join(root, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "references", "SKILL.md"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	sk := discoverSkills(root)
	if len(sk) != 1 || sk[0].Name != filepath.Base(root) {
		t.Fatalf("root-is-skill should yield exactly one skill named %q, got %+v", filepath.Base(root), sk)
	}
}
