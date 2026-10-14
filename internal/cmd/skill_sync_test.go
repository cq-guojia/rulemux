package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkTestSkill 造一个测试 skill 目录：<root>/<name>/SKILL.md + references/ref.md。
func mkTestSkill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: test\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "ref.md"), []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSyncSkillsWorkspaceScope：hook 路径下 [[skill_source]] 落进工作区 .codebuddy/skills，
// 带前缀、覆盖更新、删源清残留、用户自带 skill 不碰。
func TestSyncSkillsWorkspaceScope(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	skillSrc := filepath.Join(tmp, "skills")
	for _, d := range []string{home, ws, skillSrc} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	mkTestSkill(t, skillSrc, "alpha")
	// 用户自己的 skill（无前缀，必须不碰）
	own := filepath.Join(ws, ".codebuddy", "skills", "my-own")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "SKILL.md"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 非 skill 子目录（无 SKILL.md）应被忽略
	if err := os.MkdirAll(filepath.Join(skillSrc, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	ruleSrc := filepath.Join(tmp, "r.md")
	if err := os.WriteFile(ruleSrc, []byte("# rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp, "[[source]]\npath = \""+ruleSrc+"\"\n\n[[skill_source]]\npath = [\""+skillSrc+"\"]\n")

	skillsDir := filepath.Join(ws, ".codebuddy", "skills")
	Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"})

	// 首拷：skill 整树落盘（带前缀）
	if _, err := os.Stat(filepath.Join(skillsDir, "__rulemux__alpha", "SKILL.md")); err != nil {
		t.Fatalf("skill 未落盘: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "__rulemux__alpha", "references", "ref.md")); err != nil {
		t.Fatalf("skill 附属文件未落盘: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "__rulemux__not-a-skill")); !os.IsNotExist(err) {
		t.Fatal("非 skill 子目录不该被同步")
	}
	// rules 照旧落盘
	if _, err := os.Stat(filepath.Join(ws, ".codebuddy", "rules", "__rulemux__r.md")); err != nil {
		t.Fatalf("rules 未落盘: %v", err)
	}

	// 变更：改 SKILL.md ⇒ 覆盖
	if err := os.WriteFile(filepath.Join(skillSrc, "alpha", "SKILL.md"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"})
	if got, _ := os.ReadFile(filepath.Join(skillsDir, "__rulemux__alpha", "SKILL.md")); string(got) != "v2" {
		t.Fatalf("skill 内容未更新: %q", got)
	}

	// 删源 ⇒ 清残留；用户自己的 skill 不碰
	if err := os.RemoveAll(filepath.Join(skillSrc, "alpha")); err != nil {
		t.Fatal(err)
	}
	Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"})
	if _, err := os.Stat(filepath.Join(skillsDir, "__rulemux__alpha")); !os.IsNotExist(err) {
		t.Fatal("删源后残留未清")
	}
	if got, _ := os.ReadFile(filepath.Join(own, "SKILL.md")); string(got) != "mine" {
		t.Fatal("用户自己的 skill 被碰了")
	}
}

// TestSyncUserScopeDoubleWrite：scope=user 的 rules 与 skills 双写
// ~/.codebuddy 与 ~/.workbuddy 两套用户级目录；且 skills-only 的变化不触发
// 「新开会话」提示（stdout 必须为空）——热重载行为待 canary，宁可不提示。
func TestSyncUserScopeDoubleWrite(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	skillSrc := filepath.Join(tmp, "skills")
	for _, d := range []string{home, ws, skillSrc} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	mkTestSkill(t, skillSrc, "alpha")
	ruleSrc := filepath.Join(tmp, "r.md")
	if err := os.WriteFile(ruleSrc, []byte("# rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp,
		"[[source]]\npath = \""+ruleSrc+"\"\nscope = \"user\"\n\n[[skill_source]]\npath = [\""+skillSrc+"\"]\nscope = \"user\"\n")

	Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"})

	// 用户级 rules + skills：两家目录都写
	for _, base := range []string{".codebuddy", ".workbuddy"} {
		if _, err := os.Stat(filepath.Join(home, base, "rules", "__rulemux__r.md")); err != nil {
			t.Fatalf("用户级 rules 未双写（缺 %s）: %v", base, err)
		}
		if _, err := os.Stat(filepath.Join(home, base, "skills", "__rulemux__alpha", "SKILL.md")); err != nil {
			t.Fatalf("用户级 skills 未双写（缺 %s）: %v", base, err)
		}
	}

	// skills-only 变化 ⇒ 不触发提示（stdout 空）
	if err := os.WriteFile(filepath.Join(skillSrc, "alpha", "SKILL.md"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"})
	})
	if strings.TrimSpace(out) != "" {
		t.Fatalf("skills-only 变化不应触发提示，stdout 实得: %q", out)
	}
}

// TestSyncSkillsOnlyChangeNoNotice：只有 [[skill_source]]（连 rules 都没有）时，
// skills 首拷也不该触发「新开会话」提示。
func TestSyncSkillsOnlyChangeNoNotice(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	skillSrc := filepath.Join(tmp, "skills")
	for _, d := range []string{home, ws, skillSrc} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	mkTestSkill(t, skillSrc, "alpha")
	cfgPath := writeConfig(t, tmp, "[[skill_source]]\npath = [\""+skillSrc+"\"]\n")

	out := captureStdout(t, func() {
		Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "codebuddy"})
	})
	if strings.TrimSpace(out) != "" {
		t.Fatalf("只有 skill 源时不该触发提示，stdout 实得: %q", out)
	}
	if _, err := os.Stat(filepath.Join(ws, ".codebuddy", "skills", "__rulemux__alpha", "SKILL.md")); err != nil {
		t.Fatalf("skill 未落盘: %v", err)
	}
}

// TestSyncSkillsDshTier2：dsh 是 Tier-2（规则纯注入、无规则目录），但 skills 有原生目录
// ⇒ hook 路径下 [[skill_source]]（agents=["dsh"]）照样落进 <ws>/.dsh/skills；用户级
// （scope=user）落 ~/.dsh/skills；.codebuddy/.agents 目录与 .dsh/rules 一律不出现。
// 2026-10-14 用户拍板 + 源码级坐实（deepseek-ai/deepseek-harness@master，
// 见 docs/design/external/skill-ecosystem.md §三·再续）。
func TestSyncSkillsDshTier2(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	skillSrc := filepath.Join(tmp, "skills")
	for _, d := range []string{home, ws, skillSrc} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	mkTestSkill(t, skillSrc, "alpha")
	cfgPath := writeConfig(t, tmp,
		"[[skill_source]]\npath = [\""+skillSrc+"\"]\nagents = [\"dsh\"]\n\n"+
			"[[skill_source]]\npath = [\""+skillSrc+"\"]\nagents = [\"dsh\"]\nscope = \"user\"\n")

	Sync([]string{"--config", cfgPath, "--workspace", ws, "--hook", "--agent", "dsh"})

	// 工作区级：dsh 的 .dsh/skills（不是 .codebuddy/skills，更不是 .agents/skills）
	if _, err := os.Stat(filepath.Join(ws, ".dsh", "skills", "__rulemux__alpha", "SKILL.md")); err != nil {
		t.Fatalf("dsh skills 未落盘: %v", err)
	}
	for _, bad := range []string{
		filepath.Join(ws, ".codebuddy", "skills"),
		filepath.Join(ws, ".agents", "skills"),
		filepath.Join(ws, ".dsh", "rules"),
	} {
		if _, err := os.Stat(bad); !os.IsNotExist(err) {
			t.Fatalf("不该出现 %s（dsh 不写 codebuddy/.agents 目录，规则纯注入不落盘）", bad)
		}
	}
	// 用户级：~/.dsh/skills（不写 ~/.agents/skills——跨框架公约目录）
	if _, err := os.Stat(filepath.Join(home, ".dsh", "skills", "__rulemux__alpha", "SKILL.md")); err != nil {
		t.Fatalf("dsh 用户级 skills 未落盘: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills")); !os.IsNotExist(err) {
		t.Fatal("不该写 ~/.agents/skills（跨框架公约目录）")
	}

	// create 门禁：裸 --agent（无 --hook）⇒ 目录不存在时不建，什么都不写
	tmp2 := t.TempDir()
	ws2 := filepath.Join(tmp2, "ws")
	if err := os.MkdirAll(ws2, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp2, "home"))
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp2, "state"))
	Sync([]string{"--config", cfgPath, "--workspace", ws2, "--agent", "dsh"})
	if _, err := os.Stat(filepath.Join(ws2, ".dsh", "skills")); !os.IsNotExist(err) {
		t.Fatal("裸 --agent（无 --hook）不该创建 .dsh/skills 目录（create 门禁）")
	}
}

// TestUninstallPrunesUserDirs：卸载后用户级目录里的 __rulemux__ 前缀内容被清掉，
// 目录本身与用户自己的文件保留。
func TestUninstallPrunesUserDirs(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	ws := filepath.Join(tmp, "ws")
	skillSrc := filepath.Join(tmp, "skills")
	for _, d := range []string{home, ws, skillSrc} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("RULEMUX_STATE_DIR", filepath.Join(tmp, "state"))

	// 预建四套用户级目录（手动 sync 不建目录，只写已存在）
	for _, base := range []string{".codebuddy", ".workbuddy"} {
		for _, sub := range []string{"rules", "skills"} {
			if err := os.MkdirAll(filepath.Join(home, base, sub), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	// 用户级钩子（seed 一条 rulemux 自己的条目，卸载时摘它）
	if err := os.MkdirAll(filepath.Join(home, ".codebuddy"), 0o755); err != nil {
		t.Fatal(err)
	}
	hook := `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"rulemux sync --hook --agent codebuddy"}]}]}}`
	if err := os.WriteFile(filepath.Join(home, ".codebuddy", "settings.json"), []byte(hook), 0o644); err != nil {
		t.Fatal(err)
	}
	// 用户自己的文件（必须保留）
	if err := os.WriteFile(filepath.Join(home, ".codebuddy", "rules", "my-own.md"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	mkTestSkill(t, skillSrc, "alpha")
	ruleSrc := filepath.Join(tmp, "r.md")
	if err := os.WriteFile(ruleSrc, []byte("# rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, tmp,
		"[[source]]\npath = \""+ruleSrc+"\"\nscope = \"user\"\n\n[[skill_source]]\npath = [\""+skillSrc+"\"]\nscope = \"user\"\n")

	// 手动 sync（create=false ⇒ 只写已存在目录）
	Sync([]string{"--config", cfgPath, "--workspace", ws, "--agent", "codebuddy"})
	for _, base := range []string{".codebuddy", ".workbuddy"} {
		if _, err := os.Stat(filepath.Join(home, base, "rules", "__rulemux__r.md")); err != nil {
			t.Fatalf("用户级 rules 未落盘（%s）: %v", base, err)
		}
		if _, err := os.Stat(filepath.Join(home, base, "skills", "__rulemux__alpha", "SKILL.md")); err != nil {
			t.Fatalf("用户级 skills 未落盘（%s）: %v", base, err)
		}
	}

	Uninstall([]string{"--agent", "codebuddy", "--config", cfgPath, "--workspace", ws, "--yes"})

	for _, base := range []string{".codebuddy", ".workbuddy"} {
		if _, err := os.Stat(filepath.Join(home, base, "rules", "__rulemux__r.md")); !os.IsNotExist(err) {
			t.Fatalf("卸载后用户级 rules 残留未清（%s）", base)
		}
		if _, err := os.Stat(filepath.Join(home, base, "skills", "__rulemux__alpha")); !os.IsNotExist(err) {
			t.Fatalf("卸载后用户级 skills 残留未清（%s）", base)
		}
		// 目录本身保留
		if _, err := os.Stat(filepath.Join(home, base, "rules")); err != nil {
			t.Fatalf("用户级 rules 目录被删了（%s）: %v", base, err)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".codebuddy", "rules", "my-own.md")); string(got) != "mine" {
		t.Fatal("用户自己的文件被删了")
	}
	// 钩子已摘
	b, _ := os.ReadFile(filepath.Join(home, ".codebuddy", "settings.json"))
	if strings.Contains(string(b), "rulemux sync") {
		t.Fatalf("钩子未摘除: %s", b)
	}
}
