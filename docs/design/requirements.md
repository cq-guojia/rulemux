# rulemux 产品需求（真源）

> **状态**：📝 待拍板
> **来源**：用户立项诉求（原根 `DESIGN.md`，已分解迁入本文件）
> **配套**：[`architecture.md`](architecture.md)（为什么这么设计）· [`features.md`](features.md)（怎么做）· [`../PROGRESS.md`](../PROGRESS.md)（未决项）

---

## 一、一句话需求

**一份中央真源规则 → 投递到各家 AI coding agent**：一处管理、多处生效，效果与 token 与直接在每个工作区写 `AGENTS.md` 一致。

## 二、产品形态

- **形态**：命令行工具（CLI）。
- **输入**：一份中央真源规则（一组文档）。
- **输出**：把真源同步进各 agent 的工作区规则目录。

> 真源由谁维护、放在哪（本地目录 / 仓库 / 配置指定）—— ⬜ **待用户拍板**。

## 三、验收标准（满足 / 不满足的判据）

| # | 标准 | 判据 |
|---|---|---|
| A1 | **效果等价** | 与直接写 `AGENTS.md` 等价 |
| A2 | **token 相当** | 与直接写 `AGENTS.md` 相当，不累积、不膨胀 |
| A3 | **永不淡出** | 不随对话老化 / 压缩而丢失 |
| A4 | **不用软链** | 一律真实拷贝 |
| A5 | **加删自由** | 真源加 / 删文档，目标目录跟着变 |
| A6 | **开源通用** | 不绑某一家 agent |

> ⚠️ **A1–A3 是硬指标**：任一不满足 ⇒ 判为**该 agent 未接入成功**，**不是**降级通过。
> 每个 agent 接完都要按 [`features/verification.md`](features/verification.md) 实测。

## 四、支持范围

Claude Code / Trae / CodeBuddy / WorkBuddy / Codex / OpenCode / DeepSeek Harness。
各家的工作区规则目录与读取行为见 [`external/agent-rules-dirs.md`](external/agent-rules-dirs.md) —— **该表尚未源码级核实，不得作为实现依据**。

## 五、明确不做

- **不改规则内容**：不做改写 / 模板渲染，真源写什么就落什么（格式适配只动文件组织方式，不动正文）。
- **不把注入当主方案**：hook 注入落在动态区，做不到 A3；只在某 agent 无工作区规则目录时作**降级模式**启用，启用即视为该 agent 未达验收标准（见 [`features/hook-injection.md`](features/hook-injection.md)）。
