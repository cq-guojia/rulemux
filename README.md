# rulemux

> One set of rules, **one source of truth**: edit once, and it reaches **every agent in every
> workspace**.
> Same effect and same token cost as writing `AGENTS.md` by hand — **never fades out, never accumulates, no symlinks, free to add and remove files**.

[中文版](README.zh-CN.md) | [Design docs](docs/design/architecture.md)

---

## 1. What is this

### 1.1 The pain

- **Every agent keeps rules somewhere else**: `.codebuddy/rules/`, `.trae/rules/`, a single
  `AGENTS.md`, an injected recall block… one coding convention has to be rewritten once per agent.
- **And every workspace keeps its own copy**: to make a rule apply in a workspace you write it
  there again.
- So the rules that are genuinely **shared** (team conventions, coding standards) can neither be
  factored out nor maintained: one sentence means opening *agents × workspaces* files, with no
  guarantee you did not miss one.

### 1.2 What rulemux does

You maintain **one source of truth**: a set of rule documents you wrote, declared in
`~/.rulemux/config.toml` as the tree below. `rulemux sync` copies them — really copies — into
each agent's **native rules directory**. Upper layers are inherited by the ones below them, so
**one edit reaches every agent in every workspace on the next sync**.

```
One source of truth (your own .md files, declared in ~/.rulemux/config.toml)
│
├─ Layer 1 · org-wide rules ─────────────► every agent × every workspace
│   │
│   ├─ Layer 2 · dev rules
│   │   ├─ Layer 3 · mini-app rules
│   │   │   └─ …
│   │   ├─ Layer 3 · backend rules
│   │   │   └─ …
│   │   └─ Layer 3 · client-side rules
│   │       └─ …
│   │
│   └─ Layer 2 · non-dev rules
│       └─ Layer 3 · workspace daily rules …
│           └─ …
│
└─ Layer 4 · one workspace's own rules ──► only in that workspace

              rulemux sync  ↓  real copies (never fades out / never accumulates / no symlinks)

    workspace A ──► .codebuddy/rules/   .trae/rules/        (dsh: injected, nothing on disk)
    workspace B ──► .codebuddy/rules/   .trae/rules/        (dsh: injected, nothing on disk)
```

- **Rules are reused**: layer 3 inherits layer 2, layer 2 inherits layer 1 — write what is shared
  once, and get more specific as you go down.
- **Layers fan out**: each layer declares which agents and which workspaces it applies to; no
  file is duplicated per combination.
- **Edit once, applies everywhere**: change a sentence up top and every workspace and agent that
  inherits it is aligned on the next sync.
- **No depth limit**: the `…` under layer 3 is a padding layer — `use` is expanded **recursively**,
  so you can nest as deep as you like. The only thing not allowed is a **cycle**
  (A uses B and B uses A), which fails at config load.

The tree is built with `use` on `[[file_group]]` (inherit the layer above) plus `workspace` on
`[[source]]` (where it lands):

```toml
[[file_group]]
name = "base"                    # layer 1: org-wide
path = ["/rules/00-base.md"]

[[file_group]]
name = "dev"                     # layer 2: dev rules
use  = ["base"]                  # inherits layer 1
path = ["/rules/10-dev.md"]

[[file_group]]
name = "miniapp"                 # layer 3: mini-app rules
use  = ["dev"]                   # inherits layer 2 (and therefore layer 1)
path = ["/rules/20-miniapp.md"]

[[source]]
groups    = ["miniapp"]          # lands as base + dev + miniapp
workspace = ["/work/miniapp-a", "/work/miniapp-b"]

[[source]]
path      = ["/rules/proj-x-only.md"]   # layer 4: this workspace only
workspace = ["/work/proj-x"]
```

See §5.2 for the full grouping and matching syntax.

### 1.3 The invariants

- **rulemux does not own or maintain any rule content**: your source files stay yours — rulemux
  only delivers them. Which agents / workspaces they apply to is declared in your config.
- **The session hook is (almost) only the courier.** Rule *content* is never injected: files are
  really copied into the directory the agent loads natively, so they enjoy static-prefix semantics
  (never fade out mid-conversation, never accumulate). The single, deliberate exception is one
  **transient notice line** the hook emits when it detects that the rules really changed — it asks
  you to start a new session. See §5.3.
- **No symlinks, free add/remove**: real copies only; delete a source from the config and its copy
  disappears on the next sync.

> Why not "just inject with a hook"? Hook injection lands in the dynamic part of the context: it
> gets summarised away on compaction (fades out) or re-appended every turn (token blow-up).
> See [design/architecture.md](docs/design/architecture.md) §一.

---

## 2. Supported scope

rulemux ships **one adapter per agent**, and an adapter is only installable once its rules
directory and hook location have been confirmed by a real canary test. Today:

