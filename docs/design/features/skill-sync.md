# skill 同步（v1 已实现：codebuddy / workbuddy）

> **状态**：🟢 **v1 已实现**（2026-10-12，`internal/engine/skill.go` + registry 双域字段 +
> `[[skill_source]]`/`scope` 配置 + sync/uninstall 编排；`go build/vet/test` + `scripts/smoke.sh` 20 项全绿）。
> **v2 已拍板**（2026-10-14，用户定序 Trae → DeepSeek harness，dsh 先行）：dsh 接入方案见 §七，
> 外部事实源码级坐实见 [`../external/skill-ecosystem.md`](../external/skill-ecosystem.md) §三·再续。
> 范围：v1 = **仅 codebuddy / workbuddy**；v2 = **+ dsh**（2026-10-14）+ **trae**（2026-10-15，CN 双域：工作区 `.trae/skills` 🔴 待 canary、用户级 `~/.trae-cn/skills` 已坐实）；用户级（全局）双域同版交付。
> **S3 远程 / 市场源**：2026-10-15 用户拍板走「嫁接」——`[[skill_source]]` 的 `path` 多态（本地目录 / `gh:owner/repo` / git URL / `npm:pkg`），首次 sync 惰性 clone 进 `~/.rulemux/skills/<key>/`、hook 阻塞带 `fetch_timeout` 超时警告跳过、升级仅显式 `rulemux skill update`（原地 `git pull`）。方案见 §八（**已定稿，未实现**）。
> 剩余 canary 待办见 §四/§七。
> **配套**：[`../requirements.md`](../requirements.md) §六（需求与验收判据）· [`../external/skill-ecosystem.md`](../external/skill-ecosystem.md)（上游事实：各家 skills 目录、市场格局）· [`dir-sync.md`](dir-sync.md)（规则同步主方案，本篇沿用其哲学）· [`../external/agent-rules-dirs.md`](../external/agent-rules-dirs.md) §5.5（双品牌同源模型）

## 一、与规则同步的相同与不同

| 维度 | 规则同步（已交付） | skill 同步（本篇） |
|---|---|---|
| 投递方式 | SessionStart hook 调 `rulemux sync`（投递员） | **相同**——同一 hook、同一命令，顺带同步 skills |
| 落盘方式 | 真实拷贝进原生规则目录 | **相同**（真实拷贝；明确不采用 npx skills 的默认 symlink） |
| 载体 | 单文件 `.md` + `__rulemux__` 前缀 | **目录** `<skill-name>/SKILL.md` + 附属 resources ⇒ 前缀要落在**目录名**上 |
| 加载语义 | 会话开始静态全量 | 渐进式披露（启动只载 name+description，按名调用才读全文）⇒ hook 注入全文不可行，只能落原生目录 |
| 作用域 | 仅工作区 | **双域**：工作区级 + 用户级（全局），由配置声明 |
| 源形态 | 本地文件路径 | 本地目录 / git 仓库（GitHub 简写等，参照 npx skills 源写法）/ 市场索引引用 |

## 二、实现口径（v1，2026-10-12 落地）

1. **引擎（`internal/engine/skill.go`）**：`SyncSkills(dir, srcs, create)` 与文件级 `Sync` 同构——
   目标名 = `__rulemux__<skill目录名>/`（跨源同名追加源目录短 hash）；逐文件字节比对增量复制；
   计划内目录多出的文件删除、变空子目录清理；计划外 `__rulemux__*` 目录 `RemoveAll`；
   用户自建（非前缀）目录一律不碰；`create=false` 且目录不存在返回 `SkippedNoDir`。
   skill 识别 = 目录含 `SKILL.md`（大小写不敏感）；源根本身含 SKILL.md 时根本身也是一个 skill
   （单技能目录布局）；无 SKILL.md 的子目录静默跳过。符号链接跳过（shortcut：只同步真实文件）。
