// Package cmd 实现 rulemux 的各个子命令。
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
)

// Flags 是极简命令行参数解析的结果（不引第三方库，保持零依赖）。
type Flags struct {
	Vals map[string]string // --key value / --key=value
	Bool map[string]bool   // --flag
	Rest []string          // 非 -- 开头的裸参数
}

// ParseFlags 解析参数列表。
func ParseFlags(args []string) *Flags {
	f := &Flags{Vals: map[string]string{}, Bool: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			f.Rest = append(f.Rest, a)
			continue
		}
		key := strings.TrimPrefix(a, "--")
		if idx := strings.Index(key, "="); idx >= 0 {
			k := key[:idx]
			f.Vals[k] = key[idx+1:]
			f.Bool[k] = true
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			f.Vals[key] = args[i+1]
			f.Bool[key] = true
			i++
			continue
		}
		f.Bool[key] = true
	}
	return f
}

// Get 返回字符串参数值，未提供或为空时返回 def。
func (f *Flags) Get(key, def string) string {
	if v, ok := f.Vals[key]; ok && v != "" {
		return v
	}
	return def
}

// Has 返回开关参数是否出现过。
func (f *Flags) Has(key string) bool { return f.Bool[key] }

// workspace 返回工作区绝对路径，未指定则取当前工作目录。
func workspace(v string) (string, error) {
	if v != "" {
		return filepath.Abs(v)
	}
	return os.Getwd()
}

// hookWorkspace 在 hook 调用时从 stdin 载荷解析工作区。
//
// Trae / Claude Code 的 hook 事件会把 `cwd` 与 `workspace_roots` 透传进 JSON 载荷
// （2026-10-09 Trae SessionStart 实测：
//  {"cwd":"/workspace/Temp","workspace_roots":["/workspace/Temp"],"hook_event_name":"SessionStart",...}），
// 由此定位工作区比依赖 os.Getwd() 更可靠——钩子可能从任意 cwd 拉起 rulemux。
// 解析失败（无管道 / 交互式终端 / 非 JSON / 缺字段）返回 ("", false)，调用方回退到 os.Getwd()。
func hookWorkspace() (string, bool) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return "", false
	}
	// 交互式终端（无管道输入）不读，避免 io.ReadAll 阻塞等 EOF。
	if fi.Mode()&os.ModeCharDevice != 0 {
		return "", false
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil || len(b) == 0 {
		return "", false
	}
	var p struct {
		CWD            string   `json:"cwd"`
		WorkspaceRoots []string `json:"workspace_roots"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return "", false
	}
	if p.CWD != "" {
		return p.CWD, true
	}
	if len(p.WorkspaceRoots) > 0 {
		return p.WorkspaceRoots[0], true
	}
	return "", false
}

// agentReq 把解析出的 agent 与「用户 --agent 里写的原始标识」绑定，供安装路径把该标识
// 透传到钩子命令与回显（品牌分离：用户写 workbuddy 就写 --agent workbuddy，而非 codebuddy）。
type agentReq struct {
	Agent   agents.Agent
	Display string // 用户 --agent 里写的原始标识（可能是别名，如 workbuddy）
}

// parseAgents 把 --agent 的值（如 "codebuddy,workbuddy"）拆成 Agent 列表，
// 逐个在注册表里校验；含未知 id 或「尚未做好」（Verified==false）的 agent 即报错。
// 绝不扫描机器上的 agent。
func parseAgents(s string) ([]agents.Agent, error) {
	out, err := parseAgentsDisplay(s)
	if err != nil {
		return nil, err
	}
	agentsList := make([]agents.Agent, len(out))
	for i, ar := range out {
		agentsList[i] = ar.Agent
	}
	return agentsList, nil
}

// parseAgentsDisplay 同 parseAgents，但为每个去重后的 agent 保留「首个匹配到的请求标识」
// 作为 Display，供安装时写入钩子命令与回显使用。
func parseAgentsDisplay(s string) ([]agentReq, error) {
	var out []agentReq
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		a, ok := agents.Get(p)
		if !ok {
			return nil, fmt.Errorf("unknown agent %q (supported: %s)", p, agents.SupportedSummary())
		}
		if !a.Verified {
			return nil, fmt.Errorf("agent %q is not installable yet: only verified agents are supported: %s (the other adapters are still in progress and cannot be installed until ready)", a.ID, agents.SupportedSummary())
		}
		// 去重：同一 agent 的多个写法（如别名 codebuddy-cn 与 codebuddy）只保留首次出现的
		// 请求标识作为 Display；不同 agent（codebuddy、workbuddy）各自独立保留。
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		out = append(out, agentReq{Agent: a, Display: p})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no agent specified")
	}
	return out, nil
}
