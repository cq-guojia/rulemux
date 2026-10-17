package engine

// 远程 / 市场 skill 源（S3，docs/design/features/skill-sync.md §八）：
// 把「配置里的一个源写法」解析成本地目录——本地路径原样返回；git / npm 源按需
// 抓取进 ~/.rulemux/skills/<key>/（首次惰性 clone / pack，之后零网络）。
//
// 硬约束：Go 单二进制零第三方依赖 ⇒ 不引 git / npm 库，只 spawn 本机 git / npm
// 二进制；解包走标准库 archive/tar + compress/gzip。real-copy 底线不变（不软链）。

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SourceKind 是一条 skill 源的形态。
type SourceKind int

const (
	SourceLocal  SourceKind = iota // 本地目录
	SourceGitHub                   // gh:owner/repo（展开成 https GitHub）
	SourceGit                      // 完整 git URL（原样 clone，SSH/HTTPS 看 URL）
	SourceNPM                      // npm:pkg（npm pack 取包解包）
)

// 解压安全上限（借鉴 npx skills：≤25MiB 解压 / ≤1000 文件）。
const (
	maxExtractFiles = 1000
	maxExtractBytes = 25 << 20 // 25 MiB
)

// keySanitize 把任意串规整成 staging 目录名可用的小写片段。
var keySanitize = regexp.MustCompile(`[^a-z0-9]+`)

// ClassifySource 判定一条 skill 源 spec 的形态、staging key、抓取参照（ref）与子路径（sub）。
// 本地源 key 为空、ref 为原路径；调用方据 kind 决定是否抓取。sub 是 `#` 后的仓库内子路径
// （可选）：指向某容器目录或单个 skill 目录；staging key 只由 `#` 前的 base 决定，故同一 repo
// 的多个子路径共享一次 clone。
//
// 识别顺序（先显式前缀，再协议，再本地，最后裸 owner/repo 兜底）：
//   - npm:pkg / gh:owner/repo / github:owner/repo
//   - 完整 git URL（git@… / git:// / http(s):// / ssh://）
//   - 本地路径（/、~、./、../ 开头，或磁盘上已存在）
//   - 裸 owner/repo ⇒ 默认 GitHub
//   - 其余 ⇒ 按本地（BuildSkillPlan 会记 missing）
func ClassifySource(spec string) (kind SourceKind, key, ref, sub string) {
	base, sub := splitSubpath(spec)
	s := strings.TrimSpace(base)
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "npm:"):
		pkg := strings.TrimSpace(s[len("npm:"):])
		return SourceNPM, "npm-" + sanitizeKey(pkg), pkg, sub
	case strings.HasPrefix(lower, "gh:"):
		repo := strings.TrimSpace(s[len("gh:"):])
		return SourceGitHub, keyFromRepo(repo), "https://github.com/" + repo + ".git", sub
	case strings.HasPrefix(lower, "github:"):
		repo := strings.TrimSpace(s[len("github:"):])
		return SourceGitHub, keyFromRepo(repo), "https://github.com/" + repo + ".git", sub
	case isGitURL(s):
		return SourceGit, keyFromURL(s), s, sub
	case isLocalish(s):
		return SourceLocal, "", s, sub
	case isOwnerRepo(s):
		return SourceGitHub, keyFromRepo(s), "https://github.com/" + s + ".git", sub
	default:
		return SourceLocal, "", s, sub
	}
}

// splitSubpath 把 spec 按 `#` 拆成 base 与子路径。`#` 在 git URL / gh: / npm: 里都不出现，
// 故可无歧义作子路径分隔符（本地路径若含字面 `#` 需避开，属罕见）。
func splitSubpath(spec string) (base, sub string) {
	if i := strings.Index(spec, "#"); i >= 0 {
		return strings.TrimSpace(spec[:i]), strings.TrimSpace(spec[i+1:])
	}
	return spec, ""
}

// StagingDir 返回某 staging key 对应的本地目录（~/.rulemux/skills/<key>）。
func StagingDir(key string) string {
	base, err := stagingBase()
	if err != nil {
		return filepath.Join(".rulemux", "skills", key)
	}
	return filepath.Join(base, key)
}

