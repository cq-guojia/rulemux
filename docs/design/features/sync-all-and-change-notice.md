# 同步范围改造 + 变化提示（v4 · 定稿）

> **状态**：✅ **已定稿，执行中**（2026-10-09）
> **来源**：用户 2026-10-09 多轮方向拍板 + **四轮**专家团评审
> **前身**：v1 `sync-all-and-inject-new.md`（注入新增正文，已废）→ v2（改为"变化提示"）→ v3（补目录两原则、撤销共用目录合并）→ **本稿 v4**（统一"want 空"处置、`create` 判据改用 `--hook`、补命令语义矩阵/迁移专章/锁方案/测试矩阵）
> **影响面**：`internal/state`、`internal/cmd`、`internal/hooks`、`internal/engine`、`internal/agents`、`internal/config`、`README*`、`docs/**`

---

## 〇、平白版（先说人话）

**要干的四件事：**

1. **一条命令同步所有工作区**（`rulemux sync --all`）——只查账本，不再一个个跑。
2. **改了规则就提示用户重开会话**——hook 同步完如果发现规则变了（新增/修改/删除任一），就让模型告诉用户一句"规则有变化，请新开会话"；**没变就一个字都不输出**（用户完全无感）。
3. **顺手同步**——某个 agent 开会话同步时，把这个工作区历史上用过的**别的写目录的 agent** 也一起同步了。
4. **账本升级**——记"工作区 → 用过哪些 agent"，并把"目录已经不在"的旧记录清掉。

**两条铁律（用户 2026-10-09 定）：**

- **只有该 agent 自己的会话 hook 触发时**，才发现它的规则目录不在就创建；**其它任何情况都不建目录**。
- **顺手同步 / 手动同步**：**只往已经存在的目录里写**；目录不存在就跳过，**绝不创建**。

---

## 一、要解决的三个问题

1. **手动同步太碎**：`rulemux sync` 只处理当前工作区，多工作区得一个个跑。
2. **差一拍**（已坐实：读规则先于 hook 写入）：新工作区、或改动过的规则，在当前会话读不到。
3. **多 agent 共用工作区**：一个工作区可能被多个 agent 打开，同步要覆盖"该工作区用过的所有 agent"。

> **已坐实前提**（2026-10-08「清空目录后开新会话」对照实验）：**读规则先于 hook 写入**。
> ⇒ `external/agent-rules-dirs.md:117`「钩子先于读规则 ✅」是**假阳性残留**，必须撤回。
> ⇒ `README*` 的"快照早于 hook"才是对的。（本稿执行时须用**对照探针**复核一次，见 §九 迁移/实证。）

## 二、原则：目录的生命周期

### 原则 1 —— 谁能创建目录

**只有「该 agent 自己的会话 hook 触发时」（针对它所在的工作区）**，若发现该 agent 的规则目录**不存在**才创建；已存在则不动；**其它任何情况都不创建**。

落地（`engine.Sync` 现在**无条件** `os.MkdirAll(dir)`，`engine/sync.go:98`，必须改）：

- 采用方案 A（最小、单生产调用点）：`engine.Sync(dir, srcs, fm, create bool)`；
  `create=false` 且目录不存在 ⇒ **立即返回**（不建、不读、不删），`SyncResult` 带 `SkippedNoDir=true`。

**`create` 的判据（关键修正）**：

- ❌ 不能写成 `create = (target.ID == 请求的 --agent)` —— **手动**跑 `rulemux sync --agent codebuddy` 同样满足，会误建目录、违反原则 1。
- ✅ 改为 **`create = --hook && target.ID == 请求的 --agent`**：只有 hook 调用且被点名的那一个 agent 才允许建目录。
- ⇒ 因此 **hook 命令行必须带 `--hook`**（`internal/hooks/install.go` 写死的命令串要从 `rulemux sync --agent X` 改为 `rulemux sync --hook --agent X`）。
- `--hook` 同时是另一项新语义的唯一信号源：**能否向 stdout 写协议提示**（见 §六）。两者共用同一开关，避免语义分裂。
- `--hook` 且未给 `--agent` ⇒ 报错（hook 必然点名 agent）。

