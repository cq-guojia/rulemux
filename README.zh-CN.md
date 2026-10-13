# rulemux

> 一套规则，**一个真源**：一处修改，同步到**所有 agent 的所有工作区**。
> 效果与 token 与直接写 `AGENTS.md` 一致：**永不淡出、不累积、不用软链、加删文件自由**。

[English](README.md) | [设计文档](docs/design/architecture.md)

---

## 1. 这是什么

### 1.1 先说痛点

- **每家 agent 的规则目录都不一样**：`.codebuddy/rules/`、`.claude/rules/`、`.dsh/rules/`……
  同一条编码约定，要按各家的方式抄 N 遍。
- **每个工作区又各有一份**：想让规则在哪个工作区生效，就得去那个工作区再写一遍。
- 结果就是：那些**真正通用的规则**（团队约定、编码规范）既抽不出来，也管不动 ——
  改一句话，要打开「agent 数 × 工作区数」个文件，改完还不敢保证哪份漏了。

### 1.2 rulemux 的做法

你只维护**唯一的一份真源**：一组自己写的规则文档，在 `~/.rulemux/config.toml` 里按下面这棵树
声明。`rulemux sync` 把它们**真实拷进每个 agent 的原生规则目录** —— 上层规则被下层继承复用，
**改一处，所有 agent、所有工作区下一次同步就全变了**。

```
唯一真源（你自己写的 .md，在 ~/.rulemux/config.toml 里声明）
│
├─ 第 1 层 · 统一规则 ─────────────► 所有 agent × 所有工作区
│   │
│   ├─ 第 2 层 · 开发类规则
│   │   ├─ 第 3 层 · 小程序开发规则
│   │   │   └─ …
│   │   ├─ 第 3 层 · 后台开发规则
│   │   │   └─ …
│   │   └─ 第 3 层 · C 端开发规则
│   │       └─ …
│   │
│   └─ 第 2 层 · 非开发类规则
│       └─ 第 3 层 · 工作区日常规则 …
│           └─ …
│
└─ 第 4 层 · 某个工作区自己的规则 ──► 只在这个工作区生效

              rulemux sync  ↓  真实拷贝（永不淡出 / 不累积 / 不用软链）

    工作区 A ──► .codebuddy/rules/   .workbuddy/rules/   .dsh/rules/
    工作区 B ──► .codebuddy/rules/   .workbuddy/rules/   .dsh/rules/
```

- **规则复用**：第 3 层继承第 2 层、第 2 层继承第 1 层 —— 通用的只写一次，越往下越具体。
- **层级下放**：每一层各自声明「给哪些 agent、哪些工作区」，不用为每个组合复制一份文件。
- **改一处，处处生效**：上层一句话改动，所有继承它的工作区与 agent 同步即到位。
- **层数不限**：第 3 层下面那个 `…` 是垫层 —— `use` 是**递归展开**的，想套多少层就套多少层；
  唯一不允许的是**成环**（A 引 B、B 又引 A），配置一加载就报错。

搭这棵树用的就是 `[[file_group]]` 的 `use`（继承上层）+ `[[source]]` 的 `workspace`（限定落到哪）：

```toml
[[file_group]]
name = "base"                    # 第 1 层：统一规则
path = ["/rules/00-base.md"]

[[file_group]]
name = "dev"                     # 第 2 层：开发类
use  = ["base"]                  # 继承第 1 层
path = ["/rules/10-dev.md"]

[[file_group]]
name = "miniapp"                 # 第 3 层：小程序开发
use  = ["dev"]                   # 继承第 2 层（也就含了第 1 层）
path = ["/rules/20-miniapp.md"]

[[source]]
groups    = ["miniapp"]          # 落地 = base + dev + miniapp
workspace = ["/work/miniapp-a", "/work/miniapp-b"]

[[source]]
path      = ["/rules/proj-x-only.md"]   # 第 4 层：只给这一个工作区
workspace = ["/work/proj-x"]
```

