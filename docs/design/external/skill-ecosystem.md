# skill 生态外部事实：市场格局 + 各家 skills 目录

> **类型**：外部事实（上游 agent 侧 + 第三方工具侧）
> **适用版本**：CodeBuddy 4.12.1（本机，2026-10-12）；Trae CN（本机 `~/.trae-cn`，2026-10-12）；vercel-labs/skills README（2026-10-12 抓取）；上游升级后据此复核
> **状态**：🟡 **部分核实** —— CodeBuddy / Trae CN 的**用户级** skills 目录经本机实测坐实（目录存在且装有真实技能）；Claude Code  skills 目录经官方文档 + npx skills 兼容表（交叉一致）；**WorkBuddy / dsh 仅二手来源，用户级 rules 目录全部未坐实 ⇒ 均标 🔴，不得作为实现依据**
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
| **CodeBuddy** | `.codebuddy/skills/` | **`~/.codebuddy/skills/`** | ✅ 本机实测 | ① 本机 `/root/.codebuddy/skills/hindsight-coding-agent/` 真实存在（2026-10-12）；② genie 扩展产物含 `"skills"`/`"skills-marketplace"`/`"skillsService"`/`"skillsManager"`/`"skills/"` 字符串（`~/.codebuddy-server-cn/bin/stable-757a5b2f…/extensions/genie/out/extension/index.js`）；③ npx skills 兼容表；④ 本仓库 `agent-rules-dirs.md` §二（`product.json:926-932`：`.codebuddy/` 下技能落 `skills/`） |
| **Trae（CN）** | `.trae/skills/` | **`~/.trae-cn/skills/`** | ✅ 本机实测（用户级）；工作区级为社区文档 | ① 本机 `/root/.trae-cn/skills/hindsight-coding-agent/` 真实存在（2026-10-12）⇒ **CN 用户级落点坐实，且与 hook 落点同规律（CN=`~/.trae-cn`，非 README 的 `~/.trae`）**；② `~/.trae-cn/` 另有 `builtin_skills/`；③ npx skills 表（intl 口径 `~/.trae/skills`）；④ 社区文档（gotocx/trae-skills 等，2026-03） |
| **Claude Code** | `.claude/skills/` | `~/.claude/skills/` | 🟡 官方文档级（未本机实测） | npx skills 兼容表 + Claude Code 官方文档（skills 机制广泛一致，2025-12 随 Agent Skills 标准发布） |
| **WorkBuddy** | 🔴 未知 | 🔴 `~/.workbuddy/skills/`（二手一致） | 🔴 未核实 | 多篇二手一致（CSDN 2026-10-02「把技能包目录放到 `~/.workbuddy/skills/` 下」、腾讯云开发者社区 2026-06-24「放到 `~/.workbuddy/skills/技能名/` 下，重启生效」；「兼容 OpenClaw 技能生态」）；**本机无 WorkBuddy（macOS 应用）无法实测**；⚠️ 教训：`agent-rules-dirs.md` §5.4——「用户级独立 ≠ 工作区级独立」，两级必须分别实测 |
| **dsh** | 🔴 项目根 `.dsh/skills` 或 `.agents/skills`（**不递归**，直属子目录或扁平 `.md`） | 🔴 未知 | 🔴 未核实 | 阿里云开发者社区 2026-10-03（引「官方 skills 子系统文档」）+ CSDN 2026-08-21 同；另 dsh 插件可经 `ctx.skills` 注册技能（zuozewei 2026-09-27）；dsh 既有全局**规则**目录 `$DSH_HOME/rules`（见 `agent-rules-dirs.md` §一） |

