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

// parseAgents 把 --agent 的值（如 "codebuddy,workbuddy"）拆成 Agent 列表，
// 逐个在注册表里校验；含未知 id 或「尚未做好」（Verified==false）的 agent 即报错。
// 绝不扫描机器上的 agent。
func parseAgents(s string) ([]agents.Agent, error) {
	var out []agents.Agent
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
			return nil, fmt.Errorf("agent %q 尚未支持安装：当前仅支持已验证的 %s（其余 agent 还在做，未就绪前不允许安装）", a.ID, agents.SupportedSummary())
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no agent specified")
	}
	return out, nil
}
