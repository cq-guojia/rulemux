# 20261013-2349-未安装-已卸载-agent-仍被同步-dsh-规则目录定位待拍板-11fd：未安装/已卸载 agent 仍被同步 + dsh 规则目录定位待拍板

> **✅ 完成封卷**：2026-10-14 17:33:27（@GuoJia-Agent）
> **创建**：2026-10-13 23:49:26（@GuoJia-Agent）

## 背景

用户本机只装了 WorkBuddy，未装/未初始化 dsh（DeepSeek Harness），2026-10-13 执行同步命令时在 `Temp` 工作区发现 `.dsh/rules/` 下仍有 rulemux 同步产物：`__rulemux__100.BASE.md`（当天 21:13 被改写）、`__rulemux__200.WORKSPACE.md`（2026-10-10 13:55 创建）。用户原话两问：

1. 「我机器上没有装 DSH 的 Agent，也没有初始化……我都把 Agent 给删了，它还在不停同步，这肯定是有问题的」（怀疑账本/字典里还有记录）。
2. 「DSH 怎么会有这个目录？DSH 是不认这个目录的。而且我们的 DSH 是以插件的方式，用 hooks 注入的方式解决的」。

用户明确：**先查不改**，建任务记录，讨论定案后再修。

## 待办

**已实施完毕**（见「过程」实施记录）：D1/D2/D3 + 侧门 R1/R4 + 空列表暗雷 + 文档漂移 5/6 全部落地，`go build/vet/test` 与 `scripts/smoke.sh` 全绿。

剩余待办：

