package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/engine"
	"github.com/cq-guojia/rulemux/internal/hooks"
	"github.com/cq-guojia/rulemux/internal/state"
	"github.com/cq-guojia/rulemux/internal/ui"
)

// Sync 把配置所列源文件真实拷贝进各 agent 的原生规则目录（Tier-1）。
//
// 典型调用：由各 agent 的 SessionStart 钩子执行 `rulemux sync --hook --agent <id>`。
// 未指定 --agent 时处理全部「已验证」的 Tier-1 agent —— 规则是公共的，默认全给。
//
// 关键语义（详见 docs/design/features/sync-all-and-change-notice.md §二/§六/§八）：
//   - 只有「hook 调用」且「被 --agent 点名」的那一个 agent 才允许创建它的规则目录；
//     其它任何路径只写**已存在**的目录。
//   - 人类可读输出一律走 stderr；stdout 只在 hook 路径且**确有变化**时输出一条协议 JSON，
//     无变化则一个字节都不输出（hook 绝不注入未经用户同意的内容）。
func Sync(args []string) int {
	f := ParseFlags(args)
	cfgPath := f.Get("config", config.DefaultPath())
	wsFlag := f.Get("workspace", "")
	hook := f.Has("hook")
	requested := f.Get("agent", "")

	// hook 由各 agent 的 SessionStart 钩子调用，必然点名 agent；缺了就是配置错误。
	if hook && requested == "" {
		fmt.Fprintln(os.Stderr, "rulemux sync: --hook requires --agent <id>")
		return 2
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: failed to read config: %v\n  Hint: run rulemux init to generate a sample config\n", err)
		return 1
	}
	// Feature-switch first: confirm the requested agent is supported before validating
	// config content (unverified agents are rejected outright).
	if requested != "" {
		if a, ok := agents.Get(requested); !ok || !a.Verified {
			if ok && !a.Verified {
				fmt.Fprintf(os.Stderr, "rulemux: agent %q is not supported yet: only verified agents are available: %s\n", requested, agents.SupportedSummary())
			} else {
				fmt.Fprintf(os.Stderr, "rulemux: unknown agent %q\n", requested)
			}
			return 1
		}
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "rulemux:", err)
		return 1
	}

	// `sync --all`：手动全量对账 —— 只走账本（不读 cwd、不用配置通配符），
	// 因此与 --workspace / --hook / --agent 都互斥。
	if f.Has("all") {
		if wsFlag != "" {
			fmt.Fprintln(os.Stderr, "rulemux sync: --all and --workspace are mutually exclusive")
			return 2
		}
		if hook || requested != "" {
			fmt.Fprintln(os.Stderr, "rulemux sync: --all cannot be combined with --hook or --agent")
			return 2
		}
		return syncAll(cfg)
	}

	ws, err := workspace(wsFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}
	// hook 调用且未显式给 --workspace 时，优先用载荷里的 cwd / workspace_roots
	// （Trae/Claude Code 的 hook 事件会透传，比 os.Getwd() 更可靠；缺失则回退）。
	if wsFlag == "" && hook {
		if w, ok := hookWorkspace(); ok {
			ws, _ = workspace(w)
		}
	}

	// 守卫（安全事故防线）：目标工作区必须被配置里某条 source 声明。
	// 否则「空 want ⇒ 删残留」会清空一个我们无权管辖的目录。
	// 见 docs/design/features/sync-all-and-change-notice.md §二「清空的正确姿势」/ §八。
	if !cfg.DeclaresWorkspace(ws) {
		fmt.Fprintf(os.Stderr, "rulemux: %s is not a declared workspace; nothing to do\n", ws)
		return 0
	}

	targets := targetAgents(requested)
	if len(targets) == 0 {
		if requested != "" {
			if a, ok := agents.Get(requested); ok && !a.Verified {
				fmt.Fprintf(os.Stderr, "rulemux: agent %q is not supported yet: only verified agents are available: %s\n", requested, agents.SupportedSummary())
			} else {
				fmt.Fprintf(os.Stderr, "rulemux: unknown agent %q\n", requested)
			}
		} else {
			fmt.Fprintln(os.Stderr, "rulemux: no verified agent to process")
		}
		return 1
	}

	// 自愈（不依赖 npm 的安装脚本）：顺手把「我们自己装过的那条钩子」升级到当前格式。
	// 任何安装方式（npm / tarball / 容器入口 / go install）都会在下一次 sync 时自动跟上；
	// 旧格式钩子（缺 --hook）正是在这里被修好的 —— 因为它调起我们时也不带 --hook，
	// 所以这条自愈必须在「非 hook 路径」同样生效。
	healHooks(ws, targets)

	// 只有 hook 点名的那一个 agent 才允许创建自己的规则目录（设计 §二 原则 1）。
	requestedID := ""
	if requested != "" {
		if ra, ok := agents.Get(requested); ok {
			requestedID = ra.ID
		}
	}
	createAgent := ""
	if hook && requestedID != "" {
		createAgent = requestedID
	}

	run := newSyncRun(cfg, ws, createAgent)
	run.wsList = append(run.wsList, ws)
	for _, a := range targets {
		if a.Tier == agents.Tier2 {
			// Tier-2 没有规则目录可丢文件，由 inject 子命令负责注入
			continue
		}
		run.agent(a)
	}

	// 顺手同步（仅 hook 路径）：把这个工作区历史上用过的**其它** agent 也一并同步。
	// 只读账本 + 只写已存在目录（原则 2），因此不会新建任何目录、不会碰到没配置的地方。
	if hook {
		run.cascade()
	}

	// 记入账本（收窄触发条件）：只有确实落到一个已存在目录上（没被 SkippedNoDir 跳过）
	// 才算「用过」。记的是「工作区 × agent」；一个 agent 本轮同步多个目录只记一次。
	// 钩子是全局的，但规则文件落在各工作区本地；卸载时要靠这份账本逐一回访清理，
	// 否则钩子一去、残留将永无机会被自动删除。
	for _, e := range run.landed {
		if err := state.Record(e.ws, e.id); err != nil {
			fmt.Fprintf(os.Stderr, "rulemux: warning: failed to write the workspace ledger: %v\n", err)
		}
	}

	// hook 路径下、且确有变化时，给模型一句话（它应转告用户重开会话）。
	// 无变化 ⇒ stdout 保持为空（一个字节都不输出）。
	//
	// per-agent 门禁：只有被点名的那个 agent 明确「需要提示 + 协议已知」时才输出
	// （见设计 §六「per-agent」）——有的 agent 会话内会重读规则，根本不该提示。
	// 注意 changed 只统计 rules（含用户级 rules）；skills 变化不计入——热重载行为
	// 待 canary（docs/design/external/skill-ecosystem.md §六-4），不注入未验证的提示。
	if hook && run.changed {
		if a, ok := agents.Get(requested); ok && a.SessionHint && a.HintProtocol != "" {
			emitChangeNotice(os.Stdout, a.HintProtocol)
		}
	}
	run.printSyncReport(os.Stderr, "rulemux sync")
	if run.failed {
		return 1
	}
	return 0
}