2. **适配器（`internal/agents/registry.go`）**：Agent 增加 `SkillsDir`（工作区）/
   `UserSkillsDirs` / `UserRulesDirs`（用户级，`~` 走 `ExpandHome`）。codebuddy 与 workbuddy 填同值：
   `SkillsDir=".codebuddy/skills"`；用户级 skills/rules 各**双目录**（`~/.codebuddy/…` + `~/.workbuddy/…`）
   —— 用户级要覆盖两家必须双写（§一证据同前）。dsh 于 v2 填入 `.dsh/skills` + `~/.dsh/skills`（§七）；
   trae 于 2026-10-15 接入 CN 双域：`SkillsDir=".trae/skills"` + `UserSkillsDirs=["~/.trae-cn/skills"]`；工作区级 🔴 待真机 canary 坐实（社区文档一致），用户级已本机坐实（见 agent-rules-dirs.md §二·续）。
   共享判定 `SharingSkillsDir` / `SharingUserDirs` 与 rules 侧 `SharingRulesDir` 同规律。
3. **共享目录**：工作区级 skills 与 rules 同规律——两家共用 `.codebuddy/skills`，同步按并集
   （`unionSkillIDs`），谁跑都不删别人的。用户级不共享且方向不对称 ⇒ 按品牌各写各的，
   双写重叠由宿主重名去重（无害）；卸载时两个目录都登记清理。
4. **配置（`~/.rulemux/config.toml`）**：新增 `[[skill_source]]`（`path`/`agents`/`workspace`/`scope`
   + 分组键 `groups`（引用新增的 `[[skill_group]]`）/ `workspace_groups`（复用 `[[workspace_group]]`，
   2026-10-13 补齐，见 §六）；`[[source]]` 增加同一 `scope` 字段实现 S2 双域。
   查询分层：`SourcesFor(Any)` / `SkillSourcesFor(Any)` 只返 workspace 域，`UserSourcesFor(Any)` /
   `UserSkillSourcesForAny` 返 user 域（忽略 workspace 匹配）。`scope=user` 的条目让
   `DeclaresWorkspace` 恒真（全局目录本就与工作区无关）。
5. **hook 与提示语义**：`rulemux sync --hook` 同步规则 + skills + 用户级，同一道 create 门禁
   （仅被 `--agent` 点名的那一个 agent 可建目录）。**skills 变化不计入「新开会话」提示**
   （shortcut：热重载待 canary，见 §四）；rules（含用户级 rules）变化照旧提示。
6. **命令面（最小集）**：零新命令——`sync`（含 hook 自愈、`--all`、cascade）顺带同步 skills 与用户级；
   `init` 示例配置与 `doctor` 自检同步扩展；`uninstall` 对用户级目录追加收敛（只删前缀内容，目录保留）。

## 三、明确不做（对齐 requirements §6.4）

- 不自建市场服务端；不做 skill 内容生成/改写；不做 lockfile 版本锁定。
- 不做 Tier-2 式 hook 注入 skills（破坏渐进式披露，B2 不满足）。
- **不自建市场 / 不做市场索引**（§八：抓取与市场发现交给 git / npm / SkillHub 等外部，rulemux 只做分发）；
- 远程源**不做成市场客户端**、不做 lockfile 版本锁定、不做自动升级检查（§八 8.5）；
- `rulemux skill list` / 市场索引接入不做；zip/tar 直链 YAGNI（§八 8.7）。

## 四、剩余 canary 待办（🔴，需真机会话；全部来自 `external/skill-ecosystem.md` §六）

1. WorkBuddy skills 两级**装载** Mac canary（落点已代码级确定：用户级 `~/.workbuddy/skills`、工作区级 `.codebuddy/skills`）+ 用户级 rules `~/.workbuddy/rules` 装载；
2. ~~dsh skills 目录（`.dsh/skills` / `.agents/skills`、不递归）官方文档复核 + 用户级落点~~ —— 2026-10-14 **源码级坐实**（external §三·再续），落点判据已足；装机装载 canary 转 §七-5；
3. Trae 工作区 `.trae/skills` 本机 canary（**进行中 2026-10-15**：registry 已接双域、verify 已能种技能探针，待用户真机开 Trae 会话验收装载）+ intl `~/.trae/skills` 探测（未做）；
4. skills 加载行为 canary（渐进式披露 / `skills-changed` 是否热重载 / 要不要「新开会话」提示）；
5. CodeBuddy 用户级 rules `~/.codebuddy/rules` 装载 canary（落点已代码级坐实，验合并加载与 frontmatter 语义）。

