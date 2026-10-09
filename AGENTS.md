# 本工作区的强制约定（Agent 每次会话自动遵守）

> 本工作区是 Git 仓库，由 CodeBuddy / TraeCode / Claude Code 等多个 coding agent、多台机器共用；文件随仓库提交入 Git，**从 Git 取用**。
> 本文件每轮全量加载 ⇒ 只写本仓库独有、不写就会做错的规则；会变的事实不写。

---

# rulemux — agent 操作守则

> 本文件只写**本仓库独有**的东西：本项目的文档落点与本项目约定。

## 一、本项目的文档落点（要做什么 → 先读哪份）

| 我要做 | 先读 |
|---|---|
| **用户到底要什么**（需求 / 验收标准） | [`docs/design/requirements.md`](docs/design/requirements.md) |
| 为什么这么设计（核心原理 / 方案选型 / 命名） | [`docs/design/architecture.md`](docs/design/architecture.md) |
| 有哪些功能、每个干什么 | [`docs/design/features.md`](docs/design/features.md) |
| 改「目录同步」（一级方案） | [`docs/design/features/dir-sync.md`](docs/design/features/dir-sync.md) |
| 改「hook 注入」（降级方案） | [`docs/design/features/hook-injection.md`](docs/design/features/hook-injection.md) |
| 接某个 agent（它的规则目录长什么样） | [`docs/design/external/agent-rules-dirs.md`](docs/design/external/agent-rules-dirs.md) |
| 验证某个 agent 是不是真生效 | [`docs/design/features/verification.md`](docs/design/features/verification.md) |
| 某个功能当时怎么做的、踩过什么坑 | `docs/worklog/<工作包名>.md` |

- 目录、命名、每类「必须写 / 不得写」、生命周期、硬规则：**全在 [`docs/README.md`](docs/README.md)**，本文件不复述。

## 二、本项目独有的约定

> 1–3 项 ⬜ **待补**（代码定型后再写，编号先占位）；第 4 项现在即适用。

1. **构建与提交**：⬜ 待补（定语言 / 构建方式后写，含是否需 build 后再提交）。
2. **验证方式**：⬜ 待补（是否提供冒烟 / 类型检查命令）。
3. **远端与推送**：⬜ 待补（确认远端与协议后写）。
4. **凡涉及各家 agent 的规则目录 / hooks 配置，一律「先读源码与文档，再动手」，禁止靠运行时试探猜**：
   - **要读到实现本体**（目录在哪、扩展名、读不读全部、frontmatter 语义），不是只看文档描述；结论带出处（`文件:行号` 或包版本）。
   - **读到的结论必须回写**（这一步最容易被漏）：写进 [`docs/design/external/`](docs/design/external/) —— 已有文档就补充或更正，**没有**就新建一篇（头部按外部事实类规矩写：类型 / 适用版本 / 状态 / 来源 / 配套）；过程另记 `docs/worklog/<工作包>.md`。**不许只留在对话里** —— 下次没人知道，同一个点会被重复排查。
   - **每条结论都标「适用版本」**：写清是哪个 agent、哪个版本（上游升级后据此复核）。
   - **未核实的内容不得写成事实**：标注 🔴 未核实，并把核实任务挂进 [`docs/PROGRESS.md`](docs/PROGRESS.md) 未决项。
