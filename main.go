// Command rulemux 把用户自己维护的规则文件，投递到各家 AI coding agent 的原生规则目录。
//
// 设计要点（docs/design/implementation.md）：
//   - 无守护进程、不监听文件改动：由各 agent 的 SessionStart 钩子调起本程序。
//   - Tier-1（读整个规则文件夹的 agent）：真实拷贝，文件名加前缀 .rulemux__。
//   - Tier-2（只认单文件 AGENTS.md 的 agent）：走钩子注入，不碰用户自己的 AGENTS.md。
package main

import (
	"fmt"
	"os"

	"github.com/cq-guojia/rulemux/internal/cmd"
)

// version 是 rulemux 的版本号。
const version = "0.1.0"

func usage() {
	fmt.Fprint(os.Stderr, `rulemux - 一份规则，投递到各家 AI coding agent

用法:
  rulemux sync    [--agent <id>] [--config <path>] [--workspace <dir>]   同步规则（被各 agent 的 SessionStart 钩子调用）
  rulemux inject  --agent <id>  [--config <path>]                        Tier-2：把规则输出到 stdout 供钩子注入
  rulemux init    [--config <path>] [--workspace <dir>]                  生成示例配置并为各 agent 安装 SessionStart 钩子
  rulemux doctor  [--config <path>] [--workspace <dir>]                  环境自检
  rulemux verify  [--agent <id>] [--clean] [--workspace <dir>]           canary 验收

支持的 agent:
  claude     Claude Code      Tier-1  .claude/rules/
  codebuddy  CodeBuddy        Tier-1  .codebuddy/rules/
  workbuddy  WorkBuddy        Tier-1  .codebuddy/rules/（与 CodeBuddy 同目录，合并处理）
  trae       Trae（CN/国际版统一） Tier-1  .trae/rules/
  codex      Codex            Tier-2  注入（不碰用户 AGENTS.md）
  opencode   OpenCode         Tier-2  注入（不碰用户 AGENTS.md）

其它:
  --version   打印版本
  --help      打印本帮助
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
	default:
		fmt.Fprintf(os.Stderr, "rulemux: 未知子命令 %q\n\n", args[0])
		usage()
		os.Exit(2)
	}
}
