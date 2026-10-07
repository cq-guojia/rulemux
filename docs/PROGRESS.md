# 进度（进行中）

> **这是什么**：本仓库**唯一**的在办事项真源 —— 现在做到哪、欠什么、下一步做什么。
> **已结案的**看 [`PROGRESS-HISTORY.md`](PROGRESS-HISTORY.md)（一行一条：时间 / 完成了什么 / 过程文档）。
> **文档规矩**看 [`README.md`](README.md)。
>
> 本文件**只装在办的事**；事项结案即从本文件移入 `PROGRESS-HISTORY.md`。
> ⚠️ README / issue / 聊天记录都不是真源。

---

## 一、当前状态

### 1.1 需求与方案定型 —— ⏸️ **待启动**（文档骨架已就绪）

> 定型层全部文档均标 📝 待拍板 / 🔴 未核实：
> [`design/requirements.md`](design/requirements.md)（产品需求与验收标准）、[`design/architecture.md`](design/architecture.md)（核心原理 / 选型 / 命名）、[`design/features.md`](design/features.md) + `features/` 三份（怎么做）、[`design/external/agent-rules-dirs.md`](design/external/agent-rules-dirs.md)（各家规则目录，**未核实**）。

---

## 二、未决项

| # | 问题 | 现状与影响 | 将来怎么解（方向，未定） |
|---|---|---|---|
| T0 | **需求待拍板项**：真源由谁维护、放在哪（本地目录 / 仓库 / 配置指定） | `requirements.md` §二 标 ⬜ 待拍板；不定则同步器无输入契约 | 用户拍板后写进 `requirements.md` |
| T1 | **Trae / CodeBuddy `.mdc` 的 frontmatter 怎么写才无条件常驻** | `.mdc` 带 frontmatter（`alwaysApply` / `globs` / `description`）。写法不对 ⇒ 规则只在命中 glob 时生效，达不到「等价 `AGENTS.md`」 | 逐家查证 frontmatter 语义（要读到实现，不猜），结论回写 `external/agent-rules-dirs.md` |
| T2 | **Codex（单文件）/ OpenCode（配置列表）的分支处理** | 这两家不扫描目录：Codex 只认单文件 `AGENTS.md`，OpenCode 要在 `opencode.json` 显式列 instruction 文件 | 退化为：Codex 拼接进 `AGENTS.md` 的**受管区**；OpenCode 改 `opencode.json` 列文件。需先定**受管区标记格式**，避免覆盖用户自己写的内容 |
| T3 | **DeepSeek Harness 的规则目录未确认** | 目录 / 扩展名 / 是否读目录全部均未知，阻塞该 agent 接入 | 查证后补进 `external/agent-rules-dirs.md` |
| T4 | **各 agent 核实未完成** | Claude Code 已于 2026-10-07 经官方文档核实（hooks + 加载时机，见 `design/external/agent-rules-dirs.md` §三）；CodeBuddy 源码核查中；Trae / Codex / OpenCode / WorkBuddy / DeepSeek 🔴 待补 ⇒ **除已核实项外不得作实现依据** | 其余各家按 `features/verification.md` 跑实测闭环，并补齐适用版本与出处 |
| T5 | **真源里删掉的文档要不要从目标目录清掉** | 「加删自由」（A5）要求删除也生效，但同步器要能区分「我方曾同步过的」与「用户自己的」 | 定方案（如落一份 manifest 记录我方同步过的文件），写进 `features/dir-sync.md` |
| T6 | **各 agent hooks 配置落点待查证** | `features/hook-injection.md` 表里 CodeBuddy / Codex / WorkBuddy 三项「待补」，且该表属**外部事实**，应迁入 `design/external/` | 查证后新建 `external/` 文档并标适用版本，原表改为引用 |
| T7 | **正式发版前要加 `files` 字段**（包名已占位） | ✅ `rulemux@0.0.1` 空包已发布占位。但 tarball 把 `AGENTS.md` / `RULES.md` / `PROGRESS*.md` / `worklog/` 全打进去了（18.7 kB） | `package.json` 加 `files: ["dist", "README.md", "LICENSE"]`，只发产物 |
| T8 | **实现细节逐条拍板**（作为任务跟进） | 用户 2026-10-07 立项 [`design/implementation.md`](design/implementation.md) 为任务清单，含待定项 1–15（语言/CLI/真源/targets/适配/幂等/降级/验证/工程化），覆盖 T0–T7 之外的新增决策 | 基于该文档逐项讨论拍板，结论回写对应 design 文档，划掉一项 |

---

## 三、下一步

1. **需求拍板**：确认 [`design/requirements.md`](design/requirements.md) 的待拍板项（真源位置与维护方式、支持 agent 范围、验收标准 A1–A6）。
2. **外部事实核实（T1 / T3 / T4 / T6）**：把各家规则目录与 hooks 配置查到源码级、补全「适用版本」，并按 [`design/features/verification.md`](design/features/verification.md) 实测闭环。
3. **方案拍板**：确认 [`design/architecture.md`](design/architecture.md) 的选型 —— 目录同步为主选、hook 注入为**降级**（不满足 A3 即视为该 agent 未接入成功）。
4. **技术选型**：定语言 / 分发方式 —— 结论**进 [`design/architecture.md`](design/architecture.md)**（属架构选型，不是代码规范）。
5. **落码**：按 [`design/features/dir-sync.md`](design/features/dir-sync.md) 实现目录同步。
