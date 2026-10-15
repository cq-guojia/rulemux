package cmd

import (
	"fmt"
	"os"

	"github.com/cq-guojia/rulemux/internal/agents"
	"github.com/cq-guojia/rulemux/internal/config"
	"github.com/cq-guojia/rulemux/internal/engine"
)

// Verify is the canary acceptance check: drop a probe file carrying a secret
// token into the rules dir, then have the user open a NEW session and ask the
// agent to recite it. That proves the "SessionStart copy -> loaded in this
// session" chain end to end.
//
// It validates two things at once (docs/design/implementation.md #13):
//  1. the hook really fires before rules are read (otherwise this session can't see it)
//  2. the agent really loads our probe file: a non-hidden __rulemux__ name, carrying an
//     alwaysApply:true frontmatter so frontmatter-gated agents (CodeBuddy/WorkBuddy) pick it up
//
// targets 两条路（R4）：显式 --agent 永远允许（onboarding 唯一入口）；裸 verify 只给
// 「在用」的 Tier-1 agent 种 canary（与裸 sync 同一条公式）——否则一次裸 verify 就给
// 没装 / 已卸的 agent 建出规则目录，成为后续回填的跳板。
func Verify(args []string) int {
	f := ParseFlags(args)
	ws, err := workspace(f.Get("workspace", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "rulemux: cannot determine workspace:", err)
		return 1
	}

	requested := f.Get("agent", "")
	var targets []agents.Agent
	if requested != "" {
		// 显式点名：永远允许（onboarding 的唯一入口，设计原则 1 的例外）。
		targets = targetAgents(requested)
		if len(targets) == 0 {
			fmt.Fprintf(os.Stderr, "rulemux: unknown agent %q\n", requested)
			return 1
		}
	} else {
		// 裸 verify：只给「在用」的 Tier-1 agent 种 canary（与裸 sync 同一条公式）。
		// 否则一次裸 verify 就给没装 / 已卸的 agent 建出规则目录，成为后续回填的
		// 跳板（R4）。读不了配置 ⇒ 降级为仅账本，不崩。
		cfg, cfgErr := config.Load(f.Get("config", config.DefaultPath()))
		if cfgErr != nil {
			fmt.Fprintf(os.Stderr, "rulemux: cannot read config (%v) — using the ledger only\n", cfgErr)
			cfg = nil
		} else if verr := cfg.Validate(); verr != nil {
			fmt.Fprintf(os.Stderr, "rulemux: invalid config (%v) — using the ledger only\n", verr)
			cfg = nil
		}
		for _, a := range bareTargets(cfg, ws) {
			if a.Tier == agents.Tier1 {
				targets = append(targets, a)
			}
		}
		// ⚠ 空 targets 不能在这里对 --clean 也早退：裸 --clean 是超集清理（下面会把
		// targets 覆盖成 allTier1），用「在用集合为空」就返回会让 canary（例如
		// `verify --agent dsh` 种给不在用 agent 的那颗）永远清不掉 —— 正是 R4 要堵的跳板。
		if len(targets) == 0 && !f.Has("clean") {
			fmt.Fprintln(os.Stderr, "rulemux: no agent recorded for this workspace yet — nothing to verify")
			fmt.Fprintln(os.Stderr, "  run `rulemux sync --agent <id>` once, or pass --agent <id> explicitly")
			return 0
		}
	}

	// Cleanup mode
	if f.Has("clean") {
		// 清理集合必须 ⊇ 写入集合：裸 --clean 清本工作区**所有**已验证 Tier-1 的
		// canary（含显式 --agent 种给不在用 agent 的那颗）——否则它永远没人清（R4）。
		if requested == "" {
			targets = allTier1()
		}
		failed := false
		for _, a := range targets {
			dir := a.RulesDirAbs(ws)
			if dir == "" {
				if requested != "" {
					fmt.Printf("  %s: Tier-2 has no rules dir — nothing to clean\n", a.ID)
				}
				continue
			}
			if err := engine.CleanCanary(dir); err != nil {
				fmt.Fprintf(os.Stderr, "rulemux: failed to clean canary for %s: %v\n", a.ID, err)
				failed = true
				continue // 清完剩下的再汇总报错，不半途而废
			}
			// 清完 canary 后目录若确认为空也一并删掉（与 uninstall 的 D3 同规）：
			// 否则「探针建出来的空目录」会留在盘上，成为下次回填的位置。
			removeEmptyLeaf(dir)
			// skills 探针同样清理（工作区级，与写入对称）。
			if sdir := a.SkillsDirAbs(ws); sdir != "" {
				if err := engine.CleanSkillCanary(sdir); err != nil {
					fmt.Fprintf(os.Stderr, "rulemux: failed to clean skill canary for %s: %v\n", a.ID, err)
					failed = true
				}
			}
			fmt.Printf("Canary cleaned: %s\n", a.ID)
		}
		if failed {
			return 1
		}
		return 0
	}

	token := engine.CanaryToken()
	fmt.Println("rulemux verify — canary acceptance")
	fmt.Println("Token:", token)
	fmt.Println("Probes: rules dir (always); workspace skills dir (if the agent supports skills)")
	fmt.Println()
	for _, a := range targets {
		dir := a.RulesDirAbs(ws)
		if dir == "" {
			fmt.Printf("  %s: Tier-2 has no rules dir; ask it directly in a session whether the rules took effect\n", a.ID)
			continue
		}
		p, err := engine.WriteCanary(dir, token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: failed to write canary: %v\n", a.ID, err)
			return 1
		}
		fmt.Printf("  %s (rules): wrote %s\n", a.ID, p)
		// skills 验收：支持 skills 的 agent 在工作区级 skills 目录也种一颗技能探针
		// （仅本工作区，不碰用户级，与 rules canary 同口径）。这颗探针会让 Trae/dsh 等
		// 在会话里出现一个 __rulemux__canary-skill 技能，验证「推送是否真装载」。
		if sdir := a.SkillsDirAbs(ws); sdir != "" {
			sp, err := engine.WriteSkillCanary(sdir, token)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %s: failed to write skill canary: %v\n", a.ID, err)
				return 1
			}
			fmt.Printf("  %s (skills): wrote %s\n", a.ID, sp)
		}
	}

	fmt.Println()
	fmt.Println("Open a NEW session for each agent and ask it:")
	fmt.Printf("  rules:  \"Can you see this token in your rules: %s\"\n", token)
	fmt.Printf("  skills: \"Do you have a skill named %s? Can you invoke it by name?\"\n", engine.SkillCanaryDirName)
	fmt.Println()
	fmt.Println("How to read the result (docs/design/features/verification.md):")
	fmt.Println("  · Recites it ⇒ the chain works: the SessionStart copy took effect in this session,")
	fmt.Println("    and the agent loaded rulemux's probe file")
	fmt.Println("  · Cannot ⇒ check first: is the hook installed? did you open a NEW session? If it still")
	fmt.Println("    fails, the agent may gate loading on file shape — hidden dotfiles are skipped, and some")
	fmt.Println("    agents (e.g. CodeBuddy) require an alwaysApply:true frontmatter. Adjust the adapter's")
	fmt.Println("    Prefix / NeedsFrontmatter accordingly.")
	fmt.Println()
	fmt.Println("Clean up when done: rulemux verify --clean")
	fmt.Println("(bare --clean removes every agent's canary in this workspace, in use or not;")
	fmt.Println("the canary also carries the __rulemux__ prefix, so a later `rulemux sync`")
	fmt.Println("treats it as residue and deletes it automatically)")
	return 0
}
