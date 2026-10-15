# 20261010-0000-dsh-plugin-settings-ui-2m8n：DSH 插件设置界面结案（列表展示 0.3.4 + 详情页说明面板 0.3.5）+ 两个 npm 包首发

> **✅ 完成封卷**：2026-10-10 00:00:00（@GuoJia-Agent）
> **创建**：2026-10-10 00:00:00（@GuoJia-Agent）

## 背景
`rulemux-dsh` 没有 dsh 原生设置界面，用户配置全靠手改 `~/.rulemux/config.toml`（Go CLI 配置，插件装载期用 `rulemux init --agent dsh` 生成满注释样板，再由用户填 `[[source]]`）。目标：给 rulemux-dsh 加一个 dsh 设置界面。

## 待办
（已结案）详情页是否出现面板、中英切换、复制按钮、外链可点，待用户实机确认。

## 下一步
（已结案）

## 过程
① 列表展示（0.3.4）：图标 `dsh-plugin/icon.svg`（128×128，取自用户给的 `话术提示词-选中.svg`，原件已删）；标题/说明 `locale/{zh,en}.json` 的 `meta.title/description`（写法照 `dsh-session-title-pattern`）；简介口径改为浓缩核心痛点（唯一真源 / 层级下放复用 / 一处修改全量同步）；版本 0.3.3 → 0.3.4。
② 拍板设置区做成「只读说明面板」不做保存：核实——官方配置表单值只能存进 dsh 自己的设置文档（host 端无读写拦截钩子）；详情页 `scope:'root'`、无 sessionId ⇒ 浏览器端读不到磁盘文件；用户不接受把配置存进官方当第二份真源 ⇒ 放弃编辑/保存/重置，改只读说明。落码：`src/client/index.tsx` 注册 `plugins.bundle.config`、`config-panel.tsx` 说明面板（路径+复制+示例 toml+GitHub 外链）、`locales.ts` 中英词典、`index.mjs` 新增 `Config` 导出（占位字段只为让宿主登记命名空间）、构建链产出 `lib/client.js` 提交进仓库。
③ 0.3.5 修正面板不出现根因：详情页渲染判据 `configured = ledger.bundles.has(openPkg.name)` 只看注册不看命名空间，而 0.3.4 把注册包在 `configForms.whileServed(['rulemux'])` 里（判据要 host 侧 `Config` 派生命名空间，依赖没装上就不回调 ⇒ 从不注册 ⇒ 面板消失）。修法：client 直接 `slots.inject`+`slots.register`，去掉 `configForms`/`whileServed`；host 侧 `Config` 与 schemastery 依赖一并去掉，回到零运行时依赖；加三条 `console.info` 便于排查「浏览器半边没加载」还是「加载了但槽位没渲染」。

## 结论
DSH 插件设置界面结案：列表展示 0.3.4 + 详情页只读说明面板 0.3.5；npm 主包 `rulemux` 发 `v0.3.2`、插件包 `rulemux-dsh` 首发 `v0.3.5`；结论回写 `docs/design/external/dsh-plugin-manifest.md`。
