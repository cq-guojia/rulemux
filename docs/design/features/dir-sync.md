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
  ⚠ **此判据只用于共享目录的并集计算**。裸 `sync` 的**投递范围**另有一条账本门禁（2026-10-14）：
  `账本(ws) ∪ 配置显式点名 − off`，`off` = 机器级「已卸载」集合（`state.Ledger.Off`）；
  注入交付的 agent（dsh，2026-10-14 起 Tier-2）没有规则目录、也没有钩子，两组判据都不参与。
- **兜底**：共享组里一个都没装钩子（纯手动 `rulemux sync` 场景）⇒ 退回该组**全部** agent 的并集（减 `off`），
  保证手动跑仍有东西落。
  ⚠ 注意**两侧不对称**：收敛侧（`remainingPeers`）**没有**这个兜底 —— 纯手动场景下若组里一个钩子都没装，
  `uninstall --agent X` 会把整组前缀文件一并清掉（无法证明谁还在用 ⇒ 不保留）。这是既有取舍，未在本次改动。
- **卸载收敛**（`internal/cmd/uninstall.go` 的 `pruneRulesDir`）：卸载某个 agent 时，把目录
  **收敛到「仍在用的同目录 agent 的应有集合」** —— 删掉被卸者独有的、保留共有的；剩余为空则清空
  （这正好等于 `uninstall --all` 的全清语义）。⚠ **剩余集合必须在 `hooks.Uninstall` 之前算好**，
  钩子一摘就判不出谁还在用了。
  2026-10-14 补：收敛后**确认为空**的工作区级目录连目录一起删（`removeEmptyLeaf`：Lstat 排除软链、
  `len(ReadDir)==0` 才算空、只 `os.Remove` 绝不上溯父目录、用户级目录永不删）；且**先清理、后动账本**
  —— 该 agent 收进 `off` 并从「清过或确无东西可清」的工作区列表移除，清理被跳过（配置读不了 /
  工作区未声明 / 单文件失败）的工作区条目原样保留。当前工作区的 skills 目录与 rules 同规收敛。
- **不新增状态**：整套判定只依赖「钩子装没装」这一既存事实 + 配置本身，无 manifest。
- 不共享目录的 agent（claude `.claude/rules`、trae `.trae/rules`、codex/opencode 不落盘）走原逻辑，
  `SharingRulesDir` 只返回自己，行为完全不变。

⚠ **代价**：目录共享时，最终落盘的是**并集** —— 若某条 source 只写了 `agents = ["workbuddy"]`，
它的文件同样会出现在 `.codebuddy/rules` 里被 CodeBuddy 读到。这是共享目录的固有语义，无法两全。

## DSH（DeepSeek Harness）：纯注入交付（2026-10-14 起）

dsh 是 **plugin-first** 宿主：它**不原生扫描规则目录**，扩展面是 Cordis 生命周期事件（`agent/session-start`、
`agent/pre-step`…），且**没有 hook binary**。因此 dsh 走**注入交付**，与 codex / opencode 同类（Tier-2）：

- **不落盘**：`rulemux sync` 对 dsh 是空操作（Tier-2 没有规则目录，同步循环直接跳过），工作区里没有任何中间
  目录 —— 没有东西需要保持同步，也没有残渣需要清理。
- **读（仓库子包 [`dsh-plugin/`](../../../dsh-plugin/)，npm 名 `rulemux-dsh`）**：一个 DSH 插件包，**按 DSH 正常方式安装** ——
  `dsh plugin --profile <p> add rulemux-dsh`（npm / 本地 `.tgz` / git），**不靠 rulemux 自己装**。行为：
  `agent/pre-step` 首个带用户输入的回合跑 `rulemux inject --agent dsh`（cwd = 会话工作区）取渲染文本，
  **只注入一次**为 `plugin:rulemux` recall 消息（compaction 遮蔽后才补回）；per-turn 不做任何文件扫描；
  调用失败只记日志、本回合不注入，不影响会话。
