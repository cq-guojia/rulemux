# rulemux 架构：为什么这么设计

> **状态**：📝 待拍板
> **来源**：用户前期调研（原根 `DESIGN.md` §1 / §7，已分解迁入本文件）
> **配套**：[`requirements.md`](requirements.md)（用户要什么）· [`features.md`](features.md)（怎么做）· [`external/agent-rules-dirs.md`](external/agent-rules-dirs.md)（各家规则目录）

---

## 一、核心原理：为什么必须走 harness 原生规则目录

每次 API 请求的结构是 `system`（静态前缀）+ `messages`（对话）。

1. **Prompt caching 靠前缀匹配**：`system` 的静态前缀被冻结以保缓存 ⇒ 任何每轮变化的内容都会毁掉缓存。
2. **hook 注入落在动态区**：`additionalContext` / `system-reminder` 属动态区——
   - `SessionStart` 注一次 ⇒ 随对话老化、压缩时被摘要 ⇒ **淡出**；
   - 每轮注（`UserPromptSubmit`）⇒ 追加累积 ⇒ **token 爆炸**；
   - **无法写入冻结前缀**（这是缓存 + 信任模型的设计，不是能力缺陷）。
3. **结论**：要「和 `AGENTS.md` 一模一样」（永不淡出 / 不累积 / 恒定 token）＝ **必须走 harness 原生加载的规则文件 / 目录**（即静态前缀语义）。

> ⚠️ 由此定下铁律：**hook 只当「投递员」，不负责注入。**
> 验收判据见 [`requirements.md`](requirements.md) §三（A1–A3）。

## 二、方案选型

| 方案 | 定位 | 取舍理由 |
|---|---|---|
| **目录同步** | 主选 | 走 harness 原生加载 ⇒ 满足 A1–A3（等价 / 不累积 / 永不淡出）；无软链；加删自由。代价：要**按各家格式分别落文件** |
| **hook 注入** | **降级，不主选** | 落在动态区 ⇒ **做不到 A3（永不淡出）**，`PreCompact` 只是延缓。仅在某 agent 无工作区规则目录时启用，**启用即视为该 agent 未达验收标准** |

> 两个方案的具体做法分别在 [`features/dir-sync.md`](features/dir-sync.md)、[`features/hook-injection.md`](features/hook-injection.md)；本文件只记**为什么这么选**。

## 三、命名

**`rulemux`**（首选）：mux = 多路复用，一份源 → 多 agent；短且有代表性。

候选（备查）：`rulecast`（广播）/ `rulehub`（中央枢纽）/ `ruleseed`（播种）/ `ruleflow`；其它可用：`agentrules`、`unirules`、`ruleslot`、`rulefeed`、`rulesink`、`rulewise`、`ruleup`、`rulery`、`ruleseek`。

> npm 是否占用属**外部事实**且会变化 ⇒ 发布前重新核实，记录日期与来源（见 [`../PROGRESS.md`](../PROGRESS.md) 未决项）。
