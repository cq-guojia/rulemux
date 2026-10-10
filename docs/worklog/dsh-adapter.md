# DSH（DeepSeek Harness）适配器

> 状态：🔧 进行中（2026-10-09 开块；2026-10-10 定稿「仓库子包」方案并连做四轮收口：自给自足 → 三步全成或抛错 → 就绪时机提到 dsh 启动装载）
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
| 版本合规（新增） | 常量 `REQUIRED_CLI = ">=0.3.1"`（**第一个真正含 dsh 适配器的发布版本**，也决定升级目标；最初误定为 `>=0.3.0`，见下「发布缺口」）；`probeCliVersion()` 跑 `--version` 自解析（`rulemux X.Y.Z`），`satisfies()` 自实现 `>=` / 精确 pin 比较（**不写进 `dependencies`/`peerDependencies`**，否则又变安装期硬依赖）。不满足 ⇒ `pnpm add -g` → 退 `npm install -g` 升级/覆盖 ⇒ **复查仍不满足即抛错**（被旧副本遮蔽、等于是「没被覆盖」⇒ 失败，不假装成功） |
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

## 就绪时机提到「插件装载」（2026-10-10 第四轮）

用户实测后当场指出流程不合理（逐字要点）：**「难道不应该是我装了这个，我就去改配置，改完配置我再开会话，相应的文件就给我注入了吗？」**
「我装完 init，然后我再去会话随便聊一个天儿，它才把这个给我出来，我再去改配置，我再去开会话」——即：不该为「生成配置」多付一次会话。

| 项 | 内容 |
|---|---|
| 根因 | 就绪链（`provisionOnce` = ① CLI + ③ 配置）原先**只在 `agent/session-start`** 被踢起，所以「装完 + 重启」之后、没开过会话时 `~/.rulemux/` 根本不存在（用户实测 `No such file or directory`，与代码行为一致） |
| 修法 | 在 `apply(ctx)` 里**立即** `provisionOnce().catch(() => {})` —— dsh 启动装载插件时就会调用 `apply`，早于任何会话；而 ①③ 都不需要工作区，可以直接跑。`session-start` 的踢起**保留**作兜底，并继续承担「每会话一次 sync」 |
| 失败语义不变 | 装载期不把拒绝抛给 dsh（避免未处理拒绝扰动启动），同一个 Promise 在首个 `pre-step` 的 `await` 处照旧抛出 ⇒ 失败依然**会话可见** |
| 为什么 `sync` 不提前 | sync 的入参是会话工作区，装载期没有上下文；强行提前会写错目录 |
| 为什么不能在安装期做 | dsh 安装插件不执行任何脚本（本包零依赖的由来），所以「装完就有配置」只能靠「重启后装载时生成」 |
| 版本 | `0.3.0` → **`0.3.1`**（用户刚按 git spec 装过 0.3.0，必须能区分是否真换到新代码） |

新增实测（假 ctx；`RMX_MODE=load` 只调 `apply()`、**不触发任何会话事件**）：

| 场景 | 期望 | 实测 |
|---|---|---|
| 只装载、不开会话（隔离 HOME） | `~/.rulemux/config.toml` 由 `apply()` 自己生成 | ✅ `LOAD_ONLY config-after-load=true` |
| 配置无来源 | sync 失败**只记日志**、不抛错、不注入 | ✅ |
| 配好来源后开会话 | 一次会话即注入规则正文、无任何提示 | ✅ |
| 无 CLI | 装载期不炸；首个 pre-step 抛错 | ✅ |
| 旧版 + 升级无效 / 升级成功 | 抛错 / 通过（升级到 0.3.1） | ✅ |

**新增 canary 待核**：① 真机上「重启后**不开任何会话**」`~/.rulemux/config.toml` 是否已存在 —— 这条依赖「dsh 在启动装载时就会调用 `apply()`」这一前提，本机查 hindsight 知识库未见既有结论，属未坐实假设；② 装载期的失败是否确实不在启动时炸掉 dsh（应只在会话里抛）。

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
3. **🔴 先发 0.3.1（当前阻塞，见上「发布缺口」）**：`REQUIRED_CLI` 已改为 `>=0.3.1`，而 npm 上最新仍是 0.3.0 且其不含 dsh ⇒ 在 0.3.1 发布前任何机器都会卡在第①/③步。发版 = master 推 tag `v0.3.1`；发完 `npm view rulemux version` 核对（CI 的 npm 步骤可能因缺 `NPM_TOKEN` 被静默跳过）。
4. **发 npm？**：用户口径「能发就发，发不了 git 装也行」——包已可 `npm pack`，是否 publish 待定。
5. **二期**：`$DSH_HOME/rules` 全局规则需 rulemux 目前没有的「用户级 sources」概念，暂不做。

## 发布缺口（2026-10-10 发现，当前阻塞）

