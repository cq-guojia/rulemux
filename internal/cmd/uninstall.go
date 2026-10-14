package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/engine"
	"github.com/cq-guojia/rulemux/internal/hooks"
	"github.com/cq-guojia/rulemux/internal/state"
)

// Uninstall removes rulemux's SessionStart hook (and, for Tier-1 agents, the
// synced __rulemux__* files, the rules/skills dir once it is empty, and the
// agent's ledger entries) for the agent(s) named by --agent, OR for every
// agent when --off / --all is given.
//
// Exactly one mode is required:
//   - --agent <id[,id...]>   remove only the named agent(s)
//   - --off | --all          remove for ALL agents
//
// Nothing is removed until the user confirms (unless --yes is passed).
//
// 断根顺序（D2/D3）：先清理、后动账本。清理三态判定——目录不存在（确无东西可清）
// 或删净了 ⇒ 账本可动 + 可删空目录；没试（配置读不了 / 工作区未声明）或失败 ⇒
// 告警且账本原样保留（残留还在，不能假装卸干净）。钩子没摘掉的 agent 整个跳过：
// 它还在用，什么都不该动。
func Uninstall(args []string) int {
	f := ParseFlags(args)
	if f.Has("help") || f.Has("h") {
		UninstallHelp()
		return 0
	}

	all := f.Has("off") || f.Has("all")
	agentArg := f.Get("agent", "")

	// Mutually-exclusive, exactly-one-required.
	if !all && agentArg == "" {
		fmt.Fprintln(os.Stderr, "rulemux uninstall: must specify either --agent <id[,id...]> OR --off/--all")
		fmt.Fprintln(os.Stderr, "  run 'rulemux uninstall --help' for details")
		return 2
	}
	if all && agentArg != "" {
		fmt.Fprintln(os.Stderr, "rulemux uninstall: --agent and --off/--all are mutually exclusive")
		return 2
	}

	var targets []agents.Agent
	if all {
		// 与 sync 统一口径：只有「能装进去的」才需要卸（未验证 agent 从不产生产物）。
		targets = agents.Supported()
	} else {
		ts, err := parseAgents(agentArg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rulemux uninstall:", err)
			return 2
		}
		targets = ts
	}

	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}

	// Interactive confirmation, unless --yes skips it (for scripts/CI).
	// 账本单次快照贯穿 confirm → sweep → 写账本（Others 被 Load 两次会 TOCTOU）。
	ledger := state.Load()
	if !f.Has("yes") {
		if ok := confirmUninstall(targets, ws, ledger); !ok {
			fmt.Println("rulemux uninstall: cancelled")
			return 0
		}
	}

	// 收敛目录需要知道「卸载后还有谁在用这个目录」，而这依赖配置（要算剩余 agent 的文件集合）。
	// 读不到配置 ⇒ 不动任何文件（只摘钩子），宁可留残留也不误删仍在用 agent 的文件。
	cfg, cfgErr := config.Load(f.Get("config", config.DefaultPath()))
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ cannot read config (%v): synced files are left untouched\n", cfgErr)
	} else if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ invalid config (%v): synced files are left untouched\n", err)
		cfg = nil
	}

	// 账本收敛收集器：每个「真卸干净」的 agent ⇒ off += 它 + 从清过（或确无东西
	// 可清）的工作区列表里移除它。最后批量一次加锁写（见循环后）。
	var updates []ledgerUpdate

	for _, a := range targets {
		// ⚠ 必须先于 hooks.Uninstall 计算：判据是「谁还装着钩子」，钩子一摘就判不出来了。
		keep := remainingPeers(a, ws)

		if a.Style == "external" {
			// R1 警告（2026-10-14 随 dsh 改纯注入重写措辞）：宿主侧插件不由 rulemux 装，
			// 也卸不掉；而且它的行为变了——不再往盘上同步任何东西，改为每个会话自己跑
			// `rulemux inject --agent <id>` 取渲染结果注入。所以「卸载停不掉它」依然
			// 成立，只是原因从「插件会把文件写回来」变成「插件会继续向会话注入」。
			// 只印有据的停用方式（cordis.patch.yml 的 `disabled: true`，见
			// dsh-plugin/cordis.patch.yml）；未经坐实的 remove 语法不印
			// （未核实不得作为实现依据）。
			fmt.Printf("  ⚠ %-10s the host-side plugin is not rulemux's to remove.\n", a.ID)
			fmt.Printf("      If it is still installed, every %s session keeps calling\n", a.ID)
			fmt.Printf("      `rulemux inject --agent %s` and injecting the rules it renders — so this\n", a.ID)
			fmt.Printf("      uninstall does not stop that. Disable the plugin row in the profile's cordis.patch.yml\n")
			fmt.Printf("      (ID `%s`, set `disabled: true`), or remove it with %s's own plugin command.\n", a.ID, a.ID)
			if n, dir := legacyResidue(a, ws); n > 0 {
				fmt.Printf("      Note: %d leftover file(s) from the old directory hand-off are still in %s;\n", n, dir)
				fmt.Printf("      rulemux no longer writes there — delete them yourself if you want them gone.\n")
			}
		} else if err := hooks.Uninstall(a, ws); err != nil {
			// 钩子没摘掉 = agent 还在用：不动它的文件、不动它的账本，提示修好重跑。
			fmt.Fprintf(os.Stderr, "  ✗ %-10s hook: %v\n", a.ID, err)
			fmt.Fprintf(os.Stderr, "      %s is still in use — its files and ledger entry are untouched; fix the hook config and re-run\n", a.ID)
			continue
		} else {
			fmt.Printf("  ✓ %-10s hook removed (%s)\n", a.ID, a.HookFileAbs(ws))
		}

		// 当前工作区收敛：rules 与 skills 都收（skills 此前只在 sweep 里收，见 H3）。
		wsOK := true
		if dir := a.RulesDirAbs(ws); dir != "" {
			deleted, outcome := pruneRulesDir(cfg, dir, keep, ws, a.NeedsFrontmatter)
			reportPrune(a.ID, dir, deleted, len(keep), outcome)
			if outcome == pruneOK {
				removeEmptyLeaf(dir)
			} else {
				wsOK = false
			}
		}
		if dir := a.SkillsDirAbs(ws); dir != "" {
			deleted, outcome := pruneSkillsDir(cfg, dir, keep, ws)
			reportPrune(a.ID, dir, deleted, len(keep), outcome)
			if outcome == pruneOK {
				removeEmptyLeaf(dir)
			} else {
				wsOK = false
			}
		}

		// 用户级（全局）目录收敛：用户级目录与工作区无关，按「卸载后还有谁在用」
		// （SharingUserDirs 里还装着钩子的）收敛；只删 __rulemux__ 前缀内容，目录本身保留。
		if len(a.UserRulesDirs)+len(a.UserSkillsDirs) > 0 {
			userKeep := remainingUserPeers(a, ws)
			if n := pruneUserDirs(cfg, a, userKeep); n > 0 {
				fmt.Printf("  ✓ %-10s removed %d user-level item(s) no longer needed (kept %d still in use)\n",
					a.ID, n, len(userKeep))
			}
		}

		// Revisit the other workspaces recorded in the ledger: the hook is now gone,
		// so without this sweep their residue would never be removed.
		cleaned, skipped, eligible := sweepRecordedWorkspaces(a, ws, cfg, keep, ledger)
		if cleaned > 0 || skipped > 0 {
			fmt.Printf("  ✓ %-10s other workspaces: removed %d residue file(s); skipped %d workspace(s) that no longer exist (kept in the ledger — revisited if the path reappears)\n",
				a.ID, cleaned, skipped)
		}

		wsList := eligible
		if wsOK {
			wsList = append([]string{ws}, eligible...)
		}
		updates = append(updates, ledgerUpdate{agent: a.ID, ws: wsList})
	}

	// 账本收敛（一次加锁批量写）：off += 每个卸掉的 agent；从「清过或确无东西可清」
	// 的工作区列表里移除它。先清理后删账本的落地口——清理被跳过的工作区不在
	// wsList 里，条目原样保留。
	if len(updates) > 0 {
		if err := state.UpdateLedger(func(l *state.Ledger) error {
			for _, u := range updates {
				l.AddOff(u.agent)
				for _, ws := range u.ws {
					l.ForgetAgent(ws, u.agent)
				}
			}
			return nil
		}); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ files were cleaned, but the ledger could not be updated: %v\n", err)
			fmt.Fprintln(os.Stderr, "    re-run this uninstall to finish the ledger side")
		}
	}
	return 0
}