完整的分组与匹配写法见 §5.2。

### 1.3 几条不变的规矩

- **rulemux 不持有、不维护任何规则内容**：源文件完全由你写，rulemux 只负责投递；
  「给哪些 agent / 哪些工作区」在你的配置里声明。
- **会话钩子（几乎）只当投递员**：规则正文从不注入，文件是真正拷进 agent 原生加载的目录，
  因此享有静态前缀语义 —— 不淡出、不累积。唯一刻意的例外：检测到规则确有变化时，注入
  **一条瞬态提示**请你新开会话（见 §5.3）。
- **不用软链、加删自由**：一律真实拷贝；配置里删掉某源文件，下次同步它的副本即消失。

> 为什么不直接用钩子注入？注入落在上下文的动态区，压缩时会被摘要掉（淡出），
> 或每轮追加（token 爆炸）。见 [`docs/design/architecture.md`](docs/design/architecture.md) §一。

---

## 2. 支持范围

rulemux **每个 agent 一套适配器**；只有「规则目录 + 钩子落点」经真实 canary 实测坐实的 agent
才允许安装。当前：

| Agent | 层级 | 规则目录 | skills 目录 | 状态 |
|---|---|---|---|---|
| **codebuddy** | Tier-1（真实拷贝） | `.codebuddy/rules/`；用户级 `~/.codebuddy/rules/` | `.codebuddy/skills/`；用户级 `~/.codebuddy/skills/` + `~/.workbuddy/skills/`（双写） | ✅ **已验证，可安装**（`codebuddy-cn` 是它的别名） |
| **workbuddy** | Tier-1（真实拷贝） | `.codebuddy/rules/`（与 codebuddy **共享**，2026-10-09 实测坐实） | `.codebuddy/skills/`（与 codebuddy 共享）；用户级同样双写两家 | ✅ **已验证，可安装**（独立应用：自有 `~/.workbuddy/settings.json` 钩子，不再是 codebuddy 的别名） |
| claude（Claude Code） | Tier-1 | `.claude/rules/` | 暂不支持 | ⚠️ 已注册，**尚未验证**，不可安装 |
| **trae**（Trae CN） | Tier-1（真实拷贝） | `.trae/rules/` | 暂不支持 | ✅ **已验证，可安装**（2026-10-09 canary 坐实：用户级钩子 `~/.trae-cn/hooks.json`；国际版目录 `~/.trae` 尚未接入） |
| codex | Tier-2（注入） | 无 —— 注入上下文 | 暂不支持 | ⚠️ 已注册，**尚未验证** |
| opencode | Tier-2（注入） | 无 —— 注入上下文 | 暂不支持 | ⚠️ 已注册，**尚未验证** |
| **dsh**（DeepSeek Harness） | Tier-1（真实拷贝） | `.dsh/rules/` | 暂不支持 | ✅ **已验证，可安装**；读取那半由**独立的 dsh 插件**做 —— 用 `dsh plugin --profile <p> add rulemux-dsh` 安装（rulemux 只负责同步，不替你装插件） |

- `rulemux init` **会拒绝**未验证的 agent —— 不会给你装一个行为未经验证的半成品。
  `rulemux doctor` 会给未验证的 agent 标 ⚠。
- **skill 注入（2026-10-12 上线，仅 codebuddy / workbuddy）**：用 `[[skill_source]]` 声明本地 skill 目录，
  真实拷贝进 `.codebuddy/skills/`（`__rulemux__<skill名>/` 前缀，你自己的 skill 一律不碰）；
  `scope = "user"` 时落到**用户级（全局）**目录，规则与 skill 通用，详见 §5.4。
  规则与 skill 都支持双域（工作区 / 用户级）；trae / dsh / claude / codex / opencode 的 skills 暂不支持。
