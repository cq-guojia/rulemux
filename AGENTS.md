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
| 某个功能当时怎么做的、踩过什么坑 | `docs/worklog/`（由 koc skill 建档归档） |

- 目录、命名、每类「必须写 / 不得写」、生命周期等通用规矩由**注入的工作区级规则**承载，本文件不复述；本项目槽位落点查 [`docs/README.md`](docs/README.md)，特有硬约束见下文第 4 条。

## 二、本项目独有的约定

> 以下四项现在均适用。

1. **构建与提交**：Go 单二进制（模块 `github.com/cq-guojia/rulemux`）；代码变更提交前 `go build ./...`、`go vet ./...`、`go test ./...` 须全绿，发布类改动另跑 `scripts/smoke.sh` 冒烟；提交与推送节奏以注入规则为准，本文件不复述。
2. **验证方式**：`go build ./...` / `go vet ./...` / `go test ./...` / `scripts/smoke.sh`（端到端冒烟）；agent 规则是否真生效的 canary 验证见 [`docs/design/features/verification.md`](docs/design/features/verification.md)。
3. **远端与推送**：远端为 GitHub `git@github.com:cq-guojia/rulemux.git`，打 tag 触发 release 流水线；发布方式定稿见 [`docs/design/implementation.md`](docs/design/implementation.md)。网络层与推送口径以注入规则为准，本文件不述。
4. **凡涉及各家 agent 的规则目录 / hooks 配置，一律「先读源码与文档，再动手」，禁止靠运行时试探猜**：
   - **要读到实现本体**（目录在哪、扩展名、读不读全部、frontmatter 语义），不是只看文档描述；结论带出处（`文件:行号` 或包版本）。
   - **读到的结论必须回写**（这一步最容易被漏）：写进 [`docs/design/external/`](docs/design/external/) —— 已有文档就补充或更正，**没有**就新建一篇（头部按外部事实类规矩写：类型 / 适用版本 / 状态 / 来源 / 配套）；过程另记 `docs/worklog/`（由 koc skill 建档）。**不许只留在对话里** —— 下次没人知道，同一个点会被重复排查。
   - **每条结论都标「适用版本」**：写清是哪个 agent、哪个版本（上游升级后据此复核）。
   - **未核实的内容不得写成事实**：标注 🔴 未核实，并把核实任务挂进 [`docs/progress.json`](docs/progress.json) 未决项。
5. **CLI 与 dsh 插件是两条独立发布线，必须配对推进**（否则用户机器上的 CLI 不会自己往前走）：
   - 两条线**互不触发**：CLI 打 `vX.Y.Z` tag（`publish.yml` → 发 npm `rulemux`）；插件打 `dsh-vX.Y.Z` tag（`dsh-publish.yml` → 发 npm `rulemux-dsh`）。**打 `v*` tag 不会带上插件**。
   - 插件是否把机器上的 CLI 升到最新，只由**插件内置的 `REQUIRED_CLI` 下限**决定（`dsh-plugin/index.mjs`）：CLI 低于下限 → 插件自动装 `rulemux@latest`；CLI 已达下限 → 插件不动它（**不追最新**）。这段判定写死在插件里，**CLI 侧改不了它**。
   - 所以：**要让用户的 CLI 跟到某个新版本（即抬高过 `REQUIRED_CLI`），就必须同步发一个 `dsh-vX.Y.Z`**；只发 CLI、不发插件，已装旧插件的机器不会被推动。凡改动「CLI 获取 / 版本判定」逻辑，同样必须发插件版本。
