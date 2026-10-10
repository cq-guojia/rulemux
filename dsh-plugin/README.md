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

The plugin declares `rulemux` as a dependency, so the CLI comes along; dsh installs plugin
dependencies the usual way. (No local build is needed — `index.mjs` is plain ESM.)

No npm publish is required — any of these work:

```bash
# straight from git, pointing at this sub-package. The `#path:/<subdir>` spec is how dsh
# installs a package that lives in a subdirectory of a repo (quote it: `#` starts a shell comment).
dsh plugin --profile <name> add "github:cq-guojia/rulemux#path:/dsh-plugin"

# or from a packed tarball
npm pack                       # -> rulemux-dsh-<version>.tgz
dsh plugin --profile <name> add ./rulemux-dsh-<version>.tgz
```

> Publishing to npm is **optional**: it lets dsh install the prebuilt package (skipping the git
> source-build `allowBuilds` step) and gives the plugin a download count in the market — but
> listing and installation work exactly the same with the git spec above.

There is no build step: `index.mjs` is plain ESM, so a git install needs no `allowBuilds` approval.

## What it does

- `agent/session-start` → runs `rulemux sync --hook --agent dsh` **once**, so the on-disk copies are
  current.
- `agent/pre-step` → on the first turn that carries user input, reads `.dsh/rules/__rulemux__*.md`
  and injects them as recalled material. It injects **once** per session and only re-injects if a
  later compaction provably dropped the block — never every turn.
- It reads **only** files with rulemux's `__rulemux__` prefix, so it coexists with other
  `.dsh/rules` readers.

## Requirements

- `rulemux` (installed with this plugin as a dependency, or on `PATH`). If it is missing, the plugin
  still injects whatever is already in `.dsh/rules`.
- A rulemux config (`~/.rulemux/config.toml`) whose `[[source]]` entries cover the workspace.

## License

MIT — same as rulemux.
