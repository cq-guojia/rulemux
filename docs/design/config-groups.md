# 配置组与分组（设计文档 · 待确认）

> 状态：**已确认并实现**（核心逻辑见 `internal/config/config.go` 的 `resolveGroups`；`exampleConfig` 注释已同步）。

## 1. 背景与目标

当前每条 `[[source]]` 只能列举具体文件路径，且只能针对具体工作区路径。用户诉求：

1. **文件组**：把常用的若干文件打包成一个"组"（如 `base` 组含 3~4 个文件），在 `[[source]]` 里引用组名即可整组带入。
2. **混合引用**：`[[source]]` 里既能引用组，也能混列单个文件（如"base 组 + 额外一个 B 文件"）。
3. **组嵌套**：组与组之间可互相引用（`proj` 组 = 引入 `base` 组 + 再加若干文件）。
4. **工作区分组**：同样的概念用于工作区（如 `dev` 组含 1/2/3 号工作区，`qa` 组含 3/4/5 号），`[[source]]` 用组名引用一批工作区。
5. **重复无害**：A 组有 `a.md`、B 组也引入 A 组，最终 `a.md` 重复出现没关系——程序对最终文件列表按源路径去重，每个文件只做一次。

> 🧠 **From Hindsight memory (Component map)** — rulemux 是单二进制 Go 程序，**零第三方依赖**（手写 TOML 子集解析器，NAS 离线可 `go build`）。新增配置表必须沿用此约束，不能引入 `toml` 解析库。
> 🧠 **From Hindsight memory (Core concepts)** — 产品不变量之一"**不累积**"：最终按源路径去重，组间重复文件无害，正好契合既有 `engine.BuildPlan` 的去重逻辑。

## 2. 设计原则

- **零依赖、手写解析**：继续扩展现有手写 TOML 子集解析器，不引入外部库。
- **向后兼容**：现有 `path` / `agents` / `workspace` 全部语义不变；"组"是纯增量能力。
- **不累积**：展开组后按源路径去重；组间重复文件不会重复投递。
- **engine 不感知组**：组在配置加载阶段就"展开"成具体 `Paths` / `Workspaces`，下游 `engine` / `agents` / `hooks` 零改动。

## 3. 配置语法（推荐方案）

### 3.1 文件组 `[[file_group]]`

```toml
[[file_group]]
name = "base"
path = ["/rules/a.md", "/rules/b.md"]

[[file_group]]
name = "proj"
use = ["base"]            # 嵌套引用 base 组（可列多个：use = ["base", "other"]）
path = ["/rules/c.md"]    # 在引用基础上再追加自己的文件
```

### 3.2 工作区分组 `[[workspace_group]]`

```toml
[[workspace_group]]
name = "dev"
workspace = ["/proj/1", "/proj/2"]

[[workspace_group]]
name = "qa"
use = ["dev"]             # 复用 dev 组
workspace = ["/proj/3"]
```

### 3.3 `[[source]]` 引用（混合）

```toml
[[source]]
groups = ["base", "proj"]   # 引用文件组 → 递归展开为组内所有文件
path = ["/extra.md"]        # 仍可混列单个文件
agents = ["codebuddy"]
workspace_groups = ["dev"]  # 引用工作区分组 → 展开为组内所有路径
workspace = ["/standalone"] # 也可混具体路径
```

- 解析时：`groups` 递归展开写入 `Paths`；`workspace_groups` 递归展开写入 `Workspaces`；原有的 `path` / `workspace` 字段保持原值，与组展开结果**合并**。
- `groups` / `path` 可只用一个或并存；`workspace_groups` / `workspace` 同理。
- 嵌套深度不限，但必须无环。
- **路径支持 `~` 展开**：`path` / `workspace`（含 `file_group` / `workspace_group` 组内）写 `~` 或 `~/` 开头会展开为 HOME，因此同一份配置可跨机器、跨用户复用，不必硬编码 `/Users/xxx` 这类前缀；含 glob 的值（如 `~/proj/*`）同样先展开再匹配，glob 语义不变。展开在**组引用展开之前**完成（`Load` → `expandHomes` → `resolveGroups`），所以 `"/a.md"` 与 `"~/a.md"` 不会被当成两个文件而重复投递。实现真源是 `agents.ExpandHome`，配置侧不另做一份。

