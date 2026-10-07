package cmd

import (
	"fmt"
	"os"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/engine"
)

// Inject 把规则内容输出到 stdout，供 Tier-2 agent 的 SessionStart 钩子注入上下文。
//
// 用法：rulemux inject --agent codex
// Tier-2（只认单文件 AGENTS.md）没有目录可丢文件，因此走注入，且**不碰用户自己的 AGENTS.md**。
func Inject(args []string) int {
	f := ParseFlags(args)
	id := f.Get("agent", "")
	if id == "" {
		fmt.Fprintln(os.Stderr, "rulemux: inject 需要 --agent <id>")
		return 2
	}
	a, ok := agents.Get(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "rulemux: 未知 agent %q\n", id)
		return 2
	}

	cfg, err := config.Load(f.Get("config", config.DefaultPath()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: 读取配置失败: %v\n", err)
		return 1
	}

	srcs := cfg.SourcesFor(a.ID)
	if len(srcs) == 0 {
		// 没有该 agent 的源：静默退出，避免往上下文注入空内容
		return 0
	}
	out, err := engine.RenderInject(srcs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: 渲染注入内容失败: %v\n", err)
		return 1
	}
	fmt.Print(out)
	return 0
}
