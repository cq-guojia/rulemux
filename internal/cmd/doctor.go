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
	"github.com/cq-guojia/rulemux/internal/state"
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
	fmt.Println(ui.Dim("workspace: " + ui.HomeRel(ws)))
	fmt.Println()

	// 1. Install：怎么装的、装在哪、在不在 PATH
	printInstall(in)
	fmt.Println()

	// 2. Config：配置文件在哪、合法性、源清单（每源一行，带存在性）
	cfgOK := printConfig(cfgPath, cfg, cfgErr, ws)
	fmt.Println()

	// 3. Agents：初始化了哪些（每 agent 一行；在用但未初始化的折叠一行；没用过的不提）
	printAgents(cfg, ws)

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
	fmt.Println(ui.Header("Install"))
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

// printConfig 打 Config 段：配置文件在哪、合法性、它管理哪些文件。受管文件按
// rules（规则文件）/ skills（skill 目录）分组罗列，只报路径与存在性——投给哪些
// agent、在哪些 workspace 生效这类过滤条件不在这里展开（health check 不承担
// 配置教程）；被 workspace 范围跳过的源折叠成一行暗色说明。
// 同一文件可能被多个 source 声明（file_group 继承，如 base 被三层继承），清单
// 按路径去重保序——这里只回答「管哪些文件」，不问声明次数。
// 返回配置是否可用（合法）。
func printConfig(cfgPath string, cfg *config.Config, cfgErr error, ws string) bool {
	fmt.Println(ui.Header("Config"))
	if cfgErr != nil {
		fmt.Printf("  %s %v\n", ui.Red(ui.GlyphFail), cfgErr)
		fmt.Println("    Hint: run rulemux init first")
		return false
	}
	mark, note := ui.Green(ui.GlyphOK), "valid"
	if verr := cfg.Validate(); verr != nil {
		mark, note = ui.Yellow(ui.GlyphWarn), verr.Error()
	}
	var ruleLists, skillLists [][]string
	var skipped int
	for _, s := range cfg.Sources {
		ruleLists = append(ruleLists, s.Paths)
		if !s.MatchesWorkspace(ws) {
			skipped++
		}
	}
	for _, s := range cfg.SkillSources {
		skillLists = append(skillLists, s.Paths)
		if !s.MatchesWorkspace(ws) {
			skipped++
		}
	}
	ruleFiles := dedupePaths(ruleLists...)
	skillDirs := dedupePaths(skillLists...)
	fmt.Printf("  %s %s — %s · %s · %s\n",
		mark, ui.HomeRel(cfgPath), note,
		ui.Plural(len(ruleFiles), "rule file"), ui.Plural(len(skillDirs), "skill dir"))
	if len(ruleFiles) > 0 {
		fmt.Println("    rules")
		for _, p := range ruleFiles {
			fmt.Printf("      %s %s\n", pathMark(p, false), ui.HomeRel(p))
		}
	}
	if len(skillDirs) > 0 {
		fmt.Println("    skills")
		for _, p := range skillDirs {
			fmt.Printf("      %s %s\n", pathMark(p, true), ui.HomeRel(p))
		}
	}
	if skipped > 0 {
		fmt.Printf("    %s %s scoped to other workspaces — skipped in this workspace\n",
			ui.Dim(ui.GlyphSkip), ui.Plural(skipped, "source"))
	}
	return true
}