### 原则 2 —— 谁能写目录

**顺手同步 / 手动 `sync --all` / `sync` 未点名的目标**：**只能写已存在的目录** —— 可新增/修改/删除其中的文件，但**目录不存在就跳过，绝不创建**。

**目录是否存在，只认 `os.Stat`，不认账本**（账本只回答"历史用过哪些 agent"，目录可能已被手工删除）。
不存在 ⇒ 跳过 + stderr 一行日志（默认安静）。

### ✅ 清空的正确姿势（v3 的"保守处置"已废弃）

**核心原则：rulemux 写进去的东西，rulemux 必须自己负责清掉** —— 用户从配置里删掉规则，下次同步就**自动清掉目录里的副本**，**绝不能让用户手动去清**。
⇒ `want` 为空时 **照常清空**（`engine/sync.go:133-153` 的删残留照旧生效）。**v3 曾写"want 空就不删、让用户跑 `uninstall`"，那是错的，已废。**

**真正要防的只有"清错地方"**，共两种情况：

1. **在"非配置工作区"里清** —— cwd 不匹配配置里任何 `workspace` ⇒ **整体跳过、什么都不碰**（这才是原先那个"清空 bug"）。
2. **两个 agent 共用一个目录时互删** —— 靠 §三 **把共用目录的 agent 合并 / 归属清楚**（方案 A）。

只要 1、2 成立，`want` 为空就**正常清空**，**不需要**任何特殊处理。

### 连带：`verify` 与 `doctor`

- `verify`（**验收测试命令**：往规则目录放一个带暗号的探针，再让用户开新会话问 agent 有没有看到）保留为**开发/运维工具**，**允许它建目录**（原则 1 的**唯一例外**，写进 `agent-onboarding.md`）。它**不自动判定通过与否**——判定永远靠实际开会话看。
- `doctor` 现在打印 "directory does not exist (created on first sync)"（`cmd/doctor.go:80-85`）⇒ 改为 "不存在（仅当该 agent 自身 hook 触发时才创建）"。

## 三、改动一：每个 agent 独立处理（撤销"共用目录"特殊合并）

**用户要求**：copy / 卸载 / 提示，**一切按 agent 单独处理**；两个**完全相同**的 agent 本质就是**同一个 agent**。

**现状（2026-10-09 前）**：codebuddy 与 workbuddy 曾并作一条记录（workbuddy 作为 codebuddy 的 `Aliases`），靠 `ByRulesDir`+`unionSources`（`cmd/sync.go:72-76`）合并 —— **该合并逻辑已撤销**。

⚠️ **不能只删调用**：若保留两条记录却不再合并，A 同步会把 B 的文件当残留删掉，B 再反删 ⇒ **每次会话文件来回消失**。

**2026-10-09 实测推翻「方案 A」**：本机实测表明 WorkBuddy 有**独立**的用户级配置目录 `~/.workbuddy` 与独立钩子文件 `~/.workbuddy/settings.json`，**不读** CodeBuddy 的 `~/.codebuddy/settings.json`。因此：

- **已撤销方案 A 的别名合并**，改为 WorkBuddy 作为 `registry.go` 里**独立条目**（`HookFile: "~/.workbuddy/settings.json"`）；两者钩子各装各的，互不影响。
- 用户面分离仍保留：安装时把用户写的 `--agent workbuddy` 原样透传到钩子命令与回显（`hooks.TargetCommandFor` + `Install` 的 display 参数）；`Refresh`/`doctor` 比对改用 `ExpectedCommand`，沿用文件里已有的 `--agent` 标识，自愈或 `init --refresh` 都不会把 workbuddy 悄悄改回 codebuddy。

