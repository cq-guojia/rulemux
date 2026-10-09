package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cq-guojia/rulemux/internal/agents"
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
# by each [[source]] below into that agent's native rules directory, using a __rulemux__
# prefix: it cleans up its own residue and never touches your own files.
#
# Each [[source]] accepts these fields (all optional, each has a default):
#
#   path      Source file(s); may live anywhere on disk. Both forms work:
#               path = "/abs/path/to/your-rules.md"          # a single file
#               path = ["/abs/path/a.md", "/abs/path/b.md"]  # a list (shares agents/workspace below)
#             A leading "~" or "~/" is expanded to your home directory, so the same
#             config works on every machine without hardcoding a username:
#               path = "~/Agent.Workspace/00.RULES/base.md"
#
#   agents    Which agents receive this batch; omitted = every *supported* agent.
#             Current verified values (installable):
#               workbuddy   Tier-1 (own dir .workbuddy/rules; own hook ~/.workbuddy/settings.json) — use this for WorkBuddy
#               codebuddy   Tier-1 (.codebuddy/rules; ~/.codebuddy/settings.json)
#               trae        Tier-1 (.trae/rules; CN ~/.trae-cn/hooks.json, intl ~/.trae) — verified 2026-10-09
#             These are SEPARATE agents, each with its own config dir and hook file;
#             install whichever you actually use (or all). rulemux remembers the name
#             you chose and keeps it on every later refresh.
#             Other agents (claude / codex / opencode) have not passed canary
#             verification yet, so they cannot be installed or targeted for now;
#             once their adapters are proven they open up automatically — no edit needed here.
#             Example: agents = ["workbuddy", "trae"]
#
#   workspace Which workspaces these rules apply to. Forms:
#               workspace = "/abs/path/to/proj"          # single workspace (exact match)
#               workspace = ["/abs/path/proj-a", "/b"]   # list: matching any one is enough
#               workspace = "*"  /  "**"  /  "all"       # every workspace (global wildcard)
#             Standard globbing is supported: "*" is one segment, "**" recurses across
#             segments and may appear in the middle, e.g.:
#               workspace = "/abs/**/B"                 # any depth under /abs named B
#               workspace = "**/B"                      # B at any location or nesting level
#             A leading "~" / "~/" is expanded to your home directory here as well,
#             and still works with globs:
#               workspace = "~/Agent.Workspace/*"
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

// Init 有两个模式：
//
//   - 默认（安装）：为 --agent 指定的 agent 装 SessionStart 钩子，允许创建配置目录与文件
//     —— 这是用户的显式动作。
//   - --refresh（刷新）：只把「已存在的自家钩子条目」升级到当前格式。不创建任何文件或目录，
//     未装过的一律跳过；别人的钩子条目与文件里其它键原样保留。**主路径是 sync 的自愈**
//     （见 healHooks）；这条命令供手动 / 诊断使用（见 design「升级即生效」）。
//
// --agent is REQUIRED in install mode. rulemux never scans the machine for installed
// agents; you must explicitly say which agent(s) you want.
func Init(args []string) int {
	f := ParseFlags(args)
	if f.Has("help") || f.Has("h") {
		InitHelp()
		return 0
	}

	agentArg := f.Get("agent", "")
	if f.Has("refresh") {
		return refreshHooks(agentArg, f)
	}
	if agentArg == "" {
		fmt.Fprintln(os.Stderr, "rulemux init: --agent is required")
		fmt.Fprintln(os.Stderr, "  rulemux does NOT auto-detect agents installed on your machine.")
		fmt.Fprintln(os.Stderr, "  name the agent(s) explicitly, e.g. --agent codebuddy")
		fmt.Fprintln(os.Stderr, "  run 'rulemux init --help' for details")
		return 2
	}
	targets, err := parseAgentsDisplay(agentArg)
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
	for _, ar := range targets {
		p, err := hooks.Install(ar.Agent, ws, ar.Display)
		if err != nil {
			fmt.Printf("  ✗ %-10s %v\n", ar.Agent.ID, err)
			continue
		}
		mark := "✓"
		if !ar.Agent.Verified {
			mark = "⚠"
		}
		// 回显用用户写的标识（如 workbuddy），让用户面与安装命令一致。
		fmt.Printf("  %s %-10s %s → rulemux %s --agent %s\n", mark, ar.Display, p, hooks.SubcommandFor(ar.Agent), ar.Display)
	}

	fmt.Println("\nLegend:")
	fmt.Println("  ✓ = rules dir / hook path verified;  ⚠ = pending canary verification")
	fmt.Println("  Next: rulemux doctor  (self-check)    rulemux verify  (canary acceptance)")
	return 0
}

