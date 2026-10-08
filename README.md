# rulemux

> 你维护的一套规则文档 → 同步投递到各家 AI coding agent（Claude Code / Trae / CodeBuddy / WorkBuddy / Codex / OpenCode / DeepSeek Harness）。
> 效果与 token 与直接写 `AGENTS.md` 一致：**永不淡出、不累积、不用软链、加删文件自由**。

## 这是什么

你维护着一套规则文档（编码约定、项目规矩……），希望每个 AI coding agent 在每个工作区都读到它，且效果和直接写进 `AGENTS.md` 一模一样。
`rulemux` 负责把你在配置里列出的规则文件同步进各家 agent 的工作区规则目录 —— 一处管理，多处生效。（rulemux 本身不持有、不维护任何规则内容，源文件完全由你维护；「适用于哪些 agent / 哪些工作区」在配置里声明。）

**核心做法**：hook 只当「投递员」，把规则文档**真实拷贝**进各 agent 原生加载的规则目录，由 harness 自己读 ⇒ 享有静态前缀语义，因此永不淡出、不累积。
（为什么不靠 hook 注入：见 [`docs/design/architecture.md`](docs/design/architecture.md) §二）

## 安装

`rulemux` 是零依赖的 Go 单二进制，跨平台交叉编译可得。

- **下载预编译二进制**（推荐）：从 GitHub Release 取对应平台产物，放到 PATH（如 `/usr/local/bin/rulemux`）；Windows 用 `rulemux.exe`（hook 的 exec 形式才能直接 spawn）。
- **从源码编译**：
  ```bash
  git clone <repo> && cd rulemux
  go build -o rulemux .                              # 当前平台
  GOOS=darwin  GOARCH=arm64 go build -o rulemux-darwin-arm64 .
  GOOS=windows GOARCH=amd64 go build -o rulemux.exe .
  ```
- （可选）`npm i -g rulemux` 仅作为把二进制放进 PATH 的便捷通道，包内不含运行时。

## 用法

```bash
rulemux init --agent codebuddy   # 1. 为指定 agent 安装 SessionStart 钩子（--agent 必填），首次会生成示例 config.toml
# 2. 编辑 ~/.rulemux/config.toml，把 path 改成自己真实的规则文件
rulemux doctor                   # 3. 环境自检：二进制/PATH、各 agent 目录与钩子状态、配置合法性
rulemux verify                   # 4. canary 验收：开新会话问 agent 能否念出暗号 RULEMUX-CANARY-43371345
# 5. 之后每次开新会话，钩子自动触发 rulemux sync --agent X

# 卸载（--agent 与 --off/--all 二选一、必带其一；执行前会交互确认，--yes 跳过）
rulemux uninstall --agent codex            # 卸单个 agent（支持逗号多个：--agent codebuddy,codex）
rulemux uninstall --off                    # 卸全部 agent（--all 等价）
```

> ⚠️ **`init` 必须指定 `--agent`**：rulemux 不会扫描你机器上装了哪些 agent，你得明确说要装哪个（可逗号分隔多个，如 `--agent codebuddy,codex`）。
> ⚠️ **钩子按工作区安装**：`rulemux init` 只给当前工作区装钩子；换工作区须在该工作区再跑一次 `init`，否则那个工作区不会触发同步（曾出现「A 工作区能读到、B 读不到」即因此）。`uninstall` 同理要进同一个工作区执行。

配置（`~/.rulemux/config.toml`）示例：

```toml
[[source]]
path = ["/你的规则/a.md", "/你的规则/b.md"]
agents = ["claude", "codex"]     # 省略 = 全部 agent
# workspace 省略 / "*" / "all" = 所有工作区；也可写数组限定特定工作区
# workspace = ["/path/to/proj-a", "/path/to/proj-b"]

[[source]]
path = "/笔记/c.txt"
agents = ["trae"]
workspace = ["/path/to/proj-a"]
```

同步算法（每次 `sync`）：计算「应生成的带前缀文件名集合 S」→ 目标目录里不在 S 中的 `.rulemux__*` 删掉（删残留）→ S 中缺失/不一致则复制/覆盖，一致则跳过；非 `.rulemux__` 前缀的用户文件一律不碰。

## 文档

| 想看 | 去哪 |
|---|---|
| 现在做到哪、欠什么 | [`docs/PROGRESS.md`](docs/PROGRESS.md) |
| 文档怎么摆、怎么写 | [`docs/README.md`](docs/README.md) |
| **用户要什么**（需求 / 验收标准） | [`docs/design/requirements.md`](docs/design/requirements.md) |
| 设计（核心原理 / 选型 / 命名） | [`docs/design/architecture.md`](docs/design/architecture.md) |
| 功能列表 | [`docs/design/features.md`](docs/design/features.md) |
| 各家 agent 的规则目录 | [`docs/design/external/agent-rules-dirs.md`](docs/design/external/agent-rules-dirs.md) |