// syncRun 是一次 sync 运行的共享状态：跨 agent 的目录去重 + 变化/落盘汇总。
//
// 每个 agent 同步四类目录（工作区 rules / 工作区 skills / 用户级 rules / 用户级 skills），
// 与规则同步完全同构：共享目录按并集（否则删残留互删对方文件）、同一目录只跑一次。
type syncRun struct {
	cfg *config.Config
	ws  string
	// createAgent 是「允许创建目录」的 agent（只有 hook 路径被 --agent 点名的那一个）；
	// 空 = 谁都不建（手动 sync / cascade / sync --all 一律只写已存在目录）。
	createAgent string
	doneDirs    map[string]bool // 工作区目录（rules + skills）去重
	doneUser    map[string]bool // 用户级目录去重（全局，跨工作区只跑一次）
	changed     bool            // rules 变化（含用户级 rules；skills 不计入）
	failed      bool            // 任一路径出错（抬退出码）
	landed      []landedEntry   // 落过盘的（工作区, agent），供账本记账
	lines       []*syncLine     // 报告行（按出现顺序）
	lineIdx     map[string]*syncLine
	wsList      []string // 本轮处理过的工作区（按顺序，含无落盘的）
}

// landedEntry 是一条「在某个工作区落过盘」的记录（账本 key）。
type landedEntry struct {
	ws string
	id string
}

func newSyncRun(cfg *config.Config, ws, createAgent string) *syncRun {
	return &syncRun{
		cfg:         cfg,
		ws:          ws,
		createAgent: createAgent,
		doneDirs:    map[string]bool{},
		doneUser:    map[string]bool{},
		lineIdx:     map[string]*syncLine{},
	}
}

