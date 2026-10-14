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
	// LegacyRulesDir 是这套交付**曾经**用过的规则目录（相对工作区根；目前只有 dsh 的
	// ".dsh/rules"）：改成注入交付后它已不再被 rulemux 读写，保留这个字段只为在 uninstall
	// 时**报告**用户盘上可能残留的旧文件——残留不强制清除（用户 2026-10-14 定调：新的不许
	// 再写，旧的清不掉就算了），但不能装作它不存在。
	LegacyRulesDir string
	// SkillsDir 是工作区 skills 目录（相对工作区根）；空 = 该 agent 本轮不接 skill 同步。
	// 例：codebuddy 为 ".codebuddy/skills"（bundle getProjectSkillsDir，2026-10-12 代码级坐实，
	// 见 docs/design/external/agent-rules-dirs.md §5.5）。
	SkillsDir string
	// UserRulesDirs 是用户级（全局）规则目录（可含 ~，经 ExpandHome 展开）；空 = 不投用户级 rules。
	// 例：codebuddy 家族 {"~/.codebuddy/rules", "~/.workbuddy/rules"} —— 用户级要覆盖两家必须
	// 双写（CodeBuddy 读 ~/.codebuddy/rules，WorkBuddy 读自己的 ~/.workbuddy/rules，
	// 2026-10-12 bundle 代码级，见 agent-rules-dirs.md §5.5）。
	UserRulesDirs []string
	// UserSkillsDirs 是用户级（全局）skills 目录（可含 ~）；空 = 不投用户级 skills。
	// 例：{"~/.codebuddy/skills", "~/.workbuddy/skills"} —— CodeBuddy 合并读两家、
	// WorkBuddy 只读自己的（resolveUserSkillsDirs，2026-10-12），双写对两家均安全。
	UserSkillsDirs []string
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
		// skills 双域（2026-10-12 bundle 代码级坐实，见 external/agent-rules-dirs.md §5.5）：
		// 工作区 .codebuddy/skills（getProjectSkillsDir，与 WorkBuddy 共享）；
		// 用户级双写 ~/.codebuddy/skills + ~/.workbuddy/skills（CodeBuddy 合并读两家、
		// WorkBuddy 只读自己的 ⇒ 双写才覆盖两家）；rules 用户级同理双目录。
		SkillsDir:      ".codebuddy/skills",
		UserSkillsDirs: []string{"~/.codebuddy/skills", "~/.workbuddy/skills"},
		UserRulesDirs:  []string{"~/.codebuddy/rules", "~/.workbuddy/rules"},
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
		// **用户级**配置目录 ~/.workbuddy 与钩子文件 ~/.workbuddy/settings.json（2026-10-09 实测）。
		// 它不读 CodeBuddy 的 ~/.codebuddy/settings.json，故拆为独立条目；此前当成别名收纳
		// （方案 A）会把钩子写错目录、永不触发。
		//
		// ⚠ 但**工作区级**规则目录与 CodeBuddy 相同，是 .codebuddy/rules（2026-10-09 三位置
		// 对照探针坐实：只有放进 .codebuddy/rules 的探针进入 WorkBuddy 会话上下文，放进
		// .workbuddy/rules 的不被读）。「用户级独立」不等于「工作区级也独立」，别再据此推断。
		// ⇒ 它与 codebuddy 共享规则目录，同步走并集、卸载走收敛（见 SharingRulesDir）。
		ID:       "workbuddy",
		Tier:     Tier1,
		RulesDir: ".codebuddy/rules", // 与 codebuddy 共享；2026-10-09 探针实测坐实，非推断
		// 与 codebuddy 同值：工作区 skills 共享 .codebuddy/skills；用户级双写两家目录
		//（2026-10-12 bundle 代码级，见 external/agent-rules-dirs.md §5.5）。
		SkillsDir:      ".codebuddy/skills",
		UserSkillsDirs: []string{"~/.codebuddy/skills", "~/.workbuddy/skills"},
		UserRulesDirs:  []string{"~/.codebuddy/rules", "~/.workbuddy/rules"},
		HookFile:       "~/.workbuddy/settings.json",
		HookAbs:        true,
		// WORKBUDDY_CONFIG_DIR 仅出现在 safe-delete 日志白名单（见 external/agent-rules-dirs.md §五），
		// 未坐实是配置目录 ⇒ 不采用；本机 env 实测该变量为空，按 ~/.workbuddy 是稳的。
		HookDirEnv:       "",
		HookFileBase:     "",
		Style:            "claude",
		Verified:         true,
		NeedsFrontmatter: true,
		SessionHint:      true,
		HintProtocol:     ProtocolSessionStartAdditionalContext,
		Note:             "Independent app (macOS Electron, /Applications/WorkBuddy.app/). Local inspection 2026-10-09 proved it has its OWN user-level config dir ~/.workbuddy and hook file ~/.workbuddy/settings.json — it does NOT read CodeBuddy's ~/.codebuddy/settings.json, so it is a separate registry entry (previously wrongly folded into codebuddy as an alias, design §三 plan A). BUT its WORKSPACE-level rules dir is .codebuddy/rules, the same as CodeBuddy (2026-10-09 three-position canary: only the probe in .codebuddy/rules reached the session context; .workbuddy/rules was never read) — so the two agents SHARE a rules dir and need union-on-sync / prune-on-uninstall (see SharingRulesDir). WORKBUDDY_CONFIG_DIR is only in a safe-delete log whitelist, not adopted (external/agent-rules-dirs.md §五).",
	},
	{
		// TraeCode（Trae CN）的全局 hook 落点是**用户级** `~/.trae-cn/hooks.json`（不是工作区相对）。
		// 已核实（2026-10-09，取自 hindsight 实装 installer.ts:2092-2127 + 本机 ~/.trae-cn/hooks.json 实测）：
		//   - 用 Claude Code 的 hook 协议，事件挂在顶层 `hooks` 键下，并要求顶层 `version` 字段；
		//   - 全局 hook 能触发（本机已有一条 hindsight 的 SessionStart/UserPromptSubmit/Stop 在运行）；
		//   - 国际版目录为 `~/.trae`（按存在探测、无环境变量覆盖）；本条目先校准 CN 版，国际版的
		//     「二选一探测」留作后续（registry 现不支持候选目录探测，故先写死 CN）。
		// 规则目录 `.trae/rules` 经 2026-10-09 canary 坐实：递归读取；`.md`（带/不带 frontmatter 均可）
		// 会被加载；`.mdc` **不**被当作规则加载（直接读文件才看得到，不在会话上下文）。hook 事件载荷
		// 里带 `cwd` 与 `workspace_roots`，故 `sync --hook` 可直接用载荷定位工作区（见 cmd.flags.hookWorkspace）。
		ID:       "trae",
		Aliases:  []string{"trae-cn", "trae-intl"},
		Tier:     Tier1,
		RulesDir: ".trae/rules", // canary 坐实（2026-10-09）：递归读取；`.md` 加载（frontmatter 可选）；`.mdc` 不加载
		HookFile: "~/.trae-cn/hooks.json",
		HookAbs:  true,
		// Trae 无配置目录环境变量（与 CodeBuddy 的 CODEBUDDY_CONFIG_DIR 不同），按 ~ 展开即可。
		HookDirEnv:   "",
		HookFileBase: "",
		Style:        "trae",
		Verified:     true, // 安装目录 + 规则目录均经 2026-10-09 实测坐实
		Note:         "CN build uses user-level ~/.trae-cn/hooks.json (verified 2026-10-09 from hindsight's live installer + this machine); international build uses ~/.trae (probe needed, not wired yet). Hook protocol is Claude Code's: top-level `hooks` key + required top-level `version` field. Global hooks DO fire (a hindsight SessionStart hook is already live there). Rules dir `.trae/rules` verified by canary 2026-10-09: recursive; `.md` loaded (frontmatter optional); `.mdc` is NOT loaded as a rule. Hook payload carries `cwd`/`workspace_roots`, so `sync --hook` resolves the workspace from the payload (cmd/flags.go hookWorkspace).",
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
	{
		// DeepSeek Harness (dsh) is plugin-first: no hook config file, and it does NOT scan a rules
		// directory on its own — so it takes no rules directory at all and is delivered by
		// INJECTION, exactly like codex / opencode: the repo sub-package `dsh-plugin/` (npm name
		// `rulemux-dsh`) asks the CLI for the rendered rules (`rulemux inject --agent dsh`) on the
		// first user turn and injects that stdout. Nothing is written into the workspace.
		//
		// 2026-10-14 用户拍板：原本的 WRITE half（sync 真实拷贝进 <workspace>/.dsh/rules、插件读回）
		// 作废 —— 那个目录宿主根本不认，只是两边之间的交接物；忘了清理就永久留在用户盘上，
		// 正是「机器上没装 dsh，.dsh/rules 却被反复重写」那个缺陷的舞台。现在没有中间目录，
		// 也就没有东西需要保持同步、没有残渣需要清理。
		// rulemux does NOT install the plugin ⇒ Style "external": init/doctor print the install hint,
		// uninstall leaves the host config alone.
		ID:       "dsh",
		Aliases:  []string{"deepseek", "deepseek-harness"},
		Tier:     Tier2,
		RulesDir: "",
		// 旧交接目录：2026-10-14 前的设计往这里写真实拷贝。现在只用于 uninstall 时报一句
		// 「这里还有旧文件，rulemux 不再写它了」——不删（那只目录不是 rulemux 的）。
		LegacyRulesDir: ".dsh/rules",
		Style:          "external",
		// 2026-10-10 先行开放；同日容器内真机 canary 通过（装载期生成配置、首轮注入一次成功、
		// token 成本与宿主原生读文件字节一致 ⇒ 见 worklog 202610/10-0000-dsh-adapter-9f1e.md）。
		// ⚠ 2026-10-14 交付改为纯注入（不再有 .dsh/rules），该 canary 结论需按新链路重跑。
		Verified: true,
		// 插件每会话首轮现取现注入 ⇒ 无 CodeBuddy 式「差一拍」，不需要变化提示。
		SessionHint:  false,
		HintProtocol: "",
		Note:         "Plugin-first host (no hook binary, and no rules directory at all): delivery is injection. The repo sub-package `dsh-plugin/` (npm name `rulemux-dsh`) is installed through the dsh plugin command (`dsh plugin --profile <p> add github:cq-guojia/rulemux#path:/dsh-plugin`, or npm) and asks the CLI for the workspace's rules (`rulemux inject --agent dsh`) on the first user turn — rulemux does not install it (Style \"external\"), and nothing is written into the workspace. Canary passed 2026-10-10 on a real dsh in a container (load-time config generation, one successful first-turn injection, token cost byte-identical to the host reading files natively) — see docs/worklog/202610/10-0000-dsh-adapter-9f1e.md; that was the directory hand-off, so it needs re-running for the injection-only delivery of 2026-10-14. Uninstalling rulemux cannot remove the host-side plugin, and it cannot stop it either: the plugin keeps calling `rulemux inject --agent dsh` in every session, so `uninstall --agent dsh` says exactly that (disable the plugin row in the profile's cordis.patch.yml, or remove it with dsh's own plugin command).",
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

// SkillsDirAbs 返回该 agent 工作区 skills 目录的绝对路径；未声明（空）返回空串。
func (a Agent) SkillsDirAbs(workspace string) string {
	if a.SkillsDir == "" {
		return ""
	}
	if filepath.IsAbs(a.SkillsDir) {
		return a.SkillsDir
	}
	return filepath.Join(workspace, a.SkillsDir)
}

// UserSkillsDirsAbs 返回用户级 skills 目录的绝对路径列表（~ 经 ExpandHome 展开）。
func (a Agent) UserSkillsDirsAbs() []string {
	return expandHomesAll(a.UserSkillsDirs)
}

// UserRulesDirsAbs 返回用户级 rules 目录的绝对路径列表（~ 经 ExpandHome 展开）。
func (a Agent) UserRulesDirsAbs() []string {
	return expandHomesAll(a.UserRulesDirs)
}

// expandHomesAll 把一组路径里的 "~" / "~/" 展开为 HOME（复用 ExpandHome 单一真源）。
func expandHomesAll(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, p := range in {
		out[i] = ExpandHome(p)
	}
	return out
}

// equalStrings 报告两个字符串切片是否等长且逐项相等。
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SharingSkillsDir 返回「与 a 落进同一个工作区 skills 目录」的 agent 列表（含 a 自己）。
// 与 SharingRulesDir 同规律：目录共享 ⇒ 同步必须按并集，否则删残留会互删对方文件。
func SharingSkillsDir(a Agent) []Agent {
	if a.SkillsDir == "" {
		return []Agent{a}
	}
	out := []Agent{a}
	for _, b := range All() {
		if b.ID == a.ID || b.SkillsDir == "" {
			continue
		}
		if b.SkillsDir == a.SkillsDir {
			out = append(out, b)
		}
	}
	return out
}

// SharingUserDirs 返回「与 a 用户级目录完全相同」的 agent 列表（含 a 自己）。
// 用户级目录（rules + skills 两组）都相同的 agent 互为共享者 ⇒ 用户级同步同样按并集，
// 否则一家的 hook 会把另一家投的文件当残留删掉。未声明用户级目录的 agent 只会匹配到自己。
func SharingUserDirs(a Agent) []Agent {
	if len(a.UserSkillsDirs) == 0 && len(a.UserRulesDirs) == 0 {
		return []Agent{a}
	}
	out := []Agent{a}
	for _, b := range All() {
		if b.ID == a.ID {
			continue
		}
		if equalStrings(a.UserSkillsDirsAbs(), b.UserSkillsDirsAbs()) &&
			equalStrings(a.UserRulesDirsAbs(), b.UserRulesDirsAbs()) {
			out = append(out, b)
		}
	}
	return out
}

// SharingRulesDir 返回「与 a 落进同一个工作区规则目录」的 agent 列表（含 a 自己）。
//
// 目前只有 codebuddy 与 workbuddy 共享 `.codebuddy/rules`（2026-10-09 探针实测坐实）；
// claude / trae 各用各的目录，codex / opencode 不落盘（RulesDir 为空）⇒ 它们只会匹配到自己，
// 调用方无须为它们做特殊处理。
//
// 共享目录意味着：任一方同步时的「应有文件集合」必须是**所有共享者的并集**，否则
// engine 的删残留（整目录扫 __rulemux__ 前缀、不在本次计划内就删）会把另一方的文件
// 当残留删掉 —— 表现为两个 agent 的钩子轮流触发时文件来回消失。
//
// 只收 Tier-1：Tier-2 的 RulesDir 为空，天然不匹配任何非空目录。
func SharingRulesDir(a Agent) []Agent {
	if a.RulesDir == "" {
		return []Agent{a}
	}
	out := []Agent{a}
	for _, b := range All() {
		if b.ID == a.ID || b.Tier != Tier1 {
			continue
		}
		if b.RulesDir == a.RulesDir {
			out = append(out, b)
		}
	}
	return out
}

// HookFileAbs 返回该 agent 钩子配置文件的绝对路径。
func (a Agent) HookFileAbs(workspace string) string {
	p, _ := a.HookFileAbsWithSource(workspace)
	return p
}

// HookFileAbsWithSource 与 HookFileAbs 相同，但额外返回「目录被哪个环境变量覆盖」
// （未覆盖时为空串）。供 doctor 解释路径来源：用户设过环境变量时能一眼看出为什么是这个路径。
func (a Agent) HookFileAbsWithSource(workspace string) (path string, envUsed string) {
	// 无宿主配置文件的 agent（Style "external"：插件由宿主自己的命令安装）⇒ 空路径，
	// 让 Inspect/doctor 一眼看出「没有可装的钩子」，而不是误拼成 workspace 本身。
	if a.HookFile == "" {
		return "", ""
	}
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
