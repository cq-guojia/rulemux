package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cq-guojia/rulemux/internal/agents"
)

// dshAgent 返回一个 patch/插件都落在临时目录里的 dsh 适配器（HookFile 用绝对路径，
// ExpandHome 原样返回，故与 ~ 无关）。
func dshAgent(t *testing.T) agents.Agent {
	t.Helper()
	dir := t.TempDir()
	return agents.Agent{
		ID:       "dsh",
		Tier:     agents.Tier1,
		RulesDir: ".dsh/rules",
		HookFile: filepath.Join(dir, "cordis.patch.yml"),
		HookAbs:  true,
		Style:    "dsh",
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// 安装：写出插件文件 + 顶层数组里的标记块；重复安装字节不变（幂等）。
func TestInstallDshPlugin_WritesBlockAndPlugin_Idempotent(t *testing.T) {
	a := dshAgent(t)
	patch := a.HookFileAbs("")
	plugin := dshPluginPath(a)

	p, err := installDshPlugin(a, "")
	if err != nil {
		t.Fatalf("installDshPlugin: %v", err)
	}
	if p != patch {
		t.Fatalf("returned path = %q, want %q", p, patch)
	}
	if got := readFile(t, plugin); got != string(dshPluginJS) {
		t.Errorf("plugin file content mismatch (got %d bytes)", len(got))
	}
	content := readFile(t, patch)
	for _, want := range []string{dshMarkerStart, dshMarkerEnd, "id: rulemux", fileURL(plugin)} {
		if !strings.Contains(content, want) {
			t.Errorf("patch missing %q:\n%s", want, content)
		}
	}

	if _, err := installDshPlugin(a, ""); err != nil {
		t.Fatalf("re-install: %v", err)
	}
	if after := readFile(t, patch); after != content {
		t.Errorf("re-install changed the file (not idempotent):\n%s", after)
	}
}

// 安装必须保留用户自己的 patch 行；对一个只含空表占位 [] 的文件，则归一为只留我方块
// （否则会出现两个顶层 YAML 文档，dsh 启动失败）。
func TestInstallDshPlugin_PreservesUserPatches_NormalizesEmptyList(t *testing.T) {
	a := dshAgent(t)
	patch := a.HookFileAbs("")

	user := "- insert:\n    - id: other\n      name: \"file:///other.js\"\n"
	if err := os.WriteFile(patch, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installDshPlugin(a, ""); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, patch)
	if !strings.Contains(got, "id: other") || !strings.Contains(got, "id: rulemux") {
		t.Errorf("user patch lost or our block missing:\n%s", got)
	}

	// 只含 [] 的文件：安装后不应残留 "[]"（会被拼成第二个顶层文档）。
	b := dshAgent(t)
	if err := os.WriteFile(b.HookFileAbs(""), []byte("[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installDshPlugin(b, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, b.HookFileAbs("")), "[]") {
		t.Errorf("[] placeholder not normalized away:\n%s", readFile(t, b.HookFileAbs("")))
	}
}

// 卸载：移除我方块与插件文件；无其它 patch 时写回 []（dsh 顶层必须是数组）。
func TestUninstallDsh_RemovesBlockAndPlugin_WritesEmptyList(t *testing.T) {
	a := dshAgent(t)
	patch := a.HookFileAbs("")
	if _, err := installDshPlugin(a, ""); err != nil {
		t.Fatal(err)
	}
	if err := uninstallDsh(patch, a); err != nil {
		t.Fatalf("uninstallDsh: %v", err)
	}
	if got := readFile(t, patch); got != "[]\n" {
		t.Errorf("patch after uninstall = %q, want %q", got, "[]\n")
	}
	if _, err := os.Stat(dshPluginPath(a)); !os.IsNotExist(err) {
		t.Errorf("plugin file should be removed, stat err = %v", err)
	}
}

// 卸载保留用户自己的 patch 行。
func TestUninstallDsh_KeepsOthers(t *testing.T) {
	a := dshAgent(t)
	patch := a.HookFileAbs("")
	user := "- insert:\n    - id: other\n      name: \"file:///other.js\"\n"
	if err := os.WriteFile(patch, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installDshPlugin(a, ""); err != nil {
		t.Fatal(err)
	}
	if err := uninstallDsh(patch, a); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, patch)
	if !strings.Contains(got, "id: other") || strings.Contains(got, "id: rulemux") {
		t.Errorf("uninstall should keep others and drop our block:\n%s", got)
	}
}

// Inspect / ExpectedCommand / TargetCommand 对 dsh 都按「安装项 = 插件 URL」工作。
func TestDshInspect_And_ExpectedItem(t *testing.T) {
	a := dshAgent(t)
	patch := a.HookFileAbs("")

	if installed, _, err := Inspect(patch, a); err != nil || installed {
		t.Fatalf("before install: installed=%v err=%v, want false/nil", installed, err)
	}
	if _, err := installDshPlugin(a, ""); err != nil {
		t.Fatal(err)
	}
	installed, item, err := Inspect(patch, a)
	if err != nil || !installed {
		t.Fatalf("after install: installed=%v err=%v", installed, err)
	}
	if want := dshPluginURL(a); item != want {
		t.Errorf("install item = %q, want %q", item, want)
	}
	if got := ExpectedCommand(a, item); got != item {
		t.Errorf("ExpectedCommand = %q, want %q", got, item)
	}
	if got := TargetCommand(a); got != item {
		t.Errorf("TargetCommand = %q, want %q", got, item)
	}
}

// Refresh 把已存在的过期 URL 重写为当前插件 URL，返回 changed=true。
func TestRefreshDsh_UpdatesStaleURL(t *testing.T) {
	a := dshAgent(t)
	patch := a.HookFileAbs("")
	stale := dshBlock("file:///old/moved/rulemux-dsh.js")
	if err := os.WriteFile(patch, []byte(stale+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := Refresh(patch, a)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !changed {
		t.Error("Refresh should report changed for a stale URL")
	}
	if got := readFile(t, patch); !strings.Contains(got, dshPluginURL(a)) {
		t.Errorf("refreshed patch does not carry the current URL:\n%s", got)
	}
	// 再刷新：已是最新 ⇒ changed=false。
	changed, err = Refresh(patch, a)
	if err != nil || changed {
		t.Errorf("second Refresh: changed=%v err=%v, want false/nil", changed, err)
	}
}