// agent 同步一个 agent 的全部目录：工作区 rules + 工作区 skills + 用户级 rules ×N + 用户级 skills ×N。
func (r *syncRun) agent(a agents.Agent) {
	// 只有被 hook 点名的那一个 agent 才允许创建目录（设计 §二 原则 1）。
	create := r.createAgent != "" && a.ID == r.createAgent

	// 1. 工作区 rules：共享目录（codebuddy / workbuddy 同落 .codebuddy/rules）按并集同步，
	// 否则 engine 的删残留会把另一方的文件当残留删掉 ⇒ 两个钩子轮流触发时文件来回消失。
	if a.Tier == agents.Tier1 && a.RulesDir != "" {
		dir := a.RulesDirAbs(r.ws)
		if !r.doneDirs[dir] {
			r.doneDirs[dir] = true
			r.syncFiles(a, dir, r.cfg.SourcesForAny(unionAgentIDs(a, r.ws), r.ws), a.NeedsFrontmatter, create, false)
		}
	}

	// 2. 工作区 skills：与 rules 同规律（共享 .codebuddy/skills，2026-10-12 代码级，见 registry 注释）。
	if a.SkillsDir != "" {
		dir := a.SkillsDirAbs(r.ws)
		if !r.doneDirs[dir] {
			r.doneDirs[dir] = true
			r.syncSkillDirs(a, dir, r.cfg.SkillSourcesForAny(unionSkillIDs(a, r.ws), r.ws), create, false)
		}
	}

	// 3-4. 用户级（全局）rules / skills：目录与工作区无关 ⇒ 忽略 workspace 匹配；
	// codebuddy 家族双目录（~/.codebuddy + ~/.workbuddy）才覆盖两家（CodeBuddy 合并读两家、
	// WorkBuddy 只读自己的，见 docs/design/external/agent-rules-dirs.md §5.5）。
	userIDs := unionUserIDs(a, r.ws)
	for _, dir := range a.UserRulesDirsAbs() {
		if r.doneUser[dir] {
			continue
		}
		r.doneUser[dir] = true
		r.syncFiles(a, dir, r.cfg.UserSourcesForAny(userIDs), a.NeedsFrontmatter, create, true)
	}
	for _, dir := range a.UserSkillsDirsAbs() {
		if r.doneUser[dir] {
			continue
		}
		r.doneUser[dir] = true
		r.syncSkillDirs(a, dir, r.cfg.UserSkillSourcesForAny(userIDs), create, true)
	}
}

// syncFiles 同步一个规则目录（工作区或用户级）。结果按 (agent × 作用域) 累进
// syncLine 供报告汇总，不再逐目录打印。
func (r *syncRun) syncFiles(a agents.Agent, dir string, srcs []config.Source, frontmatter, create, userLevel bool) {
	res, err := engine.Sync(dir, srcs, frontmatter, create)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: failed to sync %s: %v\n", a.ID, err)
		r.failed = true
		return
	}
	l := r.line(a, userLevel)
	if res.SkippedNoDir {
		return // 目录不存在且不允许创建：不建、不读、不删，报告里也不出行
	}
	l.dirs = append(l.dirs, dir)
	l.rules += len(res.Copied) + len(res.Updated) + len(res.Skipped)
	l.rulesAdd += len(res.Copied)
	l.rulesDel += len(res.Deleted)
	l.rulesUpd += len(res.Updated)
	l.missing = append(l.missing, res.Missing...)
	l.errs = append(l.errs, res.Errors...)
	if res.HasChanges() {
		r.changed = true
	}
	r.landed = append(r.landed, landedEntry{ws: r.ws, id: a.ID})
}

// syncSkillDirs 同步一个 skills 目录（工作区或用户级）。与 syncFiles 同构：
// 一个 skill 树算一个单位（与旧输出 "1 file(s)" 同口径）。
func (r *syncRun) syncSkillDirs(a agents.Agent, dir string, srcs []config.SkillSource, create, userLevel bool) {
	res, err := engine.SyncSkills(dir, srcs, create)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: failed to sync %s: %v\n", a.ID, err)
		r.failed = true
		return
	}
	l := r.line(a, userLevel)
	if res.SkippedNoDir {
		return
	}
	l.dirs = append(l.dirs, dir)
	l.skills += len(res.Copied) + len(res.Updated) + len(res.Skipped)
	l.skillsAdd += len(res.Copied)
	l.skillsDel += len(res.Deleted)
	l.skillsUpd += len(res.Updated)
	l.missing = append(l.missing, res.Missing...)
	l.errs = append(l.errs, res.Errors...)
	// shortcut: skills 变化不计入 changed（不触发「新开会话」提示）——skills 是否热重载
	// 待 canary（docs/design/external/skill-ecosystem.md §六-4）；坐实后再按 per-agent 判据补。
	r.landed = append(r.landed, landedEntry{ws: r.ws, id: a.ID})
}