1. **两轮专家复审**（用户 2026-10-14 要求）：对完整 diff 做对抗性复查（漏网场景 / 回归面 / 删除安全边界 / 文档一致性），有问题逐项修到「无问题」才收。
2. **待用户验收**：验收判据 = `go build ./...` && `go vet ./...` && `go test ./...` && `bash scripts/smoke.sh` 全绿；外加用户机场景端到端（账本只有 workbuddy 的机器上裸 `rulemux sync` 不再创建 `.dsh/rules` / `.trae/rules`）。
3. **问题 2（dsh 定位）已获批改纯注入**（用户 2026-10-14 晚「行，我接受改」）：dsh 从「Tier-1 目录交接」改为「Tier-2 注入交付」，插件直接 `rulemux inject --agent dsh` 取 stdout 注入，`.dsh/rules` 中间目录取消。发布顺序 = **插件先发、CLI 的 Tier 翻转后发**（`inject` 在现行 CLI 已存在 ⇒ 新插件对旧 CLI 照样可用；反之旧插件 + 新 CLI 会读到残留目录注入陈旧内容）。canary 记录见 worklog `202610/10-0000-dsh-adapter-9f1e.md`。

   **Batch A（插件一半）已完成**（2026-10-14，未提交）：`dsh-plugin/index.mjs` 去掉「写/读两半」全部机制——不再有 `RULES_SUBDIR`/`FILE_PREFIX`/`readdirSync`/`readFileSync`，`syncOnce`（`sync --hook --agent dsh`）与 `readRules`（读 `.dsh/rules/__rulemux__*.md`）删除，改为 `rulesFromCli()` 跑 `inject --agent dsh` 取 stdout 注入；`startReady` 只做就绪（CLI + config），不再附带同步；就绪三步与 `REQUIRED_CLI`（`>=0.3.1`，理由改写为「`inject` 是老路径，但 `--agent dsh` 需要 dsh 适配器」）不变。`package.json` 描述改写 + 新增 `"test": "node --test"`；`cordis.patch.yml` 首行注释改写；`README.md` 三节改写（不再有 `.dsh/rules` 交接叙事）。新增 `dsh-plugin/test/host.test.mjs`（3 个用例：注入的就是 CLI stdout 且工作区零写入、每会话只问一次（静默宿主不重复注入）、inject 失败只记日志不断会话）——用 PATH 上的桩 CLI 驱动真实插件代码，本机没有 dsh 宿主的替代证据。验证：`node --check index.mjs` 通过、`node --test` 3/3 通过、`rulemux inject --agent dsh` 端到端 stdout = `<!-- rulemux:managed -->` + 源内容且工作区保持空（对照 `sync --hook --agent dsh` 仍写 `.dsh/rules`，即 CLI 尚未翻转）。插件版本号**未改**（按仓库节奏，升版在发布时单独提交 `chore(rulemux-dsh): x.y.z`）。

   **Batch B（CLI 那一半）已完成**（2026-10-14 晚，未提交）：

   | 落点 | 改动 |
   |---|---|
   | `internal/agents/registry.go` | dsh 条目 `Tier1 → Tier2`、`RulesDir: ""`、注释与 Note 改写（写明「无规则目录，交付＝注入」与历史作废理由）；**新增 `Agent.LegacyRulesDir` 字段**（dsh = `.dsh/rules`），语义是「旧交接目录，只用于报告，绝不读写」 |
   | `internal/cmd/sync.go` | `agentsFromLedger` 收紧为**只认 Tier-1**（Tier-2 注入型即使被账本记着也不进表）；哨兵分支去掉已无意义的 `Style=="external"` 判断；`printTotals` 的未初始化名单改为**「在用集合 − 目录存在」**（用户「没用过的别出现」拍板的落地：从没用过的、已卸载的一律不提）；新增 `mentionedInUse()`（不按 Tier 过滤的「这台机器有没有在用它」，供 doctor 用） |
   | `internal/cmd/doctor.go` | `printAgents` 名单同口径收窄（只列已初始化 + 在用未初始化；一个都没有时印 `Agents — none in use in this workspace`，不印 `0 of 0`）；**注入型且探测不到安装状态（Tier-2 + external = dsh）单列一行说明**，不再误报「未初始化」（那会把用户支去跑修不好它的 init） |
   | `internal/cmd/uninstall.go` | R1 警告**按新机制重写**（插件不再回填磁盘，改为每会话调 `inject` 注入 ⇒ 卸载停不掉它，需在 cordis.patch.yml 置 `disabled: true`）；新增 `legacyResidue()`：卸载 dsh 时报出旧 `.dsh/rules` 还剩几个 `__rulemux__` 文件（**只报不删**，符合「残留不强制清除」）；`--help` 文案同步 |
   | `internal/cmd/init.go` / `inject.go` | external 分支注释与 InitHelp 文案（dsh 改 Tier-2 注入、工作区零产物）；`inject` 头注释补 dsh |
   | 测试 | 把「带 `.dsh/rules` 的 Tier-1」夹具整体迁到 `trae`（`uninstall_test.go` 的 SkippedPrune/D3 边界/非空/软链/前缀子目录、`verify_test.go` 的 R4 三条、`doctor_test.go` 的卸载隐藏、`sync_test.go` 的 M3 与 config×off、`sync_report_test.go`）；新增锁：`TestSyncHookTier2AgentWritesNothing`（`sync --hook --agent dsh` 不改写也不删旧残留、不入账本）、`TestVerifyTier2AgentWritesNothing`（`verify --agent dsh` 不建目录、只提示）、`TestDoctorShowsInjectionOnlyAgent`（dsh 单列一行、不报未初始化）、`TestUninstallExternalAgentWarnsAndReportsLegacyResidue`、`TestSyncBareReportListsOnlyInUseUninitialized`（两阶段：在用未初始化才列名 / 都初始化后整行消失）；`TestAgentsFromLedger` 表补「Tier-2 不进表」 |
   | 冒烟 | `scripts/smoke.sh`：M3/D3/D3 边界/R4/R4B 迁到 trae，**新增第 26b 步**（dsh 注入型全链：`sync --hook --agent dsh` 零写入 → `inject --agent dsh` 出 `rulemux:managed` 文本 → `uninstall --agent dsh` 输出含 `inject --agent dsh` / `disabled: true` / `leftover`，且旧残留文件仍在）；scope note 改写 |
   | 文档 | `requirements.md` Tier 清单（dsh 移入 Tier-2）、`dir-sync.md` §DSH 整节重写（含历史作废说明与发布顺序）、`external/agent-rules-dirs.md` dsh 行 + 2026-10-14 更新说明、`sync-all-and-change-notice.md`、README×2（表格行 / 两半叙事 / §3.1 / 卸载段 / 支持范围 / 痛点与示意图） |

   验证：`go build ./...` + `go vet ./...` + `go test ./...` 全绿（新增/迁移用例全过）；`scripts/smoke.sh` **33 步全绿**；插件侧 `node --check` + `node --test` 3/3。

   **未完成/待办**：① 容器真机 canary 未重跑（新链路；需 dsh 环境）；② 两半都**未提交**（按用户 2026-10-14 指令：全部做完他再发版）；③ 发布顺序硬约束 —— **插件先发**（`chore(rulemux-dsh): 0.4.0` + `dsh-v0.4.0` tag，走 `.github/workflows/dsh-publish.yml`），**CLI 的 Tier 翻转后发**；④ `dsh-plugin/src/client/locales.ts` 面板文案里「rulemux 把规则同步进各家规则目录」这句对 dsh 已不精确，但改它要重跑 `npm run build:client`（需 dsh client devDeps，本机没装）⇒ 留待下次动客户端时一并做。

