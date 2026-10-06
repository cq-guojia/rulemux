# 功能总索引

> **状态**：📝 待拍板
> **来源**：用户前期调研（原根 `DESIGN.md` §2 / §4 / §6）
> **配套**：[`requirements.md`](requirements.md)（用户要什么）· [`architecture.md`](architecture.md)（为什么这么设计）

| 功能 | 文档 | 干什么 | 状态 |
|---|---|---|---|
| 目录同步 | [`features/dir-sync.md`](features/dir-sync.md) | 把中央真源的每个文档同步进各 agent 的工作区规则目录（MD5 比对）—— 一级方案 | 📝 待拍板 |
| hook 注入 | [`features/hook-injection.md`](features/hook-injection.md) | 某 agent 无规则目录时的**降级**投递：`SessionStart` + `PreCompact` | 📝 待拍板 |
| 实测验证法 | [`features/verification.md`](features/verification.md) | 每接入一个 agent，验证「进没进上下文 / 会不会被压缩丢掉」 | 📝 待拍板 |