// dedupePaths 拉平多组路径并按路径去重（保序）：同一文件被多个 source 声明时
// （file_group 继承的常态），doctor 清单只列一次。
func dedupePaths(lists ...[]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, list := range lists {
		for _, p := range list {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

// pathMark 存在性标记：规则文件 / skill 目录在盘上 ⇒ 绿 ✔，否则黄 ⚠。
// wantDir 为 true 时还要求是目录（skill 源指向文件属配置错误）。
func pathMark(p string, wantDir bool) string {
	if st, err := os.Stat(p); err == nil && (!wantDir || st.IsDir()) {
		return ui.Green(ui.GlyphOK)
	}
	return ui.Yellow(ui.GlyphWarn)
}

// agentInitialized 报告该 agent 是否已初始化：钩子已装（latest 与否另见
// agentStateLine）。external 样式没有 rulemux 可装的钩子——插件由宿主装、rulemux
// 探测不到，若它还有规则目录就以目录存在为据；没有目录的（dsh：纯注入，规则由插件
// 每个会话现取现注入，盘上什么都不留）**永远判不出来**，由 printAgents 单列一行
// 说明，而不是当「未初始化」报——判错还会把用户支去跑一个修不好它的 init。
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

// printAgents 报告 agent 的初始化状态（2026-10-14 用户拍板：没用过的一律不出现）：
//   - 已初始化的（钩子已装；external 的规则目录存在）逐行列出；
//   - 在用的（账本记录 ∪ 配置点名 − off）但未初始化 ⇒ 折叠一行提示；
//   - 从没用过的一律不提，也不计入分母。
//
// 已卸载（off）的同样不提：卸载即断联，不出现、不 nag。
func printAgents(cfg *config.Config, ws string) {
	off := state.Load().Off
	// 「在用」的判据在 doctor 里要比裸 sync 宽一档：只问「这个工作区有没有用过它」，
	// 不按 Tier 过滤 —— 注入型（dsh）没有目录也没有钩子，只有这条判据看得见它。
	mention := mentionedInUse(cfg, ws)
	inUse := func(a agents.Agent) bool {
		for _, id := range append([]string{a.ID}, a.Aliases...) {
			if mention[strings.ToLower(id)] {
				return true
			}
		}
		return false
	}

	var inited, unined, injectionOnly []agents.Agent
	visible := 0
	for _, a := range agents.All() {
		if containsFold(off, a.ID) {
			continue
		}
		switch {
		case agentInitialized(a, ws):
			inited = append(inited, a)
		case a.Tier == agents.Tier2 && a.Style == "external":
			// 注入型且探测不到安装状态（dsh）：工作区里没有可初始化的东西。在用就提一句
			// 它由宿主侧插件交付，不用就不提。
			if !inUse(a) {
				continue
			}
			injectionOnly = append(injectionOnly, a)
		case inUse(a):
			unined = append(unined, a)
		default:
			continue // 从没用过：既不计入分母，也不列名
		}
		visible++
	}

	if visible == 0 {
		// 名单收窄后（2026-10-14）分母只算在用的 agent：一个都没有时别印「0 of 0」。
		fmt.Println(ui.Header("Agents — none in use in this workspace"))
		return
	}
	fmt.Println(ui.Header(fmt.Sprintf("Agents — %d of %d initialized", len(inited), visible)))
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
	for _, a := range injectionOnly {
		fmt.Printf("  %s %-10s delivered by the host-side plugin (rulemux renders, the plugin injects) — nothing to initialize here\n",
			ui.Dim(ui.GlyphSkip), a.ID)
	}
}

// agentStateLine 只报已初始化 agent 的钩子健康度（2026-10-13 用户拍版二版：
// 「我理解你下面的 Agent 就是做判断对吧？……判断它的钩子正不正常，不就行了吗？
// 搞这么多干嘛呢？」）。规则/skill 目录的存在性不再罗列——sync 已经报过，doctor
// 不重复刷屏。external 样式没有 rulemux 可装的钩子——插件由宿主装、rulemux 探测不到，
// 有规则目录的以其存在为据（见 agentInitialized）；dsh 这种纯注入的连目录都没有，
// 由 printAgents 单列一行说明（这里的 external 分支对它不会命中）。
func agentStateLine(a agents.Agent, ws string) string {
	if a.Style == "external" {
		return "plugin (external)"
	}
	hookPath, envUsed := a.HookFileAbsWithSource(ws)
	srcNote := ""
	if envUsed != "" {
		srcNote = fmt.Sprintf(" [dir from $%s]", envUsed)
	}
	installed, cmdStr, err := hooks.Inspect(hookPath, a)
	switch {
	case errors.Is(err, hooks.ErrRefreshUnsupported):
		return "hook unchecked (format not verified yet)" + srcNote
	case err != nil:
		return fmt.Sprintf("hook unreadable → %s: %v", hookPath, err)
	case !installed:
		return "hook not installed → rulemux init --agent " + a.ID
	case cmdStr == hooks.ExpectedCommand(a, cmdStr):
		return "hook up to date" + srcNote
	default:
		return ui.Yellow("hook OUTDATED → rulemux init --refresh")
	}
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
