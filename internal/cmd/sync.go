package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/engine"
	"github.com/cq-guojia/rulemux/internal/state"
)

// Sync 把配置所列源文件真实拷贝进各 agent 的原生规则目录（Tier-1）。
//
// 典型调用：由各 agent 的 SessionStart 钩子执行 rulemux sync --agent <id>。
// 未指定 --agent 时处理全部 Tier-1 agent。
func Sync(args []string) int {
	f := ParseFlags(args)
	cfgPath := f.Get("config", config.DefaultPath())
	wsFlag := f.Get("workspace", "")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: failed to read config: %v\n  Hint: run rulemux init to generate a sample config\n", err)
		return 1
	}
	// Feature-switch first: confirm the requested agent is supported before validating
	// config content (unverified agents are rejected outright).
	if reqAgent := f.Get("agent", ""); reqAgent != "" {
		if a, ok := agents.Get(reqAgent); !ok || !a.Verified {
			if ok && !a.Verified {
				fmt.Fprintf(os.Stderr, "rulemux: agent %q is not supported yet: only verified agents are available: %s\n", reqAgent, agents.SupportedSummary())
			} else {
				fmt.Fprintf(os.Stderr, "rulemux: unknown agent %q\n", reqAgent)
			}
			return 1
		}
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "rulemux:", err)
		return 1
	}

	ws, err := workspace(wsFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}

	requested := f.Get("agent", "")
	targets := targetAgents(requested)
	if len(targets) == 0 {
		if requested != "" {
			if a, ok := agents.Get(requested); ok && !a.Verified {
				fmt.Fprintf(os.Stderr, "rulemux: agent %q is not supported yet: only verified agents are available: %s\n", requested, agents.SupportedSummary())
			} else {
				fmt.Fprintf(os.Stderr, "rulemux: unknown agent %q\n", requested)
			}
		} else {
			fmt.Fprintln(os.Stderr, "rulemux: no verified agent to process")
		}
		return 1
	}

	exit := 0
	for _, a := range targets {
		if a.Tier == agents.Tier2 {
			// Tier-2 没有规则目录可丢文件，由 inject 子命令负责注入
			continue
		}
		dir := a.RulesDirAbs(ws)
		// 共用同一目录的 agent（如 codebuddy/workbuddy）合并计算源，
		// 避免其中一个把另一个的文件当残留删掉。
		group := agents.ByRulesDir(a.RulesDir)
		srcs := unionSources(cfg, group, ws)
		// 只要该目录下有任一 agent 需要 frontmatter（如 CodeBuddy/WorkBuddy），
		// 落盘文件就统一带上 alwaysApply:true 头。
		needFM := false
		for _, x := range group {
			if x.NeedsFrontmatter {
				needFM = true
				break
			}
		}
		res, err := engine.Sync(dir, srcs, needFM)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rulemux: failed to sync %s: %v\n", a.ID, err)
			exit = 1
			continue
		}
		printSyncResult(a, dir, res)
		if len(res.Missing) > 0 {
			exit = 1
		}
	}

	// 记入账本：钩子是全局的，但规则文件落在各工作区本地。
	// 卸载时要靠这份账本逐一回访清理，否则钩子一去、残留将永无机会被自动删除。
	if err := state.Record(ws); err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: warning: failed to write the workspace ledger: %v\n", err)
	}
	return exit
}

// targetAgents 决定本次处理哪些 agent；未指定则处理全部「已验证」的 Tier-1。
// 未做好的 agent（Verified==false）一律不参与，从源头保证只动做好的适配。
func targetAgents(id string) []agents.Agent {
	if id != "" {
		a, ok := agents.Get(id)
		if !ok || !a.Verified {
			return nil
		}
		return []agents.Agent{a}
	}
	var out []agents.Agent
	for _, a := range agents.Supported() {
		if a.Tier == agents.Tier1 {
			out = append(out, a)
		}
	}
	return out
}

// unionSources 合并多个 agent 的源规则，按文件列表去重；并按当前工作区过滤。
func unionSources(cfg *config.Config, list []agents.Agent, workspace string) []config.Source {
	seen := map[string]bool{}
	var out []config.Source
	for _, a := range list {
		for _, s := range cfg.SourcesFor(a.ID, workspace) {
			key := strings.Join(s.Paths, "|")
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, s)
		}
	}
	return out
}

// printSyncResult 打印单个 agent 的同步结果。
func printSyncResult(a agents.Agent, dir string, r *engine.SyncResult) {
	fmt.Printf("rulemux: %s → %s\n", a.ID, dir)
	if len(r.Copied) > 0 {
		fmt.Println("  Added:", strings.Join(r.Copied, ", "))
	}
	if len(r.Updated) > 0 {
		fmt.Println("  Updated:", strings.Join(r.Updated, ", "))
	}
	if len(r.Deleted) > 0 {
		fmt.Println("  Removed residue:", strings.Join(r.Deleted, ", "))
	}
	if len(r.Skipped) > 0 {
		fmt.Printf("  Unchanged, skipped: %d file(s)\n", len(r.Skipped))
	}
	for _, m := range r.Missing {
		fmt.Fprintf(os.Stderr, "  ⚠ source file missing: %s\n", m)
	}
	if len(r.Skipped) == 0 && r.IsEmpty() {
		fmt.Println("  Nothing to do (no source files listed for this agent)")
	}
}
