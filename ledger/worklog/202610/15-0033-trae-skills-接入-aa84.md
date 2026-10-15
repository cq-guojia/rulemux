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

- [ ] Trae 工作区 skills 目录 `.trae/skills` 本机 canary（存在性 + 渐进式披露装载）；
      CN 装机规律是用户级走 `~/.trae-cn`（rules 侧已坐实），skills 侧按同规律推断，需实测
- [ ] Trae CN 用户级 `~/.trae-cn/skills` **装载** canary（目录存在已坐实，装载未验）；
      国际版 `~/.trae/skills` 探测（registry 现不支持候选目录探测 ⇒ 与 HookDirEnv 同决策：
      先写死 CN 口径，intl 留后续）
- [ ] registry：Trae 填 `SkillsDir=".trae/skills"` + `UserSkillsDirs=["~/.trae-cn/skills"]`
      （CN 口径；注释写明依据与 intl 未接的原因）
- [ ] SKILL.md 契约确认：Trae 是否同为 Anthropic 简式 frontmatter（npx skills 兼容表有 trae，
      预期同类；name 命名规则/调用键差异待核——dsh 的严式校验是前车之鉴）
- [ ] 触发：Trae hook 已装 ⇒ `rulemux sync --hook --agent trae` 天然覆盖 skills，
      验证 create 门禁与 cascade 对 skills 的作用（预期零新机制）
- [ ] 文档回写：external/agent-rules-dirs.md 与 skill-ecosystem.md（Trae 行升实测）、
      skill-sync.md（v2 范围）、README 中英矩阵；顺带销 external §六-3 的 🔴
- [ ] 测试：registry 断言 + smoke Trae skills 步 + doctor/init 口径

## 下一步

1. **先读实现本体再动手**（AGENTS.md §4）：列本机 `~/.trae-cn` 目录结构，找 Trae 的 skills
   扫描实现（安装产物里搜 `skills` 目录拼接逻辑），确认 `.trae/skills` 与 `~/.trae-cn/skills`
   是否原生扫描、是否递归、认哪些 frontmatter——结论带 `文件:行号` 或产物路径；
2. 读不到实现本体 ⇒ 退而 canary：在临时工作区建 `.trae/skills/__rulemux__canary-skill/SKILL.md`
   （frontmatter 带暗号 description），开 Trae 会话验证是否出现在技能面板且可按名调用；
3. 坐实后按「待办」顺序落地（registry → 触发验证 → 文档 → 测试）。

## 过程

<!-- 踩坑、定位、证据。带 revision 和 文件:行号 或符号名（行号会漂移）。 -->

## 结论

<!-- 收尾时填：做成了什么、留了什么坑。 -->
