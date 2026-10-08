package cmd

import (
	"fmt"
	"os"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/engine"
)

// Inject prints the rendered rules to stdout so a Tier-2 agent's SessionStart hook
// can feed them into the context.
//
// Usage: rulemux inject --agent codex
// Tier-2 agents (single-file AGENTS.md only) have no directory to drop files into,
// hence injection — and your own AGENTS.md is never touched.
func Inject(args []string) int {
	f := ParseFlags(args)
	id := f.Get("agent", "")
	if id == "" {
		fmt.Fprintln(os.Stderr, "rulemux: inject requires --agent <id>")
		return 2
	}
	a, ok := agents.Get(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "rulemux: unknown agent %q\n", id)
		return 2
	}
	if !a.Verified {
		fmt.Fprintf(os.Stderr, "rulemux: agent %q is not supported yet: only verified agents are available: %s\n", id, agents.SupportedSummary())
		return 2
	}

	cfg, err := config.Load(f.Get("config", config.DefaultPath()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: failed to read config: %v\n", err)
		return 1
	}

	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}

	srcs := cfg.SourcesFor(a.ID, ws)
	if len(srcs) == 0 {
		// Nothing for this agent: exit quietly rather than injecting empty content.
		return 0
	}
	out, err := engine.RenderInject(srcs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: failed to render injection content: %v\n", err)
		return 1
	}
	fmt.Print(out)
	return 0
}
