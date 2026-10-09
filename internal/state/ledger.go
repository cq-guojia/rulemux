// Package state 维护 rulemux 极小的一部分运行时状态：「曾经在哪些工作区投放过规则文件」，
// 以及每个工作区「用过哪些 agent」。
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
// （已改用 __rulemux__ 前缀就地解决）；本账本记的是「同步过哪些工作区 / 用过哪些 agent」。
//
// 结构版本见 Version：v1 只有 workspaces；v2 追加 agents（工作区 → 该工作区用过的 agent）。
// v1 → v2 迁移**绝不丢条目**：老条目的 agents 用哨兵 AnyAgent 占位（表示「未知 / 全部已支持」）。
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cq-guojia/rulemux/internal/config"
)

// FileName 是账本文件名，位于 ~/.rulemux/ 下（与 config.toml 同级）。
const FileName = "workspaces.json"

// EnvStateDir 可覆盖账本所在目录（测试/CI 用，避免污染真实 ~/.rulemux）。
const EnvStateDir = "RULEMUX_STATE_DIR"

// Version 是当前账本结构版本。
const Version = 2

// AnyAgent 是「该工作区用过哪些 agent」的哨兵值，表示「未知 / 全部已支持 agent」。
//
// v1 账本只记工作区、不记 agent ⇒ 迁移时老条目填它。消费方应把它解释为
// 「全部已支持 agent」，而不要当成一个真实 agent ID。
const AnyAgent = "*"

// Path 返回账本文件的绝对路径。
// 优先取环境变量 RULEMUX_STATE_DIR（便于测试隔离），否则 ~/.rulemux/。
func Path() string {
	if d := os.Getenv(EnvStateDir); d != "" {
		return filepath.Join(d, FileName)
	}
	home, err := os.UserHomeDir()
	dir := ".rulemux"
	if err == nil && home != "" {
		dir = filepath.Join(home, ".rulemux")
	}
	return filepath.Join(dir, FileName)
}

// LockPath 返回账本锁文件路径（与账本同级）。
func LockPath() string { return Path() + ".lock" }

// Ledger 是「曾同步工作区 × 其用过的 agent」账本。
type Ledger struct {
	// Version 为结构版本；读到 v1 形态时 Load 会就地迁移并把它升到 Version。
	Version int `json:"version"`
	// Workspaces 记录所有曾经同步过的工作区绝对路径；保存时去重并排序，便于稳定比对。
	Workspaces []string `json:"workspaces"`
	// Agents 记录每个工作区用过哪些 agent（规范 ID）。值可能含哨兵 AnyAgent。
	Agents map[string][]string `json:"agents,omitempty"`
}

// Load 读取账本。文件不存在、为空或损坏时返回空账本而非报错——
// 账本只服务于「尽力清理」，不值得为此阻塞主线命令。
//
// 若读到 v1 形态（只有 workspaces、没有 agents），就地迁移：老条目的 agents
// 记哨兵 AnyAgent，**绝不丢条目**（丢条目会让卸载再也回访不到，残留永存）。
func Load() Ledger {
	l := Ledger{Version: Version, Workspaces: []string{}, Agents: map[string][]string{}}
	b, err := os.ReadFile(Path())
	if err != nil || len(b) == 0 {
		return l
	}
	var raw Ledger
	if err := json.Unmarshal(b, &raw); err != nil {
		return l // 损坏：返回空账本（尽力清理，不阻塞主线）
	}
	l.Workspaces = raw.Workspaces
	if raw.Agents != nil {
		l.Agents = raw.Agents
	}
	// v1 → v2：没有 agents 信息的老条目用哨兵占位。
	for _, ws := range l.Workspaces {
		if _, ok := l.Agents[ws]; !ok {
			l.Agents[ws] = []string{AnyAgent}
		}
	}
	return l
}

// Record 把「工作区 × agent」写进账本（幂等）。
//
// 并发安全：先加锁（O_EXCL 锁文件），再 load→改→save，且 save 走「临时文件 + rename」
// 原子替换。二者互补——rename 防「写坏文件」，锁防「并发覆盖丢更新」。
// 拿不到锁时返回错误，由调用方降级为告警（不阻塞主线）。
func Record(ws, agent string) error {
	ws = resolveWS(ws)
	if ws == "" {
		return nil
	}
	unlock, err := acquireLock()
	if err != nil {
		return err
	}
	defer unlock()

	l := Load()
	l.add(ws, agent)
	return l.Save()
}

