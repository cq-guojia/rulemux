# skill 同步（草案，未拍板）

> **状态**：🔴 草案（2026-10-12 立项，方案结论待用户逐项拍板；实现前置见 §四）
> **配套**：[`../requirements.md`](../requirements.md) §六（需求与验收判据草案）· [`../external/skill-ecosystem.md`](../external/skill-ecosystem.md)（上游事实：各家 skills 目录、市场格局）· [`dir-sync.md`](dir-sync.md)（规则同步主方案，本篇沿用其哲学）

## 一、与规则同步的相同与不同

| 维度 | 规则同步（已交付） | skill 同步（本篇） |
|---|---|---|
| 投递方式 | SessionStart hook 调 `rulemux sync`（投递员） | **相同**——同一 hook、同一命令，顺带同步 skills |
| 落盘方式 | 真实拷贝进原生规则目录 | **相同**（真实拷贝；明确不采用 npx skills 的默认 symlink） |
| 载体 | 单文件 `.md` + `__rulemux__` 前缀 | **目录** `<skill-name>/SKILL.md` + 附属 resources ⇒ 前缀要落在**目录名**上 |
| 加载语义 | 会话开始静态全量 | 渐进式披露（启动只载 name+description，按名调用才读全文）⇒ hook 注入全文不可行，只能落原生目录 |
| 作用域 | 仅工作区 | **双域**：工作区级 + 用户级（全局），由配置声明 |
| 源形态 | 本地文件路径 | 本地目录 / git 仓库（GitHub 简写等，参照 npx skills 源写法）/ 市场索引引用 |

## 二、方案草案（只记结论，不动码）

1. **引擎（`internal/engine/`）**：在现有「文件 + 前缀 + 内容比对 + 删残留」旁增加**目录树同步**：目标名 = `__rulemux__<skill-name>/`，整树内容比对（逐文件），删残留扩展到**目录级**（不在应有集合内的 `__rulemux__*` 目录整树删除）；用户自建（非前缀）skill 目录一律不碰。
2. **适配器（`internal/agents/registry.go`）**：Agent 增加 skills 落点字段——`SkillsDir`（工作区，如 `.claude/skills`）与用户级 skills 目录（如 `~/.codebuddy/skills`、`~/.trae-cn/skills`）；全局注入升级还需**用户级 rules 目录**字段。**未实测坐实的落点一律留空 + 标 🔴，`rulemux init/doctor` 不开放**（沿用现有 Verified 闸门）。注意 Trae CN 的用户级是 `~/.trae-cn/skills`（本机实测），不是 intl 口径 `~/.trae/skills`。
3. **共享目录冲突**：规则侧 `SharingRulesDir` 的并集/收敛机制需平移到 skills 侧（codebuddy 与 workbuddy 的 skills 是否共享目录**未实测**，先按「不共享、各自落」设计，实测后校正——🔴）。
4. **配置（`~/.rulemux/config.toml`）**：新增 `[[skill_source]]`（本地目录 / git 源 / 市场索引引用），复用现有 `agents` / `workspace` / `workspace_groups` / `use` 匹配机制；新增**作用域声明**（工作区 / 全局），规则侧同步加同一字段实现 S2 双域。
5. **hook 与提示语义**：`rulemux sync --hook` 同步规则 + skills；skill 变更**要不要**「请新开会话」提示待 canary——CodeBuddy 产物有 `skills-changed` 事件字符串（疑似热重载），若坐实则 skills 不需要该提示（与规则侧 `SessionHint` 的判据分开）。
6. **命令面（最小集）**：`sync` 默认带 skills（配置里有 `[[skill_source]]` 才动 skills 目录）；源刷新后续加 `rulemux skill list / update`（S3 的「跟进上游」）。本轮不设计交互式安装 UI。

## 三、明确不做（对齐 requirements §6.4）

- 不自建市场服务端；不做 skill 内容生成/改写；不做 lockfile 版本锁定。
- 不做 Tier-2 式 hook 注入 skills（破坏渐进式披露，B2 不满足）。

## 四、实现前置（🔴 未核实项，全部来自 `external/skill-ecosystem.md` §六）

1. WorkBuddy skills 两级目录实测（用户级二手一致指向 `~/.workbuddy/skills/`；工作区级未知）；
2. dsh skills 目录（`.dsh/skills` / `.agents/skills`、不递归）对官方文档复核 + 用户级落点；
3. Trae 工作区 `.trae/skills` 本机 canary + intl `~/.trae/skills` 探测；
4. Claude Code skills 本机 canary（目录文档级已足，会话级验证未做）；
5. 各家**用户级 rules 目录**（S2 全局注入的前提，CodeBuddy 尤其）；
6. skills 加载行为 canary（渐进式披露是否符合预期 / `skills-changed` 是否热重载）；
7. CodeBuddy 官方市场是否支持第三方源接入（决定 S3 能否借宿主机制）。
