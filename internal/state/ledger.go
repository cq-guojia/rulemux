// Package state 维护 rulemux 极小的一部分运行时状态：「曾经在哪些工作区投放过规则文件」。
//
// 为什么需要它：
//   - SessionStart 钩子装在 user 级 host 配置里，对所有工作区**全局**生效；
//   - 但规则文件落在**每个工作区各自**的规则目录中；
//   - 于是卸载时钩子被一次性全局移除，各工作区曾经投放过的文件若不同步清理，
//     就再没有机会被删除（钩子已不在 ⇒ 不会再有下一次 sync 去触发前缀残留清理）。
//
// 因此用一个最小的「工作区账本」记下所有曾经同步过的工作区；卸载时按账本逐一回访，
// 即使某个工作区后来被清空或删除也保留记录——路径重现（如重新 clone）时仍会被回访清理。
//
// 注意：这不是 T5 否掉的那份「文件清单」。T5 否的是「用 manifest 记录同步了哪些文件」
//（改用 .rulemux__ 前缀就地解决）；本账本记的是「同步过哪些工作区」，两者不是一回事。
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// FileName 是账本文件名，位于 ~/.rulemux/ 下（与 config.toml 同级）。
const FileName = "workspaces.json"

// Path 返回账本文件的绝对路径。
func Path() string {
	home, err := os.UserHomeDir()
	dir := ".rulemux"
	if err == nil && home != "" {
		dir = filepath.Join(home, ".rulemux")
	}
	return filepath.Join(dir, FileName)
}

// Ledger 是「曾同步工作区」账本。
type Ledger struct {
	// Workspaces 记录所有曾经同步过的工作区绝对路径；保存时去重并排序，便于稳定比对。
	Workspaces []string `json:"workspaces"`
}

// Load 读取账本。文件不存在、为空或损坏时返回空账本而非报错——
// 账本只服务于「尽力清理」，不值得为此阻塞主线命令。
func Load() Ledger {
	b, err := os.ReadFile(Path())
	if err != nil || len(b) == 0 {
		return Ledger{}
	}
	var l Ledger
	if err := json.Unmarshal(b, &l); err != nil {
		return Ledger{}
	}
	return l
}

// Record 把工作区写入账本（幂等：已存在则不写）。
func Record(ws string) error {
	ws = filepath.Clean(ws)
	l := Load()
	for _, w := range l.Workspaces {
		if filepath.Clean(w) == ws {
			return nil
		}
	}
	l.Workspaces = append(l.Workspaces, ws)
	return l.Save()
}

// Save 去重排序后写回磁盘。
func (l Ledger) Save() error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	seen := make(map[string]bool, len(l.Workspaces))
	out := make([]string, 0, len(l.Workspaces))
	for _, w := range l.Workspaces {
		w = filepath.Clean(w)
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.Strings(out)
	b, err := json.MarshalIndent(Ledger{Workspaces: out}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// Others 返回账本中除 except 之外的其它工作区列表。
func Others(except string) []string {
	except = filepath.Clean(except)
	var out []string
	for _, w := range Load().Workspaces {
		if filepath.Clean(w) != except {
			out = append(out, w)
		}
	}
	return out
}
