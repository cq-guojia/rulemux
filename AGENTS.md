# 本工作区的强制约定（Agent 每次会话自动遵守）

> 本工作区是 Git 仓库，由 CodeBuddy / TraeCode / Claude Code 等多个 coding agent、多台机器共用；文件随仓库提交入 Git，**从 Git 取用**。
> 本文件每轮全量加载 ⇒ 只写「不写就会做错」的规则；会变的事实不写。

<!-- ==== RULES BEGIN ==== -->
> ⚠️ 本标记区内容由程序自动注入，禁止手改（改了下轮同步即被覆盖）；工作区自有规则写在下方「RULES END」标记之后。

**公用规则（强制）**：全文在同级 [`RULES.md`](RULES.md)，由用户独占维护、随时会改。

- 会话开始前**必须完整读取该文件并遵守**，不要凭印象代替阅读。
- 本文件**不复述、不摘引、不指代**它的任何条目与序号；两边序号各自独立。
<!-- ==== RULES END ==== -->

---

# rulemux — agent 操作守则

> 本文件只写**本仓库独有**的东西：本项目的文档落点与本项目约定。
> 通用规矩（写操作、git、接手入口、文档骨架）在程序注入区指向的那份公用规则文件里，**本文件不重复、不摘引、不指代**它。

## 一、本项目的文档落点（要做什么 → 先读哪份）

| 我要做 | 先读 |
|---|---|
| 这个项目是什么 / 怎么用 | 根 [`README.md`](README.md) |
| 现在做到哪、欠什么、下一步 | [`docs/PROGRESS.md`](docs/PROGRESS.md) |
| 已结案的历史 | [`docs/PROGRESS-HISTORY.md`](docs/PROGRESS-HISTORY.md) |
| **用户到底要什么**（需求 / 验收标准） | [`docs/design/requirements.md`](docs/design/requirements.md) |
| 为什么这么设计（核心原理 / 方案选型 / 命名） | [`docs/design/architecture.md`](docs/design/architecture.md) |
| 有哪些功能、每个干什么 | [`docs/design/features.md`](docs/design/features.md) |
| 改「目录同步」（一级方案） | [`docs/design/features/dir-sync.md`](docs/design/features/dir-sync.md) |
| 改「hook 注入」（降级方案） | [`docs/design/features/hook-injection.md`](docs/design/features/hook-injection.md) |
| 接某个 agent（它的规则目录长什么样） | [`docs/design/external/agent-rules-dirs.md`](docs/design/external/agent-rules-dirs.md) |
| 验证某个 agent 是不是真生效 | [`docs/design/features/verification.md`](docs/design/features/verification.md) |
| 某个功能当时怎么做的、踩过什么坑 | `docs/worklog/<工作包名>.md` |
| **写 / 归置任何文档** | [`docs/README.md`](docs/README.md)（唯一索引） |

## 二、本项目独有的约定

> 1–3 项 ⬜ **待补**（代码定型后再写，编号先占位以保持与通用模板同构）；4–5 项现在即适用。

1. **构建与提交**：⬜ 待补（定语言 / 构建方式后写，含是否需 build 后再提交）。
2. **验证方式**：⬜ 待补（是否提供冒烟 / 类型检查命令）。
3. **远端与推送**：⬜ 待补（确认远端与协议后写）。
4. **凡涉及各家 agent 的规则目录 / hooks 配置，一律「先读源码与文档，再动手」，禁止靠运行时试探猜**：
   - **要读到实现本体**（目录在哪、扩展名、读不读全部、frontmatter 语义），不是只看文档描述；结论带出处（`文件:行号` 或包版本）。
   - **读到的结论必须回写**（这一步最容易被漏）：写进 [`docs/design/external/`](docs/design/external/) —— 已有文档就补充或更正，**没有**就新建一篇（头部按外部事实类规矩写：类型 / 适用版本 / 状态 / 来源 / 配套）；过程另记 `docs/worklog/<工作包>.md`。**不许只留在对话里** —— 下次没人知道，同一个点会被重复排查。
   - **每条结论都标「适用版本」**：写清是哪个 agent、哪个版本（上游升级后据此复核）。
   - **未核实的内容不得写成事实**：标注 🔴 未核实，并把核实任务挂进 [`docs/PROGRESS.md`](docs/PROGRESS.md) 未决项。
5. **文档与代码冲突时，以源码现状为准**：发现文档说法与实现不一致，先读源码确认「现在实际是什么」，按实现回改文档并向用户提示分歧；**不得凭文档猜实现**。

## 三、本项目的文档体系（细则一律见 [`docs/README.md`](docs/README.md)）

| 层 | 本项目落位 | 规矩 |
|---|---|---|
| 现场·进行中 | [`docs/PROGRESS.md`](docs/PROGRESS.md) | 只装在办的事：当前状态 / 未决项 / 下一步 |
| 现场·已结案 | [`docs/PROGRESS-HISTORY.md`](docs/PROGRESS-HISTORY.md) | 一行一条：时间 / 完成了什么 / 过程文档；不写过程 |
| 叙事 | `docs/worklog/<工作包名>.md` | 一个工作包一个文件；完成即封卷，此后不改 |
| 定型 | `docs/design/`（需求 + 架构 + `features/` 功能文档 + `external/` 外部事实） | 改它 = 改规矩 |
| 样例 | `docs/examples/` | 与代码零耦合 |

- **拍板结论写进归属文档**（需求决策进需求文档、架构决策进架构文档、功能决策进功能文档……），**不另建决策文件**。
- 目录、命名、每类「必须写 / 不得写」、生命周期、硬规则：**全在 [`docs/README.md`](docs/README.md)**，本文件不复述。
