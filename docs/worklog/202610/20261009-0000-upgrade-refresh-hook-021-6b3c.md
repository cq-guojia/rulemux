> **✅ 完成封卷**（2026-10-09 00:00，GuoJia-Agent）
`hooks.Inspect`/`Refresh`：只重写 rulemux 自己装过的钩子条目（含 `--workbuddy` 别名旧格式）、**不创建文件或目录**、已是最新不写盘；新增 `rulemux init --refresh`（无 `--agent` 时遍历全部注册 agent 只刷已装者）；npm `postinstall`（仅全局安装、非 CI、非 sudo、任何异常 exit 0）实现「全局升级即自动刷新」；`doctor` 钩子检测改三态（最新/已装但格式过期/未装）并给修复命令；核实 CodeBuddy 4.12.1 的 `CODEBUDDY_CONFIG_DIR`（`getCodeBuddyHomeDir()`）并落地为「环境变量优先、缺失回退 `~`」
