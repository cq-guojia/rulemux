# skill 生态外部事实：市场格局 + 各家 skills 目录

> **类型**：外部事实（上游 agent 侧 + 第三方工具侧）
> **适用版本**：CodeBuddy 4.12.1（本机，2026-10-12）；Trae CN（本机 `~/.trae-cn`，2026-10-12）；CodeBuddy CN bundle `stable-757a5b2f`（构建 2026-09-20，2026-10-12 核查）；vercel-labs/skills README（2026-10-12 抓取）；DeepSeek Harness 源码 `deepseek-ai/deepseek-harness@master`（2026-10-14 经 GitHub contents API 读取，§三·再续）；上游升级后据此复核
> **状态**：🟡 **部分核实** —— CodeBuddy / Trae CN 的**用户级** skills 目录经本机实测坐实（目录存在且装有真实技能）；Claude Code skills 目录经官方文档 + npx skills 兼容表（交叉一致，**但不在支持范围**）；**2026-10-12 bundle 代码级新坐实**：CodeBuddy 用户级 rules 落点 `~/.codebuddy/rules`、CodeBuddy ↔ WorkBuddy 同源模型（§三·续）、官方市场**不支持第三方源**；**2026-10-14 dsh skills 子系统源码级坐实**（多根 + rank / 非递归 / frontmatter 契约 / 热重载 / git 根定位，§三·再续），**2026-10-15 真机装载验收通过**（用户实测：CLI 0.5.0 + 插件 0.5.0，`__rulemux__` 前缀技能落盘即装载）；细分时序项（热重载/用户级/`DSH_HOME`）仍 🔴，见 §六；**WorkBuddy 的 skills 装载仍待实测（需 Mac），Trae 工作区级未 canary ⇒ 相关项标 🔴，不得作为实现依据**
> **来源**：2026-10-12 本机目录实测（`/root/.codebuddy`、`/root/.trae-cn`）+ CodeBuddy 安装产物（`~/.codebuddy-server-cn/bin/stable-757a5b2f…/extensions/genie/`）+ vercel-labs/skills GitHub README 抓取 + 社区二手（各表已标来源与日期）
> **配套**：[`../requirements.md`](../requirements.md)（skill 注入需求）· [`../features/skill-sync.md`](../features/skill-sync.md)（方案草案）· [`agent-rules-dirs.md`](agent-rules-dirs.md)（规则目录，姊妹篇）· 任务 `20261012-1724-skill-注入调研与设计-5731`（worklog 待办节挂核实清单）

> ⚠️ 本文件只记**上游/第三方事实**（目录、格式、能力边界）。
> 「我方据此怎么落 skill」是设计，归 [`../features/skill-sync.md`](../features/skill-sync.md)，**不写在这**。

---

## 一、市场格局：skill 管理项目都有什么形态

按「装机量 / 成熟度」排序的核心玩家（2026-10-12 调研）：

