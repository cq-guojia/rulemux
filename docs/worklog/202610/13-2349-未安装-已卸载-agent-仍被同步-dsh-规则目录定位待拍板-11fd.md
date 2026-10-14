# 20261013-2349-未安装-已卸载-agent-仍被同步-dsh-规则目录定位待拍板-11fd：未安装/已卸载 agent 仍被同步 + dsh 规则目录定位待拍板

> **创建**：2026-10-13 23:49:26（@GuoJia-Agent）

## 背景

用户本机只装了 WorkBuddy，未装/未初始化 dsh（DeepSeek Harness），2026-10-13 执行同步命令时在 `Temp` 工作区发现 `.dsh/rules/` 下仍有 rulemux 同步产物：`__rulemux__100.BASE.md`（当天 21:13 被改写）、`__rulemux__200.WORKSPACE.md`（2026-10-10 13:55 创建）。用户原话两问：

1. 「我机器上没有装 DSH 的 Agent，也没有初始化……我都把 Agent 给删了，它还在不停同步，这肯定是有问题的」（怀疑账本/字典里还有记录）。
2. 「DSH 怎么会有这个目录？DSH 是不认这个目录的。而且我们的 DSH 是以插件的方式，用 hooks 注入的方式解决的」。

用户明确：**先查不改**，建任务记录，讨论定案后再修。

## 待办

问题 1（三个缺陷，修复口径待拍板）：

1. **D2 账本无按 agent 移除**：uninstall 后账本仍记 (工作区, agent)，任一 agent 的 hook sync 都会 cascade 复活已卸载 agent。拍板：账本加按 agent 删除入口，uninstall 时同步移除。
2. **D3 uninstall 留空目录**：只删 `.dsh/rules` 内 `__rulemux__*` 文件，空目录本身保留，下一次 sync 即回填。拍板：Tier-1 目录删空后是否连目录一起删。
3. **D1 手动 sync 无「在用」判据**：不带 `--agent` 时 targets = 全部已验证 Tier-1 agent；对 Style=external 的 dsh，`hookInstalled` 永为 false，现有「装着钩子=还在用」判据不成立。拍板：手动 sync 的 targets 该按什么收窄（仅显式 `--agent`？目录非空？）。

问题 2（设计分歧，待拍板）：

4. dsh 定位：保留 `.dsh/rules` 作为 Go/插件交接点（现状，Tier-1），还是改纯注入（registry 降 Tier-2，插件直读 `~/.rulemux/config.toml` 源，JS 侧重写同步逻辑）。canary 记录见 worklog `202610/10-0000-dsh-adapter-9f1e.md`（拆两半方案容器内通过）。

附带文档漂移（顺手修，不阻塞）：

5. registry dsh 注释仍写「本机无 dsh 可跑 canary / 注入链未验证」，与 10-10 canary 已通过矛盾（`internal/agents/registry.go` dsh 条目注释）。
6. `init --help` / `uninstall --help` 的 agent 清单漏 dsh（`internal/cmd/init.go` 帮助文本、`internal/cmd/uninstall.go` 帮助文本）。

## 下一步

与用户逐项拍板：先定 D2/D3 的修复口径（账本按 agent 移除 + 空目录处置），再定 D1 的手动 sync targets 判据，最后定 dsh 的 Tier 定位；四项拍板后才动代码。修复实施按项目约定过 `go build ./...` / `go vet ./...` / `go test ./...`，发布类改动另跑 `scripts/smoke.sh`。

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

## 结论

根因已定位并记录（见「过程」）：问题 1 = D1/D2/D3 三缺陷叠加（用户机实测元凶为 D1：裸 sync 全量 targets；本开发机另有 D2 账本污染实例），问题 2 = dsh `.dsh/rules` 交接目录的设计分歧（非缺陷，「走源」能力已具备，留目录是选型）。

**用户拍板（2026-10-14）**：① 遗留残留（已存在、清不掉的文件/目录）不强制清除，修复目标是「新的不许再写」；② 可预判且有方案的边界情况属实现要求（写代码时写好），不称风险。

**实现要求（据此固化，开工即按此写）**：D1 裸 sync targets 按账本收窄（`*`=全部；账本空 ⇒ 提示 init、exit 0、不写盘；init 落地时记账；healHooks 维持全量与同步范围解耦）；D2 uninstall 先清理后删账本（清理跳过 ⇒ 告警不动账本；`*` 哨兵先展开再减；只删 Stat 成功的工作区）；D3 uninstall 删空目录仅限工作区级 leaf + 确认为空 + 不碰软链 + 不上溯父目录（用户级保留）；R1 uninstall external agent 硬警告「插件未卸会持续回填」；R4 verify 按「在用」过滤 targets。连必修 `agentsFromLedger` 空列表陷阱（`internal/cmd/sync.go:447`，空列表现被当「全部 agent」）。遗留残留处置：用户机现存 `.dsh/rules` 手动删或修好后跑 `rulemux uninstall --agent dsh`，新代码下不会再被写。
