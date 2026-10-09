# 各 agent 工作区规则目录（外部事实）

> **类型**：外部事实（上游 agent 侧）
> **适用版本**：见下表「适用版本」列；上游升级后据此复核
> **状态**：🟡 **部分核实** —— Claude Code 经官方文档核实（2026-10-07，见 §三）；CodeBuddy / WorkBuddy 经 canary 实测坐实（2026-10-08，**同日复核并更正**，见 §四）；Trae / Codex / OpenCode 仍待补；**Claude 运行时 canary 未落地（open bug，见 §四）**。**除已核实项外不得作为实现依据**
> **来源**：前期调研（原根 `DESIGN.md` §3）+ 本机 CodeBuddy 安装目录源码核查 + Claude Code 官方文档（`code.claude.com/docs/en/hooks`、`/memory`，2026-10-07）+ CodeBuddy 官方规则文档（`www.codebuddy.ai/docs/zh/ide/User-guide/Rules`，2026-10-08）
> **配套**：[`../features/dir-sync.md`](../features/dir-sync.md)（我方怎么适配）· [`../../PROGRESS.md`](../../PROGRESS.md)（核实任务）

> ⚠️ 本文件只记**上游读取能力**（目录 / 扩展名 / 读不读全部）。
> 「我方据此怎么落文件」是设计，归 [`../features/dir-sync.md`](../features/dir-sync.md)，**不写在这**。

## 一、总表（除 CodeBuddy 外均待核实）

| Agent | 工作区规则目录 | 扩展名 / 结构 | 读目录全部？ | 会话内重读？ | hook stdout 协议 | 适用版本 | 备注 |
|---|---|---|---|---|---|---|---|
| **CodeBuddy** | `.codebuddy/rules/`（canary 坐实，见 §二/§四） | `.md` / `.mdc` 均可 | ⚠️ **只读平铺「非隐藏」`.md`**，且**须带 `alwaysApply:true` frontmatter**；点开头隐藏文件被**跳过** | ❌ **不会**：会话开始即固定规则快照，改动要下一个会话才生效（见 §四） | `hookSpecificOutput{hookEventName:"SessionStart", additionalContext}`（2026-10-09 实证，见下） | 4.12.1（本机） | 「用户级 / 项目级」两类规则；运行时真值 `.codebuddy/rules` 非官方文案 `.rules`；`SessionHint=true` |
| Claude Code | `.claude/rules/` | `.md` | ✅ 全部（官方文档） | 部分：`/compact` 会重读项目 `CLAUDE.md`，但 `.claude/rules/` 子目录**不保证**自动回读 | 同上（`hookSpecificOutput.additionalContext`） | v2.0.64+ | ⚠️ 本工作区 canary 未落地（open bug，见 §四） |
| Trae | `.trae/rules/` | `.mdc` | ✅ 递归读，最多 3 层 | 待补 | 待补 | 待补 | 需 frontmatter |
| WorkBuddy | `.workbuddy/rules/` 🔴（待 canary 实测校准，可能实际为 `.codebuddy/rules`，见 §五） | `.md`（带 alwaysApply:true） | ⚠️ 同 CodeBuddy 读平铺非隐藏 `.md` | ❌ 不会（会话开始固定快照） | 同上 | 5.7.6（本机实测） | **2026-10-09 起拆为独立 adapter**，钩子落到 `~/.workbuddy/settings.json`（不再复用 CodeBuddy 那份） |
| Codex | ❌ 无目录 | 单文件 `AGENTS.md`（沿目录树向上合并，每目录最多一个） | ❌ | — | Tier-2：stdout 注入**规则正文** | 待补 | — |
| OpenCode | ⚠️ 非目录扫描 | `AGENTS.md` + `opencode.json` 显式列 instruction 文件 | ❌ | — | Tier-2：stdout 注入**规则正文** | 待补 | — |
| DeepSeek Harness | — | — | — | — | — | 移出范围 | 用户 2026-10-07 决定移出当前范围（最开放、支持插件，后续以插件市场解决） |

