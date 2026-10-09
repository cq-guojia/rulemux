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
