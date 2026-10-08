package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/engine"
	"github.com/cq-guojia/rulemux/internal/hooks"
	"github.com/cq-guojia/rulemux/internal/state"
)

// Uninstall removes rulemux's SessionStart hook (and, for Tier-1 agents, the
// synced .rulemux__* files) for the agent(s) named by --agent, OR for every
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
		targets = agents.All()
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

	for _, a := range targets {
		if err := hooks.Uninstall(a, ws); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %-10s hook: %v\n", a.ID, err)
		} else {
			fmt.Printf("  ✓ %-10s hook removed (%s)\n", a.ID, a.HookFileAbs(ws))
		}

		if dir := a.RulesDirAbs(ws); dir != "" {
			removed, err := removeRuleFiles(dir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ %-10s files: %v\n", a.ID, err)
			} else if len(removed) > 0 {
				fmt.Printf("  ✓ %-10s removed %d synced file(s) from %s\n", a.ID, len(removed), dir)
			} else {
				fmt.Printf("  · %-10s no synced files in %s\n", a.ID, dir)
			}
		}

		// 回访账本里记录的其它工作区：钩子已全局移除，这里不补清就再没机会了。
		if cleaned, skipped := sweepRecordedWorkspaces(a, ws); cleaned > 0 || skipped > 0 {
			fmt.Printf("  ✓ %-10s 其它工作区：清理 %d 个残留文件；%d 个工作区已不存在而跳过（账本保留，路径重现仍会回访）\n",
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
		kind := "hook only"
		if a.RulesDirAbs(ws) != "" {
			kind = "hook + synced files"
		}
		fmt.Printf("  - %s (%s)\n", a.ID, kind)
	}
	if others := state.Others(ws); len(others) > 0 {
		fmt.Printf("  另外，钩子是装在 user 级 host 配置里的（全局生效），本次卸载后不再有 sync 触发。\n")
		fmt.Printf("  因此还会按账本回访以下曾同步过的 %d 个工作区，一并清理残留文件：\n", len(others))
		for _, w := range others {
			fmt.Printf("    - %s\n", w)
		}
		fmt.Printf("  （只删 .rulemux__ 前缀的我方文件，你的源文件与其它文件一律不碰）\n")
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
// 钩子此刻已被全局移除，若不做这一步，那些工作区里的 .rulemux__* 将再无机会被删除。
//
// 返回：清理掉的文件数、因工作区已不存在而跳过的工作区数。
// 说明：即便路径已不存在也不从账本剔除——万一该工作区日后重现（如重新 clone），
// 下一次卸载仍会回访并清理它。
func sweepRecordedWorkspaces(a agents.Agent, current string) (cleaned, skipped int) {
	if a.RulesDir == "" {
		return 0, 0 // Tier-2 磁盘上不留任何文件，无需回访
	}
	for _, ws := range state.Others(current) {
		if _, err := os.Stat(ws); err != nil {
			skipped++
			continue // 工作区已被删除，本次无从清理
		}
		removed, err := removeRuleFiles(a.RulesDirAbs(ws))
		if err != nil {
			continue
		}
		cleaned += len(removed)
	}
	return cleaned, skipped
}

// removeRuleFiles deletes every .rulemux__* file in dir (Tier-1 leftovers).
func removeRuleFiles(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, engine.Prefix+"*"))
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			return removed, err
		}
		removed = append(removed, filepath.Base(m))
	}
	return removed, nil
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
    2. deletes the synced .rulemux__* files from that agent's rules dir
       (Tier-1 agents only; Tier-2 agents like codex/opencode keep nothing on disk).

CONFIRMATION:
  Before anything is removed, rulemux lists the targets and asks "Proceed? [y/N]".
  Type y or yes to continue; anything else (or EOF on non-interactive stdin)
  cancels safely.
  Pass --yes to skip the prompt (for scripts / CI).

Supported (verified) ids: codebuddy, workbuddy.
Other registered agents (claude / trae / codex / opencode) are NOT yet verified and
cannot be installed or removed until their adapter is canary-tested.

OTHER FLAGS:
  --workspace <dir>   Workspace whose synced .rulemux__* files should be
                      removed (default: current dir). NOTE: the SessionStart
                      hook itself lives in the user-level host config
                      (~/.codebuddy/settings.json), so removing it takes
                      effect globally regardless of --workspace.
  --yes               Skip the confirmation prompt.
  --help, -h          Show this help.

NOTE: this only removes rulemux's own artifacts. Your agent's other
configuration and your own (non-.rulemux__) rule files are never touched.
`)
}
