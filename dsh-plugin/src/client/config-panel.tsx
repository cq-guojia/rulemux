/**
 * 插件详情页的说明面板（`plugins.bundle.config` 槽位）。
 *
 * **刻意不做编辑、不做保存**：规则配置的真源是 `~/.rulemux/config.toml`，
 * dsh 的设置文档不是它的另一个家（那会变成第二份真源）。本面板只回答三件事：
 * 文件在哪、要手动改什么、改出来长什么样 —— 外加一个到 GitHub 的外链。
 *
 * 「打开」按钮为什么不做：浏览器端打不开宿主的本地文件，而这个槽位是
 * `scope: 'root'`、没有会话上下文，插件也没有自定义 remote 通道可用来让宿主
 * 代开。所以退一步给「复制路径」，复制失败也只是一条提示，不影响其它内容。
 */
import { useState } from 'react';
import type { PropsLocale } from '@deepseek-ai/dsh-client-ui-slots';
// 图标必须从命名空间按名取用并兜底：上游改名时具名导入会在运行时拿到 undefined，
// React 渲染直接抛错。
import * as primitives from '@deepseek-ai/dsh-client-ui-primitives';

import { LOCALE_NS } from './locales';

/** 仓库地址：底部外链指向它（那里是完整帮助文档）。 */
const DOCS_URL = 'https://github.com/cq-guojia/rulemux';

/** 配置文件路径。`~` 是用户家目录，跨机器、跨平台都这么写。 */
const CONFIG_PATH = '~/.rulemux/config.toml';

/** 示例：与 `rulemux init` 生成的样板同一形状。 */
const EXAMPLE = `[[source]]
path = ["/your/rules/base.md"]     # 你的规则文件
# agents = ["codebuddy"]           # 只给某几家；不写 = 所有 agent
# workspace = ["/work/proj-a"]     # 只给某几个工作区；不写 = 所有工作区

[[file_group]]
name = "base"                      # 可复用的一层，供下面 use 继承
path = ["/your/rules/base.md"]

[[file_group]]
name = "miniapp"
use  = ["base"]                    # 继承 base，再叠加自己的
path = ["/your/rules/miniapp.md"]

[[source]]
groups    = ["miniapp"]
workspace = ["/work/miniapp-a"]`;

/** 槽位组件收到的 props：owner 的 view + 框架注入的 t。 */
export type ConfigPanelProps = PropsLocale<typeof LOCALE_NS> & {
  /** 详情页要求的视图；bundle 配置只渲染 `page`。 */
  view: 'summary' | 'page';
};

/** 链接图标的兜底链：上游改名时退回不渲染图标，绝不整块消失。 */
const LinkIcon = ((primitives as unknown as Record<string, unknown>).IconLinkOutlineRegular ??
  (primitives as unknown as Record<string, unknown>).IconLinkOutlineMedium ??
  null) as ((props: { size?: number }) => React.ReactNode) | null;

export function ConfigPanel({ view, t }: ConfigPanelProps) {
  const [copied, setCopied] = useState<'idle' | 'ok' | 'failed'>('idle');

  if (view !== 'page') return null;

  const copyPath = (): void => {
    const clipboard = navigator.clipboard;
    if (clipboard === undefined || typeof clipboard.writeText !== 'function') {
      setCopied('failed');
      return;
    }
    void clipboard.writeText(CONFIG_PATH).then(
      () => setCopied('ok'),
      () => setCopied('failed'),
    );
  };

  const title: React.CSSProperties = {
    fontSize: 13,
    fontWeight: 600,
    color: 'var(--dsw-alias-label-primary)',
  };
  const body: React.CSSProperties = {
    fontSize: 12,
    lineHeight: 1.7,
    color: 'var(--dsw-alias-label-secondary)',
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <span style={title}>{t('whereTitle')}</span>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <code
            style={{
              flex: 1,
              fontFamily: 'var(--dsw-alias-font-mono, monospace)',
              fontSize: 12,
              color: 'var(--dsw-alias-label-primary)',
              background: 'var(--dsw-alias-bg-layer-3)',
              border: '.5px solid var(--dsw-alias-border-l4)',
              borderRadius: 8,
              padding: '6px 10px',
              overflowX: 'auto',
              whiteSpace: 'nowrap',
            }}
          >
            {CONFIG_PATH}
          </code>
          <button
            type="button"
            onClick={copyPath}
            aria-label={t('pathLabel')}
            style={{
              appearance: 'none',
              font: 'inherit',
              cursor: 'pointer',
              background: 'transparent',
              border: '.5px solid var(--dsw-alias-border-l4)',
              borderRadius: 8,
              padding: '0 12px',
              height: 30,
              fontSize: 12,
              color: 'var(--dsw-alias-label-primary)',
              whiteSpace: 'nowrap',
            }}
          >
            {copied === 'ok' ? t('copied') : t('copy')}
          </button>
        </div>
        {copied === 'failed' ? <span style={body}>{t('copyFailed')}</span> : null}
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <span style={title}>{t('manualTitle')}</span>
        <span style={body}>{t('manualBody')}</span>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <span style={title}>{t('exampleTitle')}</span>
        <pre
          style={{
            margin: 0,
            fontFamily: 'var(--dsw-alias-font-mono, monospace)',
            fontSize: 12,
            lineHeight: 1.6,
            color: 'var(--dsw-alias-label-primary)',
            background: 'var(--dsw-alias-bg-layer-3)',
            border: '.5px solid var(--dsw-alias-border-l4)',
            borderRadius: 8,
            padding: 10,
            overflowX: 'auto',
          }}
        >
          {EXAMPLE}
        </pre>
        <span style={body}>{t('exampleHint')}</span>
      </div>

      {/* 外链：官方没有通用 Link 组件，用原生 <a> 做成按钮外观；
          沿用官方外链习惯（https + target=_blank + noopener）。 */}
      <a
        href={DOCS_URL}
        target="_blank"
        rel="noopener noreferrer"
        aria-label={t('docsAria')}
        style={{
          alignSelf: 'flex-start',
          display: 'inline-flex',
          alignItems: 'center',
          gap: 6,
          font: 'inherit',
          fontSize: 13,
          textDecoration: 'none',
          color: 'var(--dsw-alias-label-primary)',
          background: 'transparent',
          border: '.5px solid var(--dsw-alias-border-l4)',
          borderRadius: 8,
          padding: '0 12px',
          height: 30,
          lineHeight: '30px',
        }}
      >
        {LinkIcon === null ? null : <LinkIcon size={16} />}
        <span>{t('docs')}</span>
      </a>
    </div>
  );
}
