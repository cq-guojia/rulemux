# 接入一个新 Agent 需要做什么

> **状态**：✅ 已定（2026-10-08，随 user 级钩子落点 + 工作区账本一并定型）
> **来源**：用户 2026-10-08 要求沉淀「要接一个新的 Agent，需要做哪些工作」
> **配套**：[`../architecture.md`](../architecture.md) §四（三层结构）· [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md)（外部事实）· [`../features.md`](../features.md)（功能总索引）

> **当前范围声明**：已实现且可安装的只有 **codebuddy / workbuddy**。
> claude / trae / codex / opencode 虽已注册，但**尚未做安装与删除的实现**，等在做的那一刻再按本清单补齐；在此之前它们一律 `Verified=false`，`init` 拒绝安装、`doctor` 标 ⚠。

一个 agent 适配器由**注册表里的一条 `Agent` + 三类动作**构成。下方 checklist 逐条照抄即可。

## 零、先做外部事实核实（不许跳过）

> 规矩见项目要求：**涉及到各家 agent 的规则目录 / hooks 配置，必须先读源码与文档再动手，禁止靠运行时试探猜**。

要核实并记录到 [`external/agent-rules-dirs.md`](../external/agent-rules-dirs.md)（每条带出处 `文件:行号` 或包版本，并标适用版本）：

- [ ] 工作区规则目录在哪、扩展名是什么、是否读目录下**全部**文件
- [ ] 是否有目录模式；无目录（只认单文件 `AGENTS.md`）⇒ 判为 **Tier-2**，走注入
- [ ] hooks 配置文件的落点与 schema；是否支持 `type:"command"` 直接 spawn 二进制
- [ ] hook 的**会话生命周期事件**有哪些（`SessionStart` 等）

只有核实完成后才允许标 `Verified=true`。未核实前： **`Verified=false` 且不得投入实现依据**。

## 一、注册表登记（`internal/agents/registry.go`）

- [ ] `ID`（配置里 `agents` 字段写它）、可选 `Aliases`
- [ ] `Tier`：`Tier1`（有目录，真拷贝）或 `Tier2`（无目录，走注入）
- [ ] `RulesDir`：规则目录，相对工作区根；Tier-2 留空 `""`（`RulesDirAbs` 返回空串 ⇒ 卸载时不找文件）
- [ ] `HookFile` + `HookAbs`：
  - 钩子在 user 级 host 配置 ⇒ `HookFile: "~/..."` + `HookAbs: true`（**现在 codebuddy / workbuddy / codex 都是这种**，见 [`architecture.md`](../architecture.md) §五）
  - 钩子在工作区级 ⇒ 相对路径 + `HookAbs: false`
- [ ] `Style`：钩子配置写法 `claude` / `trae` / `json`（均为 JSON）或 `codex`（TOML）
- [ ] `Verified`：**未 canary 坐实前必须 `false`**
- [ ] `Note`：写清核实结论与日期

## 二、安装动作（install）

- [ ] `internal/hooks/install.go` 里给该 `Style` 加写入分支（现有 `installJSON` / `installCodex`）
- [ ] 写入**必须幂等**：同 agent 重复安装不产生第二条（现有 `hasRulemuxHook` 判定）
- [ ] **必须保留文件里已有内容**，只追加 rulemux 那一条（不覆盖别家钩子——本机 Hindsight 的钩子就与 rulemux 并存，见 [`external/hindsight.md`](../external/hindsight.md)）
- [ ] `SubcommandFor`：Tier-1 用 `sync`，Tier-2 用 `inject`
- [ ] `internal/cmd/init.go` 的帮助文本与可安装清单更新（只允许 Verified）

## 三、删除动作（uninstall）——最易漏的一项

- [ ] `internal/hooks/uninstall*.go` 给该 Style 加**移除**分支，且只删 rulemux 自己那条（现有 `isRulemuxHook` / `blockIsRulemux`）
- [ ] 文件清理走 `removeRuleFiles(dir)`，它只 glob `.rulemux__` 前缀 ⇒ **用户的源文件与规则目录里的其它文件天然不受影响**（详见 §五）
- [ ] Tier-2（`RulesDir == ""`）**磁盘上不留文件**，卸载只需删钩子——不要为它去找"残留文件"
- [ ] **跨工作区清理**：钩子若在 user 级（全局），卸载时必须按工作区账本回访其它工作区，否则残留再无机会被删除（见 §四）

## 四、工作区账本（`internal/state/ledger.go`）

> 背景：钩子装在 user 级 ⇒ 对所有工作区全局生效；但规则文件落在**各工作区本地**。
> 一旦卸载移除了钩子，就不再有 sync 触发 ⇒ 其它工作区的残留永不会被自动清理。

- [ ] Tier-1 同步成功后调用 `state.Record(ws)` 记账（`sync.go` 已接）
- [ ] 卸载时调用 `sweepRecordedWorkspaces` 按账本逐个工作区回访清理
- [ ] **账本只增不减**：即便工作区已被删除/清空也保留记录 ⇒ 路径日后重现（如重新 clone）仍会被回访
- [ ] 卸载**必须**先把要回访的工作区列给用户确认（破坏性操作，需明示）

## 五、不可逾越的红线

| 红线 | 保障机制 |
|---|---|
| 源文件（用户 `path` 所指）**绝不删改** | 全程只读 `os.ReadFile(it.Src)`（`engine/sync.go:102`）；删除只针对规则目录 |
| 规则目录里用户自己的文件**绝不删** | 删残留时 `!HasPrefix(name, Prefix)` 即跳过（`engine/sync.go:132-134` 标注「用户自己的文件，不碰」） |
| 别家钩子 / 配置**绝不覆盖** | `installJSON` 先读再改、只追加；`uninstallJSON` 按 `command=="rulemux"` + agentID 精确剔除 |
| 不扫描机器检测 agent | `--agent` 必填，禁止自动发现本机装了哪些 agent |
