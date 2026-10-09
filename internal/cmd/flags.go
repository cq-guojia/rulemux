// Package cmd 实现 rulemux 的各个子命令。
package cmd

import (
	"fmt"
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

// agentReq 把解析出的 agent 与「用户 --agent 里写的原始标识」绑定，供安装路径把别名
// 透传到钩子命令与回显（品牌分离：workbuddy 与 codebuddy 是同一 agent，但用户面写的是 workbuddy）。
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
		// 别名去重：codebuddy,workbuddy 指的是同一个 agent（设计 §三 方案 A）。
		// 并用首次出现的请求标识（如 workbuddy）作为 Display。
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
