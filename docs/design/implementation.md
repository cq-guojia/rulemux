# rulemux 实现方案：待定项清单

> **状态**：📝 进行中（逐条拍板，拍板一项划掉一项）
> **来源**：用户 2026-10-07 要求立项为「实现细节设计方案」任务，基于本文档逐项讨论后落定
> **配套**：[`requirements.md`](requirements.md)（要什么）· [`architecture.md`](architecture.md)（为什么）· [`features.md`](features.md)（怎么做）· [`external/agent-rules-dirs.md`](external/agent-rules-dirs.md)（各家目录）· [`../PROGRESS.md`](../PROGRESS.md)（在办任务 T0–T7）

---

## 用法

本文件是「实现阶段」的**任务清单 + 决策记录**。每条初始状态 `⬜ 待讨论`；讨论拍板后改为 `✅ 已定`，并把结论简短写进「结论」栏。
不在此复述需求/架构——那两套是真源，本文件只记「要拍什么板、拍完是什么」。

---

## 一、总体形态

### 1. 实现语言与构建 ✅ 已定
- **问题**：Node/TS 还是 Go/Rust 单二进制？
- **关联**：PROGRESS 下一步第 4 条（技术选型，结论进 `architecture.md`）。
- **结论**：**Go 单二进制**。已核实 Claude Code 的 SessionStart hook 可经 exec 形式直接 spawn PATH 上的编译二进制（见 `external/agent-rules-dirs.md` §三），故**零运行时依赖**（无 Node/VM）可行；Go 比 Rust 更直白易读、三平台交叉编译容易、体积小吃得开。若你改主意要 Rust 说一声。（按你上条表态：可读性 OK + 零依赖即选之）
- **待定**：具体构建脚本（`go build` 交叉编译）、产物命名。

### 2. CLI 形态与 bin 名 ✅ 已定（默认，可调）
- **问题**：单命令 + 子命令？bin 名？
- **结论**：**`rulemux`** 单二进制 + 子命令：
  - `rulemux sync`：主命令，被各 agent 的 SessionStart hook 调起，按配置把源文件复制进对应 agent 规则目录（带前缀 + 比对 + 删残留）。
  - `rulemux init`：生成示例 `config.toml` 并为已装 agent 安装 SessionStart hook（幂等）。
  - `rulemux doctor`：环境自检（PATH、各 agent 是否装 hook、配置是否合法）。
  - `rulemux verify`：canary 验收（第 13 条）。
- **默认**：子命令集合如上；`init` 生成示例配置并装 hook。如需增减子命令再调。

### 3. 分发与平台 ✅ 已定
- **问题**：npm 全局安装 / `npx rulemux` / 预编译二进制？
- **结论**：**交叉编译的 Go 原生二进制**为主分发形态；`npm i -g rulemux` 仅作为把二进制放进 PATH 的便捷通道（或直接 GitHub Release 下载）。零运行时，win/mac/linux 通吃。Windows 发 `rulemux.exe`（exec 形式 hook 可直接 spawn）。
- **待定**：是否仍保留 npm 包作为分发入口（包内不含运行时，只搬运二进制）。

## 二、源文件模型（无中央真源）

### 4. 源文件位置 ✅ 已定
- **问题**：有没有一个 rulemux 拥有的「中央真源」目录？
- **结论**：**没有中央真源**。rulemux 不拥有、不维护任何规则内容。用户在配置里**列出任意位置的源文件**（可在 C 盘、D 盘、任何路径，磁盘任意处），rulemux 只负责同步时把它们复制进各 agent 目录。源文件完全由用户自己维护——改名、挪位置、换内容，rulemux 都不干涉，只按配置当前列出的项做事。（除非做远程/多端同步才需要中央真源；当前不做。）
- **依据**：用户 2026-10-07 决策。

### 5. 源文件约定 ✅ 已定
- **问题**：源文件格式？允不允许子目录？要不要中央元信息（frontmatter）？
- **结论**：**源文件即用户任意 `.md` / `.txt` 等文本文件**，rulemux **原样复制**，不做任何格式转换、不加 frontmatter、不拼合。无中央元信息——「适用于哪些 agent」由配置文件（第 6 条）声明，而非源文件内嵌。子目录：源可以是任意路径文件，复制进目标时取原 basename（见第 10 条命名规则）。
- **待定**：无（格式约定已定）。

