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