- **Tier-2 是刻意的降级**：面向没有规则目录、只认单个 `AGENTS.md` 的 agent，走会话钩子注入，
  且绝不碰你自己的 `AGENTS.md`；但它满足不了「永不淡出」。见
  [`docs/design/features/hook-injection.md`](docs/design/features/hook-injection.md)。
- **dsh 是「两半」的故事**：它没有钩子配置文件。rulemux 负责写 `.dsh/rules/`（Tier-1）；读取那半由本仓库里
  **独立的插件包** [`dsh-plugin/`](dsh-plugin/) 做，按 dsh 正常方式安装（npm 上就是 `rulemux-dsh`）：
  `dsh plugin --profile web add rulemux-dsh`（也可 git 直装：
  `add "github:cq-guojia/rulemux#path:/dsh-plugin"`）。
  所以 `rulemux init --agent dsh` 什么都不装 —— 只打印插件安装命令。
  装完后该插件会**在 dsh 启动时（插件装载）自动就绪**（装完插件记得重启 dsh）。就绪是**三步，必须全部成立**：
  `rulemux` 命令行工具可得**且版本满足插件要求**（偏旧会用 pnpm/npm 升级并复查，不会「装了就当成过」）、
  插件本身已装载、`~/.rulemux/config.toml` 存在（缺失则替你生成，**已存在绝不覆盖**）。
  任何一步不成立就**抛错**，让失败在会话里可见，绝不悄悄跑在没有规则的状态下。
  因为发生在启动时，你**去改配置时那份 `config.toml` 已经在**：装插件 → 重启 → 改配置 → 开**一次**会话即可。
  同步是另一回事：`rulemux sync` 失败只记日志，会话照常继续。

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

### 3.1 dsh：装的是插件，不是钩子

其余 agent 都是「`rulemux init --agent <id>` 装一条会话钩子」，**dsh 例外**：它没有钩子配置文件，
所以 `rulemux init --agent dsh` 什么都不装 —— 只把下面这条命令打印给你。规则由插件那半读：

```bash
dsh plugin --profile <你的 profile> add rulemux-dsh        # npm
# 想试未发布的改动，也可以从 git 直装：
dsh plugin --profile <你的 profile> add "github:cq-guojia/rulemux#path:/dsh-plugin"
```

**装完要重启 dsh**：插件在 dsh 启动时就绪 —— 检查 `rulemux` 命令行工具（版本不够会自动升级并复查）、
并在 `~/.rulemux/config.toml` 缺失时替你生成（已存在绝不覆盖）。三步缺一步就抛错，不会悄悄跑在
没有规则的会话里。因为就绪发生在启动时，你去改配置时那份文件已经在了：装插件 → 重启 → 改配置 →
开**一次**会话。

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
| `scope` | 落到哪个域：`"workspace"`（默认）= 各工作区的 agent 目录；`"user"` = 该 agent 的**用户级（全局）**目录，所有工作区共享。`scope = "user"` 时 `workspace` 被忽略 |

glob 沿用业界惯例：`*` 匹配单个路径段，`**` 跨段递归且可出现在中间 ——
例如 `workspace = "/abs/**/B"` 匹配任意深度下名为 `B` 的目录。

```toml
[[source]]
path = ["/你的规则/a.md", "/你的规则/b.md"]
agents = ["codebuddy"]              # 省略则投递给全部已支持 agent
# workspace 省略 => 所有工作区
# workspace = ["/path/to/proj-a", "/path/to/proj-b"]
# workspace = "/abs/**/B"           # glob：任意深度、名为 B
# scope = "user"                    # 改投到该 agent 的用户级（全局）目录
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
- **层数没有上限** —— `use` 递归展开（文件组、工作区分组都是），想套多少层就套多少层；
  唯一不允许的是**成环**（A 引 B、B 又引 A），配置加载时直接报错。
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

### 5.4 skill 注入（`[[skill_source]]`，2026-10-12 上线）

skill 是**目录**（`SKILL.md` + 可选的 `references/` / `scripts/`），宿主按「渐进式披露」加载
（会话开始只载 name+description，按名调用才读全文）—— 所以它和规则一样只能**真实拷贝**进原生 skills 目录，
不能靠钩子注入全文。在 `~/.rulemux/config.toml` 里加 `[[skill_source]]`：

```toml
[[skill_source]]
path = ["~/my-skills"]              # skill 根目录：每个含 SKILL.md 的子目录就是一个 skill
agents = ["codebuddy"]              # 省略 = 全部支持的 agent
workspace = "*"                     # 省略 = 全部工作区（仅 workspace 域生效）
# groups = ["frontend"]             # 改用 [[skill_group]] 组，省得重复写 path
# workspace_groups = ["dev"]        # 引用 [[workspace_group]] 组（与规则共用）

