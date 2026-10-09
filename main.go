// Command rulemux 把用户自己维护的规则文件，投递到各家 AI coding agent 的原生规则目录。
//
// 设计要点（docs/design/implementation.md）：
//   - 无守护进程、不监听文件改动：由各 agent 的 SessionStart 钩子调起本程序。
//   - Tier-1（读整个规则文件夹的 agent）：真实拷贝，文件名加前缀 __rulemux__。
//   - Tier-2（只认单文件 AGENTS.md 的 agent）：走钩子注入，不碰用户自己的 AGENTS.md。
package main

import (
	"fmt"
	"os"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/cmd"
)

// version is the rulemux version.
// It is a var (not a const) so the release pipeline can inject the real tag:
//
//	go build -ldflags="-X main.version=1.2.3"
var version = "0.2.3"

// usage prints the top-level help.
//
// The agent list is generated from registry.Supported() (i.e. Verified adapters only)
// so the help can never advertise an adapter that is not installable yet.
func usage() {
	fmt.Fprint(os.Stderr, `rulemux - one set of rules, delivered to every AI coding agent

USAGE:
  rulemux sync      [--hook] [--agent <id>] [--config <path>] [--workspace <dir>]
                                                                          Sync rules (--hook = invoked by an agent's SessionStart hook)
  rulemux sync --all [--config <path>]                                    Sync every workspace recorded in the ledger (never reads cwd)
  rulemux inject    --agent <id>   [--config <path>]                       Tier-2: print rules to stdout for hook injection
  rulemux init      --agent <id[,id...]> [--config <path>] [--workspace <dir>]
                                                                          Generate a sample config + install SessionStart hooks
  rulemux init      --refresh [--agent <id[,id...]>]                        Refresh already-installed hooks to the current format
  rulemux doctor    [--config <path>] [--workspace <dir>]                  Environment self-check
  rulemux verify    --agent <id> [--clean] [--workspace <dir>]             Canary acceptance test
  rulemux uninstall --agent <id[,id...]> | --off | --all [--yes]           Remove rulemux hooks and synced files

`)
	fmt.Fprint(os.Stderr, "SUPPORTED AGENTS (only these can be installed):\n")
	for _, a := range agents.Supported() {
		tier := "Tier-1"
		if a.Tier == agents.Tier2 {
			tier = "Tier-2"
		}
		target := a.RulesDir
		if target == "" {
			target = "inject (your AGENTS.md is never touched)"
		}
		fmt.Fprintf(os.Stderr, "  %-11s %-7s %s\n", a.ID, tier, target)
	}
	fmt.Fprintf(os.Stderr, `
NOTE: other agents may already be registered but are NOT yet enabled; they are
omitted here until their adapter passes canary verification.

OTHER:
  --version   Print version
  --help      Print this help
`)
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	switch args[0] {
	case "--version", "-v", "version":
		fmt.Println("rulemux", version)
		return
	case "--help", "-h", "help":
		usage()
		return
	case "sync":
		os.Exit(cmd.Sync(args[1:]))
	case "inject":
		os.Exit(cmd.Inject(args[1:]))
	case "init":
		os.Exit(cmd.Init(args[1:]))
	case "doctor":
		os.Exit(cmd.Doctor(args[1:]))
	case "verify":
		os.Exit(cmd.Verify(args[1:]))
	case "uninstall":
		os.Exit(cmd.Uninstall(args[1:]))
	default:
		fmt.Fprintf(os.Stderr, "rulemux: unknown subcommand %q\n\n", args[0])
		usage()
		os.Exit(2)
	}
}
