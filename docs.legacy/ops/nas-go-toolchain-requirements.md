# 在 NAS 上安装并持久化 Go 工具链 —— 需求说明

> **面向对象**：维护本人 NAS 的 Agent（请评估并给出实施方案）
> **提出方**：rulemux 项目（一个 Go 写的 CLI 工具）
> **状态**：📋 待 NAS 维护方评估
> **日期**：2026-10-07

---

## 一、背景：rulemux 是什么、跑在哪里

- **rulemux** 是一个用 **Go** 写的命令行工具（单二进制）。它被各 AI coding agent（CodeBuddy / Claude Code / Trae / WorkBuddy 等）的 **SessionStart 钩子**调用，把用户自己维护的规则文件同步进各 agent 的规则目录，实现"一处管理、多处生效"。
- 这些 agent（至少 CodeBuddy Server）以及 rulemux 本身，都**跑在 NAS 的容器/镜像里** ⇒ **`rulemux` 二进制必须在 NAS 上**，钩子才调得到。
- 当前容器环境：**Debian GNU/Linux 13 (trixie)，架构 x86_64（linux/amd64）**。
- **关键约束（用户确认）**：该容器/镜像**重启后会丢弃容器可写层** —— 装在容器里的东西（包括 `apt install` 装的、解压到 `/usr/local/go` 的）重启即消失。

---

## 二、为什么要装 Go（必要性与边界）

### 2.1 核心事实：运行不需要 Go，编译才需要

| 场景 | 需要 Go 工具链？ | 需要什么 |
|---|---|---|
| **运行** rulemux（被 agent 钩子调用） | ❌ **完全不需要** | 只要一个编译好的 `rulemux` 二进制 |
| **编译** rulemux（把 `.go` 源码变成二进制） | ✅ **需要** | Go SDK 工具链（`go` 命令） |

Go 是**编译型**语言：产物是**自包含的原生机器码二进制**，零运行时依赖（不需要 Go 运行时、不需要 Node/Python/VM），拷贝到任何机器可直接执行。

### 2.2 那为什么 NAS 上还要装 Go？

因为**源码在 NAS 上，且源码会迭代**：

1. rulemux 需要新增/修改 agent 适配器（Claude Code / CodeBuddy / WorkBuddy / Trae 等各自的规则目录不同）、调整同步逻辑、升级版本 —— 每次改完都要**重新编译出新的二进制**。
2. 用户**明确不接受**"每次在别的机器上手动编译、再上传到 NAS"这种流程。
3. 因此**构建能力必须留在 NAS 上** ⇒ NAS 上必须有 Go 工具链，改完源码当场 `go build` 就能出新二进制并立即生效。

### 2.3 附带好处（交叉编译）

Go 支持交叉编译，NAS 上装**一个** Go 就能产出所有平台的产物，以后给 Windows / Mac 机器分发不用另找机器：

```bash
GOOS=windows GOARCH=amd64 go build   # → rulemux.exe
GOOS=darwin  GOARCH=arm64  go build   # → macOS
GOOS=linux   GOARCH=amd64  go build   # → Linux
```

### 2.4 结论（一句话）

> **NAS 上"跑" rulemux 不需要 Go；但要做到"改完源码能在 NAS 上当场重新编译出二进制"（而不是每次手动从别处编译上传），就必须在 NAS 上装 Go，并且要持久化。**

---

## 三、要装什么（规格）

| 项 | 要求 |
|---|---|
| **软件** | **Go 工具链（SDK）**，即含 `go` 命令行（编译器 + 链接器 + 构建工具）。**不需要** IDE（VS Code / GoLand）、不需要任何额外运行时 |
| **版本** | **Go 1.22 或更新的稳定版**（建议最新稳定版，如 1.24.x）。低于 1.22 可能有语法不兼容 |
| **平台 / 架构** | **linux / amd64**（已确认 `uname -m` = `x86_64`，Debian 13） |
| **体积** | 安装后约 **300–500 MB** 磁盘（官方 tar 包约 70–100 MB） |
| **系统依赖** | 无（官方 tar 包解压即用，无需 apt 依赖） |
| **安装时网络** | 需要联网下载一次。官方 `https://go.dev/dl/`；国内镜像可选 `https://golang.google.cn/dl/` 或 `https://mirrors.aliyun.com/golang/` |
| **构建时网络** | **本项目可做到零第三方依赖（只用 Go 标准库）**，因此后续 `go build` **不需要再联网下载任何依赖** |

**建议安装方式（官方 tar 包，最干净）**：

```bash
# 以 go1.24.5 为例，实际请替换为最新稳定版
curl -fsSL -o /tmp/go.tgz https://go.dev/dl/go1.24.5.linux-amd64.tar.gz
mkdir -p <持久化目录>
tar -C <持久化目录> -xzf /tmp/go.tgz     # 得到 <持久化目录>/go/bin/go
```

---

## 四、持久化要求（重点）

### 4.1 问题

镜像/容器重启后，容器可写层被丢弃。装在这些位置**都会丢**：

- `apt-get install golang-go`（装进容器系统层）
- 解压到 `/usr/local/go`、`/opt/go` 等容器内部路径（未挂载持久卷的）

### 4.2 必须持久化的 4 样东西