[[skill_source]]
groups = ["frontend"]               # 把多个 skill 根目录打包成一个名字来引用
agents = ["codebuddy"]
scope = "user"                      # 用户级（全局）：与工作区无关，每个工作区都生效

[[skill_group]]                     # 打包 skill 根目录，供多条 [[skill_source]] 复用
name = "frontend"
path = ["~/react-skills", "~/css-skills"]
# use = ["base"]                    # 嵌套引用另一个 skill 组（数组，可多个）
```

- **skill 源字段与规则一致**：`path`、`agents`、`workspace`、`scope`，外加两个打包键
  `groups`（引用 `[[skill_group]]`）与 `workspace_groups`（引用 `[[workspace_group]]`）。
  `[[skill_group]]` 打包多个 skill 根目录（`name` / `path` / `use`），嵌套与成环检测与文件组完全相同。
- **落到哪**（仅 codebuddy / workbuddy）：工作区级 → `<工作区>/.codebuddy/skills/`（两家共享）；
  用户级 → `~/.codebuddy/skills/` **和** `~/.workbuddy/skills/`（都写，两家才都看得到）。
  规则侧同理：`[[source]]` 也支持 `scope = "user"`，落到 `~/.codebuddy/rules/` + `~/.workbuddy/rules/`。
- **纪律与规则完全相同**：目标名 `__rulemux__<skill名>/`；你自己的 skill（无前缀）一律不碰；
  配置里删掉源，下一轮同步清掉对应副本；源目录只读。
- **第一版只支持本地目录源**；远程 / git 源（跟进上游更新）在后续版本。
- trae / dsh / claude / codex / opencode 的 skills 暂不支持（它们的 skills 目录尚未实测坐实）。
- skill 的增改**不会**触发「请新开会话」提示（是否热重载待实测）；规则的提示行为不变。

### 5.5 完整的默认配置（带注释）

`rulemux init` 首次运行会把这份文件写到 `~/.rulemux/config.toml`。**全部是注释掉的** —— 在你取消
注释、或加入自己带真实路径的配置之前，什么都不会同步。这里与生成的文件一一对应地列出，方便你在
动手改自己的那份之前先通读一遍。

```toml
# rulemux 配置文件
#
# 默认位置：~/.rulemux/config.toml（任何命令都可用 --config <path> 指定别的路径）
#
# 每次 agent 会话开始时，rulemux 会把下面每条 [[source]] 描述的「源规则文件」拷贝进该
# agent 的原生规则目录，文件名带 __rulemux__ 前缀：它只清理自己的残留，绝不碰你自己的文件。
#
# 每条 [[source]] 接受以下字段（都可选，各有默认值）：
#
#   path      源文件，可放在磁盘任意位置。两种写法都行：
#               path = "/abs/path/to/your-rules.md"          # 单个文件
#               path = ["/abs/path/a.md", "/abs/path/b.md"]  # 数组（共享下面的 agents/workspace）
#             开头的 "~" 或 "~/" 会展开成你的用户目录，因此同一份配置
#             在每台机器上都可用，不必硬编码用户名：
#               path = "~/Agent.Workspace/00.RULES/base.md"
#
#   agents    这批规则投给哪些 agent；省略 = 全部「已支持」的 agent。
#             当前已验证（可安装）的取值：
#               workbuddy   Tier-1（自有目录 .workbuddy/rules；自有钩子 ~/.workbuddy/settings.json）—— WorkBuddy 用这个
#               codebuddy   Tier-1（.codebuddy/rules；~/.codebuddy/settings.json）
#               trae        Tier-1（.trae/rules；CN ~/.trae-cn/hooks.json，国际版 ~/.trae）—— 2026-10-09 验证
#             它们是各自独立的 agent，各有自己的配置目录与钩子文件；
#             你实际用哪个就装哪个（也可都装）。rulemux 会记住你选的名字，之后每次刷新都沿用。
#             其它 agent（claude / codex / opencode）尚未通过 canary 验证，暂时不能安装或指定；
#             等它们的适配器被证明后会自动开放 —— 这里无需改动。
#             示例：agents = ["workbuddy", "trae"]
#
#   workspace 这些规则适用于哪些工作区。写法：
#               workspace = "/abs/path/to/proj"          # 单个工作区（精确匹配）
#               workspace = ["/abs/path/proj-a", "/b"]   # 数组：命中任一个即可
#               workspace = "*"  /  "**"  /  "all"       # 所有工作区（全局通配）
#             支持标准 glob："*" 匹配单段，"**" 跨段递归且可出现在中间，如：
#               workspace = "/abs/**/B"                 # /abs 下任意深度、名为 B
#               workspace = "**/B"                      # 任意位置或层级的 B
#             这里的开头 "~" / "~/" 同样展开成用户目录，且与 glob 兼容：
#               workspace = "~/Agent.Workspace/*"
#
#   scope     这批规则落到哪（可选，默认 "workspace"）：
#               scope = "workspace"  # 各工作区的 agent 目录（如 <ws>/.codebuddy/rules）
#               scope = "user"       # 该 agent 的「用户级（全局）」目录，所有工作区共享 ——
#                                   # 例如 CodeBuddy 家族的 ~/.codebuddy/rules 与
#                                   # ~/.workbuddy/rules（两个都写，两家应用才都看得到）；
#                                   # 用户级条目忽略 workspace 匹配：全局规则本就与工作区无关。
#
# --- Skills（[[skill_source]]）----------------------------------------------------
# 除了规则文件，rulemux 还能把 SKILL 目录同步进各 agent 的原生 skills 目录。一个 skill 是一个
# 「目录」（含 SKILL.md，外加可选的 references/scripts）；宿主按渐进式披露加载
# （会话开始只载 name+description，按需才读全文），所以它只能靠真实拷贝进原生 skills 目录送达
# —— 不能靠钩子注入。
#
#   [[skill_source]] 字段：
#     path              skill 根目录（单个或多个）—— 其下每个含 SKILL.md 的子目录算一个 skill
#                       （若根本身含 SKILL.md，则根本身也算一个 skill）。本版仅本地路径。
#     agents            同 [[source]]：省略 = 全部已支持 agent。
#     workspace         同 [[source]]（仅 workspace 域生效；scope="user" 时忽略）。
#     groups            引用 [[skill_group]] 组（与 [[source]].groups 同义）：
#                       每个组打包多个 skill 根目录。
#     workspace_groups  引用 [[workspace_group]] 组（工作区在规则与 skill 间共用）。
#     scope             同 [[source]]："workspace"（默认）或 "user"。
#
#   落到哪（2026-10-12，对照 CodeBuddy bundle 核实）：
#     workspace 域 → <ws>/.codebuddy/skills   （codebuddy + workbuddy 共享）
#     user 域      → ~/.codebuddy/skills 与 ~/.workbuddy/skills（两个都写）
#   skill 以 __rulemux__<skill名>/ 目录落地；你自己的 skill 一律不碰，
#   删掉源则下次同步移除其副本。
#   其它 agent（trae / dsh / claude / codex / opencode）暂不支持 skill。
#
# --- 进阶：用「组」打包，避免重复 -------------------------------------------------
# 文件多了以后，把常用文件 / 工作区打包成组，在 [[source]] 里按名引用。
#
# 文件组 [[file_group]]：打包规则文件，通过 groups 引用。
#   name  组名（必填）
#   path  组内文件；单个值或数组
#   use   引用其它文件组 —— [数组，可多个，可嵌套]：
#           use = ["dev"]            # 一个组
#           use = ["dev", "qa"]      # 多个：一次全部复用
#           # A 组引入 B 组再加自己的文件 => A == B 的文件 + A 的文件
#
# 工作区分组 [[workspace_group]]：打包工作区，通过 workspace_groups 引用。
#   name       组名（必填）
#   workspace  组内工作区；单个值或数组，允许 glob
#   use        引用其它工作区分组 —— [数组，可多个，可嵌套]：
#           use = ["dev"]            # 一个组
#           use = ["dev", "qa"]      # 多个：一次全部复用
#
# skill 组 [[skill_group]]：打包 skill 根目录，通过 [[skill_source]] 的 groups 引用。
#   name  组名（必填）
#   path  组内 skill 根目录；单个值或数组
#   use   引用其它 skill 组 —— [数组，可多个，可嵌套]
#
# 在 [[source]] 里：
#   groups            = ["gA", "gB"]     # 文件组（数组，可多个；展开成它们的文件）
#   workspace_groups  = ["gX", "gY"]     # 工作区分组（数组，可多个）
#   [允许混写] groups 可与 path 并存，workspace_groups 可与 workspace 并存：
#       groups = ["base", "proj"]            # 用 base 与 proj 两个组的全部文件
#       path   = ["/abs/path/extra.md"]      # ……再加这一个文件
#     => 这条 source 投递 base 的文件 + proj 的文件 + extra.md，按值去重。
#   （workspace_groups / workspace 同理：既引用组、又列一个单独的工作区。）
#   组间重复无害 —— 最后按值去重，每个文件只处理一次。
#   文件组与工作区分组是两套独立命名空间，允许同名。
#
# 下面全部注释掉、不生效。按需取消注释，把占位符换成你的真实值。

