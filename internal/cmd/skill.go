package cmd

// `rulemux skill` 子命令（S3，docs/design/features/skill-sync.md §八）：
// 远程源首次由 `rulemux sync` 惰性抓取进 ~/.rulemux/skills/<key>/；本命令负责
// 查看这些源、并对已抓取的远程源做升级（git pull / npm 重 pack）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/engine"
	"github.com/cq-guojia/rulemux/internal/ui"
)

// Skill 实现 `rulemux skill`：
//
//	rulemux skill list            列出配置里的 skill 源及 staging 状态
//	rulemux skill update <key>    升级某个已抓取的远程源
//	rulemux skill update --all    升级全部已抓取的远程源
func Skill(args []string) int {
	f := ParseFlags(args)
	cfgPath := f.Get("config", config.DefaultPath())
	sub := ""
	if len(f.Rest) > 0 {
		sub = f.Rest[0]
	}
	switch sub {
	case "list":
		return skillList(cfgPath)
	case "update":
		key := ""
		if len(f.Rest) > 1 {
			key = f.Rest[1]
		}
		return skillUpdate(cfgPath, key, f.Has("all"))
	case "":
		fmt.Fprintln(os.Stderr, "rulemux skill: missing subcommand")
		skillUsage()
		return 2
	default:
		fmt.Fprintf(os.Stderr, "rulemux skill: unknown subcommand %q\n", sub)
		skillUsage()
		return 2
	}
}

func skillUsage() {
	fmt.Fprint(os.Stderr, `USAGE:
  rulemux skill list             List configured skill sources and their staging status
  rulemux skill update <key>     Update one staged remote source (git pull / npm re-pack)
  rulemux skill update --all     Update every staged remote source
`)
}

func skillList(cfgPath string) int {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: failed to read config: %v\n", err)
		return 1
	}
	if len(cfg.SkillSources) == 0 {
		fmt.Fprintln(os.Stderr, "rulemux: no [[skill_source]] configured")
		return 0
	}
	fmt.Fprintf(os.Stderr, "%s\n\n", ui.Bold("Skill sources — "+cfgPath))
	for _, s := range cfg.SkillSources {
		scope := "workspace"
		if s.IsUserScope() {
			scope = "user"
		}
		targets := skillTargets(s)
		for _, p := range s.Paths {
			kind, key, ref, sub := engine.ClassifySource(p)
			root := effectiveRoot(kind, ref, key, sub)
			if kind == engine.SourceLocal {
				status := "ok"
				if _, err := os.Stat(root); err != nil {
					status = "missing"
				}
				fmt.Fprintf(os.Stderr, "  %-30s %-6s %s\n", truncateRunes(p, 30), "local",
					ui.Dim("["+status+"]  "+scope+"  "+targets))
				continue
			}
			status := "not staged"
			if isStagedDir(root) {
				status = "staged"
			}
			fmt.Fprintf(os.Stderr, "  %-30s %-6s %s\n", truncateRunes(p, 30), kindLabel(kind),
				ui.Dim("→ "+ui.HomeRel(root)+"  ["+status+"]  "+scope+"  "+targets))
		}
	}
	return 0
}

func skillUpdate(cfgPath, key string, all bool) int {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulemux: failed to read config: %v\n", err)
		return 1
	}
	if !all && key == "" {
		fmt.Fprintln(os.Stderr, "rulemux skill update: specify <key> or --all")
		return 2
	}
	timeout := cfg.FetchTimeoutDuration()
	matched, exit := 0, 0
	done := map[string]bool{} // 同一 repo（skey）多个子路径只升级一次（pull 一次全更）
	for _, s := range cfg.SkillSources {
		for _, p := range s.Paths {
			kind, skey, _, _ := engine.ClassifySource(p)
			if kind == engine.SourceLocal {
				continue
			}
			if !all && p != key && skey != key {
				continue
			}
			if done[skey] {
				continue
			}
			done[skey] = true
			matched++
			if err := engine.UpdateStaged(p, timeout); err != nil {
				fmt.Fprintf(os.Stderr, "rulemux: %s %s: %v\n", ui.Red(ui.GlyphFail), skey, err)
				exit = 1
				continue
			}
			fmt.Fprintf(os.Stderr, "rulemux: %s %s updated (%s)\n", ui.Green(ui.GlyphOK), skey, ui.HomeRel(engine.StagingDir(skey)))
		}
	}
	if matched == 0 {
		if all {
			fmt.Fprintln(os.Stderr, "rulemux: no remote skill sources configured")
		} else {
			fmt.Fprintf(os.Stderr, "rulemux: no remote skill source matches %q\n", key)
		}
	}
	return exit
}

func skillTargets(s config.SkillSource) string {
	agents := "all"
	if len(s.Agents) > 0 {
		agents = strings.Join(s.Agents, ",")
	}
	return "agents: " + agents
}

func kindLabel(k engine.SourceKind) string {
	switch k {
	case engine.SourceGitHub, engine.SourceGit:
		return "git"
	case engine.SourceNPM:
		return "npm"
	default:
		return "?"
	}
}

// effectiveRoot 算一条源的实际本地根：本地源 = ref；远程源 = staging 目录；再拼上 `#子路径`。
func effectiveRoot(kind engine.SourceKind, ref, key, sub string) string {
	root := ref
	if kind != engine.SourceLocal {
		root = engine.StagingDir(key)
	}
	if sub != "" {
		root = filepath.Join(root, filepath.FromSlash(sub))
	}
	return root
}

func isStagedDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

// truncateRunes 按 rune 截断（避免把多字节字符切成乱码），超长以 … 收尾。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
