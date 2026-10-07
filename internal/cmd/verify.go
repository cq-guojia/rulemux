package cmd

import (
	"fmt"
	"os"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/engine"
)

// Verify canary 验收：在规则目录放一个带暗号的探针文件，
// 由用户开一个新会话问 agent 能否念出暗号，从而坐实「SessionStart 复制 → 本会话加载」链路。
//
// 这同时验证两件事（docs/design/implementation.md #13）：
//  1. 钩子确实在读规则之前触发（否则本会话读不到）
//  2. 该 agent 的「读全部 .md」不跳过点开头的隐藏文件（.rulemux__ 前缀）
func Verify(args []string) int {
	f := ParseFlags(args)
	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: 无法确定工作区:", err)
		return 1
	}

	requested := f.Get("agent", "")
	targets := targetAgents(requested)
	if len(targets) == 0 {
		fmt.Fprintf(os.Stderr, "rulemux: 未知 agent %q\n", requested)
		return 1
	}
	// 未指定 agent 时只处理 Tier-1（Tier-2 无规则目录可放探针）
	if requested == "" {
		var onlyTier1 []agents.Agent
		for _, a := range targets {
			if a.Tier == agents.Tier1 {
				onlyTier1 = append(onlyTier1, a)
			}
		}
		targets = onlyTier1
	}

	// 清理模式
	if f.Has("clean") {
		for _, a := range targets {
			dir := a.RulesDirAbs(ws)
			if dir == "" {
				continue
			}
			if err := engine.CleanCanary(dir); err != nil {
				fmt.Fprintf(os.Stderr, "rulemux: 清理 %s 的 canary 失败: %v\n", a.ID, err)
				return 1
			}
			fmt.Printf("已清理 canary: %s\n", a.ID)
		}
		return 0
	}

	token := engine.CanaryToken()
	fmt.Println("rulemux verify —— canary 验收")
	fmt.Println("暗号:", token)
	fmt.Println()
	for _, a := range targets {
		dir := a.RulesDirAbs(ws)
		if dir == "" {
			fmt.Printf("  %s: Tier-2 无规则目录，请直接在会话里问它规则内容是否生效\n", a.ID)
			continue
		}
		p, err := engine.WriteCanary(dir, token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: 写入 canary 失败: %v\n", a.ID, err)
			return 1
		}
		fmt.Printf("  %s: 已写入 %s\n", a.ID, p)
	}

	fmt.Println()
	fmt.Println("请对每个 agent 开一个【新会话】，问它：")
	fmt.Printf("  「你能在规则里看到这个暗号吗：%s」\n", token)
	fmt.Println()
	fmt.Println("判据（docs/design/features/verification.md）：")
	fmt.Println("  · 念得出 ⇒ 链路通：SessionStart 复制在本会话生效，且该 agent 会读点开头的隐藏文件")
	fmt.Println("  · 念不出 ⇒ 先排查：钩子装了吗？开的是新会话吗？")
	fmt.Println("    若确认是「跳过点文件」⇒ 需把前缀 .rulemux__ 改为非点前缀 rulemux__")
	fmt.Println()
	fmt.Println("验完清理：rulemux verify --clean")
	fmt.Println("（canary 带 .rulemux__ 前缀，即便不清理，下一次 rulemux sync 也会当残留自动删掉）")
	return 0
}
