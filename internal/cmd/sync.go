package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/engine"
	"github.com/cq-guojia/rulemux/internal/hooks"
	"github.com/cq-guojia/rulemux/internal/state"
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

	exit := 0
	changed := false
	var processed []string
	doneDirs := map[string]bool{} // 共享同一目录的 agent 只处理一次（见下）
	for _, a := range targets {
		if a.Tier == agents.Tier2 {
			// Tier-2 没有规则目录可丢文件，由 inject 子命令负责注入
			continue
		}
		dir := a.RulesDirAbs(ws)
		// 共享同一规则目录的 agent（codebuddy / workbuddy 都是 .codebuddy/rules）必须**按并集**
		// 同步：engine 的删残留是「整目录下带 __rulemux__ 前缀、不在本次计划内的一律删」
		// （engine/sync.go:164-185），若各自只算自己的 sources，就会互相把对方的文件当残留
		// 删掉 ⇒ 两个钩子轮流触发时文件来回消失。取并集后谁跑都不会删别人的。
		//
		// 同一目录也可能被 targets 里的多个 agent 命中（手动 sync 不带 --agent 时），
		// 并集结果相同 ⇒ 第二次是纯白跑，还会重复记账，故按目录去重。
		if doneDirs[dir] {
			continue
		}
		doneDirs[dir] = true
		srcs := cfg.SourcesForAny(unionAgentIDs(a, ws), ws)
		create := hook && requestedID != "" && a.ID == requestedID
		res, err := engine.Sync(dir, srcs, a.NeedsFrontmatter, create)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rulemux: failed to sync %s: %v\n", a.ID, err)
			exit = 1
			continue
		}
		printSyncResult(os.Stderr, a, dir, res)
		if res.HasChanges() {
			changed = true
		}
		// 只有真正落到一个已存在目录上（没被 SkippedNoDir 跳过）才算「用过」。
		if !res.SkippedNoDir {
			processed = append(processed, a.ID)
		}
	}

	// 记入账本（收窄触发条件）：只有确实处理过某个目录时才记账 —— 否则原则 2 下
	// 大量「跳过」会把无关工作区灌进账本。记的是「工作区 × agent」。
	// 钩子是全局的，但规则文件落在各工作区本地；卸载时要靠这份账本逐一回访清理，
	// 否则钩子一去、残留将永无机会被自动删除。
	for _, id := range processed {
		if err := state.Record(ws, id); err != nil {
			fmt.Fprintf(os.Stderr, "rulemux: warning: failed to write the workspace ledger: %v\n", err)
		}
	}

	// 顺手同步（仅 hook 路径）：把这个工作区历史上用过的**其它**写目录 agent 也一并同步。
	// 只读账本 + 只写已存在目录（原则 2），因此不会新建任何目录、不会碰到没配置的地方。
	if hook {
		done := make(map[string]bool, len(processed))
		for _, id := range processed {
			done[id] = true
		}
		cascade(cfg, ws, done)
	}

	// hook 路径下、且确有变化时，给模型一句话（它应转告用户重开会话）。
	// 无变化 ⇒ stdout 保持为空（一个字节都不输出）。
	//
	// per-agent 门禁：只有被点名的那个 agent 明确「需要提示 + 协议已知」时才输出
	// （见设计 §六「per-agent」）——有的 agent 会话内会重读规则，根本不该提示。
	if hook && changed {
		if a, ok := agents.Get(requested); ok && a.SessionHint && a.HintProtocol != "" {
			emitChangeNotice(os.Stdout, a.HintProtocol)
		}
	}
	return exit
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

// cascade 顺手同步该工作区历史上用过的**其它** Tier-1 agent。
//
// 硬规则（设计 §七）：
//   - 候选来自**账本**（该工作区用过哪些 agent），不是扫盘、也不是配置通配符；
//   - 目录存在性**只认 os.Stat**（账本不作数，目录可能已被手工删）⇒ 不存在就跳过，**绝不创建**；
//   - 只处理已核实（Verified）的 agent；查不到 / 未核实 ⇒ 跳过（账本条目保留）；
//   - want 为空 ⇒ 什么也不做（不去清别人家的目录）；
//   - **只一层**，不顺带再顺带；失败只记 warning，不影响主流程退出码。
func cascade(cfg *config.Config, ws string, done map[string]bool) {
	l := state.Load()
	for _, a := range agentsFromLedger(l.AgentsFor(ws)) {
		if done[a.ID] || a.Tier != agents.Tier1 {
			continue
		}
		dir := a.RulesDirAbs(ws)
		if dir == "" {
			continue
		}
		if _, err := os.Stat(dir); err != nil {
			continue // 目录不在（或不可读）⇒ 不碰、更不建
		}
		// 同主循环：共享目录取并集，避免删掉另一方的文件。
		srcs := cfg.SourcesForAny(unionAgentIDs(a, ws), ws)
		if len(srcs) == 0 {
			continue // want 为空 ⇒ 不碰
		}
		res, err := engine.Sync(dir, srcs, a.NeedsFrontmatter, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rulemux: warning: cascade sync for %s failed: %v\n", a.ID, err)
			continue
		}
		if !res.IsEmpty() {
			printSyncResult(os.Stderr, a, dir, res)
		}
		if !res.SkippedNoDir {
			if err := state.Record(ws, a.ID); err != nil {
				fmt.Fprintf(os.Stderr, "rulemux: warning: failed to write the workspace ledger: %v\n", err)
			}
		}
	}
}

