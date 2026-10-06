# rulemux

> 一份**中央真源**规则 → 投递到各家 AI coding agent（Claude Code / Trae / CodeBuddy / WorkBuddy / Codex / OpenCode / DeepSeek Harness）。
> 效果与 token 与直接写 `AGENTS.md` 一致：**永不淡出、不累积、不用软链、加删文件自由**。

## 这是什么

你有一份规则文档（编码约定、项目规矩……），希望每个 AI coding agent 在每个工作区都读到它，且效果和直接写进 `AGENTS.md` 一模一样。
`rulemux` 负责把这份真源同步到各家 agent 的工作区规则目录 —— 一处管理，多处生效。

**核心做法**：hook 只当「投递员」，把真源文档**真实拷贝**进各 agent 原生加载的规则目录，由 harness 自己读 ⇒ 享有静态前缀语义，因此永不淡出、不累积。
（为什么不靠 hook 注入：见 [`docs/design/architecture.md`](docs/design/architecture.md) §二）

## 安装

⬜ 待补（项目尚未落码）。

## 用法

⬜ 待补（项目尚未落码）。

## 文档

| 想看 | 去哪 |
|---|---|
| 现在做到哪、欠什么 | [`docs/PROGRESS.md`](docs/PROGRESS.md) |
| 文档怎么摆、怎么写 | [`docs/README.md`](docs/README.md) |
| **用户要什么**（需求 / 验收标准） | [`docs/design/requirements.md`](docs/design/requirements.md) |
| 设计（核心原理 / 选型 / 命名） | [`docs/design/architecture.md`](docs/design/architecture.md) |
| 功能列表 | [`docs/design/features.md`](docs/design/features.md) |
| 各家 agent 的规则目录 | [`docs/design/external/agent-rules-dirs.md`](docs/design/external/agent-rules-dirs.md) |
