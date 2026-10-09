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
	for _, id := range []string{"claude", "trae", "codex", "opencode"} {
		if IsSupported(id) {
			t.Fatalf("agent %s 尚未验证，不应可装", id)
		}
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
	if got, want := wb.HookFileAbs(""), expandHome("~/.workbuddy/settings.json"); got != want {
		t.Errorf("workbuddy HookFileAbs = %q, want %q", got, want)
	}
	if wb.RulesDir != ".workbuddy/rules" {
		t.Errorf("workbuddy RulesDir = %q, want .workbuddy/rules", wb.RulesDir)
	}
	if wb.HookFileAbs("") == expandHome("~/.codebuddy/settings.json") {
		t.Error("workbuddy 不应再指向 ~/.codebuddy/settings.json")
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

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
