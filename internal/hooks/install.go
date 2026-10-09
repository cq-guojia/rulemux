// Package hooks 负责为各 agent 安装 SessionStart 钩子。
//
// 设计（docs/design/implementation.md #8）：无守护进程、不监听文件改动，
// 唯一触发机制就是各 agent 的 SessionStart 钩子调起 rulemux。
package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
)

// SubcommandFor 返回该 agent 的钩子应该调用的子命令：Tier-2 注入，Tier-1 同步。
func SubcommandFor(a agents.Agent) string {
	if a.Tier == agents.Tier2 {
		return "inject"
	}
	// Tier-1 必须带 --hook：它是「本次调用来自会话钩子」的唯一信号源，决定
	// 能否创建该 agent 的规则目录（设计 §二 原则 1）与能否向 stdout 写变化提示（§六）。
	return "sync --hook"
}

// Install 为指定 agent 安装 SessionStart 钩子，返回写入的配置文件路径（幂等）。
// display 是写入钩子命令的 --agent 标识：传用户 --agent 里写的原始值（可能是别名 workbuddy），
// 传空串则回退规范 ID。它控制钩子里写的是 --agent workbuddy 还是 --agent codebuddy。
func Install(a agents.Agent, workspace, display string) (string, error) {
	path := a.HookFileAbs(workspace)
	switch a.Style {
	case "claude", "trae", "json":
		return path, installJSON(path, a, display)
	case "codex":
		return path, installCodex(path, a.ID)
	default:
		return path, fmt.Errorf("unknown hook config style %q", a.Style)
	}
}

// ErrRefreshUnsupported 表示该 agent 的钩子配置格式尚未核实，因此刷新会跳过、绝不改文件。
var ErrRefreshUnsupported = errors.New("hook config style not verified for refresh")

// TargetCommand 是当前代码期望写进该 agent 钩子的完整命令行（规范 ID 版）。
//
// 参数必须写进 command 整串，不能用单独的 args 字段：宿主（如 CodeBuddy）只执行 command
// 字段本身、会丢弃 args（实测见 docs/design/external/agent-rules-dirs.md §二）。
func TargetCommand(a agents.Agent) string {
	return TargetCommandFor(a, "")
}

// TargetCommandFor 与 TargetCommand 相同，但允许用 display 覆盖 --agent 后面写的标识：
// 安装时用户可能写的是别名（如 --agent workbuddy），钩子与回显就该透传 workbuddy，
// 而不是回退到规范 ID codebuddy —— 这是「WorkBuddy / CodeBuddy 用户面分离」的一部分。
// display 为空时使用规范 ID（保持旧行为）。
func TargetCommandFor(a agents.Agent, display string) string {
	id := a.ID
	if display != "" {
		id = display
	}
	return fmt.Sprintf("rulemux %s --agent %s", SubcommandFor(a), id)
}

// AgentTokenOf 从一条钩子命令行中取出 --agent 后面的标识（支持 "--agent x" 与 "--agent=x"），
// 取不到返回空串。旧格式钩子（缺 --hook）或别名写法（--agent workbuddy）都能正确取出。
func AgentTokenOf(line string) string {
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == "--agent" && i+1 < len(fields) {
			return fields[i+1]
		}
		if strings.HasPrefix(f, "--agent=") {
			return strings.TrimPrefix(f, "--agent=")
		}
	}
	return ""
}

// ExpectedCommand 返回「相对某条已存在命令 cur，我们应当写出的目标命令」：沿用 cur 里
// 已有的 --agent 标识（保留用户当初选的品牌别名），仅在命令结构（如补 --hook）上做归一。
// cur 取不到标识时回退到规范 ID。用于 doctor 与 refresh 的比对：既不会把 workbuddy 误判为
// 过期，也不会让自愈把 workbuddy 悄悄改回 codebuddy。
func ExpectedCommand(a agents.Agent, cur string) string {
	if tok := AgentTokenOf(cur); tok != "" {
		return TargetCommandFor(a, tok)
	}
	return TargetCommand(a)
}

