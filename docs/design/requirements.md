# rulemux 产品需求（真源）

> **状态**：✅ 已定（2026-10-07 逐项拍板，结论进 [`implementation.md`](implementation.md)）
> **来源**：用户立项诉求（原根 `DESIGN.md`，已分解迁入本文件）
> **配套**：[`architecture.md`](architecture.md)（为什么这么设计）· [`features.md`](features.md)（怎么做）· [`../progress.json`](../progress.json)（在办任务）

---

## 一、一句话需求

**用户在配置里列出的一组自有规则文件 → 投递到各家 AI coding agent**：一处管理、多处生效，效果与 token 与直接在每个工作区写 `AGENTS.md` 一致。

> 没有"中央真源"：rulemux 不拥有、不维护规则内容，只负责把用户配置所列的源文件复制 / 注入进各 agent 目录（见 [`implementation.md`](implementation.md) #4/#5）。

## 二、产品形态

- **形态**：命令行工具（CLI，`rulemux`）。
- **输入**：一份 **TOML 配置**（`~/.rulemux/config.toml`，目录可任意指定，非必须工作区），其中 `[[source]]` 列出若干源文件（磁盘任意位置的 `.md` / `.txt` 等文本）+ 目标 agent。
- **输出**：把每个源文件同步进各 agent 的工作区规则目录（Tier-1 真实拷贝；Tier-2 走 hooks 注入）。

## 三、验收标准（满足 / 不满足的判据）

| # | 标准 | 判据 |
|---|---|---|
| A1 | **效果等价** | 与直接写 `AGENTS.md` 等价 |
| A2 | **token 相当** | 与直接写 `AGENTS.md` 相当，不累积、不膨胀 |
| A3 | **永不淡出** | 不随对话老化 / 压缩而丢失（Tier-1 目录同步满足；Tier-2 hooks 注入视为降级，不保证 A3） |
| A4 | **不用软链** | 一律真实拷贝 |
| A5 | **加删自由** | 配置加 / 删源文件，目标目录跟着变（带前缀 + 删残留保证） |
| A6 | **开源通用** | 不绑某一家 agent |

> ⚠️ **A1–A3 是硬指标**：Tier-1 必须满足；Tier-2（单文件 agent 走 hooks 注入）启用即视为该 agent **降级、未达 A3 验收**。每个 agent 接完都要按 [`features/verification.md`](features/verification.md) 实测。

> **例外登记（2026-10-09）**：Tier-1 的 SessionStart hook 在检测到规则**有变化**（新增 / 修改 / 删除任一）时，会注入**恰好一条瞬态提示**（提示用户重开会话生效）。
> 该提示**不含规则正文、只发一次、无变化零输出**，**不参与 A1–A3 对"规则投递"的判定**（规则仍 100% 走原生目录）；仅 §五"不把注入当主方案 / hook 只当投递员"这一条因此获得一个显式例外。
> 详见 [`features/sync-all-and-change-notice.md`](features/sync-all-and-change-notice.md) §六。

## 四、支持范围

- **Tier-1（有原生规则目录 ⇒ 真实拷贝丢文件）**：CodeBuddy、WorkBuddy、Trae。
  Claude Code 的适配器已注册但尚未 canary 坐实，暂不可安装。
- **Tier-2（没有 rulemux 写的规则目录 ⇒ 走注入交付，不碰用户文件）**：Codex、OpenCode（已注册，尚未核实）
  与 **DSH**（2026-10-10 纳入，**2026-10-14 由 Tier-1 改为注入交付**：原先 Go 把真实拷贝写进 `.dsh/rules`、
  再由仓库子包插件读回，而那个目录宿主根本不认、只是两边之间的交接物，清不干净就永久留在用户盘上 ⇒ 现改为
  插件每个会话自己调用 `rulemux inject --agent dsh` 取渲染文本注入，工作区零产物，见
  [`features/dir-sync.md`](features/dir-sync.md) §DSH）。

各家的工作区规则目录与读取行为见 [`external/agent-rules-dirs.md`](external/agent-rules-dirs.md)。

## 五、明确不做

