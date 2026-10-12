# 文档规范：本项目的槽位登记表

> 通用文档规矩（落位全集、真源、命名、生命周期、硬规则）由**工作区级规则**统一注入，本文件**不复制、也不转述**通用规范，只登记本项目实际落点与差异。
> 最后更新：2026-10-12。

## 一、槽位登记表（本项目实例）

| 槽位 | 本项目落点 | 说明 |
|---|---|---|
| 文档规范 | `docs/README.md` | 本文件 |
| 状态表 | `docs/progress.json` | 2026-10-11 由旧 `PROGRESS.md` 迁入 |
| 已结案 | `docs/PROGRESS-HISTORY.md` | — |
| 过程叙事 | `docs/worklog/<年月>/` | 按月归档（<年月> 取任务创建月份）；文件名由 koc-code-doc-progress skill 生成，实际路径以状态表 `worklog` 字段为准，命名规则不在此复制 |
| 运行逻辑 | `docs/design/architecture.md` | — |
| 功能总索引 | `docs/design/features.md` | — |
| 各功能 | `docs/design/features/` | 含 `config-groups.md` 等 |
| 外部事实 | `docs/design/external/` | 各 agent 规则目录等 |
| 需求（追加槽位） | `docs/design/requirements.md` | 回答「用户要什么」，与架构分离 |
| 实现期调研归档（项目特例） | `docs/design/implementation.md` | 前期调研 / 跨工作包决策归档，非单工作包 worklog |
| 数据库说明 | 不适用（无数据库） | 保留行 |
| 样式规范 | 不适用（无 UI） | 保留行 |
| 方法规范 | `docs/design/`（项目自定） | 代码规范 `code-conventions.md` ⬜ 待补（代码定型后建） |
| 样例 | `docs/examples/` | — |

## 二、本项目追加与特例

1. **需求类（追加槽位）**：`docs/design/requirements.md` —— 需求变了改这里、方案变了改架构。
2. **实现期调研归档（特例）**：`docs/design/implementation.md` 是项目启动前的调研与跨工作包决策记录，体量跨整个实现期、不是单个工作包，故独立成文；搬入时同步更新过时内容。
3. **状态表已迁 `progress.json`**：2026-10-11 由旧 `PROGRESS.md`（Markdown）迁入。

## 三、rulemux 特有硬约束

涉及各家 agent 规则目录 / hooks 配置的特有硬约束，全文见 [`AGENTS.md`](../AGENTS.md) 项目段第 4 条，本文件不转述。