**CodeBuddy 附带事实（本机 2026-10-12）**：官方市场组件 `~/.codebuddy/skills-marketplace/`（README：100+ 技能；`skills/` 子目录即各技能目录；`version.txt` 为 GUID）。市场技能 SKILL.md frontmatter 字段：`name` / `display_name` / `description`（+`description_zh`/`description_en`）/ `version` / `allowed-tools`；用户自装技能（hindsight-coding-agent）用 Anthropic 简式（`name`+`description`）同样在装 ⇒ **两种 frontmatter 都被接受**。skill = **目录**（`SKILL.md` + `references/` 等附属），不是单文件。

---

## 四、SKILL.md 格式与加载语义（与 rules 的本质差异）

| 维度 | rules（已交付） | skills（本节） |
|---|---|---|
| 加载时机 | 会话开始**静态全量**读入（static prefix） | **渐进式披露**：会话开始只载 name+description；模型按名调用时才读 SKILL.md 全文 |
| 载体 | 单文件（`.md`） | **目录**（`<skill-name>/SKILL.md` + 可选 scripts/references/assets） |
| 调用方式 | 无需调用，常驻 | 模型按 description 匹配后按名调用 |
| 对 rulemux 的含义 | Tier-1 真实拷贝进原生目录即满足 A1–A3 | 同样只能走「真实拷贝进原生 skills 目录」；**hook 注入全文会撑 token 且破坏按名调用机制**（Tier-2 思路对 skills 不适用） |

**待 canary 证实的线索**：CodeBuddy genie 产物含 `"skills-changed"` 事件字符串 ⇒ 疑似 skills 热重载（若坐实，则 skill 变更**不需要** rules 那种「请新开会话」提示）；Trae/dsh 的会话内重读行为未知。均列为后续实现任务的 canary 项。

---

## 五、用户级 rules 目录现状（「全局注入」升级的前置）

规则侧要做全局注入，需要先坐实各家**用户级规则目录**：

| Agent | 用户级 rules 落点 | 状态 |
|---|---|---|
| Claude Code | `~/.claude/CLAUDE.md`（用户级 memory，官方文档）；`~/.claude/rules/` 形式未坐实 | 🟡 |
| CodeBuddy | 用户级规则落盘位置与「合并 or 覆盖」策略 | 🔴 **未确定**（本仓库 `agent-rules-dirs.md` §二「仍未确定」第 5 条既有记录；本机 bundle 有 `"rules"`/`"internal-rules"`/`"rulesService"` 字符串，但路径常量未提取到） |
| Trae | 🔴（`~/.trae-cn/` 下未见 rules 目录；有 `builtin/`/`memory/`） | 🔴 |
| dsh | `$DSH_HOME/rules`（既有记录，`agent-rules-dirs.md` §一） | 🟡 二手转引 |
| WorkBuddy | 🔴 | 🔴 |

---

## 六、未核实项清单（🔴，全部挂任务 `20261012-1724-skill-注入调研与设计-5731` 的 worklog 待办节）

1. **WorkBuddy skills 两级目录**：用户级 `~/.workbuddy/skills/`（二手一致）需实测；工作区级完全未知。用户级与工作区级必须**分别**实测（§5.4 教训）。
2. **dsh skills**：项目根 `.dsh/skills` / `.agents/skills` 及「不递归」行为需对官方 skills 子系统文档复核；用户级 skills 目录未知；插件注册（`ctx.skills`）与目录扫描是否并存未知。
3. **Trae 工作区 skills 目录** `.trae/skills/`：社区文档一致，未本机 canary；国际版 `~/.trae/skills` 未探。
4. **Claude Code skills 本机 canary**：目录官方文档级，但未在本工作区跑过会话验证（与 `.claude/rules` 同样的 open bug）。
5. **用户级 rules 目录**（§五全部 🔴 项）：全局注入升级的实现前置。
6. **skills 加载行为 canary**：渐进式披露是否如预期（启动只载 name+description）；`skills-changed` 是否热重载；skill 新增/变更要不要「新开会话」提示。
7. **CodeBuddy 官方市场是否支持第三方源接入**（git 仓库 / marketplace.json）：决定「市场贡献与跟进」需求能否借宿主机制。
