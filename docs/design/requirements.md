# rulemux 产品需求（真源）

> **状态**：✅ 已定（2026-10-07 逐项拍板，结论进 [`implementation.md`](implementation.md)）
> **来源**：用户立项诉求（原根 `DESIGN.md`，已分解迁入本文件）
> **配套**：[`architecture.md`](architecture.md)（为什么这么设计）· [`features.md`](features.md)（怎么做）· [`../PROGRESS.md`](../PROGRESS.md)（在办任务）

---

## 一、一句话需求

**用户在配置里列出的一组自有规则文件 → 投递到各家 AI coding agent**：一处管理、多处生效，效果与 token 与直接在每个工作区写 `AGENTS.md` 一致。

> 没有"中央真源"：rulemux 不拥有、不维护规则内容，只负责把用户配置所列的源文件复制 / 注入进各 agent 目录（见 [`implementation.md`](implementation.md) #4/#5）。

## 二、产品形态

- **形态**：命令行工具（CLI，`rulemux`）。
- **输入**：一份 **TOML 配置**（`~/.rulemux/config.toml`，目录可任意指定，非必须工作区），其中 `[[source]]` 列出若干源文件（磁盘任意位置的 `.md` / `.txt` 等文本）+ 目标 agent。
- **输出**：把每个源文件同步进各 agent 的工作区规则目录（Tier-1 真实拷贝；Tier-2 走 hooks 注入）。

## 三、验收标准（满足 / 不满足的判据）

| # | 标准 | 判据 |
|---|---|---|
| A1 | **效果等价** | 与直接写 `AGENTS.md` 等价 |
| A2 | **token 相当** | 与直接写 `AGENTS.md` 相当，不累积、不膨胀 |
| A3 | **永不淡出** | 不随对话老化 / 压缩而丢失（Tier-1 目录同步满足；Tier-2 hooks 注入视为降级，不保证 A3） |
| A4 | **不用软链** | 一律真实拷贝 |
| A5 | **加删自由** | 配置加 / 删源文件，目标目录跟着变（带前缀 + 删残留保证） |
| A6 | **开源通用** | 不绑某一家 agent |

> ⚠️ **A1–A3 是硬指标**：Tier-1 必须满足；Tier-2（单文件 agent 走 hooks 注入）启用即视为该 agent **降级、未达 A3 验收**。每个 agent 接完都要按 [`features/verification.md`](features/verification.md) 实测。

## 四、支持范围

- **Tier-1（读整个规则文件夹 ⇒ 真实拷贝丢文件）**：Claude Code、CodeBuddy、Trae、WorkBuddy。
- **Tier-2（只认单文件 `AGENTS.md` ⇒ 走 hooks 注入，不碰用户文件）**：Codex、OpenCode。
- **DeepSeek harness**：**移出当前范围**（最开放、支持插件，后续以插件市场解决）。

各家的工作区规则目录与读取行为见 [`external/agent-rules-dirs.md`](external/agent-rules-dirs.md)。

## 五、明确不做

- **不改规则内容**：不做改写 / 模板渲染，源文件写什么就落什么（格式适配只动文件组织方式，不动正文）。
- **不把注入当主方案**：hook 注入落在动态区，做不到 A3；仅对**无工作区规则目录的单文件 agent（Tier-2）**启用降级，且**不碰用户自己的 `AGENTS.md`**（由 SessionStart hook 注入上下文）。启用即视为该 agent 未达验收标准（见 [`features/hook-injection.md`](features/hook-injection.md)）。
- **不常驻、不监听**：无守护进程、不监听文件改动，极致简单（见 [`implementation.md`](implementation.md) #8）。