> ⚠️ **扩展名 / 结构各家不同** ⇒ 同步器必须**按 agent 分别落格式**，不能一个 `.md` 通吃。
>
> ⚠️ **「会话内重读？」+「hook stdout 协议」这两列是「是否开启变化提示」的判据**（落地为
> `internal/agents/registry.go` 的 `SessionHint` / `HintProtocol` 字段）。结论：**会话内不会重读 ⇒ 需要提示**；
> 会热重载的 agent 应保持 `SessionHint=false`。当前**没有任何 Tier-1 agent 被坐实热重载** ⇒ 现状都开启提示（待逐个核实）。
> `hookSpecificOutput{SessionStart, additionalContext}` 的实证来源：本机 `~/.hindsight/coding-agents/dist/codebuddy-sessionstart-hook.js:2915-2918, 3940-3945`。

## 二、CodeBuddy 已核实事实（本机 4.12.1）

**核查路径**：`/root/.codebuddy-server-cn/bin/stable-757a5b2f…/extensions/genie/`（打包产物）+ `/root/.local/share/CodeBuddyExtension/`。

### 已推翻的旧结论

| 旧结论（前期调研） | 核查结果 |
|---|---|
| 每条规则 = 子文件夹 + **`RULE.mdc`** | ❌ **证伪**：全盘搜索 `RULE.mdc` **0 命中**（`find / -name "RULE.mdc"` 无结果） |

### 已确认

| 项 | 结论 | 出处 |
|---|---|---|
| 规则文件扩展名 | **`.mdc`**（官方 generate-rules 提示词要求）；实测平铺 **`.md`** 同样被加载 | `product.json:753` + 2026-10-08 canary |
| frontmatter 字段 | **`alwaysApply`（bool）+ `description`（string）**；示例 `---\nalwaysApply: true\n---`；**实测：会话开始自动加载须 `alwaysApply: true`** | `product.json:753` + 2026-10-08 canary |
| `globs` 字段 | ❌ 无据（本机 0 处出现，勿当真） | — |
| 总开关 | `enableWorkspaceRules`（自动读取） | `package.json` / `package.nls.json:64` |
| 规则层级 | 存在**用户级** + **项目级**两类（4.0.0 起） | `CHANGELOG.md:441`、`l10n/bundle.l10n.en.json:145-146` |
| 工作区配置根 | `.codebuddy/`（技能落 `.codebuddy/skills/`，与规则目录是两套东西） | `product.json:926-932` |

### hook 执行语义（2026-10-08 实测坐实）

| 项 | 结论 | 出处 |
|---|---|---|
| hook 命令取哪个字段 | **只执行 `command` 字段（整串命令行），完全丢弃 `args` 字段** | 本机日志 `~/.local/share/CodeBuddyExtension/Logs/CodeBuddyIDE/2026-10-08/rulemux__*.log:93968`：`[HookExecutor] Executing hook command: rulemux`（配置为 `command:"rulemux"` + `args:["sync","--agent","codebuddy"]`，实际只跑了裸 `rulemux`） |
| 对照（正常样例） | Hindsight 的钩子把参数写进 command 整串（`node "<abs>.js"`），被执行原样 | 同目录 Hindsight 日志行 / `hindsight.md` §2.2 |
| 适用版本 | CodeBuddy 4.12.1、WorkBuddy 5.7.6（均本机；两者的钩子均只执行 `command` 整串、丢弃 `args`，执行语义一致，但**配置目录各自独立**——见 §五） | — |

> **硬约定**：钩子的 `command` 必须写成**完整命令行字符串**（参数全部写在里面），**不得依赖 `args` 字段** —— 否则会被执行成裸程序名（无参数、只打印帮助），同步从不发生。我们的适配器（`internal/hooks/install.go`）据此把 `rulemux <subcmd> --agent <id>` 写进 `command` 整串。

