package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/engine"
	"github.com/cq-guojia/rulemux/internal/hooks"
	"github.com/cq-guojia/rulemux/internal/state"
)

// Uninstall removes rulemux's SessionStart hook (and, for Tier-1 agents, the
// synced __rulemux__* files) for the agent(s) named by --agent, OR for every
// agent when --off / --all is given.
//
// Exactly one mode is required:
//   - --agent <id[,id...]>   remove only the named agent(s)
//   - --off | --all          remove for ALL agents
//
// Nothing is removed until the user confirms (unless --yes is passed).
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
	if !f.Has("yes") {
		if ok := confirmUninstall(targets, ws); !ok {
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

	for _, a := range targets {
		// ⚠ 必须先于 hooks.Uninstall 计算：判据是「谁还装着钩子」，钩子一摘就判不出来了。
		keep := remainingPeers(a, ws)

		if a.Style == "external" {
			// 宿主侧插件由宿主自己的命令移除；rulemux 只负责收敛下面的规则目录。
			fmt.Printf("  · %-10s plugin is installed by the host — remove it with the host's own command (dsh: `dsh plugin ... remove`)\n", a.ID)
		} else if err := hooks.Uninstall(a, ws); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %-10s hook: %v\n", a.ID, err)
		} else {
			fmt.Printf("  ✓ %-10s hook removed (%s)\n", a.ID, a.HookFileAbs(ws))
		}

		if dir := a.RulesDirAbs(ws); dir != "" {
			deleted, untouched := pruneRulesDir(cfg, dir, keep, ws, a.NeedsFrontmatter)
			switch {
			case untouched:
				fmt.Printf("  · %-10s files untouched (%s is not a declared workspace, or config unreadable)\n", a.ID, ws)
			case deleted > 0:
				fmt.Printf("  ✓ %-10s removed %d file(s) no longer needed in %s (kept %d still in use)\n",
					a.ID, deleted, dir, len(keep))
			default:
				fmt.Printf("  · %-10s no synced files in %s\n", a.ID, dir)
			}
		}

		// Revisit the other workspaces recorded in the ledger: the hook is now gone,
		// so without this sweep their residue would never be removed.
		if cleaned, skipped := sweepRecordedWorkspaces(a, ws, cfg, keep); cleaned > 0 || skipped > 0 {
			fmt.Printf("  ✓ %-10s other workspaces: removed %d residue file(s); skipped %d workspace(s) that no longer exist (kept in the ledger — revisited if the path reappears)\n",
				a.ID, cleaned, skipped)
		}
	}
	return 0
}

// confirmUninstall prints the targets and asks the user to confirm.
// Returns false on any answer other than an explicit yes, or on EOF
// (non-interactive stdin without --yes) so we fail safe.
func confirmUninstall(targets []agents.Agent, ws string) bool {
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
	if others := state.Others(ws); len(others) > 0 {
		fmt.Printf("  Note: the hook lives in the user-level host config (applies globally), so after\n")
		fmt.Printf("  this uninstall no sync will ever fire again. rulemux will therefore also revisit\n")
		fmt.Printf("  these %d previously synced workspace(s) from its ledger and clean their residue:\n", len(others))
		for _, w := range others {
			fmt.Printf("    - %s\n", w)
		}
		fmt.Printf("  (only files with the __rulemux__ prefix are removed; your source files and everything else are untouched)\n")
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
// 返回：清理掉的文件数、因工作区已不存在而跳过的工作区数。
// 说明：即便路径已不存在也不从账本剔除——万一该工作区日后重现（如重新 clone），
// 下一次卸载仍会回访并清理它。
func sweepRecordedWorkspaces(a agents.Agent, current string, cfg *config.Config, keep []agents.Agent) (cleaned, skipped int) {
	if a.RulesDir == "" {
		return 0, 0 // Tier-2 磁盘上不留任何文件，无需回访
	}
	for _, ws := range state.Others(current) {
		if _, err := os.Stat(ws); err != nil {
			skipped++
			continue // 工作区已被删除，本次无从清理
		}
		deleted, _ := pruneRulesDir(cfg, a.RulesDirAbs(ws), keep, ws, a.NeedsFrontmatter)
		cleaned += deleted
	}
	return cleaned, skipped
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
// 返回：删掉的文件数、是否因「工作区未被配置声明 / 配置不可读」而完全没动。
func pruneRulesDir(cfg *config.Config, dir string, keep []agents.Agent, ws string, frontmatter bool) (deleted int, untouched bool) {
	if cfg == nil {
		return 0, true // 配置不可读 ⇒ 不动文件，宁可留残留也不误删
	}
	if !cfg.DeclaresWorkspace(ws) {
		return 0, true // 未声明的工作区 ⇒ 与 sync 同一条防线，不碰
	}
	ids := make([]string, 0, len(keep))
	for _, b := range keep {
		ids = append(ids, b.ID)
	}
	res, err := engine.Sync(dir, cfg.SourcesForAny(ids, ws), frontmatter, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ failed to prune %s: %v\n", dir, err)
		return 0, true
	}
	if res.SkippedNoDir {
		return 0, true
	}
	return len(res.Deleted), false
}

// UninstallHelp prints detailed English help for `rulemux uninstall`.
func UninstallHelp() {
	fmt.Print(`rulemux uninstall - remove rulemux from one or all agents

USAGE:
  rulemux uninstall --agent <id[,id...]> [--workspace <dir>] [--yes]
  rulemux uninstall --off  | --all        [--workspace <dir>] [--yes]

EXACTLY ONE MODE IS REQUIRED (they are mutually exclusive):
  --agent <id[,id...]>   Remove only the named agent(s), e.g. --agent codex
                          or --agent codebuddy,codex.
  --off  | --all          Remove hooks (and Tier-1 synced files) for ALL agents.

WHAT IT DOES:
  For each target agent, it:
    1. removes rulemux's SessionStart hook from that agent's hook config;
    2. deletes the synced __rulemux__* files from that agent's rules dir
       (Tier-1 agents only; Tier-2 agents like codex/opencode keep nothing on disk).

CONFIRMATION:
  Before anything is removed, rulemux lists the targets and asks "Proceed? [y/N]".
  Type y or yes to continue; anything else (or EOF on non-interactive stdin)
  cancels safely.
  Pass --yes to skip the prompt (for scripts / CI).

Supported (verified) ids: codebuddy (alias: codebuddy-cn), workbuddy (separate agent, own ~/.workbuddy/settings.json).
Other registered agents (claude / trae / codex / opencode) are NOT yet verified and
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
