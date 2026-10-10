/**
 * rulemux-dsh — a native Cordis plugin for DeepSeek Harness (dsh).
 *
 * Two halves:
 *   - WRITE half: the `rulemux` CLI copies your rule files into <cwd>/.dsh/rules/__rulemux__*.md
 *     (real copies, never symlinks).
 *   - READ half (this file): reads those copies back and injects them into the session ONCE, on the
 *     first user turn — the same text, hence the same token cost, as a host that scans a rules
 *     directory natively. It reads ONLY rulemux's own `__rulemux__` prefix, so it coexists with any
 *     other `.dsh/rules` reader.
 *
 * Why this package self-provisions instead of depending on `rulemux`:
 *   A hard npm dependency makes the whole `dsh plugin add` fail whenever the registry mirror lags —
 *   an install-time failure the user cannot act on, for a plugin they only wanted to add. So this
 *   package declares NO dependencies: it always installs, and the CLI is obtained on FIRST RUN,
 *   in-process, exactly once (provisionOnce):
 *     1. resolve it (a dependency copy if one exists → `rulemux` on PATH);
 *     2. if missing, install it globally (pnpm -g, falling back to npm -g);
 *     3. create ~/.rulemux/config.toml if missing (via `rulemux init --agent dsh`).
 *   Then the session syncs and injects as usual.
 *
 * The CLI is a HARD prerequisite, not a nice-to-have: without it there is no sync, so the session
 * would run on stale (or no) rules while looking perfectly healthy. Therefore provisioning BLOCKS
 * the first turn until it finishes — there is deliberately no "carry on without it" fallback — and
 * if it ultimately fails it THROWS, so the failure is visible rather than a quietly rule-less
 * session. A wrong rule set is worse than a loud failure.
 *
 * The one thing that is NOT a failure: a config that has no active [[source]] yet (our CLI exits
 * non-zero for that). That is a setup state, not breakage — nothing configured means nothing to
 * sync, not something broken — so it is REPORTED to the user once (see readyNotice) instead of
 * raised. Raising there would make a fresh install permanently unusable, since the config we
 * auto-create starts empty.
 *
 * It imports nothing from dsh: every host shape is structurally typed here, so any dsh whose event
 * names still match can load it (the same approach hindsight's coding-agents takes in src/dsh.ts).
 *
 * Why inject ONCE, not every turn: the rules are static, so re-adding them per turn would duplicate
 * the same text in history and waste tokens. If a later compaction drops the block we re-inject on
 * the next user turn — but only when we can POSITIVELY tell the host had been holding it (see
 * stillInContext).
 */

import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { createRequire } from "node:module";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

export const name = "rulemux";
export const inject = ["agents"];

const RULES_SUBDIR = ".dsh/rules"; // where `rulemux sync` drops the copies (workspace-relative)
const FILE_PREFIX = "__rulemux__"; // rulemux's ownership prefix: other files are never read
const MANAGED_HEADER = "<!-- rulemux:managed -->"; // marks our injected block (re-injection check)
const SOURCE_KIND = "plugin:rulemux"; // producer-owned source kind dsh expects for injected messages
const CONFIG_REL = [".rulemux", "config.toml"]; // mirrors config.DefaultPath() in the Go CLI

// `rulemux sync` exits non-zero when the config declares no [[source]] at all. That means "you have
// not configured any rules yet", not "rulemux is broken" — matched on our own CLI's wording; both
// halves ship from this repo, so the string moves together with the plugin.
const NO_SOURCES = /has no \[\[source\]\]/i;

const INSTALL_TIMEOUT_MS = 120_000; // global install / sync: generous, but never hang forever
const PROBE_TIMEOUT_MS = 15_000; // resolving / probing the CLI

const require_ = createRequire(import.meta.url);

/** Where the Go CLI keeps its config (config.DefaultPath() = ~/.rulemux/config.toml). */
function configPath() {
  return join(homedir(), ...CONFIG_REL);
}

/** Last `max` characters of a captured stream, whitespace-collapsed, for error messages. */
function tail(s, max = 400) {
  const t = String(s || "").trim().replace(/\s+/g, " ");
  return t.length > max ? `…${t.slice(-max)}` : t;
}

/**
 * Run a child process, capturing both streams, always resolving (never rejecting) with
 * { code, out, err, error }. `code === 0` means success; a timeout or a spawn failure comes back as
 * `error` with `code === null` — callers treat both as failure.
 */
