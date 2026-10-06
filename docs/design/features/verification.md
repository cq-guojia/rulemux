# 实测验证法（每个 agent 跑一次）

> **状态**：📝 待拍板
> **来源**：原根 `DESIGN.md` §6
> **配套**：[`../requirements.md`](../requirements.md)（验收标准）· [`hook-injection.md`](hook-injection.md)（验证失败后的降级）

1. 埋两个**互相独立**的随机串：`TOKEN-XYZ` 与 `TOKEN-QRS`；
2. 开新会话问模型：能否念出 `TOKEN-XYZ`（验证**是否进上下文**）；
3. 跑长任务触发一次压缩，再问 `TOKEN-QRS`（验证**会不会被丢掉**）。

**两步都过才算这个 agent 闭环**：

- 念不出 `TOKEN-XYZ` ⇒ 没进上下文（同步没生效 / 该目录根本没被读）；
- 压缩后念不出 `TOKEN-QRS` ⇒ 会淡出 ⇒ 不满足 A3，该 agent 不能走目录同步，退回 [`hook-injection.md`](hook-injection.md)（**降级，不算通过验收**）。

> ⚠️ **压缩后为什么问另一个串**：`TOKEN-XYZ` 在第 2 步已被写进对话，压缩摘要里可能仍留着它 ⇒ 念得出来也可能是**假阳性**。压缩后必须问一个**此前从未在对话中出现过**的串，才能证明「规则此刻真在上下文里」。
