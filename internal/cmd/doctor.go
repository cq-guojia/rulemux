package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/hooks"
	"github.com/cq-guojia/rulemux/internal/ui"
)

// Doctor 环境自检。四段式（2026-10-13 用户拍版）：前 10 行直接回答四个问题——
// 怎么装的、装在哪、初始化了哪些 agent、配置文件在哪；末尾汇总 + 下一步。
// 未安装/未初始化的 agent 不打叉，折叠成一行暗色名单（初始化多了也不刷屏）。
func Doctor(args []string) int {
	f := ParseFlags(args)
	cfgPath := f.Get("config", config.DefaultPath())

	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}

	in := detectInstall()
	cfg, cfgErr := config.Load(cfgPath)

	fmt.Println(ui.Bold("rulemux doctor — environment check"))
	fmt.Println(ui.Dim("workspace: " + ws))
	fmt.Println()

	// 1. Install：怎么装的、装在哪、在不在 PATH
	printInstall(in)
	fmt.Println()

	// 2. Config：配置文件在哪、合法性、源清单（每源一行，带存在性）
	cfgOK := printConfig(cfgPath, cfg, cfgErr, ws)
	fmt.Println()

	// 3. Agents：初始化了哪些（每 agent 一行；未初始化的折叠一行）
	printAgents(ws)

	// 4. 汇总 + 下一步
	printDoctorSummary(cfgOK, in.OnPATH)
	return 0
}

// installInfo 是「怎么装的、装在哪」的判定结果。
type installInfo struct {
	Method     string // npm / pnpm / yarn / go install / standalone binary (manual / GitHub Release)
	Version    string
	Binary     string // 解析符号链接后的真实可执行文件
	PackageDir string // npm 家族布局时的包目录（判定得出才非空）
	OnPATH     bool
	PathBin    string
}

// detectInstall 判定安装方式与路径：只看可执行文件的真实路径（npm 布局含
// node_modules、go install 落在 go/bin），PATH 单查一条。零文件系统魔法。
func detectInstall() installInfo {
	in := installInfo{Version: Version}
	if bin, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(bin); err == nil {
			bin = resolved
		}
		in.Binary = bin
		in.Method, in.PackageDir = installMethod(bin)
	}
	if p, err := exec.LookPath("rulemux"); err == nil {
		in.OnPATH = true
		in.PathBin = p
	}
	return in
}

// installMethod 按可执行文件路径判定安装方式（纯函数，表驱动可测）。
func installMethod(bin string) (method, pkgDir string) {
	if strings.Contains(bin, "/node_modules/") {
		switch {
		case strings.Contains(bin, "/.pnpm/"):
			method = "pnpm"
		case strings.Contains(bin, "/yarn/"):
			method = "yarn"
		default:
			method = "npm"
		}
		// 包目录：截到最后一个 node_modules/<包名>
		const marker = "/node_modules/"
		if i := strings.LastIndex(bin, marker); i >= 0 {
			rest := bin[i+len(marker):]
			if j := strings.Index(rest, "/"); j > 0 {
				pkgDir = bin[:i+len(marker)] + rest[:j]
			}
		}
		return method, pkgDir
	}
	if strings.Contains(bin, "/go/bin/") {
		return "go install", ""
	}
	return "standalone binary (manual / GitHub Release)", ""
}

func printInstall(in installInfo) {
	fmt.Println(ui.Bold("Install"))
	mark := ui.Green(ui.GlyphOK)
	if !in.OnPATH {
		mark = ui.Red(ui.GlyphFail) // exec 式钩子要求 rulemux 在 PATH 上，这是真错误
	}
	fmt.Printf("  %s rulemux %s — %s\n", mark, in.Version, in.Method)
	fmt.Printf("    binary    %s\n", in.Binary)
	if in.PackageDir != "" {
		fmt.Printf("    package   %s\n", in.PackageDir)
	}
	if in.OnPATH {
		fmt.Printf("    PATH      %s %s\n", ui.Green(ui.GlyphOK), in.PathBin)
	} else {
		fmt.Printf("    PATH      %s not found — exec-style hooks will fail\n", ui.Red(ui.GlyphFail))
	}
}

// printConfig 打 Config 段：路径 + 计数 + 校验结论 + 源清单（每源一行）。
// 返回配置是否可用（合法）。
func printConfig(cfgPath string, cfg *config.Config, cfgErr error, ws string) bool {
	fmt.Println(ui.Bold("Config"))
	if cfgErr != nil {
		fmt.Printf("  %s %v\n", ui.Red(ui.GlyphFail), cfgErr)
		fmt.Println("    Hint: run rulemux init first")
		return false
	}
	mark, note := ui.Green(ui.GlyphOK), "valid"
	if verr := cfg.Validate(); verr != nil {
		mark, note = ui.Yellow(ui.GlyphWarn), verr.Error()
	}
	fmt.Printf("  %s %s — %s · %s · %s\n",
		mark, cfgPath, ui.Plural(len(cfg.Sources), "source"), ui.Plural(len(cfg.SkillSources), "skill source"), note)
	for _, s := range cfg.Sources {
		target := "all agents"
		if len(s.Agents) > 0 {
			target = strings.Join(s.Agents, ", ")
		}
		scope := "all workspaces"
		if len(s.Workspaces) > 0 {
			scope = strings.Join(s.Workspaces, ", ")
		}
		skip := ""
		if !s.MatchesWorkspace(ws) {
			skip = " ← skipped: not the current workspace"
		}
		for _, p := range s.Paths {
			fileMark := ui.Green(ui.GlyphOK)
			if _, err := os.Stat(p); err != nil {
				fileMark = ui.Yellow(ui.GlyphWarn)
			}
			fmt.Printf("    %s %s → %s [%s]%s\n", fileMark, ui.HomeRel(p), target, scope, skip)
		}
	}
	for _, s := range cfg.SkillSources {
		target := "all agents"
		if len(s.Agents) > 0 {
			target = strings.Join(s.Agents, ", ")
		}
		for _, p := range s.Paths {
			dirMark := ui.Green(ui.GlyphOK)
			if st, err := os.Stat(p); err != nil || !st.IsDir() {
				dirMark = ui.Yellow(ui.GlyphWarn)
			}
			fmt.Printf("    %s %s → %s [skills]\n", dirMark, ui.HomeRel(p), target)
		}
	}
	return true
}

