# rulemux 实现工作计划

> **状态**：🚧 进行中（源码已实现，待编译 → 待 canary 实测）
> **日期**：2026-10-07
> **配套**：[`../design/implementation.md`](../design/implementation.md)（决策清单）· [`nas-go-toolchain-requirements.md`](nas-go-toolchain-requirements.md)（Go 环境需求）· [`../PROGRESS.md`](../PROGRESS.md)（在办任务）

---

## 一、当前进展

| 阶段 | 内容 | 状态 |
|---|---|---|
| 1 | 设计决策逐项拍板（`implementation.md` #1–#13） | ✅ 完成 |
| 2 | design 文档对齐新模型（去中央真源 / Tier 分层 / 前缀 / DeepSeek 移出） | ✅ 完成 |
| 3 | **Go 源码实现（核心引擎 + 全部 agent 适配器 + 4 个子命令）** | ✅ 完成（本批） |
| 4 | 编译出二进制 | ✅ 完成（Go 1.27.1，`go build` + `go vet` 全绿） |
| 5 | canary 实测坐实（点文件 / 目录结构 / 钩子时序 / 注入） | 🚧 **部分完成**：CodeBuddy/WorkBuddy ✅ 坐实（2026-10-08：平铺**非隐藏** `.md` + `alwaysApply:true` 才加载、**点文件被跳过**；2026-10-09 对照实测复证**读规则先于钩子写入**）；Claude ❌ 本工作区未落地（open bug，见 T11）；trae/codex/opencode 待实测 |
| 6 | 单测 + 冒烟 | ✅ 完成（`go test ./...` 通过；`scripts/smoke.sh` 10/10） |
| 7 | CI 交叉编译 + Release 分发 | ⬜ 待做 |
| 8 | T7：`package.json` 加 `files` 字段 | ⬜ 待做 |

---

## 二、代码结构（核心引擎 + 每 agent 适配器）

```
rulemux/
├── main.go                       CLI 入口：sync / inject / init / doctor / verify
├── go.mod                        模块 github.com/cq-guojia/rulemux，Go 1.22，零第三方依赖
├── internal/
│   ├── config/config.go          TOML 配置解析（自写 TOML 子集解析器，保持零依赖）
│   │                             + SourcesFor(agent) + Validate
│   ├── agents/registry.go        「每 agent 适配器」注册表：
│   │                             claude / codebuddy / workbuddy / trae / codex / opencode
│   │                             每个声明：Tier、规则目录、钩子落点、是否已核实
│   ├── engine/                   核心引擎（被适配器调用的统一方法）
│   │   ├── sync.go               前缀命名 + 内容比对 + 删残留（Tier-1 目录同步）
│   │   ├── inject.go             Tier-2 注入渲染（不碰用户 AGENTS.md）
│   │   └── canary.go             canary 探针（写入 / 清理）
│   ├── hooks/install.go          SessionStart 钩子安装（幂等）
│   │                             JSON 风格（claude/trae/json）+ Codex TOML 追加
│   └── cmd/                      子命令实现 + 极简参数解析
│       ├── flags.go  sync.go  inject.go  init.go  doctor.go  verify.go
└── docs/
    ├── design/                   设计文档（决策 / 架构 / 功能 / 外部事实）
    └── ops/                      本文件 + Go 环境需求
```

**调用关系**（即「统一抽象方法 + 每 agent 单独定义 + 调用统一方法」三层）：

```
各 agent 的 SessionStart 钩子
        │
        ▼
rulemux sync --agent X   /   rulemux inject --agent X
        │
        ├─► agents.Get(X)          ← 适配器：告诉引擎「X 的规则目录在哪 / 走注入」
        │
        └─► engine.Sync(dir, srcs) ← 统一方法：前缀命名 / 内容比对 / 覆盖 / 删残留
            engine.RenderInject()  ← 统一方法：Tier-2 渲染注入文本
```

- **统一方法**（`internal/engine`）：文件怎么命名、怎么比对、怎么删残留 —— 只有一份实现。
- **每 agent 单独定义**（`internal/agents`）：各自的规则目录、钩子落点、Tier —— 一张注册表。
- **调用**（`internal/cmd`）：适配器信息 + 引擎方法在子命令里组合。

---

## 三、任务拆解与优先级

| # | 任务 | 优先级 | 状态 |
|---|---|---|---|
| W1 | 设计决策锁定 + 文档对齐 | P0 | ✅ |
| W2 | Go 源码：模块骨架 + CLI | P0 | ✅ |
| W3 | Go 源码：配置解析（零依赖 TOML 子集） | P0 | ✅ |
| W4 | Go 源码：agent 注册表（6 个 agent） | P0 | ✅ |
| W5 | Go 源码：核心引擎 sync / inject / canary | P0 | ✅ |
| W6 | Go 源码：init / doctor / verify 子命令 + 钩子安装 | P0 | ✅ |
| W7 | **编译出二进制** | P0 | ✅ Go 1.27.1 编译通过，`go vet` 无告警 |
| W8 | 核心引擎单测（命名/比对/删残留/幂等）+ 冒烟脚本 | P1 | ✅ `go test ./...` 通过 + `scripts/smoke.sh` 10/10 |
| W9 | canary 实测：点文件 / CodeBuddy 结构 / 钩子时序 / Codex 注入 | P1 | 🚧 **部分**：CodeBuddy/WorkBuddy ✅；Claude ❌（open bug，T11）；trae / codex / opencode 仍待 |
| W10 | 据实测校准未核实项（落点、前缀是否改非点） | P1 | ✅ 据 CodeBuddy 实测校准：前缀保持 `__rulemux__`（点文件会被读），落点确认为 `.codebuddy/rules` |
| W11 | GitHub Actions 交叉编译 + Release | P2 | ⬜ |
| W12 | T7：`package.json` 加 `files` 字段 | P2 | ⬜ |
| W13 | 排查 Claude 侧 sync 未落地（open bug，T11） | P0 | ⬜ |

