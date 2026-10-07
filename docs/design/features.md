# 功能总索引

> **状态**：✅ 已定（2026-10-07 拍板，措辞与 Tier 分层对齐 `implementation.md`）
> **来源**：用户前期调研（原根 `DESIGN.md` §2 / §4 / §6）
> **配套**：[`requirements.md`](requirements.md)（用户要什么）· [`architecture.md`](architecture.md)（为什么这么设计）

| 功能 | 文档 | 干什么 | 状态 |
|---|---|---|---|
| 目录同步（Tier-1） | [`features/dir-sync.md`](features/dir-sync.md) | 把配置所列源文件 1:1 复制进各 agent 规则目录（加前缀 `.rulemux__` + 内容比对 + 删残留）—— 主方案 | ✅ 已定 |
| hook 注入（Tier-2 降级） | [`features/hook-injection.md`](features/hook-injection.md) | 单文件 agent（Codex/OpenCode）无目录可丢时的降级：SessionStart 注入，不碰用户 `AGENTS.md` | ✅ 已定 |
| 实测验证法 | [`features/verification.md`](features/verification.md) | 每接入一个 agent，验证「进没进上下文 / 会不会被压缩丢掉 / 点文件是否被读」 | ✅ 已定（canary 法） |