// ledgerUpdate 是一条待写入账本的收敛记录：agent 从这些工作区的列表里移除，
// 并收进机器级 off 集合。
type ledgerUpdate struct {
	agent string
	ws    []string
}

// pruneOutcome 是 prune 一次目录的结果三态：
//   - pruneOK：删净了，或目录本来就不存在（SkippedNoDir ⇒ 确无东西可清）
//     ⇒ 可删空目录、账本可动；
//   - pruneSkipped：没试（配置读不了 / 工作区未被配置声明）⇒ 账本不能动；
//   - pruneFailed：试了但出错（残留可能还在）⇒ 账本不能动。
type pruneOutcome int

const (
	pruneOK pruneOutcome = iota
	pruneSkipped
	pruneFailed
)

// reportPrune 打一次目录收敛的结果（三态；失败时明确说账本没动）。
func reportPrune(id, dir string, deleted, kept int, outcome pruneOutcome) {
	switch {
	case outcome == pruneSkipped:
		fmt.Printf("  · %-10s files untouched (%s is not a declared workspace, or config unreadable)\n", id, dir)
	case outcome == pruneFailed:
		fmt.Printf("  ⚠ %-10s could not fully clean %s (%d file(s) removed) — ledger entry kept\n", id, dir, deleted)
	case deleted > 0:
		fmt.Printf("  ✓ %-10s removed %d file(s) no longer needed in %s (kept %d still in use)\n", id, deleted, dir, kept)
	default:
		fmt.Printf("  · %-10s no synced files in %s\n", id, dir)
	}
}

