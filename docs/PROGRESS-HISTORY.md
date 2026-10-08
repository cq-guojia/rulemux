# 进度历史（已结案）

> **这是什么**：已结案事项的**一行索引** —— 什么时候、完成了什么、过程文档在哪。
> **过程 / 踩坑 / 注意事项一律不写在这**，要看去 `worklog/`。
> **在办的事**看 [`PROGRESS.md`](PROGRESS.md)。
> **规矩**：事项结案即从 `PROGRESS.md` 移入本文件（追加一行），见 [`README.md`](README.md)。

| 时间 | 工作包 | 完成了什么 | 过程文档 |
|---|---|---|---|
| 2026-10-06 | 项目初始化 + 文档体系重建 | 从 `dsh-task-dispatch-table` 文档模板建立 rulemux 文档体系：分类表去 dsh 化、需求草稿分解迁入 `docs/design/`、`PROGRESS` 系列重置、`AGENTS.md` 项目段改写、根 `README.md` 新建 | [worklog/docs-reorg.md](worklog/docs-reorg.md) |
| 2026-10-06 | 文档架构专家团评审 | 三方评审（骨架一致性 / 需求与设计落位 / 现场层与链接）后整改：分类表恢复通用模板 1–7 编号并标注不适用、新增「需求」槽位 `design/requirements.md`、架构文档只留原理与选型、外部事实剥离我方设计并标🔴未核实、实测法修 canary 假阳性、`AGENTS.md` §二 恢复编号骨架 | 本文件（结论已并入各归属文档，不另建决策文件） |
| 2026-10-07 | npm 包名占位 | 发布空包 **`rulemux@0.0.1`**（MIT）到 npm，包名占位完成。两个前置卡点：① npm 账号邮箱需为 verified（hotmail 收不到验证邮件 ⇒ 改用 Gmail）；② 2FA 为 `auth-and-writes` 且只绑安全密钥 ⇒ CLI 出不了 OTP，改用 **automation token** 发布 | 本文件 |
| 2026-10-07 | Go 源码实现 | 完成核心引擎（sync/inject/canary）+ 6 个 agent 适配器（claude/codebuddy/workbuddy/trae/codex/opencode）+ CLI（sync/inject/init/doctor/verify）+ 自写零依赖 TOML 子集解析器 | [`ops/implementation-workplan.md`](../ops/implementation-workplan.md) |
| 2026-10-07 | Go 编译 + 单测 + 冒烟 | Go 1.27.1 linux/amd64 装好；`go build`/`go vet` 全绿，`go test ./...` 通过，`scripts/smoke.sh` 10/10；二进制装到 `/usr/local/bin/rulemux`，6 agent 钩子已装 | [`ops/implementation-workplan.md`](../ops/implementation-workplan.md) W7/W8 |
| 2026-10-07 | 配置增强：path 数组 + workspace | `Source.Paths` 支持单串/数组；新增 `Workspaces`（数组，空/`*`/`all`=全部）；`SourcesFor`/`MatchesWorkspace` 实现「先算匹配条目→合并文件清单」 | [`design/implementation.md`](../design/implementation.md) #6 |
| 2026-10-08 | CodeBuddy / WorkBuddy canary 坐实 | 新会话能念出暗号 `RULEMUX-CANARY-43371345`；坐实点文件会被读、`.codebuddy/rules` 平铺加载、钩子先于读；代码标 `Verified=true` | [`design/external/agent-rules-dirs.md`](../design/external/agent-rules-dirs.md) §四 · [worklog/canary-2026-10-08.md](worklog/canary-2026-10-08.md) |
| 2026-10-08 | npm 正式发版 `rulemux@0.1.0` | 发布可用包（非占位）：补齐 `bin` 启动器 + `files` + `.npmignore` + 交叉编译脚本；5 平台二进制经 `-ldflags="-s -w"` 裁剪（4.2M→2.8M/份，包 12.6M→6.2M）；README 重写为英文默认 + 中文版。已上线且为 `latest` | 本文件 |