| 项目 | 形态 | 装机目标 | 源管理 | 更新机制 | 定位判断 |
|---|---|---|---|---|---|
| **vercel-labs/skills**（`npx skills`，skills.sh，MIT，33.9k★/2.9k fork） | 通用 skill CLI，75+ agent | 项目级（默认，**相对 cwd**）或全局（`-g`）；symlink（推荐）或 copy | GitHub 简写 / 完整 Git URL / 仓库内子路径 / GitLab / Azure / 任意 git URL / 本地路径 / 直链 zip-tar；兼容 `.claude-plugin/marketplace.json` 发现 | `npx skills update [-g\|-p\|-y]` 拉最新；**无 lockfile** | 交互式安装器；无配置驱动扇出、无 hook 实时同步、无工作区账本、无残留清理 |
| **CodeBuddy 官方技能市场**（`~/.codebuddy/skills-marketplace/`，本机实测） | 宿主内置市场组件 | 装进 `~/.codebuddy/skills/`（本机实测有 `hindsight-coding-agent` 在装） | 官方目录 100+ 技能（腾讯系为主：腾讯文档/会议/云 COS/TAPD/CNB/飞书/钉钉…） | 随宿主分发 | 封闭官方目录；第三方源接入方式未坐实 |
| **dsh 插件市场**（awesome-deepseek-harness-plugins，deepseek1024.com） | dsh 插件商店 | `dsh plugin … add`（插件可经 `ctx.skills` 注册技能） | GitHub 仓库自动收集 + 格式校验 + 免费搜索 API | 随插件更新 | dsh 生态的官方外市场；11,000+ 插件（2026-10-03 页面数据） |
| **Agent Skills CLI**（agentskills.in / Karanjot786/agent-skills-cli） | 市场索引型 CLI | 宣称 42+ agent；项目/全局范围**未在官网述明** | `skills submit-repo owner/repo` 收录 GitHub 仓库 | 更新机制未述明 | 市场分发向，细节不透明 |
| **SkillsMP**（skillsmp.com） | 聚合市场（第三方） | 复制安装命令（含 `/plugin install` 与 npx skills 两种） | 索引 GitHub 源（宣称 70 万+） | — | 纯目录/索引 |
| **SkillHub**（BiglionX/SkillHub、LiuYuLu/SkillHub） | 可自托管市场平台 | Docker 一键部署；服务端 | 发布/分享/管理/多租户/命名空间/审核 | — | 服务端市场，非本地分发工具 |
| **anthropics/skills** | 官方技能仓库 | — | 内容源（xlsx/docx/pptx/pdf 等） | 跟随上游 | 内容而非管理器 |
| **aiagentsdirectory.com/skills** | 目录聚合 | — | 索引 | — | 纯索引 |

**一句话结论**：现有全是「**安装器 / 市场**」形态——交互式、单次、按 cwd 或全局落盘；没有一个提供「配置声明一份源 → 同步实时分发到 N agent × M 工作区 + 残留清理」。这正是 rulemux 规则注入已交付的能力，skill 侧是空白。

---

## 二、npx skills（vercel-labs/skills）能力边界（与 rulemux 的差异化依据）

README 逐字引用（2026-10-12 抓取）：

**兼容表（COMPATIBILITY，节选头部列）**：

> Feature OpenCode OpenHands Claude Code Cline **CodeBuddy** Codex Command Code Kiro CLI Cursor Antigravity Roo Code Github Copilot Amp OpenClaw Neovate Pi Qoder Zencoder
> Basic skills Yes Yes Yes Yes **Yes** Yes Yes Yes Yes Yes Yes Yes Yes Yes Yes Yes Yes Yes Yes

| 关键事实 | 结论 |
|---|---|
| **rulemux 四 agent 的覆盖** | **codebuddy ✅（唯一）**；claude（`claude-code`）✅；**trae ✅（`trae`）**；**workbuddy ❌ 不在表**；**dsh / DeepSeek ❌ 不在表** |
| `--agent` 标识与目录 | `codebuddy` → `.codebuddy/skills/`（项目）/ `~/.codebuddy/skills/`（全局）；`claude-code` → `.claude/skills/` / `~/.claude/skills/`；`trae` → `.trae/skills/` / `~/.trae/skills/`（**注意：`~/.trae` 是国际版口径；CN 版本机实测为 `~/.trae-cn/skills`，见 §三**） |
| **安装目标** | INSTALLATION SCOPE 表逐字：`Project (default) ./<agent>/skills/`；`Global -g ~/<agent>/skills/` |
| **有无目标工作区参数** | **无**。OPTIONS 仅 `-g/--global`、`-a/--agent`、`-s/--skill`、`-l/--list`、`-c/--copy`、`-y/--yes`、`--all`。**没有 `--target`/`--workspace`/`--dir`** ⇒ 要装进别的目录只能先 `cd` 过去 ⇒ 「推送到某一个工作区」需逐个目录手动跑，无配置驱动 |
| 更新语义 | `update` = 拉最新；**无 lockfile/版本锁定**；无「跟上游 diff 再决定」的机制 |
| 其他边界 | symlink 模式下 canonical 副本在 `~/.agents/skills/`（**与 rulemux「不用软链」底线冲突**；`-c/--copy` 可绕开）；下载 ≤10 MiB / 解压 ≤25 MiB / ≤1000 文件；发现深度默认 3 层；默认遥测（`DISABLE_TELEMETRY=1` 关） |

