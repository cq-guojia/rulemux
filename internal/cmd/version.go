package cmd

// Version 是 rulemux 的版本号，由 main 包在 init 时同步注入。
//
// 真源是 main.version：发布管线用 -ldflags="-X main.version=<tag>" 注入它
// （见 .github/workflows/publish.yml）。cmd 包不持第二份真源，只在启动时
// 复制过来供 doctor 等命令展示。
var Version = "dev"
