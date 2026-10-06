# Agent Rules Sync — 设计文档

> 一份中央真源规则 → 投递到各家 AI coding agent；效果与 token 等同于直接写 AGENTS.md。
> 要求：永不淡出、不累积、不用软链、加删文件自由、开源通用。

---

## 1. 核心原理（为什么必须这样做）

- 每次 API 请求结构：`system`（静态前缀） + `messages`（对话）。
- Prompt caching 靠**前缀匹配**；`system` 静态前缀被冻结以保缓存 —— 任何每轮变化的内容都会毁掉缓存。
- **hook 注入**（`additionalContext` / `system-reminder`）落在**动态区**：
  - `SessionStart` 注一次 → 随对话老化，压缩时被摘要 → **淡出**；
  - 每轮注（`UserPromptSubmit`）→ 追加累积 → **token 爆炸**；
  - **无法写入冻结前缀**（这是缓存 + 信任模型的设计，不是能力缺陷）。
- **结论**：要"和 AGENTS.md 一模一样"（永不淡出 / 不累积 / 恒定 token） = 必须走
  **harness 原生加载的规则文件/目录**（= 静态前缀语义）。
  **hook 只当"投递员"，不负责注入。**

---

## 2. 一级方案（主选）：目录同步

hook 在会话开始触发（或独立命令）→ 对每个 agent：

1. 定位其**工作区规则目录**。
2. 对真源里**我方每个文档**：
   - 目标不存在 → **copy** 过去（按该 agent 要求的结构 / 扩展名）；
   - 已存在 → 比对 **MD5**：相同跳过；不同则**覆盖**。
3. 其它非我方文档**一律不碰**。

优点：原生加载 = 永不淡出 + 不累积；无软链；加删文件自由。

---

## 3. 各家工作区规则目录（已查证）

| Agent | 工作区规则目录 | 扩展名 / 结构 | 读目录全部？ | 备注 |
|---|---|---|---|---|
| **Claude Code** | `.claude/rules/` | `.md` | ✅ 全部 | v2.0.64+ |
| **Trae** | `.trae/rules/` | `.mdc` | ✅ 递归读，最多 3 层 | 需 frontmatter |
| **CodeBuddy** | `.codebuddy/rules/` | 每条规则 = 子文件夹 + `RULE.mdc` | ⚠️ 固定结构 | 重载项目时自动扫描 |
| **WorkBuddy** | `.codebuddy/rules/`（复用 CodeBuddy 机制） | 同上 | ⚠️ 固定结构 | — |
| **Codex** | ❌ 无目录 | 单文件 `AGENTS.md`（沿目录树向上合并，每目录最多一个） | ❌ | 需退化为"拼接进单文件" |
| **OpenCode** | ⚠️ 非目录扫描 | `AGENTS.md` + `opencode.json` 显式列 instruction 文件 | ❌ | 需改配置列文件 |
| **DeepSeek Harness** | 待确认 | — | — | TODO |

> 扩展名 / 结构各家不同：**同步器必须按 agent 分别落格式**，不能一个 `.md` 通吃。

---

## 4. 备用方案：hook 注入（一级方案不支持时）

- hooks：**`SessionStart` 读一次 + `PreCompact` 压缩前再写一次**；
- **不挂 `UserPromptSubmit`**（会累积爆炸）；
- 一个薄脚本 + 各 agent 的 hooks 配置落点；
- 落点：Claude Code `.claude/settings.json`、CodeBuddy、Trae `hooks.json`、Codex、WorkBuddy（各家均跟 Claude Code hooks 规范）。

---

## 5. 未决 / TODO

- [ ] Trae / CodeBuddy `.mdc` 的 **frontmatter**（`alwaysApply` / `globs` / `description`）怎么写才**无条件常驻**；
- [ ] **Codex**（单文件）/ **OpenCode**（配置列表）的分支处理：拼接进 `AGENTS.md` 受管区 / 改 `opencode.json`；
- [ ] **DeepSeek Harness** 的规则目录确认；
- [ ] 各 agent 实测闭环。

---

## 6. 30 秒实测法（每个 agent 跑一次）

1. 注入一个随机串 `TOKEN-XYZ`；
2. 开新会话问模型：能否念出 `TOKEN-XYZ`（验证是否进上下文）；
3. 跑长任务触发一次压缩，再问一次（验证是否被丢掉）。

---

## 7. 命名（候选，npm 已确认未占用）

- **`rulemux`** ← 首选：mux = 多路复用，一份源 → 多 agent，短且有代表性
- `rulecast`：cast = 广播，把规则分发到各家
- `rulehub`：中央枢纽
- `ruleseed`：把规则播种到各工作区
- `ruleflow`
- 其它可用：`agentrules`、`unirules`、`ruleslot`、`rulefeed`、`rulesink`、`rulewise`、`ruleup`、`rulery`、`ruleseek`
