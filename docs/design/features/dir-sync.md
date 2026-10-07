# 目录同步（Tier-1 主方案）

> **状态**：✅ 已定（2026-10-07 拍板）
> **配套**：[`../requirements.md`](../requirements.md)（验收标准）· [`../architecture.md`](../architecture.md)（为什么选它）· [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md)（各家目录事实）

## 干什么

各 agent 的 **SessionStart hook** 调起 `rulemux sync`（无守护进程、不监听文件改动），对每个 Tier-1 agent：

1. 读 TOML 配置，取出该 agent 应接收的源文件（`[[source]]` 中 `agents` 含本 agent 或省略）。
2. 定位其**工作区规则目录**（见 [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md)）。
3. 对每个源文件：**1:1 复制**，目标文件名 = `.rulemux__` + 源 basename（碰撞加短 hash），**不改内容、不加 frontmatter、不拼合**。
4. **删残留**：枚举目标目录所有 `.rulemux__*` 文件，不在本次应生成集合内的 ⇒ 删除。

## 业务规则

- **前缀隔离**：我方文件统一 `.rulemux__` 前缀；目标目录里用户自己的（非此前缀）文件一律不碰、不删、不改。
- **不用软链**：一律真实拷贝（软链在部分 agent / 跨平台下不可靠）—— 对应 A4。
- **按 agent 分别落目录**：各家规则目录不同（Claude Code `.claude/rules/`、CodeBuddy/Trae `.codebuddy`/`.trae` 规则目录等），但内容都是原样 `.md`/`.txt`，不转格式。
- **幂等、不累积**：内容一致则跳过；配置移除某项 ⇒ 其带前缀文件被删 ⇒ 不累积陈旧文件（无 manifest / 无状态文件）—— 对应 A5。
- **加删自由**（A5）：配置加一个源 ⇒ 下次同步多一个；删掉 ⇒ 其带前缀文件被清理。

## 边界（不做什么）

- 不做内容改写 / 模板渲染 —— 源写什么就落什么。
- 不做注入 —— 注入属 Tier-2 降级方案 [`hook-injection.md`](hook-injection.md)；Tier-1 只真实拷贝。
- 不常驻、不监听文件改动（极致简单，见 [`../implementation.md`](../implementation.md) #8）。
