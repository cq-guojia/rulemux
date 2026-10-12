# 20261010-0000-dsh-adapter-9f1e：DeepSeek Harness（dsh）适配器 canary 坐实 + 插件就绪收口

> **✅ 完成封卷**（2026-10-10 00:00，GuoJia-Agent）
> **创建**：2026-10-10 00:00（GuoJia-Agent）

## 背景
新增 dsh（DeepSeek Harness）适配器。dsh 是 **plugin-first** 宿主（Cordis 生命周期事件）：**没有 hook binary**，也**不原生扫描规则目录**。需与已实现的 Go 同步引擎配合。

## 欠什么
（已结案）npm 发布 `rulemux-dsh` 待定（git 装已可用）；二期 `$DSH_HOME/rules` 全局规则明确不做。

## 下一步
（已结案）

## 过程
定稿方案=拆两半：**写**一半由 Go 二进制 `rulemux sync` → `<cwd>/.dsh/rules/__rulemux__*.md`；**读**一半由仓库子包子包 `rulemux-dsh` 在 `agent/pre-step` 读 `.dsh/rules` 注入一次。守住三不变量（真实文件非软链 / engine 删残留不累积 / 首轮 pre-step 注入一次不淡出）；两阶段各只跑一次。
就绪=三步全成或抛错：① CLI 可得且版本合规或认识 dsh；② 插件已装载；③ `~/.rulemux/config.toml` 存在；已有即算成功、绝不覆盖。同步与就绪解耦（sync 退出非 0 只 `console.error` 记日志）。否决方案 A（Go 内嵌插件），改子包方案，A 的代码已删。
真机（容器 `ccaf4e6ccb4a`）canary 通过：装插件+重启→装载期生成 `~/.rulemux/config.toml`、注入一次成功、token 成本与宿主原生读文件字节一致且命中提示缓存。
修发布缺口：npm `latest` 当时是 0.3.0，但 0.3.0 不含 dsh（dsh 提交全在 `v0.3.0` 之后）；`REQUIRED_CLI` 由 `>=0.3.0` 改为 `>=0.3.1` 并先发 0.3.1。另一坑：版本号不能当能力凭证——`main.go` 的 `version` 是写死默认值，不带 ldflags 编出的二进制会自报 0.3.0 但其实能干（含 dsh），判据改为 `cliOk()` = 版本满足或 `knowsDsh()`（`--help` 匹配 `dsh`）。

## 结论
dsh 适配器 canary 坐实、插件就绪收口；registry 标 Tier1 / `Style=external` / `Verified=true`；CLI 发布缺口 `v0.3.1` 已发、npm `latest` 闭环。实现进展与真源出处已记 `docs/design/external/agent-rules-dirs.md` §一。
