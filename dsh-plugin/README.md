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

## Nothing else to install: the first run sets itself up

This package declares **no dependencies** on purpose: a hard `rulemux` dependency would make the
whole `dsh plugin add` fail whenever the registry mirror lags — an install-time failure you cannot
act on, for a plugin you only wanted to add. The `rulemux` CLI is instead obtained on the **first
run**, inside the session, exactly once:

1. resolve it — a dependency copy if one exists, else `rulemux` on `PATH`;
2. if missing, install it globally: `pnpm add -g rulemux`, falling back to `npm install -g rulemux`;
3. create `~/.rulemux/config.toml` if it is missing (a commented sample — add your `[[source]]` entries).

Then the session syncs and injects as usual, and the first session carries one short notice telling
you what was installed and created.

**Failure is loud, not silent.** The CLI is a hard prerequisite: without it nothing syncs, so the
session would run on stale (or empty) rules while looking perfectly healthy. Provisioning therefore
**blocks** the first turn until it finishes, and if it ultimately fails it **raises an error**,
naming each reason and the command to run by hand — it never carries on without rules.

The one thing that is *not* an error: a config with no `[[source]]` yet. That is a setup state, not
a broken install, so it is reported to you once instead of raised.

## What it does

- `agent/session-start` → starts the one-time setup above, then runs `rulemux sync --hook --agent dsh`
  **once**, so the on-disk copies are current.
- `agent/pre-step` → on the first turn that carries user input, reads `.dsh/rules/__rulemux__*.md`
  and injects them as recalled material. It injects **once** per session and only re-injects if a
  later compaction provably dropped the block — never every turn.
- It reads **only** files with rulemux's `__rulemux__` prefix, so it coexists with other
  `.dsh/rules` readers.

## Requirements

- Node 18+ (plain ESM) — dsh's own runtime satisfies this.
- Network and permission for a global install, **only if** `rulemux` is not already resolvable.
- A rulemux config (`~/.rulemux/config.toml`) whose `[[source]]` entries cover the workspace; it is
  created for you on first run, then it is yours to fill in.

## License

MIT — same as rulemux.
