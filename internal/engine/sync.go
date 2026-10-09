// Package engine 是 rulemux 的核心引擎：被各 agent 的适配器调用的那套统一方法。
//
// 对应设计（docs/design/implementation.md #10/#11）：
//   - 目标文件名 = 前缀 __rulemux__ + 源 basename（同名冲突时追加源路径短 hash）
//   - 内容一致 ⇒ 跳过；不一致 ⇒ 覆盖
//   - 带前缀但不在本次计划内的 ⇒ 删除（删残留，保证加删自由、不累积）
//   - 不带前缀的文件（用户自己的）一律不碰
package engine

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"

	"github.com/cq-guojia/rulemux/internal/config"
)

// Prefix 是 rulemux 落盘文件的统一前缀，用来区分「我方同步过的」与「用户自己的」。
// 必须是非隐藏前缀：CodeBuddy 实测会跳过点开头的隐藏文件（见 docs/design/external/agent-rules-dirs.md）。
const Prefix = "__rulemux__"

// AutoApplyFrontmatter 是 CodeBuddy/WorkBuddy 这类 agent 的落盘文件必须带的 YAML 头：
// 只有带 alwaysApply:true 的规则才会在会话开始被自动加载（2026-10-08 实测）。
// ponytail: 直接前置、不解析源文件本身是否已有 frontmatter —— 源规则文件应只放正文。
const AutoApplyFrontmatter = "---\nalwaysApply: true\n---\n"

// PlanItem 是一条「源文件 → 目标文件」的映射。
type PlanItem struct {
	Src     string // 源文件路径
	DstName string // 目标文件名
	DstPath string // 目标文件完整路径
}

// Plan 是某次同步的完整计划。
type Plan struct {
	Dir   string
	Items []PlanItem
}

// BuildPlan 计算目标文件名：前缀 + 源 basename；同名 basename 冲突时追加源路径短 hash。
// 同一源文件路径若被多条 source 同时引用，只在最终列表里保留一次（按源路径去重），
// 保证一个文件只被同步一次，不会因命中多条策略而重复注入（否则会生成两个不同目标名）。
func BuildPlan(dir string, srcs []config.Source) *Plan {
	seen := make(map[string]bool)
	var paths []string
	for _, s := range srcs {
		for _, p := range s.Paths {
			if seen[p] {
				continue // 同一源文件跨多条 source 只注入一次
			}
			seen[p] = true
			paths = append(paths, p)
		}
	}
	count := map[string]int{}
	for _, p := range paths {
		count[filepath.Base(p)]++
	}
	items := make([]PlanItem, 0, len(paths))
	for _, p := range paths {
		base := filepath.Base(p)
		name := Prefix + base
		if count[base] > 1 {
			ext := filepath.Ext(base)
			stem := strings.TrimSuffix(base, ext)
			name = Prefix + stem + "-" + shortHash(p) + ext
		}
		items = append(items, PlanItem{
			Src:     p,
			DstName: name,
			DstPath: filepath.Join(dir, name),
		})
	}
	return &Plan{Dir: dir, Items: items}
}

// SyncResult 记录一次同步实际做了什么。
type SyncResult struct {
	Copied  []string // 新复制的
	Updated []string // 内容变化被覆盖的
	Skipped []string // 内容一致跳过的
	Deleted []string // 清掉的残留
	Missing []string // 源文件不存在的
	Errors  []string // 单文件写/删失败（不中断其余处理，也不抬进程退出码）

	// SkippedNoDir 表示目标目录不存在、而本次不允许创建（create=false），
	// 于是整次同步被跳过（不建、不读、不删）。见设计 §二 原则 2。
	SkippedNoDir bool
}

// HasChanges 报告本次是否对目标目录产生了实质变化（新增 / 修改 / 删除）。
//
// 这就是「变化提示」的判据 —— 注意它**不包含** Missing：源文件不存在时磁盘
// 并没有任何变化，提示用户重开会话只会白折腾。见设计 §六。
func (r *SyncResult) HasChanges() bool {
	return len(r.Copied)+len(r.Updated)+len(r.Deleted) > 0
}

// IsEmpty 报告本次同步是否没有任何变更（全是跳过或压根没有源）。
func (r *SyncResult) IsEmpty() bool {
	return len(r.Copied)+len(r.Updated)+len(r.Deleted)+len(r.Missing)+len(r.Errors) == 0 && !r.SkippedNoDir
}

// Sync 把源列表同步进目标目录。整个过程无状态文件，靠内容比对 + 前缀删残留保证幂等。
// autoApplyFrontmatter 为 true 时（CodeBuddy/WorkBuddy），每个落盘文件前置 alwaysApply:true 头；
// 比对与写入都基于「加头后」的内容，保证幂等。
//
// create 控制「能否创建目标目录」——只有该 agent **自己的会话 hook** 调用时才为 true。
// create=false 且目录不存在时，整次同步立即返回（不建、不读、不删），
// 这是「只写已存在目录」这条铁律的落点（见 docs/design/features/sync-all-and-change-notice.md §二）。
//
// 单文件失败（写 / 删）不再中断整次同步：记入 Errors 后继续，既保证已完成的结果能被
// 上层看见（变化提示据此判定），也不会因一个文件拖垮整次同步。
func Sync(dir string, srcs []config.Source, autoApplyFrontmatter, create bool) (*SyncResult, error) {
	if _, err := os.Stat(dir); err != nil {
		if !create {
			return &SyncResult{SkippedNoDir: true}, nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create rules directory %s: %w", dir, err)
		}
	}

	plan := BuildPlan(dir, srcs)
	want := make(map[string]struct{}, len(plan.Items))
	for _, it := range plan.Items {
		want[it.DstName] = struct{}{}
	}

	res := &SyncResult{}
	for _, it := range plan.Items {
		data, err := os.ReadFile(it.Src)
		if err != nil {
			res.Missing = append(res.Missing, it.Src)
			continue
		}
		if autoApplyFrontmatter {
			data = append([]byte(AutoApplyFrontmatter), data...)
		}
		old, errOld := os.ReadFile(it.DstPath)
		if errOld == nil && bytes.Equal(old, data) {
			res.Skipped = append(res.Skipped, it.DstName)
			continue
		}
		if err := os.WriteFile(it.DstPath, data, 0o644); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", it.DstName, err))
			continue
		}
		switch {
		case errOld == nil:
			res.Updated = append(res.Updated, it.DstName)
		case os.IsNotExist(errOld):
			res.Copied = append(res.Copied, it.DstName)
		default:
			// 目标存在但读不动（权限 / IO）——文件确实被覆盖了，但语义上不是「新增」。
			res.Errors = append(res.Errors, fmt.Sprintf("%s: unreadable before overwrite: %v", it.DstName, errOld))
			res.Updated = append(res.Updated, it.DstName)
		}
	}

	// 删残留：带前缀但不在本次计划内的一律删除
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read rules directory %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, Prefix) {
			continue // 用户自己的文件，不碰
		}
		if _, ok := want[name]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		res.Deleted = append(res.Deleted, name)
	}
	return res, nil
}

// shortHash 返回字符串的 8 位十六进制短哈希，用于同名文件去重。
func shortHash(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}