| Agent | Tier | Rules directory | Skills directory | Status |
|---|---|---|---|---|
| **codebuddy** | Tier-1 (real copy) | `.codebuddy/rules/`; user-level `~/.codebuddy/rules/` | `.codebuddy/skills/`; user-level `~/.codebuddy/skills/` + `~/.workbuddy/skills/` (both written) | ✅ **Verified — installable** (`codebuddy-cn` is an alias) |
| **workbuddy** | Tier-1 (real copy) | `.codebuddy/rules/` (**shared** with codebuddy, canary-proven 2026-10-09) | `.codebuddy/skills/` (shared with codebuddy); user-level: both dirs written as above | ✅ **Verified — installable** (separate app: own `~/.workbuddy/settings.json` hook, no longer an alias of codebuddy) |
| claude (Claude Code) | Tier-1 | `.claude/rules/` | not yet | ⚠️ registered, **not verified yet** — cannot be installed |
| **trae** (Trae CN) | Tier-1 (real copy) | `.trae/rules/` | `.trae/skills/` (workspace, 🔴 pending canary); user-level `~/.trae-cn/skills/` (verified) | ✅ **Verified — installable** (2026-10-09 canary: user-level hook `~/.trae-cn/hooks.json`; the international `~/.trae` build is not wired yet) |
| codex | Tier-2 (injection) | none — injects into context | not yet | ⚠️ registered, **not verified yet** |
| opencode | Tier-2 (injection) | none — injects into context | not yet | ⚠️ registered, **not verified yet** |
| **dsh** (DeepSeek Harness) | Tier-2 (injection) | none — injects into context | `.dsh/skills/`; user-level `~/.dsh/skills/` | ✅ **Verified — installable**; rules are delivered by a **separate dsh plugin** — install it with `dsh plugin --profile <p> add rulemux-dsh` (rulemux only renders the text with `inject`, and writes nothing into your workspace for rules). Skills are real copies into the native skills dirs, synced by the same plugin at session start (source-verified 2026-10-14 against `deepseek-ai/deepseek-harness`, verified on a real dsh 2026-10-15) |

- `rulemux init` **refuses** any agent that is not verified — it will not half-install an adapter
  whose behaviour has not been proven. `rulemux doctor` marks unverified agents with ⚠.
- **Skill injection (shipped 2026-10-12, codebuddy / workbuddy; dsh added 2026-10-14)**: declare local
  skill directories with `[[skill_source]]`; they are really copied into the agent's native skills
  directory (as `__rulemux__<skill-name>/` — your own skills are never touched). With `scope = "user"`
  they land in the **user-level (global)** directories. Rules and skills both support the dual scope;
  see §5.4. claude / codex / opencode do not take skills yet; trae takes skills (CN: workspace `.trae/skills` + user-level `~/.trae-cn/skills`, workspace pending canary).
- **Tier-2 is a deliberate downgrade** for agents that have no rules directory (they only read a
  single `AGENTS.md`). It injects via the session hook and never touches your own `AGENTS.md`, but
  it cannot satisfy the "never fades out" bar. See
  [design/features/hook-injection.md](docs/design/features/hook-injection.md).
- **dsh is plugin-delivered: for rules nothing lands on disk, for skills real copies land in
  `.dsh/skills/`**: it has no hook file and no rules directory, so rulemux never writes into your
  workspace for rules. A **separate plugin package** in this repo
  ([`dsh-plugin/`](dsh-plugin/)) — on npm as `rulemux-dsh` — asks the CLI for the rules rendered for
  the session's workspace (`rulemux inject --agent dsh`) and injects that text once, on the first
  user turn. Install it the normal dsh way — `dsh plugin --profile web add rulemux-dsh` (or straight
  from git: `add "github:cq-guojia/rulemux#path:/dsh-plugin"`); `rulemux init --agent dsh` installs
  nothing — it just prints that command.
  **Skills (2026-10-14)**: dsh does have a native skills subsystem (`.dsh/skills` in the workspace,
  `~/.dsh/skills` user-level — both source-verified against `deepseek-ai/deepseek-harness`), so the
  same plugin also runs `rulemux sync --hook --agent dsh` when a session starts and real copies land
  there as `__rulemux__<skill-name>/`. Nothing else is written, and `.agents/skills` (a cross-framework
  convention dir) is never touched.
  The plugin then **sets itself up as dsh starts** (restart dsh after installing): readiness is three
  steps and **all of them must hold** — the `rulemux` CLI is resolved *and* its version satisfies the
  plugin's minimum (an older one is upgraded with pnpm/npm and re-checked — never accepted as-is),
  the plugin itself is loaded, and `~/.rulemux/config.toml` exists (created for you when missing,
  never overwritten). Any step that does not hold **raises**, so the session fails visibly instead of
  quietly running without rules. Because it happens at startup, `~/.rulemux/config.toml` is already
  there when you go to edit it: install → restart → edit the config → open **one** session. Asking for
  the rules is a separate concern: a failing `rulemux inject` is logged, and the session carries on
  without an injected block. Uninstalling rulemux cannot stop the plugin (it belongs to the host), so
  `rulemux uninstall --agent dsh` says so — and points out any leftover `.dsh/rules` files from the
  old design, which rulemux no longer writes but also no longer deletes.