### 仍未确定

1. ~~**目录名冲突**~~ → **已解**：运行时真值是 **`.codebuddy/rules`**（canary 坐实，非官方文案 `.rules`）。
2. 是否递归扫描子目录、有无层数 / 大小 / 数量上限（实测平铺有效，子目录 `RULE.mdc` 加载不稳定，不依赖）。
3. 加载时机（打开工作区 / 重载窗口 / 每轮）与改动后是否热更新。
4. ~~缺 frontmatter 时的降级行为~~ → **已解**：无 `alwaysApply:true` 的规则**不会**在会话开始自动加载（见 §四）。
5. 用户级规则的落盘位置与「合并 or 覆盖」策略。

> 官方文档称「每条规则 = 一个含 `RULE.mdc` 的子目录」（`www.codebuddy.ai/docs/zh/ide/User-guide/Rules`，2026-10-08 抓取），但**本机实测与该文案不符**：平铺 `RULE.mdc` 子目录的加载**不稳定**，平铺非隐藏 `.md`（带 `alwaysApply:true`）才是稳定生效的形态。**以实测为准。**

### 一条容易误判的线索

`.codebuddy/rules/tcb/rules/<规则名>/rule.md` —— 这是 **CloudBase（tcb）技能自己的私有约定**（写在它的提示词里让模型去读），**不是 CodeBuddy 的通用规则机制**。别拿它当规范。

---

## 三、Claude Code 已核实事实（官方文档 2026-10-07）

> **来源**：Claude Code 官方文档 `https://code.claude.com/docs/en/hooks` 与 `https://code.claude.com/docs/en/memory`（抓取于 2026-10-07）
> **适用版本**：文档未标注具体版本号；按文档当前内容记录，上游升级后复核

### 3.1 hook 能执行我们的二进制（命门 a，已解）
- SessionStart 等会话级事件支持 `type: "command"`，即「run a shell command」。
- 两种写法：
  - Shell 形式：`"command": "rulemux sync"` → 交给 `sh -c`（mac/linux）执行。
  - **Exec 形式**：`"command": "rulemux", "args": ["sync"]` → `rulemux` 在 PATH 上解析为可执行文件，**不经 shell 直接 spawn**。
- ⇒ 编译型单二进制（Go/Rust 的 `rulemux`）完全可被 hook 调起，无需 Node/VM。
- ⚠️ Windows：exec 形式要求 `command` 是真实可执行文件（`.exe`）；npm 的 `.cmd`/`.bat` 垫片不行，须走 shell 形式或直接给 `.exe`。我们发 `rulemux.exe` 即可。
- 相关事件齐全：`SessionStart` / `PreCompact` / `PostCompact` / `InstructionsLoaded` / `FileChanged` / `ConfigChange` 等 30+ 种。

### 3.2 规则加载时机（命门 b，已解 + 已知边界）
- `.claude/rules/` 中**无 `paths` frontmatter** 的规则，在**会话开始**一次性读入上下文（优先级同 `.claude/CLAUDE.md`）；项目根 `CLAUDE.md`、用户级 `~/.claude/CLAUDE.md` 同理。
- 每个会话从全新上下文开始；读入后留在上下文，**不会每轮重读磁盘**；发生 `/compact` 时项目根 `CLAUDE.md` **会重新从磁盘读取并注入**，子目录/路径规则之后按需重载。
- **对 rulemux 的含义**：
  - 文件**持久化**在原生规则目录 ⇒ 每个新会话（及每次 `/compact` 后）必然加载 ⇒ 满足 Tier-1 / A1–A3（不淡出、不累积、token 恒定）。
  - SessionStart hook 当轮写入「会话开始即加载」位置时，**通常下个会话才生效**（加载已先发生于 hook 之前，时序官方未文档化）→ 但文件已落盘，下轮必读，无功能缺失。
  - 同会话即时新鲜度（改了中央规则想本轮就见）非保证；以 `PreCompact` 再 copy 一次兜底，或重开会话即可。