## 三、目标发现与注册

### 6. targets 与配置来源 ✅ 已定
- **问题**：targets 怎么来？配置格式？配置放哪？
- **结论**：读一份 **TOML 配置**（默认 `~/.rulemux/config.toml`，位置可任意指定，非必须工作区——因 rulemux 由 hook 在 agent 调用中拉起，读的是自己的配置而非工作区）。字段：
  - `workspace`（可选，顶层）：工作区根目录。**留空 = 用「当前工作目录」**——钩子调起 rulemux 时 agent 的 cwd 天然就是工作区，所以通常不用配。优先级：`--workspace` 命令行 > 配置里的 `workspace` > cwd。
  - `[[source]]`：一条投递规则，含
    - `path`：源文件路径（磁盘任意位置），**单个字符串或数组都支持**（数组里的同一批文件共享下面的 `agents`）；
    - `agents`：投递给哪些 agent，取值 `claude / codebuddy / workbuddy / trae / codex / opencode`，**省略 = 全部**。
  各 agent 适配器按自身规则目录落地。
- **示例**：
  ```toml
  workspace = "/path/to/project"   # 可选；省略则用当前工作目录

  [[source]]
  path = ["C:/rules/a.md", "C:/rules/b.md"]   # 数组：一批文件共享 agents
  agents = ["claude", "codex"]

  [[source]]
  path = "D:/notes/c.txt"                     # 单个字符串也支持
  agents = ["trae"]
  ```
- **依据**：用户 2026-10-07 决策（TOML 定；配置目录可任意）。
- **待定**：无（配置形态已定）；多 workspace 支持留作实现细节。

### 7. 各 agent 规则目录事实 ⬜（文件夹支持已核实，DeepSeek 已移出）
- **问题**：各家是否读**整个文件夹**的全部 `.md`（决定 Tier-1 丢文件 vs Tier-2 注入）、目录路径、扩展名、是否读隐藏/点文件。
- **关联**：T1 / T3 / T4 / T6 / 第 9·12 条。
- **进度（2026-10-07 官方/社区核实）**：
  - **Tier-1（读整个文件夹 ⇒ 丢前缀文件）**：Claude Code `.claude/rules/*.md` ✅；CodeBuddy `.codebuddy/rules/` ✅（社区确认平铺 .md 自动加载；官方文档暗示「每条规则一个子文件夹」，精确结构以 canary 确认）；Trae `.trae/rules/*.md` ✅；WorkBuddy（与 CodeBuddy 同源，`.codebuddy/rules/`）✅ 待 canary。
  - **Tier-2（只认单文件 `AGENTS.md` ⇒ 走 hooks 注入，不碰用户文件）**：Codex `AGENTS.md`（根目录）❌ 无文件夹模式；OpenCode `AGENTS.md` ❌（文档仅提单文件，待确认有无文件夹）。
  - **DeepSeek harness**：用户 2026-10-07 决定**移出当前范围**（最开放、支持插件，后续以插件市场解决）。
- **待 canary 验收（第 13 条）**：① 各家「读全部 .md」是否**跳过点文件**（影响 `.rulemux__` 隐藏前缀是否生效）；② CodeBuddy 规则目录精确结构（平铺 vs 每规则子文件夹）；③ 钩子确实在读规则之前触发（第 8 条链路）。
- **待补写**：把上述目录事实回写 `external/agent-rules-dirs.md`。

## 四、同步与适配