---

## 3. Install

rulemux is a **single Go binary with zero third-party dependencies**.

```bash
# a) from a GitHub Release (recommended): put the binary on your PATH
#    e.g. /usr/local/bin/rulemux   (Windows: rulemux.exe)

# b) from source
go install github.com/cq-guojia/rulemux@latest
# or
git clone https://github.com/cq-guojia/rulemux.git && cd rulemux && go build -o rulemux .

# c) via npm (convenience channel that puts the binary on your PATH)
npm i -g rulemux
```

There is **no auto-update**. Upgrades are manual and handled by your package manager
(`go install …@latest`, Homebrew/Scoop/apt later on). This is a deliberate decision — see
[design/requirements.md](docs/design/requirements.md) §五.

**An upgrade keeps your hooks current — by itself, with no npm install scripts.** A hook command can
change between releases (0.2.0 added `--hook`), so rulemux fixes its own stale hook the next time an
agent runs it: the first `rulemux sync` after an upgrade rewrites rulemux's own SessionStart entry
into the current format. Nothing else is touched — other tools' hooks and every other key stay as they
were, and nothing is ever created. Because this lives in the binary, it works no matter how rulemux was
installed and needs no `postinstall` allow-list from npm.

You can also do it on demand:

```bash
rulemux init --refresh   # rewrite rulemux's OWN hooks only: never installs, never creates files
```

`rulemux doctor` now reports a hook that is installed but out of date (and how to fix it). If you
moved this agent's config directory with `CODEBUDDY_CONFIG_DIR`, rulemux follows it — see
[design/external/agent-rules-dirs.md](docs/design/external/agent-rules-dirs.md) §五.

### 3.1 dsh: a plugin, not a hook

Every other agent is “`rulemux init --agent <id>` installs a session hook”. **dsh is the exception**:
it has no hook file and no rules directory, so `rulemux init --agent dsh` installs nothing and only
prints the command below — a dsh plugin does the delivery, asking rulemux for the text:

```bash
dsh plugin --profile <your profile> add rulemux-dsh        # npm
# or straight from git, to try an unreleased change:
dsh plugin --profile <your profile> add "github:cq-guojia/rulemux#path:/dsh-plugin"
```

**Restart dsh after installing**: the plugin becomes ready when dsh starts — it checks the
`rulemux` CLI (upgrading it and re-checking when it is too old) and creates `~/.rulemux/config.toml`
when it is missing (never overwriting one that exists). Any step that fails raises, so a session
never quietly runs without rules. Because this happens at startup, the config file is already there
when you go to edit it: install → restart → edit the config → open **one** session.

On that session the plugin runs `rulemux inject --agent dsh` and injects the rendered rules —
**nothing is written into your workspace**: no `.dsh/rules`, no copies, nothing left behind to clean
up. (Older versions of rulemux wrote copies into `.dsh/rules` and the plugin read them back; that
directory is now unused — if you have one left over, delete it yourself, or run
`rulemux uninstall --agent dsh` which will point at it.)

---

## 4. Quick start

```bash
# 1. Install the session hook for an agent (--agent is REQUIRED)
rulemux init --agent workbuddy
#    First run also writes a sample config to ~/.rulemux/config.toml

# 2. Edit ~/.rulemux/config.toml and point `path` at your real rule files

rulemux doctor    # 3. Self-check: binary/PATH, agents, config validity
rulemux verify    # 4. Canary check: drop a probe and ask the agent to recite its token

# From now on every new session triggers `rulemux sync --hook --agent codebuddy` automatically.

rulemux sync --all   # optional: re-align EVERY workspace recorded in the ledger, in one shot

rulemux uninstall --agent codebuddy   # remove rulemux again (--yes skips the confirmation)
```

**Where the hook goes:** into the **user-level host config** `~/.codebuddy/settings.json` (the same
place a tool like Hindsight registers itself). **Install once, and it applies to every workspace** —
you do *not* re-run `init` per project. When it fires, rulemux uses the current workspace (its cwd)
to match the `workspace` entries in your config and decides what to deliver.

---

## 5. Configuration

Config file: `~/.rulemux/config.toml` (override with `--config <path>` on any command).

### 5.1 Each `[[source]]`

| Field | Meaning |
|---|---|
| `path` | Source file(s), anywhere on disk. Single value or a list. A list shares the `agents`/`workspace` below. |
| `agents` | Which agents receive this batch. Omitted = all supported agents. |
| `workspace` | Which workspaces these rules apply to. Omitted / `"*"` / `"**"` / `"all"` = every workspace. Single value, a list, or a glob. |
| `scope` | Where the batch lands: `"workspace"` (default) = each workspace's agent dir; `"user"` = the agent's **user-level (global)** dir, shared by every workspace. With `scope = "user"`, `workspace` is ignored. |

Globbing follows the usual rules: `*` is one path segment, `**` spans segments and may appear in
the middle — e.g. `workspace = "/abs/**/B"` matches a directory named `B` at any depth.

