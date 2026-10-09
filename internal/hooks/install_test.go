package hooks

import (
	"encoding/json"
	"errors"
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
	if err := installJSON(path, codebuddy, ""); err != nil {
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
	if err := installJSON(path, codebuddy, ""); err != nil {
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
	if err := installJSON(path, codebuddy, ""); err != nil {
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
	if err := installJSON(path, codebuddy, ""); err != nil {
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

// 刷新（升级自愈）：只把「我们自己那条旧钩子」（缺 --hook）就地升级为当前命令，
// 别家的钩子与文件里其它键必须逐字保留 —— 用户强调「只改自己的」。
func TestRefresh_UpgradesOldHookAndKeepsEverythingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{
  "theme": "dark",
  "mcpServers": {"hindsight": {"command": "node", "args": ["/h.js"]}},
  "hooks": {"SessionStart": [
    {"hooks": [{"type": "command", "command": "node \"/other/h.js\""}]},
    {"matcher": "", "hooks": [{"type": "command", "command": "rulemux sync --agent codebuddy"}]}
  ]},
  "stopHook": {"Stop": [{"hooks": [{"type": "command", "command": "node \"/stop.js\""}]}]}
}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := Refresh(path, codebuddy)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !changed {
		t.Fatal("旧格式钩子应被刷新，changed 却为 false")
	}
	list := loadSessionStart(t, path)
	if len(list) != 2 {
		t.Fatalf("want 2 groups（别家 1 条 + 我们升级后的 1 条）, got %d", len(list))
	}
	if cmd, _ := hookCommand(t, list[0]); cmd != `node "/other/h.js"` {
		t.Errorf("别家的钩子被改动了：%q", cmd)
	}
	if cmd, _ := hookCommand(t, list[1]); cmd != "rulemux sync --hook --agent codebuddy" {
		t.Errorf("自家旧钩子未升级为整串新格式：%q", cmd)
	}

	var doc map[string]interface{}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"theme", "mcpServers", "stopHook"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("刷新不得丢键，%q 不见了: %s", key, b)
		}
	}
}

// 幂等：已是最新时一个字节都不写（调用方靠它做到「无变化零输出」）。
func TestRefresh_NoWriteWhenAlreadyCurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --hook --agent codebuddy"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := Refresh(path, codebuddy)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if changed {
		t.Error("已是最新不该报告 changed")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("已是最新却改写了文件:\n之前: %s\n之后: %s", before, after)
	}
}

// 硬约束：刷新绝不创建文件或目录。只有「该 agent 自己的会话钩子」与「用户显式 init」
// 才允许创建 —— 否则装个包就会在用户机器上凭空多出配置目录。
func TestRefresh_NeverCreatesFileOrDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-there")
	path := filepath.Join(dir, "settings.json")

	changed, err := Refresh(path, codebuddy)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if changed {
		t.Error("没有自家钩子时不该报告 changed")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("刷新路径不该创建目录 %s", dir)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("刷新路径不该创建文件 %s", path)
	}
}

// 别名条目（旧安装的 `--agent workbuddy`）要能被识别并合并为单条，且保留用户选的别名
// （不被强行改回 codebuddy）—— 否则它与新写的条目并存，同一 SessionStart 会双触发，
// 也违背 WorkBuddy / CodeBuddy 用户面分离。
func TestRefresh_MergesAliasEntryIntoCanonical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent workbuddy"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := Refresh(path, codebuddy)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !changed {
		t.Fatal("别名旧钩子应被合并刷新，changed 却为 false")
	}
	list := loadSessionStart(t, path)
	if len(list) != 1 {
		t.Fatalf("别名条目应合并为 1 条, got %d", len(list))
	}
	// 结构归一（补 --hook）且保留 workbuddy 别名，而非改回 codebuddy。
	if cmd, _ := hookCommand(t, list[0]); cmd != "rulemux sync --hook --agent workbuddy" {
		t.Errorf("command = %q", cmd)
	}
}

// Inspect 是 doctor 与刷新的共同判据：未装 / 已装但过期 / 已装且最新，三态要分得清。
func TestInspect_ReportsState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")

	if installed, cmd, err := Inspect(path, codebuddy); err != nil || installed || cmd != "" {
		t.Fatalf("文件不存在时 want (false,\"\",nil), got (%v,%q,%v)", installed, cmd, err)
	}

	seed := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent codebuddy"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	installed, cmd, err := Inspect(path, codebuddy)
	if err != nil || !installed {
		t.Fatalf("want installed=true, got (%v,%v)", installed, err)
	}
	if cmd != "rulemux sync --agent codebuddy" {
		t.Errorf("应报出旧命令以便识别过期：%q", cmd)
	}
	if cmd == TargetCommand(codebuddy) {
		t.Error("旧格式不该被判为最新")
	}

	if _, err := Refresh(path, codebuddy); err != nil {
		t.Fatal(err)
	}
	if _, cmd, _ := Inspect(path, codebuddy); cmd != TargetCommand(codebuddy) {
		t.Errorf("刷新后应为最新命令, got %q", cmd)
	}
}

// 刷新必须保留用户当初选的别名（workbuddy），不能悄悄改回规范 ID codebuddy ——
// 这是 WorkBuddy / CodeBuddy 用户面分离的关键。旧格式（缺 --hook）照样补上、但别名不变。
func TestRefresh_PreservesExistingAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent workbuddy"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Refresh(path, codebuddy); err != nil { // 用规范 ID 刷新，模拟 init --refresh 不带 --agent
		t.Fatal(err)
	}
	installed, cmd, err := Inspect(path, codebuddy)
	if err != nil || !installed {
		t.Fatalf("刷新后应仍装有钩子, got (%v,%v)", installed, err)
	}
	if want := "rulemux sync --hook --agent workbuddy"; cmd != want {
		t.Errorf("别名应被保留: got %q, want %q", cmd, want)
	}
}

// 未核实格式的 agent（codex 是 TOML，schema 未坐实）不参与解析/刷新，绝不改其文件。
func TestRefresh_SkipsUnverifiedStyle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	seed := "[features]\ncodex_hooks = true\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	codex := agents.Agent{ID: "codex", Tier: agents.Tier2, Style: "codex"}

	if _, err := Refresh(path, codex); !errors.Is(err, ErrRefreshUnsupported) {
		t.Errorf("Refresh want ErrRefreshUnsupported, got %v", err)
	}
	if _, _, err := Inspect(path, codex); !errors.Is(err, ErrRefreshUnsupported) {
		t.Errorf("Inspect want ErrRefreshUnsupported, got %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != seed {
		t.Errorf("未核实格式的 agent 其文件被改动了: %q", b)
	}
}

// 与刷新路径的关键区别：显式安装允许创建配置目录与文件。
func TestInstallJSON_CreatesDirAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", ".codebuddy", "settings.json")
	if err := installJSON(path, codebuddy, ""); err != nil {
		t.Fatalf("installJSON: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("显式安装应创建配置文件: %v", err)
	}
}

// 重复安装同一 agent 也必须幂等（第二次不写盘，避免无谓改动用户文件的 mtime）。
func TestInstallJSON_IdempotentSecondRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := installJSON(path, codebuddy, ""); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := installJSON(path, codebuddy, ""); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("第二次安装不该再写盘:\n之前: %s\n之后: %s", before, after)
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
