package engine

import (
	"bytes"
	"os"
	"strings"

	"github.com/cq-guojia/rulemux/internal/config"
)

// managedHeader 标记这段内容由 rulemux 注入，便于在上下文里识别来源。
const managedHeader = "<!-- rulemux:managed -->"

// RenderInject 把源文件内容拼成待注入的文本。
//
// 用于 Tier-2（只认单文件 AGENTS.md、无规则目录的 agent，如 Codex / OpenCode）：
// 由 SessionStart 钩子把本函数的输出交给 harness 注入上下文，
// 从而做到「不碰用户自己的 AGENTS.md」（设计 #9/#12）。
func RenderInject(srcs []config.Source) (string, error) {
	var b strings.Builder
	b.WriteString(managedHeader)
	b.WriteString("\n")
	for _, s := range srcs {
		for _, p := range s.Paths {
			data, err := os.ReadFile(p)
			if err != nil {
				// 单个源文件缺失不影响整体注入
				continue
			}
			b.WriteString("\n")
			b.Write(data)
			if !bytes.HasSuffix(data, []byte("\n")) {
				b.WriteString("\n")
			}
		}
	}
	return b.String(), nil
}
