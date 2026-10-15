// Package state 维护 rulemux 极小的一部分运行时状态：「曾经在哪些工作区投放过规则文件」，
// 每个工作区「用过哪些 agent」，以及机器级「已卸载」的 agent 集合（off）。
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
// 结构版本见 Version：v1 只有 workspaces；v2 追加 agents（工作区 → 该工作区用过的 agent）；
// v3 追加 Seen（每个「工作区 × agent」条目的 lastSeen 时间戳，支撑过期门禁）。
// v1 → v2 / v2 → v3 迁移**绝不丢条目**：老条目沿用旧语义、Seen 缺失即视为「在用」（保守）。
// Load 不按版本分支（老文件无缝），Version 只作诚实标注。
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

// Version 是当前账本结构版本（v3 起含 Seen 时间戳）。
const Version = 3

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
	// **不设 omitempty**：空 map 也落 "agents":{}，保证文件永不退化成 v1 形态
	// （v1 形态会让 Load 给所有工作区回填哨兵，把卸载成果冲掉——见 Load）。
	// 某工作区是空列表 = 明确的「该工作区无 agent」（uninstall 清空后的终态）。
	Agents map[string][]string `json:"agents"`
	// Off 是机器级「已卸载」集合：uninstall --agent X 时加入，init 装入时移除。
	// 展开哨兵 * 与例行输出（sync 汇总未初始化名单 / doctor）时减去它 ⇒ 已卸
	// agent 对用户不可见（卸载即断联）。缺省空 = 没有卸载记录。
	Off []string `json:"off,omitempty"`
	// Seen 记录每个「工作区 × agent」条目的最近活动时间戳（unix 秒，omitempty）。
	// 仅在该 agent 的钩子被触发、用户在该工作区手动 sync、或 init 时刷新；例行
	// sync --all 不刷新（否则过期门禁失效）。缺失（老账本 / 未观测）＝保守：视为在用。
	Seen map[string]map[string]int64 `json:"seen,omitempty"`
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
	l.Off = append([]string(nil), raw.Off...)
	l.Seen = raw.Seen
	if raw.Agents != nil {
		l.Agents = raw.Agents
		return l
	}
	// v1 → v2：**整个文件没有 agents 键**（v1 老形态）才给每条工作区填哨兵占位，
	// 绝不丢条目（丢条目会让卸载再也回访不到，残留永存）。
	// 键存在但缺某工作区 = 明确的「该工作区无 agent」——这是 uninstall 清空后的
	// 正常终态，绝不能再回填成 *：那会让下一次 Load 把卸载成果整个冲掉（B1）。
	for _, ws := range l.Workspaces {
		l.Agents[ws] = []string{AnyAgent}
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
	return UpdateLedger(func(l *Ledger) error {
		l.add(ws, agent)
		l.see(ws, agent) // 钩子触发 / 手动 sync / init ⇒ 刷新在用时间
		return nil
	})
}

// Touch 只保账本成员、不刷新 Seen：供 sync --all 落盘点使用，避免例行同步把 lastSeen
// 永久续命、使过期门禁失效。真正的「在用」信号（钩子触发 / 手动 sync / init）仍走 Record。
func Touch(ws, agent string) error {
	ws = resolveWS(ws)
	if ws == "" {
		return nil
	}
	return UpdateLedger(func(l *Ledger) error {
		l.add(ws, agent)
		return nil
	})
}

// see 把 (ws, agent) 的 lastSeen 刷成 now（agent 为空 / 哨兵时跳过）。
func (l *Ledger) see(ws, agent string) {
	if agent == "" || agent == AnyAgent {
		return
	}
	if l.Seen == nil {
		l.Seen = map[string]map[string]int64{}
	}
	if l.Seen[ws] == nil {
		l.Seen[ws] = map[string]int64{}
	}
	l.Seen[ws][agent] = time.Now().Unix()
}

// IsStale 报告 (ws, agent) 是否超过 StaleAfter 未活动。
// 缺 Seen / 缺条目 / 哨兵 ⇒ false（保守：视为在用，不误杀老账本与未观测条目）。
func (l Ledger) IsStale(ws, agent string) bool {
	ws = resolveWS(ws)
	if l.Seen == nil || ws == "" || agent == "" || agent == AnyAgent {
		return false
	}
	m := l.Seen[ws]
	if m == nil {
		return false
	}
	ts, ok := m[agent]
	if !ok {
		return false
	}
	return time.Since(time.Unix(ts, 0)) > StaleAfter
}