- **验证手段**：官方提供 `InstructionsLoaded` hook（可观察哪些规则被加载）与 `/context`（查看已加载 Memory）。→ 这正好是我们 `features/verification.md` 的 canary 实测法要落地的对象，**不当场猜，实测为准**。

---

## 四、canary 验收清单与结果

> 见 [`../implementation.md`](../implementation.md) #7 / #13。

### 已坐实（2026-10-08，CodeBuddy 对照 canary 实测）

在 `/code/open-lab/rulemux` 工作区同时放入三个对照探针，各开一个新会话询问水印，得到明确对照：

| 探针（位于 `.codebuddy/rules/`） | 结构 | 被加载？ |
|---|---|---|
| `rulemux__canary_flat.md` | 平铺、**非隐藏**、带 `alwaysApply:true` | ✅ **两次均加载** |
| `.rulemux__canary_hidden.md` | 平铺、**点开头隐藏**、带 `alwaysApply:true` | ❌ **未加载** |
| `rulemux__canary_dir/RULE.mdc` | 子目录 + `RULE.mdc` | ⚠️ 一次加载、一次未加载（**不稳定，不依赖**） |

由此坐实（并**更正**本节旧结论）：

1. **点文件被跳过** ❌ —— CodeBuddy **不读** `.rulemux__` 点开头的隐藏文件 ⇒ 落盘前缀必须改为**非隐藏**的 `__rulemux__`。
2. **须带 frontmatter** ✅ —— 平铺非隐藏 `.md` 只有带 `alwaysApply:true` 才会在会话开始**自动加载**（与 §二 frontmatter 字段一致）。
3. **目录结构** —— 运行时真值是 `.codebuddy/rules`（非官方文案 `.rules`）；平铺即可，**无需** `RULE.mdc` 子目录（其加载不稳定）。
4. ~~**钩子先于读规则**~~ ❌ **已推翻（2026-10-08 首证，2026-10-09 对照实测复证）** —— 实际是**读规则先于钩子写入**：宿主在会话开始瞬间固定规则快照，SessionStart 的写入发生在其后 ⇒ **本次会话读不到刚投递 / 刚变更的规则，要下一个会话才生效**。
   实证（本机 CodeBuddy 4.12.1，工作区 `/code/open-lab/rulemux`）：先删掉 `.codebuddy/rules/__rulemux__*.md`，再**开一个全新会话**，在该会话内问两个问题（明确禁止调用工具、只凭上下文回答）→ 结果 **「上下文里有没有该规则内容」= 无；「有没有收到『规则有变化，请开新会话』提示」= 有**。钩子确实跑过并把文件重新投递（否则不会检测到「新增」而输出提示），但该会话读不到 ⇒ **差一拍成立**，且「变化提示」（`SessionHint`）是此机制下唯一正确的兜底。
   预期：**再下一个会话**应为「能抄出规则标题 + 无提示」（无变更即不提示）。
5. **WorkBuddy** ✅ —— 见 §五：本机 2026-10-09 实测其用户级配置为**独立**目录 `~/.workbuddy`（非 `~/.codebuddy`），故 2026-10-09 起已拆为独立 registry adapter；其工作区级 rules 目录（`.workbuddy/rules` vs `.codebuddy/rules`）仍待 canary 校准（🔴）。**此结论推翻了此前「WorkBuddy 复用 CodeBuddy 同一目录与机制」的推断。**

> **更正说明**：本节此前的旧结论（"点文件会被读 ⇒ 保留 `.rulemux__`"，据称 2026-10-08 canary 念出 `RULEMUX-CANARY-43371345`）属**假阳性**——当时只验证了「文件被 copy 进目录」，**未用对照探针区分「隐藏 vs 非隐藏」**，把 copy 成功误当成加载成功。2026-10-08 用三探针对照后更正。**教训：canary 必须用对照探针，且以「内容是否进入本会话上下文」为判据，而非「文件是否落盘」。**