**⚠️ 后续更正（同日，三位置对照探针）**：工作区级规则目录实测为 **`.codebuddy/rules`**（与 CodeBuddy **共享**），并非上面一度推断的 `.workbuddy/rules` —— 见 `external/agent-rules-dirs.md` §5.4。**「用户级独立」≠「工作区级独立」**。

⇒ 因此 `RulesDir` 取 `.codebuddy/rules`，两者**共享目录**，「每个 agent 独立处理」必须升级为：

- **同步取并集**：`agents.SharingRulesDir(a)` 找出同目录的 agent，`config.SourcesForAny(ids, ws)` 取它们 sources 的并集，`engine.Sync` 一次落齐 ⇒ 谁跑都不会删别人的（`ByRulesDir`+`unionSources` 的能力以这个形式**回来了**）。
- **「谁算在用」= 还装着钩子**（`hookedPeers` / `hookInstalled`，基于 `hooks.Inspect`）：卸载即摘钩子、天然退出并集，**不新增任何状态文件**。共享组内一个都没装（纯手动 `sync`）⇒ 退回该组全部 agent。
- **卸载改为收敛**：`pruneRulesDir` 用 `engine.Sync(dir, remainingSrcs, false)` 把目录收敛到「仍在用 agent 的应有集合」；剩余为空 ⇒ want 为空 ⇒ 全清（自然覆盖 `--all`）。⚠ 剩余集合必须在 `hooks.Uninstall` **之前**算好。
- 详见 `dir-sync.md`「共享规则目录」一节。

## 四、改动二：账本升级为「工作区 × Agent」

**结构**（`internal/state/ledger.go`）：

```json
{
  "version": 2,
  "workspaces": ["/abs/ws1", "/abs/ws2"],
  "agents": { "/abs/ws1": ["codebuddy"] }
}
```

- **保留 `workspaces`**（旧文件照常读入），**追加 `agents` map**；`Others()` 语义不变。
- 写入：在 `cmd/sync.go` 的同步循环里按 `a.ID` 记录；**只存规范 `a.ID`**。
- 读取器：`AgentsFor(ws) []string`；哨兵 `"*"`（`state.AnyAgent`）表示「未知 / 全部已支持 agent」，由消费方展开为 `Supported()`。
  v1 老条目迁移时**就地填入并落盘**该哨兵（占位，避免丢信息）；已知具体 agent 的条目则记真实 `a.ID`，两者不混写。
- **路径归一**：入库统一 `resolvePath`（`EvalSymlinks`+`Clean`），与 config 口径一致（否则软链路径会记成两条）。⇒ 需要**导出** `config.ResolvePath`（现为私有 `config.go:253`）。
- **写入安全**：`Save` 改 **临时文件 + rename**（防半截文件）；`Record` 的"load→改→save"再加**文件级锁**（防并发丢更新）。二者互补，缺一不可（详见 §九）。
- ⚠️ **`Record` 的触发条件要收窄**：现在 `cmd/sync.go:100` **无条件**记账 ⇒ 改成**至少有一个目录被真正处理**时才记（否则原则 2 下大量"跳过"会污染账本）。
- **迁移**：旧 `{"workspaces":[...]}` 若被 `json.Unmarshal` 忽略新键 ⇒ 旧记录变"空 agents" ⇒ 下一次 `Save` **整文件覆写、永久丢失** ⇒ `Load` 检测"有 `workspaces`、无 `agents`"时按 v1 迁移，旧条目用**哨兵 `["*"]`** 保留，**绝不删条目**。

## 五、改动三：`rulemux sync --all`（手动全量，只走账本）

