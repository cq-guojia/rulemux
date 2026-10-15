# 20261008-0000-codebuddy-workbuddy-canary-a1b2：CodeBuddy / WorkBuddy canary 坐实

> **✅ 完成封卷**：2026-10-08 00:00:00（@GuoJia-Agent）
> **创建**：2026-10-08 00:00:00（@GuoJia-Agent）

## 背景
需在 CodeBuddy 工作区验证暗号 `RULEMUX-CANARY-43371345` 能否被念出；并确认用户提出的「workspace 数组 + all/*/unset + 合并→文件清单→增删比对」方案是否已实现。

## 待办
（已结案）Claude 侧 sync 未落地，留为 open bug（T11）；trae/codex/opencode 待实测。

## 下一步
（已结案）

## 过程
CodeBuddy/WorkBuddy 坐实：新会话准确念出暗号；落点 `.codebuddy/rules` 平铺**非隐藏** `.md`（须带 `alwaysApply:true`）才加载、**点文件被跳过**；初始误判「点文件 `.rulemux__` 会被读」已被推翻（落盘前缀改非隐藏的 `__rulemux__`）；「钩子先于读规则」亦被推翻——实际是**读规则先于 hook 写入**，新增文件首会话差一拍。WorkBuddy 复用 CodeBuddy 机制随同视为坐实；代码 `Verified=true`（`internal/agents/registry.go`）。
Claude 未落地：该工作区 `.claude/` 仅 `settings.json`、无 `rules/` 目录，暗号未读到，根因未定。
用户提案确认：用户提的「workspace 数组」方案 = 已实现设计（`implementation.md` #6 + `config.go` 的 `Workspaces`/`SourcesFor`/`MatchesWorkspace`），无需新代码。

## 结论
CodeBuddy/WorkBuddy canary 坐实，代码 `Verified=true`；两处初始结论（点文件被读、钩子先于读规则）被推翻并更正；Claude 侧留 open bug T11，其余 agent 待实测。更正确认回写 `docs/design/external/agent-rules-dirs.md` §四。
