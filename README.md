# rulemux

> One set of rules, delivered to every AI coding agent.
> Same effect and same token cost as writing `AGENTS.md` by hand — **never fades out, never accumulates, no symlinks, free to add and remove files**.

[中文版](README.zh-CN.md) | [Design docs](docs/design/architecture.md)

---

## 1. What is this

You maintain a set of rule documents (coding conventions, project rules, …) and want every AI
coding agent to read them in every workspace — with the exact same effect as if you had written
`AGENTS.md` yourself.

`rulemux` takes the source files you list in a config and copies them into each agent's **native
workspace rules directory**. You manage them in one place; they take effect everywhere.

- rulemux **does not own or maintain any rule content**. Your source files stay yours — rulemux
  only copies them. Which agents / workspaces they apply to is declared in your config.
- **The session hook is only the courier.** It is not used to inject text. Files are really copied
  into the directory the agent loads natively, so they enjoy static-prefix semantics: they never
  fade out mid-conversation, and they never accumulate.
- Invariants: **no symlinks** (real copies only) and **free add/remove** (delete a source from the
  config and its copy disappears on the next sync).

> Why not "just inject with a hook"? Hook injection lands in the dynamic part of the context: it
> gets summarised away on compaction (fades out) or re-appended every turn (token blow-up).
> See [design/architecture.md](docs/design/architecture.md) §一.

---

## 2. Supported scope

rulemux ships **one adapter per agent**, and an adapter is only installable once its rules
directory and hook location have been confirmed by a real canary test. Today:

| Agent | Tier | Rules directory | Status |
|---|---|---|---|
| **codebuddy** | Tier-1 (real copy) | `.codebuddy/rules/` | ✅ **Verified — installable** |
| **workbuddy** | Tier-1 (real copy) | `.codebuddy/rules/` (shared with CodeBuddy) | ✅ **Verified — installable** |
| claude (Claude Code) | Tier-1 | `.claude/rules/` | ⚠️ registered, **not verified yet** — cannot be installed |
| trae (Trae) | Tier-1 | `.trae/rules/` | ⚠️ registered, **not verified yet** — cannot be installed |
| codex | Tier-2 (injection) | none — injects into context | ⚠️ registered, **not verified yet** |
| opencode | Tier-2 (injection) | none — injects into context | ⚠️ registered, **not verified yet** |

- `rulemux init` **refuses** any agent that is not verified — it will not half-install an adapter
  whose behaviour has not been proven. `rulemux doctor` marks unverified agents with ⚠.
- **Tier-2 is a deliberate downgrade** for agents that have no rules directory (they only read a
  single `AGENTS.md`). It injects via the session hook and never touches your own `AGENTS.md`, but
  it cannot satisfy the "never fades out" bar. See
  [design/features/hook-injection.md](docs/design/features/hook-injection.md).

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

---

## 4. Quick start

```bash
# 1. Install the session hook for an agent (--agent is REQUIRED)
rulemux init --agent codebuddy
#    First run also writes a sample config to ~/.rulemux/config.toml

# 2. Edit ~/.rulemux/config.toml and point `path` at your real rule files

rulemux doctor    # 3. Self-check: binary/PATH, agents, config validity
rulemux verify    # 4. Canary check: drop a probe and ask the agent to recite its token

# From now on every new session triggers `rulemux sync --agent codebuddy` automatically.

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

Globbing follows the usual rules: `*` is one path segment, `**` spans segments and may appear in
the middle — e.g. `workspace = "/abs/**/B"` matches a directory named `B` at any depth.

```toml
[[source]]
path = ["/your/rules/a.md", "/your/rules/b.md"]
agents = ["codebuddy"]              # omit to target every supported agent
# workspace is omitted => every workspace
# workspace = ["/path/to/proj-a", "/path/to/proj-b"]
# workspace = "/abs/**/B"           # glob: any depth, named B
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
- **Mixing is allowed**: `groups` with `path`, and `workspace_groups` with `workspace`, in the same
  source. The result is the union.
