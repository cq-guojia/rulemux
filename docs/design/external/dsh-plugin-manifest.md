# dsh 插件的展示元信息（图标 / 标题 / 说明）与详情页配置区

> **类型**：外部事实（DeepSeek Harness 宿主行为）
> **适用版本**：dsh `>=0.1.7-rc.1`（本包 `engines.dsh`）；核实对象 `@deepseek-ai/dsh-client-ui-plugin-manager@0.2.0-rc.1`
> **状态**：✅ 已核实（官方包 README 级；渲染效果待实机确认）
> **来源**：`@deepseek-ai/dsh-client-ui-plugin-manager/README.zh.md:32`（英文同 `README.md:32`）；参考实现 `dsh-session-title-pattern`（`package.json:5`、`locale/zh.json`、`locale/en.json`、`DEVELOPMENT.md:22-27`）
> **配套**：工作包 [`../../worklog/dsh-plugin-settings-ui.md`](../../worklog/dsh-plugin-settings-ui.md) · 子包 [`../../../dsh-plugin/`](../../../dsh-plugin/)

## 一、图标：package.json 顶层 `icon`

- 插件卡片、详情页、组件行显示**该插件自己** `package.json.icon` 声明的图片；未声明或无法解码时保留默认插画。
  出处：`@deepseek-ai/dsh-client-ui-plugin-manager/README.zh.md:32`。
- 宿主**读文件内容转 base64 data URL** 渲染，格式支持 SVG / PNG / JPEG / WebP，上限 256 KiB。
  出处：参考实现 `dsh-session-title-pattern/DEVELOPMENT.md:25-27`（`package.json` 顶层 `"icon": "./icon.svg"`，文件已入 `files`）。
- 尺寸口径：参考实现的 `icon.svg` 为 `viewBox="0 0 128 128" width="128" height="128"`（`dsh-session-title-pattern/icon.svg:1`），卡片里实际按小尺寸（约 28px 档）渲染 ⇒ **给 128×128 即可，宿主自行缩放**。本包沿用：
  `dsh-plugin/icon.svg` 保留原图 `viewBox="0 0 1024 1024"`，仅把 `width`/`height` 设为 `128`。

## 二、标题与说明：导出的 locale `meta`

- 已安装的组合包及其插件行在**卡片和详情页**中，按**当前界面语言**显示各自的标题与描述；
  每个字段**先读取导出的 locale `meta`**，缺失时回退到该插件地址下可访问的 `package.json`
  （标题最终用完整包名或模块名，两处都没有描述时不给描述）。
  出处：`@deepseek-ai/dsh-client-ui-plugin-manager/README.zh.md:32`。
- 因此包必须导出并按语言放文件：`"./locale/*.json": "./locale/*.json"`（`exports`），
  `locale/zh.json` / `locale/en.json` 各带 `meta.title` / `meta.description`；
  **`en.json` 是入口，缺了整个目录不读，两份必须同时在**。
  出处：参考实现 `dsh-session-title-pattern/package.json:30`、`locale/{zh,en}.json`、`DEVELOPMENT.md:22-24`。
- 读取的是**文件**，不执行插件代码 ⇒ 纯 host 侧插件（如本包，无 client 产物）同样生效。

## 四、详情页配置区（`plugins.bundle.config`）与读写边界

| 结论 | 出处 |
|---|---|
| 该槽位是 **keyed** 槽位，key 必须是**完整 npm 包名**；`scope: 'root'`，只以 `view: 'page'` 渲染 | `@deepseek-ai/dsh-client-ui-plugin-manager/lib/types/client/slot-contract.d.ts:95-104` |
| 要让它渲染，宿主必须登记本插件的**设置命名空间**；命名空间是 **profile entry id**（本包 `rulemux`），不是包名（`rulemux-dsh`） | `@deepseek-ai/dsh-settings/README.md:10-12`；`cordis.patch.yml:13-16` |
| 命名空间由 host 侧导出的 `Config`（schemastery，字段 `.volatile()`）**自动派生** | 参考实现 `dsh-session-title-pattern/src/host/index.ts:141-170`、`:840-844` |
| 表单值落在 **dsh 自己的设置文档（当前 profile 的 Cordis patch）**，不在插件自己的文件里 | `@deepseek-ai/dsh-settings/README.md:10-12` |
| host 端**没有**设置读写的拦截钩子（无 `onRead`/`onWrite`/provider），`SettingsForms` 只暴露 configure / writable / documentPath / prepareDocument / describe / update / replace / mutate | `@deepseek-ai/dsh-settings/lib/types/index.d.ts:61-117` |
| 浏览器端**读不到磁盘文件**：详情页无 sessionId，`remote.commands.execute(sessionId…)` 强制要 sessionId；`remote.workspaceFiles.read` 首参也是 SessionId 且只认工作区 scope；第三方没有注册自定义 remote 服务的公开口子 | `slot-contract.d.ts:95-104`；`@deepseek-ai/dsh-commands/lib/typert.remote-client.d.ts:7-23`；`dsh-api-remotes/lib/client.js:13117-13164`；`dsh-api-remotes/README.md:71-77` |
| 官方表单状态机：`SettingsFormModel`（primitives）提供 `dirty` / `overridden` / `saving` / `failed`；`SettingsForm` 只带失败文案，成功提示要自己加 | `@deepseek-ai/dsh-client-ui-primitives/lib/types/settings-form/form-model.d.ts:101-125`、`SettingsForm.d.ts:15-26` |
| 外链：primitives **没有**通用 `Link` / `Anchor`（`Button` 也不收 `href`）；用原生 `<a>`，官方自身对 http(s) 外链的习惯是 `target="_blank"` + `rel="noopener noreferrer"` | `dsh-client-ui-primitives/lib/types/index.d.ts:1-64`、`Button.d.ts:1-18`、`lib/index.js:11333-11353` |

**因此本插件的口径**：详情页只放**说明**（文件在哪、要手动改什么、示例、GitHub 外链），
**不做保存** —— `~/.rulemux/config.toml` 是唯一真源，dsh 设置文档不是它的第二份拷贝。
`Config` 里那个占位字段只为让宿主登记命名空间，界面不渲染、不写入。

---

## 四、本包现状

| 项 | 落点 | 内容 |
|---|---|---|
| 图标 | `dsh-plugin/icon.svg`（`package.json` `icon` 字段） | 128×128，橙紫色闪电 + 环形箭头 |
| 中文 | `dsh-plugin/locale/zh.json` | 会话规则注入(rulemux-dsh) |
| 英文 | `dsh-plugin/locale/en.json` | Session Rules Injection (rulemux-dsh) |

> 标题写法沿用参考实现：**前半句说干什么 + 半角括号包完整包名**（中文括号前无空格，英文有空格）。
