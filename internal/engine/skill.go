package engine

// skill 同步：把 skill 源**目录树**真实拷贝进 agent 的原生 skills 目录。
//
// 与文件级 Sync 同构同语义（docs/design/features/skill-sync.md）：
//   - 目标名 = 前缀 __rulemux__ + skill 目录名（跨源同名追加源目录短 hash）
//   - 逐文件内容比对：一致 ⇒ 跳过；不一致 / 缺失 ⇒ 覆盖
//   - 带前缀但不在本次计划内的目录 ⇒ 整树删除（删残留，保证加删自由、不累积）
//   - 计划内目录里多出的文件 ⇒ 删除（源里删了文件，副本跟着删）
//   - 不带前缀的目录（用户自己的 skill）一律不碰
//
// 与 Sync 的差异只在载体：skill 是目录（SKILL.md + references/scripts），
// 且 SKILL.md 大小写不敏感（2026-10-12 bundle 实证：
// SKILL_FILE_NAMES=["SKILL.md","skill.md"]，见 docs/design/external/skill-ecosystem.md §四）。

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cq-guojia/rulemux/internal/config"
)

// skillFileNames 是宿主认的 skill 定义文件名（大小写不敏感匹配）。
var skillFileNames = []string{"SKILL.md", "skill.md"}

// skillRef 是一个待同步的 skill：源目录 + 目录名。
type skillRef struct {
	Src  string // skill 源目录（含 SKILL.md）
	Name string // skill 目录名（目标名去前缀后）
}

// hasSkillFile 报告目录下是否存在 SKILL.md（大小写不敏感）。
func hasSkillFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		for _, n := range skillFileNames {
			if strings.EqualFold(e.Name(), n) {
				return true
			}
		}
	}
	return false
}

// maxSkillScanDepth 是自动探测 skill 的最大目录深度（源根自身为第 0 层）。
// shortcut: 探到第 4 层；更深的仓库布局再抬这个常量。
const maxSkillScanDepth = 4

// discoverSkills 在一个源根下自动发现 skill（对齐 npx skills「扫仓库枚举 skill」）：
//   - 根本身含 SKILL.md ⇒ 整个根就是一个 skill（单 skill 布局，不再下钻）；
//   - 否则限深向下扫，遇含 SKILL.md 的目录即认定为一个 skill，并**不再往它内部钻**
//     （其 references/scripts 是自身资源，不是独立 skill）；
//   - 跳过 .git / node_modules 等隐藏或依赖目录，避免无谓遍历。
//
// 这样 `gh:anthropics/skills`（skill 埋在 skills/ 子目录）无需 `#子路径` 也能把
// template / docx / pdf / pptx / xlsx 全部找出；`#子路径` 仍可用于精确指定起点。
func discoverSkills(root string) []skillRef {
	if hasSkillFile(root) {
		return []skillRef{{Src: root, Name: filepath.Base(root)}}
	}
	var out []skillRef
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == root {
			return nil // 根自身已在上面判定；读不了的子目录静默跳过
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			return filepath.SkipDir // 不进 .git / node_modules 等
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		if strings.Count(rel, string(filepath.Separator))+1 > maxSkillScanDepth {
			return filepath.SkipDir
		}
		if hasSkillFile(p) {
			out = append(out, skillRef{Src: p, Name: name})
			return filepath.SkipDir // 是 skill 了，不再往它里面钻
		}
		return nil
	})
	return out
}

// BuildSkillPlan 计算 skill 同步计划。
//
// 每个源根目录下「含 SKILL.md 的子目录」是一个 skill；根目录自身含 SKILL.md 时，
// 根本身也是一个 skill（单技能目录布局同样支持，如 path 直接指向某个 skill）。
// 不存在的源根记入 Plan.Missing（调用方决定怎么报）。
func BuildSkillPlan(dir string, srcs []config.SkillSource) *Plan {
	seen := map[string]bool{} // 源根去重（同一根被多条 skill_source 引用只扫一次）
	var skills []skillRef
	var missing []string
	for _, s := range srcs {
		for _, root := range s.Paths {
			if seen[root] {
				continue
			}
			seen[root] = true
			st, err := os.Stat(root)
			if err != nil || !st.IsDir() {
				missing = append(missing, root)
				continue
			}
			skills = append(skills, discoverSkills(root)...)
		}
	}
	count := map[string]int{}
	for _, sk := range skills {
		count[sk.Name]++
	}
	items := make([]PlanItem, 0, len(skills))
	for _, sk := range skills {
		name := Prefix + sk.Name
		if count[sk.Name] > 1 {
			// 跨源同名 skill：追加源目录短 hash，避免两个 skill 挤同一个目标目录
			name = Prefix + sk.Name + "-" + shortHash(sk.Src)
		}
		items = append(items, PlanItem{
			Src:     sk.Src,
			DstName: name,
			DstPath: filepath.Join(dir, name),
		})
	}
	return &Plan{Dir: dir, Items: items, Missing: missing}
}

