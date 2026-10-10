# DSH（DeepSeek Harness）适配器

> 状态：🔧 进行中（2026-10-09 开块；2026-10-10 定稿「仓库子包」方案；同日插件改为「无依赖 + 首次运行自动就绪」）
> 类型：新 agent 适配器（plugin-first 宿主）
> 关联：`docs/design/external/agent-rules-dirs.md` §一 DSH 行 · `docs/PROGRESS.md` T3 · 子包 [`dsh-plugin/`](../../dsh-plugin/)

## 定稿方案：写一半（Go 同步）+ 读一半（仓库子包插件）

DSH 是 **plugin-first** 宿主（Cordis 生命周期事件）：**没有 hook binary**，也**不原生扫描规则目录**。所以 rulemux 拆成两半：

| 半边 | 谁做 | 落点 |
|---|---|---|
| **写**（把规则真实拷贝进工作区） | rulemux 的 Go 二进制 | `rulemux sync` → `<cwd>/.dsh/rules/__rulemux__*.md` |
| **读**（读回来注入会话） | **仓库子包 `dsh-plugin/`**（npm 名 `rulemux-dsh`） | 插件在 `agent/pre-step` 读 `.dsh/rules/__rulemux__*.md` 注入一次 |

- 插件**按 DSH 正常方式安装**（npm 非必需）：`dsh plugin --profile <p> add "github:cq-guojia/rulemux#path:/dsh-plugin"`（git 源码，子包用 `#path:/` spec）／`... add rulemux-dsh`（npm 包）／`... add ./rulemux-dsh-<v>.tgz`（本地包）。
  rulemux **不**替它装 ⇒ registry 里 `dsh` 的 `Style="external"`（init/doctor 只打印装插件提示，uninstall 只收敛规则目录）。
- 守住三条不变量：真实文件（非软链）/ engine 删残留（不累积）/ 首轮 pre-step 注入一次（不淡出）。
- **两阶段都只跑一次，不每轮**：`agent/session-start` 触发一次 `rulemux sync`；`agent/pre-step` 只在首个带用户输入的回合注入一次，compaction 挤掉后才补回。per-turn 只做亚毫秒本地读。
- **token 成本与 Trae/CodeBuddy 原生载入文件完全相同**（同一段文本、字节一致）。

> 历史：曾按「方案 A」把它做成 Go 二进制内嵌插件 + `rulemux init --agent dsh`（含 `internal/hooks/dsh.go`、`assets/rulemux-dsh.js`）。
> 2026-10-10 用户否决 A（那不是 DSH 生态的正常装法，且与子包路径会重复注入），改为本节子包方案，**A 的代码已删除**。

## 插件自给自足：无依赖 + 首次运行自动就绪（2026-10-10）

用户的判断（逐字要点）：装插件拿不到 CLI 就等于没用，**「我宁愿失败」**；等 10 秒还是 20 秒不是问题，
**「跳过后继续」才是问题**；装插件本来就要重启，重启可接受。

| 决策 | 内容 |
|---|---|
| 去掉 npm 依赖 | `dsh-plugin/package.json` 删掉 `dependencies.rulemux`（版本 0.1.0 → 0.2.0）。硬依赖会让**整个 `dsh plugin add`** 因镜像/源滞后失败（用户第一次即遇 `ERR_PNPM_NO_MATCHING_VERSION`），而那是用户无法处理的安装期失败 |
| CLI 改在首次运行取得 | `provisionOnce()`（模块级单例，全进程一次）：① 解析（依赖副本 → PATH）；② 缺失则 `pnpm add -g rulemux` → 退 `npm install -g rulemux`（装完还查 pm 自己的 global bin，避开 `PNPM_HOME` 不在 PATH）；③ `~/.rulemux/config.toml` 不存在则 `rulemux init --agent dsh` 生成注释样板 |
| **阻塞，不降级** | 首次就绪**阻塞**首个回合，**不设等待上限**（单条 spawn 120s 只为防死锁，超时按失败算）；任一必得步骤最终失败即**抛错**，附每条失败原因 + 手动命令 —— **不**退回「只读已有 `.dsh/rules` 继续」 |
| 唯一例外 | 配置里没有任何 `[[source]]` 时 `rulemux sync` 退出非零，但那是**未配置**而非坏掉（自动生成的样板本来就是空的）；按「无事可同步」处理并在会话里提示一次，否则全新安装将永远不可用 |
| 一次性提示 | 首次就绪后注入一条 `plugin:rulemux` recall：CLI 装到哪、配置生成在何处、去填 `[[source]]` |
| 重启 | 装完插件需重启 dsh（DSH 装插件的常规要求） |

验证（本机无 dsh：用假 ctx 驱动插件的生命周期回调 + 真 CLI，脚本在 `/tmp`，未入库）：

| 场景 | 期望 | 实测 |
|---|---|---|
| 全新 HOME（无配置） | 自动 `init` 生成配置 + 一条一次性提示，不崩 | ✅ |
| 配好 `[[source]]` | 真拷贝进 `.dsh/rules/__rulemux__base.md`，注入规则正文、无提示 | ✅ |
| 无 CLI 且装不上（PATH 里无 rulemux/pnpm/npm） | **抛错** + 逐条原因 + 手动命令 | ✅ |
| 注入一次 / compaction 才补回 | turn1 注入 1；turn2（无法判断）0；turn3（宿主确证还在）0；turn4（宿主证明确实丢了）1 | ✅ |

