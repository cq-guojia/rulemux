# 20261013-2147-sync-与-doctor-输出重做-聚合行-总计-颜色-doctor-四段式-e2a2：sync 与 doctor 输出重做：聚合行+总计+颜色，doctor 四段式

> **创建**：2026-10-13 21:47:10（@GuoJia-Agent）

## 背景

用户贴出 `rulemux sync --all` 真实输出（6 工作区、27 个 agent×目录块、满屏文件名与
"Unchanged, skipped: N file(s)"），原话：「怎么会有这么丑的界面呀？你找几个专家组好好讨论
讨论，模仿模仿别人，比如 NPM」「该省略就省略好吗？不需要把所有的文件名这些都显示出来」
「最后得要有个总计啊，我总计通过了多少个 Agent」「可以有点颜色吗？skills 和规则文件
怎么表现的？我也看不出来」「--verbose 这个先不用加」。

doctor 原话：「一片乱麻。我不知道它是不是用来做检查的，也不知道应该用 info 还是用
doctor。希望通过它看到：1 安装方式（NPM/PNPM/其他）2 安装路径 3 Agent 初始化情况
4 配置文件路径」「你用那个 hooks 删除不是也找不到吗？」

**2026-10-13 深夜用户对方案的两处关键修正**（示例确认前，开发未启动）：

1. 「静默即成功肯定是不行的呀，你也得给我做出示例」——默认视图必须**可见**
   （每个工作区/agent 都有确认行），省略的只是逐文件/逐目录噪音；静默只适用于
   hook 路径（stdout 协议那侧）。
2. 「（dsh）就是没装的，没有 init 的。不需要打叉，没有意义。你以后 init 做多了，
   你不可能啪我他妈来个十几二十行」——未安装/未初始化的 agent **不打 ✗**，
   折叠成一行暗色汇总（如 `· 3 not initialized: workbuddy, claude, codex`）；
   Agents 段每 agent 一行封顶，杜绝 6-8 行/agent 的刷屏。

流程要求（用户原话）：「你先建任务，然后再给我出事例，我们再慢慢聊，确定了再去开发。」

## 待办

- [ ] 出 sync / doctor 示例输出，与用户逐版对齐（当前阶段，开发未启动）
- [ ] 对齐后：internal/ui 包（TTY/NO_COLOR 颜色 + glyph + 截断）+
      sync 聚合输出 + doctor 四段式 + detectInstall + 测试 + 文档

## 下一步

在对话中出三份示例（`sync --all` 用用户贴的真实数据、单工作区 `sync`、
`doctor`），等用户拍版后进入开发。

## 过程

**示例对齐（2026-10-13 夜，两轮）**：
- 第一版示例后用户修正两条：①「静默即成功肯定不行」⇒ 默认视图必须可见
  （每个 scope/agent 都有确认行），静默只属 hook 的 stdout 协议侧；
  ②「（dsh）没装的不用打叉，以后 init 多了不可能啪十几二十行」⇒ 未初始化
  agent 不打 ✗、折叠成一行暗色名单，Agents 段每 agent 一行封顶。
- 第二版示例后用户拍版：「目录单独放一行，下面的每个 Agent 放一行」「同步文件
  的明细都没必要了，只需要：现有多少规则/skill、rules 加几个删几个、skill 加几个
  删几个」「global 也不好看，组织专家团参考别人」⇒ global 改为与工作区块同构的
  scope 块（`~ user-level (every workspace)`）放最前；文件名明细全删，只报数。

**落码**：
- 新包 `internal/ui`（零依赖）：TTY/NO_COLOR/TERM=dumb 自适应颜色 + glyph
  （✔ ↑ ✗ ⚠ ·）+ `TruncateJoin` + `HomeRel` + `Plural`；单测覆盖颜色四态、
  截断边界、~ 压缩。（踩坑：HomeRel 首版少拼分隔符产出 `~rules/a.md`，单测抓住。）
- `internal/cmd/sync.go`：`printSyncResult`（逐目录平铺）退役 ⇒ `syncRun` 扩
  `lines/lineIdx/wsList` + `syncLine`（计数全部取自 engine.SyncResult 切片长度，
  零额外 IO）+ `printSyncReport`（global 块 → 工作区块 → 警告区 → 总计）；
  增量后缀按最长标题 rune 对齐；global 行目录提示按 agent 根目录去重
  （`~/.codebuddy/rules`+`~/.codebuddy/skills` ⇒ `~/.codebuddy`）。
- `internal/cmd/doctor.go`：四段式重写（Install/Config/Agents/汇总）；新增
  `detectInstall`/`installMethod`（纯函数表驱动可测：npm/pnpm/yarn/go install/
  standalone + 包目录推导）；`agentInitialized`（hook 已装；external 样式以
  规则目录存在为据）；`agentStateLine` 一行一 agent（工作区目录相对 ws 显示，
  用户级 ~ 压缩，缺失暗色 (missing)）；新增 `cmd.Version`（main.init 同步，
  ldflags 注入点不变）。
- 顺带修正：doctor 补 `workspace:` 上下文行（agent 初始化态按工作区判）；
  source 计数改用 `ui.Plural`（消灭 "3 source(s)"）。
- 测试：`ui_test.go`（6 例）、`doctor_test.go`（installMethod 表驱动 6 路径 +
  doctor 结构冒烟 + agentStateLine）、`sync_report_test.go`（报告版式 8 项断言 +
  幂等二跑全 ✔ 零增量）。（踩坑：断言里 `%-10s` 补位数错、`ws+"\n"` 与增量后缀
  同行——均为断言错非输出错，修正后全绿。）

**验证**：`gofmt` 干净；`go build/vet/test ./...` 全绿；`scripts/smoke.sh`
ALL SMOKE TESTS PASSED；本机实跑 `sync --all` 两次（第二次全 ✔ 零增量，幂等）
与 `doctor`（四段式齐全）；`script` 伪终端验证颜色（bold/绿/dim 转义在场），
管道无色。（`koc-codelint` skill 本环境不存在，以等价四件套代替。）

## 结论

已完成并推送：sync 输出重做为「global 块 + 工作区块（目录一行、agent 一行）+
仅变化出增量 + 结尾总计 + 未初始化折叠一行 + TTY 颜色」，doctor 重做为四段式
（安装方式/路径 → 配置 → agent 初始化概览 → 汇总）。stdout 协议 JSON、退出码、
hook 无变化零输出语义一格不动；既有 40+ 测试与冒烟全绿。
对示例的两处偏离已向用户声明：Config 段保留源清单（每源一行，旧 doctor 既有
诊断能力，不想要可删）；agent 行内不再打印长 Note（canary 备注已在其归属文档）。
遗留：`--verbose` 按用户要求未做；增量后缀操作符 `+ − ~` 待用户最终确认（示例
阶段建议符号，用户未反对）。