function run(cmd, args, { cwd, timeoutMs = INSTALL_TIMEOUT_MS } = {}) {
  return new Promise((resolve) => {
    let out = "";
    let err = "";
    let settled = false;
    let timer = null;
    const finish = (code, error) => {
      if (settled) return;
      settled = true;
      if (timer) clearTimeout(timer);
      resolve({ code, out, err, error, cmd, args });
    };

    let child;
    try {
      child = spawn(cmd, args, { cwd, stdio: ["ignore", "pipe", "pipe"] });
    } catch (e) {
      finish(null, e);
      return;
    }
    if (timeoutMs > 0) {
      timer = setTimeout(() => {
        try {
          child.kill("SIGKILL");
        } catch {
          /* already gone */
        }
        finish(null, new Error(`timed out after ${timeoutMs}ms`));
      }, timeoutMs);
      if (typeof timer.unref === "function") timer.unref();
    }
    child.stdout?.on("data", (b) => {
      out += b.toString();
    });
    child.stderr?.on("data", (b) => {
      err += b.toString();
    });
    child.on("error", (e) => finish(null, e));
    child.on("close", (code) => finish(code, undefined));
  });
}

/**
 * How to invoke the rulemux CLI, or null when it is nowhere to be found:
 *   - a copy installed alongside us as an npm dependency (legacy layout), else
 *   - `rulemux` on PATH.
 */
async function resolveCli() {
  try {
    const pkg = require_.resolve("rulemux/package.json");
    const shim = join(dirname(pkg), "bin", "rulemux.js");
    if (existsSync(shim)) return { cmd: process.execPath, args: [shim], how: "dependency" };
  } catch {
    /* not installed as a dependency (the normal case: this package has no dependencies) */
  }
  const probe = await run("rulemux", ["--version"], { timeoutMs: PROBE_TIMEOUT_MS });
  if (probe.code === 0) {
    return { cmd: "rulemux", args: [], how: "PATH" };
  }
  return null;
}

/** The global bin directory a package manager installs executables into ("" if unknown). */
async function globalBinDir(pm) {
  if (pm === "npm") {
    const r = await run("npm", ["prefix", "-g"], { timeoutMs: PROBE_TIMEOUT_MS });
    if (r.code !== 0) return "";
    const prefix = r.out.trim().split("\n").filter(Boolean).pop()?.trim() || "";
    return prefix ? join(prefix, "bin") : "";
  }
  const r = await run("pnpm", ["bin", "-g"], { timeoutMs: PROBE_TIMEOUT_MS });
  if (r.code !== 0) return "";
  return r.out.trim().split("\n").filter(Boolean).pop()?.trim() || "";
}

/**
 * A global bin dir is not always on the PATH of the process that just installed into it (pnpm's
 * PNPM_HOME being the usual offender), so after an install we also look where the pm itself says it
 * put things. Without this, a *successful* install could still look like a failure.
 */
async function cliInGlobalBin(pm) {
  const dir = await globalBinDir(pm);
  if (!dir) return null;
  for (const candidate of [join(dir, "rulemux"), join(dir, "rulemux.cmd")]) {
    if (existsSync(candidate)) return { cmd: candidate, args: [], how: `${pm} -g (${dir})` };
  }
  return null;
}

/**
 * Install the CLI globally: pnpm first (dsh itself uses pnpm), then npm. Returns
 * { cli, installedBy } on success, or null with every failure appended to `reasons`.
 */
async function installCli(reasons) {
  for (const pm of ["pnpm", "npm"]) {
    const args = pm === "npm" ? ["install", "-g", "rulemux"] : ["add", "-g", "rulemux"];
    const r = await run(pm, args);
    if (r.error) {
      reasons.push(`${pm}: cannot run (${r.error.message})`);
      continue;
    }
    if (r.code !== 0) {
      reasons.push(`${pm} ${args.join(" ")} → exit ${r.code}: ${tail(r.err || r.out) || "no output"}`);
      continue;
    }
    const cli = (await resolveCli()) || (await cliInGlobalBin(pm));
    if (cli) return { cli, installedBy: cli.how === "PATH" ? `${pm} -g` : cli.how };
    reasons.push(`${pm}: reported success but rulemux is still not resolvable`);
  }
  return null;
}

/**
 * The one-time, process-wide setup. Idempotent by construction: a module-level singleton, plus an
 * existsSync check before touching the config. Rejects (never resolves partially) when the CLI
 * cannot be obtained or the config cannot be created — see the file header for why that is not
 * softened into a fallback.
 */
