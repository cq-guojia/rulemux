# 各 agent 工作区规则目录（外部事实）

> **类型**：外部事实（上游 agent 侧）
> **适用版本**：见下表「适用版本」列；上游升级后据此复核
> **状态**：🟡 **部分核实** —— Claude Code 已于 2026-10-07 经官方文档核实（hooks 执行二进制 + 规则加载时机，见 §三）；CodeBuddy 走源码级核查（本机 4.12.1）；其余各家仍待补，**除 §三 已核实项外不得作为实现依据**
> **来源**：前期调研（原根 `DESIGN.md` §3）+ 本机 CodeBuddy 安装目录源码核查 + Claude Code 官方文档（`code.claude.com/docs/en/hooks`、`/memory`，2026-10-07）
> **配套**：[`../features/dir-sync.md`](../features/dir-sync.md)（我方怎么适配）· [`../../PROGRESS.md`](../../PROGRESS.md)（核实任务）

> ⚠️ 本文件只记**上游读取能力**（目录 / 扩展名 / 读不读全部）。
> 「我方据此怎么落文件」是设计，归 [`../features/dir-sync.md`](../features/dir-sync.md)，**不写在这**。

## 一、总表（除 CodeBuddy 外均待核实）

| Agent | 工作区规则目录 | 扩展名 / 结构 | 读目录全部？ | 适用版本 | 备注 |
|---|---|---|---|---|---|
| **CodeBuddy** | ⚠️ **存疑**，见 §二 | `.mdc` | 待定 | 4.12.1（本机） | 有「用户级 / 项目级」两类规则 |
| Claude Code | `.claude/rules/` | `.md` | ✅ 全部 | v2.0.64+ | — |
| Trae | `.trae/rules/` | `.mdc` | ✅ 递归读，最多 3 层 | 待补 | 需 frontmatter |
| WorkBuddy | `.codebuddy/rules/`（复用 CodeBuddy 机制） | 同上 | ⚠️ 固定结构 | 待补 | — |
| Codex | ❌ 无目录 | 单文件 `AGENTS.md`（沿目录树向上合并，每目录最多一个） | ❌ | 待补 | — |
| OpenCode | ⚠️ 非目录扫描 | `AGENTS.md` + `opencode.json` 显式列 instruction 文件 | ❌ | 待补 | — |
| DeepSeek Harness | 待确认 | — | — | 待补 | TODO |

> ⚠️ **扩展名 / 结构各家不同** ⇒ 同步器必须**按 agent 分别落格式**，不能一个 `.md` 通吃。

## 二、CodeBuddy 已核实事实（本机 4.12.1）

**核查路径**：`/root/.codebuddy-server-cn/bin/stable-757a5b2f…/extensions/genie/`（打包产物）+ `/root/.local/share/CodeBuddyExtension/`。

### 已推翻的旧结论

| 旧结论（前期调研） | 核查结果 |
|---|---|
| 每条规则 = 子文件夹 + **`RULE.mdc`** | ❌ **证伪**：全盘搜索 `RULE.mdc` **0 命中**（`find / -name "RULE.mdc"` 无结果） |
| 目录固定为 `.codebuddy/rules/` | ⚠️ **存疑**，见下 |

### 已确认

| 项 | 结论 | 出处 |
|---|---|---|
| 规则文件扩展名 | **`.mdc`**（官方 generate-rules 提示词要求） | `product.json:753` |
| frontmatter 字段 | **`alwaysApply`（bool）+ `description`（string）**；示例 `---\nalwaysApply: true\n---` | `product.json:753` |
| `globs` 字段 | ❌ 无据（本机 0 处出现，勿当真） | — |
| 总开关 | `enableWorkspaceRules`（自动读取） | `package.json` / `package.nls.json:64` |
| 规则层级 | 存在**用户级** + **项目级**两类（4.0.0 起） | `CHANGELOG.md:441`、`l10n/bundle.l10n.en.json:145-146` |
| 工作区配置根 | `.codebuddy/`（技能落 `.codebuddy/skills/`，与规则目录是两套东西） | `product.json:926-932` |

### 仍未确定（阻塞第一批实现）

