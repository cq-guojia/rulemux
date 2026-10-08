package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/hooks"
)

// exampleConfig 是 init 生成的示例配置内容。
const exampleConfig = `# rulemux 配置
#
# path      = 源文件路径（磁盘任意位置）。单个字符串或数组都行。
# agents    = 投递给哪些 agent；省略 = 全部。
# workspace = 适用于哪些工作区；单个字符串或数组都行，省略 / "*" / "all" = 所有工作区。

[[source]]
path = ["C:/rules/team-conventions.md", "C:/rules/style.md"]
agents = ["claude", "codebuddy", "trae"]
# workspace 省略 = 所有工作区都适用

[[source]]
path = "D:/notes/project-a.txt"
agents = ["codex"]
workspace = ["/path/to/proj-a", "/path/to/proj-b"]

# agents 取值：
#   Tier-1（真实拷贝进规则目录）：claude / codebuddy / workbuddy / trae
#   Tier-2（SessionStart 注入，不碰用户文件）：codex / opencode
#   trae 的 CN 版与国际版是同一套机制，统一写 trae
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
		fmt.Println("  ⚠ change the 'path' fields to your real rule files")
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

SUPPORTED AGENTS (value of --agent):
  claude      Claude Code     Tier-1  .claude/rules/
  codebuddy   CodeBuddy      Tier-1  .codebuddy/rules/
  workbuddy   WorkBuddy      Tier-1  .codebuddy/rules/ (shared with CodeBuddy)
  trae        Trae           Tier-1  .trae/rules/
  codex       Codex          Tier-2  SessionStart injection (does not touch AGENTS.md)
  opencode    OpenCode       Tier-2  SessionStart injection (does not touch AGENTS.md)

Install several at once, comma-separated:
  rulemux init --agent codebuddy,codex

HOOKS ARE PER-WORKSPACE. Run init inside each workspace you want covered.

OTHER FLAGS:
  --config <path>     Config file (default ~/.rulemux/config.toml).
  --workspace <dir>   Workspace to install hooks into (default: current dir).
  --help, -h          Show this help.

The first run also writes a sample ~/.rulemux/config.toml if none exists.
Edit its 'path' fields to point at your own rule files, then open a new session
in the agent to trigger the sync.
`)
}