```toml
[[source]]
path = ["/your/rules/a.md", "/your/rules/b.md"]
agents = ["codebuddy"]              # omit to target every supported agent
# workspace is omitted => every workspace
# workspace = ["/path/to/proj-a", "/path/to/proj-b"]
# workspace = "/abs/**/B"           # glob: any depth, named B
# scope = "user"                    # land in the agent's user-level (global) dir instead
```

### 5.2 Bundling with "groups" (optional)

When you have many files, bundle them and reference the bundle by name.

```toml
# ---- File groups ---------------------------------------------------------
[[file_group]]
name  = "base"
path  = ["/your/rules/team-conventions.md", "/your/rules/style.md"]

[[file_group]]
name = "proj"
use  = ["base"]                     # nests the base group (a list: ["base", "dev"])
path = ["/your/rules/project-a.md"]

# ---- Workspace groups ----------------------------------------------------
[[workspace_group]]
name      = "dev"
workspace = ["/path/to/proj-1", "/path/to/proj-2"]

# ---- Use them ------------------------------------------------------------
[[source]]
groups           = ["base", "proj"]      # expands to every file in those groups
path             = ["/your/rules/extra.md"]   # mixing is allowed: groups + standalone files
agents           = ["codebuddy"]
workspace_groups = ["dev"]               # expands to every workspace in the group
workspace        = ["/path/to/standalone"]    # also allowed alongside workspace_groups
```

Things worth knowing:

- **`use` is a list** — `use = ["dev", "qa"]` reuses several groups at once, and groups nest
  (A pulls in B plus its own files).
- **There is no depth limit** — `use` is expanded **recursively** (for file groups and workspace
  groups alike), so you can nest as deep as you like. The only thing not allowed is a **cycle**
  (A uses B and B uses A), which fails at config load.
- **Mixing is allowed**: `groups` with `path`, and `workspace_groups` with `workspace`, in the same
  source. The result is the union.
- **Duplicates are harmless** — everything is deduplicated by value, so each file is processed once.
- File groups and workspace groups are **separate namespaces**; the same name may be used in both.

### 5.3 What sync does, and what it never touches

On every sync rulemux computes the set of prefixed files that should exist, then removes any
`__rulemux__*` file that is not in that set, copies/overwrites the ones that are missing or changed,
and skips the rest.

- Destination name: `__rulemux__` + source basename (a short hash is appended on name collisions).
- **Your source files are only ever read** — never modified or deleted.
- **Files without the `__rulemux__` prefix in the rules directory are never touched**, so your own
  rule files stay safe.

> ⚠️ **Run `rulemux sync` after editing your config or source docs**
> After you edit `~/.rulemux/config.toml` (adding/removing a `[[source]]`, changing `path`) or any
> source rule document, run `rulemux sync --agent codebuddy` (a bare `rulemux sync` only touches the
> agents already recorded for this workspace — name `--agent <id>` to add one).
> - **If you run sync**: the next new session reads the updated rules.
> - **If you don't**: CodeBuddy / WorkBuddy still auto-sync on every session start via the SessionStart
>   hook, but the harness fixes the rule snapshot at the **start of the session** (before the hook runs),
>   so the current session still sees the old version — the update only takes effect on the **second**
>   new session.

### 5.4 Skill injection (`[[skill_source]]`, shipped 2026-10-12)

A skill is a **directory** (`SKILL.md` plus optional `references/` / `scripts/`), loaded
progressively (name+description at session start, full text on demand) — so, exactly like rules,
it can only be delivered by a **real copy** into the native skills directory, never by hook
injection. Add `[[skill_source]]` to `~/.rulemux/config.toml`:

```toml
[[skill_source]]
path = ["~/my-skills"]              # skill root: every child dir containing SKILL.md is one skill
agents = ["codebuddy"]              # omitted = every supported agent
workspace = "*"                     # omitted = every workspace (workspace scope only)
# groups = ["frontend"]             # reuse a [[skill_group]] bundle instead of repeating path
# workspace_groups = ["dev"]        # reference a [[workspace_group]] bundle (shared with rules)

[[skill_source]]
groups = ["frontend"]               # bundle several skill roots under one name
agents = ["codebuddy"]
scope = "user"                      # user-level (global): workspace-independent, every workspace

[[skill_group]]                     # bundle skill roots so several sources can reuse them
name = "frontend"
path = ["~/react-skills", "~/css-skills"]
# use = ["base"]                    # nest another skill group (list, may be multiple)
```

- **Skill sources take the same fields as rules**: `path`, `agents`, `workspace`, `scope`, plus the
  bundling keys `groups` (→ `[[skill_group]]`) and `workspace_groups` (→ `[[workspace_group]]`).
  `[[skill_group]]` bundles several skill root directories (`name` / `path` / `use`); nesting and
  cycle detection work exactly like file groups.