// cascade 顺手同步该工作区历史上用过的**其它** Tier-1 agent。
//
// 硬规则（设计 §七）：
//   - 候选来自**账本**（该工作区用过哪些 agent），不是扫盘、也不是配置通配符；
//   - 只处理已核实（Verified）的 Tier-1 agent；查不到 / 未核实 ⇒ 跳过（账本条目保留）；
//   - **任何域都没有源** ⇒ 什么也不做（不去清一个配置已不再覆盖的 agent 的目录）；
//   - **只一层**，不顺带再顺带；失败只记 warning，不影响主流程退出码。
func (r *syncRun) cascade() {
	l := state.Load()
	done := map[string]bool{}
	for _, e := range r.landed {
		if e.ws == r.ws {
			done[e.id] = true
		}
	}
	for _, a := range agentsFromLedger(l.AgentsFor(r.ws)) {
		if done[a.ID] || a.Tier != agents.Tier1 {
			continue
		}
		if !r.cfg.HasAnyFor(unionAgentIDs(a, r.ws), r.ws) {
			continue // 任何域都没有源 ⇒ 不碰
		}
		r.agent(a)
	}
}

// healHooks 把 targets 里每个 agent「已存在的自家钩子」升级到当前格式（幂等）。
//
// 这是「升级即生效」的自愈路径，替代原先依赖 npm postinstall 的做法：npm 11 起安装脚本
// 默认需要白名单放行（`allow-scripts`），靠它迟早会静默失效 —— 不押在别人的策略上。
//
// 硬规则（与 init --refresh 同一口径）：只重写已存在的自家条目；没装过不创建、别人的条目
// 与其它键不碰、已是最新不写盘。诊断一律走 stderr，绝不污染 stdout 的协议输出。
func healHooks(ws string, targets []agents.Agent) {
	for _, a := range targets {
		path := a.HookFileAbs(ws)
		installed, cmd, err := hooks.Inspect(path, a)
		if err != nil || !installed || cmd == hooks.TargetCommand(a) {
			continue // 读不了 / 没装过 / 已是最新 ⇒ 什么都不做
		}
		if _, err := hooks.Refresh(path, a); err != nil {
			fmt.Fprintf(os.Stderr, "rulemux: warning: could not update the SessionStart hook in %s: %v\n", path, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "rulemux: updated the SessionStart hook to the current format (%s)\n", cmd)
	}
}

// peerIDs 取一组 agent 的规范 ID（供 config 的 *ForAny 查询用）。
func peerIDs(ps []agents.Agent) []string {
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	return ids
}

// hookedPeersIn 返回 group 中还装着 rulemux 钩子的 agent；一个都没装 ⇒ 退回整组
// （手动 sync 的兜底：没钩子可判时按整组并集，保证手动跑仍有东西落）。
//
// 「还装着」是判断某个 agent 是否仍在使用的唯一依据：卸载 = 摘钩子，钩子一没它就自然
// 退出并集，因此**不需要任何额外的状态文件**。
func hookedPeersIn(group []agents.Agent, ws string) []agents.Agent {
	var hooked []agents.Agent
	for _, b := range group {
		if hookInstalled(b, ws) {
			hooked = append(hooked, b)
		}
	}
	if len(hooked) == 0 {
		return group
	}
	return hooked
}

// syncAll 实现 `rulemux sync --all`：只遍历账本记录过的工作区，把它们各自对齐到当前配置。
//
// 语义（设计 §五 / §八）：
//   - **不读 cwd**、**不使用配置里的 workspace 通配符**去发现工作区；
//   - 每个工作区按账本记录的 agents 逐个同步（哨兵 `*` ⇒ 全部已支持 agent）；
//   - **只写已存在的目录**（create=false，原则 2），目录不存在就跳过、绝不新建；
//   - 顺手 GC：路径**确实不存在**（ENOENT）的账本条目会被清掉；其它错误（权限 / 断连）
//     只跳过、保留条目（可能是临时不可用的外挂盘）。
//
// 用户级目录是全局的：整个 --all 只同步一次（syncRun.doneUser 跨工作区去重）。
func syncAll(cfg *config.Config) int {
	l := state.Load()
	workspaces := l.AllWorkspaces()
	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stderr, "rulemux: no workspace recorded in the ledger yet; nothing to do")
		return 0
	}

	exit := 0
	var gone []string
	run := newSyncRun(cfg, "", "") // createAgent 空 ⇒ 一律只写已存在目录
	for _, ws := range workspaces {
		if _, err := os.Stat(ws); err != nil {
			if os.IsNotExist(err) {
				gone = append(gone, ws) // 确实不存在 ⇒ 待 GC
			} else {
				fmt.Fprintf(os.Stderr, "rulemux: skip %s: %v\n", ws, err)
			}
			continue
		}
		run.ws = ws
		run.wsList = append(run.wsList, ws)
		for _, a := range agentsFromLedger(l.AgentsFor(ws)) {
			run.agent(a)
		}
	}

	// 记账：只记确实落过盘的（工作区 × agent）；一个 agent 同步多个目录只记一次。
	for _, e := range run.landed {
		if err := state.Record(e.ws, e.id); err != nil {
			fmt.Fprintf(os.Stderr, "rulemux: warning: failed to write the workspace ledger: %v\n", err)
		}
	}

	// 账本 GC：只清「确实不存在」的条目（ENOENT）。其它错误一律保留。
	if len(gone) > 0 {
		if err := state.Prune(gone); err != nil {
			fmt.Fprintf(os.Stderr, "rulemux: warning: failed to prune the ledger: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "rulemux: pruned %d ledger entr(ies) whose directory no longer exists\n", len(gone))
		}
	}
	run.printSyncReport(os.Stderr, "rulemux sync --all")
	if run.failed {
		exit = 1
	}
	return exit
}