**对「要不要复用」的意义**：npx skills 可当**源获取**的参照实现（六种源写法 + marketplace.json 发现），但其分发模型（交互、cwd、symlink、无 lockfile）与 rulemux 的三条底线（永不淡出/不累积/不用软链）+ 配置驱动扇出**不兼容** ⇒ 不能直接复用为主通道。

---

## 三、各家 agent 的 skills 目录（工作区级 / 用户级两级）

| Agent | 工作区 skills 目录 | 用户级（全局）skills 目录 | 证据等级 | 出处 |
|---|---|---|---|---|
| **CodeBuddy** | `.codebuddy/skills/` | **`~/.codebuddy/skills/` + `~/.workbuddy/skills/` 合并** | ✅ 本机实测 + 代码级 + 活证 | ① 本机 `/root/.codebuddy/skills/hindsight-coding-agent/` 真实存在且**本会话装载可用**（2026-10-12）；② bundle：`userSkillsDir=join(homedir(),".codebuddy","skills")`、`getProjectSkillsDir()=join(root,WORKSPACE_DATA_DIR_NAME,"skills")`、`resolveUserSkillsDirs()` 品牌为 `.codebuddy` 时合并 `.workbuddy`（§三·续）；③ 本仓库 `.codebuddy/skills/koc-code-doc-progress` 在装；④ npx skills 兼容表 |
| **Trae（CN）** | `.trae/skills/` | **`~/.trae-cn/skills/`** | ✅ 本机实测（用户级）；工作区级为社区文档 | ① 本机 `/root/.trae-cn/skills/hindsight-coding-agent/` 真实存在（2026-10-12）⇒ **CN 用户级落点坐实，且与 hook 落点同规律（CN=`~/.trae-cn`，非 README 的 `~/.trae`）**；② `~/.trae-cn/` 另有 `builtin_skills/`；③ npx skills 表（intl 口径 `~/.trae/skills`）；④ 社区文档（gotocx/trae-skills 等，2026-03） |
| **Claude Code** | `.claude/skills/` | `~/.claude/skills/` | 🟡 官方文档级（未本机实测） | npx skills 兼容表 + Claude Code 官方文档（skills 机制广泛一致，2025-12 随 Agent Skills 标准发布）。**不在支持范围**（README §8 定稿四家、不计划补齐）⇒ 不排 canary |
| **WorkBuddy** | `.codebuddy/skills/`（🟡 代码级：静态 `WORKSPACE_DATA_DIR_NAME=".codebuddy"`，与 rules 侧实测同规律；**装载待 Mac canary**） | `~/.workbuddy/skills/`（🟡 代码级：品牌目录模型，§三·续；**不读** `~/.codebuddy/skills`——合并单向） | 🟡 代码级（2026-10-12 bundle）；装载实测待 Mac | 同源模型：两者同一 bundle，用户级目录由 product `dataFolderName` 决定（§三·续）；二手一致（CSDN 2026-10-02「把技能包目录放到 `~/.workbuddy/skills/` 下」、腾讯云开发者社区 2026-06-24「放到 `~/.workbuddy/skills/技能名/` 下，重启生效」）；⚠️ 教训：`agent-rules-dirs.md` §5.4——「用户级独立 ≠ 工作区级独立」，两级必须分别实测 |
| **dsh** | **`<git根>/.dsh/skills`**（rank 100，最高）；另认 `<git根>/.agents/skills`（200） | **`~/.dsh/skills`**（rank 400，`DSH_HOME` 默认 `~/.dsh`）；另认 `~/.agents/skills`（500） | ✅ **源码级 + 真机验收**（2026-10-14 源码，`deepseek-ai/deepseek-harness@master`，§三·再续；2026-10-15 用户真机：前缀技能落盘即装载） | `packages/skill/skill-filesystem/src/index.ts`（roots/ranks/扫描/frontmatter/热重载）+ `packages/skill/skill/src/index.ts`（isSkillName/rank 合并/registerProvider）；项目根按最近 `.git` 定位（`findProjectRoot`） |