- **Where they land**: workspace scope → the agent's native skills dir — `<workspace>/.codebuddy/skills/`
  (shared by codebuddy and workbuddy) or `<git根>/.dsh/skills/` (dsh) or `<workspace>/.trae/skills/` (trae, CN,
  🔴 pending canary); user scope → `~/.codebuddy/skills/`
  **and** `~/.workbuddy/skills/` (both written, so both apps see it) or `~/.dsh/skills/` (dsh) or `~/.trae-cn/skills/` (trae, CN).
  Rules work the same way: `[[source]]` also accepts `scope = "user"`, landing in
  `~/.codebuddy/rules/` + `~/.workbuddy/rules/`.
- **Same discipline as rules**: the target is `__rulemux__<skill-name>/`; your own skills
  (no prefix) are never touched; removing a source removes its copies on the next sync; sources
  are only ever read.
- **Remote / market sources** (`path` takes more than a local dir): `gh:owner/repo` or a full git URL
  (`git@host:owner/repo.git` = SSH, `https://…` = HTTPS) is `git clone`d into
  `~/.rulemux/skills/<owner>-<repo>/` on the first sync; `npm:<package>` is `npm pack`ed and unpacked
  there. After the first fetch it is treated as a local dir (zero network on later syncs). Transport
  (SSH vs HTTPS) is whatever the URL you wrote implies — rulemux never picks for you. Skills under the
  source root are **auto-discovered** (bounded scan for `SKILL.md`, like `npx skills --all`), so a repo
  that groups many skills (e.g. `gh:anthropics/skills`) works with no extra config; append `#subpath`
  (e.g. `gh:anthropics/skills#skills/docx`) to point at one container or a single skill. Inspect with
  `rulemux skill list`; upgrade with `rulemux skill update <key>` / `--all` (git pull / npm re-pack, in
  place — manual only, never in the hook). Top-level `fetch_timeout = "30s"` caps how long a first-time
  fetch may run before it warns and skips on the hook path.
- claude / codex / opencode do not take skills yet; trae takes skills (CN).
- Skill add/change does **not** trigger the "start a new session" notice (hot-reload is still
  being verified); rule behaviour is unchanged.
- **dsh skill frontmatter quirks** (2026-10-14, source-verified): dsh requires the frontmatter
  `name` to match `^[a-z0-9]+(?:-[a-z0-9]+)*$` and rejects the legacy camelCase invocation keys
  (`disableModelInvocation` / `modelInvocable` / `userInvocable`) by dropping the whole skill —
  CodeBuddy accepts both. rulemux copies skills byte-for-byte and never rewrites content, so a
  skill that loads in CodeBuddy may silently not load in dsh. CodeBuddy marketplace extras
  (`display_name` / `description_zh` / `version` / `allowed-tools`) are simply ignored by dsh.
  dsh also finds project skills relative to the **git root** (nearest `.git`), not your cwd.

### 5.5 The complete default config (annotated)

`rulemux init` writes this file to `~/.rulemux/config.toml` on first run. **Everything is commented
out** — nothing is synced until you uncomment an example or add your own with real paths. It is
reproduced here one-to-one so you can read the whole thing before editing your copy.