**已销项（2026-10-12）**：Claude canary（不在支持范围）、CodeBuddy 用户级 rules 落点（代码级坐实）、市场第三方源接入（已答**不支持** ⇒ S3 由 rulemux 自管，不借宿主机制）。

## 五、v1 实现记录（2026-10-12）

| 层 | 落点 | 内容 |
|---|---|---|
| 配置 | `internal/config/config.go` | `SkillSource` 结构 + `[[skill_source]]` 解析；`Source.Scope`；域查询 `SourcesFor(Any)`/`SkillSourcesFor(Any)`/`UserSourcesFor(Any)`/`UserSkillSourcesForAny`/`HasAnyFor`；`Validate` 放宽（有任一来源即可 + scope 校验）；`DeclaresWorkspace` 覆盖 skill 源与 user 域 |
| 引擎 | `internal/engine/skill.go`（新） | `BuildSkillPlan` / `SyncSkills` / `syncTree` / `pruneEmptyDirs`；复用 `Prefix`/`shortHash`/`SyncResult`（`Plan.Missing` 记录不可用源根） |
| 适配器 | `internal/agents/registry.go` | `SkillsDir`/`UserSkillsDirs`/`UserRulesDirs` + `SkillsDirAbs`/`UserSkillsDirsAbs`/`UserRulesDirsAbs`；`SharingSkillsDir`/`SharingUserDirs`；codebuddy/workbuddy 填双域同值 |
| 编排 | `internal/cmd/sync.go` | `syncRun`（四路同步 + 跨 agent 目录去重 + `landed` 记账）；`unionSkillIDs`/`unionUserIDs`；cascade 方法化（`HasAnyFor` 闸）；`syncAll` 同规则（用户级目录全局只跑一次） |
| 卸载 | `internal/cmd/uninstall.go` | `remainingUserPeers`/`pruneUserDirs`/`pruneSkillsDir`；账本回访加 skills 清理 |
| 命令面 | `internal/cmd/init.go` / `doctor.go` | 示例配置补 `scope` 与 `[[skill_source]]`；doctor 展示 skills/用户级目录存在性 |
| 测试 | `internal/engine/skill_test.go`、`internal/config/config_test.go`、`internal/agents/registry_test.go`、`internal/cmd/skill_sync_test.go`、`scripts/smoke.sh`（17–20 步） | 首拷/幂等/覆盖/清残留/树内清理/同名 hash/缺失源/create 门禁/用户文件不碰；用户级双写；skills-only 不触发提示；卸载用户级收敛 |

**shortcut 清单**（已知限制，升级时机）：

- skill 源默认**本地目录**；远程源（git/npm）见 §八——**clone 一次、不自动升级、不 lockfile**；本轮 git 优先，`npm:` 为 follow-up。
- skills 变化**不触发**「新开会话」提示 = 待 §四-4 热重载 canary 后按 per-agent 判据补。
- 符号链接不同步（只同步真实文件）= 有需求再处理。
- 用户级目录只在 hook 路径创建（被 `--agent` 点名时）；手动 sync / `--all` 只写已存在目录 = 与 rules 同一条铁律。
- WorkBuddy 用户级落点（`~/.workbuddy/{rules,skills}`）为**代码级推断**，双写策略对两家均安全，装载待 Mac canary（§四-1）。

## 六、skill 分组与配置语义定稿（2026-10-13 补齐）

`[[skill_source]]` 原先不支持分组（原 §五 shortcut「源是目录不是文件」）；用户要求「skills 也要支持
分组」，并借此把配置语义在 README 讲清。落地：