**CodeBuddy 附带事实（本机 2026-10-12）**：官方市场组件 `~/.codebuddy/skills-marketplace/`（README：100+ 技能；`skills/` 子目录即各技能目录；`version.txt` 为 GUID）。市场技能 SKILL.md frontmatter 字段：`name` / `display_name` / `description`（+`description_zh`/`description_en`）/ `version` / `allowed-tools`；用户自装技能（hindsight-coding-agent）用 Anthropic 简式（`name`+`description`）同样在装 ⇒ **两种 frontmatter 都被接受**。skill = **目录**（`SKILL.md` + `references/` 等附属），不是单文件。

### 三·续、CodeBuddy ↔ WorkBuddy 同源模型（2026-10-12 bundle 代码级坐实）

> 完整模型（含 rules 侧与对 rulemux 的影响）见 [`agent-rules-dirs.md`](agent-rules-dirs.md) §5.5；此处只记 skills 侧结论，不复制正文。

| 问题 | 结论 |
|---|---|
| 两者什么关系 | **同一套代码库的两个品牌**：bundle 内 38 处 `workbuddy` 引用；用户级目录名由 product 配置 `dataFolderName` 决定（`getBrandUserDataDirName()`，兜底 `.codebuddy`） |
| 用户级 skills 谁读谁 | CodeBuddy 读 `~/.codebuddy/skills` **+ `~/.workbuddy/skills`（合并，单向）**；WorkBuddy **只读** `~/.workbuddy/skills`（`resolveUserSkillsDirs()`：仅品牌为 `.codebuddy` 时才并列 `.workbuddy`；重名时「skipped in favor of earlier」） |
| 工作区级 skills 谁读谁 | 两家都读 `<ws>/.codebuddy/skills`（`getProjectSkillsDir()` 用静态 `WORKSPACE_DATA_DIR_NAME=".codebuddy"`，bundle 内仅此一个值） |
| CN 安装特例 | 本机 CN `product.json` 为 `dataFolderName=".codebuddycn"` / `applicationName="buddycn"`，但**实际生效目录仍是 `~/.codebuddy`**（活证：settings.json 钩子、skills、rules 注入均在此且被读取）⇒ 品牌函数与静态常量两套路径并存，**以静态常量为生效口径** |
| 对 rulemux 的含义 | 用户级 skills 要覆盖两家 ⇒ 写**两个**目录（`~/.codebuddy/skills` + `~/.workbuddy/skills`）；双写重叠无害（CodeBuddy 合并读时重名去重）；工作区级维持 `.codebuddy/skills` 单写两家读 |

### 三·再续、dsh 原生 skills 子系统（2026-10-14 源码级坐实）

> 依据：`deepseek-ai/deepseek-harness` @ master（2026-10-14 经 GitHub contents API 读取）——
> `packages/skill/skill-filesystem/src/index.ts`（本地文件系统 provider）+
> `packages/skill/skill/src/index.ts`（注册表核心：`isSkillName` / rank 合并 / `registerProvider`）。
> ⚠ 与 rules 侧（dsh 无规则目录、纯插件注入，见 [`agent-rules-dirs.md`](agent-rules-dirs.md) §一）不同：
> **skills 是 dsh 的原生子系统，不是插件专属能力**；插件 `ctx.skills` 只是第二条轨。

