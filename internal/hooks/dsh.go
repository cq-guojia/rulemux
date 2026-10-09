// DSH（DeepSeek Harness）的「安装形态」：它不是装一条 SessionStart 钩子，而是把一个小型原生
// Cordis 插件注册进 $DSH_HOME/cordis.patch.yml。dsh 顶层把该文件当 YAML 数组读取，任何非法内容
// 会让它启动失败，因此写入必须严格：保留他人 patch 行、我方用标记块幂等增删、删空后写回 []。
//
// 参照实现：hindsight 的 coding-agents/src/installer.ts（DSH 安装器）——
//   - patch 行 name 必须是 file:// URL（Cordis 按 ES module specifier 解析，裸绝对路径会被静默跳过）；
//   - 标记块 + 空表归一（installer.ts:1716-1740）；
//   - dsh home = $DSH_HOME 否则 ~/.dsh（installer.ts:1742-1745）。
package hooks

import (
	"bytes"
	_ "embed"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
)

// dshPluginJS 是随二进制分发的 Cordis 插件（go:embed），init 时释放到 <dshHome>/rulemux/。
//
//go:embed assets/rulemux-dsh.js
var dshPluginJS []byte

const (
	dshMarkerStart = "# RULEMUX_DSH_START"
	dshMarkerEnd   = "# RULEMUX_DSH_END"
)

var (
	// 剔除标记块（连同其两侧多余换行）：替换式写入，保证重复安装只留一份、且能修好失效路径。
	dshBlockRe = regexp.MustCompile(`(?s)\n?` + regexp.QuoteMeta(dshMarkerStart) + `.*?` + regexp.QuoteMeta(dshMarkerEnd) + `\n?`)
	// 取出标记块本身（用于读取已装 URL）。
	dshBlockFindRe = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(dshMarkerStart) + `.*?` + regexp.QuoteMeta(dshMarkerEnd))
	// 从标记块里取 name 的值。
	dshNameRe = regexp.MustCompile(`(?m)^\s*name:\s*"([^"]*)"`)
)

// dshPluginPath 返回插件文件落点：<dshHome>/rulemux/rulemux-dsh.js。
// dsh 的 home 与工作区无关（HookAbs=true），故 ws 传空即可。
func dshPluginPath(a agents.Agent) string {
	patch, _ := a.HookFileAbsWithSource("")
	return filepath.Join(filepath.Dir(patch), "rulemux", "rulemux-dsh.js")
}

// dshPluginURL 返回插件文件的 file:// URL（patch 行 name 的值）。
func dshPluginURL(a agents.Agent) string {
	return fileURL(dshPluginPath(a))
}

// fileURL 把一个绝对路径转成 file:// URL（含百分号转义），与 Node 的 pathToFileURL 等价。
func fileURL(p string) string {
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// dshBlock 生成我方在 cordis.patch.yml 里的标记块。
func dshBlock(pluginURL string) string {
	return dshMarkerStart + "\n" +
		"- insert:\n" +
		"    - id: rulemux\n" +
		"      name: " + strconv.Quote(pluginURL) + "\n" +
		dshMarkerEnd
}

// dshUserContent 返回「用户自己的 patch 内容」：剔除我方标记块，并把空表占位 [] 归一为空串。
// [] 是上一次卸载留下的占位（非用户内容）；若不归一，安装时会与我们的块拼成两个 YAML 文档，
// dsh 将拒绝解析、启动失败（installer.ts:1727-1739 的同款坑）。
func dshUserContent(existing string) string {
	others := strings.TrimSpace(dshBlockRe.ReplaceAllString(existing, "\n"))
	if others == "[]" {
		return ""
	}
	return others
}

// writeDshPluginFile 写出/更新插件文件（内容一致则不写，保持幂等）。
func writeDshPluginFile(path string) error {
	if b, err := os.ReadFile(path); err == nil && bytes.Equal(b, dshPluginJS) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, dshPluginJS, 0o644)
}

// installDshPlugin 释放插件文件并把 patch 行写入 cordis.patch.yml（幂等），返回 patch 路径。
func installDshPlugin(a agents.Agent, ws string) (string, error) {
	patchPath, _ := a.HookFileAbsWithSource(ws)
	pluginPath := dshPluginPath(a)
	if err := writeDshPluginFile(pluginPath); err != nil {
		return patchPath, err
	}

	existing := ""
	if b, err := os.ReadFile(patchPath); err == nil {
		existing = string(b)
	} else if !os.IsNotExist(err) {
		return patchPath, err
	}

	block := dshBlock(fileURL(pluginPath))
	if strings.Contains(existing, block) {
		return patchPath, nil // 已是最新：不写盘
	}

	others := dshUserContent(existing)
	out := block + "\n"
	if others != "" {
		out = others + "\n\n" + block + "\n"
	}
	if err := os.MkdirAll(filepath.Dir(patchPath), 0o755); err != nil {
		return patchPath, err
	}
	if err := os.WriteFile(patchPath, []byte(out), 0o644); err != nil {
		return patchPath, err
	}
	return patchPath, nil
}

// inspectDsh 报告 patch 里是否已有我方块，以及其 name（URL）。只读。
func inspectDsh(patchPath string) (installed bool, url string, err error) {
	b, err := os.ReadFile(patchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", err
	}
	block := dshBlockFindRe.FindString(string(b))
	if block == "" {
		return false, "", nil
	}
	if m := dshNameRe.FindStringSubmatch(block); m != nil {
		return true, m[1], nil
	}
	return true, "", nil
}

// refreshDsh 把已存在的我方块重写为当前插件 URL（仅当过期），并确保插件文件在盘。
// 未装 → 不创建（与其它 agent 的刷新口径一致）。
func refreshDsh(patchPath string, a agents.Agent) (changed bool, err error) {
	installed, url, err := inspectDsh(patchPath)
	if err != nil || !installed {
		return false, err
	}
	pluginPath := dshPluginPath(a)
	want := fileURL(pluginPath)
	if url == want {
		return false, writeDshPluginFile(pluginPath)
	}
	if err := writeDshPluginFile(pluginPath); err != nil {
		return false, err
	}
	b, err := os.ReadFile(patchPath)
	if err != nil {
		return false, err
	}
	block := dshBlock(want)
	out := block + "\n"
	if others := dshUserContent(string(b)); others != "" {
		out = others + "\n\n" + block + "\n"
	}
	if err := os.WriteFile(patchPath, []byte(out), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// uninstallDsh 移除我方块与插件文件；保留他人 patch 行。删空后写回 []（dsh 顶层必须是数组）。
func uninstallDsh(patchPath string, a agents.Agent) error {
	b, err := os.ReadFile(patchPath)
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.Remove(dshPluginPath(a))
			return nil
		}
		return err
	}
	_ = os.Remove(dshPluginPath(a))
	if others := dshUserContent(string(b)); others != "" {
		return os.WriteFile(patchPath, []byte(others+"\n"), 0o644)
	}
	return os.WriteFile(patchPath, []byte("[]\n"), 0o644)
}
