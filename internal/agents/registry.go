// Package agents 是 rulemux 的「每 agent 适配器」注册表。
//
// 每个 agent 在这里声明：属于哪个 Tier、规则目录在哪、钩子配置落在哪、该事实是否已核实。
// 核心同步逻辑（复制/比对/删残留）在 internal/engine，适配器只提供「落到哪里」的信息。
package agents

import (
	"os"
	"path/filepath"
	"strings"
)

// Tier 表示 agent 的适配层级。
type Tier int

const (
	// Tier1：有原生规则目录，直接真实拷贝文件（满足 A1–A3）。
	Tier1 Tier = iota + 1
	// Tier2：只认单文件（如 AGENTS.md），无目录可丢，走 SessionStart 钩子注入（降级，不保证 A3）。
	Tier2
)

// String 返回 Tier 的可读名称。
func (t Tier) String() string {
	switch t {
	case Tier1:
		return "Tier-1 目录同步"
	case Tier2:
		return "Tier-2 钩子注入"
	}
	return "未知"
}

// Agent 描述一个 agent 的适配信息。
type Agent struct {
	// ID 是唯一标识，配置里 agents 字段写它或它的别名。
	ID string
	// Aliases 是别名。
	Aliases []string
	// Tier 决定走目录同步还是钩子注入。
	Tier Tier
	// RulesDir 是工作区规则目录（相对工作区根），Tier2 为空。
	RulesDir string
	// HookFile 是钩子配置文件路径；HookAbs 为 true 时表示它是绝对路径（含 ~）。
	HookFile string
	// HookAbs 表示 HookFile 是否为绝对路径（不拼工作区）。
	HookAbs bool
	// Style 是钩子配置的写入风格：claude / trae / json（均为 JSON）或 codex（TOML）。
	Style string
	// Verified 表示「规则目录 / 钩子落点」是否已经官方核实。未核实的在 doctor 里标 ⚠，
	// 需按 features/verification.md 的 canary 法实测坐实后才可当实现依据。
	Verified bool
	// Note 备注。
	Note string
}

// registry 是当前支持的全部 agent。
var registry = []Agent{
	{
		ID:       "claude",
		Tier:     Tier1,
		RulesDir: ".claude/rules",
		HookFile: ".claude/settings.json",
		Style:    "claude",
		// 目录与 hook 能力已通过官方文档核实（external/agent-rules-dirs.md §三），但本工作区运行时
		// canary 仍未落地（open bug，T11 / §四），未坐实前不得作为实现依据 ⇒ 标 false，doctor 会标 ⚠。
		Verified: false,
		Note:     "官方文档已核实：会话开始读 .claude/rules/*.md；SessionStart 可 exec 形式 spawn 二进制。但本工作区运行时 canary 未落地（open bug，见 T11 / external/agent-rules-dirs.md §四），待开一次 Claude Code 会话坐实",
	},
	{
		ID:       "codebuddy",
		Aliases:  []string{"codebuddy-cn"},
		Tier:     Tier1,
		RulesDir: ".codebuddy/rules",
		HookFile: ".codebuddy/settings.json",
		Style:    "claude",
		Verified: true,
		Note:     "2026-10-08 canary 已坐实：SessionStart 钩子先于规则加载执行，且会读 .codebuddy/rules/ 下 .rulemux__ 点开头的隐藏文件",
	},
	{
		ID:       "workbuddy",
		Tier:     Tier1,
		RulesDir: ".codebuddy/rules",
		HookFile: ".codebuddy/settings.json",
		Style:    "claude",
		Verified: true,
		Note:     "复用 CodeBuddy 机制（随 CodeBuddy 一并 canary 坐实）；与 codebuddy 共用同一目录，sync 时按同目录合并计算应保留文件，避免互相误删",
	},
	{
		ID:       "trae",
		Aliases:  []string{"trae-cn", "trae-intl"},
		Tier:     Tier1,
		RulesDir: ".trae/rules",
		HookFile: "hooks.json",
		Style:    "trae",
		Verified: false,
		Note:     "CN 版与国际版差异仅在模型/账号/网络/合规，IDE 的规则目录与钩子机制是同一套 ⇒ 统一为一个适配器",
	},
	{
		ID:       "codex",
		Tier:     Tier2,
		RulesDir: "",
		HookFile: "~/.codex/config.toml",
		HookAbs:  true,
		Style:    "codex",
		Verified: false,
		Note:     "只认单文件 AGENTS.md（无目录模式）⇒ 走 SessionStart 注入，不碰用户自己的 AGENTS.md",
	},
	{
		ID:       "opencode",
		Tier:     Tier2,
		RulesDir: "",
		HookFile: "opencode.json",
		Style:    "json",
		Verified: false,
		Note:     "只认单文件 AGENTS.md ⇒ 走 SessionStart 注入，不碰用户自己的 AGENTS.md",
	},
}

// All 返回全部已注册的 agent。
func All() []Agent {
	out := make([]Agent, len(registry))
	copy(out, registry)
	return out
}

// Supported 返回当前已「做好」（canary 实测坐实，Verified==true）的 agent，
// 即安装程序允许安装的清单。未在此列的 agent 尚未就绪，禁止安装。
func Supported() []Agent {
	out := make([]Agent, 0, len(registry))
	for _, a := range registry {
		if a.Verified {
			out = append(out, a)
		}
	}
	return out
}

// IsSupported 报告该 ID/别名对应的 agent 是否已就绪、允许安装。
func IsSupported(idOrAlias string) bool {
	a, ok := Get(idOrAlias)
	return ok && a.Verified
}

// SupportedIDs 返回当前允许安装的 agent ID 列表。
func SupportedIDs() []string {
	ids := make([]string, 0, len(registry))
	for _, a := range Supported() {
		ids = append(ids, a.ID)
	}
	return ids
}

// SupportedSummary 返回逗号分隔的「当前可安装 agent」列表，用于错误/帮助文本。
func SupportedSummary() string {
	return strings.Join(SupportedIDs(), ", ")
}

// Get 按 ID 或别名查找 agent。
func Get(idOrAlias string) (Agent, bool) {
	for _, a := range registry {
		if strings.EqualFold(a.ID, idOrAlias) {
			return a, true
		}
		for _, al := range a.Aliases {
			if strings.EqualFold(al, idOrAlias) {
				return a, true
			}
		}
	}
	return Agent{}, false
}

// RulesDirAbs 返回该 agent 规则目录的绝对路径；Tier2 返回空串。
func (a Agent) RulesDirAbs(workspace string) string {
	if a.RulesDir == "" {
		return ""
	}
	if filepath.IsAbs(a.RulesDir) {
		return a.RulesDir
	}
	return filepath.Join(workspace, a.RulesDir)
}

// HookFileAbs 返回该 agent 钩子配置文件的绝对路径。
func (a Agent) HookFileAbs(workspace string) string {
	if a.HookAbs {
		return expandHome(a.HookFile)
	}
	return filepath.Join(workspace, a.HookFile)
}

// ByRulesDir 返回所有与给定相对规则目录相同的 Tier-1 agent。
// 用于「多个 agent 共用同一目录」时合并计算应保留文件集合，避免互相删掉对方的文件。
func ByRulesDir(rulesDirRel string) []Agent {
	if rulesDirRel == "" {
		return nil
	}
	var out []Agent
	for _, a := range registry {
		if a.Tier == Tier1 && a.RulesDir == rulesDirRel {
			out = append(out, a)
		}
	}
	return out
}

// expandHome 把 ~/ 开头的路径展开为当前用户的 HOME。
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil && h != "" {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}
