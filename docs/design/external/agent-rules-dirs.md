# 各 agent 工作区规则目录（外部事实）

> **类型**：外部事实（上游 agent 侧）
> **适用版本**：见下表「适用版本」列；上游升级后据此复核
> **状态**：🔴 **未核实** —— 除 Claude Code 外均未走源码级核实，**不得作为实现依据**
> **来源**：前期调研（原根 `DESIGN.md` §3）
> **配套**：[`../features/dir-sync.md`](../features/dir-sync.md)（我方怎么适配）· [`../../PROGRESS.md`](../../PROGRESS.md)（核实任务 T4）

> ⚠️ 本文件只记**上游读取能力**（目录 / 扩展名 / 读不读全部）。
> 「我方据此怎么落文件」是设计，归 [`../features/dir-sync.md`](../features/dir-sync.md)，**不写在这**。

| Agent | 工作区规则目录 | 扩展名 / 结构 | 读目录全部？ | 适用版本 | 备注 |
|---|---|---|---|---|---|
| Claude Code | `.claude/rules/` | `.md` | ✅ 全部 | v2.0.64+ | — |
| Trae | `.trae/rules/` | `.mdc` | ✅ 递归读，最多 3 层 | 待补 | 需 frontmatter |
| CodeBuddy | `.codebuddy/rules/` | 每条规则 = 子文件夹 + `RULE.mdc` | ⚠️ 固定结构 | 待补 | 重载项目时自动扫描 |
| WorkBuddy | `.codebuddy/rules/`（复用 CodeBuddy 机制） | 同上 | ⚠️ 固定结构 | 待补 | — |
| Codex | ❌ 无目录 | 单文件 `AGENTS.md`（沿目录树向上合并，每目录最多一个） | ❌ | 待补 | — |
| OpenCode | ⚠️ 非目录扫描 | `AGENTS.md` + `opencode.json` 显式列 instruction 文件 | ❌ | 待补 | — |
| DeepSeek Harness | 待确认 | — | — | 待补 | TODO |

> ⚠️ **扩展名 / 结构各家不同** ⇒ 同步器必须**按 agent 分别落格式**，不能一个 `.md` 通吃。