另：`node --check` 通过；`npm pack --dry-run` → `rulemux-dsh@0.2.0`、4 个文件、**无依赖**。

## 真源（已查，非文档推测）

| 事实 | 出处 |
|---|---|
| DSH plugin-first，canonical 扩展面是 Cordis 生命周期事件，无 hook binary | `hindsight-integrations/coding-agents/src/dsh.ts:1-24` |
| 一个包既是 CLI 又是 DSH 插件：`package.json` 的 `dsh.bundle.patch` + 根目录 `cordis.patch.yml` | `hindsight-integrations/coding-agents/{package.json,cordis.patch.yml}` |
| DSH 插件是 monorepo 里的**子包**（`hindsight-integrations/*` 一大家），**不是**独立仓库 | `/code/fork/hindsight/hindsight-integrations/` |
| 三条安装通道：**git 源码**（`github:owner/repo`，子包加 `#path:/<subdir>`）／ npm 包 ／ Release `.tgz`；**npm 非必需、不影响收录** | `awesome-dsh-plugin/site/locales.mjs`（GH_C/NPM_C/TGZ_C）、`scripts/lib/capabilities.mjs:44-46`（`#path:/` spec）、`scripts/scan-decay.mjs:274`、`contributing.md` |
| `.dsh/rules` + `$DSH_HOME/rules` 由插件在 pre-step 读取注入，compaction 遮蔽后自动补回 | `awesome-dsh-plugin/README.zh.md:1650`（dsh-loulan-rules） |
| rulemux 三条不变量；「hook 文本注入动态区」列为非交付形态 | Hindsight 知识页 *Core concepts* |
| DSH 当初被定为「以插件市场解决」 | `docs/design/requirements.md:42` |

## 实现进展（2026-10-10）

| 步骤 | 状态 | 落点 |
|---|---|---|
| 子包（插件） | ✅ | `dsh-plugin/`（`package.json` 的 `dsh.bundle.patch`、`cordis.patch.yml`、`index.mjs`、`README.md`） |
| 插件自给自足（无依赖 + 首次就绪） | ✅ | `dsh-plugin/index.mjs`（`provisionOnce` / `syncOnce` / `ensureReady` / `readyNotice`）+ `package.json` 去依赖 |
| registry `dsh` 条目 | ✅ | `internal/agents/registry.go`（Tier1、`.dsh/rules`、`Style="external"`、`Verified=true`） |
| Go 侧「不装宿主配置」 | ✅ | `internal/hooks/install.go` 的 `ErrHookExternal`；init/doctor/uninstall 提示分支 |
| 移除旧方案 A | ✅ | 删 `internal/hooks/dsh.go`、`assets/rulemux-dsh.js`、`dsh_test.go` |
| 验证 | ✅ | `go build` / `go vet` / `go test ./...` 全绿；`npm pack --dry-run` 4 文件、`node --check` 通过；假 ctx 冒烟见上节 |
| **canary 坐实** | 🔴 待办 | 本机无 dsh；需在装了 dsh 的机器装插件并核验注入 |

## 待办 / 未决

1. **🔴 canary**：装 dsh → 重启 → `dsh plugin --profile web add rulemux-dsh`（或 `.tgz` / git spec）→ 新会话核验「首次就绪提示 + 规则被读到 + 无重复注入」。坐实后回写 `agent-rules-dirs.md` §四 并复核 `Verified` 语义。
2. **首次就绪的两个待核**：① 装完插件后 `pnpm/npm` 在这个宿主进程里是否真的可执行（本机冒烟是直接跑 CLI，没经过 dsh 的进程环境）；② 抛错在真机上是否表现为「会话可见的失败」（而不是被宿主吞掉后静默继续）。
3. **发 npm？**：用户口径「能发就发，发不了 git 装也行」——包已可 `npm pack`，是否 publish 待定。
4. **二期**：`$DSH_HOME/rules` 全局规则需 rulemux 目前没有的「用户级 sources」概念，暂不做。

## 风险

- 本机无 dsh ⇒ 插件能否被 `dsh plugin add` 正常加载、事件名是否匹配、面板是否显示，**均未真机验证**；照用户已跑通的两个插件与 hindsight 子包布局照抄。
- ~~插件声明了 `rulemux`（npm）依赖以触发 sync~~ **已改（2026-10-10）**：依赖已删除，CLI 由首次运行自动全局安装；同时**取消**了原先设想的「退回只读 `.dsh/rules`」降级 —— 拿不到 CLI 就抛错（见上节）。
- 「配置无 `[[source]]` 不算失败」这条依赖 `rulemux sync` 的**报错文案**（正则 `has no \[\[source\]\]`）。两半同仓发布、插件随之升级，但若将来 CLI 改文案而插件没跟上，全新安装会退化成硬失败；canary 时一并确认。
- ~~git 直装子包未核实~~ **已核实（2026-10-10）**：子包**可以** git 直装，spec 为 `github:cq-guojia/rulemux#path:/dsh-plugin`（`#path:/<subdir>` 是 DSH 装子包的标准写法，见 `awesome-dsh-plugin/scripts/lib/capabilities.mjs:44-46`、`scripts/scan-decay.mjs:274`）。**npm 发布非必需**——git 源码安装是基线通道，`.tgz` 是另一条免 npm 路。
