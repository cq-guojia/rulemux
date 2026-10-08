# 修复 hook 命令格式（参数从 args 挪进 command 整串）

> **状态**：✅ 完成封卷（2026-10-08）
> **工作包**：hook-command-2026-10-08
> **配套**：[`../design/external/agent-rules-dirs.md`](../design/external/agent-rules-dirs.md) §二「hook 执行语义」· [`../PROGRESS-HISTORY.md`](../PROGRESS-HISTORY.md)

## 现象

本机已装 rulemux、CodeBuddy 也显示钩子已装，但新会话里规则**从不生效**：`.codebuddy/rules` 一直是空目录。

## 定位过程

1. 依次排除（均非病因）：
   - **配置路径**：`~/.rulemux/config.toml` 里 `path = ["/workspace/00.RULES/100.BASE.md"]` 绝对路径正确，真源存在且非空；本工作区命中 `open-lab` 源（`doctor` 标 ✓、非 skipped）。
   - **二进制可执行性**：`rulemux` 的 Go 二进制静态链接、零依赖，`env -i`（无 PATH、无 node）可独立运行。
   - **二进制在不在 PATH**：`/usr/local/bin/rulemux` 存在（symlink → npm shim），系统 PATH 可解析。
   - ⚠️ **一度误判**为「会话 hook 是非交互进程、其 PATH 少了 rulemux」——后被证伪（二进制确实在系统 PATH）。
2. **决定性证据（CodeBuddy 自己的执行日志）**：
   `~/.local/share/CodeBuddyExtension/Logs/CodeBuddyIDE/2026-10-08/rulemux__814bfc….log:93966,93968`：
   ```
   [HookExecutor] Executing 3 unique hooks for event SessionStart (3 total before dedup)
   [HookExecutor] Executing hook command: rulemux
   ```
   配置里明明是 `"command":"rulemux"` + `"args":["sync","--agent","codebuddy"]`，实际执行的却只有裸 `rulemux` —— **`args` 被丢弃**。
   而裸 `rulemux`（无参数）只打印帮助、**什么都不干** ⇒ `.codebuddy/rules` 永远空。

## 根因

**CodeBuddy 的 hook 只执行 `command` 字段（完整命令行），忽略 `args` 字段。** rulemux 把子命令与参数放在 `args` 里，被宿主丢弃 → 每次会话被执行为裸 `rulemux` → 不同步。

> 对照：Hindsight 的钩子把参数写进 command 整串（`node "<abs>/xx.js"`），被执行原样，故一直正常。

## 修复

- `internal/hooks/install.go`：钩子 `command` 改为**整串** `rulemux <subcmd> --agent <id>`，**不再写 `args`**；安装时先剔除该 agent 的旧式（args）钩子、再统一写新式（幂等，且能把旧配置迁移过来）。
- `internal/hooks/install_test.go`（新增）：回归断言 —— command 为整串且无 args；旧式会被迁移；其它工具的钩子原样保留；`uninstall` 能正确移除。
- 本机以新代码重跑 `init --agent codebuddy`，旧钩子已就地迁移为 `rulemux sync --agent codebuddy`。

## 证据出处

- CodeBuddy 执行日志：`…/Logs/CodeBuddyIDE/2026-10-08/rulemux__814bfc….log:93966,93968`。
- 外部事实回写：`design/external/agent-rules-dirs.md` §二「hook 执行语义（2026-10-08 实测坐实）」。
