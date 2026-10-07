---
name: rulemux-docs-reorg
overview: 把从 dsh-task-dispatch-table 整拷过来的 docs/ 体系重做成 rulemux 自己的文档体系：按 doc 系统（docs/README.md）的分类把 DESIGN.md 需求草稿分解落位、重置被污染的 PROGRESS 系列、重写 AGENTS.md 项目段、新建根 README.md。RULES.md 保持不动。
todos:
  - id: reform-docs-readme
    content: 改造 docs/README.md 分类表，去除 dsh 专属类并适配 rulemux
    status: completed
  - id: migrate-design-docs
    content: 将 DESIGN.md 分解迁入 docs/design/ 各设计文档与功能索引
    status: completed
    dependencies:
      - reform-docs-readme
  - id: reset-progress
    content: 重置 PROGRESS 系列并新建 worklog 记录本工作包
    status: completed
    dependencies:
      - reform-docs-readme
      - migrate-design-docs
  - id: rewrite-agents
    content: 改写 AGENTS.md 项目段（落点表+约定留空待补）
    status: completed
    dependencies:
      - reform-docs-readme
      - migrate-design-docs
  - id: create-root-readme
    content: 新建根 README.md 说明项目并指向设计文档
    status: completed
    dependencies:
      - migrate-design-docs
  - id: delete-root-design
    content: 删除根 DESIGN.md（内容已迁移，避免双真源）
    status: completed
    dependencies:
      - migrate-design-docs
---

## 项目背景与现状诊断

`rulemux` 是一个新建的开源项目：把一份「中央真源」规则投递到各家 AI coding agent（Claude Code / Trae / CodeBuddy / WorkBuddy / Codex / OpenCode / DeepSeek Harness），要求效果与 token 等同于直接写 `AGENTS.md`，且永不淡出、不累积、不用软链、加删文件自由。

但当前 `docs/` 整目录是从旧项目 `dsh-task-dispatch-table`（一个 DSH 宿主 UI 插件）拷过来的，**文档内容与 rulemux 完全无关，属于污染，必须重做**。已查证的事实：

- 根目录只有 `docs/`、`AGENTS.md`、`DESIGN.md`、`RULES.md`，**没有根 `README.md`**（用户以为拷来了，实际没有，需新建）。
- `RULES.md`：全区通用规则真源，用户独占维护，**禁止改动**。
- `DESIGN.md`：你的前期需求草稿（7 节：核心原理 / 目录同步 / 各家规则目录 / hook 注入 / 未决 TODO / 30秒实测法 / 命名候选），是真实需求种子，但写法不规范，需按 doc 系统分类分解落位。
- `AGENTS.md`：标题还是 `dsh-task-dispatch-table`，§一 文档落点表、§二 六项约定全是 DSH 专属（dist 入库 / 冒烟 / ssh remote / DSH 宿主读源码 / 真实取数禁模拟），rulemux 里这些文档与约定都不存在。
- `docs/README.md`：doc 规范本身可用，但 §二 分类落位表含 dsh 专属类（数据库、样式、DSH 宿主外部事实），需去 dsh 化。
- `docs/PROGRESS.md` / `docs/PROGRESS-HISTORY.md`：满篇 dsh 工作项与结案历史，全部重置。
- `docs/design/external/`、`docs/design/features/`、`docs/worklog/`、`docs/examples/`：均为空，正好给 rulemux 用。

## 改造总方案（按 doc 系统分类重建）

严格遵循 `docs/README.md` 的分层与分类，把 DESIGN.md 需求草稿**分解**后落到定型层，把被污染的现场层/项目规则文件**重置/改写**，并补齐缺失的根 README。

### 一、DESIGN.md 分解落位（需求 → 定型层）

| 原 DESIGN.md 章节 | 落位文件 | 类别 |
| --- | --- | --- |
| §1 核心原理（为什么必须走原生规则目录）、§7 命名候选、方案选型 | `docs/design/architecture.md` | 方法/架构 |
| §2 一级方案：目录同步 | `docs/design/features/dir-sync.md` | 功能 |
| §4 备用方案：hook 注入（SessionStart+PreCompact） | `docs/design/features/hook-injection.md` | 功能 |
| §3 各家工作区规则目录（已查证表格） | `docs/design/external/agent-rules-dirs.md` | 外部事实（按 agent 分、标适用版本） |
| §6 30 秒实测法 | `docs/design/features/verification.md` | 功能（验证方法） |
| §5 未决/TODO | `docs/PROGRESS.md` 未决项 + 开 `worklog/` | 进度/叙事 |


功能总索引新建 `docs/design/features.md`，列出上述功能文档。

### 二、各文档具体改法

1. **`docs/README.md`（改）**：§一 目录树保留三层结构；§二 分类表去 dsh 化——删除「数据库」「样式」两类（rulemux 无 DB/无 UI），将「外部事实」改造为「各 agent 工作区规则目录事实 → `design/external/agent-rules-dirs.md`」，「功能」指向 `design/features.md` + `design/features/`；新增「架构」类 → `design/architecture.md`；「进度/叙事/定型/对外」保留。§四 各类写法去掉 DSH 措辞。
2. **`AGENTS.md`（改）**：标题改为 `rulemux — agent 操作守则`；头部「多 agent 共用 git 仓库」说明保留（契合本项目主题）；§一 文档落点表换成 rulemux 真实文档（根 README / docs/README / docs/PROGRESS / docs/design/architecture.md / docs/design/features.md / docs/design/external/agent-rules-dirs.md / worklog）；§二 约定类按你确认**先留空**，删掉全部 DSH 专属约定，待代码定型再补；RULES 注入区标记不动。
3. **`docs/PROGRESS.md`（重置）**：清空 dsh 内容，写入 rulemux 真实状态——当前状态=本工作包「文档体系规整（进行中）」；未决项=DESIGN.md §5 的 TODO（Trae/CodeBuddy frontmatter 常驻写法、Codex/OpenCode 分支处理、DeepSeek Harness 规则目录确认、各 agent 实测闭环）；下一步=文档规整完成后进入设计落码。
4. **`docs/PROGRESS-HISTORY.md`（重置）**：清空 dsh 历史，仅保留表头与一行「项目从 dsh 文档模板初始化 rulemux 文档体系（2026-10-06）」，旧内容全删。
5. **`README.md`（根，新建）**：doc 系统要求的「对外」第一入口。写 rulemux 是什么 / 怎么装 / 怎么用（install/use 因暂无代码先占位），不写内部设计，指向 `docs/design/architecture.md` 看详情。
6. **`DESIGN.md`（根，删除）**：内容已全量迁入 `docs/design/`，删除根文件避免双真源（原始草稿仍在 git 历史，无信息丢失）。

### 三、范围边界

- `RULES.md` 不动。
- 本次只做文档规整，**不写代码**。
- `docs/design/external/`、`docs/design/features/`、`docs/worklog/`、`docs/examples/` 子目录结构保留，仅填充内容。