**范围约束（用户 2026-10-14 晚，长期有效）**：skill（技能包）注入目前**只做了 WorkBuddy 与 CodeBuddy 两家**，其余 agent 一律未做。**在用户明确说「做」之前，不得推进任何其它 agent 的 skill 注入工作**——推进方式是一家一家来，每做完一家由用户亲自测试一家。本批改动不新增任何 agent 的 skill 注入：workbuddy/codebuddy 的既有 skills 目录只在 uninstall/verify 里被**清理与收敛**（清理 ≠ 注入）；dsh 改注入后仍只走 rules 一路（与今天一致，不含 skills）。

## 下一步

执行两轮 code-explorer 专家复审（用户要求），按复审结论修完 → 交用户验收。验收通过后再由用户说「结案」才走 `close`，不在 agent 侧自行结案。

## 过程

排查基线 revision：`25bde37`（HEAD）。全程只读，未改任何代码。

**根因链（三个缺陷环环相扣，与 dsh 装没装无关，任何 agent 卸了都会中招）**：

| # | 缺陷 | 证据（revision 25bde37） |
|---|---|---|
| D1 | 手动 `rulemux sync`（不带 `--agent`）把全部已验证 Tier-1 agent 当目标，dsh 在内；无「agent 是否在用」门禁，唯一防线是「目录不存在才不写」 | `internal/cmd/sync.go:100`（`targets := targetAgents(requested)`）→ `internal/cmd/sync.go:572-577`（空 id 返回 `Supported()` 全部 Tier-1）；create 仅在 hook 点名时为 true：`internal/cmd/sync.go:127-130` |
| D2 | hook 路径按账本 cascade 同步历史 agent；账本只有「按工作区整条删」，无按 agent 删除入口；uninstall 不碰账本 agents 记录 | cascade 触发：`internal/cmd/sync.go:144-146`；cascade 取账本：`internal/cmd/sync.go:316-333`；账本仅 `Prune(workspaces)`：`internal/state/ledger.go:212`；`internal/cmd/uninstall.go` 全文无账本 agents 移除 |
| D3 | `rulemux uninstall --agent dsh` 只删目录内 `__rulemux__*` 文件，空目录本身保留；下一次 sync（手动 D1 或 hook cascade D2）回填 | 删文件不删目录：`internal/cmd/uninstall.go:96-107`；engine 只建不删目录：`internal/engine/sync.go:120-128` |

**现象复现路径**：开一次 WorkBuddy 会话 → `~/.workbuddy/settings.json` 的 SessionStart 钩子触发 `rulemux sync --hook --agent workbuddy` → cascade 从账本捞出 dsh → `.dsh/rules` 目录仍在 → 重新写入 `__rulemux__100.BASE.md`。用户无需做任何事，会话一开就发生（BASE.md 当天 21:13 被改即此路径）。

**`.dsh/rules` 当初怎么来的**（用户问「是不是原来初始化过」）：唯一能创建该目录的路径是 `rulemux sync --hook --agent dsh`（只有 hook 点名才允许建目录，`internal/cmd/sync.go:127-130`）。两个可能触发者：① 2026-10-10 dsh 适配器联调时手动执行——`__rulemux__200.WORKSPACE.md` 的 mtime 2026-10-10 13:55 与 worklog `202610/10-0000-dsh-adapter-9f1e.md` 联调日期吻合；② dsh-plugin 每会话自行执行 `rulemux sync --hook --agent dsh`（`dsh-plugin/index.mjs:365`）——插件只要装过并开过一次会话，目录即落下。无论哪种，此后该目录 + 账本 dsh 记录构成「不死之身」。

