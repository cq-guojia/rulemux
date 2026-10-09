# DSH（DeepSeek Harness）适配器

> 状态：🔧 进行中（2026-10-09 开块；2026-10-10 定稿为「仓库子包」方案）
> 类型：新 agent 适配器（plugin-first 宿主）
> 关联：`docs/design/external/agent-rules-dirs.md` §一 DSH 行 · `docs/PROGRESS.md` T3 · 子包 [`dsh-plugin/`](../../dsh-plugin/)

## 定稿方案：写一半（Go 同步）+ 读一半（仓库子包插件）

DSH 是 **plugin-first** 宿主（Cordis 生命周期事件）：**没有 hook binary**，也**不原生扫描规则目录**。所以 rulemux 拆成两半：

| 半边 | 谁做 | 落点 |
|---|---|---|
| **写**（把规则真实拷贝进工作区） | rulemux 的 Go 二进制 | `rulemux sync` → `<cwd>/.dsh/rules/__rulemux__*.md` |
| **读**（读回来注入会话） | **仓库子包 `dsh-plugin/`**（npm 名 `rulemux-dsh`） | 插件在 `agent/pre-step` 读 `.dsh/rules/__rulemux__*.md` 注入一次 |

- 插件**按 DSH 正常方式安装**：`dsh plugin --profile <p> add rulemux-dsh`（npm 包，或本地 `npm pack` 出的 `.tgz`）。
  rulemux **不**替它装 ⇒ registry 里 `dsh` 的 `Style="external"`（init/doctor 只打印装插件提示，uninstall 只收敛规则目录）。
- 守住三条不变量：真实文件（非软链）/ engine 删残留（不累积）/ 首轮 pre-step 注入一次（不淡出）。
- **两阶段都只跑一次，不每轮**：`agent/session-start` 触发一次 `rulemux sync`；`agent/pre-step` 只在首个带用户输入的回合注入一次，compaction 挤掉后才补回。per-turn 只做亚毫秒本地读。
- **token 成本与 Trae/CodeBuddy 原生载入文件完全相同**（同一段文本、字节一致）。

> 历史：曾按「方案 A」把它做成 Go 二进制内嵌插件 + `rulemux init --agent dsh`（含 `internal/hooks/dsh.go`、`assets/rulemux-dsh.js`）。
> 2026-10-10 用户否决 A（那不是 DSH 生态的正常装法，且与子包路径会重复注入），改为本节子包方案，**A 的代码已删除**。

## 真源（已查，非文档推测）

| 事实 | 出处 |
|---|---|
| DSH plugin-first，canonical 扩展面是 Cordis 生命周期事件，无 hook binary | `hindsight-integrations/coding-agents/src/dsh.ts:1-24` |
| 一个包既是 CLI 又是 DSH 插件：`package.json` 的 `dsh.bundle.patch` + 根目录 `cordis.patch.yml` | `hindsight-integrations/coding-agents/{package.json,cordis.patch.yml}` |
| DSH 插件是 monorepo 里的**子包**（`hindsight-integrations/*` 一大家），**不是**独立仓库 | `/code/fork/hindsight/hindsight-integrations/` |
| 正规安装：`dsh plugin --profile web add <pkg>`；本地装 = 打包 **`.tgz`** 再 `dsh plugin add ./x.tgz` | 用户插件 `dsh-session-title-pattern/DEVELOPMENT.md`、市场条目 `dsh-local-installer` |
| `.dsh/rules` + `$DSH_HOME/rules` 由插件在 pre-step 读取注入，compaction 遮蔽后自动补回 | `awesome-dsh-plugin/README.zh.md:1650`（dsh-loulan-rules） |
| rulemux 三条不变量；「hook 文本注入动态区」列为非交付形态 | Hindsight 知识页 *Core concepts* |
| DSH 当初被定为「以插件市场解决」 | `docs/design/requirements.md:42` |

## 实现进展（2026-10-10）

| 步骤 | 状态 | 落点 |
|---|---|---|
| 子包（插件） | ✅ | `dsh-plugin/`（`package.json` 的 `dsh.bundle.patch`、`cordis.patch.yml`、`index.mjs`、`README.md`） |
| registry `dsh` 条目 | ✅ | `internal/agents/registry.go`（Tier1、`.dsh/rules`、`Style="external"`、`Verified=true`） |
| Go 侧「不装宿主配置」 | ✅ | `internal/hooks/install.go` 的 `ErrHookExternal`；init/doctor/uninstall 提示分支 |
| 移除旧方案 A | ✅ | 删 `internal/hooks/dsh.go`、`assets/rulemux-dsh.js`、`dsh_test.go` |
| 验证 | ✅ | `go build` / `go vet` / `go test ./...` 全绿；`npm pack --dry-run` 4 文件、`node --check` 通过 |
| **canary 坐实** | 🔴 待办 | 本机无 dsh；需在装了 dsh 的机器装插件并核验注入 |

## 待办 / 未决

1. **🔴 canary**：装 dsh → `dsh plugin --profile web add rulemux-dsh`（或 `.tgz`）→ `rulemux sync --hook --agent dsh`（或由插件自动触发）→ 新会话核验规则被读到。坐实后回写 `agent-rules-dirs.md` §四 并复核 `Verified` 语义。
2. **发 npm？**：用户口径「能发就发，发不了 git 装也行」——包已可 `npm pack`，是否 publish 待定。
3. **二期**：`$DSH_HOME/rules` 全局规则需 rulemux 目前没有的「用户级 sources」概念，暂不做。

## 风险

- 本机无 dsh ⇒ 插件能否被 `dsh plugin add` 正常加载、事件名是否匹配、面板是否显示，**均未真机验证**；照用户已跑通的两个插件与 hindsight 子包布局照抄。
- 插件声明了 `rulemux`（npm）依赖以触发 sync；在未发布 / 离线环境装插件时该依赖可能解析失败 —— 届时可去掉依赖，插件退回「只读 `.dsh/rules`」（仍能注入，只是不会自动 sync）。
- **git 直装子包未核实**：`dsh plugin add github:cq-guojia/rulemux` 会解析到**仓库根**（Go CLI 包），不是 `dsh-plugin/`。子包期间只能走 **npm 发布** 或**本地 `.tgz`**；若日后要 git 直装，需另想办法（如把子包拆成独立仓库 / 打 tag 指向子目录）。