| 问题 | 结论（源码级） |
|---|---|
| 根目录与优先级 | rank **数值小者胜**（`compareIndexedCandidates`：rank → providerOrder → localOrder；同名后到者 logger.warn 丢弃）：项目级 `<git根>/.dsh/skills`=**100**、`<git根>/.agents/skills`=200、自定义目录（`customSkillDirs`）=300；用户级 `~/.dsh/skills`=**400**（`DSH_HOME`，默认 `~/.dsh`）、`~/.agents/skills`=500（`DSH_AGENTS_HOME`，默认 `~/.agents`）；bundled=600；插件运行时注册（`ctx.skills.registerProvider`）=250 |
| 扫描方式 | **非递归**：每根一次 `readdir`，只认顶层条目；二级子目录里的技能找不到 |
| 技能形态 | 两种都认：`<根>/<技能名>/SKILL.md`（目录包，可带 references/scripts）与 `<根>/<技能名>.md`（扁平单文件） |
| frontmatter 契约 | `name` + `description` 必填（非空字符串）；可选 `whenToUse`；调用策略只认 `disable-model-invocation` / `user-invocable` 两个**连字符**键 |
| name 校验 | `isSkillName` = `^[a-z0-9]+(?:-[a-z0-9]+)*$`，**只校验 frontmatter 的 `name`，不校验目录名** ⇒ `__rulemux__<技能名>/` 前缀目录可被发现（发现名取自 frontmatter），rulemux 现有目录前缀引擎零改动兼容 |
| 旧式驼峰键 | frontmatter 出现 `disableModelInvocation` / `modelInvocable` / `userInvocable`（camelCase）⇒ `parseInvocationPolicy` 抛错 ⇒ **整个技能被静默丢弃**（logger.warn）。CodeBuddy 不报此错 ⇒ 跨 agent 复用技能时的高频坑（只警示、不改写内容） |
| 未知字段 | 其余字段（`display_name` / `description_zh` / `version` / `allowed-tools` 等 CodeBuddy 市场扩展）被忽略，无害 |
| 热重载 | 每根挂 chokidar watcher（`depth:1`、`awaitWriteFinish` 200ms、poll 100ms、默认跟随软链），add/addDir/change/unlink/unlinkDir 事件触发失效重发现；另有宿主写操作观察（`fs/observed`）⇒ 会话中新增/修改技能无需重启（嵌套 SKILL.md 的 watcher 覆盖时序留 canary） |
| 项目根定位 | `findProjectRoot`：从 cwd **逐级向上找最近的 `.git`**，找不到回退 cwd ⇒ dsh 眼里的「工作区」= git 根，不一定是会话 cwd |
| 插件通道 | `ctx.skills.registerProvider` 注册的 SkillProvider（list/get）与文件系统扫描汇入同一技能表、**按 rank 合并**：插件注册（250）打得过用户级目录（400/500）、打不过项目级目录（100/200） |

**对 rulemux 的含义**（设计见 [`../features/skill-sync.md`](../features/skill-sync.md) §七）：工作区落 `<git根>/.dsh/skills`（rank 100，托管拷贝压过 `.agents/skills` 里的同名手工技能）、用户级落 `~/.dsh/skills`（400）；**`.agents` 两个目录只读不写**——跨框架公约目录（npx skills 的 canonical store 就在 `~/.agents/skills`），且写了也无收益（rank 更低）。

---

## 四、SKILL.md 格式与加载语义（与 rules 的本质差异）

| 维度 | rules（已交付） | skills（本节） |
|---|---|---|
| 加载时机 | 会话开始**静态全量**读入（static prefix） | **渐进式披露**：会话开始只载 name+description；模型按名调用时才读 SKILL.md 全文 |
| 载体 | 单文件（`.md`） | **目录**（`<skill-name>/SKILL.md` + 可选 scripts/references/assets） |
| 调用方式 | 无需调用，常驻 | 模型按 description 匹配后按名调用 |
| 对 rulemux 的含义 | Tier-1 真实拷贝进原生目录即满足 A1–A3 | 同样只能走「真实拷贝进原生 skills 目录」；**hook 注入全文会撑 token 且破坏按名调用机制**（Tier-2 思路对 skills 不适用） |

**SKILL.md 大小写不敏感**（bundle `SKILL_FILE_NAMES=["SKILL.md","skill.md"]`）：安装包校验 `[INVALID_PACKAGE] SKILL.md not found`；市场安装走 `installSkill`（按 skill 名加锁 `install-skill-<name>`，默认装 `userSkillsDir`）。

