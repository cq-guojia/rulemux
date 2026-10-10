window.__ModuleLoader__.load({
	id: "rulemux-dsh",
	factory: (require) => {
		var module = { exports: {} };
		var exports = module.exports;
		Object.defineProperty(exports, Symbol.toStringTag, { value: "Module" });
		//#region \0rolldown/runtime.js
		var __create = Object.create;
		var __defProp = Object.defineProperty;
		var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
		var __getOwnPropNames = Object.getOwnPropertyNames;
		var __getProtoOf = Object.getPrototypeOf;
		var __hasOwnProp = Object.prototype.hasOwnProperty;
		var __copyProps = (to, from, except, desc) => {
			if (from && typeof from === "object" || typeof from === "function") for (var keys = __getOwnPropNames(from), i = 0, n = keys.length, key; i < n; i++) {
				key = keys[i];
				if (!__hasOwnProp.call(to, key) && key !== except) __defProp(to, key, {
					get: ((k) => from[k]).bind(null, key),
					enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable
				});
			}
			return to;
		};
		var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(isNodeMode || !mod || !mod.__esModule || !__hasOwnProp.call(mod, "default") ? __defProp(target, "default", {
			value: mod,
			enumerable: true
		}) : target, mod));
		//#endregion
		let react = require("react");
		let _deepseek_ai_dsh_client_ui_primitives = require("@deepseek-ai/dsh-client-ui-primitives");
		_deepseek_ai_dsh_client_ui_primitives = __toESM(_deepseek_ai_dsh_client_ui_primitives, 1);
		let react_jsx_runtime = require("react/jsx-runtime");
		//#region src/client/config-panel.tsx
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
		/** 仓库地址：底部外链指向它（那里是完整帮助文档）。 */
		const DOCS_URL = "https://github.com/cq-guojia/rulemux";
		/** 配置文件路径。`~` 是用户家目录，跨机器、跨平台都这么写。 */
		const CONFIG_PATH = "~/.rulemux/config.toml";
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
		/** 链接图标的兜底链：上游改名时退回不渲染图标，绝不整块消失。 */
		const LinkIcon = _deepseek_ai_dsh_client_ui_primitives.IconLinkOutlineRegular ?? _deepseek_ai_dsh_client_ui_primitives.IconLinkOutlineMedium ?? null;
		function ConfigPanel({ view, t }) {
			const [copied, setCopied] = (0, react.useState)("idle");
			if (view !== "page") return null;
			const copyPath = () => {
				const clipboard = navigator.clipboard;
				if (clipboard === void 0 || typeof clipboard.writeText !== "function") {
					setCopied("failed");
					return;
				}
				clipboard.writeText(CONFIG_PATH).then(() => setCopied("ok"), () => setCopied("failed"));
			};
			const title = {
				fontSize: 13,
				fontWeight: 600,
				color: "var(--dsw-alias-label-primary)"
			};
			const body = {
				fontSize: 12,
				lineHeight: 1.7,
				color: "var(--dsw-alias-label-secondary)"
			};
			return /* @__PURE__ */ (0, react_jsx_runtime.jsxs)("div", {
				style: {
					display: "flex",
					flexDirection: "column",
					gap: 14
				},
				children: [
					/* @__PURE__ */ (0, react_jsx_runtime.jsxs)("div", {
						style: {
							display: "flex",
							flexDirection: "column",
							gap: 6
						},
						children: [
							/* @__PURE__ */ (0, react_jsx_runtime.jsx)("span", {
								style: title,
								children: t("whereTitle")
							}),
							/* @__PURE__ */ (0, react_jsx_runtime.jsxs)("div", {
								style: {
									display: "flex",
									alignItems: "center",
									gap: 8
								},
								children: [/* @__PURE__ */ (0, react_jsx_runtime.jsx)("code", {
									style: {
										flex: 1,
										fontFamily: "var(--dsw-alias-font-mono, monospace)",
										fontSize: 12,
										color: "var(--dsw-alias-label-primary)",
										background: "var(--dsw-alias-bg-layer-3)",
										border: ".5px solid var(--dsw-alias-border-l4)",
										borderRadius: 8,
										padding: "6px 10px",
										overflowX: "auto",
										whiteSpace: "nowrap"
									},
									children: CONFIG_PATH
								}), /* @__PURE__ */ (0, react_jsx_runtime.jsx)("button", {
									type: "button",
									onClick: copyPath,
									"aria-label": t("pathLabel"),
									style: {
										appearance: "none",
										font: "inherit",
										cursor: "pointer",
										background: "transparent",
										border: ".5px solid var(--dsw-alias-border-l4)",
										borderRadius: 8,
										padding: "0 12px",
										height: 30,
										fontSize: 12,
										color: "var(--dsw-alias-label-primary)",
										whiteSpace: "nowrap"
									},
									children: copied === "ok" ? t("copied") : t("copy")
								})]
							}),
							copied === "failed" ? /* @__PURE__ */ (0, react_jsx_runtime.jsx)("span", {
								style: body,
								children: t("copyFailed")
							}) : null
						]
					}),
					/* @__PURE__ */ (0, react_jsx_runtime.jsxs)("div", {
						style: {
							display: "flex",
							flexDirection: "column",
							gap: 6
						},
						children: [/* @__PURE__ */ (0, react_jsx_runtime.jsx)("span", {
							style: title,
							children: t("manualTitle")
						}), /* @__PURE__ */ (0, react_jsx_runtime.jsx)("span", {
							style: body,
							children: t("manualBody")
						})]
					}),
					/* @__PURE__ */ (0, react_jsx_runtime.jsxs)("div", {
						style: {
							display: "flex",
							flexDirection: "column",
							gap: 6
						},
						children: [
							/* @__PURE__ */ (0, react_jsx_runtime.jsx)("span", {
								style: title,
								children: t("exampleTitle")
							}),
							/* @__PURE__ */ (0, react_jsx_runtime.jsx)("pre", {
								style: {
									margin: 0,
									fontFamily: "var(--dsw-alias-font-mono, monospace)",
									fontSize: 12,
									lineHeight: 1.6,
									color: "var(--dsw-alias-label-primary)",
									background: "var(--dsw-alias-bg-layer-3)",
									border: ".5px solid var(--dsw-alias-border-l4)",
									borderRadius: 8,
									padding: 10,
									overflowX: "auto"
								},
								children: EXAMPLE
							}),
							/* @__PURE__ */ (0, react_jsx_runtime.jsx)("span", {
								style: body,
								children: t("exampleHint")
							})
						]
					}),
					/* @__PURE__ */ (0, react_jsx_runtime.jsxs)("a", {
						href: DOCS_URL,
						target: "_blank",
						rel: "noopener noreferrer",
						"aria-label": t("docsAria"),
						style: {
							alignSelf: "flex-start",
							display: "inline-flex",
							alignItems: "center",
							gap: 6,
							font: "inherit",
							fontSize: 13,
							textDecoration: "none",
							color: "var(--dsw-alias-label-primary)",
							background: "transparent",
							border: ".5px solid var(--dsw-alias-border-l4)",
							borderRadius: 8,
							padding: "0 12px",
							height: 30,
							lineHeight: "30px"
						},
						children: [LinkIcon === null ? null : /* @__PURE__ */ (0, react_jsx_runtime.jsx)(LinkIcon, { size: 16 }), /* @__PURE__ */ (0, react_jsx_runtime.jsx)("span", { children: t("docs") })]
					})
				]
			});
		}
		//#endregion
		//#region src/client/locales.ts
		/** 词典命名空间（= 设置命名空间 = profile entry id `rulemux`）。 */
		const LOCALE_NS = "rulemux";
		/** 简体中文词典。 */
		const zh = {
			whereTitle: "规则配置在哪",
			pathLabel: "配置文件",
			copy: "复制",
			copied: "已复制",
			copyFailed: "复制失败，请手动选中上面的路径",
			manualTitle: "怎么改",
			manualBody: "用编辑器打开上面这个文件，在里面列出你的规则源文件、它们给哪些 agent、给哪些工作区；保存后开一个新会话，rulemux 就会把规则同步进各家 agent 的规则目录。本面板只负责告诉你文件在哪、长什么样 —— 它不保存任何内容，真源永远是这个文件本身。",
			exampleTitle: "示例",
			exampleHint: "把 path 换成你自己的规则文件路径即可，其余字段可不写（不写 = 所有 agent / 所有工作区）。",
			docs: "访问 GitHub",
			docsAria: "在 GitHub 上打开 rulemux 的帮助文档"
		};
		/** English dictionary. */
		const en = {
			whereTitle: "Where the rules are configured",
			pathLabel: "Config file",
			copy: "Copy",
			copied: "Copied",
			copyFailed: "Copy failed — select the path above by hand",
			manualTitle: "How to edit it",
			manualBody: "Open the file above in an editor and list your rule sources there, which agents they go to and which workspaces they apply to; save it and start a new session — rulemux then syncs the rules into every agent’s rules directory. This panel only tells you where the file is and what it looks like: it saves nothing, and that file stays the single source of truth.",
			exampleTitle: "Example",
			exampleHint: "Replace path with your own rule files; every other field is optional (omitted = every agent / every workspace).",
			docs: "Open GitHub",
			docsAria: "Open the rulemux documentation on GitHub"
		};
		//#endregion
		//#region src/client/index.tsx
		const name = "rulemux-dsh";
		/**
		* 插件详情页配置槽位。keyed 槽位，key 必须与包名逐字相同。
		*/
		const BUNDLE_CONFIG_SLOT = "plugins.bundle.config";
		/** 排查用的日志前缀：控制台里搜它就能知道浏览器半边到底跑没跑。 */
		const LOG = "[rulemux-dsh]";
		function apply(ctx) {
			console.info(`${LOG} client apply()`);
			ctx.inject(["locale"], (localeCtx) => {
				ctx.effect(() => localeCtx.locale.register(LOCALE_NS, {
					zh,
					en
				}));
				console.info(`${LOG} 词典已注册 (${LOCALE_NS})`);
			});
			ctx.inject(["slots"], (sub) => {
				ctx.effect(() => sub.slots.inject(BUNDLE_CONFIG_SLOT, () => sub.slots.register({
					name: BUNDLE_CONFIG_SLOT,
					key: name,
					locale: LOCALE_NS
				}, ConfigPanel)));
				console.info(`${LOG} 已注册说明面板到 ${BUNDLE_CONFIG_SLOT}`);
			});
		}
		//#endregion
		exports.apply = apply;
		exports.name = name;
		return module.exports;
	}
});

//# sourceMappingURL=client.js.map