let provisioning = null;

function provisionOnce() {
  if (!provisioning) {
    provisioning = (async () => {
      const reasons = [];
      let cli = await resolveCli();
      let installedBy = "";

      if (!cli) {
        const installed = await installCli(reasons);
        if (!installed) {
          throw new Error(
            "rulemux-dsh: the `rulemux` CLI is required but could not be installed, so this " +
              "session cannot sync your rules.\n" +
              reasons.map((x) => `  - ${x}`).join("\n") +
              "\n  Install it yourself and start a new dsh session:\n" +
              "    npm install -g rulemux        # or: pnpm add -g rulemux",
          );
        }
        cli = installed.cli;
        installedBy = installed.installedBy;
      }

      const cfg = configPath();
      let createdConfig = false;
      if (!existsSync(cfg)) {
        const r = await run(cli.cmd, [...cli.args, "init", "--agent", "dsh"]);
        if (r.error || r.code !== 0) {
          throw new Error(
            `rulemux-dsh: could not create ${cfg} (\`rulemux init --agent dsh\`).\n` +
              `  ${tail(r.err || r.out) || r.error?.message || `exit ${r.code}`}`,
          );
        }
        createdConfig = true;
      }

      return { cli, installedBy, createdConfig, configPath: cfg };
    })();
  }
  return provisioning;
}

/**
 * The one-time notice shown in the first session after we installed / initialised something, or
 * after discovering that the config has no active [[source]] yet.
 */
function readyNotice(prov) {
  const lines = [];
  if (prov.installedBy) {
    lines.push(`- The rulemux CLI was installed globally (${prov.installedBy}).`);
  }
  if (prov.createdConfig) {
    lines.push(`- A configuration file was created at ${prov.configPath}.`);
  }
  if (prov.nothingToSync) {
    lines.push(
      `- ${prov.configPath} declares no active [[source]] yet, so no rules are being synced. Add ` +
        "your rule files there as [[source]] entries to start.",
    );
  }
  if (!lines.length) return "";
  return [
    "rulemux (one-time setup): the rulemux dsh plugin finished setting itself up.",
    ...lines,
    "Tell the user this once, in the language you are currently using with them, before answering " +
      "their request.",
  ].join("\n");
}

/**
 * Sync once for this workspace. A non-zero exit is a failure and is raised — except for two cases:
 *   - a config with no [[source]] at all ⇒ { nothingToSync: true } (see the file header);
 *   - an OLD CLI that predates the dsh adapter answers "unknown agent": upgrade it globally once
 *     and retry.
 */
async function syncOnce(root, cli) {
  let r = await run(cli.cmd, [...cli.args, "sync", "--hook", "--agent", "dsh"], { cwd: root });
  if (r.code === 0) return { nothingToSync: false };
  if (NO_SOURCES.test(`${r.err}\n${r.out}`)) return { nothingToSync: true };

  const text = `${r.err}\n${r.out}`;
  if (/unknown agent|not supported yet/i.test(text)) {
    const up = await run("npm", ["install", "-g", "rulemux@latest"]);
    if (up.code === 0) {
      const newer = (await resolveCli()) || (await cliInGlobalBin("npm"));
      if (newer) {
        const retry = await run(newer.cmd, [...newer.args, "sync", "--hook", "--agent", "dsh"], {
          cwd: root,
        });
        if (retry.code === 0) return { nothingToSync: false };
        if (NO_SOURCES.test(`${retry.err}\n${retry.out}`)) return { nothingToSync: true };
        r = retry;
      }
    }
  }

  throw new Error(
    `rulemux-dsh: \`rulemux sync --hook --agent dsh\` failed in ${root}.\n` +
      `  ${tail(r.err || r.out) || r.error?.message || `exit ${r.code}`}`,
  );
}

/** Ready-to-inject state for a workspace: CLI + config guaranteed, rules synced. */
async function ensureReady(root) {
  const prov = await provisionOnce();
  const sync = await syncOnce(root, prov.cli);
  return { ...prov, nothingToSync: sync.nothingToSync };
}

/** The directory a session is working in (dsh records it on the session header). */
function workspaceRoot(agent) {
  return agent?.session?.header?.cwd || process.cwd();
}