**待 canary 证实的线索**：CodeBuddy genie 产物含 `"skills-changed"` 事件字符串 ⇒ 疑似 skills 热重载（若坐实，则 skill 变更**不需要** rules 那种「请新开会话」提示）；Trae/dsh 的会话内重读行为未知。均列为后续实现任务的 canary 项。

---

## 五、用户级 rules 目录现状（「全局注入」升级的前置）

规则侧要做全局注入，需要先坐实各家**用户级规则目录**：

| Agent | 用户级 rules 落点 | 状态 |
|---|---|---|
| Claude Code | `~/.claude/CLAUDE.md`（用户级 memory，官方文档）；`~/.claude/rules/` 形式未坐实 | 🟡 **不在支持范围**（README §8），不排 canary |
| CodeBuddy | **`~/.codebuddy/rules`**（2026-10-12 bundle 代码级坐实：`userBasePath=join(homePath,USER_DATA_DIR_NAME,"rules")`，是 `DefaultRuleManager(project, user, hidden)` 三路径之一、带文件监听、`RuleScope.USER` 有权限检查；与项目级同源 ⇒ 预期同为「非隐藏 `.md` + `alwaysApply`、合并加载」，待 canary） | ✅ 代码级（2026-10-12） |
| Trae | 🔴（`~/.trae-cn/` 下未见 rules 目录；有 `builtin/`/`memory/`） | 🔴 |
| dsh | `$DSH_HOME/rules`（既有记录，`agent-rules-dirs.md` §一） | 🟡 二手转引 |
| WorkBuddy | `~/.workbuddy/rules`（🟡 同源模型推断：用户级目录按品牌分，§三·续；其 `~/.workbuddy/settings.json` 已实测独立） | 🟡 推断，待 Mac canary |

---

## 六、未核实项清单（🔴，全部挂任务 `20261012-1724-skill-注入调研与设计-5731` 的 worklog 待办节）

1. **WorkBuddy skills 两级装载实测（需 Mac）**：落点已代码级确定（用户级 `~/.workbuddy/skills`、工作区级 `.codebuddy/skills`，§三·续），待实测**装载**；并反证「WorkBuddy 不读 `~/.codebuddy/skills`」（合并单向，代码级）；用户级 rules `~/.workbuddy/rules` 同样待证。
2. ~~**dsh skills 官方复核**~~ —— 2026-10-14 **源码级坐实**（§三·再续）：目录 / rank / 非递归 / frontmatter 契约 / 热重载 / git 根定位全部落实，用户级 `~/.dsh/skills`（`DSH_HOME` 可覆盖）已知。**2026-10-15 真机验收通过**（用户实测：`__rulemux__` 前缀技能落盘即装载，canary 项①销）。**剩余 🔴**：② 会话中 sync 的热重载时序（watcher `depth:1` 对嵌套 `SKILL.md` 的覆盖，未单独验证）；③ `~/.dsh/skills` 用户级装载（源码级坐实，未单独实测）；④ `DSH_HOME` 被设置时 rulemux 写死 `~/.dsh` 的错位（与 codebuddy 的 `CODEBUDDY_CONFIG_DIR` 同类，先文档标注）。
3. **Trae 工作区 skills 目录** `.trae/skills/`：社区文档一致，未本机 canary；国际版 `~/.trae/skills` 未探。
4. **skills 加载行为 canary**：渐进式披露是否如预期（启动只载 name+description）；`skills-changed` 是否热重载；skill 新增/变更要不要「新开会话」提示。

**已销项**：

- ~~Claude Code skills canary~~ —— **不在支持范围**（README §8 定稿四家、不计划补齐），不再排期。
- ~~CodeBuddy 用户级 rules 落点~~ —— 2026-10-12 bundle 代码级坐实 `~/.codebuddy/rules`（§五）；合并加载策略待 canary（随实现任务）。
- ~~CodeBuddy 官方市场第三方源接入~~ —— 2026-10-12 已答：**不支持**（单一官方 git 仓库、293 技能全在库内、无外部源 URL，§一/§二）⇒「市场贡献与跟进」（S3）由 rulemux 自管，不借宿主机制。