// agentsFromLedger 把账本里记录的一组 agent 解释为具体的 Tier-1 适配器：
//   - 空 / 含哨兵 state.AnyAgent ⇒ 全部已支持 agent；
//   - 其它值按注册表解析（认别名）；查不到或未验证的**跳过**（条目保留，等将来就绪）。
func agentsFromLedger(ids []string) []agents.Agent {
	any := len(ids) == 0
	for _, id := range ids {
		if id == state.AnyAgent {
			any = true
			break
		}
	}
	if any {
		return targetAgents("")
	}
	var out []agents.Agent
	seen := map[string]bool{}
	for _, id := range ids {
		a, ok := agents.Get(id)
		if !ok || !a.Verified || seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		out = append(out, a)
	}
	return out
}

// changeNotice 是注入给模型的**一段指令**（不含规则正文）：要求它立刻以警告样式告知用户重开会话。
//
// 2026-10-09 用户拍板的三条要求：
//  1. 提示方式 —— 警告样式，让用户明显看到，而不是夹在回答里的一句普通话；
//  2. 提示时机 —— 放在这一步的**所有输出之前**（先警告、后办事，最稳）；
//  3. 执行顺序 —— 先把这条提示输出完，再去做用户原本要求的事。
//
// 语言：由模型自行判断「它此刻与用户沟通所用的语言」，用那种语言重新表述；判断错了也无碍，
// 因此不做中英双语、不加配置项、不读环境变量。
//
// 背景（2026-10-09 实测）：宿主确实会把这段内容注入模型上下文，但若只写"请转告用户"，
// 模型在忙于回答用户问题时经常不转述 ⇒ 用户看不到任何提示。故改为「先输出警告」的硬指令。
const changeNotice = "rulemux: the rule files that rulemux injects into this workspace were just " +
	"updated (added / updated / removed). The copies this session already loaded are the previous " +
	"version.\n" +
	"\n" +
	"BEFORE you do anything else — before any other text, tool call, or answer — start your reply with a " +
	"clearly marked WARNING that tells the user, in the language you are currently using with them, " +
	"something equivalent to:\n" +
	"  \"⚠️ The rule files rulemux injected into this workspace have been updated. This session still " +
	"holds the previous version — start a NEW session for the update to take effect.\"\n" +
	"Always name what was updated (the rule files rulemux injected into this workspace): a bare \"the " +
	"rules changed\" leaves the user guessing which rules. Express it in the user's language — do NOT " +
	"copy this English wording verbatim. Then continue with the user's request."

// hookOutput / hookPayload 是宿主认的注入协议外壳。
type hookOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