- **新增 `[[skill_group]]`**：`name` / `path`（skill 根目录，单个或数组）/ `use`（嵌套其它 skill 组）——
  与 `[[file_group]]` 同构，但装的是**目录**而非文件。
- **`[[skill_source]]` 新增两个键**：`groups`（引用 `[[skill_group]]`）、`workspace_groups`
  （引用现有 `[[workspace_group]]`，工作区在规则 / skill 间共用）。都可与自身 `path` / `workspace`
  混写，按值去重（组展开在前、显式在后）。
- **展开与校验复用文件组同一套**：`~` 先展开再展开组、环检测、未定义 / 重名 / 未命名报错
  （`internal/config/config.go` 的 `SkillGroup` + `resolveGroups`）。
- **配置语义定稿（用户 2026-10-13 拍板）**：域由**显式 `scope`** 决定（`workspace` 默认 / `user`），
  `agents` 与 `workspace` 是**正交过滤器**。**不采用**「写了 `agents` 即 = 用户级通用」的隐式模型 ——
  它会丢掉「只给某 agent、只在该工作区」的组合，且与 `[[source]]` 语义分叉。
  （注意：`workspace = "*"` ≠ 全局；真·全局只有 `scope = "user"`。）
- **落点**：`internal/config/config.go`（`SkillGroup` 结构 + 解析 + `resolveGroups` 展开）；
  `internal/config/config_test.go`（组展开 / 工作数组 / `~` 展开 / 三类错误）；
  `internal/cmd/init.go`（示例配置补 `[[skill_group]]` 与字段说明）；README 中英双份 §5.1 / §5.4 / §5.5
  （§5.5 收录完整带注释的默认配置）。

## 七、dsh 接入（v2，2026-10-14 用户拍板）

外部事实（源码级，`deepseek-ai/deepseek-harness@master`，2026-10-14）见
[`../external/skill-ecosystem.md`](../external/skill-ecosystem.md) §三·再续；此处只记「我方怎么落」。

### 7.1 落点（用户 2026-10-14 逐条拍板）

| 域 | 落点 | 宿主 rank | 依据 |
|---|---|---|---|
| 工作区 | `<git根>/.dsh/skills` | 100（最高） | dsh 专属目录；托管拷贝（`__rulemux__` 前缀）稳定压过用户手工放在 `.agents/skills`（200）的同名技能 |
| 用户级 | `~/.dsh/skills` | 400 | 全局根，所有 dsh 会话/工作区都加载（`skipSystem` 仅跳过 `.system` 条目） |
| **不写** | `.agents/skills`（项目 200 / 用户 500） | — | 跨框架公约目录（npx skills canonical store 所在），只读不碰；rank 也更低，写了无收益 |

dsh 维持 **Tier-2 / `RulesDir` 空**（规则仍是纯插件注入，不动）；skills 是独立通道，
`SkillsDir` 非空即参与 skill 同步 —— `sync` 主循环的跳过条件相应从「Tier-2 全跳过」改为
「Tier-2 且无 skills 目录才跳过」（codex/opencode 三字段为空 ⇒ 行为逐字节不变）。

### 7.2 项目根对齐（git 根）

dsh 的「工作区」= 从会话 cwd 逐级向上找最近的 `.git`（`findProjectRoot`），而 rulemux 的 sync
按 cwd/hook 载荷定位（`internal/cmd/flags.go`）。用户在仓库**子目录**里开 dsh 会话时，两边会错位：
技能写进 `<子目录>/.dsh/skills`，dsh 永远扫不到 —— **静默失败**。故 CLI 侧对 dsh 增加
git 根对齐（标准库 `os`/`filepath` 自实现，约 15 行，放 `internal/cmd`）：从 ws 逐级向上找
`.git`，找到则以其为同步根，找不到回退 ws。与宿主同构、可单测；不把宿主逻辑复制进插件 JS。

