package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cq-guojia/rulemux/internal/agents"
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

// Init 生成示例配置，并为各 agent 安装 SessionStart 钩子（幂等）。
func Init(args []string) int {
	f := ParseFlags(args)
	cfgPath := f.Get("config", config.DefaultPath())
	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: 无法确定工作区:", err)
		return 1
	}

	// 1. 配置：不存在则生成示例
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "rulemux: 创建配置目录失败:", err)
			return 1
		}
		if err := os.WriteFile(cfgPath, []byte(exampleConfig), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "rulemux: 写入示例配置失败:", err)
			return 1
		}
		fmt.Println("✓ 已生成示例配置:", cfgPath)
		fmt.Println("  ⚠ 请把里面的 path 改成你自己的真实规则文件路径")
	} else {
		fmt.Println("· 配置已存在，跳过:", cfgPath)
	}

	// 2. 为各 agent 安装 SessionStart 钩子
	fmt.Println("\n安装 SessionStart 钩子：")
	for _, a := range agents.All() {
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

	fmt.Println("\n说明：")
	fmt.Println("  ✓ = 规则目录/钩子落点已官方核实；⚠ = 待 canary 实测坐实")
	fmt.Println("      （见 docs/design/features/verification.md 与 external/agent-rules-dirs.md §四）")
	fmt.Println("  下一步：rulemux doctor 自检；rulemux verify 跑 canary 验收")
	return 0
}