type hookPayload struct {
	HookSpecificOutput hookOutput `json:"hookSpecificOutput"`
}

// emitChangeNotice 按宿主协议把「变化提示」写到 w（hook 路径下传 os.Stdout）。
//
// 协议：{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"…"}}
// 已由本机 Hindsight 的 SessionStart 钩子实证（见设计 §六）。
// 不认识的协议**宁可不提示**，也不向 stdout 乱注入。
func emitChangeNotice(w io.Writer, protocol string) {
	if protocol != agents.ProtocolSessionStartAdditionalContext {
		return
	}
	b, err := json.Marshal(hookPayload{HookSpecificOutput: hookOutput{
		HookEventName:     "SessionStart",
		AdditionalContext: changeNotice,
	}})
	if err != nil {
		return
	}
	fmt.Fprintln(w, string(b))
}

// hookedPeers 返回「与 a 共享同一规则目录、且还装着 rulemux 钩子」的 agent（含 a 自己）。
//
// 「还装着」是判断某个 agent 是否仍在使用的唯一依据：卸载 = 摘钩子，钩子一没它就自然
// 退出并集，因此**不需要任何额外的状态文件**。
//
// 若共享组里一个都没装钩子（纯手动 sync 的场景）⇒ 退回该共享组全部 agent，
// 保证手动跑 `rulemux sync` 仍有东西落（见 hookedPeersIn）。
//
// ⚠ 卸载侧调用时必须在 `hooks.Uninstall` **之前**取好结果：钩子一摘就判不出谁还在用。
func hookedPeers(a agents.Agent, ws string) []agents.Agent {
	return hookedPeersIn(agents.SharingRulesDir(a), ws)
}

// hookInstalled 报告该 agent 是否还装着 rulemux 的 SessionStart 钩子。
//
// 读不了、解析失败、格式未核实（codex 的 TOML）一律按「未装」降级 —— 绝不因为读不了
// 配置就中断同步，更不能据此误删别人的文件。
func hookInstalled(a agents.Agent, ws string) bool {
	installed, _, err := hooks.Inspect(a.HookFileAbs(ws), a)
	return err == nil && installed
}

// unionAgentIDs 返回参与**工作区 rules** 并集的 agent 规范 ID（供 config.SourcesForAny 使用）。
func unionAgentIDs(a agents.Agent, ws string) []string {
	return peerIDs(hookedPeers(a, ws))
}

// unionSkillIDs 返回参与**工作区 skills** 并集的 agent 规范 ID（共享 skills 目录，同 rules 规律）。
func unionSkillIDs(a agents.Agent, ws string) []string {
	return peerIDs(hookedPeersIn(agents.SharingSkillsDir(a), ws))
}

// unionUserIDs 返回参与**用户级**（rules + skills）并集的 agent 规范 ID。
// 用户级目录两家共享（codebuddy 家族双目录）⇒ 同样必须按并集，否则一家的 hook 会把
// 另一家投的文件当残留删掉。
func unionUserIDs(a agents.Agent, ws string) []string {
	return peerIDs(hookedPeersIn(agents.SharingUserDirs(a), ws))
}

func targetAgents(id string) []agents.Agent {
	if id != "" {
		a, ok := agents.Get(id)
		if !ok || !a.Verified {
			return nil
		}
		return []agents.Agent{a}
	}
	var out []agents.Agent
	for _, a := range agents.Supported() {
		if a.Tier == agents.Tier1 {
			out = append(out, a)
		}
	}
	return out
}

// syncLine 是报告里的一行：某个作用域（工作区或用户级）下某个 agent 的同步结果。
// 计数全部取自 engine.SyncResult 各切片的长度，零额外 I/O。
type syncLine struct {
	ws        string // 工作区路径（userLevel 时无意义）
	agentID   string
	userLevel bool
	dirs      []string // 该作用域下实际同步过的目录（global 块显示用）

	rules, skills                   int // 同步后仍在盘上的数量
	rulesAdd, rulesDel, rulesUpd    int
	skillsAdd, skillsDel, skillsUpd int

	missing []string // 缺失的源路径
	errs    []string // 单文件写/删失败
}

// empty 报告这一行是否什么都没同步（目录全被跳过）——空行不进报告。
func (l *syncLine) empty() bool { return len(l.dirs) == 0 }

func (l *syncLine) changed() bool {
	return l.rulesAdd+l.rulesDel+l.rulesUpd+l.skillsAdd+l.skillsDel+l.skillsUpd > 0
}

