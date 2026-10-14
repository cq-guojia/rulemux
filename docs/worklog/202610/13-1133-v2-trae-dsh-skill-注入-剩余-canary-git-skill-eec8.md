# 20261013-1133-v2-trae-dsh-skill-注入-剩余-canary-git-skill-eec8：v2：Trae/dsh skill 注入 + 剩余 canary + git skill 源

> **创建**：2026-10-13 11:33:51（@GuoJia-Agent）

## 背景

v1 skill 注入（codebuddy / workbuddy）2026-10-12 交付后，用户定序「Trae → DeepSeek harness」推进下一家。
2026-10-14 用户原话：

> 「Codebuddy 和 Workbuddy 的技能同步都已经搞定了对吧？现在我们继续推进 DeepSeek harness 的 skill 注入。
> 你先去做调研，给我一个方案。我不知道 DeepSeek harness 的技能是怎么弄的，是还像 hook 一样通过插件注入进去，
> 还是它有单独的技能知识包？你去好好看看它的文档或者原文件。」

调研结论（详见「过程」§一）：dsh 有**原生 skills 子系统**（多根目录扫描 + 优先级 rank），与
codebuddy/workbuddy 装载逻辑同类；规则侧才是纯插件注入。用户逐条拍板落点（2026-10-14）：

1. 用户级 `~/.dsh/skills`（「全局跟我理解了，没问题」「我接受」）；
2. 工作区级 `<工作目录>/.dsh/skills`（「他就认，是吧？可以」）；
3. `.agents/skills`（项目级/用户级）是跨框架公约目录，**只读不写**；
4. git 根对齐执行（「行，可以，就这么定」）；插件从「workspace 零写入」扩展为「rules 仍零写入 + skills 落盘」。

## 待办

- [x] 调研：dsh skills 机制（源码级，2026-10-14，见过程 §一）
- [x] 用户拍板落点与策略（2026-10-14，见背景）
- [x] 文档回写：external/skill-ecosystem.md（§三·再续）、features/skill-sync.md（§七）、
      features/verification.md（dsh canary 四项）、README 中英双份（矩阵/§2/§5.4/§5.5/§8）
- [x] 代码：registry.go dsh 双域落点 + 注释；sync.go 放开 Tier-2 skills + git 根对齐；
      dsh-plugin sync 触发 + REQUIRED_CLI 抬版 + README 改口
- [x] 测试：registry/skill_sync/sync 单测 + smoke.sh dsh 步骤；go build/vet/test + smoke 全绿
- [ ] 发版配对：CLI `v0.5.0` + 插件 `dsh-v0.5.0`（AGENTS.md §5；用户 2026-10-14 指指令发 tag 后由用户测试）
- [ ] 真机 canary 四项（用户机器/容器 dsh）：① `__rulemux__` 前缀目录真装载；
      ② 会话中 sync 的热重载时序；③ `~/.dsh/skills` 用户级装载；④ `DSH_HOME` 错位
- [ ] Trae skills 接入（本任务另一条线，未排期）
- [ ] git/远程 skill 源（S3）与 `rulemux skill` 子命令（后续版本）

## 下一步

1. 用户已在真机 dsh 测试：装 `rulemux-dsh@0.5.0` + CLI 0.5.0，配 `[[skill_source]]`（agents=["dsh"]），
   开会话看 `<git根>/.dsh/skills` 是否出现 `__rulemux__<技能名>/` 且 `/` 面板 Skills 分组可见；
2. canary 四项按 verification.md「skill 装载 canary（dsh）」逐项过，结果回填本文件「过程」§三；
3. Trae 线：`.trae/skills` 工作区 canary + intl `~/.trae/skills` 探测（external/skill-ecosystem.md §六-3）。

## 过程

### 一、调研（2026-10-14，官方仓库源码级）

对象：`deepseek-ai/deepseek-harness` @ master，经 GitHub contents API 读取
`packages/skill/skill-filesystem/src/index.ts` + `packages/skill/skill/src/index.ts`。

| 问题 | 结论 |
|---|---|
| 根目录与 rank | rank 小者胜（`compareIndexedCandidates`）：项目级 `.dsh/skills`=100、`.agents/skills`=200、custom=300；用户级 `~/.dsh/skills`=400（`DSH_HOME`）、`~/.agents/skills`=500（`DSH_AGENTS_HOME`）；bundled=600；插件 `ctx.skills.registerProvider`=250 |
| 扫描 | 非递归：每根一次 readdir；`<name>/SKILL.md` 与 `<name>.md` 两形态 |
| frontmatter | `name`+`description` 必填；`isSkillName`=`^[a-z0-9]+(?:-[a-z0-9]+)*$` **只校验 frontmatter name、不校验目录名** ⇒ `__rulemux__` 前缀安全；旧驼峰键（`disableModelInvocation` 等）⇒ 整技能被丢弃 |
| 热重载 | 每根 chokidar watcher（depth:1、awaitWriteFinish 200ms）+ `fs/observed` 宿主写观察 |
| 项目根 | `findProjectRoot`：从 cwd 向上找最近 `.git`，找不到回退 cwd |

结论已回写 external/skill-ecosystem.md §三·再续（头部标适用版本 master@2026-10-14）。

### 二、实现（2026-10-14）

- `internal/agents/registry.go`：dsh 条目加 `SkillsDir=".dsh/skills"`、`UserSkillsDirs=["~/.dsh/skills"]`，
  注释写明 rank 依据与「不写 .agents」；Tier 维持 Tier-2（规则仍纯注入）。
- `internal/cmd/sync.go`：主循环跳过条件 `Tier==Tier2` → `Tier==Tier2 && SkillsDir==""`
  （codex/opencode 三字段空 ⇒ 零行为变化）；ws 解析对 dsh 走 git 根对齐（新 helper，标准库实现）。
- `dsh-plugin/index.mjs`：`agent/session-start` 发起 + 首个 `agent/pre-step` await 兜底
  `rulemux sync --hook --agent dsh`（`--hook` 为拿目录创建权所必需；stdin=/dev/null 时载荷解析
  自动落败回退 cwd）；失败只记日志；`REQUIRED_CLI` 抬至 `>=0.5.0`；文件头与 README 改口
  （rules 零写入不变，skills 落 `.dsh/skills`/`~/.dsh/skills`）。
- 测试：registry 断言 dsh 双域；skill_sync 加「Tier-2 + SkillsDir 落盘」与 create 门禁用例；
  sync 加 git 根解析用例（仓库根/子目录/非 git 回退）；smoke.sh 加 dsh skills 端到端步。
- 验证：`go build ./...` / `go vet ./...` / `go test ./...` / `scripts/smoke.sh` 全绿；koc-codelint 通过。

### 三、canary 结果（待用户真机测试后回填）

<!-- 四项 canary 的实测结果写这里：通过/失败 + 证据（会话截图/目录列表/dsh 日志行） -->

## 结论

<!-- 结案时填：做成了什么、留了什么坑 -->
