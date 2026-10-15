# 20261009-0000-workbuddy-split-025-7c5e：WorkBuddy 拆为独立 agent（发 `v0.2.5`）

> **✅ 完成封卷**：2026-10-09 00:00:00（@GuoJia-Agent）
> **创建**：2026-10-09 00:00:00（@GuoJia-Agent）

## 背景
本工作包背景见历史索引摘要（原过程文档未单列背景）。

## 待办
（已结案，原过程文档未单列待办项）

## 下一步
（已结案，无后续动作）

## 过程
本机实测推翻「WorkBuddy 复用 CodeBuddy 机制」的旧结论：WorkBuddy 是独立应用，有自己独立的**用户级**配置目录 `~/.workbuddy` 与钩子文件 `~/.workbuddy/settings.json`（不读 CodeBuddy 那份）⇒ registry 拆为独立条目，不再作 `codebuddy` 别名；保留 `--agent` 标识透传（安装/回显/刷新均写 `workbuddy`）；用户可见文案与外部事实文档同步更正

## 结论
已完成。成果即上方过程所述，亦见历史索引摘要。
