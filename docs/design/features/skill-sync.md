# skill 同步（v1 已实现：codebuddy / workbuddy）

> **状态**：🟢 **v1 已实现**（2026-10-12，`internal/engine/skill.go` + registry 双域字段 +
> `[[skill_source]]`/`scope` 配置 + sync/uninstall 编排；`go build/vet/test` + `scripts/smoke.sh` 20 项全绿）。
> **v2 已拍板**（2026-10-14，用户定序 Trae → DeepSeek harness，dsh 先行）：dsh 接入方案见 §七，
> 外部事实源码级坐实见 [`../external/skill-ecosystem.md`](../external/skill-ecosystem.md) §三·再续。
> 范围：v1 = **仅 codebuddy / workbuddy**；v2 = **+ dsh**（trae 仍下轮）；**仅本地源**；用户级（全局）双域同版交付。
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
   trae 三字段仍留空（下轮）⇒ 天然不参与。
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
- **第一版只支持本地 skill 源**；远程 / git 源（S3「跟进上游」）与 `rulemux skill list/update`
  子命令留待后续版本。

## 四、剩余 canary 待办（🔴，需真机会话；全部来自 `external/skill-ecosystem.md` §六）

1. WorkBuddy skills 两级**装载** Mac canary（落点已代码级确定：用户级 `~/.workbuddy/skills`、工作区级 `.codebuddy/skills`）+ 用户级 rules `~/.workbuddy/rules` 装载；
2. ~~dsh skills 目录（`.dsh/skills` / `.agents/skills`、不递归）官方文档复核 + 用户级落点~~ —— 2026-10-14 **源码级坐实**（external §三·再续），落点判据已足；装机装载 canary 转 §七-5；
3. Trae 工作区 `.trae/skills` 本机 canary + intl `~/.trae/skills` 探测；
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

- skill 源**仅本地目录**；git/远程源 = v2（S3）。
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

1. `__rulemux__` 前缀目录真被装载（源码级高可信，仍建议验一次）；
2. 会话中 sync 落盘的热重载时序（watcher `depth:1` 对嵌套 `SKILL.md` 变更的覆盖）；
3. `~/.dsh/skills` 用户级装载；
4. `DSH_HOME` 被设置时 rulemux 写死 `~/.dsh` 的错位（与 codebuddy 的 `CODEBUDDY_CONFIG_DIR` 同类；
   shortcut：先文档标注，有需求再按 env 跟随）。

### 7.7 发布配对（AGENTS.md §5）

`REQUIRED_CLI` 抬版 = 改了「CLI 获取 / 版本判定」逻辑 ⇒ CLI 发 `vX.Y.Z` 与插件发 `dsh-vX.Y.Z`
**必须同批**；只发 CLI 推不动已装旧插件的机器。
