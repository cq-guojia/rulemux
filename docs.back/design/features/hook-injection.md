# hook 注入（Tier-2 降级方案）

> **状态**：✅ 已定（2026-10-07 拍板：仅单文件 agent 降级，不碰用户 `AGENTS.md`）
> **配套**：[`../requirements.md`](../requirements.md)（验收标准 A1–A3）· [`../architecture.md`](../architecture.md)（为什么它不能当主方案）

## 什么时候用

仅当某个 agent **不支持工作区规则目录**（无目录模式，只认单文件 `AGENTS.md`，如 Codex / OpenCode）时启用。

> ⚠️ **启用即视为该 agent 降级、未达 A3（永不淡出）验收标准**：本方案落在动态区，做不到 A3。判据见 [`../requirements.md`](../requirements.md) §三。

## 怎么做

- 由该 agent 的 **`SessionStart` hook** 在会话启动时把规则内容**注入上下文**（不写入、不修改用户自己的 `AGENTS.md`）。
- **不挂 `UserPromptSubmit`**（每轮追加 ⇒ token 爆炸）。
- `PreCompact`（压缩前）仅作为 Tier-2 内、**确需「本轮刷新」时**的可选子手段，非主路径。
- 一个薄脚本 + 各 agent 的 hooks 配置落点。

## 已知代价

落在动态区 ⇒ 仍会随对话老化 / 被压缩摘要，`PreCompact` 只是延缓，**做不到「永不淡出」**。

## hooks 配置落点

> 下表属**外部事实**（各 agent 能力），以 [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md) 为准；本表仅记落点。

| Agent | 配置落点 | 备注 |
|---|---|---|
| Claude Code | `.claude/settings.json` | 仅 Tier-1 用其 SessionStart 调 sync；Tier-2 不用 |
| Codex | `codex` 配置（`[features] codex_hooks = true` + `/hooks` 批准） | SessionStart 注入 |
| OpenCode | `opencode.json` / 其 hooks 配置 | SessionStart 注入 |
| CodeBuddy / Trae / WorkBuddy | 各自 hooks 配置 | Tier-1（目录同步），SessionStart 调 `rulemux sync` |

> DeepSeek harness 已移出当前范围（后续插件市场解决）。
