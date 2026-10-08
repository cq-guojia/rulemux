# rulemux

> 一份规则，投递到各家 AI coding agent。
> 效果与 token 与直接写 `AGENTS.md` 一致：**永不淡出、不累积、不用软链、加删文件自由**。

[English](README.md) | [设计文档](docs/design/architecture.md)

---

## 1. 这是什么

你维护着一套规则文档（编码约定、项目规矩……），希望每个 AI coding agent 在每个工作区都读到它，
且效果和直接写进 `AGENTS.md` 一模一样。

`rulemux` 把你在配置里列出的源文件，复制进各 agent **原生的工作区规则目录** —— 一处管理，多处生效。

- rulemux **不持有、不维护任何规则内容**。源文件完全由你维护，rulemux 只负责复制；
  「适用于哪些 agent / 哪些工作区」在你的配置里声明。
- **会话钩子只当「投递员」，不用来注入文本**。文件是真正拷进 agent 原生加载的目录，
  因此享有静态前缀语义 —— 不会随对话老化淡出，也不会每轮累积。
- 底线：**不用软链**（一律真实拷贝）、**加删自由**（配置里删掉某源文件，下次同步它的副本即消失）。

> 为什么不直接用钩子注入？注入落在上下文的动态区，压缩时会被摘要掉（淡出），
> 或每轮追加（token 爆炸）。见 [`docs/design/architecture.md`](docs/design/architecture.md) §一。

---

## 2. 支持范围

rulemux **每个 agent 一套适配器**；只有「规则目录 + 钩子落点」经真实 canary 实测坐实的 agent
才允许安装。当前：

| Agent | 层级 | 规则目录 | 状态 |
|---|---|---|---|
| **codebuddy** | Tier-1（真实拷贝） | `.codebuddy/rules/` | ✅ **已验证，可安装** |
| **workbuddy** | Tier-1（真实拷贝） | `.codebuddy/rules/`（与 CodeBuddy 共用） | ✅ **已验证，可安装** |
| claude（Claude Code） | Tier-1 | `.claude/rules/` | ⚠️ 已注册，**尚未验证**，不可安装 |
| trae（Trae） | Tier-1 | `.trae/rules/` | ⚠️ 已注册，**尚未验证**，不可安装 |
| codex | Tier-2（注入） | 无 —— 注入上下文 | ⚠️ 已注册，**尚未验证** |
| opencode | Tier-2（注入） | 无 —— 注入上下文 | ⚠️ 已注册，**尚未验证** |

- `rulemux init` **会拒绝**未验证的 agent —— 不会给你装一个行为未经验证的半成品。
  `rulemux doctor` 会给未验证的 agent 标 ⚠。
- **Tier-2 是刻意的降级**：面向没有规则目录、只认单个 `AGENTS.md` 的 agent，走会话钩子注入，
  且绝不碰你自己的 `AGENTS.md`；但它满足不了「永不淡出」。见
  [`docs/design/features/hook-injection.md`](docs/design/features/hook-injection.md)。

---

## 3. 安装

rulemux 是**零第三方依赖的 Go 单二进制**。

```bash
# a) 从 GitHub Release 取预编译产物（推荐），放到 PATH
#    例如 /usr/local/bin/rulemux（Windows 用 rulemux.exe）

# b) 从源码
go install github.com/cq-guojia/rulemux@latest
# 或
git clone https://github.com/cq-guojia/rulemux.git && cd rulemux && go build -o rulemux .

# c) 通过 npm（把二进制放进 PATH 的便捷通道）
npm i -g rulemux
```

**没有自动更新**。升级手动、交给包管理器（`go install …@latest`，将来 Homebrew / Scoop / apt）。
这是刻意决策，见 [`docs/design/requirements.md`](docs/design/requirements.md) §五。

---

## 4. 快速开始

```bash
# 1. 为指定 agent 安装会话钩子（--agent 必填）
rulemux init --agent codebuddy
#    首次运行还会生成示例配置 ~/.rulemux/config.toml

# 2. 编辑 ~/.rulemux/config.toml，把 path 改成你真实的规则文件

rulemux doctor    # 3. 自检：二进制/PATH、各 agent 状态、配置合法性
rulemux verify    # 4. canary 验收：投放探针，开新会话让 agent 念出暗号

# 此后每次开新会话，都会自动触发 rulemux sync --agent codebuddy

rulemux uninstall --agent codebuddy   # 卸载（--yes 跳过确认）
```

**钩子装在哪**：装进 **user 级 host 配置** `~/.codebuddy/settings.json`（与 Hindsight 的落点同类）。
**一次安装、对所有工作区生效**，不必每个项目再跑一次 `init`。触发时 rulemux 以当前工作区（cwd）
去匹配配置里的 `workspace` 条目，决定投递哪些规则。

---

## 5. 配置说明

配置文件：`~/.rulemux/config.toml`（各命令均可用 `--config <path>` 指定其它路径）。

### 5.1 每条 `[[source]]`

| 字段 | 含义 |
|---|---|
| `path` | 源文件，磁盘任意位置；单个值或数组。数组内文件共享下面的 `agents` / `workspace` |
| `agents` | 投递给哪些 agent；省略 = 全部已支持的 agent |
| `workspace` | 适用哪些工作区；省略 / `"*"` / `"**"` / `"all"` = 所有工作区；单个值、数组或 glob |

glob 沿用业界惯例：`*` 匹配单个路径段，`**` 跨段递归且可出现在中间 ——
例如 `workspace = "/abs/**/B"` 匹配任意深度下名为 `B` 的目录。

