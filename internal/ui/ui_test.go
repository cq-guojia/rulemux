package ui

import (
	"os"
	"testing"
)

func TestDetectEnabled(t *testing.T) {
	// 管道（非字符设备）⇒ 关
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if detectEnabled(r) {
		t.Error("管道应判为无颜色")
	}

	// /dev/null 是字符设备 ⇒ 开（Linux/macOS CI 均可用）
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip("无法打开 /dev/null，跳过字符设备用例")
	}
	defer f.Close()
	if !detectEnabled(f) {
		t.Error("/dev/null 是字符设备，应判为有颜色")
	}

	// NO_COLOR 优先于一切
	t.Setenv("NO_COLOR", "1")
	if detectEnabled(f) {
		t.Error("NO_COLOR 应关色")
	}
	// TERM=dumb 同样关色
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if detectEnabled(f) {
		t.Error("TERM=dumb 应关色")
	}
}

func TestPaintSwitch(t *testing.T) {
	old := enabled
	defer func() { enabled = old }()

	enabled = false
	for _, fn := range []func(string) string{Green, Yellow, Red, Dim, Bold} {
		if got := fn("x"); got != "x" {
			t.Errorf("关色时应原样返回，实得 %q", got)
		}
	}
	enabled = true
	if got := Green("x"); got != "\x1b[32mx\x1b[0m" {
		t.Errorf("Green 开色时转义错误：%q", got)
	}
	if got := Dim("y"); got != "\x1b[2my\x1b[0m" {
		t.Errorf("Dim 开色时转义错误：%q", got)
	}
	if got := Bold("z"); got != "\x1b[1mz\x1b[0m" {
		t.Errorf("Bold 开色时转义错误：%q", got)
	}
}

func TestTruncateJoin(t *testing.T) {
	cases := []struct {
		items []string
		max   int
		want  string
	}{
		{nil, 3, ""},
		{[]string{"a"}, 3, "a"},
		{[]string{"a", "b", "c"}, 3, "a, b, c"},
		{[]string{"a", "b", "c", "d"}, 3, "a, b, c, …+1 more"},
		{[]string{"a", "b"}, 0, "a, b"}, // max<=0 不截断
	}
	for _, c := range cases {
		if got := TruncateJoin(c.items, c.max); got != c.want {
			t.Errorf("TruncateJoin(%v, %d) = %q, want %q", c.items, c.max, got, c.want)
		}
	}
}

func TestHomeRel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := HomeRel(home); got != "~" {
		t.Errorf("HOME 本身应为 ~，实得 %q", got)
	}
	if got := HomeRel(home + "/rules/a.md"); got != "~/rules/a.md" {
		t.Errorf("HOME 下路径应压缩，实得 %q", got)
	}
	if got := HomeRel("/other/x.md"); got != "/other/x.md" {
		t.Errorf("HOME 外路径应原样返回，实得 %q", got)
	}
}

func TestPlural(t *testing.T) {
	if got := Plural(1, "workspace"); got != "1 workspace" {
		t.Errorf("单数错误：%q", got)
	}
	if got := Plural(9, "workspace"); got != "9 workspaces" {
		t.Errorf("复数错误：%q", got)
	}
}
