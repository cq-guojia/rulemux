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
| T4 | **各 agent 核实未完成** | Claude Code 已于 2026-10-07 经官方文档核实（hooks + 加载时机，见 `design/external/agent-rules-dirs.md` §三）；CodeBuddy 源码核查中；Trae / Codex / OpenCode / WorkBuddy / DeepSeek 🔴 待补 ⇒ **除已核实项外不得作实现依据** | 其余各家按 `features/verification.md` 跑实测闭环，并补齐适用版本与出处 |
| T5 | ~~删残留方案~~ **已解**：固定前缀 `.rulemux__` 区分「我方 / 用户」+ 删残留算法，无 manifest（[`design/implementation.md`](design/implementation.md) #10/#11） | `features/dir-sync.md` 已改写 | ✅ 已解 |
| T6 | **各 agent hooks 配置落点待查证** | `features/hook-injection.md` 表里 CodeBuddy / Codex / WorkBuddy 三项「待补」，且该表属**外部事实**，应迁入 `design/external/` | 查证后新建 `external/` 文档并标适用版本，原表改为引用 |
| T7 | ~~正式发版前要加 `files` 字段~~ **已解** | ✅ 已发 `rulemux@0.1.0`（非占位，可用）：补了 `bin` 启动器 `bin/rulemux.js`、`files`、`.npmignore`（避开 `.gitignore` 把 `dist/` 排除的坑）、`scripts/build-dist.sh` 交叉编译；二进制经 `-ldflags="-s -w"` 裁剪，5 份共 6.2 MB。已上线为 `latest` | 无需动作 |
| T8 | **实现细节逐条拍板**（作为任务跟进） | [`design/implementation.md`](design/implementation.md) 含待定项 1–15；截至 2026-10-07 已逐条拍定 1–13（仅 #14 测试 / #15 文档落地 留待实现） | ✅ 基本完成，结论已回写 design 文档 |
| T9 | **Go 源码实现 + 编译** | 源码已写完（CLI + 配置 + agent 注册表 + 核心引擎 + 钩子安装 + 4 个子命令，见 [`ops/implementation-workplan.md`](ops/implementation-workplan.md)）；但本机/NAS **无 Go 工具链**，尚未编译与自测 | ✅ **已解**：Go 1.27.1 linux/amd64 已装好；`go build` + `go vet` 全绿，`go test ./...` 通过，`scripts/smoke.sh` 10/10 通过。二进制已装到 `/usr/local/bin/rulemux`，6 个 agent 的 SessionStart 钩子已装（CodeBuddy/WorkBuddy canary ✅ 已坐实；Claude 侧未落地见 T11） |
| T10 | **canary 实测坐实外部事实** | **CodeBuddy / WorkBuddy 已 ✅ 坐实（2026-10-08）**：钩子先于读规则 + 会读 `.rulemux__` 点文件 + `.codebuddy/rules` 平铺生效，代码已标 `Verified=true`。trae / codex / opencode 仍标 `Verified=false`（doctor 会 ⚠ 提示）；Claude 本工作区 canary 未落地（见 T11） | trae / codex / opencode 按 [`design/features/verification.md`](design/features/verification.md) 跑 `rulemux verify`；结论回写 `design/external/agent-rules-dirs.md` §四 |
| T11 | **Claude 侧 sync 未落地（open bug）** | 本工作区那次 Claude 会话里 `.claude/` 仅有 `settings.json`、无 `rules/` 目录，canary 暗号未被读到 ⇒ 同步链路未在该 agent 生效；根因未定（adapter 路径 / hook 触发 / 该工作区未装钩子） | 开一次 Claude Code 会话，查 `rulemux sync --agent claude` 是否真生成 `.claude/rules/.rulemux__*`；结合 `doctor` 与 `external/agent-rules-dirs.md` §四 排查 |
| T12 | **用户 2026-10-08「workspace 数组」提案 = 已实现** | 提案（workspace 数组 / `all`·`*`·省略 / 先算匹配条目→合并文件清单→增删比对）与 `implementation.md` #6 + 代码 `config.go`（`Workspaces []string` / `SourcesFor` / `MatchesWorkspace`）一致；无需新代码。前缀实为 `.rulemux__`（非消息里的「mux--」） | 无需动作；新会话可直接沿用 |
| T14 | ~~跨工作区残留清理（卸载按账本回访）~~ **已解** | 钩子改为 user 级全局后，卸载一次性移除钩子 ⇒ 其它曾同步工作区的 `.rulemux__` 残留再无机会被自动删除（自愈链路断了） | ✅ 已实现：`internal/state/ledger.go` 记录所有曾同步工作区（只增不减，工作区被删也保留以便路径重现后回访）；`sync` 成功即记账，`uninstall` 按账本逐个回访清理并先列清单确认。文档见 `design/features/agent-onboarding.md` §四 |
| T15 | **新 Agent 接入清单已沉淀** | 用户 2026-10-08 要求：把「接一个新 Agent 要做什么」写成文档；并明确当前只管 codebuddy / workbuddy，codex/claude/trae 等届时再谈 | ✅ 已文档化：`design/features/agent-onboarding.md`（核实 → 登记 → 安装 → 删除 → 账本 + 红线），已挂进 `design/features.md` 索引 |
| T13 | ~~hook 落点改成 user 级（不写 workspace 级）~~ **已解** | 2026-10-08 用户拍板：不要 workspace 级 `.codebuddy/settings.json`（散落各工作区、易被误改），改为与 Hindsight 一致的 user 级 host 配置。佐证：Hindsight 实测同时用 MCP server + 3 个 user 级 hook（`external/hindsight.md`） | ✅ 已实现：`registry.go` 的 codebuddy/workbuddy 改为 `~/.codebuddy/settings.json` + `HookAbs:true`；旧 workspace 钩子已卸载并清掉残留文件；本机已按新方式装入 `/root/.codebuddy/settings.json`（与 Hindsight 钩子并存）。结论进 `design/architecture.md` §五 |

---

## 三、下一步

1. **查 Claude 侧 sync 未落地（T11）**：开 Claude Code 会话，确认 `rulemux sync --agent claude` 是否生成 `.claude/rules/.rulemux__*`；用 `doctor` + `external/agent-rules-dirs.md` §四 定位根因（adapter 路径 / hook 触发 / 该工作区未装钩子）。
2. **其余 agent canary（T4）**：按 `design/features/verification.md` 跑 trae / codex / opencode 实测闭环，回写 `external/agent-rules-dirs.md` §四 并标 `Verified` 与适用版本。
3. **发版（W11 / W12）**：`package.json` 加 `files` 字段只发产物（W12，即 T7）✅ **已解**（2026-10-08 发出 `rulemux@0.1.0`）；**W11 仍欠**：GitHub Actions 交叉编译 + Release（届时产物可直接喂给 npm 包，也让用户能不装 Go 就取二进制）。
4. **本批文档落地**：上一轮梳理出的 PROGRESS / HISTORY / workplan / README / external 更新已落盘并提交（本批）。
