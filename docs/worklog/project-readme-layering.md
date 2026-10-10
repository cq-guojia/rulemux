# 承接：层级可无限下放（第 N 层）

> 状态：✅ 完成封卷（2026-10-10）
> 类型：对外文档（README 中英两版 + dsh 插件简介）
> 来源：用户原话（见下）
> 关联：前一篇 [`project-readme-positioning.md`](project-readme-positioning.md)（已封卷，不回改）

## 一、需求原话

> 「我觉得这个好多了，你的层级结构也很好。但是，你现在划分到第三层，下面应该都有一个点。
> 第 N 层是不是能无限支撑下去？他想套多少层就套多少层。」

## 二、代码事实（先核实再写，不猜）

| 结论 | 出处 |
|---|---|
| `use` 是**递归展开**，没有深度上限 | `internal/config/config.go:503-522`（`expandFG`）、`:525-543`（`expandWG`）—— 递归调用自身，无层数计数 |
| 唯一约束是**成环**：A 引 B、B 又引 A ⇒ 加载即报错 | `internal/config/config.go:510-512` / `:531-533`（`visiting` 环检测）；回归用例 `internal/config/config_test.go:312` `TestGroupCycleError` |
| 引用未定义组 / 组重名 ⇒ 报错 | `internal/config/config.go:507-509`、`:486-487` |
| 文件组与工作区分组**都**支持 `use` 嵌套 | `FileGroup.Uses`（`:50`）、`WorkspaceGroup.Uses`（`:58`）；用例 `config_test.go:236` `TestWorkspaceGroupExpandAndGlob` |

## 三、改了什么

| 落点 | 改动 |
|---|---|
| 两版 README §1.2 结构图 | 第 3 层每个分支下补 `└─ 第 4 层 · … · 第 N 层`（想套多深就套多深）；「第 4 层 · 单个工作区」改称**最底层（第 N 层）** |
| 两版 README §1.2 要点 | 新增「层数不限」：`use` 递归展开、想套多少层就套多少层，唯一不允许的是成环（加载即报错） |
| 两版 README §5.2 要点 | 同上补充，并点明**工作区分组同样**支持递归嵌套 |
| `dsh-plugin/locale/{zh,en}.json` | 简介的分层链改为 `… → … → 单个工作区，层数不限` / `as many layers as you like` |

## 四、口径补充（写介绍时照此）

- 图上画到第 3 层**只是常用切法**，不是上限；最底下那层通常是「某个工作区自己的规则」。
- 讲层数时不必给数字承诺，说「想套多少层就套多少层」即可 —— 硬约束只有一条：不能成环。
