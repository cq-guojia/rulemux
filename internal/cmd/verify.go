package cmd

import (
	"fmt"
	"os"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/engine"
)

// Verify is the canary acceptance check: drop a probe file carrying a secret
// token into the rules dir, then have the user open a NEW session and ask the
// agent to recite it. That proves the "SessionStart copy -> loaded in this
// session" chain end to end.
//
// It validates two things at once (docs/design/implementation.md #13):
//  1. the hook really fires before rules are read (otherwise this session can't see it)
//  2. the agent really loads our probe file: a non-hidden __rulemux__ name, carrying an
//     alwaysApply:true frontmatter so frontmatter-gated agents (CodeBuddy/WorkBuddy) pick it up
func Verify(args []string) int {
	f := ParseFlags(args)
	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}

	requested := f.Get("agent", "")
	targets := targetAgents(requested)
	if len(targets) == 0 {
		fmt.Fprintf(os.Stderr, "rulemux: unknown agent %q\n", requested)
		return 1
	}
	// With no explicit agent, only Tier-1 (Tier-2 has no rules dir to drop a probe into).
	if requested == "" {
		var onlyTier1 []agents.Agent
		for _, a := range targets {
			if a.Tier == agents.Tier1 {
				onlyTier1 = append(onlyTier1, a)
			}
		}
		targets = onlyTier1
	}

	// Cleanup mode
	if f.Has("clean") {
		for _, a := range targets {
			dir := a.RulesDirAbs(ws)
			if dir == "" {
				continue
			}
			if err := engine.CleanCanary(dir); err != nil {
				fmt.Fprintf(os.Stderr, "rulemux: failed to clean canary for %s: %v\n", a.ID, err)
				return 1
			}
			fmt.Printf("Canary cleaned: %s\n", a.ID)
		}
		return 0
	}

	token := engine.CanaryToken()
	fmt.Println("rulemux verify — canary acceptance")
	fmt.Println("Token:", token)
	fmt.Println()
	for _, a := range targets {
		dir := a.RulesDirAbs(ws)
		if dir == "" {
			fmt.Printf("  %s: Tier-2 has no rules dir; ask it directly in a session whether the rules took effect\n", a.ID)
			continue
		}
		p, err := engine.WriteCanary(dir, token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: failed to write canary: %v\n", a.ID, err)
			return 1
		}
		fmt.Printf("  %s: wrote %s\n", a.ID, p)
	}

	fmt.Println()
	fmt.Println("Open a NEW session for each agent and ask it:")
	fmt.Printf("  \"Can you see this token in your rules: %s\"\n", token)
	fmt.Println()
	fmt.Println("How to read the result (docs/design/features/verification.md):")
	fmt.Println("  · Recites it ⇒ the chain works: the SessionStart copy took effect in this session,")
	fmt.Println("    and the agent loaded rulemux's probe file")
	fmt.Println("  · Cannot ⇒ check first: is the hook installed? did you open a NEW session? If it still")
	fmt.Println("    fails, the agent may gate loading on file shape — hidden dotfiles are skipped, and some")
	fmt.Println("    agents (e.g. CodeBuddy) require an alwaysApply:true frontmatter. Adjust the adapter's")
	fmt.Println("    Prefix / NeedsFrontmatter accordingly.")
	fmt.Println()
	fmt.Println("Clean up when done: rulemux verify --clean")
	fmt.Println("(the canary carries the __rulemux__ prefix, so even if you skip this, the next")
	fmt.Println("rulemux sync treats it as residue and deletes it automatically)")
	return 0
}
