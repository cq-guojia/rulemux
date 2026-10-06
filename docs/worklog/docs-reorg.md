# 工作包：文档体系重建（rulemux 立项首包）

> **状态**：✅ **完成封卷**（2026-10-06）
> **需求原话**（用户）：
> - 「这是我新建的一个我们要新做的开源项目……`DESIGN.md` 里面放的是前期调研的需求、希望实现的功能、来龙去脉，但是放得不规范，是从其他地方直接把需求挪过来的。」
> - 「第一步不是要去做写代码，而是要把文档先给规整起来。」
> - 「`docs` 这个文件夹最重要，以后所有文档都围绕它建立：需求文档 / 工作进度 / 工作交接卡 / 需求功能实现、架构等，要维护好。这是从 DeepSeek harness 插件拷贝过来的文档结构，先仔细读，结合原有文档和现在的需求看哪些不合适、要改。」
> - 「`RULES.md` 全区通用，不要改。」
> **配套**：[`../README.md`](../README.md)（文档规矩）· [`../PROGRESS.md`](../PROGRESS.md)（现场层）

---

## 一、诊断：旧文档体系为什么不合用

`docs/` 整拷自旧项目 `dsh-task-dispatch-table`（**DSH 宿主的 UI 插件**）。逐项查证结果：

| 文件 | 查证结果 |
|---|---|
| `docs/PROGRESS.md` / `PROGRESS-HISTORY.md` | 满篇 dsh 工作项（侧边栏 z-index、任务调度、月历视图、Office 预览…），与 rulemux **零关系** ⇒ 重置 |
| `docs/README.md` §二 分类表 | 含「数据库 `data-model.md`」「样式 `ui-foundation` / `ui-style-guide`」「外部事实 `dsh-capabilities` / `session-view-ui-map`」——rulemux **无 UI、无 DB、无 DSH 宿主** ⇒ 分类表改造 |
| `AGENTS.md` | 标题仍是 `dsh-task-dispatch-table`；§一 落点表指向一批**不存在**的文档；§二 六条约定全是 dsh 专属（dist 入库 / 冒烟 / ssh remote 直连 / DSH 宿主读源码 / 真实取数禁模拟）⇒ 项目段改写 |
| `docs/design/external/`、`features/`、`worklog/`、`examples/` | **全空** ⇒ 正好留给 rulemux 用 |
| 根 `README.md` | **不存在**（用户以为拷来了，实测根目录只有 `docs/`、`AGENTS.md`、`DESIGN.md`、`RULES.md`）⇒ doc 系统第一入口缺失，需新建 |

**结论**：旧体系只有「骨架」（分层 + 分类规矩）可复用，**内容与分类表全是另一个项目的**。

---

## 二、确认过的三个岔路（用户拍板）

1. `DESIGN.md` **不原地规整** —— 按 `docs/README.md` 的分类**分解后落位**。
2. 根 `README.md` 按 doc 系统「对外」类格式**全重写**（实际是新建）。
3. `AGENTS.md` 项目段**先只放落点表**，约定类（build / 远端 / 冒烟等）等代码定了再补。

---

## 三、做了什么

### 3.1 `docs/README.md` 分类表去 dsh 化

- 删「数据库」「样式」两类，并写明「本项目无 UI、无数据库 ⇒ 不建」；
- 新增「架构」类 → `design/architecture.md`；
- 「功能」改为指向 `design/features.md` + `design/features/`，去掉「对应页面」措辞；
- 「外部事实」改造为「各 agent 的工作区规则目录」，落 `design/external/<主题>.md`；
- §四 写法表：「宿主升级后复核」→「上游升级后复核」、「对应页面与文件」→「对应文件」。

### 3.2 `DESIGN.md` 分解落位

| 原章节 | 落位 |
|---|---|
| §1 核心原理 + §7 命名 + 方案选型 | [`../design/architecture.md`](../design/architecture.md) |
| §2 一级方案：目录同步 | [`../design/features/dir-sync.md`](../design/features/dir-sync.md) |
| §4 备用方案：hook 注入 | [`../design/features/hook-injection.md`](../design/features/hook-injection.md) |
| §6 30 秒实测法 | [`../design/features/verification.md`](../design/features/verification.md) |
| §3 各家工作区规则目录 | [`../design/external/agent-rules-dirs.md`](../design/external/agent-rules-dirs.md)（外部事实类，带「类型 / 适用版本 / 状态 / 来源」头） |
| §5 未决 / TODO | [`../PROGRESS.md`](../PROGRESS.md) 未决项 T1–T4（另补 T5：删除同步） |
| — | 另建 [`../design/features.md`](../design/features.md) 功能总索引 |

### 3.3 现场层与项目规则文件

- `PROGRESS.md`：重置为 rulemux 真实状态（§1.1 本工作包进行中 / §1.2 方案待拍板）+ 未决项 T1–T5 + 下一步；
- `PROGRESS-HISTORY.md`：清空 dsh 历史，只留表头 + 一行初始化；
- `AGENTS.md`：标题改 `rulemux`，§一 落点表换成本项目真实文档，§二 约定留空待补；
- 根 `README.md`：新建（doc 系统「对外」类）；
- 根 `DESIGN.md`：内容已全量迁移 → 删除，避免双真源（原始草稿仍在 git 历史）。

---

## 四、遗留与下一步

未决项 T1–T5 与下一步清单见 [`../PROGRESS.md`](../PROGRESS.md) §二 / §三，本文件不重复。
