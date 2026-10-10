# rulemux-dsh

DeepSeek Harness (dsh) plugin for [rulemux](https://github.com/cq-guojia/rulemux).

rulemux syncs the rule files you maintain into each agent's native rules directory. dsh is
plugin-first and does **not** scan a rules directory on its own, so this plugin is the reading half:
it reads the copies rulemux puts in `<workspace>/.dsh/rules/__rulemux__*.md` and injects them into
the session once, at the first user turn — the same rules, the same token cost as a host that reads
a rules directory natively.

## Install

```bash
dsh plugin --profile <name> add rulemux-dsh
```

No npm publish is required — any of these work:

```bash
# straight from git, pointing at this sub-package. The `#path:/<subdir>` spec is how dsh
# installs a package that lives in a subdirectory of a repo (quote it: `#` starts a shell comment).
dsh plugin --profile <name> add "github:cq-guojia/rulemux#path:/dsh-plugin"

# or from a packed tarball
npm pack                       # -> rulemux-dsh-<version>.tgz
dsh plugin --profile <name> add ./rulemux-dsh-<version>.tgz
```

> Publishing to npm is **optional**: it lets dsh install the prebuilt package and gives the plugin a
> download count in the market — but listing and installation work exactly the same without it.

The host half needs no build: `index.mjs` is plain ESM. The details-page panel is bundled into
`lib/client.js`, and that artifact is committed — so a git install still needs no `allowBuilds`
approval (see “Build” below if you change `src/client/`).

**Restart dsh after installing** — the plugin mounts on the next start.

## What you see in dsh

- **Plugin list**: an icon and a title, in the language of your UI (English / Chinese).
- **Plugin detail page**: an **informational** panel — the config file path
  `~/.rulemux/config.toml` (with a copy button), what you need to edit by hand, a minimal example,
  and a button that opens the project on GitHub.

It **does not save anything**. `~/.rulemux/config.toml` is the single source of truth and you edit
it in your own editor. dsh's own settings document is not a second home for it: the official config
form can only store values in dsh's document (the active profile's Cordis patch), and a plugin on
the detail page has no session context — it cannot read a file off your disk. So the panel tells you
where the file is and what it looks like, and the editing stays where it always was.

## Build (only for the details-page panel)

```bash
npm install
npm run build      # -> lib/client.js   (re-run and commit the artifact after changing src/client/)
npm run typecheck
```

## The flow you get

```bash
dsh plugin --profile <p> add "github:cq-guojia/rulemux#path:/dsh-plugin"
# restart dsh              → ~/.rulemux/config.toml now exists (see below)
# edit it                  → add your [[source]] entries
# open ONE session         → the rules are injected
```

No throwaway "chat once so it can create the config" session: readiness starts at load, before any
session exists.

## At dsh startup: three steps, all or nothing

This package declares **no dependencies** on purpose: a hard `rulemux` dependency would make the
whole `dsh plugin add` fail whenever the registry mirror lags — an install-time failure you cannot
act on. dsh runs no install scripts either, so instead the plugin makes itself ready **when it loads,
i.e. when dsh starts**, and it does exactly three things. **Every one of them must hold; otherwise the
session fails with an error.**

1. **The CLI is there and can do the job.** `rulemux` is resolved (a dependency copy if one exists,
   else `PATH`) and must either satisfy version `>=0.3.1` **or** demonstrably know the `dsh` agent
   (`rulemux --help` lists it). Both halves matter: a version floor is meaningless if a capable CLI
   looks old — a binary built with plain `go build` / `go install` reports main.go's hardcoded default
   version whatever code it contains — and a version number alone proves nothing about capability. A
   CLI that fails both checks is upgraded with `pnpm add -g`, falling back to `npm install -g`, and
   re-checked afterwards. If it still fails, that is a **failure**, not a success.
2. **This plugin is loaded.** Nothing to check: the fact that this code runs at all is the proof.
3. **The config exists.** `~/.rulemux/config.toml`, created with `rulemux init --agent dsh` when it is
   missing. An existing config counts as success and is **never overwritten**.

Because steps 1 and 3 need no workspace, they run at load; step 2 is the load itself.

Failures are loud and actionable: the error names the step that failed, carries the raw output, and
says where to fix it (then restart the session to retry). There is deliberately no "succeeded
halfway" notice — a half-ready plugin is useless, and it must not look healthy.

## Syncing is separate, and never fails the session

`rulemux sync --hook --agent dsh` runs once per session — it needs the session's workspace, which
does not exist at load time — exactly as syncing works for every other agent. A non-zero exit is
**logged** (`console.error`, which lands in dsh's log) and the session carries on with whatever rule
files are already on disk.

A config with no `[[source]]` yet lands here too — the CLI exits non-zero, it is logged, and nothing
is injected. That is a normal syncing outcome, **not** a readiness failure: it has no bearing on the
three steps above.

## What it does

- At load (dsh startup) → starts the three-step readiness chain, so `~/.rulemux/config.toml` is there
  before you need it. At `agent/session-start` → runs `rulemux sync --hook --agent dsh` **once**, so
  the on-disk copies are current.
- `agent/pre-step` → on the first turn that carries user input, reads `.dsh/rules/__rulemux__*.md`
  and injects them as recalled material. It injects **once** per session and only re-injects if a
  later compaction provably dropped the block — never every turn.
- It reads **only** files with rulemux's `__rulemux__` prefix, so it coexists with other
  `.dsh/rules` readers.

## Requirements

- Node 18+ (plain ESM) — dsh's own runtime satisfies this.
- Network and permission for a global install, **only if** `rulemux` is missing or too old.
- A rulemux config (`~/.rulemux/config.toml`) whose `[[source]]` entries cover the workspace; it is
  created for you at dsh startup, then it is yours to fill in.

## License

MIT — same as rulemux.
