# Hindsight 在本机的实际机制（外部事实）

> **类型**：外部事实（第三方工具侧，作为 rulemux 决策对照）
> **适用版本**：本机 CodeBuddy CN 环境，观察于 2026-10-08；上游升级后据此复核
> **状态**：🟢 **已核实** —— 直接读取本机实际配置文件，非文档推测
> **来源**：本机 `/root/.codebuddy/settings.json`、`/root/.codebuddy/mcp.json`、CodeBuddy 的 `installed_servers_cache.json`（MCP 缓存）
> **配套**：[`../architecture.md`](../architecture.md) §五（rulemux 据此拍板「保持 workspace 级 hook」）

## 一、结论：Hindsight 既用 MCP，也用 CodeBuddy 原生 hook

此前假设「Hindsight 只是 MCP」**不准确**。实测本机配置，Hindsight 同时由两块构成：

1. **MCP server**：注册在 user 级 MCP host，提供可被 agent 在对话中调用的工具（如 knowledge 检索、回传）。
2. **CodeBuddy 原生 hooks**：注册在 user 级 `settings.json`，覆盖多个会话生命周期事件。

## 二、已读取的真实配置（2026-10-08）

### 2.1 MCP server —— `/root/.codebuddy/mcp.json`

```json
"hindsight": {
  "command": "node",
  "args": ["/root/.hindsight/coding-agents/dist/mcp-server.js"],
  "env": { "HINDSIGHT_MCP_HARNESS": "codebuddy" }
}
```

MCP 缓存 `installed_servers_cache.json` 中条目状态为 `"connected"`、`"configSource": "user"`，证实其为**用户级**注册。

### 2.2 CodeBuddy hooks —— `/root/.codebuddy/settings.json`（user 级）

```json
"hooks": {
  "SessionStart":     [{ "command": "node \"/root/.hindsight/coding-agents/dist/codebuddy-sessionstart-hook.js\"", "timeout": 30 }],
  "UserPromptSubmit": [{ "command": "node \"/root/.hindsight/coding-agents/dist/codebuddy-hook.js\"",              "timeout": 30 }],
  "Stop":             [{ "command": "node \"/root/.hindsight/coding-agents/dist/codebuddy-stop-hook.js\"",         "timeout": 60 }]
}
```

同目录存在 `settings.json.hindsight-backup`、`mcp.json.hindsight-backup` ⇒ Hindsight 改动前先备份，确为它所写入。

## 三、与 rulemux 的关键差异（对照）

| 维度 | rulemux（Tier-1，2026-10-08 起） | Hindsight |
|---|---|---|
| MCP server | 无（CLI 二进制，由 hook 直接 spawn） | 有（`~/.codebuddy/mcp.json`） |
| CodeBuddy hooks | 1 个：`SessionStart` | 3 个：`SessionStart` / `UserPromptSubmit` / `Stop` |
| **hook 落点** | **user 级** `~/.codebuddy/settings.json`（已同 Hindsight 对齐） | **user 级** `/root/.codebuddy/settings.json` |
| 是否随仓库进 git | 否 | 否 |
| 触发范围 | 任意工作区（全局） | 任意工作区（全局） |

要点：**Hindsight 的 hooks 写在 user 级主目录，不写在任一项目的 workspace `.codebuddy/settings.json` 里**——所以它「看不见」出现在某个具体项目的钩子配置中。其「会话中及时回传」= MCP 工具在对话中被调用 + 上述生命周期 hook 触发，全部挂在 user 级。

> ⚠️ 本条为 fact，记录「Hindsight 怎么装」；rulemux 据此的决策见 `../architecture.md` §五——**2026-10-08 起 rulemux 的 hook 落点已改为 user 级，与 Hindsight 对齐**，不再写工作区级 `.codebuddy/settings.json`。

## 四、它的"自动更新"是谁做的（2026-10-08 查证）

用户观察到「Hindsight 的插件包好像有自动更新」。查证结论：**更新不是 Hindsight 自己做的，是 CodeBuddy 的插件/技能市场在更新它。**

- Hindsight 本体是 npm 包 `@vectorize-io/hindsight-coding-agents@0.6.1`（`package.json`），以插件形态提供 `skills` / `hooks` / `mcpServers`（`plugin.json`）。
- 它的 MCP server 只是 `node dist/mcp-server.js` —— **惰性，自身不含任何更新逻辑**。
- 真正的版本检查在宿主侧：`/root/.codebuddy/.skills-marketplace-update-state`：
  ```json
  { "lastAttemptAt":..., "lastSuccessAt":..., "remoteVersion": "e056361e-...", "failureCount": 0 }
  ```
  即 **CodeBuddy skills marketplace 自带的版本检查器**（记录远端版本 / 上次尝试 / 上次成功 / 失败次数）在拉新版。

**对 rulemux 的意义**：不要误以为「做一个 MCP 就能白嫖自动更新」——那是市场机制，不是 MCP 的能力；而 rulemux 是单二进制 + 钩子，走不了插件市场。升级策略已拍板为「包管理器手动更新」，见 [`../requirements.md`](../requirements.md) §五。