### 7.3 触发（插件充当 dsh 的 SessionStart）

dsh 没有 hook 系统，skills 落盘只能由插件触发。`dsh-plugin/index.mjs`：

- `agent/session-start`：发起（不 await）`rulemux sync --hook --agent dsh`（cwd = 会话工作区）；
- 首个 `agent/pre-step`：await 同一 Promise 兜底，保证首轮模型步之前文件已落盘；
- 失败只 `console.error` 记日志、会话继续（与 rules 注入同哲学，不进 readiness 三步）；
- `--hook` 是**必需的**：目录创建权只在 hook 路径发放（`sync.go` createAgent 门禁），
  不带则 create=false、首跑目录不存在 ⇒ 技能永远落不了地；插件 spawn 时 stdin 为 /dev/null，
  载荷解析自动落败回退 cwd，语义正确；
- `REQUIRED_CLI` 抬至首个带 dsh skills 能力的发布版（发版时定值），理由按既有注释纪律写清。

### 7.4 引擎与配置面零改动

- `internal/engine/skill.go` **不动**：dsh 发现名取自 frontmatter `name`、目录名不校验 ⇒
  `__rulemux__<技能名>/` 前缀安全；非递归扫描天然兼容（引擎只写顶层前缀目录）。
- 配置面 `[[skill_source]]` 的 `agents = ["dsh"]` / `scope` / groups 全部现成，无新键。

### 7.5 frontmatter 差异警示（只写文档，不改写内容）

| 差异 | 后果 |
|---|---|
| `name` 须匹配 `^[a-z0-9]+(?:-[a-z0-9]+)*$` | 不合规（大写/下划线等）的技能在 dsh **静默消失**，CodeBuddy 不报错 |
| 旧式驼峰调用键（`disableModelInvocation` 等） | dsh **丢弃整个技能**（CodeBuddy 接受） |
| CodeBuddy 市场扩展字段（`display_name`/`description_zh`/`version`/`allowed-tools`） | 被 dsh 忽略，无害 |

rulemux 只做字节拷贝、不改内容 ⇒ 上述差异是用户写技能时的约束，进 README 与外部事实文档警示。

### 7.6 剩余 canary（🔴，真机 dsh，挂 v2 任务）

1. ~~`__rulemux__` 前缀目录真被装载~~ —— ✅ **2026-10-15 真机通过（用户验收）**：CLI 0.5.0 +
   插件 0.5.0，`[[skill_source]]`（agents=["dsh"]）开会话，前缀技能落盘即装载；
2. 会话中 sync 落盘的热重载时序（watcher `depth:1` 对嵌套 `SKILL.md` 变更的覆盖）——🔴 未单独验证；
3. `~/.dsh/skills` 用户级装载——🔴 未单独实测（落点源码级坐实）；
4. `DSH_HOME` 被设置时 rulemux 写死 `~/.dsh` 的错位（与 codebuddy 的 `CODEBUDDY_CONFIG_DIR` 同类；
   shortcut：先文档标注，有需求再按 env 跟随）——🔴 未验证，已知限制。

### 7.7 发布配对（AGENTS.md §5）

`REQUIRED_CLI` 抬版 = 改了「CLI 获取 / 版本判定」逻辑 ⇒ CLI 发 `vX.Y.Z` 与插件发 `dsh-vX.Y.Z`
**必须同批**；只发 CLI 推不动已装旧插件的机器。

---

## 八、远程 / 市场源（S3，2026-10-15 用户拍板）

> **状态**：🟢 **方案已定稿，未实现**。用户 2026-10-15 连续拍板：否决「rulemux 自建 fetch 市场客户端」（所谓「方向 1」）；不等市场开放「装到任意目录」（`npx skills` Issue #130 明确不支持、SkillHub 只往 agent 目录装，短期无望）；走「嫁接」——**外部负责抓取，rulemux 只负责分发**。

### 8.1 定位与边界