- **只遍历账本**；**不读 cwd**；**不用配置里的 `workspace` 通配符**发现工作区。
- **每个账本工作区**：先 `os.Stat` 确认存在（不存在 ⇒ 跳过，**不建**）；再按该工作区记录的 agents（`"*"` ⇒ 全部 `Supported()` ∩ 注册表）逐个同步。
- **顺手同步在此模式禁用**（`--all` 自己已覆盖）。
- **GC**：把账本里"目录已不在"的条目清掉 —— **仅 `os.IsNotExist`（ENOENT）视为真删**；权限不足/断连等其它错误保留；打印清理条数。
- 命名：帮助里**对立式**写清（`uninstall --all` 的 `--all` = 全部 **agent**；`sync --all` 的 `--all` = 全部 **工作区**）。

## 六、改动四：变化提示（Tier-1）

**机制**：Tier-1 的 SessionStart hook 跑完 sync，**若规则确有变化**，就**给模型一句话**（提示它转告用户"规则有变化，请新开会话生效"）；**没有变化 ⇒ stdout 一个字节都不输出**。

**判据（严格）**：

- "有变化" = `Copied ∪ Updated ∪ Deleted` **非空** ⇒ **新增 / 修改 / 删除 三者任一都算变化，都要提示**。
- ⚠️ **不能用 `IsEmpty()`**：它把 `Missing`（**源**文件不存在）也算变化（`engine/sync.go:90-92`）⇒ 会每次会话误报、重开一百次也没用。`Missing` 只走 stderr。
- ⚠️ **修 `Copied` 的判定**：现在 `errOld != nil`（读**目标**文件失败，非仅 ENOENT）也记 `Copied`（`engine/sync.go:118-130`）⇒ 只把 `os.IsNotExist(errOld)` 记 `Copied`；其它读错误单列 `Errors`，**不触发提示**。
- **不采用"哈希/指纹代替逐字节比较"**：无 manifest 时算哈希仍必须先读文件（读盘量不变、无收益），`bytes.Equal` 是精确判等。⇒ 判定照旧用逐字节比较。

**输出通道（硬原则）**：

> **hook 绝不注入任何未经用户同意的内容**。⇒ 给人、给日志看的一律走 **stderr**；**stdout 只在「hook 路径 且 有变化」时输出恰好那一条用户已同意的提示**；**无变化 ⇒ stdout 为空**。

- `printSyncResult`（`cmd/sync.go:143-163`）现**无条件**把人类文本打到 **stdout** ⇒ 全部改走 stderr，并重构为接收 `io.Writer` 以便单测。
- `--hook` 才允许写 stdout 协议；手动跑 `sync`/`--all` **绝不**吐协议。
- `Missing` 告警保持 stderr，且**不抬退出码**（避免 host 因非零退出丢弃 stdout）。
- 协议形态（**已实证**，2026-10-09）：stdout 输出 `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"…"}}`。
  **证据**：`~/.codebuddy/settings.json` 的 SessionStart 挂着 `node ~/.hindsight/coding-agents/dist/codebuddy-sessionstart-hook.js`；该脚本的 `emit`（`codebuddy-sessionstart-hook.js:3940-3945`）= `hookSpecificOutput{hookEventName:"SessionStart", additionalContext}`，并由 `process.stdout.write(JSON.stringify(payload))` 写出（`:2915-2918`）——用户每会话都在跑它，故宿主确实消费此形状。
  另有 `systemMessage` 通道（TUI 横幅），**本设计不使用**（保持最小，只让模型转告用户）。

**per-agent（用户反复强调"每家单独处理"）**：

- 落点三层：① **事实** → `external/agent-rules-dirs.md` 逐 agent 补两列：**会话内是否会重读规则目录** + **hook stdout 输出协议**；② **编译期开关** → `registry.go` 的 `Agent` 加 `SessionHint bool` + `HintProtocol string`（`""` = 不提示）；③ **checklist** → `agent-onboarding.md` §零 补"确认会话内加载语义 + 输出协议 ⇒ 决定是否提示"。
- 协议分支写在 `cmd/sync.go` 的输出函数里；`hooks/install.go` 只按 per-agent 字段参数化 hook 条目。
- 提示**只挂 SessionStart**，不新增 `UserPromptSubmit`（会累积）。
- **前置 canary（必须先做）**：坐实 CodeBuddy/WorkBuddy 是否真消费 `hookSpecificOutput.additionalContext`。**未坐实前 `SessionHint`/`HintProtocol` 不写死。**

