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
- **会话钩子（几乎）只当「投递员」**：规则**正文**从不注入，文件是真正拷进 agent 原生加载的目录，
  因此享有静态前缀语义 —— 不会随对话老化淡出，也不会每轮累积。
  唯一刻意的例外：当钩子检测到规则**确有变化**时，会注入**一条瞬态提示**，请你新开会话（见 §5.3）。
- 底线：**不用软链**（一律真实拷贝）、**加删自由**（配置里删掉某源文件，下次同步它的副本即消失）。

> 为什么不直接用钩子注入？注入落在上下文的动态区，压缩时会被摘要掉（淡出），
> 或每轮追加（token 爆炸）。见 [`docs/design/architecture.md`](docs/design/architecture.md) §一。

---

## 2. 支持范围

rulemux **每个 agent 一套适配器**；只有「规则目录 + 钩子落点」经真实 canary 实测坐实的 agent
才允许安装。当前：

| Agent | 层级 | 规则目录 | 状态 |
|---|---|---|---|
| **codebuddy** | Tier-1（真实拷贝） | `.codebuddy/rules/` | ✅ **已验证，可安装**（`codebuddy-cn` 是它的别名） |
| **workbuddy** | Tier-1（真实拷贝） | `.workbuddy/rules/` | ✅ **已验证，可安装**（独立应用：自有 `~/.workbuddy/settings.json` 钩子，不再是 codebuddy 的别名） |
| claude（Claude Code） | Tier-1 | `.claude/rules/` | ⚠️ 已注册，**尚未验证**，不可安装 |
| trae（Trae） | Tier-1 | `.trae/rules/` | ⚠️ 已注册，**尚未验证**，不可安装 |
| codex | Tier-2（注入） | 无 —— 注入上下文 | ⚠️ 已注册，**尚未验证** |
| opencode | Tier-2（注入） | 无 —— 注入上下文 | ⚠️ 已注册，**尚未验证** |
| **dsh**（DeepSeek Harness） | Tier-1（真实拷贝） | `.dsh/rules/` | ✅ **已验证，可安装**；读取那半由**独立的 dsh 插件**做 —— 用 `dsh plugin --profile <p> add rulemux-dsh` 安装（rulemux 只负责同步，不替你装插件） |

- `rulemux init` **会拒绝**未验证的 agent —— 不会给你装一个行为未经验证的半成品。
  `rulemux doctor` 会给未验证的 agent 标 ⚠。
- **Tier-2 是刻意的降级**：面向没有规则目录、只认单个 `AGENTS.md` 的 agent，走会话钩子注入，
  且绝不碰你自己的 `AGENTS.md`；但它满足不了「永不淡出」。见
  [`docs/design/features/hook-injection.md`](docs/design/features/hook-injection.md)。
- **dsh 是「两半」的故事**：它没有钩子配置文件。rulemux 负责写 `.dsh/rules/`（Tier-1）；读取那半由本仓库里
  **独立的插件包** [`dsh-plugin/`](dsh-plugin/) 做，按 dsh 正常方式安装 —— **直接 git 装，无需发 npm**：
  `dsh plugin --profile web add "github:cq-guojia/rulemux#path:/dsh-plugin"`。
  所以 `rulemux init --agent dsh` 什么都不装 —— 只打印插件安装命令。
  装完后该插件会**在首次运行时自动就绪**（装完插件记得重启 dsh）：它不带任何 npm 依赖，缺 `rulemux`
  就自动装到全局，配置 `~/.rulemux/config.toml` 也替你生成 —— **不需要你再敲第二条命令**。
  CLI 是硬前提：拿不到它就**抛错**，绝不悄悄跑在陈旧规则上。

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

**升级后钩子会自己跟上当前格式——不依赖 npm 的安装脚本。** 钩子命令会随版本变化（0.2.0 起加了
`--hook`），所以升级后**首次**有 Agent 调起 `rulemux sync` 时，它会**只重写 rulemux 自己装过的那条**
SessionStart 钩子为当前格式 —— 别的条目、别的键一个字节都不动，也绝不创建任何文件。这段逻辑在二进制
里，因此**任何安装方式都生效**，也不需要 npm 放行任何安装脚本。想立刻手动做一次：

```bash
rulemux init --refresh   # 只刷新「我们自己装过的」钩子：不新增、不创建任何文件或目录
```

`rulemux doctor` 现在能报出「已安装但格式过期」，并给出上面这条修复命令。若你用
`CODEBUDDY_CONFIG_DIR` 把该 agent 的配置目录挪走过，rulemux 会跟随 —— 见
[`docs/design/external/agent-rules-dirs.md`](docs/design/external/agent-rules-dirs.md) §五。