- **Duplicates are harmless** — everything is deduplicated by value, so each file is processed once.
- File groups and workspace groups are **separate namespaces**; the same name may be used in both.

### 5.3 What sync does, and what it never touches

On every sync rulemux computes the set of prefixed files that should exist, then removes any
`.rulemux__*` file that is not in that set, copies/overwrites the ones that are missing or changed,
and skips the rest.

- Destination name: `.rulemux__` + source basename (a short hash is appended on name collisions).
- **Your source files are only ever read** — never modified or deleted.
- **Files without the `.rulemux__` prefix in the rules directory are never touched**, so your own
  rule files stay safe.

---

## 6. Uninstalling

```bash
rulemux uninstall --agent codebuddy          # one agent (comma-separated for several)
rulemux uninstall --off                      # every agent (--all is the same)
rulemux uninstall --agent codebuddy --yes    # skip the confirmation prompt
```

It removes rulemux's own hook entry (other tools' hooks in the same file are preserved) and deletes
the `.rulemux__*` files it previously delivered. Because the hook lives in the user-level config,
rulemux also **revisits every workspace recorded in its ledger** (`~/.rulemux/workspaces.json`) and
cleans those too — otherwise, with the hook gone, that residue could never be removed again. You are
shown the list of workspaces before anything is deleted.

---

## 7. Commands

| Command | What it does |
|---|---|
| `rulemux sync [--agent <id>] [--config <path>] [--workspace <dir>]` | Sync rules (called by each agent's SessionStart hook) |
| `rulemux inject --agent <id> [--config <path>]` | Tier-2: print rules to stdout for hook injection |
| `rulemux init --agent <id[,id...]> [--config <path>] [--workspace <dir>]` | Write a sample config and install the SessionStart hook |
| `rulemux doctor [--config <path>] [--workspace <dir>]` | Environment self-check |
| `rulemux verify --agent <id> [--clean] [--workspace <dir>]` | Canary acceptance test |
| `rulemux uninstall --agent <id[,id...]> \| --off \| --all [--yes]` | Remove hooks and delivered files |

Run `rulemux` with no arguments for the full help.

---

## 8. Roadmap

Ongoing work and what comes next are tracked in [docs/PROGRESS.md](docs/PROGRESS.md). In short:

- **More agents** — finish the adapters that are already registered but unverified:
  - **Trae** (`.trae/rules/`) and **Claude Code** (`.claude/rules/`) as Tier-1;
  - **Codex** and **OpenCode** as Tier-2 injection.
  Each one needs its rules directory and hook location confirmed by a real canary run before it is
  switched on. The checklist for adding an agent lives in
  [design/features/agent-onboarding.md](docs/design/features/agent-onboarding.md).
- **Distribution** — GitHub Actions cross-compilation plus Release artifacts (this also feeds the
  npm package), and publishing the npm package properly.
- **Verification tooling** — make the canary check repeatable per agent.
- Rule content transformation, templating, and automatic self-update are **explicitly out of scope**
  ([design/requirements.md](docs/design/requirements.md) §五).

---

## 9. License

MIT — see [LICENSE](LICENSE).

---

## 10. Documentation

| Want to read | Where |
|---|---|
| What the user actually wants (requirements / acceptance) | [docs/design/requirements.md](docs/design/requirements.md) |
| Why it is designed this way | [docs/design/architecture.md](docs/design/architecture.md) |
| Feature list | [docs/design/features.md](docs/design/features.md) |
| Checklist for adding a new agent | [docs/design/features/agent-onboarding.md](docs/design/features/agent-onboarding.md) |
| Each agent's rules directory (external facts) | [docs/design/external/agent-rules-dirs.md](docs/design/external/agent-rules-dirs.md) |
| Current progress and open items | [docs/PROGRESS.md](docs/PROGRESS.md) |