// line 取（或建）某个 agent 在某个作用域下的报告行。userLevel 行与工作区无关，
// 整个进程只此一份（global 块）。
func (r *syncRun) line(a agents.Agent, userLevel bool) *syncLine {
	key := a.ID
	if !userLevel {
		key += "\x00" + r.ws
	}
	if l, ok := r.lineIdx[key]; ok {
		return l
	}
	l := &syncLine{ws: r.ws, agentID: a.ID, userLevel: userLevel}
	r.lineIdx[key] = l
	r.lines = append(r.lines, l)
	return l
}

// printSyncReport 打印人类可读同步报告（stderr）。版式 2026-10-13 用户拍版：
//   - global（用户级）块在前，各工作区块随后；每块 = 标题行（glyph + 路径）+ 每 agent 一行
//   - agent 行只报数：rules N · skills M；无增减即 ✔ 原封不动，不出任何明细
//   - 有变化的块，标题行尾挂当轮增量（rules −6 · skills −1，只列非零；各块按
//     最长标题对齐成一列）
//   - 结尾总计：工作区/agent 数 + rules 与 skills 各自的 +加 −删 ~改
//   - 未初始化 agent 只在总计出现一次；缺失源 / 写失败进警告区（真信号不省略）
//
// 颜色由 internal/ui 按 TTY 自适应；非 TTY 下 glyph 独立承载语义。
func (r *syncRun) printSyncReport(w io.Writer, title string) {
	fmt.Fprintf(w, "%s\n\n", ui.Bold(title))

	var glob, wsLines []*syncLine
	for _, l := range r.lines {
		if l.empty() {
			continue
		}
		if l.userLevel {
			glob = append(glob, l)
		} else {
			wsLines = append(wsLines, l)
		}
	}

	// 先汇总所有块，再统一渲染——增量后缀按最长标题对齐成一列。
	type block struct {
		header string
		lines  []*syncLine
	}
	var blocks []block
	if len(glob) > 0 {
		blocks = append(blocks, block{header: "~ user-level (every workspace)", lines: glob})
	}
	for _, ws := range r.wsList {
		var lines []*syncLine
		for _, l := range wsLines {
			if l.ws == ws {
				lines = append(lines, l)
			}
		}
		if len(lines) == 0 {
			// 没有任何 agent 落盘的工作区：一行暗色说明，不参与对齐
			fmt.Fprintf(w, "%s %s\n", ui.Dim(ui.GlyphSkip), ws)
			fmt.Fprintf(w, "  %s\n", ui.Dim("not initialized — no rule/skill dirs yet"))
			continue
		}
		blocks = append(blocks, block{header: ws, lines: lines})
	}

	maxW := 0
	for _, b := range blocks {
		if n := utf8.RuneCountInString(b.header) + 2; n > maxW { // +2 = glyph + 空格
			maxW = n
		}
	}
	for _, b := range blocks {
		r.printBlock(w, b.header, b.lines, maxW)
	}

	// 警告区：缺失源 / 写失败（省略它们等于隐瞒故障）
	warns := 0
	for _, l := range r.lines {
		for _, m := range l.missing {
			fmt.Fprintf(w, "%s %s: source file missing: %s\n", ui.Yellow(ui.GlyphWarn), l.agentID, m)
			warns++
		}
		for _, e := range l.errs {
			fmt.Fprintf(w, "%s %s: %s\n", ui.Red(ui.GlyphFail), l.agentID, e)
			warns++
		}
	}
	if warns > 0 {
		fmt.Fprintln(w)
	}

	r.printTotals(w)
}

