# 各 agent 工作区清单可读性（外部事实）

> **类型**：外部事实（上游 agent 侧）
> **适用版本**：见下表「适用版本」列；上游升级后据此复核
> **状态**：🟡 **部分核实** —— CodeBuddy / Trae 经本机实测坐实（2026-10-15，见 §一）；WorkBuddy 待 Mac canary（🔴，见 §一）；dsh 经源码核查确认为纯注入、无工作区清单（见 §一）
> **来源**：本机 2026-10-15 实测（`~/.codebuddy-server-cn` / `~/.trae-cn` 用户数据目录）+ 仓库 `internal/agents/registry.go` + `dsh-plugin/index.mjs` 核查
> **配套**：[`../features/dir-sync.md`](../features/dir-sync.md)（我方怎么适配）· [agent-rules-dirs.md](./agent-rules-dirs.md)（规则目录事实）· [`../../progress.json`](../../progress.json)（核实任务）

> ⚠️ 本文件只记**上游是否暴露「用户建立的工作区清单」**这一能力，以及它能不能当作「该工作区当前仍被 agent 使用」的判据。
> 「我方据此怎么做同步范围判定」是设计，归 [`../features/dir-sync.md`](../features/dir-sync.md)，**不写在这**。

## 一、总表

| Agent | 类型 | 是否暴露工作区清单 | 清单形态 | 路径可还原 | 语义（能否当「当前在用」判据） | 适用版本 | 备注 |
|---|---|---|---|---|---|---|---|
| **CodeBuddy** | VSCode 集成 | ✅ 是 | 按工作区建档：`genie-history/<base64(ws)>/{current.json, conversations/, backups/}`，45 条；`current.json.lastUpdated` = 最近会话时间 | ✅ 精确（标准 base64，`=`→`_`） | ⚠️ **使用历史**（append-only），不是活列表 | 4.12.1（本机） | 目录由 agent 扩展自建；删了工作区条目仍在 |
| **Trae** | VSCode 集成 | ✅ 是 | 按工作区建档：`~/.trae-cn/memory/projects/<路径slug>--p2-<hash>/<日期>/*.jsonl`，6 条；slug = `-` + 路径的 `/`→`-` | ⚠️ slug 可还原但含 `-` 的路径有歧义；hash 算法未核实 | ⚠️ **使用历史**（append-only），不是活列表 | CN `~/.trae-cn`（本机） | 同 CodeBuddy 系；删了工作区条目仍在 |
| **WorkBuddy** | 自研客户端 | 🔴 待 Mac 实测 | 本机（Linux）无 server data 目录，仅 `~/.workbuddy/{rules,skills,settings.json}`；与 CodeBuddy 同代码库双品牌（见 agent-rules-dirs.md §5.5）⇒ Mac 上预期同构 genie-history | 🔴 待验证 | 🔴 待验证 | 5.7.6（Mac） | **钩子是 user 级全局文件 `~/.workbuddy/settings.json`** |
| **DeepSeek Harness (dsh)** | 完全开放（插件） | ❌ 无 | 无 `~/.dsh`；插件按 `agent.session.header.cwd` 定位；规则注入不落盘、账本不记 Tier-2 | — | 天然免疫：rules 不落盘、账本不记；skills 只在真开会话时落 | ✅ 源码核查 | 不需要读工作区清单 |

## 二、关键事实（支撑「不读 agent 侧档案」B 方案否决 + A/C 方案设计）

1. **能读到的全是 append-only 历史，不是活列表**：CodeBuddy 的 `genie-history/` 里，已删除的工作区目录（盘上已不存在）条目仍在；Trae 的 `memory/projects/` 同理。⇒ 它只能证明「这个 agent 在这个工作区开过会话、最近一次在何时」，**不能**证明「用户当前还把它列在工作区里」。若用作「在用」判据，仍需叠加陈旧化策略，与 A 方案重叠，且随 app 版本漂移、每家路径/编码不同，维护成本高。

2. **VSCode 标准的「最近打开」列表拿不到**：真源是 `User/globalStorage/state.vscdb`（SQLite，key `history.recentlyOpenedPathsList`）或旧版 `storage.json`；本机全盘搜索 **0 命中**（VSCode-core 用户数据在本环境未持久化）；且 SQLite 读取与项目「零第三方依赖」硬约束冲突。⇒ 拿不到真正意义上的「当前工作区列表」。

3. **钩子是 user 级全局文件，与具体工作区无关**：CodeBuddy / WorkBuddy / Trae 的 `HookFile` 都是 `~/.xxx/...`（如 `~/.workbuddy/settings.json`、`~/.codebuddy/settings.json`、`~/.trae-cn/hooks.json`）。用户在 agent 里删除某个工作区，钩子文件**不动**。⇒ 「工作区从 agent 列表移除」≠「钩子没了」；钩子门禁（C）只能拦「agent 整个被全局卸载并清了配置」，治不了用户主诉的「删工作区但目录还在」。

4. **共享目录文件集由钩子决定、不由账本决定**：`.codebuddy/rules` 落什么文件由 `hookedPeers`（钩子）算并集（`internal/cmd/sync.go` 的 `unionAgentIDs` → `hookedPeers`），账本条目不影响文件集。⇒ 「停用某 agent 账本条目」的真实效果主要是用户级目录 + 报告 + 防「全停后仍推」；文件级收敛现有并集逻辑已覆盖。

## 三、结论对实现的约束

- **B 方案（读 agent 侧工作区档案）否决**：不稳定（append-only 历史非活列表）、维护成本高、且与 A 方案重叠。
- **A 方案（账本 lastSeen + 30 天过期）采用**：hook 触发 / 手动 sync / init 刷新时间戳；`sync --all` 不刷新（否则永不过期）；过期条目在裸 sync / sync --all / 顺手同步中跳过（条目保留供 uninstall 回访）。设计见 `internal/state/ledger.go`（`Seen` / `Touch` / `IsStale` / `StaleAfter`）与 `internal/cmd/sync.go`（`agentsFromLedger` 门禁）。
- **C 方案（钩子门禁）采用、仅作补充**：账本派生条目若钩子已全局卸载则跳过；与 A 同点落地于 `agentsFromLedger` 非哨兵分支。
- **dsh 天然免疫**，不在 `agentsFromLedger` 的 Tier-1 展开内，无需改动。

## 四、🔴 未决项（挂 `docs/progress.json`）

1. WorkBuddy（Mac）是否同构 CodeBuddy 的 `genie-history`，以及其 server data 目录真实路径 —— 需 Mac 侧 canary。
2. VSCode 标准 `state.vscdb`（SQLite）在本类环境为何缺失，以及是否需要/能读取「最近打开」。
3. `~/.codebuddy/projects` 目录语义（registry 里有 `getCodeBuddyProjectsDir` 但本环境不存在）—— 它是否是「当前工作区列表」的真源。
4. Trae `memory/projects` 的 `--p2-<hash>` hash 算法（用于 slug 歧义路径的精确还原）。
