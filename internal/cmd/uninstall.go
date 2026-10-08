package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/engine"
	"github.com/cq-guojia/rulemux/internal/hooks"
)

// Uninstall removes rulemux's SessionStart hook (and, for Tier-1 agents, the
// synced .rulemux__* files) for the agent(s) named by --agent.
//
// With --off, or with no --agent at all, it removes everything for every agent.
func Uninstall(args []string) int {
	f := ParseFlags(args)
	if f.Has("help") || f.Has("h") {
		UninstallHelp()
		return 0
	}

	all := f.Has("off")
	agentArg := f.Get("agent", "")

	var targets []agents.Agent
	if all || agentArg == "" {
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

	scope := "all agents"
	if !all && agentArg != "" {
		scope = agentArg
	}
	fmt.Printf("rulemux uninstall (%s) in %s\n", scope, ws)

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
	}
	return 0
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
  rulemux uninstall --agent <id[,id...]> [--workspace <dir>]
  rulemux uninstall --off                 [--workspace <dir>]
  rulemux uninstall                       [--workspace <dir>]

WHAT IT DOES:
  For each target agent, it:
    1. removes rulemux's SessionStart hook from that agent's hook config;
    2. deletes the synced .rulemux__* files from that agent's rules dir
       (Tier-1 agents only; Tier-2 agents like codex/opencode keep nothing on disk).

WHICH AGENTS:
  --agent <id[,id...]>   Remove only the named agent(s), e.g. --agent codex
                          or --agent codebuddy,codex.
  --off                   Remove hooks for ALL agents.
  (no --agent)           Equivalent to --off: remove for ALL agents.

Supported ids: claude, codebuddy, workbuddy, trae, codex, opencode.

OTHER FLAGS:
  --workspace <dir>   Workspace to act in (default: current dir).
                      Hooks are per-workspace, so uninstall in the same
                      workspace you ran init in.
  --help, -h          Show this help.

NOTE: this only removes rulemux's own artifacts. Your agent's other
configuration and your own (non-.rulemux__) rule files are never touched.
`)
}
