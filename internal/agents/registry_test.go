package agents

import "testing"

// TestSupportedOnlyVerified 校验开关：Supported 只返回 Verified==true 的 agent，
// 未坐实验证的 agent 不在可安装清单里。
func TestSupportedOnlyVerified(t *testing.T) {
	sup := Supported()
	if len(sup) == 0 {
		t.Fatal("Supported 不应为空，至少 codebuddy 应可装")
	}
	for _, a := range sup {
		if !a.Verified {
			t.Fatalf("Supported() 含未验证 agent %s", a.ID)
		}
	}

	if !IsSupported("codebuddy") {
		t.Fatal("codebuddy 应可装")
	}
	if !IsSupported("workbuddy") {
		t.Fatal("workbuddy 应独立可装（独立条目，不再复用 codebuddy）")
	}
	// 以下均未验证，不允许安装
	for _, id := range []string{"claude", "codex", "opencode"} {
		if IsSupported(id) {
			t.Fatalf("agent %s 尚未验证，不应可装", id)
		}
	}
	// trae 经 2026-10-09 canary 坐实（安装目录 + 规则目录均实测通过），现已可装
	if !IsSupported("trae") {
		t.Fatal("trae 应可装（2026-10-09 canary 已坐实：安装目录与 .trae/rules 规则目录）")
	}
	if IsSupported("nope") {
		t.Fatal("未知 agent 不应可装")
	}

	// SupportedSummary 至少应提及 codebuddy / workbuddy
	if s := SupportedSummary(); s == "" {
		t.Fatal("SupportedSummary 不应为空")
	}

	// workbuddy 必须是独立条目，且钩子落到自己的 ~/.workbuddy/settings.json，
	// 不再共享 codebuddy 的 ~/.codebuddy/settings.json。
	wb, ok := Get("workbuddy")
	if !ok {
		t.Fatal("Get(\"workbuddy\") 应解析到独立条目")
	}
	if got, want := wb.HookFileAbs(""), ExpandHome("~/.workbuddy/settings.json"); got != want {
		t.Errorf("workbuddy HookFileAbs = %q, want %q", got, want)
	}
	// 工作区级规则目录与 codebuddy 相同（2026-10-09 三位置探针坐实：WorkBuddy 只读
	// .codebuddy/rules，.workbuddy/rules 不被读）。用户级独立 ≠ 工作区级独立。
	if wb.RulesDir != ".codebuddy/rules" {
		t.Errorf("workbuddy RulesDir = %q, want .codebuddy/rules", wb.RulesDir)
	}
	if wb.HookFileAbs("") == ExpandHome("~/.codebuddy/settings.json") {
		t.Error("workbuddy 的钩子不应指向 ~/.codebuddy/settings.json（钩子仍是自己的 ~/.workbuddy）")
	}

	// codebuddy 不再把 workbuddy 当别名收纳
	cb, _ := Get("codebuddy")
	if contains(cb.Aliases, "workbuddy") {
		t.Error("codebuddy 不应再将 workbuddy 作为别名收纳")
	}
	if !contains(cb.Aliases, "codebuddy-cn") {
		t.Error("codebuddy 应保留 codebuddy-cn 别名")
	}
}