**问题 2 的事实核对**：dsh 在 registry 为 Tier-1 + `.dsh/rules` + `Style: "external"`（`internal/agents/registry.go` dsh 条目）。「拆两半」设计：Go 写真实拷贝进 `.dsh/rules/__rulemux__*.md`，dsh-plugin 在 `agent/pre-step` 钩子读该目录注入一次（`dsh-plugin/index.mjs:62-63`、`dsh-plugin/index.mjs:386-401`、`dsh-plugin/index.mjs:492-528`）。「dsh 宿主不认这个目录」属实——`.dsh/rules` 只是 rulemux 与插件间的交接点；「dsh 以插件 + hooks 注入解决」也与现状不矛盾（插件 + pre-step 注入）。真正待拍板的是：中间目录保留（复用 Go 同步引擎的前缀/去重/删残渣）还是去掉（插件直读配置源，JS 侧重写同步逻辑）。**注意：「走源」能力本来就有**——`rulemux inject --agent dsh` 已存在（`internal/cmd/inject.go:18-58` + `internal/engine/inject.go:19-38` RenderInject，Tier-2 现役路径），插件 spawn 它拿 stdout 直接注入即可，中间目录并非技术必需，是 2026-10-10 选型（理由：Tier-1 架构一致性 / A3 硬指标 / 盘上可检查，见 `docs/design/features/dir-sync.md:53-80`、`docs/design/requirements.md:27,32,49`）。

**现场实查（2026-10-14，两台机器）**：
- 本开发机：`~/.rulemux/workspaces.json` 中 `/workspace/Temp` → `["dsh","trae"]`，另有 4 个工作区记 `*` 哨兵（`/workspace`、`/workspace/NAS-Docker`、本仓库、`/code/fork/awesome-dsh-plugin`）⇒ 这些工作区的 cascade / `sync --all` 均含 dsh。
- 用户 macOS 机（HOME=/Users/guojia）：账本仅 `Agent.Workspace/Explore` 与 `Agent.Workspace/Temp` 两条、均为 `["workbuddy"]`；`which dsh` 为空、dsh 未装 ⇒ **用户机账本无 dsh 污染**。⇒ **用户机元凶是 D1**：裸 `rulemux sync`（无 `--agent`）= 全量已验证 Tier-1 写入已存在目录（`internal/cmd/sync.go:100`+`:572-577`）；`.dsh/rules` 系 2026-10-10 联调时 `sync --hook --agent dsh`（全项目唯一 create=true 路径，`internal/cmd/sync.go:127-130`）遗留；此后每次裸 sync 重写（今早用户执行的同步命令即触发者）。

**实施记录（2026-10-14，基线 `25bde37`）**：

| 项 | 落点 | 要点 |
|---|---|---|
| D1 | `internal/cmd/sync.go` | 裸 sync targets = `账本(ws) ∪ 配置显式点名 − off`（`bareTargets` / `configExplicitAgents` / `allTier1`）；空 ⇒ 提示 `sync --agent <id>`、exit 0、零写入；`healHooks` 解耦为全量；`agentsFromLedger` 暗雷修复（nil/空 = 无 agent，仅 `*` = 全部 − off）；`printTotals` 未初始化名单用 `allTier1()` 并减 `off` |
| D2 | `internal/state/ledger.go`、`internal/cmd/uninstall.go` | 账本加 `Off` 字段 + `UpdateLedger`（唯一写入口，`Record`/`Prune` 都走它）+ `ForgetAgent`/`AddOff`/`ClearOff`；`Load` 只在整文件无 `agents` 键时回填 `*`；`Save` 去 `agents` 的 omitempty；uninstall 先清理后动账本（`pruneOutcome` 三态：OK/skipped/failed），钩子没摘掉整个跳过；sweep 返回「清过或确无东西可清」的 eligible 列表；账本一次加锁批量写 |
| D3 | `internal/cmd/uninstall.go` | `removeEmptyLeaf`：Lstat 排除软链、`len(ReadDir)==0` 才算空、只 `os.Remove`、不上溯；只对工作区级 rules/skills 用；用户级目录永不删 |
| H3 | 同上 | uninstall 补当前工作区 skills 目录收敛（此前只在 sweep 里收） |
| R1 | 同上 | external agent 硬警告：只印有据的 cordis `disabled: true`（`dsh-plugin/cordis.patch.yml:9-12`），不印未坐实的 remove 语法；措辞定位为「卸载没卸干净」，非重启用引导 |
| R4 | `internal/cmd/verify.go`、`main.go` | 裸 verify targets 走同一公式（只探在用的 Tier-1）；显式 `--agent` 永久放行；裸 `--clean` 用 `allTier1()` 超集清理；新增 `--config` + 读配置失败降级为仅账本；`--clean` 错误聚合不再半途而废 |
| 卸载即断联 | `internal/cmd/doctor.go`、`init.go`、`sync.go` | doctor 初始化段减 `off`（已卸 agent 不出现）；`init` 安装模式 `clearOff`（唯一重启用入口）；uninstall 与 sync 输出**零**重启用提示 |
| 测试 | `internal/state/ledger_test.go`、`internal/cmd/{sync,uninstall,doctor}_test.go`、新 `internal/cmd/verify_test.go` | 新增约 25 个用例（账本空语义/off 往返/ForgetAgent/agentsFromLedger 表驱动/D1 四条/D2 四条/D3 边界/verify 四条/doctor）；既有测试零修改即全绿（`sync_test.go:406` 由 M3 配置点名并集自然接住） |
| 冒烟 | `scripts/smoke.sh` | 重写 4 处**预存漂移**断言（`:73/:86/:96` 旧报告字符串 + `:212` doctor 文案 —— 这 4 步在修复前就已 FAIL）；新增 8 组步骤：D1-a/b/c、M3、D2、D3+R1、D3 边界（用户文件/子目录/软链）、R4；修 `:8-12` scope note 与 `:38` 注释 |
| 文档 | README×2、`sync-all-and-change-notice.md`、`dir-sync.md`、`requirements.md`、`init/uninstall --help`、registry dsh 注释 | CLI 层「省略 --agent」口径改账本制；Tier 清单补 dsh、删「DeepSeek 移出范围」矛盾句；dsh canary 状态从 🔴 未坐实 改为 ✅ 已坐实；registry dsh 注释与 Note 同步更正。**配置层「`agents` 省略 = 所有 supported agent」一字未动**（与 CLI 层是两个机制） |