// agentInitialized 报告该 agent 是否已初始化：钩子已装（latest 与否另见
// agentStateLine）。external 样式（如 dsh）没有 rulemux 可装的钩子——插件由
// 宿主装、rulemux 探测不到，以其规则目录存在为据。
func agentInitialized(a agents.Agent, ws string) bool {
	if a.Style == "external" {
		if dir := a.RulesDirAbs(ws); dir != "" {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				return true
			}
		}
		return false
	}
	installed, _, err := hooks.Inspect(a.HookFileAbs(ws), a)
	return err == nil && installed
}

func printAgents(ws string) {
	all := agents.All()
	var inited, unined []agents.Agent
	for _, a := range all {
		if agentInitialized(a, ws) {
			inited = append(inited, a)
		} else {
			unined = append(unined, a)
		}
	}
	fmt.Println(ui.Bold(fmt.Sprintf("Agents — %d of %d initialized", len(inited), len(all))))
	for _, a := range inited {
		fmt.Printf("  %s %-10s %s\n", ui.Green(ui.GlyphOK), a.ID, agentStateLine(a, ws))
	}
	if len(unined) > 0 {
		ids := make([]string, 0, len(unined))
		for _, a := range unined {
			ids = append(ids, a.ID)
		}
		fmt.Printf("  %s not initialized: %s — rulemux init --agent <id>\n",
			ui.Dim(ui.GlyphSkip), strings.Join(ids, ", "))
	}
}

// agentStateLine 生成已初始化 agent 的行内状态：钩子状态 + 各目录存在性
// （存在的照常，缺失的暗色 (missing)）——一行一个 agent，杜绝 6-8 行刷屏。
func agentStateLine(a agents.Agent, ws string) string {
	var parts []string
	if a.Style == "external" {
		parts = append(parts, "plugin (external)")
	} else {
		hookPath, envUsed := a.HookFileAbsWithSource(ws)
		srcNote := ""
		if envUsed != "" {
			srcNote = fmt.Sprintf(" [dir from $%s]", envUsed)
		}
		installed, cmdStr, err := hooks.Inspect(hookPath, a)
		switch {
		case errors.Is(err, hooks.ErrRefreshUnsupported):
			parts = append(parts, "hook unchecked (format not verified yet)"+srcNote)
		case err != nil:
			parts = append(parts, fmt.Sprintf("hook unreadable → %s: %v", hookPath, err))
		case !installed:
			parts = append(parts, "hook not installed → rulemux init --agent "+a.ID)
		case cmdStr == hooks.ExpectedCommand(a, cmdStr):
			parts = append(parts, "hook up to date")
		default:
			parts = append(parts, "hook OUTDATED → rulemux init --refresh")
		}
	}
	if dir := a.RulesDirAbs(ws); dir != "" {
		parts = append(parts, "rules "+dirMark(dir, ws))
	}
	if dir := a.SkillsDirAbs(ws); dir != "" {
		parts = append(parts, "skills "+dirMark(dir, ws))
	}
	for _, dir := range a.UserRulesDirsAbs() {
		parts = append(parts, "user rules "+dirMark(dir, ws))
	}
	for _, dir := range a.UserSkillsDirsAbs() {
		parts = append(parts, "user skills "+dirMark(dir, ws))
	}
	return strings.Join(parts, " · ")
}

// dirMark 目录存在性标记：存在 ⇒ "路径 ✓"；缺失 ⇒ "路径 (missing)"。
// 路径显示口径：工作区下的目录相对 ws（.codebuddy/rules），用户级 ~ 压缩。
func dirMark(dir, ws string) string {
	shown := dir
	if rel, err := filepath.Rel(ws, dir); err == nil && !strings.HasPrefix(rel, "..") {
		shown = rel
	} else {
		shown = ui.HomeRel(dir)
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return shown + " ✓"
	}
	return shown + " (missing)"
}

func printDoctorSummary(cfgOK, onPATH bool) {
	var parts []string
	if cfgOK {
		parts = append(parts, ui.Green("config valid"))
	} else {
		parts = append(parts, ui.Red("config INVALID"))
	}
	if onPATH {
		parts = append(parts, ui.Green("binary on PATH"))
	} else {
		parts = append(parts, ui.Red("binary NOT on PATH"))
	}
	fmt.Println()
	fmt.Fprintln(os.Stdout, ui.Bold(strings.Join(parts, " · ")))
	fmt.Println("next: rulemux verify — confirm rules actually load in a live session")
}