```toml
# rulemux configuration file
#
# Default location: ~/.rulemux/config.toml (or pass --config <path> to any command)
#
# At the start of every agent session, rulemux copies the "source rule files" described
# by each [[source]] below into that agent's native rules directory, using a __rulemux__
# prefix: it cleans up its own residue and never touches your own files.
#
# Each [[source]] accepts these fields (all optional, each has a default):
#
#   path      Source file(s); may live anywhere on disk. Both forms work:
#               path = "/abs/path/to/your-rules.md"          # a single file
#               path = ["/abs/path/a.md", "/abs/path/b.md"]  # a list (shares agents/workspace below)
#             A leading "~" or "~/" is expanded to your home directory, so the same
#             config works on every machine without hardcoding a username:
#               path = "~/Agent.Workspace/00.RULES/base.md"
#
#   agents    Which agents receive this batch; omitted = every *supported* agent.
#             Current verified values (installable):
#               workbuddy   Tier-1 (own dir .workbuddy/rules; own hook ~/.workbuddy/settings.json) — use this for WorkBuddy
#               codebuddy   Tier-1 (.codebuddy/rules; ~/.codebuddy/settings.json)
#               trae        Tier-1 (.trae/rules; CN ~/.trae-cn/hooks.json, intl ~/.trae) — verified 2026-10-09
#             These are SEPARATE agents, each with its own config dir and hook file;
#             install whichever you actually use (or all). rulemux remembers the name
#             you chose and keeps it on every later refresh.
#             Other agents (claude / codex / opencode) have not passed canary
#             verification yet, so they cannot be installed or targeted for now;
#             once their adapters are proven they open up automatically — no edit needed here.
#             Example: agents = ["workbuddy", "trae"]
#
#   workspace Which workspaces these rules apply to. Forms:
#               workspace = "/abs/path/to/proj"          # single workspace (exact match)
#               workspace = ["/abs/path/proj-a", "/b"]   # list: matching any one is enough
#               workspace = "*"  /  "**"  /  "all"       # every workspace (global wildcard)
#             Standard globbing is supported: "*" is one segment, "**" recurses across
#             segments and may appear in the middle, e.g.:
#               workspace = "/abs/**/B"                 # any depth under /abs named B
#               workspace = "**/B"                      # B at any location or nesting level
#             A leading "~" / "~/" is expanded to your home directory here as well,
#             and still works with globs:
#               workspace = "~/Agent.Workspace/*"
#
#   scope     Where this batch lands (optional, default "workspace"):
#               scope = "workspace"  # each workspace's agent dir (e.g. <ws>/.codebuddy/rules)
#               scope = "user"       # the agent's USER-LEVEL (global) dir, shared by every
#                                   # workspace — e.g. ~/.codebuddy/rules and ~/.workbuddy/rules
#                                   # for the CodeBuddy family (both are written so both apps
#                                   # see it). Ignored for workspace matching: a global rule
#                                   # is global by definition.
#
# --- Skills ([[skill_source]]) ---------------------------------------------------
# Besides rule files, rulemux can sync SKILL directories into the agents' native
# skills directories. A skill is a DIRECTORY containing SKILL.md (plus optional
# references/scripts); it is loaded progressively (name+description at session
# start, full text on demand), so it can only be delivered by a real copy into
# the native skills dir — never by hook injection.
#
#   [[skill_source]] fields:
#     path              Skill root directory(ies) — every child directory that contains a
#                       SKILL.md is one skill (a path that itself contains SKILL.md is
#                       treated as a single skill). A local dir, gh:owner/repo or a full git
#                       URL (cloned to ~/.rulemux/skills/ on first sync), or npm:<package>.
#     agents            Same as [[source]]: omitted = every supported agent.
#     workspace         Same as [[source]] (workspace scope only; ignored when scope="user").
#     groups            Reference [[skill_group]] bundles (same idea as [[source]].groups):
#                       each group bundles several skill root directories.
#     workspace_groups  Reference [[workspace_group]] bundles (workspaces are shared with rules).
#     scope             Same as [[source]]: "workspace" (default) or "user".
#
#   Where they land:
#     workspace scope → <ws>/.codebuddy/skills   (shared by codebuddy + workbuddy)
#                      or <git-root>/.dsh/skills  (dsh; the git root, not your cwd)
#                      or <ws>/.trae/skills       (trae, CN; 🔴 workspace scope not yet canary-verified)
#     user scope      → ~/.codebuddy/skills AND ~/.workbuddy/skills (both written)
#                      or ~/.dsh/skills           (dsh)
#                      or ~/.trae-cn/skills       (trae, CN user-level; verified present)
#   Skills land as __rulemux__<skill-name>/ directories; your own skills are never
#   touched, and removing a source removes its copies on the next sync.
#   dsh never reads .agents/skills from rulemux — that dir is a cross-framework
#   convention and stays untouched.
#   Other agents (claude / codex / opencode) do not take skills yet; trae takes skills (CN).
#
# --- Advanced: bundle things into "groups" to avoid repetition -------------------
# Once you have many files, bundle your common files / workspaces into groups and
# reference them by name from [[source]].
#
# File group [[file_group]]: bundles rule files, referenced via groups.
#   name  Group name (required)
#   path  Files inside the group; single value or list
#   use   Reference other file groups -- [LIST, MAY BE MULTIPLE, NESTS]:
#           use = ["dev"]            # one group
#           use = ["dev", "qa"]      # several: reuse all of them at once
#           # group A pulls in group B plus its own files => A == B's files + A's files
#
# Workspace group [[workspace_group]]: bundles workspaces, referenced via workspace_groups.
#   name       Group name (required)
#   workspace  Workspaces inside the group; single value or list, globs allowed
#   use        Reference other workspace groups -- [LIST, MAY BE MULTIPLE, NESTS]:
#           use = ["dev"]            # one group
#           use = ["dev", "qa"]      # several: reuse all of them at once
#
# Skill group [[skill_group]]: bundles skill roots, referenced via groups on [[skill_source]].
#   name  Group name (required)
#   path  Skill root directories inside the group; single value or list
#   use   Reference other skill groups -- [LIST, MAY BE MULTIPLE, NESTS]
#
# Inside a [[source]]:
#   groups            = ["gA", "gB"]     # file groups (list, may be multiple; expands to their files)
#   workspace_groups  = ["gX", "gY"]     # workspace groups (list, may be multiple)
#   [MIXING IS ALLOWED] groups can be combined with path, and workspace_groups with workspace:
#       groups = ["base", "proj"]            # use every file in the base and proj groups
#       path   = ["/abs/path/extra.md"]      # ...and additionally this one file
#     => this source ships files of base + files of proj + extra.md, deduplicated by value.
#   (workspace_groups / workspace work the same way: reference a group and also list a
#    standalone workspace at the same time.)
#   Duplicates across groups are harmless -- everything is deduplicated by value at the end,
#   so each file is processed exactly once.
#   File groups and workspace groups are separate namespaces; identical names are allowed.
#
# Everything below is commented out and inactive. Uncomment what you need and replace
# the placeholders with your real values.

# [[file_group]]
# name = "base"
# path = ["/abs/path/to/team-conventions.md", "/abs/path/to/style.md"]

# [[file_group]]
# name = "proj"
# use = ["base"]                       # nests the base group (list, may be multiple: use = ["base", "dev"])
# path = ["/abs/path/to/project-a.md"]

# [[workspace_group]]
# name = "dev"
# workspace = ["/abs/path/to/proj-1", "/abs/path/to/proj-2"]

# [[workspace_group]]
# name = "qa"
# use = ["dev"]                        # reuses the dev group (list, may be multiple)
# workspace = ["/abs/path/to/proj-3"]

# [[source]]
# groups = ["base", "proj"]            # file groups (list, may be multiple; expands to their files)
# path = ["/abs/path/to/extra.md"]     # plus a standalone file: this source = base + proj + this file
# agents = ["codebuddy"]
# workspace_groups = ["dev"]          # workspace groups (list, may be multiple; expands to their workspaces)
# workspace = ["/abs/path/to/standalone"]   # plus a standalone workspace

# --- skill examples (same fields, but the source is a DIRECTORY of skills) ----------

# [[skill_source]]
# path = ["/abs/path/to/my-skills"]    # each child dir containing SKILL.md is one skill
# agents = ["codebuddy", "workbuddy"]  # omitted = every supported agent
# workspace = "*"                      # every workspace (default when omitted)

# [[skill_source]]
# path = ["/abs/path/to/global-skills"]
# scope = "user"                       # user-level (global): lands in ~/.codebuddy/skills
#                                       # and ~/.workbuddy/skills, in every workspace

# [[skill_source]]                      # remote source: fetched once on first sync
# path = ["gh:anthropics/skills"]       # GitHub shorthand (HTTPS); or a full git URL
#                                       # (git@host:owner/repo.git = SSH) / npm:<package>
#                                       # skills under the root are auto-discovered (no config);
#                                       # append #subpath to pick one, e.g. gh:o/r#skills/docx
# agents = ["codebuddy", "trae"]

# fetch_timeout = "30s"                 # top-level: max time a first-time remote fetch may run
#                                       # before it warns and skips (hook path)

# [[skill_group]]                      # bundle skill roots so several sources can reuse them
# name = "frontend"
# path = ["/abs/path/to/react-skills", "/abs/path/to/css-skills"]
# # use = ["base"]                     # nest another skill group (list, may be multiple)

# [[skill_source]]
# groups = ["frontend"]                # reuse the bundle instead of repeating its paths
# agents = ["codebuddy"]
# scope = "user"
```

