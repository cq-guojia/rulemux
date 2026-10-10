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
- **The session hook is (almost) only the courier.** Rule *content* is never injected: files are
  really copied into the directory the agent loads natively, so they enjoy static-prefix semantics
  (never fade out mid-conversation, never accumulate). The single, deliberate exception is one
  **transient notice line** the hook emits when it detects that the rules really changed — it asks
  you to start a new session. See §5.3.
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
| **codebuddy** | Tier-1 (real copy) | `.codebuddy/rules/` | ✅ **Verified — installable** (`codebuddy-cn` is an alias) |
| **workbuddy** | Tier-1 (real copy) | `.workbuddy/rules/` | ✅ **Verified — installable** (separate app: own `~/.workbuddy/settings.json` hook, no longer an alias of codebuddy) |
| claude (Claude Code) | Tier-1 | `.claude/rules/` | ⚠️ registered, **not verified yet** — cannot be installed |
| trae (Trae) | Tier-1 | `.trae/rules/` | ⚠️ registered, **not verified yet** — cannot be installed |
| codex | Tier-2 (injection) | none — injects into context | ⚠️ registered, **not verified yet** |
| opencode | Tier-2 (injection) | none — injects into context | ⚠️ registered, **not verified yet** |
| **dsh** (DeepSeek Harness) | Tier-1 (real copy) | `.dsh/rules/` | ✅ **Verified — installable**; the reading half is a **separate dsh plugin** — install it with `dsh plugin --profile <p> add rulemux-dsh` (rulemux only syncs; it does not install the plugin) |

- `rulemux init` **refuses** any agent that is not verified — it will not half-install an adapter
  whose behaviour has not been proven. `rulemux doctor` marks unverified agents with ⚠.
- **Tier-2 is a deliberate downgrade** for agents that have no rules directory (they only read a
  single `AGENTS.md`). It injects via the session hook and never touches your own `AGENTS.md`, but
  it cannot satisfy the "never fades out" bar. See
  [design/features/hook-injection.md](docs/design/features/hook-injection.md).
- **dsh is a two-part story**: it has no hook file. rulemux writes `.dsh/rules/` (Tier-1); a
  **separate plugin package** in this repo ([`dsh-plugin/`](dsh-plugin/)) reads it back, installed
  the normal dsh way — straight from git, no npm publish needed:
  `dsh plugin --profile web add "github:cq-guojia/rulemux#path:/dsh-plugin"`. So
  `rulemux init --agent dsh` installs nothing — it just prints the plugin command.
  That plugin then **sets itself up on first run** (restart dsh after installing). Readiness is three
  steps and **all of them must hold**: the `rulemux` CLI is resolved *and* its version satisfies the
  plugin's minimum (an older one is upgraded with pnpm/npm and re-checked — never accepted as-is),
  the plugin itself is loaded, and `~/.rulemux/config.toml` exists (created for you when missing,
  never overwritten). Any step that does not hold **raises**, so the session fails visibly instead of
  quietly running without rules. Syncing is a separate concern: a failing `rulemux sync` is logged,
  and the session carries on.

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
`__rulemux__*` file that is not in that set, copies/overwrites the ones that are missing or changed,
and skips the rest.

- Destination name: `__rulemux__` + source basename (a short hash is appended on name collisions).
- **Your source files are only ever read** — never modified or deleted.
- **Files without the `__rulemux__` prefix in the rules directory are never touched**, so your own
  rule files stay safe.

> ⚠️ **Run `rulemux sync` after editing your config or source docs**
> After you edit `~/.rulemux/config.toml` (adding/removing a `[[source]]`, changing `path`) or any
> source rule document, run `rulemux sync --agent codebuddy` (omit `--agent` to sync every supported
> agent).
> - **If you run sync**: the next new session reads the updated rules.
> - **If you don't**: CodeBuddy / WorkBuddy still auto-sync on every session start via the SessionStart
>   hook, but the harness fixes the rule snapshot at the **start of the session** (before the hook runs),
>   so the current session still sees the old version — the update only takes effect on the **second**
>   new session.

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

---

## 7. Commands

| Command | What it does |
|---|---|
| `rulemux sync [--hook] [--agent <id>] [--config <path>] [--workspace <dir>]` | Sync rules. `--hook` = this run comes from an agent's SessionStart hook (only then may it create the rules dir and emit the change notice); omit `--agent` to target every supported agent |
| `rulemux sync --all [--config <path>]` | Manually re-align every workspace recorded in the ledger (never reads cwd, never creates dirs, prunes ledger entries whose directory is gone). Mutually exclusive with `--hook` / `--agent` / `--workspace` |
| `rulemux inject --agent <id> [--config <path>]` | Tier-2: print rules to stdout for hook injection |
| `rulemux init --agent <id[,id...]> [--config <path>] [--workspace <dir>]` | Write a sample config and install the SessionStart hook |
| `rulemux init --refresh [--agent <id[,id...]>] [--workspace <dir>]` | Refresh **only** the hooks rulemux already installed. This also happens by itself on the first sync after an upgrade; use this to force it. Never adds a hook, never creates a file or directory, preserves every other entry and key in the host config |
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

## 9. Releasing (maintainers)

```bash
npm version patch|minor|major   # bumps package.json and creates the git tag v0.x.y
git push --follow-tags
```

Pushing the tag triggers [`.github/workflows/release.yml`](.github/workflows/release.yml):
cross-compile all platform binaries, attach them to a GitHub Release, and publish the
npm package with the tag's version. Set the `NPM_TOKEN` repository secret to enable
the npm step (it is skipped with a warning if absent). A plain CI run
([`ci.yml`](.github/workflows/ci.yml)) runs build/vet/tests/smoke on every push.

---

## 10. License

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