- **rulemux 干分发不干安装**：skill 的「市场发现 / 下载 / 版本 / 更新」交给外部（`git` / `npm` / 市场如 SkillHub / ClawHub），rulemux 只做「拉到本地 staging + 选择性 fan-out」。
- **否决**：① rulemux 自己实现 git/tarball fetch、自建市场客户端（重造轮子还要维护）；② 等市场开放任意目标目录开关。
- **采纳**：**配置驱动 + 惰性 clone + 委派 git/npm 二进制**。`git` 近乎必有；只 spawn 外部命令、不引 Go 库 ⇒ 保持**零第三方 Go 依赖**不变。
- **real-copy 底线不变**：远程源同样真实拷贝，不走 npx skills 的 symlink canonical 模式。
- **不加新配置键**：沿用 `[[skill_source]]` 的 `path`，类型在**源根解析层**按字符串特征判定（不引新键，`agents`/`workspace`/`scope`/groups 语义全不变）。

### 8.2 `[[skill_source]]` 的 `path` 多态解析

一个 `path` 承载本地 / git / npm 多类源，**按字符串特征自动分流，用户不显式选类型**（可再跟 `#子路径` 精确指定仓库内位置，见下）：

| `path` 写法 | 识别 | 落点 `<key>` | 抓取 |
|---|---|---|---|
| `/abs`、`~/x`、`./x`、磁盘上真实存在的目录 | 本地目录 | 不 clone，直接用 | — |
| `gh:owner/repo`（或 `github:owner/repo`） | GitHub 仓库 | `owner-repo` | `git clone https://github.com/owner/repo.git`（HTTPS 默认） |
| 完整 git URL：`git@github.com:owner/repo.git` / `https://gitlab.com/x/y` / `git://…` | 任意 git 仓库 | 取末两段 `owner-repo` | **原样 clone**，SSH / HTTPS 由 URL 决定 |
| `npm:pkg` | npm 包 | `npm-pkg` | `npm pack` 取包解包 |

- 类型前缀用 `gh:` / `npm:`，**刻意避开与 SSH 传输符 `git@` 撞车**（`git@` 只在完整 URL 里出现，原样克隆）。
- 裸 `owner/repo`（无协议无冒号）默认按 GitHub 处理；仍有歧义时提示补前缀。
- **`#子路径`（可选）**：任一路径后跟 `#仓库内子路径`，把源根精确指到某容器目录或单个 skill 目录。`gh:`/git URL/`npm:` 都不含 `#`，故无歧义；**staging key 只取 `#` 前**，同一 repo 的多个子路径共享一次 clone。例：`gh:anthropics/skills#skills/docx` 只取 docx；数组 `[…#skills/docx, …#skills/pptx]` 取两个。

### 8.3 staging 落点

- 统一落 `~/.rulemux/skills/<key>/`，`<key>` 由 URL **确定性派生**（可读、可排重）：`gh:owner/repo` → `owner-repo`；`npm:pkg` → `npm-pkg`。
- **源根 = clone/本地根 + `#子路径`**；skill 发现为**限深自动探测**（对齐 npx skills「扫仓库枚举 skill」）：根本身含 `SKILL.md` ⇒ 单 skill；否则向下扫（默认 ≤4 层），遇含 `SKILL.md` 的目录即一个 skill 并**不再往其内部钻**（其 references/scripts 是自身资源，非独立 skill），跳过 `.git`/`node_modules`。故 `gh:anthropics/skills`（skill 埋在 `skills/` 下）**不带 `#` 也能把全部 skill 找出分发**；要精确挑选再用 `#子路径`。成本：本地限深 dirent 遍历，毫秒级（下载/解压已有 ≤25MiB/≤1000 文件上限兜底）。
- 本地路径源不 clone、不进 staging。

### 8.4 拉取时机与超时

