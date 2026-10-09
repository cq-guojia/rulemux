# rulemux 架构：为什么这么设计

> **状态**：✅ 已定（2026-10-07 拍板，命名与 Tier 分层已定）
> **来源**：用户前期调研（原根 `DESIGN.md` §1 / §7，已分解迁入本文件）
> **配套**：[`requirements.md`](requirements.md)（用户要什么）· [`features.md`](features.md)（怎么做）· [`external/agent-rules-dirs.md`](external/agent-rules-dirs.md)（各家规则目录）· [`implementation.md`](implementation.md)（实现待定项清单）

---

## 一、核心原理：为什么 Tier-1 必须走 harness 原生规则目录

每次 API 请求的结构是 `system`（静态前缀）+ `messages`（对话）。

1. **Prompt caching 靠前缀匹配**：`system` 的静态前缀被冻结以保缓存 ⇒ 任何每轮变化的内容都会毁掉缓存。
2. **hook 注入落在动态区**：`additionalContext` / `system-reminder` 属动态区——
   - `SessionStart` 注一次 ⇒ 随对话老化、压缩时被摘要 ⇒ **淡出**；
   - 每轮注（`UserPromptSubmit`）⇒ 追加累积 ⇒ **token 爆炸**；
   - **无法写入冻结前缀**（这是缓存 + 信任模型的设计，不是能力缺陷）。
3. **结论**：要「和 `AGENTS.md` 一模一样」（永不淡出 / 不累积 / 恒定 token）＝ **必须走 harness 原生加载的规则文件 / 目录**（即静态前缀语义）。

> ⚠️ 由此定下铁律（Tier-1）：**hook 只当「投递员」，不负责注入。** 验收判据见 [`requirements.md`](requirements.md) §三（A1–A3）。
>
> **已登记的唯一例外（2026-10-09）**：Tier-1 在 SessionStart 后若检测到规则**有变化**（新增 / 修改 / 删除任一），会经 hook 输出**恰好一条瞬态提示**（"规则有变化，请新开会话生效"）。
> 它**不含规则正文**、**只发一次**、**无变化则零输出**（stdout 字节级为空），属"一次性、可过期"的元信息，**不承担 A1–A3 的规则投递职责**（规则投递仍 100% 走原生目录）。
> 详见 [`features/sync-all-and-change-notice.md`](features/sync-all-and-change-notice.md) §六。

## 二、方案选型与 Tier 分层

| 方案 | 定位 | 适用 agent | 取舍理由 |
|---|---|---|---|
| **目录同步（Tier-1）** | 主选 | Claude Code / CodeBuddy / Trae / WorkBuddy（读整个规则文件夹） | 走 harness 原生加载 ⇒ 满足 A1–A3（等价 / 不累积 / 永不淡出）；无软链；加删自由。代价：按各家格式分别落文件 |
| **hook 注入（Tier-2）** | **降级，仅单文件 agent** | Codex / OpenCode（只认单文件 `AGENTS.md`，无目录模式） | 落在动态区 ⇒ 做不到 A3；但单文件 agent 无目录可丢，**只能**走 SessionStart 注入且不碰用户 `AGENTS.md`。启用即视为该 agent 降级、未达 A3 |

> 两个方案的具体做法分别在 [`features/dir-sync.md`](features/dir-sync.md)、[`features/hook-injection.md`](features/hook-injection.md)；本文件只记**为什么这么选**。

## 三、命名

**`rulemux`**（已定）：mux = 多路复用，一份源 → 多 agent；短且有代表性。

候选（备查）：`rulecast` / `rulehub` / `ruleseed` / `ruleflow` 等。

> npm 是否占用属**外部事实**且会变化 ⇒ 发布前重新核实，记录日期与来源（见 [`../PROGRESS.md`](../PROGRESS.md) 未决项）。

## 四、实现结构（核心引擎 + 每 agent 适配器）

架构在代码里的落法，就是「**统一抽象方法 + 每 agent 单独定义 + 调用统一方法**」三层：

| 层 | 代码位置 | 职责 |
|---|---|---|
| **统一方法（核心引擎）** | `internal/engine/` | 文件怎么命名（前缀 `__rulemux__`）、怎么比对内容、怎么覆盖/跳过、怎么删残留、Tier-2 怎么渲染注入 —— **只有一份实现** |
| **每 agent 单独定义（适配器）** | `internal/agents/registry.go` | 一张注册表声明每个 agent 的：Tier、规则目录、钩子落点、是否已核实 |
| **调用（子命令）** | `internal/cmd/` | 查适配器拿到「落到哪」→ 调引擎的统一方法 |

```
各 agent 的 SessionStart 钩子
   → rulemux sync/inject --agent X
       → agents.Get(X)（适配器：目录/注入）
       → engine.Sync / engine.RenderInject（统一方法）
```

> 详细代码结构、任务拆解与优先级见 [`../ops/implementation-workplan.md`](../ops/implementation-workplan.md)。
> 适配器是**一个二进制内的多个适配模块**（不是每个 agent 一个独立程序），分发仍是单文件。

## 五、hook 落点决策：Tier-1 保持 workspace 级（不挪 user 级）

> **状态**：✅ 已拍板（2026-10-08）
> **配套**：[`external/hindsight.md`](external/hindsight.md)（Hindsight 实测机制，作为对照）

**结论**：Tier-1 agent 的 SessionStart 钩子**装进各自的 user 级 host 配置**：codebuddy → `~/.codebuddy/settings.json`、workbuddy → **独立的** `~/.workbuddy/settings.json`（2026-10-09 本机实测：WorkBuddy 不读 CodeBuddy 那份，见 `external/agent-rules-dirs.md` §5.4），**不再写每个工作区的 `.codebuddy/settings.json`** —— 与 Hindsight 的做法一致。

**依据（用户拍板，2026-10-08）**：「把 host 的设置方式改成和 hindsight 一样」——不要 workspace 级那份（`/code/open-lab/rulemux/.codebuddy/settings.json`），因为散落在各工作区、容易被误改。

**怎么用的**：钩子是在 user 级、全局生效 ⇒ 打开任意工作区都会触发一次 `rulemux sync --agent codebuddy`；rulemux 以 cwd 作为当前工作区（未传 `--workspace` 时 `workspace()` 返回 `os.Getwd()`），再去匹配配置里 `workspace` 条目决定投递哪些规则 ⇒ 一次安装、按工作区各自生效。

**对照 Hindsight**：Hindsight 把同款 hooks（SessionStart / UserPromptSubmit / Stop）+ MCP server 全注册在 **user 级** `~/.codebuddy/settings.json` 与 `~/.codebuddy/mcp.json`（见 `external/hindsight.md`）。rulemux 故意**不**学它，理由：

- workspace 级 ⇒ 配置随仓库走（clone 即得 hooks），各项目天然按工作区隔离；
- codebuddy 已坐实 `.codebuddy/rules` 在打开该工作区时由 SessionStart 钩子先于规则加载触发（`external/agent-rules-dirs.md` §四），cwd 即该工作区根，`rulemux sync` 由此自匹配；
- user 级会全局触发、需运行时判别工作区，反而更脆。

**既有能力**：注册表 `Agent.HookAbs` 已支持 user 级落点（`codex` 即 `HookAbs:true` + `~/.codex/config.toml`），故若日后需要可一键切换；**当前默认即 user 级**（2026-10-08 已切换）。
