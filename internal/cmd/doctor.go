package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
)

// Doctor 环境自检：配置是否就绪、本体是否在 PATH 上、各 agent 的规则目录与钩子安装情况。
func Doctor(args []string) int {
	f := ParseFlags(args)
	cfgPath := f.Get("config", config.DefaultPath())

	cfg, cfgErr := config.Load(cfgPath)
	var cfgWS string
	if cfg != nil {
		cfgWS = cfg.Workspace
	}
	ws, err := workspaceWith(f.Get("workspace", ""), cfgWS)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: 无法确定工作区:", err)
		return 1
	}

	fmt.Println("rulemux doctor")
	fmt.Println("工作区:", ws)

	// 1. 本体位置与 PATH（Claude Code 的 exec 形式钩子要求 rulemux 在 PATH 上）
	if self, err := os.Executable(); err == nil {
		fmt.Println("本体:", self)
	}
	if p, err := exec.LookPath("rulemux"); err == nil {
		fmt.Println("PATH 查找:", p)
	} else {
		fmt.Println("PATH 查找: ✗ 未找到 rulemux —— 钩子若用 exec 形式会失败，请确保它在 PATH 上")
	}

	// 2. 配置
	if cfgErr != nil {
		fmt.Printf("配置: ✗ %v\n", cfgErr)
		fmt.Println("  提示：先运行 rulemux init")
	} else {
		fmt.Printf("配置: ✓ %s（%d 条 source）\n", cfgPath, len(cfg.Sources))
		if cfg.Workspace != "" {
			fmt.Printf("  配置内 workspace: %s\n", cfg.Workspace)
		}
		for _, s := range cfg.Sources {
			target := "全部 agent"
			if len(s.Agents) > 0 {
				target = strings.Join(s.Agents, ", ")
			}
			for _, p := range s.Paths {
				mark := "✓"
				if _, err := os.Stat(p); err != nil {
					mark = "✗"
				}
				fmt.Printf("  %s %s → %s\n", mark, p, target)
			}
		}
		if err := cfg.Validate(); err != nil {
			fmt.Println("  ⚠", err)
		}
	}

	// 3. 各 agent
	fmt.Println("\n各 agent：")
	for _, a := range agents.All() {
		mark := "✓"
		if !a.Verified {
			mark = "⚠"
		}
		fmt.Printf("  %s %-10s %s\n", mark, a.ID, a.Tier)

		if dir := a.RulesDirAbs(ws); dir != "" {
			state := "目录不存在（首次 sync 会创建）"
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				state = "目录已存在"
			}
			fmt.Printf("      规则目录: %s（%s）\n", dir, state)
		} else {
			fmt.Println("      规则目录: 无（Tier-2，走 SessionStart 注入）")
		}

		hookPath := a.HookFileAbs(ws)
		if b, err := os.ReadFile(hookPath); err == nil && strings.Contains(string(b), "rulemux") {
			fmt.Printf("      钩子: ✓ 已安装 → %s\n", hookPath)
		} else {
			fmt.Printf("      钩子: ✗ 未安装 → %s（运行 rulemux init）\n", hookPath)
		}
		if a.Note != "" {
			fmt.Printf("      备注: %s\n", a.Note)
		}
	}

	fmt.Println("\n⚠ = 该 agent 的规则目录 / 钩子落点尚未核实，请用 rulemux verify 实测坐实。")
	return 0
}
