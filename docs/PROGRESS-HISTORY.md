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
| 2026-10-08 | CodeBuddy / WorkBuddy canary 坐实 | 新会话能念出暗号 `RULEMUX-CANARY-43371345`；坐实 `.codebuddy/rules` 平铺加载、且需**非隐藏** `.md` + `alwaysApply:true`（点文件**被跳过**）；同日复测**更正**了「钩子先于读规则」这一假阳性 —— 实际是**读规则先于钩子写入**（⇒ 新增文件首会话差一拍，后续由「变化提示」兜底）；代码标 `Verified=true` | [`design/external/agent-rules-dirs.md`](../design/external/agent-rules-dirs.md) §四 · [worklog/canary-2026-10-08.md](worklog/canary-2026-10-08.md) |
| 2026-10-08 | npm 正式发版 `rulemux@0.1.0` | 发布可用包（非占位）：补齐 `bin` 启动器 + `files` + `.npmignore` + 交叉编译脚本；5 平台二进制经 `-ldflags="-s -w"` 裁剪（4.2M→2.8M/份，包 12.6M→6.2M）；README 重写为英文默认 + 中文版。已上线且为 `latest` | 本文件 |
| 2026-10-08 | 发版流水线实测通过 + `rulemux@0.1.2` 上线 | 打 tag 触发 release 流水线：5 平台交叉编译 + GitHub Release 自动挂产物全绿（修复 release job 缺 checkout 的 bug）；npm 的 0.1.2 因仓库未配 `NPM_TOKEN` 由手动 `npm publish` 补发，现为 `latest` | 本文件 |
| 2026-10-08 | 修复 hook 命令格式（`args` 被宿主丢弃）| CodeBuddy 的 hook 只执行 `command` 字段、丢弃 `args` ⇒ 旧写法被执行成裸 `rulemux`（无参数、只打印帮助），规则从不生效；改为把参数写进 `command` 整串，并自动迁移旧钩子；新增回归测试 | [worklog/hook-command-2026-10-08.md](worklog/hook-command-2026-10-08.md) |
| 2026-10-09 | 同步范围改造 + 变化提示（发 `v0.2.0`） | 修掉「在非配置工作区跑同步会把已投放文件清空」的安全事故（不匹配则整体跳过）；新增 `sync --all`（只走账本、不读 cwd、目录不存在跳过 + 仅 `ENOENT` 才 GC 账本条目）；账本升级 v2（工作区 × Agent，旧格式自动迁移不丢条目）；Tier-1 钩子加 `--hook` 门禁，确有变化时输出一条「规则有变化，请新开会话」协议 JSON（无变化 stdout 字节级为空，人类日志改走 stderr）；顺手同步该工作区历史上用过的其它写目录 agent（只写已存在目录）；目录创建/同步两条硬原则；workbuddy 并入 codebuddy 别名 | [design/features/sync-all-and-change-notice.md](../design/features/sync-all-and-change-notice.md) |
