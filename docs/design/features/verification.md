# 实测验证法（每个 agent 跑一次）

> **状态**：✅ 已定（2026-10-07 拍板，canary 文件法；细节见 [`../implementation.md`](../implementation.md) #13）
> **来源**：原根 `DESIGN.md` §6
> **配套**：[`../requirements.md`](../requirements.md)（验收标准）· [`hook-injection.md`](hook-injection.md)（验证失败后的降级）

## canary 四步法（`rulemux verify` / `doctor` 落地的对象）

1. **钩子先于读规则**：在 agent 规则目录放一个带 `rulemux` 暗号的 canary 文件，开新会话，问模型「你看到了 canary 里的暗号吗」——能答出即证明 SessionStart 复制在当前会话生效（坐实链路）。
2. **点文件可读性**：同样放 `.rulemux__canary.md`（点开头隐藏），验证该 agent「读全部 .md」**不跳过点文件**；若跳过 ⇒ 退化为非点前缀 `rulemux__`。
3. **目录结构**：验证平铺 `.rulemux__*.md` 是否被加载（如 CodeBuddy 的目录结构存疑项）。
4. **token 一致性**：对比「rulemux 注入/拷贝」与「直接手写 AGENTS.md」进上下文的 token 数，应在误差内一致（A1/A2 验收）。

## 压缩淡出测试（仅 Tier-1 需过）

1. 埋两个互相独立的随机串：`TOKEN-XYZ` 与 `TOKEN-QRS`；
2. 开新会话问模型：能否念出 `TOKEN-XYZ`（验证**是否进上下文**）；
3. 跑长任务触发一次压缩，再问 `TOKEN-QRS`（验证**会不会被丢掉**）。

**两步都过才算这个 agent 闭环**：

- 念不出 `TOKEN-XYZ` ⇒ 没进上下文（同步没生效 / 该目录根本没被读）；
- 压缩后念不出 `TOKEN-QRS` ⇒ 会淡出 ⇒ 不满足 A3，该 agent 不能走目录同步，退回 [`hook-injection.md`](hook-injection.md)（**降级，不算通过验收**）。

> ⚠️ **压缩后为什么问另一个串**：`TOKEN-XYZ` 在第 2 步已被写进对话，压缩摘要里可能仍留着它 ⇒ 念得出来也可能是**假阳性**。压缩后必须问一个**此前从未在对话中出现过**的串，才能证明「规则此刻真在上下文里」。
