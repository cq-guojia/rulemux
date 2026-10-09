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
		return "Tier-1 directory sync"
	case Tier2:
		return "Tier-2 hook injection"
	}
	return "unknown"
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
	// HookDirEnv 是「可覆盖用户级配置目录」的环境变量名。**只在已核实（有上游产物 / 官方文档
	// 证据）时才可填写**；为空表示只按 ~ 展开。设了它且该变量非空时，钩子文件解析为
	// <变量值>/<HookFileBase>，优先于 HookFile 的字面量 —— 因为用户可能把配置目录挪走了，
	// 那时写默认位置等于写进一个他不会读的目录。
	HookDirEnv string
	// HookFileBase 是配置目录下的钩子文件名（仅当 HookDirEnv 生效时使用），如 settings.json。
	HookFileBase string
	// Style 是钩子配置的写入风格：claude / trae / json（均为 JSON）或 codex（TOML）。
	Style string
	// Verified 表示「规则目录 / 钩子落点」是否已经官方核实。未核实的在 doctor 里标 ⚠，
	// 需按 features/verification.md 的 canary 法实测坐实后才可当实现依据。
	Verified bool
	// NeedsFrontmatter 表示落盘文件必须带 alwaysApply:true 的 YAML 头，该 agent 才会在
	// 会话开始自动加载规则（CodeBuddy/WorkBuddy 实测如此）。Tier-2 无意义。
	NeedsFrontmatter bool
	// SessionHint 表示该 agent 的会话钩子在「规则确有变化」时应给模型注入一句话
	// （提示用户重开会话生效）。前提：该 agent 在会话内**不会**重读规则目录 ——
	// 若某家会热重载，就应保持 false（见设计 §六「per-agent」）。
	SessionHint bool
	// HintProtocol 是提示的输出协议标识；空串 = 不提示（此时 SessionHint 也应为 false）。
	HintProtocol string
	// Note 备注。
	Note string
}

// ProtocolSessionStartAdditionalContext 是 Claude 系宿主（Claude Code / CodeBuddy 等）认的
// SessionStart 注入协议：stdout 输出
//
//	{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"…"}}
//
// 已由本机 Hindsight 的 codebuddy-sessionstart-hook.js 实证（见设计 §六）。
const ProtocolSessionStartAdditionalContext = "session-start-additional-context"

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
		Note:     "Verified via official docs: .claude/rules/*.md is read at session start; SessionStart can exec-spawn a binary. However the canary has not landed in this workspace yet (open bug, see T11 / external/agent-rules-dirs.md section 4) - still needs one Claude Code session to confirm.",
	},
	{
		ID: "codebuddy",
		// WorkBuddy 曾被视为与 CodeBuddy 完全相同的 agent，以别名 "workbuddy" 收纳于本条目
		// （design §三 方案 A）。2026-10-09 本机实测推翻该结论：WorkBuddy 有自己独立的用户级
		// 配置目录 ~/.workbuddy 与独立钩子文件 ~/.workbuddy/settings.json，并不读 CodeBuddy 那份，
		// 故已拆为 registry 中的独立条目（见下方 workbuddy）。本条目不再收纳 workbuddy 别名。
		Aliases:  []string{"codebuddy-cn"},
		Tier:     Tier1,
		RulesDir: ".codebuddy/rules",
		// 2026-10-08 起钩子落点为 user 级 host 配置（不再是每工作区的 .codebuddy/settings.json），
		// 与 Hindsight 的做法一致，避免 hook 配置散落在各工作区、易被误改。
		HookFile: "~/.codebuddy/settings.json",
		HookAbs:  true,
		// 已核实（2026-10-09，本机 4.12.1 产物 extensions/genie/out/extension/index.js）：
		//   CODEBUDDY_CONFIG_DIR_ENV="CODEBUDDY_CONFIG_DIR"；
		//   getCodeBuddyHomeDir() = env["CODEBUDDY_CONFIG_DIR"] || join(homedir(), ".codebuddy")
		// ⇒ 用户用该变量挪走配置目录时必须跟随，否则钩子会被写进他不会读的位置。
		// 注意：产物里另一个变量 WORKBUDDY_CONFIG_DIR 只在 safe-delete 的日志白名单里出现，
		// 未能坐实它是配置目录 ⇒ 不采用（见 docs/design/external/agent-rules-dirs.md §五）。
		HookDirEnv:       "CODEBUDDY_CONFIG_DIR",
		HookFileBase:     "settings.json",
		Style:            "claude",
		Verified:         true,
		NeedsFrontmatter: true,
		// 会话开始即固定规则快照 ⇒ 改动/新增要下一次会话才生效 ⇒ 有变化时给模型一句话。
		SessionHint:  true,
		HintProtocol: ProtocolSessionStartAdditionalContext,
		Note:         "Canary re-confirmed 2026-10-08: CodeBuddy auto-loads FLAT, NON-hidden .md files under .codebuddy/rules that carry an alwaysApply:true frontmatter; dot-prefixed (hidden) files are SKIPPED, and the RULE.mdc subdir layout is NOT relied on (its load behaviour was inconsistent across runs). rulemux therefore writes __rulemux__<name>.md with a frontmatter header. Hook lives in the user-level ~/.codebuddy/settings.json; see external/agent-rules-dirs.md. NOTE (2026-10-09): WorkBuddy previously folded in here as the alias \"workbuddy\" (design §三 plan A), but a local inspection proved WorkBuddy uses its OWN ~/.workbuddy/settings.json and never reads this file, so it is now a separate registry entry below — this entry and WorkBuddy share nothing.",
	},
	{
		// WorkBuddy 是独立应用（macOS Electron，/Applications/WorkBuddy.app/），有自己独立的
		// 用户级配置目录 ~/.workbuddy 与钩子文件 ~/.workbuddy/settings.json（2026-10-09 本机实测）。
		// 它不读 CodeBuddy 的 ~/.codebuddy/settings.json，故拆为独立条目；此前当成别名收纳
		// （方案 A）会把钩子写错目录、永不触发。工作区级 rules 目录（.workbuddy/rules vs
		// .codebuddy/rules）待 canary 实测校准，先标 🔴。
		ID:       "workbuddy",
		Tier:     Tier1,
		RulesDir: ".workbuddy/rules", // 🔴 待 canary 实测校准（.workbuddy/rules vs .codebuddy/rules）
		HookFile: "~/.workbuddy/settings.json",
		HookAbs:  true,
		// WORKBUDDY_CONFIG_DIR 仅出现在 safe-delete 日志白名单（见 external/agent-rules-dirs.md §五），
		// 未坐实是配置目录 ⇒ 不采用；本机 env 实测该变量为空，按 ~/.workbuddy 是稳的。
		HookDirEnv:       "",
		HookFileBase:     "",
		Style:            "claude",
		Verified:         true,
		NeedsFrontmatter: true,
		SessionHint:      true,
		HintProtocol:     ProtocolSessionStartAdditionalContext,
		Note:             "Independent app (macOS Electron, /Applications/WorkBuddy.app/). Local inspection 2026-10-09 proved it has its OWN user-level config dir ~/.workbuddy and hook file ~/.workbuddy/settings.json — it does NOT read CodeBuddy's ~/.codebuddy/settings.json, so it is a separate registry entry (previously wrongly folded into codebuddy as an alias, design §三 plan A). RulesDir is .workbuddy/rules 🔴 pending canary confirmation (vs .codebuddy/rules). WORKBUDDY_CONFIG_DIR is only in a safe-delete log whitelist, not adopted (external/agent-rules-dirs.md §五).",
	},
	{
		ID:       "trae",
		Aliases:  []string{"trae-cn", "trae-intl"},
		Tier:     Tier1,
		RulesDir: ".trae/rules",
		HookFile: "hooks.json",
		Style:    "trae",
		Verified: false,
		Note:     "The CN and international builds differ only in model/account/network/compliance; the IDE's rules directory and hook mechanism are identical, so both share one adapter.",
	},
	{
		ID:       "codex",
		Tier:     Tier2,
		RulesDir: "",
		HookFile: "~/.codex/config.toml",
		HookAbs:  true,
		Style:    "codex",
		Verified: false,
		Note:     "Only reads a single-file AGENTS.md (no directory mode), so it goes through SessionStart injection and never touches your own AGENTS.md.",
	},
	{
		ID:       "opencode",
		Tier:     Tier2,
		RulesDir: "",
		HookFile: "opencode.json",
		Style:    "json",
		Verified: false,
		Note:     "Only reads a single-file AGENTS.md, so it goes through SessionStart injection and never touches your own AGENTS.md.",
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
	p, _ := a.HookFileAbsWithSource(workspace)
	return p
}