### 8. 同步触发方式 ✅ 已定
- **问题**：怎么触发同步？要不要常驻进程？
- **结论**：**无守护进程、不监听文件改动**（极致简单、不占本机资源）。唯一机制 = 各 agent 的 **SessionStart hook（exec 形式直接 spawn 二进制）调起 `rulemux sync`**，把配置所列源文件复制进该 agent 原生规则目录；文件持久化在原生目录 ⇒ 每个新会话（及 `/compact` 后）必加载，满足 Tier-1 / A1–A3。
- **同会话即时新鲜度**：不保证（已与用户确认接受）——正在跑的会话已冻结系统提示词缓存，改中央规则不会当轮重读；最贴近的刷新是 `/compact` 或重开会话。
- **压缩 hook 定位**：**不属于主路径兜底**，而是 Tier-2 降级（第 12 条）内的一种手段——仅当某 agent 无原生规则目录、走 hooks 注入时才可能涉及压缩时重写逻辑。
- **链路已对 3 家坐实（官方文档 2026-10-07 核实）**：Claude Code / CodeBuddy / Trae / Codex 均有 SessionStart hook，且均在「读规则 / 首轮对话之前」触发：
  - **CodeBuddy**：SessionStart = 启动或恢复会话时运行；Rules 文档明言 *"rules are only added at the beginning of each session… start a new conversation session after creating or modifying rules for them to take effect"*（改后需开新会话才生效）→ 新会话必重读；`CODEBUDDY.md` 默认全量加载。精确先后官方未逐字钉死，以第 13 条 canary 验收。
  - **Trae**：SessionStart 明确 *"创建 Session 后、发起第一个对话前"*；`.trae/rules/*.md` 与 `AGENTS.md` 会话开始读取。链路最清晰。
  - **Codex**：SessionStart 用途即 *"加载工作区约定、注入持久记忆"*（正合本场景）；`AGENTS.md` 会话开始读取。**前提**：`[features]` 开 `codex_hooks = true` 且 `/hooks` 批准新钩子。
  - **兜底不设守护**：因三家均在会话开始读规则，即便某 agent 未装 hook，只要 `rulemux sync` 在开会话前跑过（一次性命令、不常驻），文件即在目录中被读到 → 满足「极致简单、不常驻」。
- **依据**：`external/agent-rules-dirs.md` §三（Claude Code）；CodeBuddy《Hooks 使用指南》+《Rules》2026-08-26 / 2026-03-02；Trae `docs.trae.cn/ide_hook-configuration-reference` + 官方社区；Codex hooks 文档（SessionStart 事件）。
- **架构前提（已定）**：rulemux 采用「核心引擎 + 每 agent 独立插件/适配器」（形如 HINDSIGHT 的每 agent 集成）；适配器自报 Tier-1（有原生规则目录、SessionStart 复制）或 Tier-2（走 hooks 注入）。接口归第 9 / 12 条。
- **待定**：各 agent 具体 hook 安装路径归第 12 条；精确「钩子先于读规则」以第 13 条 canary 实测坐死。

### 9. 逐 agent 格式适配 ✅ 已定
- **问题**：中央 `.md` → 各 agent 期望格式（单文件 vs 多文件、头尾标记）。
- **结论**：**不做格式合成、不拼合**。每个源文件 **1:1 复制**进目标，文件名加前缀 `.rulemux__`（见第 10 条）。各 agent 适配器只负责「把带前缀文件放进正确的规则目录 / 或在 Tier-2 下走 hooks 注入」，**不改内容、不加 frontmatter、不合成单文件**。
  - **Tier-1（文件夹 agent）**：直接把 `.rulemux__<basename>` 丢进其规则目录。
  - **Tier-2（单文件 agent）**：不碰用户的 `AGENTS.md`，改由 SessionStart hook **注入**规则内容到上下文（见第 12 条）。
- **放弃**：早前「单文件 agent 用 RULES BEGIN/END 受管区标记」方案——用户决定单文件 agent 直接走 Tier-2 注入，不染指用户文件。
- **依据**：用户 2026-10-07 决策（目录优先、单文件降级、不碰用户配置）。

### 10. 文件映射与增删 ✅ 已定
- **问题**：源增删 → 目标同步增删（自由 add/delete）；冲突怎么处理。
- **结论**：**前缀 + 确定性命名 + 比对**，无状态文件：
  - **命名**：目标文件名 = `.rulemux__` + 源 basename；若不同目录出现同名 basename，追加源路径短 hash 去重。
  - **同步算法（每次 `sync`）**：
    1. 计算「当前配置应生成的带前缀文件名集合」S。
    2. 枚举目标目录所有 `.rulemux__*` 文件：不在 S 中的 ⇒ **删除**（删残留）。
    3. 对 S 中每项：目标不存在 ⇒ 复制；存在且内容一致 ⇒ 跳过；内容不一致 ⇒ **覆盖**。
  - **增**：新配置项 ⇒ 复制进目标。
  - **删**：配置移除某项 ⇒ 其带前缀文件不在 S ⇒ 被删。
  - **冲突/手改**：目标目录里**非** `.rulemux__` 前缀的文件（用户自己的）一律不动；用户若自己建了 `.rulemux__` 前缀文件，按「我们的」对待，照常管理/删除（用户认可）。