- **日常 `sync`**：staging 已有该 `<key>` ⇒ 当本地源用，**零网络**。
- **首次 `sync`**：本地无 ⇒ 按类型抓取（`git clone` / `npm pack`）。
- **hook 模式（`--hook`）**：抓取**阻塞等落盘**（hooks 已配就必须等），但加 `fetch_timeout`（默认 30s，可配）；超时 / 出错 / 网络失败 ⇒ stderr 一条警告（与现有 sync 警告同格式），**本轮跳过该源、继续同步别的，下回再试**；绝不无限卡住 agent 启动。
- **手动 `sync`**：同样阻塞、报完整错误（用户正盯着，慢可接受）。

### 8.5 升级语义（clone 一次 + 显式 update，不自动、不叠加）

- **不自动升级**：日常 sync **绝不**重新抓取，`fetch_timeout` 只在首次可能命中 ⇒ hook 保持飞快。不引入「每次执行查新版」的网络开销。
- **显式 `rulemux skill update <key>`**：git → 在已存在的 `~/.rulemux/skills/<key>` 里**原地 `git pull`**（或 `git fetch` + `reset --hard origin/<ref>`）；npm → 原地重新取包覆盖。**不删重克、不叠加。**`<key>` 可传原始 spec（`npm:pkg` / `gh:owner/repo`）或 staging key（`npm-pkg` / `gh-owner-repo`），亦可传**用户友好本名**（`pkg` / `repo` 末段）——list/update 的主标识统一用本名，源类型以 `[npm]`/`[git]` 限定符呈现，避免 `npm-task-ledger` 这类目录名被当成技能名误导用户。
- **未抓取也能 update**：若 `~/.rulemux/skills/<key>` 尚不存在（首次），`update` 顺带做**惰性 clone / pack**（等同 §8.4「首次 sync」抓取的同一条路径），不再报 `is not staged yet; run a sync first` 而中断用户；仍**不会**自动升级 `sync` 已抓取的源（不自动、显式触发不变）。升级输出带**版本差**：npm 显示 `package.json` 的 `version`（`old → new`），git 显示 `git describe` / 短 commit hash，未变化显 `up to date`，首次抓取显 `fetched <ver>`（对齐 `git pull` 的 `a1b2c3d..e4f5g6h`、`brew upgrade` 的 `foo 1.0 -> 1.1` 风格）。
- **上游删文件能传导**：pull 后 checkout 删除该文件，下次 sync 复用引擎现有 prune（§二「计划内目录多出的文件删除 / 变空子目录清理」）把它从各家 agent 目录一并清掉。
- **兜底**：`git pull` 失败 / 仓库损坏 ⇒ `rm -rf` 该 `<key>` 目录再重新 clone（唯一会删目录的路径）。

### 8.6 信任边界（不许砍）

- 下载大小 / 解压后大小 / 文件数上限（借鉴 npx skills：≤10 MiB 下载 / ≤25 MiB 解压 / ≤1000 文件）。
- **路径穿越（zip slip / tar 绝对路径）防护**：解包时拒绝越出目标目录的条目。
- **私有仓库凭据**：走本机 `git` / `npm` 配置（SSH key、`gh`、`GITHUB_TOKEN`/`GH_TOKEN`）；**rulemux 绝不落盘明文**，只 spawn 外部命令借本机凭据。

### 8.7 范围与后续

- **本轮**：git（GitHub / 任意 git URL）+ `npm:` **两种源同批**——同一套「spawn 外部二进制 + 取回本地」；npm 走 `npm pack` 取 tarball、标准库 `archive/tar`+`compress/gzip` 解包（strip `package/`，零第三方依赖）。zip/tar 直链 YAGNI 不做。
- **命令面**：`rulemux skill list` + `rulemux skill update [<key>|--all]` **纳入本轮**（S3「跟进上游更新」硬必需；`skill update` 与 requirements §55–57「不自动更新、手动触发」同哲学——只在显式命令跑，绝不进 hook）。仅「市场索引接入」仍不做（§三）。
- **测试**：本地 `git init` 仓库 + `httptest`（**不打真网**）；上限 / zip slip 用例；`scripts/smoke.sh` 加「本地 git 仓库作源」端到端。
