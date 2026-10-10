# DSH（DeepSeek Harness）适配器

> 状态：🔧 进行中（2026-10-09 开块；2026-10-10 定稿「仓库子包」方案并连做三轮收口：自给自足 → 三步全成或抛错）
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
- **两阶段都只跑一次，不每轮**：`agent/session-start` 触发一次就绪 + `rulemux sync`；`agent/pre-step` 只在首个带用户输入的回合注入一次，compaction 挤掉后才补回。per-turn 只做亚毫秒本地读。
- **token 成本与 Trae/CodeBuddy 原生载入文件完全相同**（同一段文本、字节一致）。

> 历史：曾按「方案 A」把它做成 Go 二进制内嵌插件 + `rulemux init --agent dsh`（含 `internal/hooks/dsh.go`、`assets/rulemux-dsh.js`）。
> 2026-10-10 用户否决 A（那不是 DSH 生态的正常装法，且与子包路径会重复注入），改为本节子包方案，**A 的代码已删除**。

## 插件就绪：三步，全成或抛错（2026-10-10 最终口径）

用户的判断（逐字要点）：插件安装只干三件事——**装基础包、装插件、init**；「**已装就算成功**」只说对了一半，
还要看**版本**：不指定版本则旧版要升级（pnpm/npm），指定版本则「没被覆盖」就**算失败**；
**同步跟这三步没关系**，「同步失败了，该记日志就记日志」；不许出现「我做成功了一半」的提示。

| 决策 | 内容 |
|---|---|
| 去掉 npm 依赖 | `dsh-plugin/package.json` 删掉 `dependencies.rulemux`（0.1.0 → 0.2.0 → 0.3.0）。硬依赖会让**整个 `dsh plugin add`** 因镜像/源滞后失败（用户第一次即遇 `ERR_PNPM_NO_MATCHING_VERSION`），而那是用户无法处理的安装期失败 |
| 就绪=三步，全成或抛错 | ① CLI 可得**且版本合规**；② 插件已装载（插件在跑即已成立，无需也无法自检）；③ `~/.rulemux/config.toml` 存在。**已有即算成功、绝不覆盖** |
| 版本合规（新增） | 常量 `REQUIRED_CLI = ">=0.3.0"`（首个含 dsh 适配器的版本，也决定升级目标）；`probeCliVersion()` 跑 `--version` 自解析（`rulemux X.Y.Z`），`satisfies()` 自实现 `>=` / 精确 pin 比较（**不写进 `dependencies`/`peerDependencies`**，否则又变安装期硬依赖）。不满足 ⇒ `pnpm add -g` → 退 `npm install -g` 升级/覆盖 ⇒ **复查仍不满足即抛错**（被旧副本遮蔽、等于是「没被覆盖」⇒ 失败，不假装成功） |
| 阻塞、不降级 | 首次就绪**阻塞**首个回合、**不设等待上限**（单条 spawn 120s 仅为防死锁，超时按失败算） |
| 同步与就绪解耦 | `rulemux sync --hook --agent dsh` 每会话一次；**退出码非 0 只 `console.error` 记日志并继续**，用盘上已有规则注入。含「配置里没有任何 `[[source]]`」这一正常失败面 |
| 删除全部提示通道 | `readyNotice()` / `noticeShown` / `NOTHING_TO_SYNC` 与 sync 层的 `unknown agent` 自愈重试全部删除；三步全过时只静默注入规则 |
| 重启 | 装完插件需重启 dsh（DSH 装插件的常规要求，用户接受） |

### 实测（本机无 dsh：假 ctx 驱动插件生命周期回调 + 真/假 CLI，脚本在 `/tmp`，未入库）

| 场景 | 期望 | 实测 |
|---|---|---|
| 全新 HOME（无配置，无 `[[source]]`） | `init` 生成配置；sync 退出 1 **只记日志**；不抛错、不提示 | ✅ 配置已生成；stderr 出日志；`RESULT=ok INJECTED=0` |
| 三步全过 + 配好 `[[source]]` | 真拷贝并注入规则正文，**无任何提示** | ✅ `INJECTED=1`，内容仅 `<!-- rulemux:managed -->` + 规则正文 |
| PATH 里无 rulemux/pnpm/npm | **抛错** + 逐条原因 + 兜底命令 | ✅ |
| 旧版 0.2.0 + 升级无效 | **抛错**：要求 vs 实测版本、遮蔽说明 | ✅ |
| 旧版 0.2.0 + npm 升级成功 | 复查 0.3.0 后继续 | ✅ 调用序列 `--version(0.2.0) → npm install → --version(0.3.0) → init → sync` |
| 注入一次 / compaction 才补回 | turn1=1；unknown=0；observed=0；宿主证明确实丢了=1 | ✅（上一轮实测，未回归） |

