# 20261009-0000-upgrade-refresh-hook-021-6b3c：升级即刷新钩子 + CodeBuddy 配置目录核实（发 `v0.2.1`）

> **✅ 完成封卷**：2026-10-09 00:00:00（@GuoJia-Agent）
> **创建**：2026-10-09 00:00:00（@GuoJia-Agent）

## 背景
本工作包背景见历史索引摘要（原过程文档未单列背景）。

## 待办
（已结案，原过程文档未单列待办项）

## 下一步
（已结案，无后续动作）

## 过程
`hooks.Inspect`/`Refresh`：只重写 rulemux 自己装过的钩子条目（含 `--workbuddy` 别名旧格式）、**不创建文件或目录**、已是最新不写盘；新增 `rulemux init --refresh`（无 `--agent` 时遍历全部注册 agent 只刷已装者）；npm `postinstall`（仅全局安装、非 CI、非 sudo、任何异常 exit 0）实现「全局升级即自动刷新」；`doctor` 钩子检测改三态（最新/已装但格式过期/未装）并给修复命令；核实 CodeBuddy 4.12.1 的 `CODEBUDDY_CONFIG_DIR`（`getCodeBuddyHomeDir()`）并落地为「环境变量优先、缺失回退 `~`」

## 结论
已完成。成果即上方过程所述，亦见历史索引摘要。