## 4. 解析与展开语义

- **递归展开**：`file_group.use` 与 `workspace_group.use` 递归收集其指向的 `path` / `workspace`。
- **环检测**：展开过程维护 `visited` 集合，发现循环引用（A→B→A）立即报配置错误，明确提示环路。
- **去重**：文件路径在展开阶段即按源路径去重（A 组与 B 组都含 `a.md` → 只保留一份）；最终每条 `source` 的 `Paths` / `Workspaces` 是"组展开 + 显式 path/workspace"的并集去重结果。
- **下游无感**：展开后 `Source.Paths` / `Source.Workspaces` 已是扁平的具体值，既有 `SourcesFor` / `MatchesWorkspace` / `BuildPlan` 全部照常工作。

## 5. 校验（`Validate`）增强

- 引用了未定义的组名 → 报错并指明缺失的组名。
- 循环引用 → 报错并指明环路。
- 一条 `[[source]]` 既无 `path` 也无 `groups` → 沿用现有"缺少文件"语义报错。

## 6. 实现要点（落代码阶段）

- `internal/config/config.go`：
  - 新增 `FileGroup{Name, Paths, Uses}`、`WorkspaceGroup{Name, Workspaces, Uses}` 结构；`Config` 增加 `FileGroups` / `WorkspaceGroups` 字段（用 `map[string]*X` 便于按名查找）。
  - `Source` 增加 `Groups []string`、`WorkspaceGroups []string` 字段（仅供解析器填充）。
  - `Load`：手写解析器扩展识别 `[[file_group]]` / `[[workspace_group]]` 两类新表及其 `name` / `path` / `workspace` / `use` 键，以及 `source` 的 `groups` / `workspace_groups` 键。
  - 新增 `resolveGroups()`：在 `Load` 末尾调用，完成递归展开、环检测、去重，并把结果写回各 `Source.Paths` / `Source.Workspaces`。
  - `Validate` 增强（见第 5 节）。
- `internal/cmd/init.go`：
  - `exampleConfig` 顶部注释补充"组"的写法（亦满足"配置注释写清楚"的要求）。
- 测试：
  - `config_test.go`：组展开、嵌套引用、混合引用、组间重复去重、`use` 环检测报错、引用未定义组报错。

## 7. 配置文件注释模板（落代码时写入 `exampleConfig`）

```toml
# # 文件组：把常用文件打包，供 [[source]] 用 groups 引用
# [[file_group]]
# name = "base"
# path = ["/abs/path/a.md", "/abs/path/b.md"]
#
# [[file_group]]
# name = "proj"
# use = ["base"]              # 嵌套引用 base 组
# path = ["/abs/path/c.md"]
#
# # 工作区分组：把一批工作区打包，供 [[source]] 用 workspace_groups 引用
# [[workspace_group]]
# name = "dev"
# workspace = ["/abs/path/proj-1", "/abs/path/proj-2"]
#
# # 引用组时，可同时混列单个文件 / 单个工作区
# [[source]]
# groups = ["base", "proj"]  # 引用文件组（会被展开成组内所有文件）
# path = ["/abs/path/extra.md"]
# agents = ["codebuddy"]
# workspace_groups = ["dev"] # 引用工作区分组（展开成组内所有路径）
# workspace = ["/abs/path/standalone"]
```

## 8. 已确认决策（用户拍板，已实现）

1. **字段命名**：采用 `groups` / `workspace_groups`（语义清晰、解析简单），不复用 `use`、也不让 `workspace` 兼当组名。
2. **工作区分组支持 glob**：组内的 `workspace` 允许 `**` 等通配，复用 `MatchesWorkspace` 的 glob 语义，原样保留不展开。
3. **混合引用**：`[[source]]` 允许 `groups` + `path`、`workspace_groups` + `workspace` 并存。
4. **独立命名空间**：文件组（`[[file_group]]`）与工作区分组（`[[workspace_group]]`）是两套独立表、各自命名空间，允许同名。