// EnsureStaged 把一条源解析成本地目录：本地源原样返回（拼上子路径）；远程源若尚未抓取则按
// kind 抓取进 staging（带 timeout），已抓取则直接返回（零网络）。最终源根 = staging/本地 + 子路径。
func EnsureStaged(spec string, timeout time.Duration) (string, error) {
	kind, key, ref, sub := ClassifySource(spec)
	var root string
	if kind == SourceLocal {
		root = ref
	} else {
		root = StagingDir(key)
		if !isNonEmptyDir(root) {
			base, err := stagingBase()
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(base, 0o755); err != nil {
				return "", err
			}
			switch kind {
			case SourceGitHub, SourceGit:
				if err := gitClone(ref, root, timeout); err != nil {
					return "", err
				}
			case SourceNPM:
				if err := npmFetch(ref, root, timeout); err != nil {
					return "", err
				}
			default:
				return "", fmt.Errorf("unsupported skill source %q", spec)
			}
		}
	}
	if sub != "" {
		root = filepath.Join(root, filepath.FromSlash(sub))
	}
	return root, nil
}

// UpdateStaged 对一条远程源做一次升级，返回升级前后的版本标识（见 stagedVersion）。
// 若尚未抓取（首次）则惰性 clone / pack（等同 EnsureStaged 的首次抓取），不再报
// 「not staged」而中断用户；已抓取则 git 原地 pull / npm 重 pack 覆盖。本地源 no-op
// 返回空串。升级针对整个 repo（子路径不影响，pull 一次全更）。绝不删重克、不叠加。
func UpdateStaged(spec string, timeout time.Duration) (before, after string, err error) {
	kind, key, ref, _ := ClassifySource(spec)
	if kind == SourceLocal {
		return "", "", nil
	}
	root := StagingDir(key)
	if !isNonEmptyDir(root) {
		// 尚未抓取：update 顺带做首次 clone/pack（与 sync 同义），避免「not staged」报错打断用户。
		if _, err = EnsureStaged(spec, timeout); err != nil {
			return "", "", err
		}
		return "", stagedVersion(kind, root), nil
	}
	before = stagedVersion(kind, root)
	switch kind {
	case SourceGitHub, SourceGit:
		if err = gitPull(root, timeout); err != nil {
			return before, before, err
		}
	case SourceNPM:
		if err = os.RemoveAll(root); err != nil {
			return before, before, err
		}
		if err = npmFetch(ref, root, timeout); err != nil {
			return before, before, err
		}
	}
	after = stagedVersion(kind, root)
	return before, after, nil
}

// stagedVersion 返回已抓取源的当前版本标识：npm 取 package.json 的 version；
// git/github 取 git describe（见 gitDescribe）；本地源返回空串。
func stagedVersion(kind SourceKind, root string) string {
	switch kind {
	case SourceNPM:
		return npmVersion(root)
	case SourceGitHub, SourceGit:
		return gitDescribe(root)
	default:
		return ""
	}
}

// npmVersion 读 <root>/package.json 的 version 字段；文件或字段缺失返回空串。
func npmVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return pkg.Version
}

// gitDescribe 返回 HEAD 的可读版本标识：优先 `git describe --tags --always`
// （有 tag 显示 v1.2.3，无则短 hash），任一 git 调用失败兜底 `git rev-parse --short HEAD`。
func gitDescribe(dir string) string {
	if out, err := runGit(dir, "describe", "--tags", "--always"); err == nil && out != "" {
		return out
	}
	if out, err := runGit(dir, "rev-parse", "--short", "HEAD"); err == nil {
		return out
	}
	return ""
}

// runGit 在 dir 下执行 git 子命令，返回裁剪后的 stdout；超时 10s。
func runGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ---- 抓取实现 ----

func gitClone(url, dir string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, dir)
	var errbuf strings.Builder
	cmd.Stderr = &errbuf
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(dir) // 清半成品，避免下次误判「已 staged」
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git clone %s timed out after %s", url, timeout)
		}
		return fmt.Errorf("git clone %s: %v: %s", url, err, strings.TrimSpace(errbuf.String()))
	}
	return nil
}

func gitPull(dir string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only")
	var errbuf strings.Builder
	cmd.Stderr = &errbuf
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git pull in %s timed out after %s", dir, timeout)
		}
		return fmt.Errorf("git pull in %s: %v: %s", dir, err, strings.TrimSpace(errbuf.String()))
	}
	return nil
}

