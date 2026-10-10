/**
 * 本插件的界面文案词典（中英双语）。
 *
 * 走平台自带的 locale 服务（`@deepseek-ai/dsh-client-locale` 提供 `ctx.locale`）：
 * - `ctx.locale.register(NS, { zh, en })` 注册词典；两种内置语言都必须给全
 * - 槽位注册项加 `locale: NS` → 框架把 `t` 注入组件，语言切换自动重渲
 *
 * 本面板是**只读说明**：不编辑、不保存任何东西 —— 规则配置的真源始终是
 * `~/.rulemux/config.toml`，由用户自己改（见 README）。
 */
import type {} from '@deepseek-ai/dsh-client-locale/client';

/** 词典命名空间（= 设置命名空间 = profile entry id `rulemux`）。 */
export const LOCALE_NS = 'rulemux';

/** 本插件渲染的全部文案键。 */
export type RulemuxLocaleKey =
  | 'whereTitle'
  | 'pathLabel'
  | 'copy'
  | 'copied'
  | 'copyFailed'
  | 'manualTitle'
  | 'manualBody'
  | 'exampleTitle'
  | 'exampleHint'
  | 'docs'
  | 'docsAria';

declare module '@deepseek-ai/dsh-client-ui-slots' {
  interface LocaleNamespaceMap {
    rulemux: RulemuxLocaleKey;
  }
}

/** 简体中文词典。 */
export const zh: Record<RulemuxLocaleKey, string> = {
  whereTitle: '规则配置在哪',
  pathLabel: '配置文件',
  copy: '复制',
  copied: '已复制',
  copyFailed: '复制失败，请手动选中上面的路径',
  manualTitle: '怎么改',
  manualBody:
    '用编辑器打开上面这个文件，在里面列出你的规则源文件、它们给哪些 agent、给哪些工作区；' +
    '保存后开一个新会话，rulemux 就会把规则同步进各家 agent 的规则目录。' +
    '本面板只负责告诉你文件在哪、长什么样 —— 它不保存任何内容，真源永远是这个文件本身。',
  exampleTitle: '示例',
  exampleHint: '把 path 换成你自己的规则文件路径即可，其余字段可不写（不写 = 所有 agent / 所有工作区）。',
  docs: '访问 GitHub',
  docsAria: '在 GitHub 上打开 rulemux 的帮助文档',
};

/** English dictionary. */
export const en: Record<RulemuxLocaleKey, string> = {
  whereTitle: 'Where the rules are configured',
  pathLabel: 'Config file',
  copy: 'Copy',
  copied: 'Copied',
  copyFailed: 'Copy failed — select the path above by hand',
  manualTitle: 'How to edit it',
  manualBody:
    'Open the file above in an editor and list your rule sources there, which agents they go to ' +
    'and which workspaces they apply to; save it and start a new session — rulemux then syncs ' +
    'the rules into every agent’s rules directory. This panel only tells you where the file is ' +
    'and what it looks like: it saves nothing, and that file stays the single source of truth.',
  exampleTitle: 'Example',
  exampleHint:
    'Replace path with your own rule files; every other field is optional ' +
    '(omitted = every agent / every workspace).',
  docs: 'Open GitHub',
  docsAria: 'Open the rulemux documentation on GitHub',
};
