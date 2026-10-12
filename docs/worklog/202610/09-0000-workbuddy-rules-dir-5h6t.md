# 20261009-0000-workbuddy-rules-dir-5h6t：WorkBuddy 工作区目录坐实 + 共享目录并集/卸载收敛（发 `v0.2.7`，本机验收通过）

> **✅ 完成封卷**：2026-10-09 00:00:00（@GuoJia-Agent）
> **创建**：2026-10-09 00:00:00（@GuoJia-Agent）

## 背景
此前模型认为 WorkBuddy **工作区级**规则目录也独立（`.workbuddy/rules`），但规则落进 WorkBuddy 从不读的目录，连续数个会话读不到——而钩子明明装对了、文件也落了盘。

## 待办
（已结案）历史遗留的孤儿目录 `.workbuddy/rules/`（按旧落点落下的 `__rulemux__*.md`）与探针文件需**手动删除**；某条 source 只写 `agents=["workbuddy"]` 时文件同样出现在 `.codebuddy/rules` 被 CodeBuddy 读到，无法两全。

## 下一步
（已结案）

## 过程
三位置对照探针：同内容探针各放一份进 `$W/.workbuddy/rules/probe_a.md`、`$W/.codebuddy/rules/probe_b.md`、`$W/.workbuddy/memory/probe_c.md`，开新会话问「上下文里有没有 `PROBE-` 字样」。**只有 `PROBE-CB-RULES` 被念出**，且 WorkBuddy 自报路径就是 `.codebuddy/rules/probe_b.md` ⇒ 坐实工作区级规则目录 = `.codebuddy/rules`（与 CodeBuddy **共享**）。
踩坑：①「用户级独立」误推「工作区级独立」——两层结论须分别实测，不能互相推，这一版发出去连续数个会话读不到；② 预览渲染吃掉 frontmatter 的 `---`，误以为文件缺 frontmatter，被用户纠正（代码 `AutoApplyFrontmatter` 一直完整）。
共享目录冲突：落点改回 `.codebuddy/rules` 后两 agent 共享同目录、钩子各装各的，`engine.Sync` 删残留按「本次计划」删 ⇒ 文件来回消失。用户拍板：同步取**并集**、卸载做**收敛**，用「钩子还装没装」当「在用」判据，**不新增状态文件**。
实现落点：`registry.go` `workbuddy.RulesDir=".codebuddy/rules"` + `SharingRulesDir`；`config.go` `SourcesForAny`；`cmd/sync.go` `hookedPeers`/`unionAgentIDs` 及三处改并集；`cmd/uninstall.go` `remainingPeers`+`pruneRulesDir`（必须在 `hooks.Uninstall` 前算）；新增 `uninstall_test.go`/`sync_test.go`/`registry_test.go`。

## 结论
WorkBuddy 工作区规则目录坐实 = `.codebuddy/rules`（共享），引入并集同步 + 卸载收敛；registry `RulesDir` 修正、相关测试补齐；结论升格 `docs/design/external/agent-rules-dirs.md` §5.4、`docs/design/features/dir-sync.md`「共享规则目录」、`docs/design/features/sync-all-and-change-notice.md` §三。
