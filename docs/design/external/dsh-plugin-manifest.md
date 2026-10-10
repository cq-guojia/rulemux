# dsh 插件的展示元信息（图标 / 标题 / 说明）

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

## 三、本包现状

| 项 | 落点 | 内容 |
|---|---|---|
| 图标 | `dsh-plugin/icon.svg`（`package.json` `icon` 字段） | 128×128，橙紫色闪电 + 环形箭头 |
| 中文 | `dsh-plugin/locale/zh.json` | 会话规则注入(rulemux-dsh) |
| 英文 | `dsh-plugin/locale/en.json` | Session Rules Injection (rulemux-dsh) |

> 标题写法沿用参考实现：**前半句说干什么 + 半角括号包完整包名**（中文括号前无空格，英文有空格）。
