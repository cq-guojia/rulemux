/**
 * rulemux-dsh 的浏览器半边：在插件详情页挂一张**只读说明**面板。
 *
 * 只做两件事：注册词典、注册 `plugins.bundle.config` 槽位（key = 包名）。
 *
 * ⚠ **不要给注册加 `configForms.whileServed()`**：详情页是否渲染配置区，取决于
 * 这个槽位有没有以该包名为 key 的注册项 —— 配置账本就是这么算的
 * （`dsh-client-ui-plugin-manager/lib/client.js:62` `bundles: keysOf("plugins.bundle.config")`，
 * 详情页 `configured: ledger.bundles.has(openPkg.name)` 同文件 `:3545`），与设置命名空间无关。
 * 0.3.4 那一版正是被 whileServed 挡住：命名空间没登记 ⇒ 永不注册 ⇒ 面板不出现。
 *
 * 刻意不导出 `inject`：客户端 entry 若声明了当前组合无法满足的依赖，会一直 pending，
 * 而 pending 的 entry 会让整个 dsh 启动失败。这里改用 apply 内的 `ctx.inject()` 延迟
 * 等待 —— 依赖没出现的最坏结果只是「详情页没有说明区」。
 */
import type { Context } from '@deepseek-ai/cordis';
// 下面几个只为拿到类型增强（SlotMap / ctx.slots），全部是 type-only，运行时不会引入，
// 因此不会触发客户端产物纯度闸门。
import type {} from '@deepseek-ai/dsh-client-ui-plugin-manager/client';
import type {} from '@deepseek-ai/dsh-client-ui-renderer/client';
import type {} from '@deepseek-ai/dsh-client-locale/client';

import { ConfigPanel } from './config-panel';
import { LOCALE_NS, zh, en } from './locales';

export const name = 'rulemux-dsh';

/**
 * 插件详情页配置槽位。keyed 槽位，key 必须与包名逐字相同。
 */
const BUNDLE_CONFIG_SLOT = 'plugins.bundle.config';

/** 排查用的日志前缀：控制台里搜它就能知道浏览器半边到底跑没跑。 */
const LOG = '[rulemux-dsh]';

export function apply(ctx: Context): void {
  console.info(`${LOG} client apply()`);

  // 界面文案（中英双语）。词典注册要等 locale 服务，所以用 ctx.inject 延迟等待。
  ctx.inject(['locale'], (localeCtx) => {
    ctx.effect(() => localeCtx.locale.register(LOCALE_NS, { zh, en }));
    console.info(`${LOG} 词典已注册 (${LOCALE_NS})`);
  });

  // 只等 slots：slots.inject 会等页面 owner 声明该槽位，owner 折叠时贡献自动移除。
  ctx.inject(['slots'], (sub) => {
    ctx.effect(() =>
      sub.slots.inject(BUNDLE_CONFIG_SLOT, () =>
        sub.slots.register(
          {
            name: BUNDLE_CONFIG_SLOT,
            key: name,
            locale: LOCALE_NS,
          },
          ConfigPanel,
        ),
      ),
    );
    console.info(`${LOG} 已注册说明面板到 ${BUNDLE_CONFIG_SLOT}`);
  });
}