---

## 四、编译方式（Go 就绪后）

```bash
cd rulemux

# 当前平台
go build -o rulemux .

# 交叉编译：一个 Go 就能出三平台产物
GOOS=windows GOARCH=amd64 go build -o rulemux.exe .
GOOS=darwin  GOARCH=arm64  go build -o rulemux-darwin-arm64 .
GOOS=linux   GOARCH=amd64  go build -o rulemux-linux-amd64 .
```

> **零第三方依赖**：只用 Go 标准库 ⇒ `go build` 不需要 `go mod download`，离线环境也能编译。

**测试**：

```bash
go test ./...        # 单测：配置解析 + 核心引擎（命名/去重/比对/覆盖/删残留/幂等/不碰用户文件）
./scripts/smoke.sh   # 端到端冒烟：init / sync / inject / doctor / verify（10 项）
```

---

## 五、使用流程（用户侧）

```bash
rulemux init      # 1. 生成示例 config.toml + 为各 agent 装 SessionStart 钩子（幂等）
# 2. 编辑 ~/.rulemux/config.toml，把 path 改成自己真实的规则文件
rulemux doctor    # 3. 环境自检：配置/本体/各 agent 目录与钩子状态
rulemux verify    # 4. canary 验收：开新会话问 agent 能否念出暗号
# 5. 之后每次开新会话，钩子自动触发 rulemux sync --agent X
```

同步算法（每次 `sync`）：

1. 计算「当前配置应生成的带前缀文件名集合」S（`__rulemux__` + 源 basename，同名冲突加短 hash）。
2. 枚举目标目录所有 `__rulemux__*`：不在 S 中的 ⇒ **删除**（删残留）。
3. S 中每项：目标不存在 ⇒ 复制；内容一致 ⇒ 跳过；不一致 ⇒ **覆盖**。
4. 不带前缀的文件（用户自己的）**一律不碰**。

---

## 六、已知待校准项（部分已坐实，其余待 canary）

代码里这些 agent 仍标 `Verified=false` 的，`doctor` 会用 ⚠ 提示：

| agent | 校准状态 / 内容 |
|---|---|
| codebuddy | ✅ **已坐实（2026-10-08）**：`.codebuddy/rules/` 平铺**非隐藏** `.md`（须带 `alwaysApply:true`）会被加载、**点文件被跳过**，且**读规则先于钩子写入**（首会话差一拍）（见 `external/agent-rules-dirs.md` §四） |
| workbuddy | ✅ 随 CodeBuddy 一并坐实（复用其机制） |
| trae | ⬜ 钩子配置落点（`hooks.json` 工作区根 or `.trae/hooks.json`）；点文件是否读 |
| codex | ⬜ 钩子落点 `~/.codex/config.toml` 与 schema（`[features] codex_hooks` + `[[hooks.SessionStart]]`） |
| opencode | ⬜ 钩子落点与注入方式 |

**全局校准**：各 agent 的「读全部 .md」是否**跳过点开头的隐藏文件** —— CodeBuddy 已证明会读点文件 ⇒ 前缀保持 `__rulemux__`；trae / codex / opencode 仍待实测确认（若某家跳过 ⇒ 把 `internal/engine/sync.go` 的 `Prefix` 改成非点前缀 `rulemux__`）。

---

## 七、当前状态与下一步

**Go 工具链**：✅ 已装（Go 1.27.1 linux/amd64），编译 / 单测 / 冒烟均已跑通。
（若 NAS 重启后仍需在本机编译，持久化需求见 [`nas-go-toolchain-requirements.md`](nas-go-toolchain-requirements.md)。）

**canary 验收准备（已就绪，待新会话判定）**：

- `rulemux` 已安装到 `/usr/local/bin/rulemux`（在 PATH 上，钩子用 exec 形式能 spawn）。
- 测试源 `~/.rulemux/rules/canary-test.md` 内含暗号；`~/.rulemux/config.toml` 把它派给 claude / codebuddy / trae / codex。
- 6 个 agent 的 SessionStart 钩子已安装（`rulemux init`），`rulemux doctor` 显示全部已安装。
- **判定方法**：开一个**新会话**，问该 agent「你能在规则里看到这个暗号吗：`RULEMUX-CANARY-43371345`」。
  - 念得出 ⇒ 链路通（该 agent 会读该规则目录下的规则文件；注意**读规则先于钩子写入** ⇒ 会话开始后才落盘的文件本会话读不到，要下一个会话 —— 这正是「变化提示」要告知用户的）；
  - 念不出 ⇒ 按 [`../design/features/verification.md`](../design/features/verification.md) 排查；若确认是「跳过点文件」⇒ 把 `internal/engine/sync.go` 的 `Prefix` 改成非点的 `rulemux__`。
