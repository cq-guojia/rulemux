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

	// codebuddy 与 workbuddy 已是两条独立 agent（各有自己的目录与钩子文件），
	// 并列时解析为两个不同条目，不再去重合并。
	got, err = parseAgents("codebuddy,workbuddy")
	if err != nil {
		t.Fatalf("codebuddy,workbuddy 应可装: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("两个独立 agent 不应被合并: %+v", got)
	}
	ids := map[string]bool{}
	for _, a := range got {
		ids[a.ID] = true
	}
	if !ids["codebuddy"] || !ids["workbuddy"] {
		t.Fatalf("codebuddy 与 workbuddy 都应出现: %+v", got)
	}

	// 单独写 workbuddy 解析为独立的 workbuddy 条目（不再是 codebuddy 别名）。
	got, err = parseAgents("workbuddy")
	if err != nil {
		t.Fatalf("workbuddy 应可装: %v", err)
	}
	if len(got) != 1 || got[0].ID != "workbuddy" {
		t.Fatalf("workbuddy 应解析为独立 ID workbuddy: %+v", got)
	}
}

// TestParseAgentsUnknown 校验未知 agent 仍报错。
func TestParseAgentsUnknown(t *testing.T) {
	if _, err := parseAgents("nope"); err == nil {
		t.Fatal("未知 agent 应报错")
	}
}