// printBlock 打一个作用域块：标题行（glyph + 名称，按 maxW 对齐）+ 增量后缀
// + 每 agent 一行。
func (r *syncRun) printBlock(w io.Writer, header string, lines []*syncLine, maxW int) {
	changed := false
	for _, l := range lines {
		if l.changed() {
			changed = true
			break
		}
	}
	head := ui.GlyphOK + " " + header
	if changed {
		head = ui.Yellow(ui.GlyphChanged + " " + header)
	} else {
		head = ui.Green(head)
	}
	if suffix := blockDelta(lines); suffix != "" {
		// 按最长标题补齐，增量对齐成一列（display 宽度按 rune 计）
		if pad := maxW - utf8.RuneCountInString(header) - 2; pad > 0 {
			head += strings.Repeat(" ", pad)
		}
		head += "  " + suffix
	}
	fmt.Fprintln(w, head)
	for _, l := range lines {
		line := fmt.Sprintf("  %-10s rules %s · skills %s", l.agentID, numOrDash(l.rules), numOrDash(l.skills))
		if l.userLevel && len(l.dirs) > 0 {
			// global 块的目录提示（暗色，~ 压缩，按 agent 根目录去重——
			// ~/.codebuddy/rules 与 ~/.codebuddy/skills 合成 ~/.codebuddy）
			line += "    " + ui.Dim(ui.TruncateJoin(agentRoots(l.dirs), 4))
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w)
}

// printTotals 打结尾总计：工作区/agent 数、rules 与 skills 的现有数与增减改、
// 未初始化 agent 名单（只在这里出现一次）。
func (r *syncRun) printTotals(w io.Writer) {
	var rules, skills, ra, rd, ru, sa, sd, su int
	seen := map[string]bool{}
	for _, l := range r.lines {
		if l.empty() {
			continue
		}
		seen[l.agentID] = true
		rules += l.rules
		skills += l.skills
		ra += l.rulesAdd
		rd += l.rulesDel
		ru += l.rulesUpd
		sa += l.skillsAdd
		sd += l.skillsDel
		su += l.skillsUpd
	}
	fmt.Fprintln(w, ui.Bold(fmt.Sprintf("%s · %s initialized",
		ui.Plural(len(r.wsList), "workspace"), ui.Plural(len(seen), "agent"))))
	fmt.Fprintf(w, "rules %s · +%d added · −%d removed · %d updated\n", ui.Plural(rules, "file"), ra, rd, ru)
	fmt.Fprintf(w, "skills %s · +%d added · −%d removed · %d updated\n", ui.Plural(skills, "file"), sa, sd, su)
	// 未初始化 = 本轮没有任何落盘的 Tier-1 agent（点名但不打叉）
	var uninit []string
	for _, a := range targetAgents("") {
		if !seen[a.ID] {
			uninit = append(uninit, a.ID)
		}
	}
	if len(uninit) > 0 {
		fmt.Fprintf(w, "%s not initialized: %s — rulemux init --agent <id>\n",
			ui.Dim(ui.GlyphSkip), strings.Join(uninit, ", "))
	}
}

// blockDelta 汇总一个块内当轮的非零增量，如 "rules −6 · skills −1" / "rules ~1"。
func blockDelta(lines []*syncLine) string {
	var ra, rd, ru, sa, sd, su int
	for _, l := range lines {
		ra += l.rulesAdd
		rd += l.rulesDel
		ru += l.rulesUpd
		sa += l.skillsAdd
		sd += l.skillsDel
		su += l.skillsUpd
	}
	var parts []string
	if s := deltaPart("rules", ra, rd, ru); s != "" {
		parts = append(parts, s)
	}
	if s := deltaPart("skills", sa, sd, su); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " · ")
}

// deltaPart 拼 "rules +2 −3 ~1"（只列非零操作；全零返回空串）。
func deltaPart(name string, add, del, upd int) string {
	var ops []string
	if add > 0 {
		ops = append(ops, fmt.Sprintf("+%d", add))
	}
	if del > 0 {
		ops = append(ops, fmt.Sprintf("−%d", del)) // U+2212 减号，与版式一致
	}
	if upd > 0 {
		ops = append(ops, fmt.Sprintf("~%d", upd))
	}
	if len(ops) == 0 {
		return ""
	}
	return name + " " + strings.Join(ops, " ")
}

// numOrDash 把 0 显示成 –（en-dash），非零原样——"rules – · skills 2" 比 "rules 0" 安静。
func numOrDash(n int) string {
	if n == 0 {
		return "–"
	}
	return fmt.Sprintf("%d", n)
}

// agentRoots 把一组目录压缩成 agent 根目录（~ 形式，去重保序）：
// ~/.codebuddy/rules 与 ~/.codebuddy/skills ⇒ ~/.codebuddy。
// 非 ~ 开头的目录原样保留（去重）。
func agentRoots(dirs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range dirs {
		root := ui.HomeRel(d)
		if strings.HasPrefix(root, "~/") {
			if segs := strings.SplitN(root, "/", 3); len(segs) >= 2 {
				root = segs[0] + "/" + segs[1] // ~/.codebuddy
			}
		}
		if !seen[root] {
			seen[root] = true
			out = append(out, root)
		}
	}
	return out
}
