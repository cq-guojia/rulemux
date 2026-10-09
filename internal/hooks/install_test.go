package hooks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cq-guojia/rulemux/internal/agents"
)

// codebuddy 带别名 codebuddy-cn（workbuddy 已拆为独立条目，见 registry.go）。
var codebuddy = agents.Agent{
	ID:       "codebuddy",
	Tier:     agents.Tier1,
	Aliases:  []string{"codebuddy-cn"},
	RulesDir: ".codebuddy/rules",
	Style:    "claude",
}

// workbuddy 是独立 agent，钩子落到自己的 ~/.workbuddy/settings.json。
var workbuddy = agents.Agent{
	ID:       "workbuddy",
	Tier:     agents.Tier1,
	RulesDir: ".workbuddy/rules",
	HookFile: "~/.workbuddy/settings.json",
	HookAbs:  true,
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

// 同一 agent 不应在 SessionStart 里留下两条自己的钩子（否则同一会话双触发）。
// 旧安装若残留两条 `--agent codebuddy`，重装应合并为一条。
func TestInstallJSON_CleansUpDuplicateOwnHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[` +
		`{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent codebuddy"}]},` +
		`{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --hook --agent codebuddy"}]}` +
		`]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installJSON(path, codebuddy, ""); err != nil {
		t.Fatalf("installJSON: %v", err)
	}
	list := loadSessionStart(t, path)
	if len(list) != 1 {
		t.Fatalf("重复的自家钩子应被合并，want 1 group, got %d", len(list))
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

// 卸载只清自己的钩子：别家（node）与「另一个 agent 的 workbuddy 钩子」都原样保留 ——
// 验证拆分后两个 agent 互不干扰。
func TestUninstallJSON_RemovesOwnHookLeavesOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[` +
		`{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent codebuddy"}]},` +
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
	if len(list) != 2 {
		t.Fatalf("应剩 2 条（workbuddy + 别家），got %d", len(list))
	}
	cmds := map[string]bool{}
	for _, g := range list {
		c, _ := hookCommand(t, g)
		cmds[c] = true
	}
	if !cmds[`node "/other/h.js"`] {
		t.Error("别家的钩子应保留")
	}
	if !cmds["rulemux sync --agent workbuddy"] {
		t.Error("workbuddy 的钩子不应被 codebuddy 卸载波及")
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

// workbuddy 是独立 agent（不再是 codebuddy 的别名）：旧格式 `--agent workbuddy` 钩子应被
// 识别、归一为最新命令（补 --hook），且身份保持 workbuddy，不应被误判为 codebuddy。
func TestRefresh_MergesWorkbuddyIntoCanonical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent workbuddy"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := Refresh(path, workbuddy)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !changed {
		t.Fatal("旧格式 workbuddy 钩子应被刷新，changed 却为 false")
	}
	list := loadSessionStart(t, path)
	if len(list) != 1 {
		t.Fatalf("应合并为 1 条, got %d", len(list))
	}
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

// 刷新必须保留 workbuddy 的身份（不被误判/改回 codebuddy）：旧格式（缺 --hook）照样补上、但身份不变。
func TestRefresh_PreservesWorkbuddyIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --agent workbuddy"}]}]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Refresh(path, workbuddy); err != nil { // 用 workbuddy 刷新
		t.Fatal(err)
	}
	installed, cmd, err := Inspect(path, workbuddy)
	if err != nil || !installed {
		t.Fatalf("刷新后应仍装有钩子, got (%v,%v)", installed, err)
	}
	if want := "rulemux sync --hook --agent workbuddy"; cmd != want {
		t.Errorf("workbuddy 身份应被保留: got %q, want %q", cmd, want)
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

// workbuddy 的钩子必须落到自己的用户级配置 ~/.workbuddy/settings.json，
// 且该路径解析正确（不混入 codebuddy 那份文件）。
func TestInstall_WorkbuddyTargetsOwnConfigPath(t *testing.T) {
	ws := t.TempDir()
	path, err := Install(workbuddy, ws, "workbuddy")
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".workbuddy", "settings.json")
	if path != want {
		t.Fatalf("workbuddy 钩子路径 = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("钩子文件应被创建: %v", err)
	}
	list := loadSessionStart(t, path)
	if len(list) != 1 {
		t.Fatalf("want 1 group, got %d", len(list))
	}
	if cmd, _ := hookCommand(t, list[0]); cmd != "rulemux sync --hook --agent workbuddy" {
		t.Errorf("command = %q", cmd)
	}
}

// workbuddy 独立条目在已有别家钩子（如 hindsight 的两条）时只动自己那条、其余原样保留。
func TestInstallJSON_WorkbuddyPreservesOtherHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	seed := `{"hooks":{"SessionStart":[` +
		`{"hooks":[{"type":"command","command":"node \"/hindsight/a.js\""}]},` +
		`{"hooks":[{"type":"command","command":"node \"/hindsight/b.js\""}]}` +
		`]}}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installJSON(path, workbuddy, ""); err != nil {
		t.Fatalf("installJSON: %v", err)
	}
	list := loadSessionStart(t, path)
	if len(list) != 3 {
		t.Fatalf("want 3 groups (2 hindsight + 1 rulemux), got %d", len(list))
	}
	cmds := map[string]bool{}
	for _, g := range list {
		c, _ := hookCommand(t, g)
		cmds[c] = true
	}
	if !cmds["rulemux sync --hook --agent workbuddy"] {
		t.Error("未写入 rulemux 钩子")
	}
	if !cmds[`node "/hindsight/a.js"`] || !cmds[`node "/hindsight/b.js"`] {
		t.Errorf("hindsight 钩子被改动: %v", cmds)
	}
}

// 回归（2026-10-09）：trae 风格的合并必须保留顶层 version 与 hindsight 等其它条目，
// 只追加 rulemux 自己的 SessionStart 分组，绝不破坏既有配置。
func TestInstallJSON_TraePreservesVersionAndOtherEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	seed := `{
  "version": 1,
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "node \"/hindsight/sessionstart.js\"", "timeout": 30}]}
    ],
    "UserPromptSubmit": [
      {"hooks": [{"type": "command", "command": "node \"/hindsight/ups.js\"", "timeout": 30}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "node \"/hindsight/stop.js\"", "timeout": 60}]}
    ]
  }
}`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	trae := agents.Agent{ID: "trae", Tier: agents.Tier1, Style: "trae", RulesDir: ".trae/rules"}
	if err := installJSON(path, trae, ""); err != nil {
		t.Fatalf("installJSON: %v", err)
	}

	var doc map[string]interface{}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	// 1. 顶层 version 保留（JSON 数字反序列化为 float64）。
	if v, ok := doc["version"]; !ok || v != float64(1) {
		t.Fatalf("顶层 version 应保留为 1, got %v", doc["version"])
	}
	// 2. hindsight 的 UserPromptSubmit / Stop 事件条目都在。
	hooks, _ := doc["hooks"].(map[string]interface{})
	for _, ev := range []string{"UserPromptSubmit", "Stop"} {
		if _, ok := hooks[ev]; !ok {
			t.Errorf("hindsight 的 %s 事件条目丢失", ev)
		}
	}
	// 3. 只新增一条 rulemux 的 SessionStart 分组；hindsight 那条仍在。
	list := loadSessionStart(t, path)
	if len(list) != 2 {
		t.Fatalf("want 2 SessionStart groups (hindsight + rulemux), got %d", len(list))
	}
	cmds := map[string]bool{}
	for _, g := range list {
		c, _ := hookCommand(t, g)
		cmds[c] = true
	}
	if !cmds[`node "/hindsight/sessionstart.js"`] {
		t.Error("hindsight 的 SessionStart 条目被改动")
	}
	if !cmds["rulemux sync --hook --agent trae"] {
		t.Error("未写入 rulemux 的 trae 钩子")
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