**专家复审（两轮，用户 2026-10-14 要求「修到确定没有问题」）**：

| 轮次 | 发现 | 处置 |
|---|---|---|
| 一 | **必修**：裸 `verify --clean` 在「在用集合为空」时命中早退 return ⇒ 超集清理被短路，`verify --agent dsh` 种下的 canary 永远清不掉（R4 自相矛盾） | 早退条件加 `&& !f.Has("clean")`；clean 循环补 `removeEmptyLeaf`；新增 `TestVerifyCleanRemovesCanaryAndEmptiedDir` + 冒烟第 28 步的空在用集合分支锁住 |
| 一 | P0：`init` 的 `clearOff`（唯一重启用入口）零测试；`pruneSkipped` 三态零覆盖；「卸载 → 后续裸 sync 被 off 阻断」无端到端测试；verify 降级分支零覆盖 | 补 `TestInitClearsOff`（含 external 分支）、`TestUninstallSkippedPruneKeepsLedgerButOffBlocks`、`TestVerifyDegradedConfigUsesLedgerOnly` |
| 一 | P1：`TestCascadeNeverCreatesDirs` 近乎恒真（目录不存在时无论如何都通过）；doctor 计数分母无断言；「配置点名 ∩ off」无测试 | 预建 `.claude/rules`/`.ghost/rules` 后断言未被写；doctor 断言 `0 of 6`；补 `TestSyncBareConfigExplicitAgentSkipsOff` |
| 一 | P2：冒烟 JSON 断言按行窗口脆弱；`↑`/`−` 非 ASCII 断言在非 UTF-8 locale 下不可靠；D1-b 注释说 workbuddy 却种 codebuddy；第 12 步隐式依赖前序状态 | 改 awk 大括号截取 agents 块 + 断言 off；断言改用 ASCII 总计小表；种子改 workbuddy（真正复刻用户机）；加显式前置断言 |
| 二 | **必修**：账本里的哨兵 `*`（v1 迁移占位；本机 4 个工作区就是它）会被 `agentsFromLedger` 展开成「全部 Tier-1」⇒ 裸 sync / cascade / `sync --all` 对 `*` 工作区里**遗留的 `.dsh/rules` 继续写入** —— 正是「新的不许再写」的禁区 | `*` 收紧为「只取**有在用证据**的 agent」：钩子型要求钩子已装（`hookInstalled`），external 型（dsh）不纳入（其目录可能是插件或 `verify` 建出来的，不构成「用过」证据）；回访清理不受影响（回访只看 `Workspaces`）。新增 `TestAgentsFromLedgerSentinel` + 冒烟第 29 步（v1 账本 + 遗留 `.dsh/rules` ⇒ 内容不得被改写） |
| 二 | 低：共享组「一个钩子都没装 ⇒ 退回整组」的兜底不读 off ⇒ 「两家钩子都不在 + off 含其中一家」时手动 sync 会把已卸那家并回来 | 兜底整组减 off；新增 `TestSharedGroupFallbackRespectsOff` |
| 二 | 低（文案/一致性）：`init --refresh` 会列出已卸 agent（违反「卸载即断联」）；R1 警告在没装 dsh 的机器上也是断言式口吻（「不能卸插件…」）；README 那句「never writes to an agent that was never used here」在 `*` 账本下失真 | `refreshHooks` 无 `--agent` 时跳过 off（`TestRefreshHidesOffAgents`）；R1 改成条件句（「若插件仍装着，则…」）；README×2 措辞按新语义改写 |
| 二 | 低（记录项）：纯手动（组内无钩子）时 uninstall 的收敛侧没有整组兜底 ⇒ 会清掉整组前缀文件（既有取舍） | 记入 `dir-sync.md` 共享目录一节的「两侧不对称」说明，本次不改行为 |