/** Concatenate rulemux's own rule files for a workspace; "" when there are none. */
function readRules(root) {
  const dir = join(root, RULES_SUBDIR);
  if (!existsSync(dir)) return "";
  const parts = [];
  for (const entry of readdirSync(dir).sort()) {
    if (!entry.startsWith(FILE_PREFIX) || !entry.endsWith(".md")) continue;
    try {
      const text = readFileSync(join(dir, entry), "utf8").trim();
      if (text) parts.push(text);
    } catch {
      /* a single unreadable file must not abort the whole injection */
    }
  }
  return parts.join("\n\n");
}

/** Our managed block: the header (re-injection check) plus whatever we have to say. */
function composeBlock(notice, rules) {
  const body = [notice, rules].filter(Boolean).join("\n\n");
  return `${MANAGED_HEADER}\n\n${body}`;
}

/** The injection message: a non-user-typed message dsh renders as recalled material. */
function injectionMessage(text) {
  return {
    id: randomUUID(),
    role: "user",
    content: [{ type: "text", text }],
    source: { kind: SOURCE_KIND, form: "recall" },
  };
}

/**
 * Is our injected block still in the session's context? Returns true / false / "unknown".
 *
 * "unknown" (host exposes no event log) must be treated as PRESENT: re-injecting every turn would
 * be the worst outcome, so we only ever re-inject when the host can prove the block is gone.
 */
function stillInContext(session) {
  const snap = session?.snapshotEvents;
  if (typeof snap !== "function") return "unknown";
  try {
    const events = snap.call(session) || [];
    const from = Math.max(0, events.length - 30);
    for (let i = events.length - 1; i >= from; i--) {
      if (JSON.stringify(events[i]).includes(MANAGED_HEADER)) return true;
    }
    return false;
  } catch {
    return "unknown";
  }
}

/** Per-session state, built once per session id (one dsh process serves many sessions). */
const sessions = new Map(); // sessionId -> { root, ready, injected, observed }

function stateFor(agent) {
  const id = agent?.session?.header?.id;
  if (!id) return undefined;
  let s = sessions.get(id);
  if (!s) {
    s = { root: workspaceRoot(agent), ready: null, injected: false, observed: false };
    sessions.set(id, s);
  }
  return s;
}

/**
 * Kick off (or reuse) the one readiness chain for a session. Started at session-start so the
 * install/init work overlaps the user typing; awaited — with no upper bound — at the first turn.
 * The session-start copy must not reject unobserved, so a no-op catch is attached: the real error
 * is surfaced where it matters, at the first pre-step.
 */
function startReady(state) {
  if (!state.ready) {
    state.ready = ensureReady(state.root);
    state.ready.catch(() => {});
  }
  return state.ready;
}

/** Shown once per process: the fact that we installed / initialised something is setup news. */
let noticeShown = false;

export function apply(ctx) {
  ctx.on("agent/session-start", ({ agent }) => {
    const state = stateFor(agent);
    if (state) startReady(state);
  });

  ctx.on("agent/pre-step", async ({ agent, signal }, next) => {
    const decision = await next();
    if (decision.kind !== "enter" || signal?.aborted) return decision;

    const state = stateFor(agent);
    if (!state) return decision;

    // Only a turn claiming new human input is worth (re)injecting on; tool continuations carry none.
    const hasUserInput = (decision.messages || []).some(
      (m) => m?.source?.kind === "user" && (m.content || []).some((b) => b?.type === "text" && b.text),
    );
    if (!hasUserInput) return decision;

    if (state.injected) {
      const present = stillInContext(agent.session);
      if (present === "unknown") return decision; // cannot tell ⇒ never re-inject
      if (present === true) {
        state.observed = true;
        return decision;
      }
      // present === false: the host showed our block before and now it is gone (compaction).
      if (!state.observed) return decision; // host never exposed it ⇒ do not spam every turn
      state.injected = false; // fall through and re-inject
    }

    // Hard prerequisite: sync + injection only happen once the CLI and config are really there.
    // A rejection here is meant to surface (see the file header) — it is not caught.
    const prov = await startReady(state);

    const rules = readRules(state.root);
    let notice = "";
    if (!noticeShown) {
      notice = readyNotice(prov);
      if (notice) noticeShown = true;
    }

    if (!rules && !notice) {
      state.injected = true; // nothing to inject: do not re-scan on every later turn
      return decision;
    }
    state.injected = true;
    return {
      kind: "enter",
      messages: [...decision.messages, injectionMessage(composeBlock(notice, rules))],
    };
  });

  ctx.on("agent/disposed", ({ agent }) => {
    const id = agent?.session?.header?.id;
    if (id) sessions.delete(id);
  });
}

export default { name, inject, apply };