// StaleAfter 是账本条目的过期阈值：lastSeen 距今超过它，就视为「该工作区不再用此 agent」，
// 在裸 sync / sync --all / 顺手同步中跳过该条目（条目保留供 uninstall 回访）。默认 30 天。
const StaleAfter = 30 * 24 * time.Hour

// UpdateLedger 是全项目唯一的账本写入口：持锁 load → mutate → 原子 save。
// mutate 里只改内存中的账本，返回 error 则不动盘。Record / Prune / uninstall
// 清账本 / init 清 off 全走它 ⇒ 锁与原子写只有一份实现。
func UpdateLedger(mutate func(*Ledger) error) error {
	unlock, err := acquireLock()
	if err != nil {
		return err
	}
	defer unlock()

	l := Load()
	if err := mutate(&l); err != nil {
		return err
	}
	return l.Save()
}

// ForgetAgent 把 agent 从指定工作区的 agents 列表里移除（只动 Agents，保留
// Workspaces 条目——卸载的回访清单不能少工作区）。
//
// 哨兵 * 的列表**不动**：「未知 = 全部」的排除由 off 在展开时完成；在这里物化成
// 具体清单，等于把「猜测此刻有哪些 agent」写进账本（B2），还会让将来新就绪的
// agent 不再被 * 覆盖。
func (l *Ledger) ForgetAgent(ws string, ids ...string) {
	if ws == "" || len(ids) == 0 {
		return
	}
	target := resolveWS(ws)
	for key, list := range l.Agents {
		if resolveWS(key) != target || containsStr(list, AnyAgent) {
			continue
		}
		l.Agents[key] = removeStrings(list, ids)
		l.forgetSeen(key, ids)
	}
}

// forgetSeen 同步删除 (ws, ids) 的 lastSeen 记录。
func (l *Ledger) forgetSeen(ws string, ids []string) {
	if l.Seen == nil {
		return
	}
	m := l.Seen[ws]
	if m == nil {
		return
	}
	for _, id := range ids {
		delete(m, id)
	}
	if len(m) == 0 {
		delete(l.Seen, ws)
	}
}

// AddOff / ClearOff 维护机器级「已卸载」集合：uninstall 时加、init 装入时清。
func (l *Ledger) AddOff(ids ...string)   { l.Off = append(l.Off, ids...) }
func (l *Ledger) ClearOff(ids ...string) { l.Off = removeStrings(l.Off, ids) }

// removeStrings 返回 list 去掉 drop 中元素后的新切片（不原地改）。
func removeStrings(list, drop []string) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		if !containsStr(drop, s) {
			out = append(out, s)
		}
	}
	return out
}

// dedupeSorted 去重排序；空 ⇒ nil（省略 JSON 键 = 缺省空集合）。
func dedupeSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
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

	out := Ledger{Version: Version, Workspaces: []string{}, Agents: map[string][]string{}, Off: dedupeSorted(l.Off)}
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

	// Seen 随清理后的 Agents 重建：ws / agent 不在 Agents 里的条目丢弃，避免孤儿时间戳。
	if l.Seen != nil {
		for ws, agents := range out.Agents {
			byAgent := l.Seen[ws]
			if byAgent == nil {
				continue
			}
			for _, a := range agents {
				if a == AnyAgent {
					continue
				}
				if ts, ok := byAgent[a]; ok && ts > 0 {
					if out.Seen == nil {
						out.Seen = map[string]map[string]int64{}
					}
					if out.Seen[ws] == nil {
						out.Seen[ws] = map[string]int64{}
					}
					out.Seen[ws][a] = ts
				}
			}
		}
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

	return UpdateLedger(func(l *Ledger) error {
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
				delete(l.Seen, k)
			}
		}
		return nil
	})
}

// Others 返回账本中除 except 之外的其它工作区列表。
func Others(except string) []string { return Load().Others(except) }

// Others 返回账本中除 except 之外的其它工作区列表（方法版：供调用方复用同一份
// 快照，避免 confirm/sweep 两次 Load 之间的 TOCTOU）。
func (l Ledger) Others(except string) []string {
	except = resolveWS(except)
	var out []string
	for _, w := range l.Workspaces {
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
