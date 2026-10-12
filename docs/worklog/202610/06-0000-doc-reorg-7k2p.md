# 20261006-0000-doc-reorg-7k2p：项目初始化 + 文档体系重建

> **✅ 完成封卷**（2026-10-06 00:00:00，GuoJia-Agent）
> **创建**：2026-10-06 00:00:00（GuoJia-Agent）

## 背景
立项首包。用户要求先把文档体系规整起来：本仓库文档结构从 `dsh-task-dispatch-table` 拷贝而来，需结合原有文档与现在需求调整；`RULES.md` 全区通用、不改。

## 欠什么
（已结案）未决项 T1–T5 与下一步清单见当时状态表。

## 下一步
（已结案）

## 过程
诊断旧文档体系：① `docs/progress.json` 与 `PROGRESS-HISTORY.md` 满篇 dsh 工作项（侧边栏 z-index、任务调度、月历视图等），与 rulemux 零关系；② `docs/README.md` 分类表含「数据库 / 样式」等不适用项；③ `AGENTS.md` 仍是 dsh 标题、§二 约定全是 dsh 专属；④ `design/external/`、`features/`、`worklog/`、`examples/` 全空；⑤ 根 `README.md` 不存在。
确认三个岔路：不原地规整 `DESIGN.md`（按 `docs/README.md` 分类分解落位）、根 `README.md` 全重写、`AGENTS.md` 先只放落点表。
执行：`docs/README.md` 分类表去 dsh 化（删数据库/样式、新增架构/外部事实类）；`DESIGN.md` 分解落位（§1/§7→architecture、§2→dir-sync、§4→hook-injection、§6→verification、§3→agent-rules-dirs、§5→未决项）；`progress.json` 重置为 rulemux 真实状态；`PROGRESS-HISTORY.md` 清空只留初始化；`AGENTS.md` 改标题+落点表；根 `README.md` 新建；`DESIGN.md` 内容全量迁移后删除避免双真源。

## 结论
文档体系从 dsh 模板重建为 rulemux 自有结构：分类表去 dsh 化、需求/架构/功能/外部事实分槽落位、根 `README.md` 与 `AGENTS.md` 项目段改写完成。