---

## 6. Uninstalling

```bash
rulemux uninstall --agent codebuddy          # one agent (comma-separated for several)
rulemux uninstall --off                      # every agent (--all is the same)
rulemux uninstall --agent codebuddy --yes    # skip the confirmation prompt
```

It removes rulemux's own hook entry (other tools' hooks in the same file are preserved) and deletes
the `__rulemux__*` files it previously delivered. Because the hook lives in the user-level config,
rulemux also **revisits every workspace recorded in its ledger** (`~/.rulemux/workspaces.json`) and
cleans those too — otherwise, with the hook gone, that residue could never be removed again. You are
shown the list of workspaces before anything is deleted.

Once a rules/skills dir is empty it is removed as well (a symlinked, non-empty or subdir-bearing dir
is left alone, and the dir above it is never touched; user-level dirs are kept). The agent is then
**dropped from the ledger** for every workspace that was actually cleaned, and recorded as
uninstalled — a later `rulemux sync` never writes to it again, and it stays out of `doctor` and of the
sync summary until *you* install it again with `rulemux init --agent <id>`. For a plugin host (dsh)
rulemux can neither remove the host-side plugin nor stop it: the plugin keeps calling
`rulemux inject --agent dsh`, so it keeps injecting rules into your sessions until you disable it.
Uninstall says exactly that, and points at the documented switch.

---

## 7. Commands

