# 20261015-0033-trae-skills-接入-aa84：Trae skills 接入

> **创建**：2026-10-15 00:33:47（@GuoJia-Agent）

## 背景

v2 任务（`20261013-1133-…-eec8`，已结案）明确移交的两条线之一。用户 2026-10-15 原话：

> 「这两个东西没什么关系，单独建成两个吧：一个是 Trae 的 skill 介入；一个是 git 和远程的 skill 源。」

排序背景：用户 2026-10-12 定序「Trae → DeepSeek harness」，dsh 已于 2026-10-14/15 交付并真机验收
（CLI/插件 0.5.0），Trae 是 skill 注入的下一家。已知事实（external/skill-ecosystem.md §三）：

- Trae CN 用户级 skills 落点 `~/.trae-cn/skills` **本机实测坐实**（2026-10-12，装有真实技能）；
- 工作区级 `.trae/skills` 仅社区文档一致（gotocx/trae-skills 等），**未本机 canary**；
- 国际版 `~/.trae/skills` 未探测；
- Trae 已有 SessionStart hook（`~/.trae-cn/hooks.json`，2026-10-09 坐实）⇒ 触发机制现成，
  sync 顺带同步 skills 即可，**不需要** dsh 那样的插件。

## 待办

- [ ] **真机 canary（待用户）**：Trae 工作区 `.trae/skills` 本机装载 canary（存在性 + 渐进式披露）；
      CN 用户级 `~/.trae-cn/skills` 的「推过去的 `__rulemux__*` 技能是否真出现在面板/可按名调用」亦待真机确认
      （目录存在已坐实，但装载未亲眼验）。⇒ 见结论节，用户开一次 Trae 会话即验；
      国际版 `~/.trae/skills` 探测留后续（registry 不支持候选目录探测）
- [x] registry：Trae 填 `SkillsDir=".trae/skills"`（🔴 工作区级待 canary）+ `UserSkillsDirs=["~/.trae-cn/skills"]`（✅ 用户级坐实），注释区分
- [x] SKILL.md 契约确认：Trae 同为 Anthropic 简式 frontmatter（name+description，大小写不敏感），与 codebuddy/dsh 一致；本机 `~/.trae-cn/skills/hindsight-coding-agent/SKILL.md` 即实证
- [x] 触发：`rulemux sync --hook --agent trae` 天然覆盖 skills（sync 主循环自动遍历 SkillsDir/UserSkillsDirs），零新机制；create 门禁经 hook 点名走通
- [x] 文档回写：agent-rules-dirs.md §二·续 + skill-ecosystem.md §三 Trae 行 + skill-sync.md（范围/字段/canary 状态）+ README 中英矩阵（trae 表/Skills directory 列、§5.4、Where-they-land、init 注释、verification 段均已同步）
- [x] 测试：registry_test（trae SkillsDir/UserSkillsDirsAbs）+ verify_test（skills canary 写入/清理）+ smoke.sh（Trae 双域同步 + verify 探针）；doctor 已自动按字段生成提示，init 仅更新 help 注释
- [x] verify 增强：新增 engine skill canary（WriteSkillCanary/CleanSkillCanary），verify 种/清 canary 遍历工作区 skills 目录，补齐 skills 验收链路

## 下一步

1. **先读实现本体再动手**（AGENTS.md §4）：列本机 `~/.trae-cn` 目录结构，找 Trae 的 skills
   扫描实现（安装产物里搜 `skills` 目录拼接逻辑），确认 `.trae/skills` 与 `~/.trae-cn/skills`
   是否原生扫描、是否递归、认哪些 frontmatter——结论带 `文件:行号` 或产物路径；
2. 读不到实现本体 ⇒ 退而 canary：在临时工作区建 `.trae/skills/__rulemux__canary-skill/SKILL.md`
   （frontmatter 带暗号 description），开 Trae 会话验证是否出现在技能面板且可按名调用；
3. 坐实后按「待办」顺序落地（registry → 触发验证 → 文档 → 测试）。

## 过程

