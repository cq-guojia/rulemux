// Package ui 提供 rulemux CLI 的人类可读输出原语：TTY 感知颜色、状态 glyph、
// 紧凑列表拼接。零第三方依赖（ANSI 转义手写）——保持单二进制离线可构建。
//
// 颜色是双通道编码里的第二通道：glyph（✔ ↑ ✗ ·）本身承载语义，颜色只是增强。
// 非 TTY（重定向 / 管道 / CI 日志）全部原样返回，输出保持可 grep 的纯文本。
package ui

import (
	"fmt"
	"os"
	"strings"
)

// 状态 glyph：无色终端下靠它们区分状态，不依赖颜色。
const (
	GlyphOK      = "✔" // 就绪 / 无变化
	GlyphChanged = "↑" // 本轮有变更
	GlyphFail    = "✗" // 真错误（配置坏了、二进制不在 PATH…）
	GlyphWarn    = "⚠" // 待确认 / 有警告
	GlyphSkip    = "·" // 未初始化 / 不适用 / 跳过
)

// ANSI 属性（手写，禁依赖）。
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiCyan   = "\x1b[36m"
	ansiDim    = "\x1b[2m"
	ansiBold   = "\x1b[1m"
)

// enabled 是进程级颜色开关，首用前判定一次。
var enabled = detectEnabled(os.Stderr)

// detectEnabled 判定颜色是否启用：NO_COLOR 非空 / TERM=dumb / w 不是字符设备
// （重定向或管道）时关闭。
func detectEnabled(w *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := w.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Enabled 报告颜色当前是否启用（测试与调试用）。
func Enabled() bool { return enabled }

func paint(code, s string) string {
	if !enabled || s == "" {
		return s
	}
	return code + s + ansiReset
}

// Green 绿：就绪 / 同步完成。
func Green(s string) string { return paint(ansiGreen, s) }

// Yellow 黄：本轮有变更 / 待确认。
func Yellow(s string) string { return paint(ansiYellow, s) }

// Red 红：错误 / 未初始化（仅真错误）。
func Red(s string) string { return paint(ansiRed, s) }

// Dim 灰暗：跳过、未初始化、次要信息。
func Dim(s string) string { return paint(ansiDim, s) }

// Bold 加粗：标题与总计。
func Bold(s string) string { return paint(ansiBold, s) }

// Cyan 青：段标题基色（doctor 的 Install / Config / Agents）。
func Cyan(s string) string { return paint(ansiCyan, s) }

// Header 段标题：粗体 + 青。与状态色（绿/黄/红）区分，无色终端下降级为纯文本。
func Header(s string) string { return paint(ansiBold+ansiCyan, s) }

// HomeRel 把路径压缩成 ~ 开头的显示形式（HOME 前缀替换），
// 取不到 HOME 或路径不在其下时原样返回。
func HomeRel(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	prefix := home + string(os.PathSeparator)
	if strings.HasPrefix(p, prefix) {
		return "~" + p[len(home):] // p[len(home):] 以分隔符开头，直接拼出 ~/…
	}
	return p
}

// TruncateJoin 用 ", " 拼接 items；超过 max 个时截断为 "a, b, …+N more"。
// max <= 0 视为不截断。
func TruncateJoin(items []string, max int) string {
	if max <= 0 || len(items) <= max {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, …+%d more", strings.Join(items[:max], ", "), len(items)-max)
}

// Plural 返回 "n word"（n != 1 时 word 加 s）——消灭 "1 workspace(s)" 这种丑态。
func Plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