---

## 4. 快速开始

```bash
# 1. 为指定 agent 安装会话钩子（--agent 必填）
rulemux init --agent workbuddy
#    首次运行还会生成示例配置 ~/.rulemux/config.toml

# 2. 编辑 ~/.rulemux/config.toml，把 path 改成你真实的规则文件

rulemux doctor    # 3. 自检：二进制/PATH、各 agent 状态、配置合法性
rulemux verify    # 4. canary 验收：投放探针，开新会话让 agent 念出暗号

# 此后每次开新会话，都会自动触发 rulemux sync --hook --agent codebuddy

rulemux sync --all   # 可选：一条命令把账本里记录过的所有工作区重新对齐

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

每次同步：算出「应该存在的带前缀文件集合」，目标目录里不在这个集合中的 `__rulemux__*` 一律删除（删残留），
缺失或内容不一致的复制/覆盖，其余跳过。

- 目标文件名：`__rulemux__` + 源 basename（同名冲突时追加短 hash）。
- **源文件只被读取** —— 绝不修改、绝不删除。
- **规则目录里不带 `__rulemux__` 前缀的文件绝不触碰**，你自己的规则文件始终安全。

> ⚠️ **规则改了，要等下一个会话生效 —— 而且 agent 会主动告诉你**
> SessionStart 钩子每次会话都会同步、并把文件真正拷到磁盘。但 harness 在会话**开始瞬间**就固定了规则快照
> （早于钩子执行），所以本次会话读到的仍是旧版，改动要**下一个**会话才会被加载。
> 钩子检测到规则**确有变化**（新增 / 修改 / 删除）时，会注入**一句话**请模型转告你「请新开会话」；
> **没有变化时它一个字节都不输出**。
>
> 想一次性把所有工作区都对齐？`rulemux sync --all` 会遍历账本（`~/.rulemux/workspaces.json`）里记录过的
> 每个工作区重新同步。它**绝不创建**规则目录（只有该 agent 自己的会话钩子才会创建），并顺手清掉账本里
> 「目录已经不存在」的条目。

---

## 6. 卸载

```bash
rulemux uninstall --agent codebuddy          # 单个 agent（逗号分隔可多个）
rulemux uninstall --off                      # 全部 agent（--all 等价）
rulemux uninstall --agent codebuddy --yes    # 跳过交互确认
```

它会移除 rulemux 自己的钩子条目（同一文件里其它工具的钩子会保留），并删掉此前投递的 `__rulemux__*` 文件。
由于钩子在 user 级配置里，rulemux 还会**按账本（~/.rulemux/workspaces.json）回访所有记录过的工作区**
一并清理 —— 否则钩子一去，那些残留就再没机会被删掉了。删除前会把要回访的工作区列表给你确认。

---

## 7. 命令一览

| 命令 | 作用 |
|---|---|
| `rulemux sync [--hook] [--agent <id>] [--config <path>] [--workspace <dir>]` | 同步规则。`--hook` = 本次由某 agent 的 SessionStart 钩子调起（**只有此时**才允许创建规则目录、才可能输出变化提示）；省略 `--agent` 则针对全部已支持 agent |
| `rulemux sync --all [--config <path>]` | 手动把账本里记录过的所有工作区重新对齐（不读 cwd、绝不创建目录、顺手 GC 掉目录已不在的条目）。与 `--hook` / `--agent` / `--workspace` 互斥 |
| `rulemux inject --agent <id> [--config <path>]` | Tier-2：把规则输出到 stdout 供钩子注入 |
| `rulemux init --agent <id[,id...]> [--config <path>] [--workspace <dir>]` | 生成示例配置并安装 SessionStart 钩子 |
| `rulemux init --refresh [--agent <id[,id...]>] [--workspace <dir>]` | **只刷新** rulemux 自己装过的钩子（升级后首次 `sync` 会自动做一次，这条用于手动强制）：不新增、不创建任何文件或目录、文件里其它条目与其它键一律原样保留 |
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

## 9. 发版（维护者）

```bash
npm version patch|minor|major   # 更新 package.json 并创建 git tag v0.x.y
git push --follow-tags
```

推送 tag 会触发 [`.github/workflows/release.yml`](.github/workflows/release.yml)：
自动交叉编译全部平台二进制、挂到 GitHub Release、并按 tag 版本发布 npm 包。
需在仓库 Secrets 里配置 `NPM_TOKEN` 才会发布 npm（未配置时跳过并告警）。
普通的 CI（[`ci.yml`](.github/workflows/ci.yml)）在每次 push 时跑 build/vet/测试/冒烟。

---

## 10. 许可证

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
