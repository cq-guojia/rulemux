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
- [ ] **会话内是否会重读规则目录**（不会 ⇒ 需要「变化提示」`SessionHint`；会热重载 ⇒ 保持 false）
- [ ] hook 的 stdout **输出协议**（能否用 `hookSpecificOutput.additionalContext` 注入）⇒ 决定 `HintProtocol`
- [ ] **用户级配置目录是否可由环境变量覆盖**（例：CodeBuddy 4.12.1 的 `CODEBUDDY_CONFIG_DIR`，见 [`external/agent-rules-dirs.md`](../external/agent-rules-dirs.md) §五）⇒ 有则填注册表的 `HookDirEnv` + `HookFileBase`；没有或没核实 ⇒ 留空，按 `~` 展开

只有核实完成后才允许标 `Verified=true`。未核实前： **`Verified=false` 且不得投入实现依据**。

## 一、注册表登记（`internal/agents/registry.go`）

- [ ] `ID`（配置里 `agents` 字段写它）、可选 `Aliases`
- [ ] `Tier`：`Tier1`（有目录，真拷贝）或 `Tier2`（无目录，走注入）
- [ ] `RulesDir`：规则目录，相对工作区根；Tier-2 留空 `""`（`RulesDirAbs` 返回空串 ⇒ 卸载时不找文件）
- [ ] `HookFile` + `HookAbs`：
  - 钩子在 user 级 host 配置 ⇒ `HookFile: "~/..."` + `HookAbs: true`（**现在 codebuddy / workbuddy / codex 都是这种**，见 [`architecture.md`](../architecture.md) §五）
  - 钩子在工作区级 ⇒ 相对路径 + `HookAbs: false`
- [ ] `HookDirEnv` + `HookFileBase`：**只在已核实**该 agent 的用户级配置目录可被环境变量覆盖时才填
      （现例：codebuddy → `"CODEBUDDY_CONFIG_DIR"` + `"settings.json"`）。
      填了之后 `HookFileAbsWithSource()` 的解析顺序是 **环境变量（非空）> `~` 展开的字面量**；
      **未核实的 agent 一律留空，绝不猜**（其余四家当前都留空）
- [ ] `Style`：钩子配置写法 `claude` / `trae` / `json`（均为 JSON）或 `codex`（TOML）
- [ ] `Verified`：**未 canary 坐实前必须 `false`**
- [ ] `Note`：写清核实结论与日期

## 二、安装动作（install）

- [ ] `internal/hooks/install.go` 里给该 `Style` 加写入分支（现有 `installJSON` / `installCodex`）
- [ ] 写入**必须幂等**：同 agent 重复安装不产生第二条（`installJSON` 先比对「已存在且等于当前目标命令」就直接不写盘）
- [ ] **升级刷新**：新二进制必须能把旧格式钩子就地升级。`hooks.Refresh` 复用同一套识别口径，
      **只重写我们自己那条**（含 `--agent <别名>` 的历史写法）、**不创建文件或目录**（`create=false`）、
      **幂等**（已是最新一个字节都不写）。入口：`rulemux init --refresh`（无 `--agent` 时遍历全部已注册
      agent，只刷已装者）+ npm `postinstall`（全局安装后自动跑，任何异常都静默 exit 0）
- [ ] **必须保留文件里已有内容**，只追加 rulemux 那一条（不覆盖别家钩子——本机 Hindsight 的钩子就与 rulemux 并存，见 [`external/hindsight.md`](../external/hindsight.md)）
- [ ] `SubcommandFor`：Tier-1 用 `sync`，Tier-2 用 `inject`
- [ ] `internal/cmd/init.go` 的帮助文本与可安装清单更新（只允许 Verified）

## 三、删除动作（uninstall）——最易漏的一项

- [ ] `internal/hooks/uninstall*.go` 给该 Style 加**移除**分支，且只删 rulemux 自己那条（现有 `isRulemuxHookFor` / `dropRulemuxHooks` / `blockIsRulemux`）
- [ ] 文件清理走 `removeRuleFiles(dir)`，它只 glob `__rulemux__` 前缀 ⇒ **用户的源文件与规则目录里的其它文件天然不受影响**（详见 §五）
- [ ] Tier-2（`RulesDir == ""`）**磁盘上不留文件**，卸载只需删钩子——不要为它去找"残留文件"
- [ ] **跨工作区清理**：钩子若在 user 级（全局），卸载时必须按工作区账本回访其它工作区，否则残留再无机会被删除（见 §四）

## 四、工作区账本（`internal/state/ledger.go`）

> 背景：钩子装在 user 级 ⇒ 对所有工作区全局生效；但规则文件落在**各工作区本地**。
> 一旦卸载移除了钩子，就不再有 sync 触发 ⇒ 其它工作区的残留永不会被自动清理。

- [ ] Tier-1 同步成功后调用 `state.Record(ws, agentID)` 记账（**账本 v2 = 工作区 × Agent**，一个工作区可对应多个 agent）
- [ ] **只在「确有处理」时记账**：被守卫跳过、或目录不存在被 `create=false` 跳过时**不记**（否则会把无关工作区灌进账本）
- [ ] 卸载时调用 `sweepRecordedWorkspaces` 按账本逐个工作区回访清理
- [ ] **账本只增不减**：即便工作区已被删除/清空也保留记录 ⇒ 路径日后重现（如重新 clone）仍会被回访。
      唯一例外：`rulemux sync --all` 的 GC 会清掉**确实不存在（ENOENT）**的条目（权限/断连等错误一律保留）
- [ ] v1 → v2 迁移**绝不丢条目**：老条目的 agents 以哨兵 `state.AnyAgent`（`"*"` = 全部已支持）占位
- [ ] 卸载**必须**先把要回访的工作区列给用户确认（破坏性操作，需明示）

## 五、不可逾越的红线

| 红线 | 保障机制 |
|---|---|
| 源文件（用户 `path` 所指）**绝不删改** | 全程只读 `os.ReadFile(it.Src)`（`engine/sync.go:102`）；删除只针对规则目录 |
| 规则目录里用户自己的文件**绝不删** | 删残留时 `!HasPrefix(name, Prefix)` 即跳过（`engine/sync.go:132-134` 标注「用户自己的文件，不碰」） |
| 别家钩子 / 配置**绝不覆盖** | `installJSON` 先读再改、只追加；`uninstallJSON` 按 `command=="rulemux"` + agentID 精确剔除 |
| 不探测「本机装了哪些 agent」 | 只信**编译期注册表**、绝不探测环境：`--agent` 可选（缺省 = 全部 `Supported()`），但从不扫描机器；「用户 init 过哪几家」也只以「该 agent 宿主配置里有没有我们自己写的那条钩子」为判据 |
| 任何钩子写入**只许动自己那条** | 安装 / 刷新 / 卸载一律按 `command` 的 basename == `rulemux` 且 agent id 命中 `MatchIDs()`（规范 ID + 全部别名）精确匹配。刷新**不新增**（没装过就跳过）、**不创建**文件或目录、**幂等**（已是最新不写盘） |