| 三 | 判定「可交付、无必修」，但指出一处与计划决策冲突的窄边界：`hookedPeersIn` 兜底减 off 时会把**正在同步的那个 agent 自己**也剔掉 ⇒ 显式 `sync --agent <已卸 agent>` 变成「want 为空 ⇒ 删它的残留」，而计划明确「显式点名按意图执行」 | 兜底加 `self` 例外（`hookedPeersIn(group, ws, self)`：正在处理的 agent 永远留在组里）；新增 `TestExplicitSyncOfOffAgentStillExecutes`；顺手把「不需要任何额外的状态文件」的注释与实际读 off 的行为对齐 |

**实施期与两轮专家评审的偏离（均已在代码注释里写明理由）**：

1. `ForgetAgent` **不展开哨兵 `*`**（计划文本写「哨兵先展开再减」）：`*` 的排除交给 `off` 在展开时完成——物化清单等于把「猜测此刻有哪些 agent」写进账本（正是 B2 要避的），还会让将来新就绪的 agent 不再被 `*` 覆盖。
2. `--off/--all` 走**统一路径**（每个 agent `AddOff` + `ForgetAgent`），而非 L4 建议的「只清 agents 不写 off」：代码路径更少，语义同样正确（全卸后全体确实处于已卸状态，`init` 可逐个清回）。
3. **init 不记账**（追认，见下），配置点名并集用严格判据「`agents` 字段非空且点名」（空 agents 命中全部，若当点名用会把 D1 收窄整个抵消 —— M1）。
4. doctor 的初始化段也减 `off`（用户「卸载即断联」指令的延伸）。

## 结论

根因已定位并**修复完毕**（见「过程」）：问题 1 = D1/D2/D3 三缺陷叠加（用户机实测元凶为 D1：裸 sync 全量 targets；本开发机另有 D2 账本污染实例），问题 2 = dsh `.dsh/rules` 交接目录的设计分歧（非缺陷，「走源」能力已具备，留目录是选型，本次未动）。

**用户拍板（2026-10-14）**：① 遗留残留（已存在、清不掉的文件/目录）不强制清除，修复目标是「新的不许再写」；② 可预判且有方案的边界情况属实现要求（写代码时写好），不称风险；③ **卸载即断联**——agent 一旦 uninstall 就从用户世界消失，例行输出不再出现、不 nag、不给重启用提示（回来只有一条路：用户主动 `rulemux init --agent <id>`）；④ **追认 init 不记账**（原工单口径「init 落地时记账」作废：dsh 插件就绪链跑 `init --agent dsh` 不带 `--workspace` 会记错工作区，「在用」由 sync 实际落盘自然产生）；⑤ uninstall 输出**不加**任何重启用提示，且 sync 汇总的未初始化名单与 doctor 初始化段都**减** `off`（已卸 agent 在任何例行输出里都不出现、不 nag）——「卸载即断联」的直接落地；「回来」只有一条路：用户主动 `rulemux init --agent <id>`（清 `off`），不由任何提示牵引。

遗留残留处置：用户机现存 `.dsh/rules` 手动删、或修好后跑 `rulemux uninstall --agent dsh`（会连空目录一起清）；新代码下不会再被写。

