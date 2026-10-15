package engine

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"time"
)

// CanaryName 是 canary 探针文件的固定名字。
// 它带 __rulemux__ 前缀，因此下一次 sync 会把它当残留自动清掉，不会污染规则目录。
const CanaryName = Prefix + "canary.md"

// CanaryToken 生成一个本次运行唯一的暗号。
func CanaryToken() string {
	h := fnv.New64a()
	_, _ = h.Write(fmt.Appendf(nil, "%d-%d", time.Now().UnixNano(), os.Getpid()))
	return fmt.Sprintf("RULEMUX-CANARY-%016x", h.Sum64())
}

// WriteCanary 在规则目录写入 canary 探针，返回写入的文件路径。
// 探针带 AutoApplyFrontmatter 头，确保 frontmatter-gated 的 agent（CodeBuddy/WorkBuddy）
// 也会在会话开始加载它，否则 canary 会误报「未生效」。
func WriteCanary(dir, token string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, CanaryName)
	content := AutoApplyFrontmatter + "# rulemux canary\n\n" +
		"This is rulemux's acceptance probe. If you can read the token below from your rules,\n" +
		"then the \"SessionStart hook copy -> loaded in this session\" chain works.\n\n" +
		"Token: " + token + "\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// CleanCanary 删除规则目录里的 canary 探针；目录或文件不存在视为已清理。
func CleanCanary(dir string) error {
	err := os.Remove(filepath.Join(dir, CanaryName))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

// SkillCanaryDirName 是 skill canary 探针目录的名字（一个带 __rulemux__ 前缀的技能目录，
// 内含 SKILL.md）。带前缀 ⇒ 下次 sync 会把它当残留自动清掉，不会污染 skills 目录。
const SkillCanaryDirName = Prefix + "canary-skill"

// WriteSkillCanary 在 skills 目录写入一颗技能探针（含 SKILL.md），返回探针目录路径。
// 探针带 Anthropic 简式 frontmatter（name + description，Trae/Claude/dsh 通用），
// description 内嵌暗号，用于验证技能「是否真被扫描并可在会话中按名调用」。
func WriteSkillCanary(dir, token string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, SkillCanaryDirName)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	content := "---\n" +
		"name: " + SkillCanaryDirName + "\n" +
		"description: rulemux skill canary. If this skill appears in your skill list and can be invoked by name, the skills delivery chain works. Token: " + token + "\n" +
		"---\n"
	if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(content), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// CleanSkillCanary 删除 skills 目录里的技能探针；目录或文件不存在视为已清理。
// 探针清掉后若父 skills 目录变空，一并删掉（与 verify 的 removeEmptyLeaf 同规，
// 否则「探针建出来的空目录」会留在盘上成为下次回填的位置）。
func CleanSkillCanary(dir string) error {
	p := filepath.Join(dir, SkillCanaryDirName)
	if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
	return nil
}