用户真机实测报「`~/.rulemux` 不存在、`rulemux -v` 还是 0.2.7」。查证后确认这是一条**发布缺口**，不是插件逻辑错：

| 事实 | 证据 |
|---|---|
| npm 上 `latest` = **0.3.0**（2026-10-09T13:05Z 发布） | `https://registry.npmjs.org/rulemux`（只读查询） |
| 但 **0.3.0 里没有 dsh 适配器** | `git show v0.3.0:internal/agents/registry.go` 无 dsh 条目；dsh 的提交全在 tag `v0.3.0` 之后（`60c405c`→`a9e913d`→`cf4483a`→`a2da91d`），master 比 v0.3.0 多 **11** 个提交 |
| 用户机器上的 0.2.7 更没有 dsh | 其 `SUPPORTED AGENTS` 只列 codebuddy / workbuddy（用户贴出的实测输出） |

因果链：就绪第①步把 0.2.7 升到 latest（0.3.0）后**版本检查反而通过**（0.3.0 ≥ 0.3.0），第③步 `rulemux init --agent dsh` 却在 0.3.0 上**报 unknown agent、退出非 0** ⇒ 抛错 ⇒ `~/.rulemux` 永不生成。

修法：① `REQUIRED_CLI` 由 `>=0.3.0` 改为 **`>=0.3.1`**（第一个真正含 dsh 的发布版本）；② **必须先发 0.3.1**（master → tag `v0.3.1` → CI 出二进制 + npm publish；⚠ `release.yml` 的 npm 步骤只在配了 `NPM_TOKEN` 时执行，否则会**静默跳过**、需手动 `npm publish`），否则任何机器都过不了这一关。

教训（留给下一个人）：**版本下限只有在「下限以下真的做不到」时才成立**。写下限时必须先核对目标版本里**是否真的含**这个能力，别拿「仓库当前版本号」当锚。

### 追加（2026-10-10 真机 log 打脸）：版本号不能当唯一判据

真机 canary 打出：插件 `apply() called`（装载模型成立 ✓），但就绪第①步失败，`in effect: PATH (rulemux)`，而那份 CLI **一边自报 `0.3.0`、一边在 `SUPPORTED AGENTS` 里列着 `dsh`** —— 它其实能干这活，只是版本号印错了。

根因：`main.go` 里 `var version = "0.3.0"` 是**写死的默认值**，只有发布流水线用 `-X main.version=<tag>` 覆盖；所以任何用 `go build` / `go install`（乃至任何不带 ldflags 的方式）编出来的二进制，只要树里有 dsh，也会自报 0.3.0。`scripts/build-dist.sh` 与 CI 都正确注入了版本，**问题只出在"版本号被当成能力凭证"**。

修法（插件 0.3.3）：判据改为 **`cliOk()` = 版本满足 `REQUIRED_CLI` 或 `knowsDsh()`**，后者跑 `rulemux --help`（输出走 stderr，故 `out+err` 一起看）匹配 `^\s*dsh\s`（id 列直接来自 registry）。同时 `installCli` 安装后把 `resolveCli()` 与 `cliInGlobalBin(pm)` **两个候选都按 `cliOk` 判**（原先 `a || b` 会短路，PATH 上的旧副本可能遮蔽刚装好的新版本）。

实测（假 CLI）：自报 `0.3.0` 但 `--help` 里带 `dsh` ⇒ 被接受、配置生成（正是真机那种）；自报 `0.3.0` 且不认 `dsh`、又装不上 ⇒ 抛错并逐条记录原因。

## 风险

- 本机无 dsh ⇒ 插件能否被 `dsh plugin add` 正常加载、事件名是否匹配、面板是否显示，**均未真机验证**；照用户已跑通的两个插件与 hindsight 子包布局照抄。
- **旧副本遮蔽 = 硬失败**是有意的：若 PATH 上（或遗留 `node_modules/rulemux`）有一个不满足版本要求的 CLI 而升级覆盖不到它，插件抛错而不是照旧跑 —— 这正是用户「没被覆盖就算失败」的口径。错误文案里给出了「in effect」的具体路径，便于排查。
- ~~「配置里没有 `[[source]]` 不算失败」依赖 CLI 报错文案匹配~~ **已消除（2026-10-10）**：该字符串匹配（`NO_SOURCES`）已随同步与就绪解耦一并删除，现在只看退出码，且 sync 失败不抛错。
- ~~git 直装子包未核实~~ **已核实（2026-10-10）**：子包**可以** git 直装，spec 为 `github:cq-guojia/rulemux#path:/dsh-plugin`（`#path:/<subdir>` 是 DSH 装子包的标准写法，见 `awesome-dsh-plugin/scripts/lib/capabilities.mjs:44-46`、`scripts/scan-decay.mjs:274`）。**npm 发布非必需**——git 源码安装是基线通道，`.tgz` 是另一条免 npm 路。
