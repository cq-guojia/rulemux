package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/hooks"
)

// exampleConfig 是 init 首次运行时生成的默认配置内容。
//
// 设计原则（与用户约定一致）：
//   - 顶部用注释把每一项配置方式都写清楚，用户照着改即可；
//   - 默认没有任何生效的 [[source]]（示例整段注释掉），即“空 / 只有初始配置”状态，
//     不塞任何示例路径或测试文档；用户自行取消注释或新增 [[source]] 后才会有动作。
const exampleConfig = `# rulemux 配置文件
#
# 默认位置：~/.rulemux/config.toml（也可用各命令的 --config <path> 指定别的路径）
#
# rulemux 会在 agent 会话开始时，把下面每条 [[source]] 描述的“源规则文件”
# 真实拷贝进对应 agent 的原生规则目录（带 .rulemux__ 前缀，删残留、不碰你的文件）。
#
# 每条 [[source]] 支持三个字段（都可选，省略有默认值）：
#
#   path      源文件路径，磁盘任意位置均可。两种写法都支持：
#               path = "/abs/path/to/your-rules.md"          # 单个文件
#               path = ["/abs/path/a.md", "/abs/path/b.md"]  # 数组（同批文件共享下面的 agents / workspace）
#
#   agents    这批文件投递给哪些 agent；省略 = 投递给全部「已支持」的 agent。取值（当前已验证、可安装）：
#               codebuddy   Tier-1（真实拷贝进 .codebuddy/rules）
#               workbuddy   Tier-1（与 codebuddy 共用目录）
#             其余 agent（claude / trae / codex / opencode）尚未坐实验证，暂不允许安装或投递；
#             等对应适配坐实、被标记为 verified 后，安装程序会自动开放，无需改这里。
#             示例：agents = ["codebuddy"]
#
#   workspace 这批规则适用于哪些工作区，写法：
#               workspace = "/abs/path/to/proj"          # 单个工作区（完全匹配）
#               workspace = ["/abs/path/proj-a", "/b"]   # 数组，命中其一即可
#               workspace = "*"  /  "**"  /  "all"       # 所有工作区（全局通配）
#               支持业界标准 glob："*" 单段、"**" 跨段递归，可出现在中间，例如：
#               workspace = "/abs/**/B"                 # 匹配 /abs 下任意深度的名为 B 的工作区
#               workspace = "**/B"                      # 匹配任意位置、任意层级名为 B 的工作区
#
# ── 进阶：用“组”打包，减少重复 ─────────────────────────────────────────────
# 文件多了之后，可以把常用文件 / 工作区打包成“组”，在 [[source]] 里用组名引用。
#
# 文件组 [[file_group]]：把若干规则文件打包，供 [[source]] 用 groups 引用。
#   name  组名（必填）
#   path  组内文件；单个或数组写法均可
#   use   引用其它文件组——【数组，可写多个，支持嵌套】：
#           use = ["dev"]            # 单个
#           use = ["dev", "qa"]      # 多个：同时复用这几个组
#           # A 组引入 B 组，再加自己的文件——A 最终 = B 的文件 + A 的文件
#
# 工作区分组 [[workspace_group]]：把若干工作区打包，供 [[source]] 用 workspace_groups 引用。
#   name       组名（必填）
#   workspace  组内工作区；单个或数组写法均可，支持上面的 glob
#   use        引用其它工作区分组——【数组，可写多个，支持嵌套】：
#           use = ["dev"]            # 单个
#           use = ["dev", "qa"]      # 多个：同时复用这几个工作区分组
#
# 在 [[source]] 里：
#   groups            = ["组A", "组B"]   # 引用文件组（数组，可写多个；自动展开为组内所有文件）
#   workspace_groups  = ["组X", "组Y"]   # 引用工作区分组（数组，可写多个）
#   【可混合书写】groups 与 path 能同时写、workspace_groups 与 workspace 也能同时写：
#       groups = ["base", "proj"]            # 我用了 base、proj 这两个组里的所有文件
#       path   = ["/abs/path/extra.md"]      # 另外再单独指定这一个文件
#     ⇒ 这条 source 投递的 = base 组文件 + proj 组文件 + extra.md；三者合并后按值去重。
#   （workspace_groups / workspace 同理：既可引用分组，也可同时单列单个工作区。）
#   组间重复的文件 / 工作区无所谓——程序最后按值去重，每个只做一次。
#   文件组与工作区分组是两套独立的命名空间，允许同名（它们分别位于不同的区域）。
#
# 下面都是示例，整段被注释掉、不会生效。需要哪个就去掉前面的 #，并改成你的真实值。

# [[file_group]]
# name = "base"
# path = ["/abs/path/to/team-conventions.md", "/abs/path/to/style.md"]

# [[file_group]]
# name = "proj"
# use = ["base"]                       # 嵌套引用 base 组（数组，可写多个：use = ["base", "dev"]）
# path = ["/abs/path/to/project-a.md"]

# [[workspace_group]]
# name = "dev"
# workspace = ["/abs/path/to/proj-1", "/abs/path/to/proj-2"]

# [[workspace_group]]
# name = "qa"
# use = ["dev"]                        # 复用 dev 组（数组，可写多个）
# workspace = ["/abs/path/to/proj-3"]

# [[source]]
# groups = ["base", "proj"]            # 引用文件组（数组，可多个；自动展开成组内所有文件）
# path = ["/abs/path/to/extra.md"]     # 同时混列单个文件：这条 source = base+proj 组文件 + 该文件
# agents = ["codebuddy"]
# workspace_groups = ["dev"]          # 引用工作区分组（数组，可多个；展开成组内所有工作区）
# workspace = ["/abs/path/to/standalone"]   # 同时混列单个工作区
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
		fmt.Println("  ⚠ 默认没有任何生效的 [[source]]；取消注释示例或新增 [[source]] 并填好你的规则文件路径")
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

HOOKS ARE PER-WORKSPACE. Run init inside each workspace you want covered.

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