// hookEntry 构造该 agent 的一条 SessionStart 钩子分组，display 为写入命令的 --agent 标识。
func hookEntry(a agents.Agent, display string) map[string]interface{} {
	return map[string]interface{}{
		"matcher": "",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": TargetCommandFor(a, display),
			},
		},
	}
}

// installJSON 显式安装：写入 SessionStart 钩子，保留配置文件里已有的其它内容。
// 只有显式安装才允许创建配置目录；刷新路径绝不创建（见 Refresh）。
// display 为写入钩子命令的 --agent 标识（传用户 --agent 写的原始值，可能为别名 workbuddy）。
func installJSON(path string, a agents.Agent, display string) error {
	_, err := writeHookJSON(path, a, true, display)
	return err
}

// Inspect 报告该 agent 在宿主配置里的钩子现状（只读，不写盘）。
//
// installed=true 表示文件里已存在我们针对该 agent 的钩子（含旧格式、含 workbuddy 这类
// 别名写法）；command 是那条钩子的完整命令行 —— 调用方与 ExpectedCommand 比较即可判断
// 是否「已装但格式过期」（比对沿用 command 里已有的 --agent 标识，保留用户选的别名）。
//
// 文件不存在 / 为空 ⇒ (false, "", nil)；JSON 解析失败 ⇒ 返回错误（是否致命由调用方决定）；
// 钩子格式尚未核实的 agent（codex）⇒ ErrRefreshUnsupported。
func Inspect(path string, a agents.Agent) (installed bool, command string, err error) {
	if err := checkRefreshable(a); err != nil {
		return false, "", err
	}
	doc, err := loadHookJSON(path)
	if err != nil || doc == nil {
		return false, "", err
	}
	installed, command = inspectSessionStart(sessionStartList(doc), a)
	return installed, command, nil
}

// Refresh 只把「已存在的自家钩子条目」重写成当前目标命令，其余一律不动：
//
//   - 文件不存在 ⇒ 不创建文件、不创建目录，返回 changed=false；
//   - 已是最新 ⇒ 一个字节都不写（幂等，保持 mtime，让 postinstall 能做到无变化零输出）；
//   - 别人的钩子条目、以及文件里其它键 ⇒ 原样保留；
//   - 旧格式（缺 --hook）⇒ 补 --hook；别名条目（--agent workbuddy）⇒ 沿用已有的别名，
//     绝不强行改回规范 ID codebuddy（这是 WorkBuddy / CodeBuddy 用户面分离的关键）。
func Refresh(path string, a agents.Agent) (changed bool, err error) {
	if err := checkRefreshable(a); err != nil {
		return false, err
	}
	doc, err := loadHookJSON(path)
	if err != nil || doc == nil {
		return false, err
	}
	list := sessionStartList(doc)
	if installed, cur := inspectSessionStart(list, a); installed {
		// 沿用文件里已有的 --agent 标识（workbuddy 仍是 workbuddy），只在命令结构上归一。
		display := AgentTokenOf(cur)
		if cur == TargetCommandFor(a, display) {
			return false, nil
		}
		return writeHookJSON(path, a, false, display)
	}
	return false, nil
}

// checkRefreshable 报告该 agent 是否支持「解析 + 刷新」自家钩子条目。
func checkRefreshable(a agents.Agent) error {
	switch a.Style {
	case "claude", "trae", "json":
		return nil
	case "codex":
		// codex 的钩子 schema 尚未核实（见 registry 的 Note）。不能拿未核实的 TOML 解析
		// 结果去改用户文件 ⇒ 刷新直接跳过。它的 adapter 目前也装不上（Verified=false）。
		return fmt.Errorf("%w (agent %s)", ErrRefreshUnsupported, a.ID)
	default:
		return fmt.Errorf("unknown hook config style %q", a.Style)
	}
}

