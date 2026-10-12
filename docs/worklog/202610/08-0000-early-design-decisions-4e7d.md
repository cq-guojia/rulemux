# 20261008-0000-early-design-decisions-4e7d：早期设计决策结案（T0 / T1 / T2 / T5 / T8 / T12 / T13 / T15）

> **✅ 完成封卷**：2026-10-08 00:00:00（@GuoJia-Agent）
> **创建**：2026-10-08 00:00:00（@GuoJia-Agent）

## 背景
本工作包背景见历史索引摘要（原过程文档未单列背景）。

## 待办
（已结案，原过程文档未单列待办项）

## 下一步
（已结案，无后续动作）

## 过程
需求拍板（无中央真源，源文件由 TOML 列出，implementation.md #4/#5/#6）；CodeBuddy/Trae `.mdc` frontmatter 语义坐实（非隐藏 `.md` + `alwaysApply:true` / Trae `.md` frontmatter 可选、`.mdc` 不加载）；Codex/OpenCode 走 Tier-2 SessionStart 注入、取消受管区拼接（#9/#12）；固定前缀 `__rulemux__` + 删残留算法无 manifest（#10/#11）；实现细节 1–13 拍定（#14 测试 / #15 文档留待实现）；用户 workspace 数组提案＝已实现；hook 落点改 user 级（codebuddy→`~/.codebuddy/settings.json`、workbuddy→`~/.workbuddy/settings.json`，`HookAbs:true`）；新 Agent 接入清单沉淀为 agent-onboarding.md

## 结论
已完成。成果即上方过程所述，亦见历史索引摘要。
