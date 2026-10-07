# rulemux 实现方案：待定项清单

> **状态**：📝 进行中（逐条拍板，拍板一项划掉一项）
> **来源**：用户 2026-10-07 要求立项为「实现细节设计方案」任务，基于本文档逐项讨论后落定
> **配套**：[`requirements.md`](requirements.md)（要什么）· [`architecture.md`](architecture.md)（为什么）· [`features.md`](features.md)（怎么做）· [`external/agent-rules-dirs.md`](external/agent-rules-dirs.md)（各家目录）· [`../PROGRESS.md`](../PROGRESS.md)（在办任务 T0–T7）

---

## 用法

本文件是「实现阶段」的**任务清单 + 决策记录**。每条初始状态 `⬜ 待讨论`；讨论拍板后改为 `✅ 已定`，并把结论简短写进「结论」栏。
不在此复述需求/架构——那两套是真源，本文件只记「要拍什么板、拍完是什么」。

---

## 一、总体形态

### 1. 实现语言与构建 ⬜
- **问题**：用 Node/TS 还是 Go/Rust 单二进制？构建产物形态（`dist/` + `bin`）。
- **关联**：PROGRESS 下一步第 4 条（技术选型，结论进 `architecture.md`）。
- **待定**：语言、打包器、是否单文件分发。

### 2. CLI 形态与 bin 名 ⬜
- **问题**：单命令 + 子命令（`rulemux sync` / `init` / `doctor` / `verify`）？还是单一可执行 + flags？`bin` 名是否就叫 `rulemux`。
- **待定**：子命令集合、`init` 是否生成示例配置。

### 3. 分发与平台 ⬜
- **问题**：npm 全局安装 / `npx rulemux`；是否保证 win / mac / linux 跨平台。
- **待定**：最低 Node 版本、是否发预编译二进制。

## 二、中央真源

### 4. 真源位置 ⬜
- **问题**：真源就是 rulemux 仓库内的目录（如 `rules/`）？还是用户各自维护的独立仓库/目录？rulemux 仓库本身是否同时充当「示例真源」。
- **关联**：T0（需求待拍板项）。
- **待定**：路径、是否允许远程 git 真源。

### 5. 真源文件约定 ⬜
- **问题**：是否就是普通 `.md`？允不允许子目录？有没有中央元信息（frontmatter 声明适用于哪些 agent / 优先级）。
- **待定**：文件格式、元信息 schema。

## 三、目标发现与注册

### 6. targets 怎么来 ⬜
- **问题**：自动探测本机已装 agent？还是读一份配置文件（`.rulemux.toml` / `.rulemux.json`）显式列出 agent + workspace 路径。
- **待定**：配置格式、是否支持多 workspace。

### 7. 各 agent 规则目录事实 ⬜
- **问题**：Claude Code / CodeBuddy / Trae / Codex / OpenCode / WorkBuddy / DeepSeek 各自的规则目录、扩展名、是否读**全部** `.md`、frontmatter 语义——目前 🔴 未核实。
- **关联**：T1 / T3 / T4 / T6。
- **待定**：逐家源码级核实后回写 `external/agent-rules-dirs.md`。

## 四、同步与适配

### 8. 同步触发方式 ⬜
- **问题**：手动 `rulemux sync`？git hook / pre-commit？CI？还是依赖各 agent 的 SessionStart hook 触发？「永不淡出 / 不累积」靠什么机制保证。
- **关联**：T4（实测闭环）。
- **待定**：触发模型、是否常驻。

### 9. 逐 agent 格式适配 ⬜
- **问题**：中央 `.md` → 各 agent 期望格式（文件名、扩展名、frontmatter、单文件 vs 多文件）。是否给每个 agent 注入不同头尾标记（如 `AGENTS.md` 的 `RULES BEGIN/END`）。
- **关联**：T1 / T2（Codex 单文件、OpenCode 配置列表的分支处理）。
- **待定**：适配层设计、受管区标记格式。

### 10. 文件映射与增删 ⬜
- **问题**：中央增删文件 → 目标同步增删（满足「自由 add/delete」）。映射如何定义；用户在目标侧手改产生冲突时如何处理（覆盖 / 跳过 / 报错）。
- **关联**：T5（删除生效 + 区分「我方同步过的」与「用户自己的」）。
- **待定**：映射规则、冲突策略、manifest 机制。

### 11. 幂等与不累积保证 ⬜
- **问题**：同一文件只写一次、不每轮追加。状态追踪用什么（内容 MD5 / 元数据），状态存哪（目标目录内隐藏文件？还是无状态纯比对）。
- **关联**：T5。
- **待定**：状态存储方案。

## 五、降级方案（Tier-2 hook 注入）

### 12. 无原生规则目录的 agent 降级 ⬜
- **问题**：启用 SessionStart + PreCompact hook 注入降级（该 agent 视为未达验收）。rulemux 如何生成/安装这些 hook，哪些 agent 支持 hook。
- **关联**：T6（hooks 落点）、`features/hook-injection.md`。
- **待定**：hook 生成器、安装路径、幂等重装。

## 六、验证

### 13. 验收自动化 ⬜
- **问题**：如何把「效果与 token 与直接写 AGENTS.md 一致」落成自带能力——30 秒测试 + canary 文件法如何做成 `rulemux verify` / `doctor` 子命令。
- **关联**：`features/verification.md`。
- **待定**：`verify` 子命令形态、canary 机制。

## 七、工程化

### 14. 测试与冒烟 ⬜
- **问题**：`package.json` 的 `build` / `test` 命令、选什么测试框架（或先无框架自测）。
- **待定**：框架、CI。

### 15. 文档落地 ⬜
- **问题**：实现相关的设计决策回写到 `docs/design/`（按现有体系），不另建。
- **说明**：约定已明确，本条仅作提醒，无需拍板。