// HookFileAbsWithSource 与 HookFileAbs 相同，但额外返回「目录被哪个环境变量覆盖」
// （未覆盖时为空串）。供 doctor 解释路径来源：用户设过环境变量时能一眼看出为什么是这个路径。
func (a Agent) HookFileAbsWithSource(workspace string) (path string, envUsed string) {
	if a.HookAbs {
		if d := a.configDirOverride(); d != "" {
			return filepath.Join(d, a.HookFileBase), a.HookDirEnv
		}
		return ExpandHome(a.HookFile), ""
	}
	return filepath.Join(workspace, a.HookFile), ""
}

// configDirOverride 返回环境变量指定的配置目录；变量未声明、为空或只有空白时返回空串
// （此时调用方回退到 HookFile 的字面量）。
func (a Agent) configDirOverride() string {
	if a.HookDirEnv == "" || a.HookFileBase == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(a.HookDirEnv))
}

// MatchIDs 返回「能唯一指名本 agent 的所有标识」：规范 ID + 全部别名。
//
// 用于钩子条目的识别与清理：安装/刷新/卸载都按它精确匹配「属于自己的那条钩子」，
// 别人的条目一律原样保留（design/features/agent-onboarding.md）。各 agent 现在是
// 独立条目，互不重叠，故某 agent 的钩子不会被误判为另一 agent 的孤儿。
func (a Agent) MatchIDs() []string {
	out := make([]string, 0, 1+len(a.Aliases))
	out = append(out, a.ID)
	out = append(out, a.Aliases...)
	return out
}

// ExpandHome 把 "~" 或 "~/" 开头的路径展开为当前用户的 HOME。
//
// 它是全项目「~ 展开」的单一真源：agents 包用它解析钩子文件路径，config 包用它解析
// 配置里的 path / workspace —— 同一逻辑不许出现第二份副本。
//
// 取不到 HOME（异常环境）时**原样返回**，绝不静默改坏用户写的路径。
// 不支持 "~user" 形式（无需求、无参照）。
func ExpandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return p // 降级：原样返回
	}
	if p == "~" {
		return h
	}
	return filepath.Join(h, p[2:])
}
