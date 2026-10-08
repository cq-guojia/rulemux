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
		fmt.Fprintf(os.Stderr, "rulemux: 读取配置失败: %v\n  提示：先运行 rulemux init 生成示例配置\n", err)
		return 1
	}
	// 开关优先：先确认请求的 agent 已支持，再校验配置内容（未验证的 agent 直接拒绝）。
	if reqAgent := f.Get("agent", ""); reqAgent != "" {
		if a, ok := agents.Get(reqAgent); !ok || !a.Verified {
			if ok && !a.Verified {
				fmt.Fprintf(os.Stderr, "rulemux: agent %q 尚未支持：当前仅支持已验证的 %s\n", reqAgent, agents.SupportedSummary())
			} else {
				fmt.Fprintf(os.Stderr, "rulemux: 未知 agent %q\n", reqAgent)
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
		fmt.Fprintln(os.Stderr, "rulemux: 无法确定工作区:", err)
		return 1
	}

	requested := f.Get("agent", "")
	targets := targetAgents(requested)
	if len(targets) == 0 {
		if requested != "" {
			if a, ok := agents.Get(requested); ok && !a.Verified {
				fmt.Fprintf(os.Stderr, "rulemux: agent %q 尚未支持：当前仅支持已验证的 %s\n", requested, agents.SupportedSummary())
			} else {
				fmt.Fprintf(os.Stderr, "rulemux: 未知 agent %q\n", requested)
			}
		} else {
			fmt.Fprintln(os.Stderr, "rulemux: 当前没有可处理的已验证 agent")
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
		srcs := unionSources(cfg, agents.ByRulesDir(a.RulesDir), ws)
		res, err := engine.Sync(dir, srcs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "rulemux: 同步 %s 失败: %v\n", a.ID, err)
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
		fmt.Fprintf(os.Stderr, "rulemux: 警告：写入工作区账本失败: %v\n", err)
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
		fmt.Println("  新增:", strings.Join(r.Copied, ", "))
	}
	if len(r.Updated) > 0 {
		fmt.Println("  更新:", strings.Join(r.Updated, ", "))
	}
	if len(r.Deleted) > 0 {
		fmt.Println("  删除残留:", strings.Join(r.Deleted, ", "))
	}
	if len(r.Skipped) > 0 {
		fmt.Printf("  内容未变跳过: %d 个\n", len(r.Skipped))
	}
	for _, m := range r.Missing {
		fmt.Fprintf(os.Stderr, "  ⚠ 源不存在: %s\n", m)
	}
	if len(r.Skipped) == 0 && r.IsEmpty() {
		fmt.Println("  无事可做（配置未列出该 agent 的源文件）")
	}
}
