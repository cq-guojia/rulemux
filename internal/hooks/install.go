// Package hooks 负责为各 agent 安装 SessionStart 钩子。
//
// 设计（docs/design/implementation.md #8）：无守护进程、不监听文件改动，
// 唯一触发机制就是各 agent 的 SessionStart 钩子调起 rulemux。
package hooks

import (
	"encoding/json"
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
	return "sync"
}

// Install 为指定 agent 安装 SessionStart 钩子，返回写入的配置文件路径（幂等）。
func Install(a agents.Agent, workspace string) (string, error) {
	path := a.HookFileAbs(workspace)
	switch a.Style {
	case "claude", "trae", "json":
		return path, installJSON(path, a.ID, SubcommandFor(a))
	case "codex":
		return path, installCodex(path, a.ID)
	default:
		return path, fmt.Errorf("unknown hook config style %q", a.Style)
	}
}

// installJSON 以 JSON 结构写入 SessionStart 钩子（hooks.SessionStart[]），
// 保留配置文件里已有的其它内容。
func installJSON(path, agentID, subcmd string) error {
	var doc map[string]interface{}
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("failed to parse existing hook config %s: %w", path, err)
		}
	}
	if doc == nil {
		doc = map[string]interface{}{}
	}

	hooksMap, _ := doc["hooks"].(map[string]interface{})
	if hooksMap == nil {
		hooksMap = map[string]interface{}{}
	}
	list, _ := hooksMap["SessionStart"].([]interface{})

	// 参数必须写进 command 整串，不能用单独的 args 字段：宿主（如 CodeBuddy）只执行
	// command 字段本身、会丢弃 args（实测见 docs/design/external/agent-rules-dirs.md §二）。
	// 旧式把参数放在 args 里的钩子会被执行成裸 `rulemux`（无参数，只打印帮助、什么都不干），
	// 所以这里先剔除该 agent 的旧钩子、再统一写新式（幂等，且能把旧配置迁移过来）。
	list = dropRulemuxHooks(list, agentID)
	list = append(list, map[string]interface{}{
		"matcher": "",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": fmt.Sprintf("rulemux %s --agent %s", subcmd, agentID),
			},
		},
	})
	hooksMap["SessionStart"] = list
	doc["hooks"] = hooksMap

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
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

// isRulemuxHookFor 报告一个 SessionStart 分组是否本工具针对 agentID 的钩子（新旧写法都认）。
func isRulemuxHookFor(g interface{}, agentID string) bool {
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
			if (f == "--agent" && i+1 < len(fields) && fields[i+1] == agentID) || f == "--agent="+agentID {
				return true
			}
		}
	}
	return false
}

// dropRulemuxHooks 返回剔除「本工具针对 agentID 的钩子」后的列表，其余条目原样保留。
func dropRulemuxHooks(list []interface{}, agentID string) []interface{} {
	kept := make([]interface{}, 0, len(list))
	for _, g := range list {
		if isRulemuxHookFor(g, agentID) {
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
		return uninstallJSON(path, a.ID)
	case "codex":
		return uninstallCodex(path, a.ID)
	default:
		return fmt.Errorf("unknown hook style %q", a.Style)
	}
}

// uninstallJSON removes the rulemux SessionStart entry (matching this agentID)
// from a JSON hook config, preserving every other key/entry in the file.
func uninstallJSON(path, agentID string) error {
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
	kept := dropRulemuxHooks(list, agentID)
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
