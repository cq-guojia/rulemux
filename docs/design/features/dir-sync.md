# 目录同步（Tier-1 主方案）

> **状态**：✅ 已定（2026-10-07 拍板）
> **配套**：[`../requirements.md`](../requirements.md)（验收标准）· [`../architecture.md`](../architecture.md)（为什么选它）· [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md)（各家目录事实）

## 干什么

各 agent 的 **SessionStart hook** 调起 `rulemux sync`（无守护进程、不监听文件改动），对每个 Tier-1 agent：

1. 读 TOML 配置，取出该 agent 应接收的源文件（`[[source]]` 中 `agents` 含本 agent 或省略）。
2. 定位其**工作区规则目录**（见 [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md)）。
3. 对每个源文件：**1:1 复制**，目标文件名 = `__rulemux__` + 源 basename（碰撞加短 hash），**不改内容、不加 frontmatter、不拼合**。
4. **删残留**：枚举目标目录所有 `__rulemux__*` 文件，不在本次应生成集合内的 ⇒ 删除。

## 业务规则

- **前缀隔离**：我方文件统一 `__rulemux__` 前缀；目标目录里用户自己的（非此前缀）文件一律不碰、不删、不改。
- **不用软链**：一律真实拷贝（软链在部分 agent / 跨平台下不可靠）—— 对应 A4。
- **按 agent 分别落目录**：各家规则目录不同（Claude Code `.claude/rules/`、CodeBuddy/Trae `.codebuddy`/`.trae` 规则目录等），但内容都是原样 `.md`/`.txt`，不转格式。
- **幂等、不累积**：内容一致则跳过；配置移除某项 ⇒ 其带前缀文件被删 ⇒ 不累积陈旧文件（无 manifest / 无状态文件）—— 对应 A5。
- **加删自由**（A5）：配置加一个源 ⇒ 下次同步多一个；删掉 ⇒ 其带前缀文件被清理。

## 共享规则目录（多个 agent 落同一个目录）

有些 agent 的**工作区级**规则目录是同一个，各自的钩子却装在不同地方、各自触发 ——
典型是 **codebuddy 与 workbuddy 都落 `.codebuddy/rules`**（2026-10-09 三位置对照探针坐实，
见 [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md) §5.4；它们的**用户级**配置目录仍是分开的）。

这带来一个必须处理的冲突：`engine.Sync` 的删残留是「整目录下带 `__rulemux__` 前缀、
不在**本次计划**内的一律删」（`internal/engine/sync.go:164-185`）。两个 agent 若各自只算自己的
sources，就会把对方落下的文件当成残留删掉 ⇒ **两个钩子轮流触发时文件来回消失**。

规则如下（实现见 `internal/agents/registry.go` 的 `SharingRulesDir`、`internal/config/config.go`
的 `SourcesForAny`、`internal/cmd/sync.go` 的 `hookedPeers`）：

- **同步取并集**：`SharingRulesDir(a)` 找出与 a 同目录的 agent（`RulesDir` 相同、Tier-1），
  `SourcesForAny(ids, ws)` 取它们 sources 的并集 ⇒ 一次同步把该目录「应有」的文件都落齐，谁跑都不会删别人的。
- **「谁算在用」= 还装着钩子**（`hookedPeers` / `hookInstalled`，基于 `hooks.Inspect`）：
  卸载 = 摘钩子 ⇒ 该 agent 天然退出并集，**不需要任何额外的状态文件**。
- **兜底**：共享组里一个都没装钩子（纯手动 `rulemux sync` 场景）⇒ 退回该组**全部** agent 的并集，
  保证手动跑仍有东西落。
- **卸载收敛**（`internal/cmd/uninstall.go` 的 `pruneRulesDir`）：卸载某个 agent 时，把目录
  **收敛到「仍在用的同目录 agent 的应有集合」** —— 删掉被卸者独有的、保留共有的；剩余为空则清空
  （这正好等于 `uninstall --all` 的全清语义）。⚠ **剩余集合必须在 `hooks.Uninstall` 之前算好**，
  钩子一摘就判不出谁还在用了。
- **不新增状态**：整套判定只依赖「钩子装没装」这一既存事实 + 配置本身，无 manifest。
- 不共享目录的 agent（claude `.claude/rules`、trae `.trae/rules`、codex/opencode 不落盘）走原逻辑，
  `SharingRulesDir` 只返回自己，行为完全不变。

⚠ **代价**：目录共享时，最终落盘的是**并集** —— 若某条 source 只写了 `agents = ["workbuddy"]`，
它的文件同样会出现在 `.codebuddy/rules` 里被 CodeBuddy 读到。这是共享目录的固有语义，无法两全。

## 边界（不做什么）

- 不做内容改写 / 模板渲染 —— 源写什么就落什么。
- 不做注入 —— 注入属 Tier-2 降级方案 [`hook-injection.md`](hook-injection.md)；Tier-1 只真实拷贝。
- 不常驻、不监听文件改动（极致简单，见 [`../implementation.md`](../implementation.md) #8）。
