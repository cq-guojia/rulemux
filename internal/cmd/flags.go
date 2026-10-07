// Package cmd 实现 rulemux 的各个子命令。
package cmd

import (
	"os"
	"path/filepath"
	"strings"
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

// workspaceWith 决定工作区：优先命令行 --workspace，其次配置里的 workspace，最后取 cwd。
func workspaceWith(flagVal, cfgVal string) (string, error) {
	if flagVal != "" {
		return filepath.Abs(flagVal)
	}
	if cfgVal != "" {
		return filepath.Abs(cfgVal)
	}
	return os.Getwd()
}