// npmFetch 用本机 npm pack 取包，再把 tarball 解包进 dest（strip package/ 前缀）。
func npmFetch(pkg, dest string, timeout time.Duration) error {
	tmp, err := os.MkdirTemp("", "rulemux-npm-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npm", "pack", pkg, "--pack-destination", tmp, "--silent")
	var errbuf strings.Builder
	cmd.Stderr = &errbuf
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("npm pack %s timed out after %s", pkg, timeout)
		}
		return fmt.Errorf("npm pack %s: %v: %s", pkg, err, strings.TrimSpace(errbuf.String()))
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return err
	}
	var tgz string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tgz") {
			tgz = filepath.Join(tmp, e.Name())
			break
		}
	}
	if tgz == "" {
		return fmt.Errorf("npm pack %s produced no tarball", pkg)
	}
	if err := extractTarGz(tgz, dest); err != nil {
		_ = os.RemoveAll(dest)
		return fmt.Errorf("extract %s: %w", pkg, err)
	}
	return nil
}

// extractTarGz 解包 tar.gz 进 dest，strip npm tarball 的 package/ 前缀，
// 带路径穿越（zip slip）防护与文件数 / 解压大小上限。符号链接等非普通文件一律跳过。
func extractTarGz(tgzPath, dest string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return err
	}

	var files int
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(hdr.Name, "package/")
		name = strings.TrimPrefix(name, "./")
		if name == "" {
			continue
		}
		target := filepath.Join(destAbs, filepath.FromSlash(name))
		if !withinDir(destAbs, target) {
			return fmt.Errorf("refusing to extract outside target: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			files++
			if files > maxExtractFiles {
				return fmt.Errorf("too many files (> %d)", maxExtractFiles)
			}
			total += hdr.Size
			if total > maxExtractBytes {
				return fmt.Errorf("extracted size exceeds %d bytes", maxExtractBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, hdr.Size)); err != nil {
				out.Close()
				return err
			}
			out.Close()
		default:
			// 符号链接 / 设备 / 其它非普通条目：跳过（skill 源应是普通文件树）。
		}
	}
	return nil
}

// ---- 判定辅助 ----

func stagingBase() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("cannot determine home directory: %v", err)
	}
	return filepath.Join(home, ".rulemux", "skills"), nil
}

func isNonEmptyDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

func withinDir(root, target string) bool {
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(os.PathSeparator))
}

func isGitURL(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "git@") ||
		strings.HasPrefix(l, "git://") ||
		strings.HasPrefix(l, "ssh://") ||
		strings.HasPrefix(l, "https://") ||
		strings.HasPrefix(l, "http://") ||
		strings.HasPrefix(l, "file://")
}

func isLocalish(s string) bool {
	if s == "" {
		return false
	}
	if s == "." || s == ".." ||
		strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") ||
		strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	if _, err := os.Stat(s); err == nil {
		return true // 磁盘上存在 ⇒ 本地
	}
	return false
}

// isOwnerRepo 判定裸 owner/repo（恰好一段斜杠、无协议/空白/@/:）。
func isOwnerRepo(s string) bool {
	if strings.ContainsAny(s, " \t\n:@") {
		return false
	}
	parts := strings.Split(s, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

// keyFromRepo 由 owner/repo 生成 key（去 .git 后缀后 sanitize）。
func keyFromRepo(repo string) string {
	return sanitizeKey(strings.TrimSuffix(strings.TrimSpace(repo), ".git"))
}

// keyFromURL 从 git URL 提取 owner/repo 生成 key（兼容 SSH git@host:owner/repo
// 与 https://host/owner/repo；仅取末两段）。
// shortcut: 不含 host ⇒ 不同 host 上同名 owner/repo 会撞同一 staging 目录（罕见；
// 真撞再把 host 并入 key）。
func keyFromURL(u string) string {
	s := u
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[i+1:] // 去 user@
	}
	s = strings.Replace(s, ":", "/", 1) // host:path → host/path
	s = strings.TrimSuffix(s, ".git")
	parts := strings.Split(strings.Trim(s, "/"), "/")
	switch {
	case len(parts) >= 2:
		s = parts[len(parts)-2] + "/" + parts[len(parts)-1]
	case len(parts) == 1:
		s = parts[0]
	}
	return sanitizeKey(s)
}

func sanitizeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = keySanitize.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