- 2026-10-15（本会话）：按 AGENTS.md §4「先读实现本体再动手」，完成 Trae skill 推送的实现本体核查：
  - 本机已装 Trae CN（用户级 `~/.trae-cn`），`~/.trae-cn/skills/` 用户级目录**确实被扫描并装载**
    （里头已有 `hindsight-coding-agent/SKILL.md` 且被 hindsight 当真实技能用）；SKILL.md 契约 =
    Anthropic 简式（`name`+`description`，大小写不敏感）—— 与 codebuddy/dsh 一致。
  - rulemux `registry.go` 的 trae 此前**只声明了 RulesDir（`.trae/rules`），未声明 SkillsDir/UserSkillsDirs`**
    ⇒ sync 完全不碰 trae 的 skills。
  - `internal/cmd/sync.go:263-288` 的 sync 主流程对每个 agent 自动遍历 `SkillsDir`（工作区）+ `UserSkillsDirs`
    （用户级）调 `SyncSkills`；hook 点名 trae 时 create=true（`sync.go:149-152`）。
    ⇒ 只需给 trae 补这两个字段，sync 即自动推送，不动 cmd、不动 hooks.json（已挂 `rulemux sync --hook --agent trae`）。
  - 工作区级 `.trae/skills` **仍 🔴 未本机 canary**（仅社区文档一致）；用户级 `~/.trae-cn/skills` 已坐实。
  - 本机 config `~/.rulemux/config.toml` 的 `[[skill_source]]` 均未写 agents ⇒ 全 agent 生效；给 trae 加目录后
    `base` 组（user scope）会自动投到 `~/.trae-cn/skills`，**不改 config**。
  - `verify.go:106` 只对 `RulesDirAbs` 种 canary，**不验 skills** —— 这是 onboarding 的洞。
  - 用户拍板：工作区级照加（🔴 待 canary）、verify 的 skills canary 一并做、intl `~/.trae` 本轮不接。
- 实现（2026-10-15）：把 Trae 当一等公民完整接入 skills 双域，并补齐 skills 验收链路：
  - `registry.go`：trae 加 `SkillsDir=".trae/skills"`（🔴 待 canary）+ `UserSkillsDirs=["~/.trae-cn/skills"]`（✅ 坐实），注释区分。
  - `engine/canary.go`：新增 `SkillCanaryDirName` + `WriteSkillCanary` + `CleanSkillCanary`（工作区技能探针，非用户级，与 rules canary 同口径）。
  - `verify.go`：种/清 canary 遍历工作区 skills 目录；提示语加 skills 验收问句。
  - `init.go` help 注释：trae 已接 skills。
  - 测试：`registry_test.go`（trae SkillsDir/UserSkillsDirsAbs 断言）、`verify_test.go`（skills canary 写入/清理）、`smoke.sh`（Trae 双域同步 + verify 探针）。
  - 文档回写：`skill-ecosystem.md` §三 Trae 行、`agent-rules-dirs.md` §二·续、`skill-sync.md` 范围/字段/canary 状态、`README.md` 中英矩阵（trae 表/Skills directory 列、§5.4、Where-they-land、init 注释、verification 段）。
  - `doctor.go` 已自动按 `SkillsDir`/`UserSkillsDirs` 生成提示，无需改。

## 结论

- 做成了：Trae（CN）已完整接入 rulemux skills 双域——`rulemux sync --hook --agent trae` 现在会自动把技能推到
  `~/.trae-cn/skills`（用户级，✅ 坐实）与 `.trae/skills`（工作区，🔴 待你真机 canary）；`rulemux verify --agent trae`
  也能种一颗技能探针验收装载。代码、测试、文档全绿，并已回写 worklog 与全部外部文档 + README。
- 留的坑 / 待你验收：
  1. **工作区级 `.trae/skills` 是否真被 Trae 扫描 = 🔴 未本机 canary**：请开一次 Trae 会话（触发 `rulemux sync --hook --agent trae`），
     看 `~/.trae-cn/skills/` 下是否出现 `__rulemux__*` 技能、`.trae/skills/` 是否出现 `rulemux verify` 种的
     `__rulemux__canary-skill` 且能在面板按名调用。若工作区级不生效，回退只留用户级。
  2. intl `~/.trae` 本轮不接（未探测）；claude/codex/opencode 仍不接 skills。