**Tier-2 的通道语义（须明确）**：Tier-2（`inject`）的 stdout 是**规则正文载荷**；§六的 stdout 是**提示协议**。二者**按 agent 互斥**（Tier-2 不走提示），文档与代码都要写清，避免后续混用。

**失败容错**：`engine.Sync` 现在任一写/删失败即 `return err`，**已完成的 `Copied` 全丢** ⇒ 改**分文件容错**：吞单文件错误、继续；返回已完成的 `Copied/Updated/Deleted` + 单列 `Errors`；提示判据只看前三者。

**并发**：合并 agent 后同一目录只剩一条 hook，双报风险大减；仍以**目录级写前判定**兜底。

## 七、改动五：账本驱动的顺手同步

**机制**：hook 触发的 `sync --hook --agent X` 完成后，查账本"该工作区还用过哪些 agent"，对其中**写目录的 Tier-1** 顺手同步（**只写已存在目录**）；**纯注入的 Tier-2** 不处理。

**硬规则**：

- **目录存在性只认 `os.Stat`**（不认账本）；不存在 ⇒ 跳过 + stderr 一行日志，**且不 `Record`**。
- 只处理 **`Verified == true`**；`Get` 查不到 / 未验证 ⇒ 跳过 + 一次性告警 + **保留账本条目**。
- **want 为空则不碰**（连目录都不建）。
- **只级联一层**（`visited`）；**`--all` 模式禁用**。
- **不污染主退出码**（顺手失败只记 warning）。
- **无开关、永远打开**（用户明确：只读账本 + 只写已存在目录，不会出问题）。
- 越界语义写清：会往"用户当前没在用、但历史用过"的 agent 目录落文件（用户要求如此）。

## 八、命令语义矩阵（新增·定稿）

| 调用 | 处理范围 | 建目录 | 记账 | 顺手同步 | stdout |
|---|---|---|---|---|---|
| `sync --hook --agent X`（SessionStart hook 调） | cwd 工作区 × X | ✅ **只 X 自己的目录** | 有实际处理才记 | ✅ 一层 | 有变化 ⇒ 协议 JSON；无变化 ⇒ 空 |
| `sync --agent X`（手动） | cwd 工作区 × X | ❌ | 有实际处理才记 | ✅ 一层 | 空 |
| `sync`（手动，无 `--agent`） | cwd 工作区 × 全部 `Supported()` | ❌ | 有实际处理才记 | ✅ 一层 | 空 |
| `sync --all` | **账本全部工作区** × 各自记录的 agents | ❌ | 有实际处理才记 | ❌ 禁用 | 空 |
| `sync --hook`（无 `--agent`） | — | — | — | — | **报错** |
| `sync --all` + `--workspace` | — | — | — | — | **报错**（互斥） |

- **cwd 不匹配任何配置 `workspace`** ⇒ 该次 `sync` 在 cwd 上**整体跳过**（不建不删不记），只保留对账本的处理（若有 `--all`）。
- **"配置认可的工作区"** = 至少有一条 `[[source]]` 的 `workspace` 匹配该 cwd（含省略/`*`/`all` 的全局 source）⇒ 新增 `config.DeclaresWorkspace(ws) bool` 承接此判定。

## 九、迁移专章 + 锁 + 实证（新增·定稿）

### 9.1 迁移清单

