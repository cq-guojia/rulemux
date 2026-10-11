# WorkBuddy 规则目录落点坐实 + 共享目录并集/卸载收敛

> **状态**：✅ 完成封卷（2026-10-09）—— 本文只记过程与踩坑，结论已升格到
> [`../design/external/agent-rules-dirs.md`](../design/external/agent-rules-dirs.md) §5.4、
> [`../design/features/dir-sync.md`](../design/features/dir-sync.md)「共享规则目录」、
> [`../design/features/sync-all-and-change-notice.md`](../design/features/sync-all-and-change-notice.md) §三。
> **需求原话**：从「WorkBuddy 现在不是装在你的这个环境里面，它的 dir 要核实」到「我用的他妈这个工作学习卡，
> 文件都放进去了，明白吗？但是他没读」，最终「把 Trae 做了」之前收口。
> **产出**：`v0.2.5`（拆独立条目）→ `v0.2.6`（配置 `~` 展开）→ `v0.2.7`（落点坐实 + 共享目录机制）。

## 一、要解决什么

WorkBuddy 是独立 macOS 应用（`/Applications/WorkBuddy.app/`）。此前的模型是

- **用户级**配置/钩子目录独立：`~/.workbuddy/settings.json`；
- **工作区级**规则目录**也**独立：`.workbuddy/rules`。

第二句错了。规则于是落进 WorkBuddy 从不读的目录，连续数个会话读不到 —— 而钩子明明装对了、文件也明明落了盘。

## 二、定位过程（三位置对照探针）

`rulemux init --agent workbuddy` 已把钩子正确写进 `~/.workbuddy/settings.json`
（与 Hindsight 的三条钩子并存、未破坏别家条目），`sync` 也把 `__rulemux__100.BASE.md` 等文件落进了
`.workbuddy/rules/`。但 WorkBuddy 会话就是读不到。

（对照 CodeBuddy 的旧 canary 教训：`canary 必须用对照探针，以「内容是否进入本会话上下文」为判据，
而非「文件是否落盘」`。）

于是把**同内容**探针各放一份进三个候选位置：

```
$W/.workbuddy/rules/probe_a.md     → PROBE-WB-RULES
$W/.codebuddy/rules/probe_b.md     → PROBE-CB-RULES
$W/.workbuddy/memory/probe_c.md    → PROBE-WB-MEMORY
```

开新会话问「上下文里有没有 `PROBE-` 字样」。**只有 `PROBE-CB-RULES` 被念出来**，且 WorkBuddy 自报
路径就是 `.codebuddy/rules/probe_b.md`。坐实：**工作区级规则目录 = `.codebuddy/rules`（与 CodeBuddy 共享）**。

另有一个佐证：`.workbuddy/` 目录下只有 `memory` 是 WorkBuddy 自己建的，**`rules` 是我们 hook 建的**
（用户指出的关键事实）—— 它从没主动建过这个目录。

## 三、踩过的坑（两条，都值得记）

1. **「用户级独立」误推「工作区级独立」**。看到 `~/.workbuddy/settings.json` 独立，就顺势把
   `RulesDir` 写成 `.workbuddy/rules`。两层的结论必须**分别实测**，不能互相推。这是本轮最贵的错误：
   整整一版发出去、连续数个会话读不到。
2. **预览渲染吃掉 frontmatter 的 `---`**。用户从 macOS 预览复制文件内容给我看，`---` 被渲染成水平线、
   复制时丢失，我据此怀疑「文件缺 frontmatter」，被用户纠正。代码里 `AutoApplyFrontmatter`
   （`internal/engine/sync.go:28`）写的是完整 `---\nalwaysApply: true\n---\n`，文件一直没问题。
   **教训：不要拿「粘贴回来的渲染结果」当原始文件内容。**

## 四、共享目录引出的冲突

落点改回 `.codebuddy/rules` 后，codebuddy 与 workbuddy 共享同一目录，而两者钩子各装各的、各自触发。
`engine.Sync` 的删残留是「整目录下带 `__rulemux__` 前缀、不在**本次计划**内的一律删」
（`internal/engine/sync.go:164-185`），于是：

- 若某条 source 写了 `agents = ["workbuddy"]`，workbuddy 的钩子落下它，**codebuddy 的钩子下次一跑就把它删了**；
- 两个钩子轮流触发 ⇒ **文件来回消失**（正是历史上被删掉的 `ByRulesDir`+`unionSources` 想解决的问题）。

用户拍板的方案（详见 `dir-sync.md`）：同步取**并集**、卸载做**收敛**，用「钩子还装没装」当「在用」的判据，
**不新增任何状态文件**。

## 五、实现落点（证据）

| 能力 | 位置 |
|---|---|
| 修正落点 | `internal/agents/registry.go` 的 `workbuddy.RulesDir = ".codebuddy/rules"` |
| 按目录分组 | `internal/agents/registry.go` `SharingRulesDir(a)` |
| 并集取源 | `internal/config/config.go` `SourcesForAny(ids, ws)`（空 ids 返回空集，保证全清语义） |
| 在用判定 | `internal/cmd/sync.go` `hookedPeers` / `hookInstalled` / `unionAgentIDs` |
| sync 三处改并集 + 目录去重 | `internal/cmd/sync.go` 主循环 / `cascade` / `syncAll` |
| 卸载收敛 | `internal/cmd/uninstall.go` `remainingPeers` + `pruneRulesDir`（**必须在 `hooks.Uninstall` 之前算**） |

测试：`internal/cmd/uninstall_test.go`（新建，此前 cmd 层卸载零覆盖）、`internal/cmd/sync_test.go`、
`internal/agents/registry_test.go`。

## 六、遗留

- 历史遗留的孤儿目录 `.workbuddy/rules/`（含按旧落点落下的 `__rulemux__*.md`）需**手动删除** ——
  改版后 rulemux 只认 `.codebuddy/rules`，够不到它。
- 探针文件（`probe_a/b/c.md`）不带 `__rulemux__` 前缀，不会被自动清理，同样需手动删。
- 「共享目录」的固有代价：某条 source 只写 `agents = ["workbuddy"]` 时，它的文件同样会出现在
  `.codebuddy/rules` 被 CodeBuddy 读到 —— 无法两全。
