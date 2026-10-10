# DSH 插件设置界面（rulemux-dsh）

> 状态：🔧 进行中（待下个会话执行；2026-10-10 由 `dsh-adapter` canary 坐实后立项）
> 类型：dsh 插件能力增强（plugin-first 宿主）
> 关联：`docs/design/external/agent-rules-dirs.md` §四 DSH 行 · 子包 [`dsh-plugin/`](../../dsh-plugin/) · 前序 [`dsh-adapter.md`](dsh-adapter.md)（canary 已坐实）

## 背景与目标

`rulemux-dsh` 目前**没有 dsh 原生的设置界面**：用户配置全靠手改 `~/.rulemux/config.toml`（Go CLI 的配置文件，插件在装载期用 `rulemux init --agent dsh` 生成一份满注释样板，再由用户填 `[[source]]`）。

目标：给 rulemux-dsh 加一个 **dsh 设置界面**（settings UI），让用户在 dsh 的设置面板里直接配置，而不是去翻 CLI 的 toml。

> ⚠️ 本文件是「下个会话待执行事项」的载体；具体实现方式（dsh 插件如何声明 settings、值存哪、插件怎么读）在调研坐实前**不写死**。

## 待执行事项

1. **调研 dsh 插件「设置界面」的声明方式（先做，未坐实不实现）**
   - 在哪个文件 / 字段定义 settings schema（候选：`cordis.patch.yml` 里加 `settings` 段？单独的 settings 描述文件？）；
   - 支持的控件类型（输入框 / 开关 / 下拉 / 文件路径选择…）；
   - 用户填的值**存到哪**、插件运行时**怎么读**（环境变量？dsh 注入的上下文对象？`cordis.patch.yml` 同目录的 json？）；
   - 真机验证手段：哪条命令 / 哪个面板能看到并改这个值。
2. **定 scope（最小集起步）**
   - rulemux-dsh 要暴露哪些设置项，先列候选再砍到最小：
     - CLI 来源 / 路径（何时需要？目前 `resolveCli()` 已先找依赖副本再回退 `PATH`）；
     - config 路径（目前写死 `~/.rulemux/config.toml`，见 `dsh-plugin/index.mjs:86` `configPath()`）；
     - 是否自动 `sync`（目前 `agent/session-start` 已每次跑一次）；
     - 日志级别 / 日志路径（目前写 `~/.rulemux-dsh-load.log`）。
3. **实现**
   - 在插件里声明 settings schema；
   - 让插件**读取 dsh 注入的设置值**，并明确它与现有「直接读 `~/.rulemux/config.toml`」的关系（共存？覆盖？二选一？需先定）。
4. **真机验证**
   - dsh 设置界面能看到该项；改值后：配置生成 / 规则注入不受影响；值在装载期 / 会话期确实被读到。

## 不做什么（先定边界，避免范围蔓延）

- **不要**在没有和用户定清楚前，把设置界面做成「绕过 Go CLI config 的第二条配置源」——`config.toml` 是 rulemux 各 agent 的统一真源，加第二条源会引入一致的歧义。
- 设置项若与 `config.toml` 已有字段重复，**以谁为准**要先定（建议：设置界面是 `config.toml` 的友好前端，最终仍落到 toml，不另起炉灶）。
- `$DSH_HOME/rules` 全局规则（二期）不在本任务内（见 `dsh-adapter.md` 待办 #5，已明确不做）。

## 真源 / 参考资料

| 事实 | 出处 |
|---|---|
| 插件当前无 settings 声明，`cordis.patch.yml` 仅有 `insert: - id: rulemux, name: rulemux-dsh` | `dsh-plugin/cordis.patch.yml:13-16` |
| 配置路径写死 `~/.rulemux/config.toml`，由 `configPath()` 返回 | `dsh-plugin/index.mjs:86-88` |
| CLI 解析先找依赖副本再回退 `PATH` | `dsh-plugin/index.mjs:153-174` `resolveCli()` |
| dsh 为 plugin-first，扩展面是 Cordis 生命周期事件，无 hook binary | `hindsight-integrations/coding-agents/src/dsh.ts:1-24` |

## 进展

### 2026-10-10 ① 列表展示：图标 + 中英标题说明 ✅ 已落码（待实机确认）

用户原话：「列表那个地方有一个图标、一个标题，还有一个像简单说明的那种…… 图标用
`docs/话术提示词-选中.svg`，名字你改…… 标题…… 后面加一个括号把包名写上，备注里说明一下这个插件是做什么的……
要根据用户的语境支持中英文。」

- 图标：`dsh-plugin/icon.svg`（128×128，`viewBox` 沿用原图 1024），`package.json` 顶层 `icon` 接线，
  并加入 `files`；源文件 `docs/话术提示词-选中.svg` 原样保留。
- 标题 / 说明：新增 `dsh-plugin/locale/zh.json` 与 `locale/en.json`（`meta.title` / `meta.description`），
  `exports` 加 `./locale/*.json`、`files` 加 `locale`；写法照参考实现 `dsh-session-title-pattern`
  （干什么 + 半角括号包完整包名）。
- 版本 `0.3.3 → 0.3.4`。
- 结论回写：[`design/external/dsh-plugin-manifest.md`](../design/external/dsh-plugin-manifest.md)。
- 未做（属 ②）：插件详情页的**配置表单** —— 仍需先调研 settings schema 的声明方式与取值落点（见「待执行事项」1–3）。