### 仍待实测

- **Trae**：`.trae/rules` 是否被读、钩子配置（`hooks.json`）落点是否正确。
- **Claude Code**：目录已核实，但本工作区那次会话里 `.claude/` 只有 `settings.json`、没有 `rules/` 目录，需开一次 Claude Code 会话确认同步真的发生。
- **Codex / OpenCode**：Tier-2 注入落点与 schema（`~/.codex/config.toml` 的 `[features] codex_hooks` + `[[hooks.SessionStart]]`）。

> ⚠️ 未坐实的项仍**不得作为实现依据**（代码里 `Verified=false`，`rulemux doctor` 会标 ⚠）。

### 历史坑（2026-10-08 已通过改落点解决）

> ⚠️ 本节记录的是**旧行为**，2026-10-08 起钩子改落到 **user 级 host 配置** `~/.codebuddy/settings.json`，已不再按工作区安装（见 [`../architecture.md`](../architecture.md) §五 与 [`hindsight.md`](hindsight.md)）。保留此节以备对照：

旧规则：`rulemux init` 只给**当前工作区**装钩子。另一个工作区若从没跑过 init，会话启动时根本不会触发 sync ⇒ 读不到任何规则
（曾出现「A 工作区能读到暗号、B 工作区读不到」的现象，原因就是 B 没装钩子，不是配置或重装问题）。

**现况：钩子装在 user 级一次即全局生效，无需在每个工作区跑 init。**

---

## 五、配置目录解析（路径怎么定位）

> **来源**：本机 CodeBuddy **4.12.1** 安装产物直接取证（2026-10-09）。
> 取证文件：`/root/.codebuddy-server-cn/bin/stable-757a5b2fd56bc7e1fcabb38b58d2ac1694f78f6d/extensions/genie/out/extension/index.js`（打包为压缩单行 JS，下面只做格式化摘录）。
> **适用版本**：CodeBuddy 4.12.1（本机）；WorkBuddy 5.7.6（本机，独立适配，见 §5.4）。

### 5.1 已坐实：用户级配置目录可由环境变量覆盖

产物中该段可逐字读出：

```js
CODEBUDDY_CONFIG_DIR_ENV = "CODEBUDDY_CONFIG_DIR";
function getCodeBuddyHomeDir() {
  const ir = process.env[CODEBUDDY_CONFIG_DIR_ENV];
  return ir && "" !== ir.trim() ? ir : path.join(os.homedir(), USER_DATA_DIR_NAME);
}
function getCodeBuddyProjectsDir() { return path.join(getCodeBuddyHomeDir(), "projects"); }
// USER_DATA_DIR_NAME = ".codebuddy"
```

| 项 | 结论 |
|---|---|
| 用户级配置目录 | `$CODEBUDDY_CONFIG_DIR`（去空白后非空时）**否则** `os.homedir()/.codebuddy` |
| 我们的钩子文件 | 上述目录 + `/settings.json`（canary 已坐实 `~/.codebuddy/settings.json` 生效，见 §四） |
| 是否走 XDG | ❌ **无** XDG 分支：产物是「`.codebuddy` 字面量 + `homedir()`」拼接，未见 `XDG_CONFIG_HOME` |
| Windows / macOS 差异 | **未发现**配置目录层面的平台分支；Windows 由 `os.homedir()`（`%USERPROFILE%`）覆盖 |
| 与安装目录的关系 | `~/.codebuddy` = **用户级配置/数据**（`settings.json`、`mcp.json`、`plugins/`、`skills/`…）；`~/.codebuddy-server-cn` = **程序主体**（VS Code 远程 server，`product.json` 的 `serverApplicationName`/`serverDataFolderName`，内含 `bin/`、`extensions/`、`data/`）。rulemux **从不写安装目录** |

