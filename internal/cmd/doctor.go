package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/hooks"
)

// Doctor 环境自检：配置是否就绪、本体是否在 PATH 上、各 agent 的规则目录与钩子安装情况。
func Doctor(args []string) int {
	f := ParseFlags(args)
	cfgPath := f.Get("config", config.DefaultPath())

	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}

	cfg, cfgErr := config.Load(cfgPath)

	fmt.Println("rulemux doctor")
	fmt.Println("Workspace:", ws)

	// 1. Binary location and PATH (Claude Code's exec-style hook requires rulemux on PATH)
	if self, err := os.Executable(); err == nil {
		fmt.Println("Binary:", self)
	}
	if p, err := exec.LookPath("rulemux"); err == nil {
		fmt.Println("PATH lookup:", p)
	} else {
		fmt.Println("PATH lookup: ✗ rulemux not found on PATH — exec-style hooks will fail; make sure it is installed there")
	}

	// 2. Config
	if cfgErr != nil {
		fmt.Printf("Config: ✗ %v\n", cfgErr)
		fmt.Println("  Hint: run rulemux init first")
	} else {
		fmt.Printf("Config: ✓ %s (%d source(s))\n", cfgPath, len(cfg.Sources))
		for _, s := range cfg.Sources {
			target := "all agents"
			if len(s.Agents) > 0 {
				target = strings.Join(s.Agents, ", ")
			}
			scope := "all workspaces"
			if len(s.Workspaces) > 0 {
				scope = strings.Join(s.Workspaces, ", ")
			}
			skip := ""
			if !s.MatchesWorkspace(ws) {
				skip = " ← skipped: not the current workspace"
			}
			for _, p := range s.Paths {
				mark := "✓"
				if _, err := os.Stat(p); err != nil {
					mark = "✗"
				}
				fmt.Printf("  %s %s → %s [%s]%s\n", mark, p, target, scope, skip)
			}
		}
		if err := cfg.Validate(); err != nil {
			fmt.Println("  ⚠", err)
		}
	}

	// 3. Each agent
	fmt.Println("\nAgents:")
	for _, a := range agents.All() {
		mark := "✓"
		if !a.Verified {
			mark = "⚠"
		}
		fmt.Printf("  %s %-10s %s\n", mark, a.ID, a.Tier)

		if dir := a.RulesDirAbs(ws); dir != "" {
			state := "directory does not exist (only this agent's own session hook creates it)"
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				state = "directory exists"
			}
			fmt.Printf("      Rules dir: %s (%s)\n", dir, state)
		} else {
			fmt.Println("      Rules dir: none (Tier-2, injected via SessionStart)")
		}

		// 钩子三态：已装且最新 / 已装但格式过期（如升级后还缺 --hook）/ 未装。
		// 光看文件里有没有 "rulemux" 字样是不够的 —— 那样旧格式也会显示 ✓（见设计 §9.1）。
		hookPath, envUsed := a.HookFileAbsWithSource(ws)
		srcNote := ""
		if envUsed != "" {
			srcNote = fmt.Sprintf(" [dir from $%s]", envUsed)
		}
		installed, cmd, err := hooks.Inspect(hookPath, a)
		// dsh 的「安装项」是一个 Cordis 插件（patch 里的 file:// URL），措辞用 Plugin 更准。
		label := "Hook"
		if a.Style == "dsh" {
			label = "Plugin"
		}
		switch {
		case errors.Is(err, hooks.ErrRefreshUnsupported):
			fmt.Printf("      %s: ? unchecked → %s%s (hook config format not verified yet)\n", label, hookPath, srcNote)
		case err != nil:
			fmt.Printf("      %s: ⚠ unreadable → %s%s: %v\n", label, hookPath, srcNote, err)
		case !installed:
			fmt.Printf("      %s: ✗ not installed → %s%s (run rulemux init --agent %s)\n", label, hookPath, srcNote, a.ID)
		case cmd == hooks.ExpectedCommand(a, cmd):
			fmt.Printf("      %s: ✓ installed, up to date → %s%s\n", label, hookPath, srcNote)
		default:
			fmt.Printf("      %s: ⚠ installed but OUTDATED → %s%s\n", label, hookPath, srcNote)
			fmt.Printf("            found:    %s\n", cmd)
			fmt.Printf("            expected: %s\n", hooks.ExpectedCommand(a, cmd))
			fmt.Printf("            fix: rulemux init --refresh\n")
		}
		if a.Note != "" {
			fmt.Printf("      Note: %s\n", a.Note)
		}
	}

	fmt.Println("\n⚠ = this agent's rules dir / hook location is not verified yet; confirm it with rulemux verify.")
	return 0
}