- **不改规则内容**：不做改写 / 模板渲染，源文件写什么就落什么（格式适配只动文件组织方式，不动正文）。
- **不把注入当主方案**：hook 注入落在动态区，做不到 A3；仅对**无工作区规则目录的单文件 agent（Tier-2）**启用降级，且**不碰用户自己的 `AGENTS.md`**（由 SessionStart hook 注入上下文）。启用即视为该 agent 未达验收标准（见 [`features/hook-injection.md`](features/hook-injection.md)）。
- **不常驻、不监听**：无守护进程、不监听文件改动，极致简单（见 [`implementation.md`](implementation.md) #8）。
- **不做自动更新、不做自更新**：升级一律**交给包管理器、由用户手动执行**（`go install github.com/cq-guojia/rulemux@latest`，将来 brew / scoop / apt 同理）。—— 2026-10-08 拍板。
  依据：核心链路是「SessionStart 钩子调 `rulemux sync`」，而各家钩子都有超时（本机实测 Hindsight 的钩子均为 30–60s），在热路径里联网自更新会拖慢甚至挂住会话；且自我替换二进制一旦失败，后续所有同步都被锁死。收益也有限：rulemux 无后台状态、无服务端，版本差异通常只影响边缘行为。
  补充：即便日后加 `rulemux update`，也须是**手动触发 + 校验 checksum/签名**，绝不放在钩子路径里自动跑。

---

## 六、skill 注入（2026-10-12 立项草案，🟡 待拍板）

> **状态**：🔴 草案 —— 2026-10-12 用户提出，调研与论证已完成并回写（`external/skill-ecosystem.md`、任务 `20261012-1724-skill-注入调研与设计-5731`），**本节未经逐项拍板，不得作为实现依据**。
> **来源**：用户 2026-10-12 诉求（原话见任务 worklog「背景」节）。
> **配套**：[`external/skill-ecosystem.md`](external/skill-ecosystem.md)（上游事实）· [`features/skill-sync.md`](features/skill-sync.md)（方案草案）

### 6.1 需求归并（用户四点）

| # | 需求 | 要点 |
|---|---|---|
| S1 | **skill 实时分发（核心）** | 沿用规则注入逻辑：配置文件声明 skill 源，经 SessionStart hook（`rulemux sync --hook`）把 skill **真实拷贝**进各 agent 的原生 skills 目录 |
| S2 | **全局注入升级** | 现有注入只到工作区；升级为**规则 + skill 同时**支持注入到 agent **用户级（全局）**目录，与工作区级并存、由配置声明作用域 |
| S3 | **市场贡献与跟进** | 管理自己的源（本地目录 / git 仓库），并能**跟进**别人开发的 skill（如 GitHub 上的）的上游更新 |
| S4 | **自定义 skill 管理** | 管理自己的 skill：创作、组织、分发、删除 |

### 6.2 验收判据草案（A1–A6 向 skill 的延伸）

| # | 标准 | 判据 |
|---|---|---|
| B1 | **效果等价** | 与手动把 skill 目录放进 `.claude/skills` 等原生目录等价——模型能按名调用 |
| B2 | **不破坏渐进式披露** | 同步只落目录、不经 hook 注入全文；启动期只暴露 name+description（与手放一致，token 不膨胀） |
| B3 | **永不淡出 + 不用软链** | 真实拷贝进原生目录；**明确不采用 npx skills 的默认 symlink 模式** |
| B4 | **加删自由** | 配置增删 skill 源，目标目录跟着变；残留 skill 目录被清理（带前缀标识，不误删用户自建 skill） |
| B5 | **全局/工作区双域** | 同一份源可按 agent 声明落工作区级或用户级；两者互不干扰、可并存 |
| B6 | **开源通用 + 不半开放** | 不绑某一家 agent；未实测坐实的 agent（及其目录）标 🔴 不开放安装 |

### 6.3 与现有工具的边界（论证结论草案）

- **复用 npx skills（vercel-labs/skills）作主通道不可行**：交互式安装、按 cwd 落盘（无目标工作区参数）、默认 symlink、无 lockfile、无 hook 实时同步、无配置驱动扇出、无残留清理；且**不覆盖 workbuddy / dsh**（兼容表逐字证据见 `external/skill-ecosystem.md` §二）。
- **可借鉴**：其源写法（GitHub 简写 / git URL / 本地路径 / `.claude-plugin/marketplace.json` 发现）作为 skill 源配置的参照。
- **rulemux 差异化成立**：配置驱动「一份源 → N agent × M 工作区（+ 全局）」+ 真实拷贝 + 残留清理 + hook 实时同步 —— 与规则注入同一套哲学。

### 6.4 暂定不做（待拍板确认）

- **不自建 skill 市场服务端**：市场形态归第三方（SkillsMP / SkillHub / 官方市场），rulemux 只做「源 → agent」的投递。
- **不做 skill 内容生成 / 改写**（同 §五规则侧）。
- **不做版本锁定 / lockfile**：沿用「update = 拉最新」的简单语义；用户若要锁定再议。
