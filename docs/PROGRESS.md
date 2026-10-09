# 进度（进行中）

> **这是什么**：本仓库**唯一**的在办事项真源 —— 现在做到哪、欠什么、下一步做什么。
> **已结案的**看 [`PROGRESS-HISTORY.md`](PROGRESS-HISTORY.md)（一行一条：时间 / 完成了什么 / 过程文档）。
> **文档规矩**看 [`README.md`](README.md)。
>
> 本文件**只装在办的事**；事项结案即从本文件移入 `PROGRESS-HISTORY.md`。
> ⚠️ README / issue / 聊天记录都不是真源。

---

## 一、当前状态

### 1.1 需求与方案定型 —— ✅ **已拍板**（2026-10-07 逐条锁定，文档对齐进行中）

> 定型层文档已于 2026-10-07 逐项拍板（结论进 [`design/implementation.md`](design/implementation.md)）：
> [`design/requirements.md`](design/requirements.md)、[`design/architecture.md`](design/architecture.md)、[`design/features.md`](design/features.md) + `features/` 三份、[`design/external/agent-rules-dirs.md`](design/external/agent-rules-dirs.md)（Claude Code 已核实；CodeBuddy/Trae/Codex/OpenCode 待 canary）现已对齐新模型。

---

## 二、未决项