**当前状态（2026-10-14 收尾）**：注入切换（Batch A/B）与未安装/已卸载同步缺陷（D1/D2/D3）均已合入并随 `rulemux-dsh` 0.4.x 发布；同日容器真机 canary 通过（插件装载即注入、`inject --agent dsh` 正常吐规则），**用户验收通过**。遗留 ④（locales.ts 面板文案对 dsh 不精确）按原计划「留待下次动客户端时一并做」，不阻塞结案。

**交叉引用（非本任务范围）**：本任务之后，2026-10-14 又发现并修复了插件 `cliOk()` 的版本判定漏洞——放行 `0.3.1`–`0.4.1` 这些「认识 dsh 却解析不了行尾 `#` 注释配置」的旧 CLI ⇒ 永不自动升级 ⇒ 静默不注入。该修复独立于本任务，单独立任务跟踪：`REQUIRED_CLI` 提到 `>=0.4.2`（rulemux-dsh 0.4.1）、`npm install -g --force` 兜底（0.4.2）。

**用户追加拍板（2026-10-14，晚）**：

- **例行输出只提「在用」的 agent**：用户原话「第二个没用过的别出现了，没有意义。用户只用这个 Agent，你把其他的提给他干嘛？」⇒ sync 汇总与 doctor 的名单口径改为**「在用集合（账本 ∪ 配置点名）− 本轮已落盘」**，从没用过的一律不出现（已卸的因在 `off`/已从账本移除，同样不出现）。副作用（已知并接受）：新机器上该行自然消失，可选 agent 的发现途径回到 `rulemux --help` / README；唯一仍会出现的情形是「用过但目录不在了」（此时提示才有用）。
- **dsh 定位：倾向改纯注入**（用户原话「明明能注入，你改着这个干嘛呢？」）。已核实的事实：dsh 今天**只**收「工作区级规则」一路（无 `SkillsDir`、无 `UserRulesDirs`，`internal/agents/registry.go:224-236`），而 `rulemux inject --agent dsh`（`internal/cmd/inject.go:18-58` → `engine.RenderInject`）交付的正是这一路 ⇒ **内容不丢**（此前「会丢用户级源/skills」的担心经核实不成立）。用户要求先给分析、**不要直接改**。

**遗留/未纳入本次的项（已知，不影响本次判据）**：
1. **问题 2（dsh 定位）**：待用户拍板，不在本次实施范围。若改纯注入，需动：① registry（`Tier: Tier1 → Tier2`、`RulesDir` 清空、Note 改写）；② 插件（`dsh-plugin/index.mjs:365` 的 `sync --hook --agent dsh` 换成 `inject --agent dsh`、`readRules` 换成 stdout、移除 `.dsh/rules` 相关常量）；③ `REQUIRED_CLI` 版本地板与**发布顺序**；④ 文档叙事（`requirements.md` 现把 Tier-2 注入定义为「降级」）+ 重跑容器 canary。**唯一的真问题**：老插件（仍跑 `sync --hook --agent dsh`）+ 新 CLI（dsh 已是 Tier-2 ⇒ sync 跳过它、不写文件）⇒ 老插件去读残留的 `.dsh/rules` 会注入**陈旧内容**；解法是「先发插件、后转 CLI」的分阶段发布或过渡期兼容写入。另外：改注入后本次为 dsh 加的 R1 硬警告**不再必要**（插件不再写文件）。
3. 手改账本把某 agent 加进 `off` = 静默停用（无提示、无确认）——设计使然，未加交互。
4. `init --refresh --agent <未核实 id>` 会返回 2，与函数注释「永远返回 0」不符（既有，与本批无关）。
5. `--agent A --agent B` 重复给值只有后者生效且不报错（既有；文档口径是逗号形式 `--agent A,B`）。
6. 并发窄窗：uninstall 的「清理」与「写账本」之间若有 hook sync 插入，会留下一批账本已删、文件仍在的一次性残留（有 R1/告警兜底，不建机制）。
7. `koc-codelint` 在本机未安装（用户级仅 hindsight-coding-agent、仓库仅 koc-code-doc-progress）⇒ 按工作区规则「提交前必须过基础代码检查」的**该 skill 无法调用**，已用等价手段替代：`go build ./... && go vet ./... && go test ./... -count=1` + `go fmt ./...`（无改动）+ `bash -n scripts/smoke.sh` + 全量 `scripts/smoke.sh` + 缓存/残留检查。装 skill 需用户授权。
