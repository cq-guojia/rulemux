/**
 * Host-plane tests: the real plugin module driven end to end, with the `rulemux` CLI stubbed.
 *
 * WHY A STUB, NOT A REAL dsh: there is no dsh in CI (and none on the machine that writes this), so
 * the only honest thing to verify here is our own half — that apply() → session-start → pre-step
 * asks the CLI for the rules, injects exactly that text once, and writes nothing into the
 * workspace. The stub is a three-line shell script placed first on PATH, which is exactly how the
 * plugin resolves a CLI (`resolveCli`). The dsh-side canary (a real session on a real dsh) is a
 * separate, manual step — see docs/worklog/202610/10-0000-dsh-adapter-9f1e.md.
 *
 * Each case gets its own temp HOME, its own stub and its own module instance: readiness is a
 * process-wide singleton inside the module, so reusing an instance across cases would silently
 * reuse the first case's CLI (the cache-busting `?case=` query is what makes the import fresh).
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const PLUGIN = join(dirname(fileURLToPath(import.meta.url)), "..", "index.mjs");
const RULES = "<!-- rulemux:managed -->\n\n# base\nkeep it short\n";

let caseNo = 0;

/**
 * Set up an isolated world: temp HOME, temp workspace, and a stub `rulemux` first on PATH.
 * The stub records every argv into $STUB_LOG, answers `--version` / `--help` / `init` / `inject`,
 * and can be told to fail `inject` (that failure must never fail the session).
 */
async function world({ injectExit = 0, rules = RULES, version = "0.4.7" } = {}) {
  const tmp = mkdtempSync(join(tmpdir(), "rulemux-dsh-test-"));
  const home = join(tmp, "home");
  const bin = join(tmp, "bin");
  const ws = join(tmp, "ws");
  for (const d of [home, bin, ws]) mkdirSync(d, { recursive: true });

  const log = join(tmp, "calls.log");
  const stub = join(bin, "rulemux");
  writeFileSync(
    stub,
    [
      "#!/bin/sh",
      'printf \'%s\\n\' "$*" >> "$STUB_LOG"',
      'case "$1" in',
      `  --version) printf "rulemux ${version}\\n"; exit 0 ;;`,
      '  --help) printf "  dsh   DeepSeek Harness (self-contained)\\n"; exit 0 ;;',
      '  init) mkdir -p "$HOME/.rulemux"; : > "$HOME/.rulemux/config.toml"; exit 0 ;;',
      '  inject) printf "%s" "$STUB_RULES"; exit "$STUB_INJECT_EXIT" ;;',
      "esac",
      "exit 0",
      "",
    ].join("\n"),
  );
  chmodSync(stub, 0o755);

  // Guard rails: an unsatisfied CLI is upgraded with pnpm, then npm. Stub both as failing no-ops so a
  // test that exercises the upgrade path can never reach the network or touch the real global install
  // (and so "old CLI ⇒ loud failure" is deterministic).
  for (const pm of ["pnpm", "npm"]) {
    const shim = join(bin, pm);
    writeFileSync(shim, ["#!/bin/sh", 'printf \'%s\\n\' "$*" >> "$STUB_LOG"', "exit 1", ""].join("\n"));
    chmodSync(shim, 0o755);
  }

  process.env.HOME = home;
  process.env.STUB_LOG = log;
  process.env.STUB_RULES = rules;
  process.env.STUB_INJECT_EXIT = String(injectExit);
  process.env.PATH = `${bin}:${process.env.PATH}`;

  const mod = await import(`${pathToFileURL(PLUGIN).href}?case=${++caseNo}`);
  const handlers = new Map();
  mod.apply({
    on: (event, fn) => {
      if (!handlers.has(event)) handlers.set(event, []);
      handlers.get(event).push(fn);
    },
  });

  const agent = { session: { header: { id: `s${caseNo}`, cwd: ws } } };
  /** Drive one pre-step: the host offers a turn carrying user input; we return what comes back. */
  const turn = async () => {
    const decision = {
      kind: "enter",
      messages: [{ source: { kind: "user" }, content: [{ type: "text", text: "hi" }] }],
    };
    const fn = handlers.get("agent/pre-step")[0];
    assert.ok(fn, "the plugin registered no agent/pre-step handler");
    return fn({ agent }, async () => decision);
  };

  return { tmp, home, ws, log, turn, calls: () => (existsSync(log) ? readFileSync(log, "utf8") : "") };
}

test("asks the CLI for the rules, injects them once, and writes nothing", async () => {
  const w = await world();

  const first = await w.turn();
  assert.equal(first.messages.length, 2, "the first turn must carry one injected block");
  const block = first.messages[1];
  assert.equal(block.content[0].text, RULES.trim(), "the injected text is the CLI's stdout");
  assert.equal(block.source.kind, "plugin:rulemux");
  assert.equal(block.source.form, "recall");

  // The whole point of this change: no middleman on disk.
  assert.deepEqual(readdirSync(w.ws), [], "the plugin must not write anything into the workspace");

  const calls = w.calls();
  assert.match(calls, /inject --agent dsh/, "the plugin must ask the CLI for the rules");
  assert.doesNotMatch(calls, /sync/, "the plugin must not sync (nothing to sync any more)");

  // Readiness still ran its two workspace-independent steps through the CLI.
  assert.match(calls, /--version/);
  assert.ok(existsSync(join(w.home, ".rulemux", "config.toml")), "missing config must be created");
});

test("injects once per session: a silent host never gets a second copy", async () => {
  const w = await world();

  const first = await w.turn();
  assert.equal(first.messages.length, 2);

  // This stub host exposes no snapshotEvents, so the plugin cannot prove the block is gone. The
  // documented policy is "unknown ⇒ assume present": re-injecting every turn would just duplicate
  // the same text in history.
  const second = await w.turn();
  assert.equal(second.messages.length, 1, "the block must not be injected twice");

  const injects = w.calls().split("\n").filter((l) => l.startsWith("inject ")).length;
  assert.equal(injects, 1, "the CLI must be asked exactly once per session");
});

test("a failing inject is logged, not fatal: the session continues without a block", async () => {
  const w = await world({ injectExit: 3 });

  const decision = await w.turn();
  assert.equal(decision.messages.length, 1, "no block may be injected when the CLI fails");
  assert.match(w.calls(), /inject --agent dsh/, "the CLI must still have been asked");
});

test("a dsh-aware but too-old CLI is not trusted: readiness fails loudly instead of injecting nothing", async () => {
  // 0.3.7 lists `dsh` in --help (so a knowsDsh-only check would wave it through), but it predates the
  // 0.4.2 config parser and mangles the config into an "undefined file group" error at inject time.
  // The stub pnpm/npm refuse to install, so the upgrade cannot happen and readiness MUST fail — a
  // visible session error — rather than silently accept the CLI and inject nothing on every turn.
  const w = await world({ version: "0.3.7" });

  await assert.rejects(() => w.turn(), /rulemux-dsh/);
  assert.doesNotMatch(w.calls(), /inject --agent dsh/, "an unusable CLI must never be asked for rules");
});