| 对象 | 识别 | 处理 |
|---|---|---|
| 账本 v1（只有 `workspaces`） | `Load` 见 `agents` 缺失 | 按 v1 迁移，旧条目 agents = 哨兵 `["*"]`（**不删条目**） |
| 旧 hook 命令串（无 `--hook`） | `install.go` 的 `isRulemuxHookFor` 读到的 command 不含 `--hook` | **重写**为新命令串；`doctor` 提示"hook 格式过旧，请重跑 `rulemux init --agent X`" |
| 旧 `--agent workbuddy` hook 条目（别名时代写在 `~/.codebuddy/settings.json`） | 方案 A 撤销后 `Get("workbuddy")` → ID=workbuddy（独立条目），它按 `~/.workbuddy/settings.json` 匹配，够不到 codebuddy 文件里那条 | **孤儿，不迁移**：WorkBuddy 本就不读 `~/.codebuddy/settings.json`，该条目永不触发；重装（拆后）会写进正确的 `~/.workbuddy/settings.json`。如需清理，手动删或 `uninstall --agent codebuddy` 会带上它（`dropRulemuxHooks`/`isRulemuxHookFor` 仍接受标识集合） |
| 旧前缀 `.rulemux__*`（点前缀，v0.1.3 及更早） | 文件名以 `.rulemux__` 开头 | **不自动清理**（用户 2026-10-09 明确）；仅 `doctor` 提示存在旧残留 |
| 历史：工作区级 `.codebuddy/settings.json` 钩子 | 已随 v0.1.1 迁到 user 级 | 不处理（历史坑，已在 `external` 文档留痕） |

### 9.2 账本写入并发方案

- **原子替换**：写 `workspaces.json.tmp` → `os.Rename`（防半截文件）。
- **防丢更新**：`Record` 的 load→改→save 用 **锁文件** `~/.rulemux/ledger.lock` 包裹（`O_CREATE|O_EXCL` 原子创建，跨平台、零依赖），重试 3×50ms；拿不到锁 ⇒ 放弃写入 + stderr warning（**不阻塞**、不抬退出码）。超时（>2s）的陈旧锁文件视为可接管。
- 二者互补：rename 防"写坏"，锁防"覆盖"。

### 9.3 动工前的实证（canary）

1. **【阻塞 §六 协议常量】** CodeBuddy/WorkBuddy 是否真消费 `hookSpecificOutput.additionalContext`。
2. 用**对照探针**复核"读规则先于 hook 写入"。
3. ~~WorkBuddy 是否读 `.codebuddy/rules` 下与 CodeBuddy 同一批文件（方案 A 的前提）~~ → **2026-10-09 已答：否**。WorkBuddy 有独立配置目录 `~/.workbuddy`（§5.4），方案 A 前提不成立；**仍待测**：其工作区级 rules 目录究竟是 `.workbuddy/rules` 还是 `.codebuddy/rules`（🔴，本次先以 `.workbuddy/rules` 占位）。
4. "无变化 ⇒ stdout 为空"时 host 不把空 stdout 当上下文注入。
5. hook 非零退出时 host 是否丢弃 stdout。

## 十、连带修正

1. **红线改写**（`agent-onboarding.md:69`）：本意是「不许枚举/探测本机装了哪些 agent」，不是"`--agent` 必填"。改为：
   > 不扫描机器检测 agent | 目标 agent **只来自编译期注册表**（`agents.Supported()`）；任何命令都不得枚举/探测本机已安装的 agent（不读 PATH、不扫安装目录、不查包管理器/进程），也不以"是否安装"作为投递前提。
   > `init` 仍**必须**显式 `--agent`；`sync` 的 `--agent` **可选**，缺省 = `Supported()` 全部。