| Command | What it does |
|---|---|
| `rulemux sync [--hook] [--agent <id>] [--config <path>] [--workspace <dir>]` | Sync rules. `--hook` = this run comes from an agent's SessionStart hook (only then may it create the rules dir and emit the change notice). Without `--agent` it only touches the agents already recorded for this workspace, or named explicitly in the config — an agent that was never used here, or one you uninstalled, is never written to. (A legacy ledger entry recorded as "all / unknown" is honoured only for agents whose hook is installed, i.e. that demonstrably read this workspace.) |
| `rulemux sync --all [--config <path>]` | Manually re-align every workspace recorded in the ledger (never reads cwd, never creates dirs, prunes ledger entries whose directory is gone). Mutually exclusive with `--hook` / `--agent` / `--workspace` |
| `rulemux inject --agent <id> [--config <path>]` | Tier-2: print rules to stdout for hook injection |
| `rulemux init --agent <id[,id...]> [--config <path>] [--workspace <dir>]` | Write a sample config and install the SessionStart hook |
| `rulemux init --refresh [--agent <id[,id...]>] [--workspace <dir>]` | Refresh **only** the hooks rulemux already installed. This also happens by itself on the first sync after an upgrade; use this to force it. Never adds a hook, never creates a file or directory, preserves every other entry and key in the host config |
| `rulemux doctor [--config <path>] [--workspace <dir>]` | Environment self-check: how/where rulemux is installed, config file, which agents are initialized (TTY-aware colours) |
| `rulemux verify [--agent <id>] [--clean] [--config <path>] [--workspace <dir>]` | Canary acceptance test. Without `--agent` it only probes agents already in use here; `--agent <id>` forces one (onboarding). `--clean` removes every canary in this workspace |
| `rulemux uninstall --agent <id[,id...]> \| --off \| --all [--yes]` | Remove hooks and delivered files |

Run `rulemux` with no arguments for the full help.

---

## 8. Scope

**The agent set is frozen.** rulemux supports exactly four agents, all confirmed by a real canary
run: **codebuddy**, **workbuddy**, **trae** (Tier-1 — real copies) and **dsh** (Tier-2 rules —
injection, delivered by its own plugin; Tier-1-style skills — real copies into `.dsh/skills`,
synced by the same plugin). `claude`, `codex` and `opencode`
stay registered but unverified, which is why `rulemux init` refuses them; there is no plan to finish
them. (The checklist for adding an agent is kept for reference:
[design/features/agent-onboarding.md](docs/design/features/agent-onboarding.md).)

- **Distribution** — done: GitHub Actions cross-compiles the binaries and attaches them to a GitHub
  Release; the npm package is published **by hand** (see §9).
- **Skill injection scope** (2026-10-12 codebuddy / workbuddy; 2026-10-14 dsh): codebuddy /
  workbuddy directories (workspace `.codebuddy/skills`, user-level `~/.codebuddy/skills` +
  `~/.workbuddy/skills`) are confirmed against the shipped bundle plus live inspection; dsh
  directories (`<git-root>/.dsh/skills`, `~/.dsh/skills`) are confirmed source-level against
  `deepseek-ai/deepseek-harness`   **and verified on a real dsh 2026-10-15** (user acceptance:
  prefixed skills land and load). trae's CN user-level skills dir (`~/.trae-cn/skills`) is verified present
  (2026-10-15, real skill observed loaded); trae's workspace `.trae/skills` is wired but pending canary
  acceptance; claude / codex / opencode skills directories are not verified yet, so they **do not take skills**
  — each opens up once proven.
- Rule content transformation, templating, and automatic self-update are **explicitly out of scope**
  ([design/requirements.md](docs/design/requirements.md) §五).
- Remote / git skill sources (tracking upstream updates) and a `rulemux skill` subcommand —
  **later versions**; this one ships local sources only.

---

## 9. Releasing (maintainers)

```bash
npm version patch|minor|major   # bumps package.json and creates the git tag v0.x.y
git push --follow-tags          # CI: build, attach binaries to a GitHub Release, and publish to npm
```

Pushing the tag triggers [`.github/workflows/publish.yml`](.github/workflows/publish.yml):
it cross-compiles all platform binaries, attaches them to a GitHub Release, and publishes the
`rulemux` npm package — all via GitHub OIDC trusted publishing, **no `NPM_TOKEN` needed** (the
workflow gets a short-lived OIDC token and signs a SLSA provenance attestation).

The dsh plugin is a separate package (`rulemux-dsh`) published by
[`.github/workflows/dsh-publish.yml`](.github/workflows/dsh-publish.yml) on its own `dsh-v*`
tag:

```bash
# in dsh-plugin/package.json, bump version (e.g. 0.3.6)
git commit -am "chore(rulemux-dsh): 0.3.6"
git tag dsh-v0.3.6
git push --follow-tags          # CI: publish rulemux-dsh via OIDC
```

A plain CI run ([`ci.yml`](.github/workflows/ci.yml)) runs build/vet/tests/smoke on every push.

---

## 10. License

MIT — see [LICENSE](LICENSE).

---

## 11. Documentation

| Want to read | Where |
|---|---|
| What the user actually wants (requirements / acceptance) | [docs/design/requirements.md](docs/design/requirements.md) |
| Why it is designed this way | [docs/design/architecture.md](docs/design/architecture.md) |
| Feature list | [docs/design/features.md](docs/design/features.md) |
| Checklist for adding a new agent | [docs/design/features/agent-onboarding.md](docs/design/features/agent-onboarding.md) |
| Each agent's rules directory (external facts) | [docs/design/external/agent-rules-dirs.md](docs/design/external/agent-rules-dirs.md) |
| Current progress and open items | [docs/progress.json](docs/progress.json) |