1. **目录名冲突** —— 官方设置文案写 **`.rules`**（`package.nls.json:64`："Automatically read smart rules from the `.rules` directory"），但打包代码里确有 **`.codebuddy/rules`**（`index.js:69`、`index.js:84`）。二者哪个是运行时真值，未定。
2. 是否递归扫描子目录、有无层数 / 大小 / 数量上限。
3. 加载时机（打开工作区 / 重载窗口 / 每轮）与改动后是否热更新。
4. 缺 frontmatter 时的降级行为（不加载 / 报错）。
5. 用户级规则的落盘位置与「合并 or 覆盖」策略。

> **为什么卡住**：加载器在 `index.js` 第 69 行（单行 259 万字符），只读检索工具取不到上下文；官方文档站是 VitePress SPA，抓取只返回空壳。**源码与文档两条路都走死了 ⇒ 下一步只能实测**（见 [`../../PROGRESS.md`](../../PROGRESS.md) 未决项）。

### 一条容易误判的线索

`.codebuddy/rules/tcb/rules/<规则名>/rule.md` —— 这是 **CloudBase（tcb）技能自己的私有约定**（写在它的提示词里让模型去读），**不是 CodeBuddy 的通用规则机制**。别拿它当规范。

---

## 三、Claude Code 已核实事实（官方文档 2026-10-07）

> **来源**：Claude Code 官方文档 `https://code.claude.com/docs/en/hooks` 与 `https://code.claude.com/docs/en/memory`（抓取于 2026-10-07）
> **适用版本**：文档未标注具体版本号；按文档当前内容记录，上游升级后复核

### 3.1 hook 能执行我们的二进制（命门 a，已解）
- SessionStart 等会话级事件支持 `type: "command"`，即「run a shell command」。
- 两种写法：
  - Shell 形式：`"command": "rulemux sync"` → 交给 `sh -c`（mac/linux）执行。
  - **Exec 形式**：`"command": "rulemux", "args": ["sync"]` → `rulemux` 在 PATH 上解析为可执行文件，**不经 shell 直接 spawn**。
- ⇒ 编译型单二进制（Go/Rust 的 `rulemux`）完全可被 hook 调起，无需 Node/VM。
- ⚠️ Windows：exec 形式要求 `command` 是真实可执行文件（`.exe`）；npm 的 `.cmd`/`.bat` 垫片不行，须走 shell 形式或直接给 `.exe`。我们发 `rulemux.exe` 即可。
- 相关事件齐全：`SessionStart` / `PreCompact` / `PostCompact` / `InstructionsLoaded` / `FileChanged` / `ConfigChange` 等 30+ 种。

### 3.2 规则加载时机（命门 b，已解 + 已知边界）
- `.claude/rules/` 中**无 `paths` frontmatter** 的规则，在**会话开始**一次性读入上下文（优先级同 `.claude/CLAUDE.md`）；项目根 `CLAUDE.md`、用户级 `~/.claude/CLAUDE.md` 同理。
- 每个会话从全新上下文开始；读入后留在上下文，**不会每轮重读磁盘**；发生 `/compact` 时项目根 `CLAUDE.md` **会重新从磁盘读取并注入**，子目录/路径规则之后按需重载。
- **对 rulemux 的含义**：
  - 文件**持久化**在原生规则目录 ⇒ 每个新会话（及每次 `/compact` 后）必然加载 ⇒ 满足 Tier-1 / A1–A3（不淡出、不累积、token 恒定）。
  - SessionStart hook 当轮写入「会话开始即加载」位置时，**通常下个会话才生效**（加载已先发生于 hook 之前，时序官方未文档化）→ 但文件已落盘，下轮必读，无功能缺失。
  - 同会话即时新鲜度（改了中央规则想本轮就见）非保证；以 `PreCompact` 再 copy 一次兜底，或重开会话即可。
- **验证手段**：官方提供 `InstructionsLoaded` hook（可观察哪些规则被加载）与 `/context`（查看已加载 Memory）。→ 这正好是我们 `features/verification.md` 的 canary 实测法要落地的对象，**不当场猜，实测为准**。
