/**
 * rulemux-dsh 的浏览器半边：在插件详情页挂一张**只读说明**面板。
 *
 * 只做两件事：注册词典、注册 `plugins.bundle.config` 槽位（key = 包名）。
 * 面板本身不读写任何设置值 —— 规则配置的真源是 `~/.rulemux/config.toml`，
 * 由用户自己改。
 *
 * 刻意不导出 `inject`：客户端 entry 若声明了当前组合无法满足的依赖，会一直
 * pending，而 pending 的 entry 会让整个 dsh 启动失败。这里改用 apply 内的
 * `ctx.inject()` 延迟等待 —— 依赖没出现的最坏结果只是「详情页没有说明区」。
 */
import type { Context } from '@deepseek-ai/cordis';
// 下面几个只为拿到类型增强（SlotMap / ctx.slots / ctx.configForms），全部是
// type-only，运行时不会引入，因此不会触发客户端产物纯度闸门。
import type {} from '@deepseek-ai/dsh-client-ui-plugin-manager/client';
import type {} from '@deepseek-ai/dsh-client-ui-renderer/client';
import type {} from '@deepseek-ai/dsh-client-ui-settings/client';
import type {} from '@deepseek-ai/dsh-client-locale/client';

import { ConfigPanel } from './config-panel';
import { LOCALE_NS, zh, en } from './locales';

export const name = 'rulemux-dsh';

/**
 * 插件详情页配置槽位（dsh 0.1.7）。keyed 槽位，key 必须与包名逐字相同 ——
 * 插件管理页用 `ledger.bundles.has(pkg.name)` 判断要不要在详情页渲染配置区。
 */
const BUNDLE_CONFIG_SLOT = 'plugins.bundle.config';

export function apply(ctx: Context): void {
  // 界面文案（中英双语）。词典注册要等 locale 服务，所以用 ctx.inject 延迟等待。
  ctx.inject(['locale'], (localeCtx) => {
    ctx.effect(() => localeCtx.locale.register(LOCALE_NS, { zh, en }));
  });

  ctx.inject(['slots', 'configForms'], (sub) => {
    // whileServed：宿主没登记这个命名空间时（比如 entry 被禁用）详情页不留痕迹。
    ctx.effect(() =>
      sub.configForms.whileServed([LOCALE_NS], () =>
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
      ),
    );
  });
}
