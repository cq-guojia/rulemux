package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cq-guojia/rulemux/internal/agents"
)

// codebuddy 带别名 workbuddy（方案 A：两者是同一个 agent）。
var codebuddy = agents.Agent{
	ID:       "codebuddy",
	Tier:     agents.Tier1,
	Aliases:  []string{"workbuddy", "codebuddy-cn"},
	RulesDir: ".codebuddy/rules",
	Style:    "claude",
}

// 回归（2026-10-08）：钩子命令必须是 command 整串，且不得带 args 字段。
// 宿主（如 CodeBuddy）只执行 command 字段、会丢弃 args，旧写法会被执行成裸
// `rulemux`（无参数、只打印帮助、不同步）——这正是线上“规则从不生效”的根因。
// 2026-10-09 起 Tier-1 的命令再补 `--hook`：它是「本次来自会话钩子」的唯一信号源，
// 决定能否创建规则目录、能否输出变化提示。
func TestInstallJSON_WritesFullCommandString(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := installJSON(path, codebuddy); err != nil {
		t.Fatalf("installJSON: %v", err)
	}
	list := loadSessionStart(t, path)
	if len(list) != 1 {
		t.Fatalf("want 1 SessionStart group, got %d", len(list))
	}
	cmd, hasArgs := hookCommand(t, list[0])
	if hasArgs {
		t.Error("hook must not carry an args field (hosts drop it)")
	}
	if want := "rulemux sync --hook --agent codebuddy"; cmd != want {
		t.Errorf("command = %q, want %q", cmd, want)
	}
}

// 旧式（args 写法）配置必须被迁移为新式整串，同时保留文件中其它工具的钩子。
func TestInstallJSON_MigratesOldArgsHookAndKeepsOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[` +
		`{"hooks":[{"type":"command","command":"node \"/other/h.js\""}]},` +
		`{"matcher":"","hooks":[{"type":"command","command":"rulemux","args":["sync","--agent","codebuddy"]}]}` +
		`]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installJSON(path, codebuddy); err != nil {
		t.Fatalf("installJSON: %v", err)
	}
	list := loadSessionStart(t, path)
	if len(list) != 2 {
		t.Fatalf("want 2 groups (other tool kept + rulemux rewritten), got %d", len(list))
	}
	if cmd, _ := hookCommand(t, list[0]); cmd != `node "/other/h.js"` {
		t.Errorf("other tool's hook not preserved: %q", cmd)
	}
	cmd, hasArgs := hookCommand(t, list[1])
	if hasArgs || cmd != "rulemux sync --hook --agent codebuddy" {
		t.Errorf("old args-style hook not migrated: cmd=%q hasArgs=%v", cmd, hasArgs)
	}
}

// 迁移（方案 A）：旧安装里 `--agent workbuddy` 那条钩子必须被一并清掉 ——
// 否则它与新写的 codebuddy 条目并存，同一 SessionStart 会双触发。
func TestInstallJSON_CleansUpAliasHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[` +
		`{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent workbuddy"}]}` +
		`]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installJSON(path, codebuddy); err != nil {
		t.Fatalf("installJSON: %v", err)
	}
	list := loadSessionStart(t, path)
	if len(list) != 1 {
		t.Fatalf("别名旧钩子应被清理，want 1 group, got %d", len(list))
	}
	if cmd, _ := hookCommand(t, list[0]); cmd != "rulemux sync --hook --agent codebuddy" {
		t.Errorf("command = %q", cmd)
	}
}

func TestUninstallJSON_RemovesHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := installJSON(path, codebuddy); err != nil {
		t.Fatal(err)
	}
	if err := uninstallJSON(path, codebuddy); err != nil {
		t.Fatalf("uninstallJSON: %v", err)
	}
	if list := loadSessionStart(t, path); len(list) != 0 {
		t.Fatalf("want 0 groups after uninstall, got %d", len(list))
	}
}

// 卸载同样要认别名：只剩 `--agent workbuddy` 的旧配置也必须被清干净。
func TestUninstallJSON_RemovesAliasHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[` +
		`{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent workbuddy"}]},` +
		`{"hooks":[{"type":"command","command":"node \"/other/h.js\""}]}` +
		`]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := uninstallJSON(path, codebuddy); err != nil {
		t.Fatalf("uninstallJSON: %v", err)
	}
	list := loadSessionStart(t, path)
	if len(list) != 1 {
		t.Fatalf("只该剩别家的 1 条，got %d", len(list))
	}
	if cmd, _ := hookCommand(t, list[0]); cmd != `node "/other/h.js"` {
		t.Errorf("other tool's hook not preserved: %q", cmd)
	}
}

func loadSessionStart(t *testing.T, path string) []interface{} {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	hooks, _ := doc["hooks"].(map[string]interface{})
	list, _ := hooks["SessionStart"].([]interface{})
	return list
}

func hookCommand(t *testing.T, group interface{}) (string, bool) {
	t.Helper()
	gm, ok := group.(map[string]interface{})
	if !ok {
		t.Fatalf("hook group is not an object: %v", group)
	}
	hs, _ := gm["hooks"].([]interface{})
	if len(hs) == 0 {
		t.Fatalf("hook group has no hooks: %v", group)
	}
	hm, _ := hs[0].(map[string]interface{})
	cmd, _ := hm["command"].(string)
	_, hasArgs := hm["args"]
	return cmd, hasArgs
}