# [[file_group]]
# name = "base"
# path = ["/abs/path/to/team-conventions.md", "/abs/path/to/style.md"]

# [[file_group]]
# name = "proj"
# use = ["base"]                       # 嵌套 base 组（数组，可多个：use = ["base", "dev"]）
# path = ["/abs/path/to/project-a.md"]

# [[workspace_group]]
# name = "dev"
# workspace = ["/abs/path/to/proj-1", "/abs/path/to/proj-2"]

# [[workspace_group]]
# name = "qa"
# use = ["dev"]                        # 复用 dev 组（数组，可多个）
# workspace = ["/abs/path/to/proj-3"]

# [[source]]
# groups = ["base", "proj"]            # 文件组（数组，可多个；展开成它们的文件）
# path = ["/abs/path/to/extra.md"]     # 再加一个单独文件：这条 = base + proj + 此文件
# agents = ["codebuddy"]
# workspace_groups = ["dev"]          # 工作区分组（数组，可多个；展开成它们的工作区）
# workspace = ["/abs/path/to/standalone"]   # 再加一个单独工作区

# --- skill 示例（字段与规则相同，但源是一个「skill 目录」）------------------------

# [[skill_source]]
# path = ["/abs/path/to/my-skills"]    # 每个含 SKILL.md 的子目录就是一个 skill
# agents = ["codebuddy", "workbuddy"]  # 省略 = 全部已支持 agent
# workspace = "*"                      # 所有工作区（省略即默认）

