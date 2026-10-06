# 目录同步（一级方案）

> **状态**：📝 待拍板
> **来源**：原根 `DESIGN.md` §2
> **配套**：[`../requirements.md`](../requirements.md)（验收标准）· [`../architecture.md`](../architecture.md)（为什么选它）· [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md)（各家目录事实）

## 干什么

hook 在会话开始触发（或独立命令）→ 对每个 agent：

1. 定位其**工作区规则目录**（见 [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md)）。
2. 对真源里**我方每个文档**：
   - 目标不存在 ⇒ **copy** 过去（按该 agent 要求的结构 / 扩展名）；
   - 已存在 ⇒ 比对 **MD5**：相同跳过；不同则**覆盖**。
3. 其它非我方文档**一律不碰**。

## 业务规则

- **只认我方文档**：目标目录里用户自己的文档不受影响，同步器不删、不改、不挪。
- **不用软链**：一律真实拷贝（软链在部分 agent / 跨平台下不可靠）—— 对应验收标准 A4。
- **按 agent 分别落格式**：各家扩展名与结构不同，不能一个 `.md` 通吃。
- **无目录型 agent 退化处理**（Codex / OpenCode 不扫描目录）：Codex 拼接进 `AGENTS.md` 的**受管区**；OpenCode 改 `opencode.json` 显式列文件。受管区标记格式见 [`../../PROGRESS.md`](../../PROGRESS.md) 未决项 T2。
- **加删自由**（A5）：真源加一个文档 ⇒ 下次同步多一个；真源删掉的要不要从目标目录清掉 ⇒ 见未决项 T5。

## 边界（不做什么）

- 不做内容改写 / 模板渲染 —— 真源写什么就落什么（格式适配只动文件组织方式，不动正文）。
- 不做注入 —— 注入属降级方案 [`hook-injection.md`](hook-injection.md)。