| # | 项 | 建议存放 | 丢了会怎样 |
|---|---|---|---|
| ① | **Go 工具链** | 持久化卷（NAS 真实磁盘目录，如 `/volume1/...`）挂载进容器 | 重启后无法编译 |
| ② | 模块缓存（`GOPATH` / `GOMODCACHE`，可选） | 持久化卷 | 只是编译变慢，可重建 |
| ③ | **编译产物 `rulemux` 二进制** | 持久化卷，并加入 `PATH` | **重启后钩子调不到 rulemux，功能全废（最严重）** |
| ④ | rulemux 配置 `config.toml`（默认 `~/.rulemux/config.toml`） | 持久化卷（**HOME 也要持久**，否则 `~` 在容器里会重置） | 重启后配置丢失 |

### 4.3 推荐实施方案（供评估，按推荐度排序）

**方案 A（推荐）：持久化卷 + 镜像/启动脚本里配环境变量**

1. 在 **NAS 真实磁盘**上建目录，例如：
   - `/volume1/docker/rulemux/tools/go`（放 Go 工具链）
   - `/volume1/docker/rulemux/bin`（放 `rulemux` 二进制）
   - `/volume1/docker/rulemux/gopath`（放 `GOPATH`/模块缓存）
   - `/volume1/docker/rulemux/home`（作为 HOME，放 `~/.rulemux/config.toml`）
2. 下载 Go tar 包并解压到 `tools/go`（得到 `tools/go/go/bin/go`）。
3. 把这些目录**挂载**进容器（例如挂到 `/opt/go`、`/opt/rulemux/bin`、`/opt/gopath`、`/opt/rulemux-home`）。
4. 在 **Dockerfile（ENV）或容器启动脚本 / compose 的 environment** 里设置（**不能只在交互 shell 里 export**）：
   ```bash
   GOROOT=/opt/go
   GOPATH=/opt/gopath
   GOMODCACHE=/opt/gopath/pkg/mod
   HOME=/opt/rulemux-home
   PATH=/opt/rulemux/bin:/opt/go/bin:$PATH
   ```
5. 把编译好的 `rulemux` 二进制放到 `/opt/rulemux/bin/rulemux` 并 `chmod +x`。

**方案 B：bake 进自定义镜像**

在 Dockerfile 里 `FROM <现有镜像>`，加一层安装 Go + 放入 `rulemux` 二进制，重建镜像。镜像层本身持久，重启不丢；代价是更新 Go 或二进制都要重建镜像。

**方案 C：一次性构建容器（不推荐作长期方案）**

临时 `docker run --rm -v 源码:/src -v 产物:/out golang:1.24 ...` 编译，编完销毁容器，只把二进制留在卷上。NAS 上不常驻 Go，但每次编译都要拉镜像/起容器。

### 4.4 验收标准（装完请逐条确认）

1. `go version` 能正常输出（例：`go version go1.24.5 linux/amd64`）。
2. **重启容器后**再执行 `go version` **仍然可用** ← 证明持久化成功（最关键）。
3. 重启后 `which rulemux` 与 `rulemux --help` 仍可用。
4. 在 rulemux 源码目录执行 `go build ./...` 能成功产出二进制。
5. 在**新开的非交互 shell** 里 `echo $GOROOT $PATH` 仍正确（证明不是临时 `export`）。
6. `echo $HOME` 指向持久目录，且 `~/.rulemux/config.toml` 重启后仍在。

---

## 五、备选方案：NAS 上完全不装 Go（请一并评估）

若维护方认为在 NAS 上装 Go 成本过高，可考虑：

- **CI 出包 + NAS 定时拉取**：GitHub Actions 编译出三平台二进制并挂到 Release；NAS 上放一个定时任务 `curl` 下载最新 Release 二进制到持久卷 + `chmod +x`。
- **优点**：NAS 上完全不用装 Go，镜像保持干净。
- **缺点**：每次改代码要先推 GitHub → 等 CI → NAS 再拉（**反馈慢、有延迟**），且依赖 NAS 能访问 GitHub Release。
- **用户倾向**：优先选**在 NAS 上装 Go（构建能力留在 NAS）**，因为不接受每次手动编译上传。但若持久化确实困难，请维护方反馈，我们可退回 CI 拉取方案。

---

## 六、需要 NAS 维护方反馈的信息

1. 当前 NAS 上 CodeBuddy 等 agent 是**怎么跑的**？（Docker 容器？哪个镜像？有没有持久化卷映射？NAS 系统型号，如 DSM / 威联通 / 自建 Docker？）
2. 选定**方案 A / B / C**，或提出更合适的做法。
3. **持久化卷的真实路径**是什么（我需要据此知道把 Go 与 `rulemux` 二进制放哪、PATH 怎么配）。
4. 是否允许在该 NAS 上长期占用 300–500 MB 磁盘装 Go。
5. NAS 能否访问外网（下载 Go 安装包）？若不能，需要提供离线安装方式（可提前下载 tar 包再传入）。

---

## 七、附：rulemux 后续工作（供维护方了解，不阻塞本次）

- 源码语言：Go，模块路径 `github.com/cq-guojia/rulemux`（仓库已存在）。
- CLI 子命令：`sync`（被钩子调用的主命令）/ `init`（装钩子 + 生成示例配置）/ `doctor`（环境自检）/ `verify`（canary 实测验收）。
- 目标 agent 适配器：Claude Code、CodeBuddy、WorkBuddy、Trae（CN 与国际版机制相同，统一为一个适配器）；Codex / OpenCode 为 Tier-2（单文件 agent，走 hook 注入）。
- 设计文档见本仓库 `docs/design/`（`implementation.md` 为实现待定项清单）。
