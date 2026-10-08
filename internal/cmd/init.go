package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/hooks"
)

// exampleConfig is the default config content written on the first run of init.
//
// Design principles (agreed with the user):
//   - the header comments explain every option, so the user only edits what they need;
//   - no active [[source]] by default (all examples commented out) — a clean slate with
//     no sample paths or test documents; nothing happens until the user uncomments an
//     example or adds their own [[source]] with real paths.
const exampleConfig = `# rulemux configuration file
#
# Default location: ~/.rulemux/config.toml (or pass --config <path> to any command)
#
# At the start of every agent session, rulemux copies the "source rule files" described
# by each [[source]] below into that agent's native rules directory, using a .rulemux__
# prefix: it cleans up its own residue and never touches your own files.
#
# Each [[source]] accepts these fields (all optional, each has a default):
#
#   path      Source file(s); may live anywhere on disk. Both forms work:
#               path = "/abs/path/to/your-rules.md"          # a single file
#               path = ["/abs/path/a.md", "/abs/path/b.md"]  # a list (shares agents/workspace below)
#
#   agents    Which agents receive this batch; omitted = every *supported* agent.
#             Current verified values (installable):
#               codebuddy   Tier-1 (really copied into .codebuddy/rules)
#               workbuddy   Tier-1 (shares the .codebuddy/rules directory)
#             Other agents (claude / trae / codex / opencode) have not passed canary
#             verification yet, so they cannot be installed or targeted for now;
#             once their adapters are proven they open up automatically — no edit needed here.
#             Example: agents = ["codebuddy"]
#
#   workspace Which workspaces these rules apply to. Forms:
#               workspace = "/abs/path/to/proj"          # single workspace (exact match)
#               workspace = ["/abs/path/proj-a", "/b"]   # list: matching any one is enough
#               workspace = "*"  /  "**"  /  "all"       # every workspace (global wildcard)
#             Standard globbing is supported: "*" is one segment, "**" recurses across
#             segments and may appear in the middle, e.g.:
#               workspace = "/abs/**/B"                 # any depth under /abs named B
#               workspace = "**/B"                      # B at any location or nesting level
#
# --- Advanced: bundle things into "groups" to avoid repetition -------------------
# Once you have many files, bundle your common files / workspaces into groups and
# reference them by name from [[source]].
#
# File group [[file_group]]: bundles rule files, referenced via groups.
#   name  Group name (required)
#   path  Files inside the group; single value or list
#   use   Reference other file groups -- [LIST, MAY BE MULTIPLE, NESTS]:
#           use = ["dev"]            # one group
#           use = ["dev", "qa"]      # several: reuse all of them at once
#           # group A pulls in group B plus its own files => A == B's files + A's files
#
# Workspace group [[workspace_group]]: bundles workspaces, referenced via workspace_groups.
#   name       Group name (required)
#   workspace  Workspaces inside the group; single value or list, globs allowed
#   use        Reference other workspace groups -- [LIST, MAY BE MULTIPLE, NESTS]:
#           use = ["dev"]            # one group
#           use = ["dev", "qa"]      # several: reuse all of them at once
#
# Inside a [[source]]:
#   groups            = ["gA", "gB"]     # file groups (list, may be multiple; expands to their files)
#   workspace_groups  = ["gX", "gY"]     # workspace groups (list, may be multiple)
#   [MIXING IS ALLOWED] groups can be combined with path, and workspace_groups with workspace:
#       groups = ["base", "proj"]            # use every file in the base and proj groups
#       path   = ["/abs/path/extra.md"]      # ...and additionally this one file
#     => this source ships files of base + files of proj + extra.md, deduplicated by value.
#   (workspace_groups / workspace work the same way: reference a group and also list a
#    standalone workspace at the same time.)
#   Duplicates across groups are harmless -- everything is deduplicated by value at the end,
#   so each file is processed exactly once.
#   File groups and workspace groups are separate namespaces; identical names are allowed.
#
# Everything below is commented out and inactive. Uncomment what you need and replace
# the placeholders with your real values.

# [[file_group]]
# name = "base"
# path = ["/abs/path/to/team-conventions.md", "/abs/path/to/style.md"]

# [[file_group]]
# name = "proj"
# use = ["base"]                       # nests the base group (list, may be multiple: use = ["base", "dev"])
# path = ["/abs/path/to/project-a.md"]

# [[workspace_group]]
# name = "dev"
# workspace = ["/abs/path/to/proj-1", "/abs/path/to/proj-2"]

# [[workspace_group]]
# name = "qa"
# use = ["dev"]                        # reuses the dev group (list, may be multiple)
# workspace = ["/abs/path/to/proj-3"]

# [[source]]
# groups = ["base", "proj"]            # file groups (list, may be multiple; expands to their files)
# path = ["/abs/path/to/extra.md"]     # plus a standalone file: this source = base + proj + this file
# agents = ["codebuddy"]
# workspace_groups = ["dev"]          # workspace groups (list, may be multiple; expands to their workspaces)
# workspace = ["/abs/path/to/standalone"]   # plus a standalone workspace
`

