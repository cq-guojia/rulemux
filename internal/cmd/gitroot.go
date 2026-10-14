package cmd

import (
	"os"
	"path/filepath"
)

// gitRoot 返回 dir 自身、或其最近的「含 .git 的祖先目录」；一路找不到返回空串。
//
// 与 dsh 宿主的 findProjectRoot 同构（deepseek-ai/deepseek-harness@master，
// packages/skill/skill-filesystem/src/index.ts：从 cwd 逐级向上找 .git，找不到回退 cwd）：
// 只给「宿主按 .git 定项目根」的 agent 用（registry 的 WorkspaceIsGitRoot，目前 dsh）。
// 呼叫方在拿到空串时应保持原目录不变（回退语义与宿主一致）。
//
// .git 是文件（worktree / 子模块）时同样算数 —— os.Stat 对文件也成功，与宿主
// 「.git 存在即项目根」的判据一致。
func gitRoot(dir string) string {
	cur := dir
	for {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "" // 到达文件系统根仍未找到
		}
		cur = parent
	}
}