# [[skill_source]]
# path = ["/abs/path/to/global-skills"]
# scope = "user"                       # 用户级（全局）：落到 ~/.codebuddy/skills
#                                       # 与 ~/.workbuddy/skills，每个工作区都生效

# [[skill_group]]                      # 打包 skill 根目录，供多条 source 复用
# name = "frontend"
# path = ["/abs/path/to/react-skills", "/abs/path/to/css-skills"]
# # use = ["base"]                     # 嵌套另一个 skill 组（数组，可多个）

# [[skill_source]]
# groups = ["frontend"]                # 复用该组，省得重复写 path
# agents = ["codebuddy"]
# scope = "user"
```

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
`scope = "user"` 投递到**用户级（全局）**目录的规则与 skill 也会按「卸载后还有谁在用」收敛
（只删 `__rulemux__` 前缀内容，目录本身保留）。

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

## 8. 支持范围（已定稿）

**支持的 agent 到此为止**，一共四家 —— 全是 Tier-1，全都经真实 canary 坐实：**codebuddy**、
**workbuddy**、**trae**、**dsh**。`claude`、`codex`、`opencode` 仍在注册表里但未核实，因此
`rulemux init` 会拒绝它们 —— **没有补齐它们的计划**。（接入新 agent 的清单留作参考：
[`docs/design/features/agent-onboarding.md`](docs/design/features/agent-onboarding.md)。）

- **skill 注入的支持范围**（2026-10-12）：**只有 codebuddy / workbuddy** —— 两家的 skills 目录
  （工作区 `.codebuddy/skills`、用户级 `~/.codebuddy/skills` + `~/.workbuddy/skills`）已经
  bundle 代码级 + 本机实测坐实；trae / dsh / claude / codex / opencode 的 skills 目录尚未实测，
  **暂不支持**，坐实一家开一家。
- **分发** —— 已完成：GitHub Actions 交叉编译二进制并挂到 GitHub Release；npm 包**手动发布**（见 §9）。
- 规则内容改写 / 模板渲染、以及自动自我更新，**明确不做**
  （[`docs/design/requirements.md`](docs/design/requirements.md) §五）。
- skill 的远程 / git 源（跟进上游更新）、`rulemux skill` 子命令 —— **后续版本**，第一版只做本地源。

---

## 9. 发版（维护者）

```bash
npm version patch|minor|major   # 更新 package.json 并创建 git tag v0.x.y
git push --follow-tags          # CI：交叉编译产物挂到 GitHub Release
npm publish                     # npm：手动，在已登录的 shell 里跑
```

推送 tag 会触发 [`.github/workflows/release.yml`](.github/workflows/release.yml)：
自动交叉编译全部平台二进制并挂到 GitHub Release。

**npm 这一步刻意不自动化**：不配 `NPM_TOKEN`（也不打算配），发布 npm 就保持在已登录的 shell 里手动
`npm publish`（工作流会跳过该步骤并告警）。dsh 插件是独立的包，它有改动时也要单独发：

```bash
cd dsh-plugin && npm publish
```

普通的 CI（[`ci.yml`](.github/workflows/ci.yml)）在每次 push 时跑 build/vet/测试/冒烟。

---

## 10. 许可证

MIT —— 见 [LICENSE](LICENSE)。

---

## 11. 文档索引

| 想看什么 | 去哪 |
|---|---|
| 用户到底要什么（需求 / 验收标准） | [`docs/design/requirements.md`](docs/design/requirements.md) |
| 为什么这么设计 | [`docs/design/architecture.md`](docs/design/architecture.md) |
| 功能列表 | [`docs/design/features.md`](docs/design/features.md) |
| 接入新 agent 的清单 | [`docs/design/features/agent-onboarding.md`](docs/design/features/agent-onboarding.md) |
| 各家 agent 的规则目录（外部事实） | [`docs/design/external/agent-rules-dirs.md`](docs/design/external/agent-rules-dirs.md) |
| 当前进度与未决项 | [`docs/progress.json`](docs/progress.json) |
