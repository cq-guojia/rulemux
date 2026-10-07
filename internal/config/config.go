// Package config 负责读取 rulemux 的 TOML 配置。
//
// 配置形态（docs/design/implementation.md #6）：
//
//	[[source]]
//	path = "C:/rules/a.md"
//	agents = ["claude", "codex"]
//
// 本包只解析 rulemux 用到的 TOML 子集（[[source]] / path / agents / 注释），
// 目的是保持零第三方依赖，使将来在离线环境（如 NAS）也能 go build。
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Source 是配置里列出的一条源规则（可含一个或多个文件）。
type Source struct {
	// Paths 是源文件路径列表，每个都可以是磁盘上的任意位置。
	// 配置里既可写单个：path = "a.md"
	// 也可写数组：path = ["a.md", "b.md"]  —— 同一批文件共享下面的 agents。
	Paths []string
	// Agents 指定这批文件投递给哪些 agent；为空表示投递给全部 agent。
	Agents []string
}

// Config 是 rulemux 的完整配置。
type Config struct {
	Sources []Source
	// Workspace 是可选的工作区根路径。留空表示用「当前工作目录」——
	// 钩子调起 rulemux 时，agent 的 cwd 天然就是工作区，所以通常不用配。
	// 需要固定到某个目录时才写：workspace = "/path/to/project"
	Workspace string
	// File 是本次实际读取的配置文件路径。
	File string
}

// DefaultPath 返回默认配置路径 ~/.rulemux/config.toml。
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".rulemux", "config.toml")
	}
	return filepath.Join(home, ".rulemux", "config.toml")
}

// Load 读取并解析指定路径的配置文件。
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := &Config{File: path}
	sc := bufio.NewScanner(f)
	cur := -1 // 当前正在填充的 [[source]] 下标，-1 表示不在 source 表内

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// [[source]] 数组表
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
			if name == "source" {
				c.Sources = append(c.Sources, Source{})
				cur = len(c.Sources) - 1
			} else {
				cur = -1
			}
			continue
		}

		// 其它普通表（如 [features]），其键值不属于 source，忽略
		if strings.HasPrefix(line, "[") {
			cur = -1
			continue
		}

		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch {
		case cur < 0 && key == "workspace":
			// 顶层键：工作区根路径（可选）
			c.Workspace = unquote(val)
		case cur >= 0 && key == "path":
			// 既支持单个字符串，也支持数组
			c.Sources[cur].Paths = parsePaths(val)
		case cur >= 0 && key == "agents":
			c.Sources[cur].Agents = parseArray(val)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

// SourcesFor 返回应投递给指定 agent 的源文件列表。
// agents 字段为空表示该源投递给全部 agent。
func (c *Config) SourcesFor(agentID string) []Source {
	out := make([]Source, 0, len(c.Sources))
	for _, s := range c.Sources {
		if len(s.Agents) == 0 || containsFold(s.Agents, agentID) {
			out = append(out, s)
		}
	}
	return out
}

// Validate 校验配置是否可用。
func (c *Config) Validate() error {
	if len(c.Sources) == 0 {
		return fmt.Errorf("配置 %s 中没有 [[source]]，请先用 rulemux init 生成示例配置并填写源文件", c.File)
	}
	for i, s := range c.Sources {
		if len(s.Paths) == 0 {
			return fmt.Errorf("配置 %s 第 %d 条 [[source]] 缺少 path", c.File, i+1)
		}
	}
	return nil
}

// parsePaths 解析 path：单个字符串或数组都支持。
func parsePaths(val string) []string {
	if strings.HasPrefix(strings.TrimSpace(val), "[") {
		return parseArray(val)
	}
	if v := strings.TrimSpace(unquote(val)); v != "" {
		return []string{v}
	}
	return nil
}

// unquote 去掉字符串两侧的成对引号。
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseArray 解析 ["a", "b"] 形式的字符串数组。
func parseArray(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, unquote(p))
	}
	return out
}

// containsFold 忽略大小写判断切片是否包含某字符串。
func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), v) {
			return true
		}
	}
	return false
}
