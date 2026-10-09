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

	// workbuddy 是 codebuddy 的别名（同一 agent、同一目录、同一钩子）⇒ 去重成 1 个。
	got, err = parseAgents("codebuddy,workbuddy")
	if err != nil {
		t.Fatalf("codebuddy,workbuddy 应可装: %v", err)
	}
	if len(got) != 1 || got[0].ID != "codebuddy" {
		t.Fatalf("别名应去重为同一个 codebuddy: %+v", got)
	}

	// 单独写别名同样解析为规范 ID。
	got, err = parseAgents("workbuddy")
	if err != nil {
		t.Fatalf("别名 workbuddy 应可装: %v", err)
	}
	if len(got) != 1 || got[0].ID != "codebuddy" {
		t.Fatalf("别名应解析为规范 ID codebuddy: %+v", got)
	}
}

// TestParseAgentsUnknown 校验未知 agent 仍报错。
func TestParseAgentsUnknown(t *testing.T) {
	if _, err := parseAgents("nope"); err == nil {
		t.Fatal("未知 agent 应报错")
	}
}