// TestSharingRulesDir：codebuddy 与 workbuddy 共享 .codebuddy/rules ⇒ 必须互相出现在
// 对方的共享列表里；claude / trae 各用各的目录 ⇒ 只匹配到自己；Tier-2 不落盘 ⇒ 同样只有自己。
func TestSharingRulesDir(t *testing.T) {
	ids := func(as []Agent) []string {
		out := make([]string, 0, len(as))
		for _, a := range as {
			out = append(out, a.ID)
		}
		return out
	}

	for _, id := range []string{"codebuddy", "workbuddy"} {
		a, _ := Get(id)
		got := ids(SharingRulesDir(a))
		if !contains(got, "codebuddy") || !contains(got, "workbuddy") {
			t.Errorf("%s 的共享列表应含 codebuddy 与 workbuddy, got %v", id, got)
		}
		if !contains(got, id) {
			t.Errorf("%s 的共享列表应含自己, got %v", id, got)
		}
	}

	// 不共享目录的：只匹配到自己
	for _, id := range []string{"claude", "trae"} {
		a, _ := Get(id)
		got := ids(SharingRulesDir(a))
		if len(got) != 1 || got[0] != id {
			t.Errorf("%s 的共享列表应只有自己, got %v", id, got)
		}
	}

	// Tier-2（RulesDir 为空，不落盘）：只有自己
	for _, id := range []string{"codex", "opencode"} {
		a, _ := Get(id)
		got := ids(SharingRulesDir(a))
		if len(got) != 1 || got[0] != id {
			t.Errorf("%s（Tier-2）的共享列表应只有自己, got %v", id, got)
		}
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// TestSkillsDirAbs 校验 skills 目录解析：相对拼工作区、绝对原样、空返回空。
func TestSkillsDirAbs(t *testing.T) {
	cb, _ := Get("codebuddy")
	if got := cb.SkillsDirAbs("/w"); got != "/w/.codebuddy/skills" {
		t.Fatalf("SkillsDirAbs 应拼工作区，实得 %q", got)
	}
	trae, _ := Get("trae")
	if got := trae.SkillsDirAbs("/w"); got != "" {
		t.Fatalf("未声明 SkillsDir 的 agent 应返回空，实得 %q", got)
	}
}

// TestUserDirsAbs 校验用户级目录 ~ 展开：codebuddy 家族双目录且以 HOME 开头。
func TestUserDirsAbs(t *testing.T) {
	cb, _ := Get("codebuddy")
	home := expandHomeForTest(t)
	skills := cb.UserSkillsDirsAbs()
	wantSkills := []string{home + "/.codebuddy/skills", home + "/.workbuddy/skills"}
	if !equalStrings(skills, wantSkills) {
		t.Fatalf("用户级 skills 目录展开错误：%v", skills)
	}
	rules := cb.UserRulesDirsAbs()
	wantRules := []string{home + "/.codebuddy/rules", home + "/.workbuddy/rules"}
	if !equalStrings(rules, wantRules) {
		t.Fatalf("用户级 rules 目录展开错误：%v", rules)
	}
	wb, _ := Get("workbuddy")
	if !equalStrings(wb.UserSkillsDirsAbs(), skills) {
		t.Fatalf("workbuddy 与 codebuddy 的用户级 skills 目录应相同：%v vs %v", wb.UserSkillsDirsAbs(), skills)
	}
}

// TestDshSkillsDirs 校验 dsh 的 skills 双域（2026-10-14 用户拍板，源码级坐实于
// deepseek-ai/deepseek-harness@master，见 docs/design/external/skill-ecosystem.md
// §三·再续）：工作区 .dsh/skills（rank 100，最高）、用户级 ~/.dsh/skills（rank 400）；
// .agents/* 一律不写；规则侧仍 Tier-2 纯注入（RulesDir 空）⇒ skills 是独立通道。
func TestDshSkillsDirs(t *testing.T) {
	dsh, ok := Get("dsh")
	if !ok {
		t.Fatal("Get(\"dsh\") 应解析到 dsh 条目")
	}
	if dsh.SkillsDir != ".dsh/skills" {
		t.Errorf("dsh SkillsDir = %q, want .dsh/skills", dsh.SkillsDir)
	}
	if got := dsh.SkillsDirAbs("/w/sub"); got != "/w/sub/.dsh/skills" {
		t.Errorf("dsh SkillsDirAbs = %q, want /w/sub/.dsh/skills", got)
	}
	home := expandHomeForTest(t)
	want := []string{home + "/.dsh/skills"}
	if !equalStrings(dsh.UserSkillsDirsAbs(), want) {
		t.Fatalf("dsh 用户级 skills 目录 = %v, want %v", dsh.UserSkillsDirsAbs(), want)
	}
	// 不写 .agents/skills（跨框架公约目录，只读不碰）
	for _, bad := range []string{home + "/.agents/skills", "/w/sub/.agents/skills"} {
		if contains(dsh.UserSkillsDirsAbs(), bad) || dsh.SkillsDirAbs("/w/sub") == bad {
			t.Errorf("dsh 不写 .agents/skills，实得涉及 %s", bad)
		}
	}
	// 规则侧维持 Tier-2 纯注入：没有规则目录可写
	if dsh.RulesDir != "" {
		t.Errorf("dsh RulesDir 应为空（规则纯注入），实得 %q", dsh.RulesDir)
	}
	if dsh.Tier != Tier2 {
		t.Errorf("dsh Tier 应维持 Tier2，实得 %v", dsh.Tier)
	}
	// skills 共享组只有自己（.dsh/skills 不与任何 agent 共享）
	if g := SharingSkillsDir(dsh); len(g) != 1 || g[0].ID != "dsh" {
		t.Errorf("dsh 的 skills 共享组应只有自己，实得 %v", ids(g))
	}
	// 宿主按 .git 定项目根 ⇒ 标记 WorkspaceIsGitRoot（sync 侧据此做 git 根对齐）
	if !dsh.WorkspaceIsGitRoot {
		t.Error("dsh 应标记 WorkspaceIsGitRoot（宿主 findProjectRoot 逐级找 .git）")
	}
	// 其它 agent 不跟这个标记（它们的工作区语义就是钩子载荷/命令行给的目录）
	for _, id := range []string{"codebuddy", "workbuddy", "trae", "claude", "codex", "opencode"} {
		a, _ := Get(id)
		if a.WorkspaceIsGitRoot {
			t.Errorf("%s 不应标记 WorkspaceIsGitRoot", id)
		}
	}
}

// TestSharingSkillsAndUserDirs 校验共享判定：
// codebuddy 与 workbuddy 共享 skills 目录与用户级目录；trae 谁都不共享。
func TestSharingSkillsAndUserDirs(t *testing.T) {
	cb, _ := Get("codebuddy")
	group := SharingSkillsDir(cb)
	if len(group) != 2 || group[1].ID != "workbuddy" {
		t.Fatalf("skills 共享组应含 workbuddy，实得 %v", ids(group))
	}
	ug := SharingUserDirs(cb)
	if len(ug) != 2 || ug[1].ID != "workbuddy" {
		t.Fatalf("用户级共享组应含 workbuddy，实得 %v", ids(ug))
	}

	trae, _ := Get("trae")
	if g := SharingSkillsDir(trae); len(g) != 1 || g[0].ID != "trae" {
		t.Fatalf("trae 的 skills 共享组应只有自己，实得 %v", ids(g))
	}
	if g := SharingUserDirs(trae); len(g) != 1 || g[0].ID != "trae" {
		t.Fatalf("trae 的用户级共享组应只有自己，实得 %v", ids(g))
	}
}

func ids(as []Agent) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.ID
	}
	return out
}

// expandHomeForTest 取当前 HOME（与 ExpandHome 同源），供测试断言绝对路径。
func expandHomeForTest(t *testing.T) string {
	t.Helper()
	got := ExpandHome("~")
	if got == "~" {
		t.Fatal("ExpandHome(\"~\") 应展开为 HOME")
	}
	return got
}