// refreshHooks 刷新「已装」的自家钩子条目：未装的不创建，别人的条目不动。
//
// 永远返回 0（个别 agent 读不了 / 写不了只打警告并继续）—— 它也可能被脚本批量调用，
// 绝不能因为个别 agent 失败就中断。
func refreshHooks(agentArg string, f *Flags) int {
	var targets []agents.Agent
	if agentArg == "" {
		// 只遍历「已核实」的 agent（与 sync、安装的范围一致）：未核实的不去读它们的配置，
		// 既省几次无意义的文件读取，也不会刷出"登记了但没实现"的噪音行。
		targets = agents.Supported()
	} else {
		ts, err := parseAgents(agentArg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rulemux init --refresh:", err)
			return 2
		}
		targets = ts
	}

	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}

	fmt.Println("Refreshing SessionStart hooks (existing entries only):")
	refreshed := 0
	for _, a := range targets {
		path, envUsed := a.HookFileAbsWithSource(ws)
		src := ""
		if envUsed != "" {
			src = fmt.Sprintf(" [dir from $%s]", envUsed)
		}

		installed, cmd, err := hooks.Inspect(path, a)
		switch {
		case errors.Is(err, hooks.ErrRefreshUnsupported):
			fmt.Printf("  · %-10s skipped: hook config format not verified yet%s\n", a.ID, src)
			continue
		case err != nil:
			fmt.Printf("  ⚠ %-10s unreadable, left untouched%s: %v\n", a.ID, src, err)
			continue
		case !installed:
			fmt.Printf("  · %-10s not installed, skipped%s\n", a.ID, src)
			continue
		}

		want := hooks.ExpectedCommand(a, cmd)
		if cmd == want {
			fmt.Printf("  ✓ %-10s already up to date%s\n", a.ID, src)
			continue
		}
		// 只重写我们自己那一条（含旧格式与 `--agent workbuddy` 这类别名写法）。
		changed, err := hooks.Refresh(path, a)
		if err != nil {
			fmt.Printf("  ⚠ %-10s refresh failed, left untouched: %v\n", a.ID, err)
			continue
		}
		if changed {
			refreshed++
			fmt.Printf("  ✓ %-10s refreshed: %q → %q%s\n", a.ID, cmd, want, src)
		}
	}
	fmt.Printf("refreshed %d hook(s). Nothing else was touched: other tools' hooks and all other keys are preserved.\n", refreshed)
	return 0
}

// InitHelp prints detailed English help for `rulemux init`.
func InitHelp() {
	fmt.Print(`rulemux init - install the SessionStart hook for one or more agents

USAGE:
  rulemux init --agent <id[,id...]> [--config <path>] [--workspace <dir>]
  rulemux init --refresh [--agent <id[,id...]>] [--workspace <dir>]

--agent is REQUIRED in install mode. rulemux never scans your machine for installed agents;
you must name the agent(s) you want. This is deliberate: auto-detecting which
agents exist locally is unreliable, so the choice is always yours.

SUPPORTED AGENTS (value of --agent; only VERIFIED agents can be installed):
  workbuddy  WorkBuddy   Tier-1  .workbuddy/rules/   [verified — install this one]
              use this if you are setting up WorkBuddy:
                rulemux init --agent workbuddy
              (separate from codebuddy: own rules dir and own hook file
               ~/.workbuddy/settings.json)
  codebuddy  CodeBuddy   Tier-1  .codebuddy/rules/   [verified]
              rulemux init --agent codebuddy
  trae       Trae (CN)   Tier-1  .trae/rules/        [verified 2026-10-09]
              CN hook ~/.trae-cn/hooks.json; intl ~/.trae/hooks.json
              rulemux init --agent trae

The following are registered but NOT YET verified, so init refuses to install them
until their adapter is canary-tested and flipped to verified:
  claude, codex, opencode

Install (only verified agents), comma-separated:
  rulemux init --agent workbuddy
  rulemux init --agent trae

The SessionStart hook is installed once into the user-level host config
(~/.workbuddy/settings.json for WorkBuddy, ~/.codebuddy/settings.json for
CodeBuddy, ~/.trae-cn/hooks.json for Trae CN), so it fires for every workspace
you open — you do NOT need to run init once per workspace. Run it once per machine.
Syncing still keys off the current workspace (cwd) to decide which rules apply.

REFRESH (the upgrade path — refreshes, never installs):
  rulemux init --refresh
      Rewrites ONLY the SessionStart hooks rulemux itself already installed, so an
      upgraded binary brings old hook commands up to date. It never creates a config
      file or directory, never adds a hook that was not there before, and preserves
      every other entry and every other key in the host config. Agents that were
      never installed are skipped. Omitting --agent checks every supported agent.
      rulemux usually does this by itself: the first sync after an upgrade rewrites
      its own stale hook, with no npm install scripts involved. This command is for
      doing it on demand.

OTHER FLAGS:
  --refresh           Refresh existing rulemux hooks only (see above).
  --config <path>     Config file (default ~/.rulemux/config.toml).
  --workspace <dir>   Workspace to install hooks into (default: current dir).
  --help, -h          Show this help.

The first run also writes a sample ~/.rulemux/config.toml if none exists.
It contains only commented-out examples and a header explaining every option;
nothing is synced until you uncomment an example or add your own [[source]] with
real 'path' values. Then open a new session in the agent to trigger the sync.
`)
}