// add 幂等地把一条 (工作区, agent) 收进账本（不写盘）。
func (l *Ledger) add(ws, agent string) {
	if l.Agents == nil {
		l.Agents = map[string][]string{}
	}
	if !containsStr(l.Workspaces, ws) {
		l.Workspaces = append(l.Workspaces, ws)
	}
	if agent == "" {
		if _, ok := l.Agents[ws]; !ok {
			l.Agents[ws] = []string{AnyAgent}
		}
		return
	}
	list := l.Agents[ws]
	// 哨兵已表示「全部已支持 agent」；已记录过同一 agent 也无需重复。
	if containsStr(list, AnyAgent) || containsStr(list, agent) {
		return
	}
	l.Agents[ws] = append(list, agent)
}

// AgentsFor 返回该工作区记录过的 agent 列表（可能含哨兵 AnyAgent）；无记录返回 nil。
// 消费方需自行把 AnyAgent 解释为「全部已支持 agent」。
func (l Ledger) AgentsFor(ws string) []string {
	if l.Agents == nil {
		return nil
	}
	list := l.Agents[resolveWS(ws)]
	return append([]string(nil), list...)
}

// AllWorkspaces 返回账本里记录过的全部工作区（拷贝，去重排序后的视图）。
func (l Ledger) AllWorkspaces() []string {
	return append([]string(nil), l.Workspaces...)
}

// Save 去重排序后用「临时文件 + rename」原子写回磁盘。
func (l Ledger) Save() error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	out := Ledger{Version: Version, Workspaces: []string{}, Agents: map[string][]string{}}
	seen := map[string]bool{}
	for _, w := range l.Workspaces {
		w = filepath.Clean(w)
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out.Workspaces = append(out.Workspaces, w)
	}
	sort.Strings(out.Workspaces)

	for ws, list := range l.Agents {
		ws = filepath.Clean(ws)
		if ws == "" {
			continue
		}
		seenA := map[string]bool{}
		vals := []string{}
		for _, a := range list {
			if a == "" || seenA[a] {
				continue
			}
			seenA[a] = true
			vals = append(vals, a)
		}
		sort.Strings(vals)
		out.Agents[ws] = vals
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Prune 从账本里删除指定工作区（连同其 agents 记录）。
//
// ⚠️ 只应由 `sync --all` 的 GC 调用，且**仅**在确认路径确实不存在（ENOENT）时使用：
// 目录只是「临时不可用」（外挂盘 / 网络盘未挂载）时删条目，会让卸载再也回访不到它，
// 那些残留就永远清不掉了。见设计 §五 账本 GC。
func Prune(workspaces []string) error {
	if len(workspaces) == 0 {
		return nil
	}
	drop := map[string]bool{}
	for _, ws := range workspaces {
		if k := resolveWS(ws); k != "" {
			drop[k] = true
		}
	}

	unlock, err := acquireLock()
	if err != nil {
		return err
	}
	defer unlock()

	l := Load()
	kept := make([]string, 0, len(l.Workspaces))
	for _, ws := range l.Workspaces {
		if drop[resolveWS(ws)] {
			continue
		}
		kept = append(kept, ws)
	}
	l.Workspaces = kept
	for k := range l.Agents {
		if drop[resolveWS(k)] {
			delete(l.Agents, k)
		}
	}
	return l.Save()
}

// Others 返回账本中除 except 之外的其它工作区列表。
func Others(except string) []string {
	except = resolveWS(except)
	var out []string
	for _, w := range Load().Workspaces {
		if resolveWS(w) != except {
			out = append(out, w)
		}
	}
	return out
}

// resolveWS 把工作区路径归一（绝对 + 软链解析），与 config 的匹配口径保持一致。
// 否则同一物理工作区经软链打开时会被记成两条。
func resolveWS(ws string) string {
	if ws == "" {
		return ""
	}
	return config.ResolvePath(ws)
}

// containsStr 报告切片是否包含 s（精确匹配）。
func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// 锁参数：重试 3 次、间隔 50ms；超过 lockStale 的锁文件视为持锁进程已崩，可接管。
const (
	lockRetries  = 3
	lockInterval = 50 * time.Millisecond
	lockStale    = 2 * time.Second
)

// acquireLock 用 O_CREATE|O_EXCL 原子地创建锁文件（跨平台、零依赖）。
// 返回解锁函数；拿不到锁则返回错误（调用方降级为告警，不阻塞主线）。
func acquireLock() (func(), error) {
	p := LockPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	for i := 0; i < lockRetries; i++ {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(p) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if fi, statErr := os.Stat(p); statErr == nil && time.Since(fi.ModTime()) > lockStale {
			_ = os.Remove(p) // 陈旧锁：接管
			continue
		}
		time.Sleep(lockInterval)
	}
	return nil, fmt.Errorf("ledger is locked by another rulemux process: %s", p)
}
