package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr 临时接管 os.Stderr（与 captureStdout 同构）。
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	_ = w.Close()
	os.Stderr = old
	return <-done
}

// TestSyncReportFormat 校验新报告版式（2026-10-13 用户拍版）：
// 工作区块 = 路径一行 + 每 agent 一行；只报数不出明细；结尾总计；
// 未初始化 agent 折叠一行（不打叉）。
func TestSyncReportFormat(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	for _, d := range []string{
		filepath.Join(home, ".codebuddy"),
		filepath.Join(ws, ".codebuddy", "rules"),
		filepath.Join(ws, ".codebuddy", "skills"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillSrc := filepath.Join(tmp, "sk")
	if err := os.MkdirAll(filepath.Join(skillSrc, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillSrc, "alpha", "SKILL.md"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp,
		"[[source]]\npath = \""+src+"\"\n\n[[skill_source]]\npath = [\""+skillSrc+"\"]\n")

	out := captureStderr(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws, "--agent", "codebuddy"})
	})

	// 结构：标题 + 工作区路径独占一行 + agent 行只报数 + 总计
	for _, want := range []string{
		"rulemux sync",
		"↑ " + ws,                       // 工作区路径独占标题行（增量后缀对齐在后）
		"codebuddy  rules 1 · skills 1", // %-10s 补位 + 格式串一个空格
		"rules +1 · skills +1",          // 首拷 ⇒ 块标题行尾的增量
		"1 workspace · 1 agent initialized",
		"rules 1 file · +1 added · −0 removed · 0 updated",
		"skills 1 file · +1 added · −0 removed · 0 updated",
		"not initialized:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("报告应包含 %q，实得:\n%s", want, out)
		}
	}
	// 不出任何源文件/目标文件明细
	if strings.Contains(out, src) {
		t.Errorf("报告不应出现源文件明细，实得:\n%s", out)
	}
	if strings.Contains(out, "__rulemux__a.md") {
		t.Errorf("报告不应出现目标文件明细，实得:\n%s", out)
	}
}

// TestSyncReportNoChangesIdempotent 校验幂等：第二次同步无增减改，
// 工作区块 ✔ 且不带增量后缀，总计全零。
func TestSyncReportNoChangesIdempotent(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	for _, d := range []string{
		filepath.Join(home, ".codebuddy"),
		filepath.Join(ws, ".codebuddy", "rules"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	src := filepath.Join(tmp, "a.md")
	if err := os.WriteFile(src, []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+src+"\"\n")

	args := []string{"--config", cfgPath, "--workspace", ws, "--agent", "codebuddy"}
	_ = captureStderr(t, func() { Sync(args) }) // 首拷
	out := captureStderr(t, func() { Sync(args) })

	if !strings.Contains(out, "✔ "+ws) {
		t.Errorf("无变化时应为 ✔ 工作区块，实得:\n%s", out)
	}
	if strings.Contains(out, "rules +") || strings.Contains(out, "rules −") || strings.Contains(out, "rules ~") {
		t.Errorf("无变化时不应出现增量后缀，实得:\n%s", out)
	}
	if !strings.Contains(out, "rules 1 file · +0 added · −0 removed · 0 updated") {
		t.Errorf("总计应全零，实得:\n%s", out)
	}
}
