# 20261013-1205-skill-源分组-skill-group-与配置语义定稿-5a41：skill 源分组（[[skill_group]]）与配置语义定稿

> **✅ 完成封卷**：2026-10-13 12:05:43（@GuoJia-Agent）
> **创建**：2026-10-13 12:05:26（@GuoJia-Agent）
> **分类**：建设 ｜ **标签**：skill、配置语义、源分组、分组、agent

## 背景

用户 2026-10-13 要求「skills 也要支持分组」，并质疑 skill 源的配置为何「没有区分工作区 / Agent」，
先把配置讨论清楚再测。讨论中把配置语义逐个问了一遍：指定 Agent 是否等于落到该 Agent 的「通用」
（用户级）？指定工作区是否就不再分 Agent？落点：`[[skill_source]]` 当时（提交 `f3e4def`）只有
`path`/`agents`/`workspace`/`scope`，且**不支持任何分组**（`skill-sync.md` 原 §五 shortcut 明列）。

## 待办

无——本任务范围内已全部完成并提交；后续项见「结论」里挂出的 v2 任务。

## 下一步

无（任务已完成结案）。相关后续（Trae/dsh skill 注入、剩余 canary、git skill 源）已挂在任务
`20261013-1133-v2-trae-dsh-skill-注入-剩余-canary-git-skill-eec8`。

## 过程

- **语义澄清**（用户 2026-10-13 拍板）：域由**显式 `scope`** 决定（`workspace` 默认 / `user`）；
  `agents` 与 `workspace` 是**正交过滤器**。**不采用**「写了 `agents` 即 = 该 Agent 用户级通用」的
  隐式模型 —— 它会丢掉「只给某 agent、只在该工作区」的组合，且与 `[[source]]` 语义分叉。另澄清：
  `workspace = "*"` ≠ 全局；真·全局只有 `scope = "user"`。（`workspace = "*"` 是「每个工作区各拷一份，
  该工作区没开过就还没有」。）
- **实现**（`internal/config/config.go`）：
  - 新增 `SkillGroup` 结构（`Name`/`Paths`/`Uses`）与 `Config.SkillGroups`；
  - 解析新增 `[[skill_group]]`（`name`/`path`/`use`）；`[[skill_source]]` 新增 `groups` / `workspace_groups` 两键；
  - `expandHomes` 覆盖 `SkillGroup.Paths`（`~` 先展开再展开组）；
  - `resolveGroups` 新增 `expandSG`（嵌套 + 环检测），把组展开进 `SkillSource.Paths`/`Workspaces`
    （组展开在前、显式在后，按值去重）；未命名 / 重名 / 未定义 / 成环均报错。
- **测试**（`internal/config/config_test.go`）：`TestSkillGroupExpand`（含混写 `path`）、
  `TestSkillSourceWorkspaceGroups`、`TestSkillGroupErrors`（未定义 / 成环 / 重名三类错）、
  `TestExpandHomes_SkillGroup`。
- **踩坑**：同一文件并行发多条 `replace_in_file` 时，有 3 处被**静默丢弃**（`SkillGroup` 结构体、
  `[[...]]` 的 `skill_group` case 之前的 skill_source 键解析等），表现为 `undefined: SkillGroup`
  编译错 / 组未展开。**教训：同一文件不要并行编辑，逐次改 + 改后复读校验。**
- **格式**：顺手 `gofmt -w` 修了 `registry.go` / `sync.go` / `config.go` 的对齐（上一轮遗留）。

证据：提交 `5b26e29`（`feat: skill 源支持分组，补齐 README 配置说明`）；`internal/config/config.go`；
文档 `docs/design/features/skill-sync.md` §二.4 / §六、`README.md` / `README.zh-CN.md` §5.1 / §5.4 / §5.5、
`internal/cmd/init.go`（示例配置）。

## 结论

- `[[skill_source]]` 现支持分组：新增 `[[skill_group]]`，并可用 `groups` / `workspace_groups` 引用
  （工作区分组复用现有 `[[workspace_group]]`）。配置语义定稿为「显式 `scope` + 正交过滤器」。
- README 中英双份把配置讲清：§5.1 补 `scope`、§5.4 补 skill 分组、**§5.5 收录完整带注释的默认配置**。
- 验证：`gofmt` 干净、`go build/vet/test` 全绿、`scripts/smoke.sh` 23/23；已提交并推送（`5b26e29`）。
- 留坑：远程 / git skill 源、Trae/dsh skill 注入不在本任务，归 v2 任务
  （`20261013-1133-v2-trae-dsh-skill-注入-剩余-canary-git-skill-eec8`）。