- **插件装载（dsh 启动）时自动就绪（自给自足）**：插件包**不声明任何依赖** —— 硬依赖会让 `dsh plugin add` 因镜像/源滞后而整体失败。
  就绪（装 CLI、生成配置）由插件在 `apply()` 被调用时**立即**开始（dsh 启动装载即调用，早于任何会话），全进程一次、幂等；
  因此 `装插件 → 重启 dsh → 改 ~/.rulemux/config.toml → 开一次会话` 即可注入，不需要「先聊一句把配置生出来」的额外会话。
- **就绪 = 三步，全成或抛错**：① CLI 可得**且版本满足 `>=0.4.2`**（0.4.2 是首个能解析本插件配置里行尾 `#` 注释的版本；偏旧则 `pnpm add -g` → 退 `npm install -g` → 再退 `npm install -g --force`
  升级后复查，仍不满足即失败；`--force` 覆盖手工放在 PATH 上的旧 `rulemux`）；② 插件已装载（插件在跑即已成立，
  无需也无法自检）；③ `~/.rulemux/config.toml` 存在（缺失则 `rulemux init --agent dsh` 生成，**已存在绝不覆盖**）。
  任一步不成立即**抛错**（阻塞首个回合、不设等待上限），不做降级、不给「成功了一半」的提示。
- **取规则不参与就绪判定**：`rulemux inject --agent dsh` 每会话跑一次；失败（含配置里没有任何 `[[source]]`、
  或没有覆盖本工作区的源）**只记日志**、本回合不注入 —— 既不影响会话，也不影响上面三步的成败。
- **token 成本**：与 Trae/CodeBuddy 原生载入文件完全相同（进上下文的是同一段规则文本，字节一致）。

> ⚠ rulemux 自身**不往任何 host 注入**，也**不替 dsh 装插件**（registry 里 dsh 是 `Style="external"`，
> init/doctor 只打印装插件提示）：rulemux 交付的是**渲染文本**（`rulemux inject`），把文本放进会话的动作
> 由**子包插件**完成。uninstall 也停不掉它 —— 插件每个会话都会继续调 `inject`，所以 `uninstall --agent dsh`
> 会明说这一点，并给出有据的停用方式（cordis.patch.yml 的行 `disabled: true`）。
>
> 📌 **历史（已作废）**：2026-10-10 ～ 2026-10-14 用过「写一半 + 读一半」的拆法 —— Go 把真实拷贝写进
> `<workspace>/.dsh/rules`，插件读回注入（当时的容器 canary 见
> [`../../worklog/202610/dsh-adapter-9f1e.md`](../../worklog/202610/dsh-adapter-9f1e.md)）。该拆法于 2026-10-14
> 作废：那个目录宿主不认、只是两边之间的交接物，一旦留下就会被反复重写 —— 正是「机器上没装 dsh，`.dsh/rules`
> 却被持续改写」那个缺陷的舞台。**旧 canary 结论随之作废，新链路需重跑 canary**；registry 里 dsh 仍标
> `Verified: true`（沿用 2026-10-10 对「插件能被装载、能注入成功」的核实）。
>
> 📌 **发布顺序（硬约束）**：**插件先发、CLI 的 Tier 翻转后发**。新插件对旧 CLI 也可用（`inject` 早已存在，
> dsh 换成 Tier-2 只影响 `sync`）；反过来则会出问题 —— 老插件仍去读残留的 `.dsh/rules`，注入的是陈旧内容。
> 事实与真源见 [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md) §一 DSH 行（含 2026-10-14 更新说明）。

## 边界（不做什么）

- 不做内容改写 / 模板渲染 —— 源写什么就落什么。
- 不做注入 —— 注入属 Tier-2 降级方案 [`hook-injection.md`](hook-injection.md)；Tier-1 只真实拷贝。
- 不常驻、不监听文件改动（极致简单，见 [`../implementation.md`](../implementation.md) #8）。
