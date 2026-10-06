# hook 注入（降级方案，非主选）

> **状态**：📝 待拍板
> **来源**：原根 `DESIGN.md` §4
> **配套**：[`../requirements.md`](../requirements.md)（验收标准 A1–A3）· [`../architecture.md`](../architecture.md)（为什么它不能当主方案）

## 什么时候用

仅当某个 agent **不支持**工作区级规则目录（目录同步落不进去）时启用。

> ⚠️ **启用即视为该 agent 未达验收标准**：本方案做不到 A3（永不淡出），是**降级模式**，不是通过验收的替代路径。判据见 [`../requirements.md`](../requirements.md) §三。

## 怎么做

- hooks：**`SessionStart` 读一次 + `PreCompact` 压缩前再写一次**；
- **不挂 `UserPromptSubmit`**（每轮追加 ⇒ token 爆炸）；
- 一个薄脚本 + 各 agent 的 hooks 配置落点。

## 已知代价

落在动态区 ⇒ 仍会随对话老化 / 被压缩摘要，`PreCompact` 只是延缓，**做不到「永不淡出」**。

## hooks 配置落点

> 下表属**外部事实**（各 agent 能力），尚未核实 ⇒ 核实后迁入 [`../external/`](../external/)，并标适用版本与来源。当前核实任务见 [`../../PROGRESS.md`](../../PROGRESS.md) 未决项。

| Agent | 配置落点 |
|---|---|
| Claude Code | `.claude/settings.json` |
| Trae | `hooks.json` |
| CodeBuddy | 待补 |
| Codex | 待补 |
| WorkBuddy | 待补 |

> 各家均跟 Claude Code hooks 规范（**待核实**）。