另：`node --check` 通过；`npm pack --dry-run` → `rulemux-dsh@0.3.0`、4 个文件、**无依赖**。

> 测试坑（记下来免得再踩）：假 CLI 场景把 `PATH` 限定成只有假目录，脚本里的 `cat`/`date` 就找不到，
> 版本被读成空并回落成默认值，看起来像插件 bug。假脚本只用 shell 内建（`read`/`echo`）即可。

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
| 插件自给自足（无依赖 + 三步就绪 + 版本合规） | ✅ | `dsh-plugin/index.mjs`（`provisionOnce` / `ensureCli` / `ensureConfig` / `syncOnce` / `ensureReady`）+ `package.json` 去依赖并升 0.3.0 |
| registry `dsh` 条目 | ✅ | `internal/agents/registry.go`（Tier1、`.dsh/rules`、`Style="external"`、`Verified=true`） |
| Go 侧「不装宿主配置」 | ✅ | `internal/hooks/install.go` 的 `ErrHookExternal`；init/doctor/uninstall 提示分支 |
| 移除旧方案 A | ✅ | 删 `internal/hooks/dsh.go`、`assets/rulemux-dsh.js`、`dsh_test.go` |
| 验证 | ✅ | `go build` / `go vet` / `go test ./...` 全绿；`npm pack --dry-run` 4 文件、`node --check` 通过；假 ctx 冒烟见上表 |
| **canary 坐实** | 🔴 待办 | 本机无 dsh；需在装了 dsh 的机器装插件并核验注入 |

## 待办 / 未决

1. **🔴 canary**：装 dsh → 重启 → `dsh plugin --profile web add rulemux-dsh`（或 `.tgz` / git spec）→ 新会话核验「规则被读到、无重复注入、无任何提示」。
2. **首次就绪的三个待核**：① 装完插件后 `pnpm/npm` 在那个宿主进程里是否真的可执行；② **抛错在真机上是否表现为会话可见的失败**（而不是被宿主吞掉后静默继续）——这是本轮设计的交付前提；③ 版本探测在真机 CLI 上的输出形态（当前按 `rulemux X.Y.Z` 解析）。
3. **`REQUIRED_CLI` 发版前复核**：必须仍是「首个含 dsh 适配器的版本」，因为它同时决定升级目标（当前 `>=0.3.0`）。
4. **发 npm？**：用户口径「能发就发，发不了 git 装也行」——包已可 `npm pack`，是否 publish 待定。
5. **二期**：`$DSH_HOME/rules` 全局规则需 rulemux 目前没有的「用户级 sources」概念，暂不做。

## 风险

- 本机无 dsh ⇒ 插件能否被 `dsh plugin add` 正常加载、事件名是否匹配、面板是否显示，**均未真机验证**；照用户已跑通的两个插件与 hindsight 子包布局照抄。
- **旧副本遮蔽 = 硬失败**是有意的：若 PATH 上（或遗留 `node_modules/rulemux`）有一个不满足版本要求的 CLI 而升级覆盖不到它，插件抛错而不是照旧跑 —— 这正是用户「没被覆盖就算失败」的口径。错误文案里给出了「in effect」的具体路径，便于排查。
- ~~「配置里没有 `[[source]]` 不算失败」依赖 CLI 报错文案匹配~~ **已消除（2026-10-10）**：该字符串匹配（`NO_SOURCES`）已随同步与就绪解耦一并删除，现在只看退出码，且 sync 失败不抛错。
- ~~git 直装子包未核实~~ **已核实（2026-10-10）**：子包**可以** git 直装，spec 为 `github:cq-guojia/rulemux#path:/dsh-plugin`（`#path:/<subdir>` 是 DSH 装子包的标准写法，见 `awesome-dsh-plugin/scripts/lib/capabilities.mjs:44-46`、`scripts/scan-decay.mjs:274`）。**npm 发布非必需**——git 源码安装是基线通道，`.tgz` 是另一条免 npm 路。