```toml
[[source]]
path = ["/你的规则/a.md", "/你的规则/b.md"]
agents = ["codebuddy"]              # 省略则投递给全部已支持 agent
# workspace 省略 => 所有工作区
# workspace = ["/path/to/proj-a", "/path/to/proj-b"]
# workspace = "/abs/**/B"           # glob：任意深度、名为 B
```

### 5.2 用「组」打包（可选）

文件多了之后，可以打包成组，按组名引用。

```toml
# ---- 文件组 --------------------------------------------------------------
[[file_group]]
name  = "base"
path  = ["/你的规则/team-conventions.md", "/你的规则/style.md"]

[[file_group]]
name = "proj"
use  = ["base"]                     # 嵌套引用 base 组（数组：["base", "dev"]）
path = ["/你的规则/project-a.md"]

# ---- 工作区分组 ----------------------------------------------------------
[[workspace_group]]
name      = "dev"
workspace = ["/path/to/proj-1", "/path/to/proj-2"]

# ---- 引用它们 ------------------------------------------------------------
[[source]]
groups           = ["base", "proj"]        # 展开成这些组里的所有文件
path             = ["/你的规则/extra.md"]   # 允许混合：组 + 单独文件
agents           = ["codebuddy"]
workspace_groups = ["dev"]                 # 展开成组内的所有工作区
workspace        = ["/path/to/standalone"] # 也可与 workspace_groups 同时写
```

要点：

- **`use` 是数组** —— `use = ["dev", "qa"]` 一次复用多个组，且支持嵌套（A 组引入 B 组再加自己的文件）。
- **允许混合书写**：同一条 source 里 `groups` 可与 `path` 并存，`workspace_groups` 可与 `workspace` 并存，结果取并集。
- **重复无害** —— 最终按值去重，每个文件只处理一次。
- 文件组与工作区分组是**两套独立命名空间**，允许同名。

### 5.3 同步做了什么，以及绝不碰什么

每次同步：算出「应该存在的带前缀文件集合」，目标目录里不在这个集合中的 `.rulemux__*` 一律删除（删残留），
缺失或内容不一致的复制/覆盖，其余跳过。

- 目标文件名：`.rulemux__` + 源 basename（同名冲突时追加短 hash）。
- **源文件只被读取** —— 绝不修改、绝不删除。
- **规则目录里不带 `.rulemux__` 前缀的文件绝不触碰**，你自己的规则文件始终安全。

---

## 6. 卸载

```bash
rulemux uninstall --agent codebuddy          # 单个 agent（逗号分隔可多个）
rulemux uninstall --off                      # 全部 agent（--all 等价）
rulemux uninstall --agent codebuddy --yes    # 跳过交互确认
```

它会移除 rulemux 自己的钩子条目（同一文件里其它工具的钩子会保留），并删掉此前投递的 `.rulemux__*` 文件。
由于钩子在 user 级配置里，rulemux 还会**按账本（~/.rulemux/workspaces.json）回访所有记录过的工作区**
一并清理 —— 否则钩子一去，那些残留就再没机会被删掉了。删除前会把要回访的工作区列表给你确认。

---

## 7. 命令一览

| 命令 | 作用 |
|---|---|
| `rulemux sync [--agent <id>] [--config <path>] [--workspace <dir>]` | 同步规则（由各 agent 的 SessionStart 钩子调用） |
| `rulemux inject --agent <id> [--config <path>]` | Tier-2：把规则输出到 stdout 供钩子注入 |
| `rulemux init --agent <id[,id...]> [--config <path>] [--workspace <dir>]` | 生成示例配置并安装 SessionStart 钩子 |
| `rulemux doctor [--config <path>] [--workspace <dir>]` | 环境自检 |
| `rulemux verify --agent <id> [--clean] [--workspace <dir>]` | canary 验收 |
| `rulemux uninstall --agent <id[,id...]> \| --off \| --all [--yes]` | 移除钩子与已投递文件 |

直接运行 `rulemux`（不带参数）可看完整帮助。

---

## 8. 后续计划

在办事项见 [`docs/PROGRESS.md`](docs/PROGRESS.md)。概要：

- **接入更多 agent** —— 补齐已注册但尚未验证的适配器：
  - **Trae**（`.trae/rules/`）、**Claude Code**（`.claude/rules/`）走 Tier-1；
  - **Codex**、**OpenCode** 走 Tier-2 注入。
  每个 agent 都要先经真实 canary 坐实「规则目录 + 钩子落点」才开放。
  接入一个新 agent 的完整清单见
  [`docs/design/features/agent-onboarding.md`](docs/design/features/agent-onboarding.md)。
- **分发** —— GitHub Actions 交叉编译 + Release 产物（同时供 npm 包使用），并正式发布 npm 包。
- **验证工具** —— 让 canary 验收对每个 agent 可重复执行。
- 规则内容改写/模板渲染、以及自动自我更新，**明确不做**
  （[`docs/design/requirements.md`](docs/design/requirements.md) §五）。

---

## 9. 许可证

MIT —— 见 [LICENSE](LICENSE)。

---

## 10. 文档索引

| 想看什么 | 去哪 |
|---|---|
| 用户到底要什么（需求 / 验收标准） | [`docs/design/requirements.md`](docs/design/requirements.md) |
| 为什么这么设计 | [`docs/design/architecture.md`](docs/design/architecture.md) |
| 功能列表 | [`docs/design/features.md`](docs/design/features.md) |
| 接入新 agent 的清单 | [`docs/design/features/agent-onboarding.md`](docs/design/features/agent-onboarding.md) |
| 各家 agent 的规则目录（外部事实） | [`docs/design/external/agent-rules-dirs.md`](docs/design/external/agent-rules-dirs.md) |
| 当前进度与未决项 | [`docs/PROGRESS.md`](docs/PROGRESS.md) |
