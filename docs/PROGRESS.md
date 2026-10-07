# 进度（进行中）

> **这是什么**：本仓库**唯一**的在办事项真源 —— 现在做到哪、欠什么、下一步做什么。
> **已结案的**看 [`PROGRESS-HISTORY.md`](PROGRESS-HISTORY.md)（一行一条：时间 / 完成了什么 / 过程文档）。
> **文档规矩**看 [`README.md`](README.md)。
>
> 本文件**只装在办的事**；事项结案即从本文件移入 `PROGRESS-HISTORY.md`。
> ⚠️ README / issue / 聊天记录都不是真源。

---

## 一、当前状态

### 1.1 需求与方案定型 —— ✅ **已拍板**（2026-10-07 逐条锁定，文档对齐进行中）

> 定型层文档已于 2026-10-07 逐项拍板（结论进 [`design/implementation.md`](design/implementation.md)）：
> [`design/requirements.md`](design/requirements.md)、[`design/architecture.md`](design/architecture.md)、[`design/features.md`](design/features.md) + `features/` 三份、[`design/external/agent-rules-dirs.md`](design/external/agent-rules-dirs.md)（Claude Code 已核实；CodeBuddy/Trae/Codex/OpenCode 待 canary）现已对齐新模型。

---

## 二、未决项

| # | 问题 | 现状与影响 | 将来怎么解（方向，未定） |
|---|---|---|---|
| T0 | ~~需求待拍板项：真源位置~~ **已解**：无中央真源，用户在 TOML 配置列出任意源文件（[`design/implementation.md`](design/implementation.md) #4/#5/#6） | `requirements.md` §二 已改写为「配置所列源文件」 | ✅ 已解 |
| T1 | **Trae / CodeBuddy `.mdc` 的 frontmatter 怎么写才无条件常驻** | `.mdc` 带 frontmatter（`alwaysApply` / `globs` / `description`）。写法不对 ⇒ 规则只在命中 glob 时生效，达不到「等价 `AGENTS.md`」 | 逐家查证 frontmatter 语义（要读到实现，不猜），结论回写 `external/agent-rules-dirs.md` |
| T2 | ~~Codex/OpenCode 分支处理~~ **已解（Tier-2）**：单文件 agent 走 SessionStart hook 注入，不碰用户 `AGENTS.md`，取消「受管区拼接」方案（[`design/implementation.md`](design/implementation.md) #9/#12） | `features/hook-injection.md` 已改写 | ✅ 已解 |
| T3 | **DeepSeek Harness 的规则目录未确认** | 目录 / 扩展名 / 是否读目录全部均未知，阻塞该 agent 接入 | 查证后补进 `external/agent-rules-dirs.md` |
| T4 | **各 agent 核实未完成** | Claude Code 已于 2026-10-07 经官方文档核实（hooks + 加载时机，见 `design/external/agent-rules-dirs.md` §三）；CodeBuddy 源码核查中；Trae / Codex / OpenCode / WorkBuddy / DeepSeek 🔴 待补 ⇒ **除已核实项外不得作实现依据** | 其余各家按 `features/verification.md` 跑实测闭环，并补齐适用版本与出处 |
| T5 | ~~删残留方案~~ **已解**：固定前缀 `.rulemux__` 区分「我方 / 用户」+ 删残留算法，无 manifest（[`design/implementation.md`](design/implementation.md) #10/#11） | `features/dir-sync.md` 已改写 | ✅ 已解 |
| T6 | **各 agent hooks 配置落点待查证** | `features/hook-injection.md` 表里 CodeBuddy / Codex / WorkBuddy 三项「待补」，且该表属**外部事实**，应迁入 `design/external/` | 查证后新建 `external/` 文档并标适用版本，原表改为引用 |
| T7 | **正式发版前要加 `files` 字段**（包名已占位） | ✅ `rulemux@0.0.1` 空包已发布占位。但 tarball 把 `AGENTS.md` / `RULES.md` / `PROGRESS*.md` / `worklog/` 全打进去了（18.7 kB） | `package.json` 加 `files: ["dist", "README.md", "LICENSE"]`，只发产物 |
| T8 | **实现细节逐条拍板**（作为任务跟进） | [`design/implementation.md`](design/implementation.md) 含待定项 1–15；截至 2026-10-07 已逐条拍定 1–13（仅 #14 测试 / #15 文档落地 留待实现） | ✅ 基本完成，结论已回写 design 文档 |

---

## 三、下一步

1. **文档对齐（进行中）**：把 `requirements.md` / `architecture.md` / `features.md` / `features/*` / `external/agent-rules-dirs.md` 对齐到 2026-10-07 新模型，并更新本文件，然后提交推送。
2. **外部事实 canary 验收（T1 / T3 / T4 / T6）**：按 [`design/features/verification.md`](design/features/verification.md) 跑实测闭环（点文件读取 / CodeBuddy 目录结构 / hook 先于读 / 单文件注入落点），补全「适用版本」，回写 `external/agent-rules-dirs.md`。
3. **落码**：按 [`design/features/dir-sync.md`](design/features/dir-sync.md) 与 [`design/implementation.md`](design/implementation.md) 实现 `rulemux sync` / `init` / `doctor` / `verify` + 各 agent 适配器（Go 单二进制）。
4. **发版前（T7）**：`package.json` 加 `files` 字段，只发产物。
