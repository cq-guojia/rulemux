# 20261008-0000-hook-command-c3d4：修复 hook 命令格式（`args` 被宿主丢弃）

> **✅ 完成封卷**：2026-10-08 00:00:00（@GuoJia-Agent）
> **创建**：2026-10-08 00:00:00（@GuoJia-Agent）

## 背景
本机已装 rulemux、CodeBuddy 也显示钩子已装，但新会话里规则**从不生效**：`.codebuddy/rules` 一直是空目录。

## 待办
（已结案）

## 下一步
（已结案）

## 过程
定位：依次排除配置路径、二进制可执行性、PATH；一度误判为「会话 hook 是非交互进程、PATH 少了 rulemux」，后被证伪。决定性证据在 CodeBuddy 自己的执行日志：配置是 `"command":"rulemux"` + `"args":["sync","--agent","codebuddy"]`，实际执行的却只有裸 `rulemux`——**`args` 被丢弃**；裸 `rulemux`（无参数）只打印帮助、什么都不干 ⇒ `.codebuddy/rules` 永远空。
根因：**CodeBuddy 的 hook 只执行 `command` 字段（完整命令行），忽略 `args` 字段**。
修复：`internal/hooks/install.go` 的 `command` 改为整串 `rulemux <subcmd> --agent <id>`，不再写 `args`；安装时先剔除该 agent 的旧式（args）钩子、再统一写新式（幂等，且能把旧配置迁移过来）；新增 `internal/hooks/install_test.go` 回归断言（command 为整串且无 args、旧式会被迁移、别家条目原样保留、`uninstall` 正确移除）；本机以新代码重跑 `init --agent codebuddy`，旧钩子已就地迁移为 `rulemux sync --agent codebuddy`。

## 结论
修复 hook 命令格式（参数写进 `command` 整串），规则恢复同步；外部事实回写 `docs/design/external/agent-rules-dirs.md` §二「hook 执行语义（2026-10-08 实测坐实）」。
