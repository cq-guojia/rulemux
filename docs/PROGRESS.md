# 进度（进行中）

> **这是什么**：本仓库**唯一**的在办事项真源 —— 现在做到哪、欠什么、下一步做什么。
> **已结案的**看 [`PROGRESS-HISTORY.md`](PROGRESS-HISTORY.md)（一行一条：时间 / 完成了什么 / 过程文档）。
> **文档规矩**看 [`README.md`](README.md)。
>
> 本文件**只装在办的事**；事项结案即从本文件移入 `PROGRESS-HISTORY.md`。
> ⚠️ README / issue / 聊天记录都不是真源。

---

## 一、当前状态

### 1.1 需求与方案定型 —— ✅ **已拍板**（2026-10-07 逐条锁定，文档对齐进行中）

> 定型层文档已于 2026-10-07 逐项拍板（结论进 [`design/implementation.md`](design/implementation.md)）：
> [`design/requirements.md`](design/requirements.md)、[`design/architecture.md`](design/architecture.md)、[`design/features.md`](design/features.md) + `features/` 三份、[`design/external/agent-rules-dirs.md`](design/external/agent-rules-dirs.md)（Claude Code 已核实；CodeBuddy/Trae/Codex/OpenCode 待 canary）现已对齐新模型。

---

## 二、未决项

| # | 问题 | 现状与影响 | 将来怎么解（方向，未定） |
|---|---|---|---|
| T3 | **DeepSeek Harness 适配器（子包方案）已实现，🔴 待 canary** | 定稿：「Go 同步进 `.dsh/rules` + 仓库子包 `dsh-plugin/`（npm 名 `rulemux-dsh`）经 `dsh plugin add` 安装注入」；registry `dsh` = Tier1 / `Style=external` / `Verified=true`（先行开放）。旧「内嵌 + `init`」方案 A 已删 | 装了 dsh 的机器 `dsh plugin --profile <p> add rulemux-dsh` + 新会话核验注入；回写 `external/agent-rules-dirs.md` §四 |
| T4 | **各 agent 核实未完成（剩余家）** | Claude Code 已于 2026-10-07 经官方文档核实；CodeBuddy、WorkBuddy 已坐实；**Trae 已于 2026-10-09 随 `v0.3.0` 坐实（见 T10）**；**Codex / OpenCode / DeepSeek 🔴 待补** ⇒ 除已核实项外不得作实现依据 | 其余各家按 `features/verification.md` 跑实测闭环，并补齐适用版本与出处 |
| T6 | **Codex hooks 配置落点待查证**（Trae 已坐实） | Trae `hooks.json` 落点与 schema 已随 `v0.3.0` 坐实；**Codex（`~/.codex/config.toml`）仍待补** | Codex 接入时再查。结论回写 `external/` |
| T10 | **canary 实测坐实外部事实（剩余家）** | CodeBuddy ✅（2026-10-08）、WorkBuddy ✅（2026-10-09, `v0.2.7`）、Trae ✅（2026-10-09, `v0.3.0`）均已坐实；**Codex / OpenCode / Claude / DeepSeek 🔴 待补** | codex / opencode 按 `features/verification.md` 跑 `rulemux verify`；结论回写 `external/agent-rules-dirs.md` §四 |
| T11 | **Claude 侧 sync 未落地（open bug）** | 本工作区那次 Claude 会话里 `.claude/` 仅有 `settings.json`、无 `rules/` 目录，canary 暗号未被读到 ⇒ 同步链路未在该 agent 生效；根因未定（adapter 路径 / hook 触发 / 该工作区未装钩子） | 开一次 Claude Code 会话，查 `rulemux sync --agent claude` 是否真生成 `.claude/rules/__rulemux__*`；结合 `doctor` 与 `external/agent-rules-dirs.md` §四 排查 |

> 已结案项（**T0 / T1 / T2 / T5 / T7 / T8 / T9 / T12 / T13 / T14 / T15**）已移入 [`PROGRESS-HISTORY.md`](PROGRESS-HISTORY.md)，本表只留在办事项。

---

## 三、下一步

## 三、下一步（待办）

- **给仓库配 `NPM_TOKEN` secret**（Settings → Secrets → Actions，Automation 类型 token）：否则 CI 的 npm 步骤按设计跳过，每次发版只能手动 `npm publish`（0.2.7 就是手动发的）。
- `WORKBUDDY_CONFIG_DIR` 是否决定用户级配置目录：仍未坐实（产物里只在 safe-delete 日志白名单出现）；坐实后再考虑加入注册表 `HookDirEnv`。
- **DSH（DeepSeek Harness）适配器（子包方案）**：Go 侧（sync 落 `.dsh/rules`、`Style=external`）与子包 `dsh-plugin/` 均完成。**待办**：① 装了 dsh 的机器 `dsh plugin --profile web add "github:cq-guojia/rulemux#path:/dsh-plugin"` → 新会话核验规则被读到（canary）；② 发不发 npm 属可选（不发也能 git 装）。过程见 `docs/worklog/dsh-adapter.md`。

> 各 agent 接入 / 核实 / canary 的剩余工作见 §二 未决项（T3 / T4 / T6 / T10 / T11）。