| # | 问题 | 现状与影响 | 将来怎么解（方向，未定） |
|---|---|---|---|
| T0 | ~~需求待拍板项：真源位置~~ **已解**：无中央真源，用户在 TOML 配置列出任意源文件（[`design/implementation.md`](design/implementation.md) #4/#5/#6） | `requirements.md` §二 已改写为「配置所列源文件」 | ✅ 已解 |
| T1 | **Trae / CodeBuddy `.mdc` 的 frontmatter 怎么写才无条件常驻** | `.mdc` 带 frontmatter（`alwaysApply` / `globs` / `description`）。写法不对 ⇒ 规则只在命中 glob 时生效，达不到「等价 `AGENTS.md`」 | 逐家查证 frontmatter 语义（要读到实现，不猜），结论回写 `external/agent-rules-dirs.md` |
| T2 | ~~Codex/OpenCode 分支处理~~ **已解（Tier-2）**：单文件 agent 走 SessionStart hook 注入，不碰用户 `AGENTS.md`，取消「受管区拼接」方案（[`design/implementation.md`](design/implementation.md) #9/#12） | `features/hook-injection.md` 已改写 | ✅ 已解 |
| T3 | **DeepSeek Harness 的规则目录未确认** | 目录 / 扩展名 / 是否读目录全部均未知，阻塞该 agent 接入 | 查证后补进 `external/agent-rules-dirs.md` |
| T4 | **各 agent 核实未完成** | Claude Code 已于 2026-10-07 经官方文档核实（hooks + 加载时机，见 `design/external/agent-rules-dirs.md` §三）；CodeBuddy、WorkBuddy 已坐实（§5.4）；Trae / Codex / OpenCode / DeepSeek 🔴 待补 ⇒ **除已核实项外不得作实现依据** | 其余各家按 `features/verification.md` 跑实测闭环，并补齐适用版本与出处 |
| T5 | ~~删残留方案~~ **已解**：固定前缀 `__rulemux__` 区分「我方 / 用户」+ 删残留算法，无 manifest（[`design/implementation.md`](design/implementation.md) #10/#11） | `features/dir-sync.md` 已改写 | ✅ 已解 |
| T6 | **各 agent hooks 配置落点待查证** | CodeBuddy / WorkBuddy 已查证并迁入 `external/agent-rules-dirs.md`（§二 / §5.4）；**Trae（`hooks.json` 落点与 schema）**、Codex（`~/.codex/config.toml`）仍待补 | Trae 随本轮接入一并查证；Codex 接入时再查。结论回写 `external/` |
| T7 | ~~正式发版前要加 `files` 字段~~ **已解** | ✅ 已发 `rulemux@0.1.0`（非占位，可用）：补了 `bin` 启动器 `bin/rulemux.js`、`files`、`.npmignore`（避开 `.gitignore` 把 `dist/` 排除的坑）、`scripts/build-dist.sh` 交叉编译；二进制经 `-ldflags="-s -w"` 裁剪，5 份共 6.2 MB。已上线为 `latest` | 无需动作 |
| T8 | **实现细节逐条拍板**（作为任务跟进） | [`design/implementation.md`](design/implementation.md) 含待定项 1–15；截至 2026-10-07 已逐条拍定 1–13（仅 #14 测试 / #15 文档落地 留待实现） | ✅ 基本完成，结论已回写 design 文档 |
| T9 | **Go 源码实现 + 编译** | 源码已写完（CLI + 配置 + agent 注册表 + 核心引擎 + 钩子安装 + 4 个子命令，见 [`ops/implementation-workplan.md`](ops/implementation-workplan.md)）；但本机/NAS **无 Go 工具链**，尚未编译与自测 | ✅ **已解**：Go 1.27.1 linux/amd64 已装好；`go build` + `go vet` 全绿，`go test ./...` 通过，`scripts/smoke.sh` 10/10 通过。二进制已装到 `/usr/local/bin/rulemux`，6 个 agent 的 SessionStart 钩子已装（CodeBuddy/WorkBuddy canary ✅ 已坐实；Claude 侧未落地见 T11） |
| T10 | **canary 实测坐实外部事实** | **CodeBuddy 已 ✅ 坐实（2026-10-08，同日对照复测并更正）**：`.codebuddy/rules` 平铺**非隐藏** `.md` 且带 `alwaysApply:true` 才会在会话开始被自动加载；点开头隐藏文件**被跳过**；「钩子先于读规则」是**假阳性**，实际是**读规则先于 hook 写入**（⇒ 新增文件首会话差一拍）。**WorkBuddy 已 ✅ 坐实 + 本机验收通过（2026-10-09，`v0.2.7`）**：**用户级**配置独立（`~/.workbuddy/settings.json`），**工作区级**规则目录经三位置对照探针坐实为 **`.codebuddy/rules`（与 CodeBuddy 共享）** ⇒ 同步取并集、卸载走收敛（`features/dir-sync.md`「共享规则目录」§ + `external/agent-rules-dirs.md` §5.4）。⚠ 教训：「用户级独立」≠「工作区级独立」，曾据此误推落点。**trae 已 ✅ 坐实并随 `v0.3.0` 发版（2026-10-09 canary）**：用户级 hook 落点 `~/.trae-cn/hooks.json`（CN）/ `~/.trae`（intl），规则目录 `.trae/rules` 递归读取、`.md` 加载（frontmatter 可选）、`.mdc` 不加载，hook 载荷带 `cwd`/`workspace_roots`；结论见 `external/agent-rules-dirs.md` §二·续。codex / opencode 仍标 `Verified=false`（doctor ⚠）；Claude 本工作区 canary 未落地（见 T11） | codex / opencode 按 [`design/features/verification.md`](design/features/verification.md) 跑 `rulemux verify`；结论回写 `design/external/agent-rules-dirs.md` §四 |
| T11 | **Claude 侧 sync 未落地（open bug）** | 本工作区那次 Claude 会话里 `.claude/` 仅有 `settings.json`、无 `rules/` 目录，canary 暗号未被读到 ⇒ 同步链路未在该 agent 生效；根因未定（adapter 路径 / hook 触发 / 该工作区未装钩子） | 开一次 Claude Code 会话，查 `rulemux sync --agent claude` 是否真生成 `.claude/rules/__rulemux__*`；结合 `doctor` 与 `external/agent-rules-dirs.md` §四 排查 |
| T12 | **用户 2026-10-08「workspace 数组」提案 = 已实现** | 提案（workspace 数组 / `all`·`*`·省略 / 先算匹配条目→合并文件清单→增删比对）与 `implementation.md` #6 + 代码 `config.go`（`Workspaces []string` / `SourcesFor` / `MatchesWorkspace`）一致；无需新代码。前缀实为 `__rulemux__`（非消息里的「mux--」） | 无需动作；新会话可直接沿用 |
| T14 | ~~跨工作区残留清理（卸载按账本回访）~~ **已解** | 钩子改为 user 级全局后，卸载一次性移除钩子 ⇒ 其它曾同步工作区的 `__rulemux__` 残留再无机会被自动删除（自愈链路断了） | ✅ 已实现：`internal/state/ledger.go` 记录所有曾同步工作区（只增不减，工作区被删也保留以便路径重现后回访）；`sync` 成功即记账，`uninstall` 按账本逐个回访清理并先列清单确认。文档见 `design/features/agent-onboarding.md` §四 |
| T15 | **新 Agent 接入清单已沉淀** | 用户 2026-10-08 要求：把「接一个新 Agent 要做什么」写成文档；并明确当前只管 codebuddy / workbuddy，codex/claude/trae 等届时再谈 | ✅ 已文档化：`design/features/agent-onboarding.md`（核实 → 登记 → 安装 → 删除 → 账本 + 红线），已挂进 `design/features.md` 索引 |
| T13 | ~~hook 落点改成 user 级（不写 workspace 级）~~ **已解** | 2026-10-08 用户拍板：不要 workspace 级 `.codebuddy/settings.json`（散落各工作区、易被误改），改为与 Hindsight 一致的 user 级 host 配置。佐证：Hindsight 实测同时用 MCP server + 3 个 user 级 hook（`external/hindsight.md`） | ✅ 已实现：`registry.go` 的 codebuddy 改为 `~/.codebuddy/settings.json`、workbuddy 改为独立的 `~/.workbuddy/settings.json`（均 `HookAbs:true`）；旧 workspace 钩子已卸载并清掉残留文件；本机已按新方式装入 `/root/.codebuddy/settings.json`（与 Hindsight 钩子并存）。结论进 `design/architecture.md` §五 |

