// Package engine 是 rulemux 的核心引擎：被各 agent 的适配器调用的那套统一方法。
//
// 对应设计（docs/design/implementation.md #10/#11）：
//   - 目标文件名 = 前缀 .rulemux__ + 源 basename（同名冲突时追加源路径短 hash）
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
const Prefix = ".rulemux__"

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
}

// IsEmpty 报告本次同步是否没有任何变更（全是跳过或压根没有源）。
func (r *SyncResult) IsEmpty() bool {
	return len(r.Copied)+len(r.Updated)+len(r.Deleted)+len(r.Missing) == 0
}

// Sync 把源列表同步进目标目录。整个过程无状态文件，靠内容比对 + 前缀删残留保证幂等。
func Sync(dir string, srcs []config.Source) (*SyncResult, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建规则目录 %s 失败: %w", dir, err)
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
		old, errOld := os.ReadFile(it.DstPath)
		if errOld == nil && bytes.Equal(old, data) {
			res.Skipped = append(res.Skipped, it.DstName)
			continue
		}
		if err := os.WriteFile(it.DstPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("写入 %s 失败: %w", it.DstPath, err)
		}
		if errOld == nil {
			res.Updated = append(res.Updated, it.DstName)
		} else {
			res.Copied = append(res.Copied, it.DstName)
		}
	}

	// 删残留：带前缀但不在本次计划内的一律删除
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取规则目录 %s 失败: %w", dir, err)
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
			return nil, fmt.Errorf("删除残留 %s 失败: %w", name, err)
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
