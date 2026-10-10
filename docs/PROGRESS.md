# 进度（进行中）

> **这是什么**：本仓库**唯一**的在办事项真源 —— 现在做到哪、欠什么、下一步做什么。
> **已结案的**看 [`PROGRESS-HISTORY.md`](PROGRESS-HISTORY.md)（一行一条：时间 / 完成了什么 / 过程文档）。
> **文档规矩**看 [`README.md`](README.md)。
>
> 本文件**只装在办的事**；事项结案即从本文件移入 `PROGRESS-HISTORY.md`。
> ⚠️ README / issue / 聊天记录都不是真源。

---

## 一、当前状态

### 1.1 需求与方案定型 —— ✅ **已拍板**（2026-10-07 逐条锁定）

> 定型层文档已于 2026-10-07 逐项拍板（结论进 [`design/implementation.md`](design/implementation.md)）：
> [`design/requirements.md`](design/requirements.md)、[`design/architecture.md`](design/architecture.md)、
> [`design/features.md`](design/features.md) + `features/` 各份、
> [`design/external/agent-rules-dirs.md`](design/external/agent-rules-dirs.md) 已对齐新模型。

### 1.2 支持范围定稿 —— ✅ **已拍板**（2026-10-10）

**就这四家，到此为止**（全是 Tier-1、全经 canary 坐实）：`codebuddy`、`workbuddy`、`trae`、`dsh`。
`claude` / `codex` / `opencode` 保留注册但**未核实、不可安装**，且**不计划补齐** —— 用户原话：
「这个项目就是这几个 Agent，到此结束」。对外表述见根 `README.md` §8。

### 1.3 发布方式定稿 —— ✅ **已拍板**（2026-10-10）

GitHub Release 由 CI（推 tag 触发）出；**npm 手动发**，**不配** `NPM_TOKEN`（用户原话：「那个不配，
以后还是手动的」）。两个包：`rulemux`、`rulemux-dsh`（`dsh-plugin/`）。步骤见根 `README.md` §9。

---

## 二、未决项

（无。）

> 已结案项（**T0–T16**）已全部移入 [`PROGRESS-HISTORY.md`](PROGRESS-HISTORY.md)，本表只留在办事项。

---

## 三、下一步

（无在办事项。新工作来了再往这里加，结案即移入 `PROGRESS-HISTORY.md`。）