---

## 三、下一步

**本轮目标：接入 Trae CN（把它从 `Verified=false` 推到「可安装」）。**
**范围**：本轮**只做 Trae CN**；国际版暂不匹配 —— 将来接入时再判定「两者是同一机制、还是并作别名、还是两条独立条目」。
核实一律按 [`design/features/agent-onboarding.md`](design/features/agent-onboarding.md) 的清单来（先读源码与文档，禁止靠运行时试探猜）：

1. **核实外部事实（阻塞项，先做完再动手）**：
   - 工作区规则目录是否 `.trae/rules`、扩展名 `.mdc` 还是 `.md`、是否读目录下全部 / 递归几层；
   - **T1：`.mdc` 的 frontmatter 怎么写才无条件常驻** —— `alwaysApply` / `globs` / `description` 三者的语义要读到实现（写法不对 ⇒ 规则只在命中 glob 时生效，达不到「等价 `AGENTS.md`」）；
   - **T6：hooks 配置的落点与 schema** —— `hooks.json` 在工作区根还是 `.trae/hooks.json`，是否支持 `type:"command"` 直接 spawn 二进制。
   - 结论回写 [`design/external/agent-rules-dirs.md`](design/external/agent-rules-dirs.md)，每条标适用版本与出处。
2. **对照探针 canary（T4）**：按 [`design/features/verification.md`](design/features/verification.md) 用**对照探针**
   （同内容探针分放候选位置 → 开新会话问「读到了吗」），以「内容是否进入上下文」为判据，不以「文件是否落盘」为准。
3. **代码侧**：按核实结果填 `registry.go` 的 trae 条目（`RulesDir` / `NeedsFrontmatter` / frontmatter 写法 /
   `SessionHint` / `HintProtocol`），并确认 hooks 落点；坐实后 `Verified=true`，即可 `init --agent trae`。
   ⚠ 若 Trae 的规则目录与 codebuddy/workbuddy 相同，走 `SharingRulesDir` 自动分组，不必另写逻辑
   （见 [`features/dir-sync.md`](features/dir-sync.md)「共享规则目录」）。

**其余（非本轮）**：

- T11 **Claude 侧 sync 未落地**（open bug）：开 Claude Code 会话确认 `rulemux sync --agent claude` 是否生成 `.claude/rules/__rulemux__*`。
- **Codex / OpenCode** 接入（Tier-2 注入，落点与 schema 待查）。
- **给仓库配 `NPM_TOKEN` secret**（Settings → Secrets → Actions，Automation 类型 token）：否则 CI 的 npm 步骤按设计跳过，每次发版只能手动 `npm publish`（0.2.7 就是手动发的）。
- `WORKBUDDY_CONFIG_DIR` 是否决定用户级配置目录：仍未坐实（产物里只在 safe-delete 日志白名单出现）；坐实后再考虑加入注册表 `HookDirEnv`。
