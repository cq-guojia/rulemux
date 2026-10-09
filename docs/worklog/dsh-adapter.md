# DSH（DeepSeek Harness）适配器

> 状态：🔧 进行中（2026-10-09 开块，方案 A 已拍板）
> 类型：新 agent 适配器（plugin-first 宿主）
> 关联：`docs/design/external/agent-rules-dirs.md` §一 DSH 行 · `docs/PROGRESS.md` T3

## 拍板的方案（A）

DSH 是 **plugin-first** 宿主（Cordis 生命周期事件），**没有 hook binary**，也不原生扫描规则目录。
rulemux 适配 = **Go 二进制把规则真实拷贝进 `<cwd>/.dsh/rules`** + **一个小 Cordis 插件在 `agent/pre-step` 读取并注入为 recall 消息**。

- 守住三条不变量：真实文件（非软链）/ engine 删残留（不累积）/ 每回合 pre-step 重注（不淡出）。
- **两阶段，都只跑一次，不每轮**：① `rulemux sync`（比+拷）只在 `agent/session-start` 或手动跑一次；② inject 只在首个带用户输入的 `agent/pre-step` 注入一次，规则留在历史；compaction 把它挤掉后才补回（照 dsh-loulan-rules）。per-turn 插件只做亚毫秒的本地读，绝不和中央源比。
- **token 成本与 Trae/CodeBuddy 原生载入文件完全相同**：进上下文的是同一段规则文本，字节一致；「注入」只是 DSH 无原生规则目录扫描器时的通路，不额外增 token（仅 recall 标签几个 token，可忽略）。

## 真源（已查，非文档推测）

| 事实 | 出处 |
|---|---|
| DSH plugin-first，canonical 扩展面是 Cordis 生命周期事件，无 hook binary | `/code/fork/hindsight/hindsight-integrations/coding-agents/src/dsh.ts:1-24` |
| 通过 `$DSH_HOME/cordis.patch.yml`（顶层 YAML 数组）加载插件，`name` 须 `file://` URL，`dsh web` 热加载 | `/code/fork/hindsight/.../src/installer.ts:18,1742-1788` |
| `.dsh/rules` + `$DSH_HOME/rules` 由插件在 pre-step 读取注入，compaction 遮蔽后自动补回 | `/code/fork/awesome-dsh-plugin/README.zh.md:1650`（dsh-loulan-rules） |
| rulemux 三条不变量；hook 文本注入动态区列为非交付形态 | Hindsight 知识页 *Core concepts* |

## 待办（实现顺序）

1. **读 `internal/engine`**：确认 Tier 分类（DSH 是「真实落盘 `.dsh/rules` 但靠插件注入」的杂交形态）与 `RulesDir` 驱动逻辑——决定 registry 条目填 `Tier1` 还是 `Tier2`。🔴 未定。
2. **写 `dsh` registry 条目**（`internal/agents/registry.go`）：`RulesDir: ".dsh/rules"`、`Verified: false`、注释标 🔴 canary 待补。
3. **写最小 Cordis 插件**（`dist/dsh.js` 或打包产物）：`agent/session-start` 触发一次 `rulemux sync`；`agent/pre-step` 仅在首个带用户输入的回合注入一次 `.dsh/rules/*.md`（+ `$DSH_HOME/rules`）为 `plugin:rulemux` recall 消息（照 `dsh.ts` 的 `injectionMessage`），并检测 compaction 遮蔽后补回；**per-turn 不做 sync/比对**。比 hindsight runtime 轻：只读取注入，无记忆逻辑。
4. **`init` 安装**：在 `$DSH_HOME/cordis.patch.yml` 写带标记块的 `insert` 行（`file://` 指向插件），uninstall 用标记块移除——照 `installer.ts:1716-1788` 幂等写法。
5. **范围**：先每工作区 `.dsh/rules`；`$DSH_HOME/rules` 全局规则需 rulemux 目前没有的「用户级 sources」概念，留二期。
6. **🔴 canary**：本机装 DSH，跑 `rulemux verify` 坐实注入后把 `Verified` 置真、回写 `agent-rules-dirs.md` §四。

## 未决 / 风险

- 本机**未装 DSH 运行时** → 所有 DSH 事实（`.dsh/rules` 为 canonical 目录、pre-step 注入生效）目前依据 hindsight 反向工程 + dsh-loulan-rules 约定，**非 DSH 官方文档/本机实测**。装一次才能坐实。
- 是否 DSH 新版本已原生读 `.dsh/rules`（使插件不再必要）？canary 时复核。
- 单 Go 二进制 + 一个 JS 插件的形态，是 hindsight 也做的同样让步；用户已接受。

## 实现进展（2026-10-10）

代码已落地并 `go build` / `go test ./...` 通过（新增单测 `internal/hooks/dsh_test.go`）：

| 步骤 | 状态 | 落点 |
|---|---|---|
| registry `dsh` 条目 | ✅ | `internal/agents/registry.go`（Tier1、`.dsh/rules`、Style=dsh、`DSH_HOME`、`Verified=false`） |
| 嵌入式 Cordis 插件 | ✅ | `internal/hooks/assets/rulemux-dsh.js`（go:embed） |
| 安装/刷新/检查/卸载 | ✅ | `internal/hooks/dsh.go`（幂等标记块、`[]` 归一、保留他人行）+ `install.go` 七处分派 |
| CLI 输出分支 | ✅ | `init.go`（plugin registered）/ `doctor.go`（Plugin 三态）/ `uninstall.go`（plugin removed） |
| 文档回写 | ✅ | 本节 + `design/features/dir-sync.md` DSH 一节 |
| **canary 坐实** | 🔴 待办 | 本机未装 dsh；需装一次 dsh，`rulemux verify` + 新会话核验注入后置 `Verified=true` |

设计细节沿用前面各节：`Tier1`（真实拷贝）+ 插件只读 `__rulemux__*`、首轮注入一次、compaction 补回（仅在宿主能证明确实丢了时才补，避免每轮重注）。
