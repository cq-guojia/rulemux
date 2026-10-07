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
		return path, fmt.Errorf("未知钩子配置风格 %q", a.Style)
	}
}

// installJSON 以 JSON 结构写入 SessionStart 钩子（hooks.SessionStart[]），
// 保留配置文件里已有的其它内容。
func installJSON(path, agentID, subcmd string) error {
	var doc map[string]interface{}
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		if err := json.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("解析现有钩子配置 %s 失败: %w", path, err)
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

	if !hasRulemuxHook(list, agentID) {
		list = append(list, map[string]interface{}{
			"matcher": "",
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": "rulemux",
					"args":    []string{subcmd, "--agent", agentID},
				},
			},
		})
	}
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

// hasRulemuxHook 判断是否已装过该 agent 的 rulemux 钩子（保证幂等、不重复装）。
func hasRulemuxHook(list []interface{}, agentID string) bool {
	for _, g := range list {
		gm, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		hs, ok := gm["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range hs {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, ok := hm["command"].(string)
			if !ok || cmd != "rulemux" {
				continue
			}
			args, ok := hm["args"].([]interface{})
			if !ok {
				continue
			}
			for _, x := range args {
				if s, ok := x.(string); ok && s == agentID {
					return true
				}
			}
		}
	}
	return false
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
	add.WriteString("command = \"rulemux\"\n")
	add.WriteString("args = [\"inject\", \"--agent\", \"" + agentID + "\"]\n")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(add.String())
	return err
}