// syncAll 实现 `rulemux sync --all`：只遍历账本记录过的工作区，把它们各自对齐到当前配置。
//
// 语义（设计 §五 / §八）：
//   - **不读 cwd**、**不使用配置里的 workspace 通配符**去发现工作区；
//   - 每个工作区按账本记录的 agents 逐个同步（哨兵 `*` ⇒ 全部已支持 agent）；
//   - **只写已存在的目录**（create=false，原则 2），目录不存在就跳过、绝不新建；
//   - 顺手 GC：路径**确实不存在**（ENOENT）的账本条目会被清掉；其它错误（权限 / 断连）
//     只跳过、保留条目（可能是临时不可用的外挂盘）。
func syncAll(cfg *config.Config) int {
	l := state.Load()
	workspaces := l.AllWorkspaces()
	if len(workspaces) == 0 {
		fmt.Fprintln(os.Stderr, "rulemux: no workspace recorded in the ledger yet; nothing to do")
		return 0
	}

	exit := 0
	var gone []string
	for _, ws := range workspaces {
		if _, err := os.Stat(ws); err != nil {
			if os.IsNotExist(err) {
				gone = append(gone, ws) // 确实不存在 ⇒ 待 GC
			} else {
				fmt.Fprintf(os.Stderr, "rulemux: skip %s: %v\n", ws, err)
			}
			continue
		}
		for _, a := range agentsFromLedger(l.AgentsFor(ws)) {
			dir := a.RulesDirAbs(ws)
			// 同主循环：共享目录取并集，避免删掉另一方的文件。
			res, err := engine.Sync(dir, cfg.SourcesForAny(unionAgentIDs(a, ws), ws), a.NeedsFrontmatter, false)
			if err != nil {
				fmt.Fprintf(os.Stderr, "rulemux: failed to sync %s in %s: %v\n", a.ID, ws, err)
				exit = 1
				continue
			}
			printSyncResult(os.Stderr, a, dir, res)
			if !res.SkippedNoDir {
				if err := state.Record(ws, a.ID); err != nil {
					fmt.Fprintf(os.Stderr, "rulemux: warning: failed to write the workspace ledger: %v\n", err)
				}
			}
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

// targetAgents 决定本次处理哪些 agent；未指定则处理全部「已验证」的 Tier-1。
// 未做好的 agent（Verified==false）一律不参与，从源头保证只动做好的适配。
// hookedPeers 返回「与 a 共享同一规则目录、且还装着 rulemux 钩子」的 agent（含 a 自己）。
//
// 「还装着」是判断某个 agent 是否仍在使用的唯一依据：卸载 = 摘钩子，钩子一没它就自然
// 退出并集，因此**不需要任何额外的状态文件**。
//
// 若共享组里一个都没装钩子（纯手动 sync 的场景）⇒ 退回该共享组全部 agent，
// 保证手动跑 `rulemux sync` 仍有东西落。
//
// ⚠ 卸载侧调用时必须在 `hooks.Uninstall` **之前**取好结果：钩子一摘就判不出谁还在用。
func hookedPeers(a agents.Agent, ws string) []agents.Agent {
	group := agents.SharingRulesDir(a)
	var hooked []agents.Agent
	for _, b := range group {
		if hookInstalled(b, ws) {
			hooked = append(hooked, b)
		}
	}
	if len(hooked) == 0 {
		return group // 一个都没装 ⇒ 退回整组（手动 sync 兜底）
	}
	return hooked
}

// hookInstalled 报告该 agent 是否还装着 rulemux 的 SessionStart 钩子。
//
// 读不了、解析失败、格式未核实（codex 的 TOML）一律按「未装」降级 —— 绝不因为读不了
// 配置就中断同步，更不能据此误删别人的文件。
func hookInstalled(a agents.Agent, ws string) bool {
	installed, _, err := hooks.Inspect(a.HookFileAbs(ws), a)
	return err == nil && installed
}

// unionAgentIDs 返回参与并集的 agent 规范 ID 列表（供 config.SourcesForAny 使用）。
func unionAgentIDs(a agents.Agent, ws string) []string {
	peers := hookedPeers(a, ws)
	ids := make([]string, 0, len(peers))
	for _, p := range peers {
		ids = append(ids, p.ID)
	}
	return ids
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

// printSyncResult 把单个 agent 的同步结果打到 w（人类可读文本，调用方统一传 os.Stderr）。
func printSyncResult(w io.Writer, a agents.Agent, dir string, r *engine.SyncResult) {
	fmt.Fprintf(w, "rulemux: %s → %s\n", a.ID, dir)
	if r.SkippedNoDir {
		fmt.Fprintln(w, "  skipped: rules dir does not exist (only the agent's own hook may create it)")
		return
	}
	if len(r.Copied) > 0 {
		fmt.Fprintln(w, "  Added:", strings.Join(r.Copied, ", "))
	}
	if len(r.Updated) > 0 {
		fmt.Fprintln(w, "  Updated:", strings.Join(r.Updated, ", "))
	}
	if len(r.Deleted) > 0 {
		fmt.Fprintln(w, "  Removed residue:", strings.Join(r.Deleted, ", "))
	}
	if len(r.Skipped) > 0 {
		fmt.Fprintf(w, "  Unchanged, skipped: %d file(s)\n", len(r.Skipped))
	}
	for _, m := range r.Missing {
		fmt.Fprintf(w, "  ⚠ source file missing: %s\n", m)
	}
	for _, e := range r.Errors {
		fmt.Fprintf(w, "  ⚠ %s\n", e)
	}
	if len(r.Skipped) == 0 && r.IsEmpty() {
		fmt.Fprintln(w, "  Nothing to do (no source files listed for this agent)")
	}
}
