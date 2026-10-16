# 20261010-0000-release-method-final-5c9a：发布方式定稿：GitHub Release 走 CI，npm **手动**发，不配 `NPM_TOKEN`

> **✅ 完成封卷**：2026-10-10 00:00:00（@GuoJia-Agent）
> **创建**：2026-10-10 00:00:00（@GuoJia-Agent）
> **分类**：运维 ｜ **标签**：release、github、git、npm、发布

## 背景
本工作包背景见历史索引摘要（原过程文档未单列背景）。

## 待办
（已结案，原过程文档未单列待办项）

## 下一步
（已结案，无后续动作）

## 过程
用户原话「那个不配，以后还是手动的」⇒ 推 tag 只触发交叉编译 + GitHub Release；npm 永远是已登录 shell 里手动 `npm publish`。两个包：`rulemux`（主包，含 5 平台二进制）、`rulemux-dsh`（`dsh-plugin/`，独立包、改了要单独发）。本轮实际发出：`rulemux@0.3.2`、`rulemux-dsh@0.3.5`（首发）。步骤写进根 README §9（中英）

## 结论
已完成。成果即上方过程所述，亦见历史索引摘要。
