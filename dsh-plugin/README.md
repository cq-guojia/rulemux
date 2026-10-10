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

There is no build step: `index.mjs` is plain ESM, so a git install needs no `allowBuilds` approval.

**Restart dsh after installing** — the plugin mounts on the next start.

## First run: three steps, all or nothing

This package declares **no dependencies** on purpose: a hard `rulemux` dependency would make the
whole `dsh plugin add` fail whenever the registry mirror lags — an install-time failure you cannot
act on. So instead the plugin makes itself ready on the **first run**, and it does exactly three
things. **Every one of them must hold; otherwise the session fails with an error.**

1. **The CLI is there and current.** `rulemux` is resolved (a dependency copy if one exists, else
   `PATH`) and its version must satisfy `>=0.3.0`. "It is installed" is only half of it: an older CLI
   (or none) is upgraded with `pnpm add -g`, falling back to `npm install -g`, and the version is
   re-read afterwards. If the version still does not satisfy — e.g. an outdated copy elsewhere keeps
   shadowing the upgrade — that is a **failure**, not a success.
2. **This plugin is loaded.** Nothing to check: the fact that this code runs at all is the proof.
3. **The config exists.** `~/.rulemux/config.toml`, created with `rulemux init --agent dsh` when it is
   missing. An existing config counts as success and is **never overwritten**.

Failures are loud and actionable: the error names the step that failed, carries the raw output, and
says where to fix it (then restart the session to retry). There is deliberately no "succeeded
halfway" notice — a half-ready plugin is useless, and it must not look healthy.

## Syncing is separate, and never fails the session

`rulemux sync --hook --agent dsh` runs once per session, exactly as it does for every other agent.
A non-zero exit is **logged** (`console.error`, which lands in dsh's log) and the session carries on
with whatever rule files are already on disk.

A config with no `[[source]]` yet lands here too — the CLI exits non-zero, it is logged, and nothing
is injected. That is a normal syncing outcome, **not** a readiness failure: it has no bearing on the
three steps above.

## What it does

- `agent/session-start` → starts the one-time three-step readiness chain, then runs
  `rulemux sync --hook --agent dsh` **once**, so the on-disk copies are current.
- `agent/pre-step` → on the first turn that carries user input, reads `.dsh/rules/__rulemux__*.md`
  and injects them as recalled material. It injects **once** per session and only re-injects if a
  later compaction provably dropped the block — never every turn.
- It reads **only** files with rulemux's `__rulemux__` prefix, so it coexists with other
  `.dsh/rules` readers.

## Requirements

- Node 18+ (plain ESM) — dsh's own runtime satisfies this.
- Network and permission for a global install, **only if** `rulemux` is missing or too old.
- A rulemux config (`~/.rulemux/config.toml`) whose `[[source]]` entries cover the workspace; it is
  created for you on first run, then it is yours to fill in.

## License

MIT — same as rulemux.