2. **`--agent` 缺省** = 全部 `Supported()`；注意现实现（`sync.go:108-123` `targetAgents("")`）只返回 Tier-1，需与"全部 Supported"对齐。
3. **口径统一**：`uninstall --all` 现用 `agents.All()`（含未验证），`sync` 用 `Supported()` ⇒ 统一到 **`Supported()`**。
4. **别名归一化（四个入口）**：① 配置 `agents` 值（`config.go:186-198` 现按原字符串匹配，`agents=["workbuddy"]` 不会命中合并后的 codebuddy）；② CLI `--agent`；③ hook 命令行 agentID；④ 账本读写。收敛为**单一入口函数**。
5. **README/usage**：删 `README.md:176-184` / `README.zh-CN.md:164-169`「改完配置后手动跑 sync」段；命令表（`README.md:208` / `README.zh-CN.md:191`）与 `main.go:31` usage 补 `sync --all` 与 `--hook`。
6. **A3 例外登记**：在 `architecture.md`（§一 铁律处）与 `requirements.md`（A1–A3 处）显式登记"变化提示"是**瞬态、一次性、有变化才发**的例外，并回链本稿。
7. **全仓旧前缀残留**（真值 `__rulemux__`）：`dir-sync.md:12,13,17`、`features.md:9`、`architecture.md:45`、`agent-onboarding.md:48`、`implementation.md:85,106,107,115,118,122,149,150`、`verification.md:10,11`、`PROGRESS.md:30,35,36,37,38,46`、`ops/implementation-workplan.md:82,126,127,145`、`worklog/canary-2026-10-08.md:15,16,26`、**`scripts/smoke.sh:63-141`**（冒烟脚本仍断言旧前缀，对当前代码已失真）。
8. **其它矛盾**：`architecture.md` §五 标题/`:76` 与 `:64` 自相矛盾；`PROGRESS.md:35`（T10 仍写"会读点文件"）与 `external/agent-rules-dirs.md:114`（不读）冲突；`external/agent-rules-dirs.md:117` 需撤回"钩子先于读规则"。
9. **`uninstall` 文件归属**：`removeRuleFiles`（`uninstall.go:158-171`）用 `dir/__rulemux__*` 通配删 ⇒ 方案 A 下每目录只剩一家，风险消除；文档标注"零改动仅在方案 A 前提成立"。
10. **`Record` 签名变更**：同步更新 `agent-onboarding.md:57` 的 `state.Record(ws)` 与 §四 账本小节。

## 十一、漏洞清单（四轮合并）

| # | 漏洞 | 处置 |
|---|---|---|
| 1 | 旧账本升级**静默丢数据** | §四 迁移 + 哨兵 `["*"]` |
| 2 | `IsEmpty()` 把 `Missing` 当变化 ⇒ 永久误报 | 判据用 `Copied∪Updated∪Deleted>0` |
| 3 | `errOld != nil` 把读失败当 `Copied` | 仅 `os.IsNotExist` 记 `Copied` |
| 4 | want 空会清空目录 | **照常清空**（写进去的必须自己清）；保证 §二 两条前提 |
| 5 | 账本读改写**无锁非原子** | 临时文件+rename **加** 锁文件（§9.2） |
| 6 | 账本只 `Clean`、config 用 `resolvePath` | §四 统一 `ResolvePath`（需导出） |
| 7 | 账本存别名/失效 agent | 只存规范 `a.ID`；跳过+告警+留条目 |
| 8 | `Sync` 无条件 `MkdirAll` | §二 加 `create` |
| 9 | 提示是动态区注入，违反铁律/A3 | §十 #6 登记为 A3 瞬态例外 |
| 10 | 「快照 vs hook 先后」文档矛盾 | 已坐实；撤回 `:117` |
| 11 | stdout 被 host 当上下文 ⇒ 污染+计费 | §六 人类文本→stderr |
| 12 | Tier-2 两次注入累积 | 只 SessionStart 一次 |
| 13 | cwd 软链/子目录 ⇒ 账本错位 | §四/§六 路径归一 |
| 14 | 顺手同步越界建目录 | §二 原则 2；§七 want 空不碰 |
| 15 | 顺手同步递归/环 | §七 仅一层；`--all` 禁用 |
| 16 | `uninstall` 整目录 glob 连删别家 | §三 方案 A + §十 #9 |
| 17 | sync 写错误即 return ⇒ 结果/提示丢失 | §六 分文件容错 + `Errors` |
| 18 | 两 hook 并发 ⇒ 双报/双提示 | §三 合并 + §六 写前判定 |
| 19 | `Missing ⇒ exit=1` ⇒ host 可能丢弃 stdout | §六 退出码与提示解耦 |
| 20 | Tier-2 使用历史不入账本 | 明确"账本只覆盖 Tier-1" |
| 21 | `Record` 无条件记账 | §四 收窄触发条件 |
| 22 | `verify` 建目录违反原则 1 | §二 列为**唯一例外** |
| 23 | `doctor` 文案过时 | §二 改文案 |
| 24 | 新契约无测试覆盖 | §十二 测试矩阵 |
| 25 | "哈希代替逐字节"无收益 | §六 说明不采用 |
| 26 | `--hook` 迁移：旧 hook 无此开关 ⇒ 提示/建目录静默失效 | §9.1 迁移 |
| 27 | 方案 A 后旧 `--agent workbuddy` 钩子成孤儿 | §9.1 迁移（别名集合匹配） |
| 28 | 别名归一化只覆盖账本 | §十 #4 四入口 |
| 29 | Tier-2 stdout 语义与提示通道冲突 | §六 末段 |
| 30 | 首次建档必然触发一次提示 | 接受（语义正确：该会话确实没读到规则） |
| 31 | `scripts/smoke.sh` 用旧前缀 | §十 #7 |
| 32 | `config.ResolvePath` 未导出，state 包用不了 | §四 导出 |