### 5.2 未坐实，故**不采用**：`WORKBUDDY_CONFIG_DIR`

同名变量在产物里出现于 `extensions/genie/out/vendor/shim/node-safe-delete-shim.cjs:286-290`，
但它与 `CODEBUDDY_CONFIG_DIR` 并列，仅用于拼 **npm 缓存日志**白名单
（`<configDir>/binaries/node/cli-connector-cache/_logs`）。
**没有任何证据表明它决定 `settings.json` 的位置** ⇒ rulemux **不采用**，留待 WorkBuddy 侧的
canary 或官方文档坐实（记在 `PROGRESS.md` 待办里）。

### 5.3 rulemux 据此怎么落地

- `internal/agents/registry.go` 的 `codebuddy` 条目声明
  `HookDirEnv: "CODEBUDDY_CONFIG_DIR"` + `HookFileBase: "settings.json"`；
  `HookFileAbsWithSource()` 的顺序是 **环境变量（非空）> `~` 展开的字面量**。
- `doctor` 与 `init --refresh` 在环境变量生效时都会标出 `[dir from $CODEBUDDY_CONFIG_DIR]`，
  用户能一眼看出钩子写到了哪。若该变量出现过、但用户并不真的使用它，可按它给出的路径去核对。
- **只填「已核实」的变量**：未核实的 agent（claude / trae / codex / opencode）该字段留空，
  一律按 `~` 或工作区字面量走 —— 这一层**绝不猜**。

### 5.4 WorkBuddy 是**独立**配置目录（2026-10-09 本机实测，已核实）

> **来源**：本机实测（`~/.workbuddy/settings.json` 实测为 WorkBuddy 的用户级钩子配置；
> WorkBuddy 为独立 macOS Electron 应用 `/Applications/WorkBuddy.app/`，不与 CodeBuddy 共享
> `~/.codebuddy`）。
> **适用版本**：WorkBuddy 5.7.6（本机）；**状态**：✅ 已核实（本机 settings.json 实测）。
> **配套**：[`../../PROGRESS.md`](../../PROGRESS.md)。

| 项 | 结论 |
|---|---|
| 用户级配置目录 | `~/.workbuddy`（**不是** `~/.codebuddy`） |
| 钩子配置文件 | `~/.workbuddy/settings.json`（与 CodeBuddy 那份**互相独立**，WorkBuddy 不读 CodeBuddy 的） |
| `WORKBUDDY_CONFIG_DIR` | 见 §5.2：仅在 safe-delete 日志白名单出现，未坐实为配置目录 ⇒ **不采用**；registry 中 `HookDirEnv` 留空，按 `~/.workbuddy` 字面量 |
| 工作区级 rules 目录 | `.workbuddy/rules` 🔴 **待 canary 实测校准**（可能实为 `.codebuddy/rules`，待后续验证） |

**对 rulemux 的影响**：

- `internal/agents/registry.go` 的 `workbuddy` 是**独立条目**（`HookFile: "~/.workbuddy/settings.json"`、`HookAbs: true`），
  不再作为 `codebuddy` 的别名收纳。此前的「方案 A」（别名合并）被本实测推翻：别名方案会把钩子写进
  `~/.codebuddy/settings.json`（CodeBuddy 那份），WorkBuddy 根本不读 ⇒ 钩子永不触发。
- `doctor` / `init --refresh` 对 `workbuddy` 比对的是 `~/.workbuddy/settings.json`，与 `codebuddy` 互不干扰。
- 历史遗留：若曾在「别名时代」用 `init --agent workbuddy` 安装，钩子被写进了 `~/.codebuddy/settings.json`
  （CodeBuddy 那份、WorkBuddy 不读），属孤儿，无需迁移；重装（拆后）会写进正确的 `~/.workbuddy/settings.json`。
