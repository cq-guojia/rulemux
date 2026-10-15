# 20261009-0000-trae-cn-onboard-030-1a9f：Trae CN 接入封板（发 `v0.3.0`）

> **✅ 完成封卷**：2026-10-09 00:00:00（@GuoJia-Agent）
> **创建**：2026-10-09 00:00:00（@GuoJia-Agent）

## 背景
本工作包背景见历史索引摘要（原过程文档未单列背景）。

## 待办
（已结案，原过程文档未单列待办项）

## 下一步
（已结案，无后续动作）

## 过程
坐实 trae 规则目录 `.trae/rules`（递归读、`.md` 加载含可选 frontmatter、`.mdc` 不加载）、用户级 hook 落点 `~/.trae-cn/hooks.json`（CN）/ `~/.trae`（intl）；`Verified=true`，`init --agent trae` 可用。**per-agent 变化提示决策落定**：trae 在会话开始即读取规则（首会话即得）⇒ `SessionHint` 保持 `false`（不设 `HintProtocol`），无需"请新开会话"提示；CodeBuddy/WorkBuddy 因"会话开始固定快照、不热重载"保持 `SessionHint:true`。统一原则＝仅对「会话内不重读规则目录」的 agent 开 `SessionHint`。`v0.3.0` 已推 origin + 发 npm `latest`，作为封板版本，**无后续代码改动**

## 结论
已完成。成果即上方过程所述，亦见历史索引摘要。