// legacyResidue 数一数「旧交接目录」（该 agent 已不再使用的规则目录）里还剩多少个 rulemux
// 自己的条目，返回数量与该目录的绝对路径。**只数不删**：那段设计已作废，残留不强制清除
// （用户 2026-10-14 定调：新的不许再写，已经清不掉的就算了），但卸载时该告诉用户它在哪，
// 而不是装作不存在。只认 engine.Prefix 前缀——别的工具放在同一个目录里的文件一概不算。
func legacyResidue(a agents.Agent, ws string) (int, string) {
	if a.LegacyRulesDir == "" {
		return 0, ""
	}
	dir := filepath.Join(ws, a.LegacyRulesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, ""
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), engine.Prefix) {
			n++
		}
	}
	return n, dir
}

// removeEmptyLeaf 删掉「确认为空」的工作区级叶子目录（rules / skills）。
//
// 安全边界（删除类操作的四条铁律）：
//   - Lstat 不跟随软链：软链或非目录 ⇒ 跳过（不删链接本身，更不碰目标）；
//   - len(ReadDir)==0 才删——任何条目（用户自己的文件、canary、前缀子目录）都算非空；
//   - 只用 os.Remove：绝不 RemoveAll、绝不上溯父目录（.dsh/.codebuddy 本身不碰）；
//   - ENOTEMPTY/ENOENT（并发 hook 正在写）当跳过，不抬错。
func removeEmptyLeaf(dir string) bool {
	if dir == "" {
		return false
	}
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return false
	}
	if err := os.Remove(dir); err != nil {
		return false
	}
	return true
}

