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
		t.Fatal("workbuddy 应可装（复用 codebuddy 验证）")
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

	// SupportedSummary 至少应提及 codebuddy
	if s := SupportedSummary(); s == "" {
		t.Fatal("SupportedSummary 不应为空")
	}
}