// Init installs the SessionStart hook for the agent(s) named by --agent.
//
// --agent is REQUIRED. rulemux never scans the machine for installed agents;
// you must explicitly say which agent(s) you want. Auto-detecting which agents
// exist locally is unreliable, so the choice is always the user's.
func Init(args []string) int {
	f := ParseFlags(args)
	if f.Has("help") || f.Has("h") {
		InitHelp()
		return 0
	}

	agentArg := f.Get("agent", "")
	if agentArg == "" {
		fmt.Fprintln(os.Stderr, "rulemux init: --agent is required")
		fmt.Fprintln(os.Stderr, "  rulemux does NOT auto-detect agents installed on your machine.")
		fmt.Fprintln(os.Stderr, "  name the agent(s) explicitly, e.g. --agent codebuddy")
		fmt.Fprintln(os.Stderr, "  run 'rulemux init --help' for details")
		return 2
	}
	targets, err := parseAgents(agentArg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux init:", err)
		return 2
	}

	cfgPath := f.Get("config", config.DefaultPath())
	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}

	// 1. config: generate a sample only if missing
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "rulemux: cannot create config dir:", err)
			return 1
		}
		if err := os.WriteFile(cfgPath, []byte(exampleConfig), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "rulemux: cannot write sample config:", err)
			return 1
		}
		fmt.Println("✓ sample config created:", cfgPath)
		fmt.Println("  ⚠ no active [[source]] yet; uncomment an example or add your own [[source]] with real rule file paths")
	} else {
		fmt.Println("· config exists, skipped:", cfgPath)
	}

	// 2. install the SessionStart hook for the requested agents only
	fmt.Println("\nInstalling SessionStart hooks:")
	for _, a := range targets {
		p, err := hooks.Install(a, ws)
		if err != nil {
			fmt.Printf("  ✗ %-10s %v\n", a.ID, err)
			continue
		}
		mark := "✓"
		if !a.Verified {
			mark = "⚠"
		}
		fmt.Printf("  %s %-10s %s → rulemux %s --agent %s\n", mark, a.ID, p, hooks.SubcommandFor(a), a.ID)
	}

	fmt.Println("\nLegend:")
	fmt.Println("  ✓ = rules dir / hook path verified;  ⚠ = pending canary verification")
	fmt.Println("  Next: rulemux doctor  (self-check)    rulemux verify  (canary acceptance)")
	return 0
}

// InitHelp prints detailed English help for `rulemux init`.
func InitHelp() {
	fmt.Print(`rulemux init - install the SessionStart hook for one or more agents

USAGE:
  rulemux init --agent <id[,id...]> [--config <path>] [--workspace <dir>]

--agent is REQUIRED. rulemux never scans your machine for installed agents;
you must name the agent(s) you want. This is deliberate: auto-detecting which
agents exist locally is unreliable, so the choice is always yours.

SUPPORTED AGENTS (value of --agent; only VERIFIED agents can be installed):
  codebuddy   CodeBuddy   Tier-1  .codebuddy/rules/   [verified — installable]
  workbuddy   WorkBuddy   Tier-1  .codebuddy/rules/   [verified — shares CodeBuddy dir]

The following are registered but NOT YET verified, so init refuses to install them
until their adapter is canary-tested and flipped to verified:
  claude, trae, codex, opencode

Install (only verified agents), comma-separated:
  rulemux init --agent codebuddy

The SessionStart hook is installed once into the user-level host config
(~/.codebuddy/settings.json), so it fires for every workspace you open —
you do NOT need to run init once per workspace. Run it once per machine.
Syncing still keys off the current workspace (cwd) to decide which rules apply.

OTHER FLAGS:
  --config <path>     Config file (default ~/.rulemux/config.toml).
  --workspace <dir>   Workspace to install hooks into (default: current dir).
  --help, -h          Show this help.

The first run also writes a sample ~/.rulemux/config.toml if none exists.
It contains only commented-out examples and a header explaining every option;
nothing is synced until you uncomment an example or add your own [[source]] with
real 'path' values. Then open a new session in the agent to trigger the sync.
`)
}