// confirmUninstall prints the targets and asks the user to confirm.
// Returns false on any answer other than an explicit yes, or on EOF
// (non-interactive stdin without --yes) so we fail safe.
func confirmUninstall(targets []agents.Agent, ws string, ledger state.Ledger) bool {
	fmt.Printf("\nrulemux uninstall: about to remove rulemux from %d agent(s) in %s\n", len(targets), ws)
	for _, a := range targets {
		what := "hook"
		if a.Style == "external" {
			what = "plugin"
		}
		kind := what + " only"
		if a.RulesDirAbs(ws) != "" {
			kind = what + " + synced files"
		}
		fmt.Printf("  - %s (%s)\n", a.ID, kind)
	}
	if others := ledger.Others(ws); len(others) > 0 {
		fmt.Printf("  Note: the hook lives in the user-level host config (applies globally), so after\n")
		fmt.Printf("  this uninstall no sync will ever fire again. rulemux will therefore also revisit\n")
		fmt.Printf("  these %d previously synced workspace(s) from its ledger and clean their residue:\n", len(others))
		for _, w := range others {
			fmt.Printf("    - %s\n", w)
		}
		fmt.Printf("  (only files with the __rulemux__ prefix are removed; your source files and everything else are untouched)\n")
		fmt.Printf("  Each agent is also dropped from the ledger for every workspace cleaned here.\n")
	}
	fmt.Print("Proceed? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println() // EOF / non-interactive: abort safely
		return false
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

// sweepRecordedWorkspaces 按账本回访「曾经同步过的其它工作区」，清掉该 agent 的残留文件。
// 钩子此刻已被全局移除，若不做这一步，那些工作区里的 __rulemux__* 将再无机会被删除。
//
// 返回：清理掉的文件数、因工作区已不存在而跳过的工作区数、以及「清过或确无东西
// 可清」的工作区列表（eligible）——只有这些工作区的账本条目允许被改；清理被跳过
// （配置读不了 / 未声明）或失败的工作区，残留可能还在，条目必须保留（先清理后删账本）。
// 即便路径已不存在也不从账本剔除——万一该工作区日后重现（如重新 clone），
// 下一次卸载仍会回访并清理它。
func sweepRecordedWorkspaces(a agents.Agent, current string, cfg *config.Config, keep []agents.Agent, ledger state.Ledger) (cleaned, skipped int, eligible []string) {
	if a.RulesDir == "" && a.SkillsDir == "" {
		return 0, 0, nil // Tier-2 磁盘上不留任何文件，无需回访
	}
	for _, ws := range ledger.Others(current) {
		if _, err := os.Stat(ws); err != nil {
			skipped++
			continue // 工作区已被删除，本次无从清理
		}
		ok := true
		if a.RulesDir != "" {
			dir := a.RulesDirAbs(ws)
			d, outcome := pruneRulesDir(cfg, dir, keep, ws, a.NeedsFrontmatter)
			cleaned += d
			if outcome == pruneOK {
				removeEmptyLeaf(dir)
			} else {
				fmt.Fprintf(os.Stderr, "  ⚠ %s: could not fully clean %s — its ledger entry is kept\n", ws, dir)
				ok = false
			}
		}
		if a.SkillsDir != "" {
			dir := a.SkillsDirAbs(ws)
			d, outcome := pruneSkillsDir(cfg, dir, keep, ws)
			cleaned += d
			if outcome == pruneOK {
				removeEmptyLeaf(dir)
			} else {
				fmt.Fprintf(os.Stderr, "  ⚠ %s: could not fully clean %s — its ledger entry is kept\n", ws, dir)
				ok = false
			}
		}
		if ok {
			eligible = append(eligible, ws)
		}
	}
	return cleaned, skipped, eligible
}

// remainingUserPeers 返回「卸载 a 之后，仍在使用同一**用户级**目录」的 agent。
// 同 remainingPeers 的判据（还装着钩子），也必须在 hooks.Uninstall 之前调用。
func remainingUserPeers(a agents.Agent, ws string) []agents.Agent {
	var out []agents.Agent
	for _, b := range agents.SharingUserDirs(a) {
		if b.ID == a.ID {
			continue
		}
		if hookInstalled(b, ws) {
			out = append(out, b)
		}
	}
	return out
}

// pruneUserDirs 把 a 的用户级（全局）目录收敛到 keep 中那些 agent 应有的集合：
// 用户级 rules 走 engine.Sync、用户级 skills 走 engine.SyncSkills（create=false
// ⇒ 目录不存在就跳过）。keep 为空 ⇒ want 为空 ⇒ 前缀内容全清（--all 的全清语义）。
// 目录本身始终保留（那是 agent 的原生目录，不该由 rulemux 删）。
// 返回删掉的条目数（文件 + skill 目录）。
func pruneUserDirs(cfg *config.Config, a agents.Agent, keep []agents.Agent) int {
	if cfg == nil {
		return 0 // 配置不可读 ⇒ 不动文件，宁可留残留也不误删
	}
	ids := peerIDs(keep)
	deleted := 0
	for _, dir := range a.UserRulesDirsAbs() {
		res, err := engine.Sync(dir, cfg.UserSourcesForAny(ids), a.NeedsFrontmatter, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ failed to prune %s: %v\n", dir, err)
			continue
		}
		deleted += len(res.Deleted)
	}
	for _, dir := range a.UserSkillsDirsAbs() {
		res, err := engine.SyncSkills(dir, cfg.UserSkillSourcesForAny(ids), false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ failed to prune %s: %v\n", dir, err)
			continue
		}
		deleted += len(res.Deleted)
	}
	return deleted
}

// pruneSkillsDir 把 skills 目录收敛到 keep 中那些 agent 应有的 skill 集合
// （同 pruneRulesDir 套路：create=false、只认 __rulemux__ 前缀、三态回传）。
func pruneSkillsDir(cfg *config.Config, dir string, keep []agents.Agent, ws string) (deleted int, outcome pruneOutcome) {
	if cfg == nil {
		return 0, pruneSkipped // 配置不可读 ⇒ 不动文件，宁可留残留也不误删
	}
	if !cfg.DeclaresWorkspace(ws) {
		return 0, pruneSkipped // 未声明的工作区 ⇒ 与 sync 同一条防线，不碰
	}
	res, err := engine.SyncSkills(dir, cfg.SkillSourcesForAny(peerIDs(keep), ws), false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ failed to prune %s: %v\n", dir, err)
		return 0, pruneFailed
	}
	if res.SkippedNoDir {
		return 0, pruneOK // 目录不存在 ⇒ 确无东西可清
	}
	if len(res.Errors) > 0 {
		return len(res.Deleted), pruneFailed // 有条目没删干净 ⇒ 残留还在
	}
	return len(res.Deleted), pruneOK
}

// remainingPeers 返回「卸载 a 之后，仍在使用同一规则目录」的 agent。
//
// 判据是「还装着钩子」（hooks.Inspect），所以**必须在 hooks.Uninstall 之前调用**：
// 钩子一摘，a 的兄弟们也判不出谁还在用了。
func remainingPeers(a agents.Agent, ws string) []agents.Agent {
	var out []agents.Agent
	for _, b := range agents.SharingRulesDir(a) {
		if b.ID == a.ID {
			continue
		}
		if hookInstalled(b, ws) {
			out = append(out, b)
		}
	}
	return out
}

// pruneRulesDir 把规则目录收敛到 keep 中那些 agent 应有的文件集合：
// 删掉不再有人要的，保留还在用的。keep 为空 ⇒ want 为空 ⇒ 全部删除（正是 --all 的全清语义）。
//
// 走 engine.Sync 而不是自己 Glob 删除，好处有三个：
//   - create=false ⇒ 目录不存在就跳过，守住「只写已存在目录」的铁律；
//   - 复用 engine 的删残留口径（跳过子目录、只认 __rulemux__ 前缀），不会误删用户自己的文件；
//   - 目录终态与 sync 完全一致。
//
// 返回三态（见 pruneOutcome）：SkippedNoDir = 确无东西可清（OK）；配置不可读 /
// 工作区未声明 = 没试（skipped）；engine 报错或有单文件失败 = 失败（失败时残留
// 可能还在，账本不能动）。
//
// shortcut: engine 删残留**跳过子目录**（不记 Errors）⇒ rules 目录里若存在
// `__rulemux__xxx/` 子目录会被判成「已清干净」（pruneOK）而账本照删，那类子目录
// 也永远删不掉。正常路径不会产生它（rules 引擎只写文件，目录只出现在 skills），
// 升级触发点：rules 引擎开始写子目录，或要把子目录残留也纳入收敛时。
func pruneRulesDir(cfg *config.Config, dir string, keep []agents.Agent, ws string, frontmatter bool) (deleted int, outcome pruneOutcome) {
	if cfg == nil {
		return 0, pruneSkipped // 配置不可读 ⇒ 不动文件，宁可留残留也不误删
	}
	if !cfg.DeclaresWorkspace(ws) {
		return 0, pruneSkipped // 未声明的工作区 ⇒ 与 sync 同一条防线，不碰
	}
	ids := make([]string, 0, len(keep))
	for _, b := range keep {
		ids = append(ids, b.ID)
	}
	res, err := engine.Sync(dir, cfg.SourcesForAny(ids, ws), frontmatter, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ failed to prune %s: %v\n", dir, err)
		return 0, pruneFailed
	}
	if res.SkippedNoDir {
		return 0, pruneOK // 目录不存在 ⇒ 确无东西可清
	}
	if len(res.Errors) > 0 {
		return len(res.Deleted), pruneFailed // 有文件没删干净 ⇒ 残留还在
	}
	return len(res.Deleted), pruneOK
}

// UninstallHelp prints detailed English help for `rulemux uninstall`.
func UninstallHelp() {
	fmt.Print(`rulemux uninstall - remove rulemux from one or all agents

USAGE:
  rulemux uninstall --agent <id[,id...]> [--config <path>] [--workspace <dir>] [--yes]
  rulemux uninstall --off  | --all        [--config <path>] [--workspace <dir>] [--yes]

EXACTLY ONE MODE IS REQUIRED (they are mutually exclusive):
  --agent <id[,id...]>   Remove only the named agent(s), e.g. --agent dsh
                          or --agent codebuddy,codex.
  --off  | --all          Remove hooks (and Tier-1 synced files) for ALL agents.

WHAT IT DOES:
  For each target agent, it:
    1. removes rulemux's SessionStart hook from that agent's hook config.
       A plugin host (dsh) has no hook rulemux can remove: it prints a WARNING
       instead — that plugin keeps calling "rulemux inject --agent dsh" in every
       session, so rulemux cannot stop the delivery on its own; disable the plugin
       in the profile's cordis.patch.yml (set the rulemux row's 'disabled: true');
    2. deletes the synced __rulemux__* files from that agent's rules/skills dirs
       (Tier-1 agents only; Tier-2 agents like codex/opencode/dsh keep nothing on disk);
    3. removes the rules/skills dir itself once it is then empty (workspace level
       only: a symlinked, non-empty or subdir-bearing dir is left alone, and the
       dir above it is never touched; user-level dirs are kept);
    4. drops the agent from the ledger for every workspace it was cleaned from and
       records it as uninstalled, so no later sync writes to it again. Step 4 only
       happens after cleaning actually succeeded: if the hook could not be removed,
       or the config / workspace could not be read, nothing is changed and the
       ledger entry is kept.

CONFIRMATION:
  Before anything is removed, rulemux lists the targets and asks "Proceed? [y/N]".
  Type y or yes to continue; anything else (or EOF on non-interactive stdin)
  cancels safely.
  Pass --yes to skip the prompt (for scripts / CI).

Supported (verified) ids: codebuddy (alias codebuddy-cn), workbuddy (own hook file
~/.workbuddy/settings.json, rules dir shared with codebuddy), trae (aliases trae-cn /
trae-intl), dsh (aliases deepseek / deepseek-harness).
Other registered agents (claude / codex / opencode) are NOT yet verified and
cannot be installed or removed until their adapter is canary-tested.

OTHER FLAGS:
  --workspace <dir>   Workspace whose synced __rulemux__* files should be
                      removed (default: current dir). NOTE: the SessionStart
                      hook itself lives in the user-level host config
                      (~/.codebuddy/settings.json), so removing it takes
                      effect globally regardless of --workspace.
  --yes               Skip the confirmation prompt.
  --help, -h          Show this help.

NOTE: this only removes rulemux's own artifacts. Your agent's other
configuration and your own (non-__rulemux__) rule files are never touched.
`)
}