// SyncSkills 把 skill 源列表同步进目标 skills 目录。
//
// create 控制「能否创建目标目录」——只有该 agent **自己的会话 hook** 调用时才为 true
// （与规则同步同一条铁律，见 Sync 注释）。create=false 且目录不存在时整次同步立即返回
// （不建、不读、不删）。
func SyncSkills(dir string, srcs []config.SkillSource, create bool) (*SyncResult, error) {
	if _, err := os.Stat(dir); err != nil {
		if !create {
			return &SyncResult{SkippedNoDir: true}, nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create skills directory %s: %w", dir, err)
		}
	}

	plan := BuildSkillPlan(dir, srcs)
	want := make(map[string]struct{}, len(plan.Items))
	for _, it := range plan.Items {
		want[it.DstName] = struct{}{}
	}

	res := &SyncResult{}
	res.Missing = append(res.Missing, plan.Missing...)
	for _, it := range plan.Items {
		_, statErr := os.Stat(it.DstPath)
		existed := statErr == nil
		changed, err := syncTree(it.Src, it.DstPath)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", it.DstName, err))
			continue
		}
		switch {
		case !existed:
			res.Copied = append(res.Copied, it.DstName)
		case changed:
			res.Updated = append(res.Updated, it.DstName)
		default:
			res.Skipped = append(res.Skipped, it.DstName)
		}
	}

	// 删残留：带前缀但不在计划内的一律删（目录整树删；文件也删，防呆）。
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read skills directory %s: %w", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, Prefix) {
			continue // 用户自己的 skill 目录，不碰
		}
		if _, ok := want[name]; ok {
			continue
		}
		p := filepath.Join(dir, name)
		var rerr error
		if e.IsDir() {
			rerr = os.RemoveAll(p)
		} else {
			rerr = os.Remove(p)
		}
		if rerr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", name, rerr))
			continue
		}
		res.Deleted = append(res.Deleted, name)
	}
	return res, nil
}

// syncTree 把 srcRoot 整树同步进 dstRoot：逐文件字节比对，缺/异则写；
// dst 里多出的文件删除，变空的子目录清掉。返回是否有写入 / 删除。
func syncTree(srcRoot, dstRoot string) (bool, error) {
	srcFiles := map[string]struct{}{}
	changed := false

	// 第一遍：走源树，逐文件与目标比对（只持有当前文件内容，不缓存整树）。
	err := filepath.WalkDir(srcRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil // shortcut: 跳过符号链接，只同步真实文件（skill 源应是普通目录树）
		}
		rel, err := filepath.Rel(srcRoot, p)
		if err != nil {
			return err
		}
		srcFiles[rel] = struct{}{}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstRoot, rel)
		old, oldErr := os.ReadFile(dst)
		if oldErr == nil && bytes.Equal(old, data) {
			return nil // 内容一致 ⇒ 跳过
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return changed, err
	}

	// 第二遍：走目标树，删掉源里没有的文件（源里删了文件，副本跟着删 ⇒ 不累积）。
	err = filepath.WalkDir(dstRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // 目标树刚被整体创建前不存在，正常
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dstRoot, p)
		if err != nil {
			return err
		}
		if _, ok := srcFiles[rel]; ok {
			return nil
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return changed, err
	}

	pruneEmptyDirs(dstRoot)
	return changed, nil
}

// pruneEmptyDirs 自底向上删除 root 下的空目录（root 自身除外）。
// 逆字典序保证子目录先于父目录处理（父路径是子路径的前缀，字典序更小）。
func pruneEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == root {
			return nil
		}
		dirs = append(dirs, p)
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		if entries, err := os.ReadDir(d); err == nil && len(entries) == 0 {
			_ = os.Remove(d)
		}
	}
}
