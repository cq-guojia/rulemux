package cmd

import "testing"

// TestParseAgentsRejectsUnverified 校验安装开关：未验证的 agent 被拒绝。
func TestParseAgentsRejectsUnverified(t *testing.T) {
	for _, arg := range []string{"codex", "claude", "trae", "opencode", "claude,codex"} {
		if _, err := parseAgents(arg); err == nil {
			t.Fatalf("未验证的 agent %q 应被拒绝", arg)
		}
	}
}

// TestParseAgentsAcceptsVerified 校验已验证的 agent 可正常解析（含多个）。
func TestParseAgentsAcceptsVerified(t *testing.T) {
	got, err := parseAgents("codebuddy")
	if err != nil {
		t.Fatalf("codebuddy 应可装: %v", err)
	}
	if len(got) != 1 || got[0].ID != "codebuddy" {
		t.Fatalf("解析结果错误: %+v", got)
	}

	got, err = parseAgents("codebuddy,workbuddy")
	if err != nil {
		t.Fatalf("codebuddy,workbuddy 应可装: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应解析出 2 个: %+v", got)
	}
}

// TestParseAgentsUnknown 校验未知 agent 仍报错。
func TestParseAgentsUnknown(t *testing.T) {
	if _, err := parseAgents("nope"); err == nil {
		t.Fatal("未知 agent 应报错")
	}
}
