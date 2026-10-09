package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedHostSettings 在隔离的 HOME 下写一份宿主钩子配置，返回其路径。
func seedHostSettings(t *testing.T, home, body string) string {
	t.Helper()
	dir := filepath.Join(home, ".codebuddy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// hookCommands 返回 SessionStart 各分组的命令（按文件顺序）。
func hookCommands(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("配置文件已被写坏: %v\n%s", err, b)
	}
	hooks, _ := doc["hooks"].(map[string]interface{})
	list, _ := hooks["SessionStart"].([]interface{})
	out := make([]string, 0, len(list))
	for _, g := range list {
		gm, _ := g.(map[string]interface{})
		hs, _ := gm["hooks"].([]interface{})
		for _, h := range hs {
			hm, _ := h.(map[string]interface{})
			cmd, _ := hm["command"].(string)
			out = append(out, cmd)
		}
	}
	return out
}

// TestInitRefresh_UpgradesOldHookOnly：升级自愈的核心 —— 只把「我们自己那条旧钩子」
// 升级为当前格式；别家的钩子、以及文件里其它键必须原样保留；刷新模式不得生成配置样例。
func TestInitRefresh_UpgradesOldHookOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(home, "state"))

	settings := seedHostSettings(t, home, `{"theme":"dark","mcpServers":{"hindsight":{"command":"node"}},`+
		`"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"node \"/other.js\""}]},`+
		`{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent codebuddy"}]}]}}`)

	var code int
	out := captureStdout(t, func() { code = Init([]string{"--refresh"}) })
	if code != 0 {
		t.Fatalf("刷新应始终成功退出，实得 %d", code)
	}
	if !strings.Contains(out, "refreshed 1 hook(s)") {
		t.Errorf("输出未报告刷新了 1 条钩子:\n%s", out)
	}

	cmds := hookCommands(t, settings)
	if len(cmds) != 2 {
		t.Fatalf("want 2 条钩子（别家 1 + 我们 1）, got %v", cmds)
	}
	if cmds[0] != `node "/other.js"` {
		t.Errorf("别家的钩子被改动了: %q", cmds[0])
	}
	if want := "rulemux sync --hook --agent codebuddy"; cmds[1] != want {
		t.Errorf("自家旧钩子未升级: %q, want %q", cmds[1], want)
	}

	b, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["theme"] != "dark" {
		t.Errorf("其它键 theme 丢失: %v", doc["theme"])
	}
	if _, ok := doc["mcpServers"].(map[string]interface{}); !ok {
		t.Errorf("其它键 mcpServers 丢失: %v", doc["mcpServers"])
	}

	// 刷新不是安装：不得顺手生成配置样例，也不得创建 ~/.rulemux。
	if _, err := os.Stat(filepath.Join(home, ".rulemux", "config.toml")); !os.IsNotExist(err) {
		t.Errorf("刷新模式不该生成配置样例（stat err=%v）", err)
	}
}

// TestInitRefresh_NeverCreates：没装过就什么都不做 —— 尤其不得凭空创建宿主配置目录。
func TestInitRefresh_NeverCreates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(home, "state"))

	var code int
	out := captureStdout(t, func() { code = Init([]string{"--refresh"}) })
	if code != 0 {
		t.Fatalf("刷新应始终成功退出，实得 %d", code)
	}
	if !strings.Contains(out, "not installed, skipped") {
		t.Errorf("未装过的 agent 应被跳过:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".codebuddy")); !os.IsNotExist(err) {
		t.Errorf("刷新路径不该创建宿主配置目录（stat err=%v）", err)
	}
}

// TestInitRefresh_UnreadableConfigIsNotFatal：配置读不了 / 解析失败只警告，
// 绝不改文件、绝不返回失败 —— 它可能被脚本批量调用，不能污染调用方的结果。
func TestInitRefresh_UnreadableConfigIsNotFatal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(home, "state"))

	broken := "{not json"
	settings := seedHostSettings(t, home, broken)

	var code int
	out := captureStdout(t, func() { code = Init([]string{"--refresh"}) })
	if code != 0 {
		t.Fatalf("解析失败不该让刷新失败，实得 %d", code)
	}
	if !strings.Contains(out, "unreadable") {
		t.Errorf("应报告无法解析:\n%s", out)
	}
	b, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != broken {
		t.Errorf("解析失败的文件绝不能被改写: %q", b)
	}
}

// TestInitRefresh_HonorsConfigDirEnv：用户用 CODEBUDDY_CONFIG_DIR 挪走配置目录时，
// 刷新必须落到那个目录，而不是 ~/.codebuddy（否则钩子写进他不会读的位置）。
func TestInitRefresh_HonorsConfigDirEnv(t *testing.T) {
	home := t.TempDir()
	moved := filepath.Join(t.TempDir(), "moved-codebuddy")
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(home, "state"))
	t.Setenv("CODEBUDDY_CONFIG_DIR", moved)

	if err := os.MkdirAll(moved, 0o755); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(moved, "settings.json")
	old := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent codebuddy"}]}]}}`
	if err := os.WriteFile(settings, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	var code int
	out := captureStdout(t, func() { code = Init([]string{"--refresh"}) })
	if code != 0 {
		t.Fatalf("刷新应始终成功退出，实得 %d", code)
	}
	if !strings.Contains(out, "dir from $CODEBUDDY_CONFIG_DIR") {
		t.Errorf("输出未标明路径来自环境变量:\n%s", out)
	}

	cmds := hookCommands(t, settings)
	if len(cmds) != 1 || cmds[0] != "rulemux sync --hook --agent codebuddy" {
		t.Errorf("被挪走的配置目录里的钩子未升级: %v", cmds)
	}
	if _, err := os.Stat(filepath.Join(home, ".codebuddy")); !os.IsNotExist(err) {
		t.Errorf("环境变量生效时不该去动 ~/.codebuddy（stat err=%v）", err)
	}
}