## 十二、测试矩阵 + 实现顺序

**测试矩阵**（全部 Go 标准库；`cmd`/`state` 目前**零测试**，需新建）：

1. `engine`：目录不存在 + `create=false` ⇒ 不建、返回 `SkippedNoDir`；`Missing` 不算变化；读目标非 ENOENT 不记 `Copied`；分文件容错（一失败其余仍完成 + `Errors` 单列）。
2. `state`（**新建** `ledger_test.go`）：v1→v2 迁移不丢数据 + 哨兵；原子写不产生半截文件；`Record` 只在"确有处理"时写；`AgentsFor`。
3. `cmd`（**新建** `sync_test.go`）：无变化 ⇒ stdout 字节级为空；人类文本走 stderr；`--hook` 才输出协议；非配置工作区整体跳过（不建/不删/不记）；`--all` 遍历账本 + `Stat` 跳过 + GC 仅 `ENOENT`；顺手同步单层/不级联/want 空不碰；`--all`+`--workspace` 报错。
4. `config`：`DeclaresWorkspace`；别名归一（4 入口）。
5. `agents`/`cmd/flags`：workbuddy → codebuddy 别名解析 + `Supported()` 去重（**会推翻现 `flags_test.go:24-30` 期望**，需同步改）。
6. `hooks`：新命令串含 `--hook`；旧 `--agent workbuddy` 条目可被清理。
7. `scripts/smoke.sh`：旧前缀改新前缀 + 补新契约端到端。

**实现顺序**（依赖修正后）：

```
1（修清空 bug） → {2（stderr 契约）, 3（create 语义）, 4（账本 v2，含先导出 ResolvePath）}
              → 5（撤销合并，与 3 同批） → {6（sync --all）, 8（顺手同步）}
              → 7（变化提示，先 canary） → 9（文档 + 脚本 + 测试收尾）
```

- **最小可交付第一步** = 步骤 1（无前置依赖、可立即 `go test ./...` + `smoke.sh` 验证）。
- 步骤 6/8 复用同一套"账本遍历"内部抽象。
- 步骤 2 需先把 `printSyncResult` 改为接收 `io.Writer`，否则"stdout 零输出"无法单测。
- 步骤 7 的协议常量等 canary 坐实后再写死。