// writeHookJSON 是安装与刷新的共同写入核心。
//
// create=true 允许创建配置目录与文件（显式安装）；create=false 时文件不存在就直接跳过
// —— 这是硬约束：刷新只写已存在的配置。（changed 表示是否真的改了文件。）
// display 为写入钩子命令的 --agent 标识（安装时透传别名；刷新时沿用文件里已有的标识）。
func writeHookJSON(path string, a agents.Agent, create bool, display string) (changed bool, err error) {
	doc, err := loadHookJSON(path)
	if err != nil {
		return false, err
	}
	if doc == nil {
		if !create {
			return false, nil // 刷新不创建
		}
		doc = map[string]interface{}{}
	}

	hooksMap, _ := doc["hooks"].(map[string]interface{})
	if hooksMap == nil {
		hooksMap = map[string]interface{}{}
	}
	list, _ := hooksMap["SessionStart"].([]interface{})

	// 已是最新 ⇒ 不写盘：幂等，且让调用方（sync 自愈 / init --refresh）只在真有变化时才说话。
	if installed, cur := inspectSessionStart(list, a); installed && cur == TargetCommandFor(a, display) {
		return false, nil
	}

	// 按「规范 ID + 全部别名」剔除我们这个 agent 的旧条目：方案 A 后旧安装里的
	// `--agent workbuddy` 也是同一 agent 的钩子，必须一并清掉，否则会与新写的条目
	// 并存、同一 SessionStart 双触发。别人的条目一律原样保留。
	list = dropRulemuxHooks(list, a.MatchIDs())
	list = append(list, hookEntry(a, display))
	hooksMap["SessionStart"] = list
	doc["hooks"] = hooksMap

	if create {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return false, err
		}
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, err
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// loadHookJSON 读取并解析钩子配置：文件不存在或为空 ⇒ (nil, nil)。
func loadHookJSON(path string) (map[string]interface{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil, nil
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse existing hook config %s: %w", path, err)
	}
	if doc == nil {
		doc = map[string]interface{}{}
	}
	return doc, nil
}

// sessionStartList 取出 doc 里 hooks.SessionStart 列表（缺失返回 nil）。
func sessionStartList(doc map[string]interface{}) []interface{} {
	hooksMap, _ := doc["hooks"].(map[string]interface{})
	if hooksMap == nil {
		return nil
	}
	list, _ := hooksMap["SessionStart"].([]interface{})
	return list
}

// inspectSessionStart 在 SessionStart 列表里找本 agent 的钩子，返回是否已装与它的完整命令行。
func inspectSessionStart(list []interface{}, a agents.Agent) (installed bool, command string) {
	for _, g := range list {
		if !isRulemuxHookFor(g, a.MatchIDs()) {
			continue
		}
		if line, ok := firstCommandLine(g); ok {
			return true, line
		}
	}
	return false, ""
}

// firstCommandLine 返回一个 SessionStart 分组里第一条钩子的完整命令行。
func firstCommandLine(g interface{}) (string, bool) {
	gm, ok := g.(map[string]interface{})
	if !ok {
		return "", false
	}
	hs, ok := gm["hooks"].([]interface{})
	if !ok {
		return "", false
	}
	for _, h := range hs {
		hm, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		if line, ok := commandLineOf(hm); ok {
			return line, true
		}
	}
	return "", false
}

// commandLineOf 返回一个 hook 的完整命令行：优先用 command 字段本身；若存在旧式 args
// 字段（历史写法）则拼回去，以便识别并迁移旧配置。
func commandLineOf(hm map[string]interface{}) (string, bool) {
	cmd, ok := hm["command"].(string)
	if !ok || cmd == "" {
		return "", false
	}
	parts := []string{cmd}
	if args, ok := hm["args"].([]interface{}); ok {
		for _, x := range args {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, " "), true
}

// isRulemuxHookFor 报告一个 SessionStart 分组是否本工具针对 ids 中任一标识的钩子
// （新旧写法都认）。ids 通常是 agent.MatchIDs()（规范 ID + 全部别名）。
func isRulemuxHookFor(g interface{}, ids []string) bool {
	gm, ok := g.(map[string]interface{})
	if !ok {
		return false
	}
	hs, ok := gm["hooks"].([]interface{})
	if !ok {
		return false
	}
	for _, h := range hs {
		hm, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		line, ok := commandLineOf(hm)
		if !ok {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || filepath.Base(fields[0]) != "rulemux" {
			continue
		}
		for i, f := range fields {
			for _, id := range ids {
				if (f == "--agent" && i+1 < len(fields) && fields[i+1] == id) || f == "--agent="+id {
					return true
				}
			}
		}
	}
	return false
}

// dropRulemuxHooks 返回剔除「本工具针对 ids 中任一标识的钩子」后的列表，其余条目原样保留。
func dropRulemuxHooks(list []interface{}, ids []string) []interface{} {
	kept := make([]interface{}, 0, len(list))
	for _, g := range list {
		if isRulemuxHookFor(g, ids) {
			continue
		}
		kept = append(kept, g)
	}
	return kept
}

// installCodex 为 Codex 追加 TOML 钩子配置。
//
// ⚠ Codex 的钩子落点与 schema 尚未核实（见 agents.Note 与 docs/design/external/agent-rules-dirs.md §四），
// 这里按「features 开关 + hooks.SessionStart」的常规写法追加，需 canary 实测后校准。
func installCodex(path, agentID string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	}
	if strings.Contains(existing, "rulemux") {
		return nil // 已装过
	}

	var add strings.Builder
	add.WriteString("\n")
	if !strings.Contains(existing, "[features]") {
		add.WriteString("[features]\n")
	}
	add.WriteString("codex_hooks = true\n\n")
	add.WriteString("[[hooks.SessionStart]]\n")
	// 参数写进 command 整串：宿主只执行 command 字段，可能丢弃 args。
	fmt.Fprintf(&add, "command = \"rulemux inject --agent %s\"\n", agentID)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(add.String())
	return err
}

// Uninstall removes rulemux's SessionStart hook for the agent (idempotent:
// a missing or empty hook config is treated as already-removed).
func Uninstall(a agents.Agent, workspace string) error {
	path := a.HookFileAbs(workspace)
	switch a.Style {
	case "claude", "trae", "json":
		return uninstallJSON(path, a)
	case "codex":
		return uninstallCodex(path, a.ID)
	default:
		return fmt.Errorf("unknown hook style %q", a.Style)
	}
}

// uninstallJSON removes the rulemux SessionStart entry (matching this agentID)
// from a JSON hook config, preserving every other key/entry in the file.
func uninstallJSON(path string, a agents.Agent) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return nil
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("parse hook config %s: %w", path, err)
	}
	hooksMap, ok := doc["hooks"].(map[string]interface{})
	if !ok {
		return nil
	}
	list, ok := hooksMap["SessionStart"].([]interface{})
	if !ok {
		return nil
	}
	kept := dropRulemuxHooks(list, a.MatchIDs())
	if len(kept) == 0 {
		delete(hooksMap, "SessionStart")
	} else {
		hooksMap["SessionStart"] = kept
	}
	doc["hooks"] = hooksMap
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(path, out, 0o644)
}

// uninstallCodex removes the rulemux [[hooks.SessionStart]] block from the
// Codex TOML config. Naive line-based removal; codex schema is unverified.
func uninstallCodex(path, agentID string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(b), "\n")
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "[[hooks.SessionStart]]" {
			block := []string{line}
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !strings.HasPrefix(strings.TrimSpace(lines[i]), "[[") {
				block = append(block, lines[i])
				i++
			}
			if blockIsRulemux(block, agentID) {
				continue // drop this block
			}
			out = append(out, block...)
			continue
		}
		out = append(out, line)
		i++
	}
	result := strings.Join(out, "\n")
	if strings.TrimSpace(result) == "" {
		return os.Remove(path) // file only ever held our additions
	}
	return os.WriteFile(path, []byte(result), 0o644)
}

// blockIsRulemux reports whether a TOML [[hooks.SessionStart]] block is the
// rulemux hook for the given agent.
func blockIsRulemux(block []string, agentID string) bool {
	text := strings.Join(block, "\n")
	if !strings.Contains(text, "rulemux") {
		return false
	}
	if !strings.Contains(text, "--agent") {
		return false
	}
	return strings.Contains(text, agentID)
}