- **依据**：用户 2026-10-07 决策（固定前缀、不建 manifest、比对删残留）。

### 11. 幂等与不累积保证 ✅ 已定
- **问题**：同一文件只写一次、不每轮追加、不累积。状态存哪？
- **结论**：**无状态文件**（用户决策：不建 manifest/阶层）。幂等由第 10 条的「内容比对 + 删残留」保证：
  - 内容一致则跳过 ⇒ 不重复写、不追加。
  - 配置移除 ⇒ 带前缀文件被删 ⇒ 不累积陈旧文件。
  - 前缀区分「我们的」与「用户的」⇒ 不误删用户文件。
- **依据**：用户 2026-10-07 决策。

## 五、降级方案（Tier-2 hook 注入）

### 12. 无原生规则目录的 agent 降级（Tier-2）✅ 已定
- **问题**：只认单文件（如 `AGENTS.md`）的 agent 怎么适配？要不要改用户文件？
- **结论**：**不碰用户自己的文件**。Tier-2 agent（当前：Codex、OpenCode）走 **SessionStart hook 注入**——在会话启动时把规则内容注入上下文，而非写入 `AGENTS.md`。
  - **压缩 hook 定位**：属 Tier-2 内的可选手段（用户决策：压缩是另一种方向，仅在此降级路径、确需「本轮刷新」时才用），非主路径。
  - **DeepSeek harness**：移出当前范围（用户决策，后续插件市场解决）。
- **依据**：用户 2026-10-07 决策（单文件不适配丢文件 ⇒ 降级 hooks；不染指用户配置）。
- **待定**：各 Tier-2 agent 的具体 hook 安装与注入实现（实现细节）。

## 六、验证

### 13. 验收自动化 ✅ 方案已定（实现待办）
- **问题**：如何把「效果与 token 和直接写 AGENTS.md 一致」落成自带能力。
- **结论（canary 文件法）**：`rulemux verify` / `doctor` 逐 agent 做：
  1. **钩子先于读规则**：在某 agent 规则目录放一个带 `rulemux` 标记的 canary 文件，开新会话，问 agent「你看到了 canary 里的暗号吗」——能答出即证明 SessionStart 复制在当前会话生效（坐实第 8 条链路）。
  2. **点文件可读**：同样放 `.rulemux__canary.md`（点开头隐藏），验证该 agent 的「读全部 .md」**不跳过点文件**；若跳过 ⇒ 退化为非点前缀 `rulemux__`（见第 7 条待 canary ①）。
  3. **CodeBuddy 结构**：验证平铺 `.rulemux__*.md` 是否被加载（见第 7 条待 canary ②）。
  4. **token 一致性**：对比「rulemux 注入」与「直接手写 AGENTS.md」进上下文的 token 数，应在误差内一致（A1 验收）。
- **关联**：`features/verification.md`。
- **待定**：canary 脚本具体实现。

## 七、工程化

### 14. 测试与冒烟 ✅ 已定（已完成）
- **结论**：用 **Go 标准库 `go test`** + 一个 shell 冒烟脚本，不引第三方测试框架。
- **单测**：`internal/config/config_test.go`（TOML 子集解析 / SourcesFor / Validate）、`internal/engine/sync_test.go`（前缀命名 / 同名去重 / 复制 / 跳过 / 覆盖 / 删残留 / 不碰用户文件 / 缺失源）。
- **冒烟**：`scripts/smoke.sh` —— 端到端 10 项：init 装钩子 + 幂等、sync 落地三目录 + 幂等、不碰用户文件、源变更覆盖、删残留、inject、doctor、verify canary。
- **实测结果**：Go 1.27.1 下 `go build` / `go vet` 全绿，`go test ./...` 通过，冒烟 10/10 通过。
- **待定**：CI（GitHub Actions 交叉编译 + Release）。

### 15. 文档落地 ⬜
- **问题**：实现相关的设计决策回写到 `docs/design/`（按现有体系），不另建。
- **说明**：约定已明确，本条仅作提醒，无需拍板。
