# 实测验证法（每个 agent 跑一次）

> **状态**：✅ 已定（2026-10-07 拍板，canary 文件法；细节见 [`../implementation.md`](../implementation.md) #13）
> **来源**：原根 `DESIGN.md` §6
> **配套**：[`../requirements.md`](../requirements.md)（验收标准）· [`hook-injection.md`](hook-injection.md)（验证失败后的降级）

## canary 四步法（`rulemux verify` / `doctor` 落地的对象）

1. **规则是否进了本会话上下文**：在 agent 规则目录放一个带 `rulemux` 暗号的 canary 文件，**先放文件、再开一个全新会话**，问模型「你看到了 canary 里的暗号吗」——能答出即证明该文件进了本会话上下文（坐实链路）。
   ⚠️ 时序**已坐实为「读规则先于钩子写入」**（2026-10-08 首证、2026-10-09 对照实测复证）：会话开始后才落盘的文件本会话读不到，须再开一个会话 ⇒ 顺序必须是"先放文件、后开会话"。
2. **点文件可读性**：同样放 `__rulemux__canary.md`（点开头隐藏），验证该 agent「读全部 .md」**不跳过点文件**；若跳过 ⇒ 退化为非点前缀 `rulemux__`。
3. **目录结构**：验证平铺 `__rulemux__*.md` 是否被加载（如 CodeBuddy 的目录结构存疑项）。
4. **token 一致性**：对比「rulemux 注入/拷贝」与「直接手写 AGENTS.md」进上下文的 token 数，应在误差内一致（A1/A2 验收）。

## 压缩淡出测试（仅 Tier-1 需过）

1. 埋两个互相独立的随机串：`TOKEN-XYZ` 与 `TOKEN-QRS`；
2. 开新会话问模型：能否念出 `TOKEN-XYZ`（验证**是否进上下文**）；
3. 跑长任务触发一次压缩，再问 `TOKEN-QRS`（验证**会不会被丢掉**）。

**两步都过才算这个 agent 闭环**：

- 念不出 `TOKEN-XYZ` ⇒ 没进上下文（同步没生效 / 该目录根本没被读）；
- 压缩后念不出 `TOKEN-QRS` ⇒ 会淡出 ⇒ 不满足 A3，该 agent 不能走目录同步，退回 [`hook-injection.md`](hook-injection.md)（**降级，不算通过验收**）。

> ⚠️ **压缩后为什么问另一个串**：`TOKEN-XYZ` 在第 2 步已被写进对话，压缩摘要里可能仍留着它 ⇒ 念得出来也可能是**假阳性**。压缩后必须问一个**此前从未在对话中出现过**的串，才能证明「规则此刻真在上下文里」。

## skill 装载 canary（dsh，2026-10-14 挂 v2 任务）

规则 canary 四步法面向「会话开始静态全量加载」；skills 是渐进式披露（启动只载
name+description、按名调用才读全文），验证法不同 —— dsh 以下列四项为准，做一项销一项：

1. **`__rulemux__` 前缀目录真被装载**：往 `<git根>/.dsh/skills` 放一个 `__rulemux__canary-skill/`
   （`SKILL.md` frontmatter 带暗号 description），新开会话输入 `/` 看命令面板 Skills 分组里
   有没有它、按名调用后模型能否念出暗号。源码级判据：发现名取自 frontmatter `name`、目录名不校验
   （`isSkillName`，external/skill-ecosystem.md §三·再续）⇒ 预期通过，此步是坐实而非证伪。
2. **会话中落盘的热重载时序**：会话开始后再触发一次 `rulemux sync --hook --agent dsh`
   （新增/改一个 `__rulemux__` 技能），不问重启、看本会话能否发现。依据：dsh 每根挂 chokidar
   watcher（`depth:1`）；嵌套 `SKILL.md` 变更的覆盖深度是待验点。
3. **用户级装载**：`~/.dsh/skills` 放技能，任意工作区会话都能看到（rank 400 全局根）。
4. **`DSH_HOME` 错位**：设了 `DSH_HOME` 时 rulemux 仍写 `~/.dsh/skills` ⇒ 预期**看不到**。
   若坐实，按 codebuddy `CODEBUDDY_CONFIG_DIR` 的先例决定是否让 dsh 用户级目录跟随 env。

> 通过标准：1/3 不过 ⇒ 落点判据有错，回 external 文档复核；2 不过 ⇒ 技能变更需要「重启 dsh」
> 提示（当前 skill 变化不提示的 shortcut 按 per-agent 判据补）；4 不过 ⇒ 记录为已知限制（先不修）。
