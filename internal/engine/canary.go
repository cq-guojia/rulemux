package engine

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"time"
)

// CanaryName 是 canary 探针文件的固定名字。
// 它带 .rulemux__ 前缀，因此下一次 sync 会把它当残留自动清掉，不会污染规则目录。
const CanaryName = Prefix + "canary.md"

// CanaryToken 生成一个本次运行唯一的暗号。
func CanaryToken() string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())))
	return fmt.Sprintf("RULEMUX-CANARY-%016x", h.Sum64())
}

// WriteCanary 在规则目录写入 canary 探针，返回写入的文件路径。
func WriteCanary(dir, token string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, CanaryName)
	content := "# rulemux canary\n\n" +
		"这是 rulemux 的验收探针。如果你能在上下文里读到下面这行暗号，\n" +
		"说明「SessionStart 钩子复制 → 本会话加载」这条链路是通的。\n\n" +
		"暗号：" + token + "\n"
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
