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
 * READINESS is exactly THREE steps, and every one of them must hold:
 *   ① the `rulemux` CLI is resolvable AND its version satisfies REQUIRED_CLI. "It is installed" is
 *      only half of it: a present-but-outdated CLI is NOT success — it is upgraded (pnpm -g, falling
 *      back to npm -g) and re-checked, and if the version still does not satisfy we FAIL;
 *   ② this plugin is loaded — implied by the fact that this code is running at all; there is nothing
 *      to check and nothing that could fail here;
 *   ③ ~/.rulemux/config.toml exists. It is created with `rulemux init --agent dsh` when missing; an
 *      existing config counts as success and is NEVER overwritten.
 * A step that does not hold THROWS, so the session fails visibly (dsh shows the error). There is
 * deliberately no "succeeded halfway" notice: a half-set-up plugin is useless, and pretending
 * otherwise is worse than failing.
 *
 * WHEN it runs: at plugin LOAD — i.e. when dsh starts — not at the first session. dsh calls apply()
 * while composing this plugin, and the workspace-independent half of readiness (① CLI, ③ config)
 * needs no session context, so it starts right there. That is what makes the flow sane: install the
 * plugin, restart dsh, find ~/.rulemux/config.toml already waiting, fill in your [[source]] entries,
 * then open ONE session — and the rules are injected. No throwaway "chat once to generate the
 * config" session. (The first session after a load still re-checks nothing: readiness is a
 * process-wide singleton.)
 *
 * SYNCING is a separate concern, exactly as it is for every other agent: `rulemux sync` runs once per
 * session (it needs the session's workspace, which does not exist at load time), and if it fails (no
 * [[source]] configured, a bad path, anything) that is LOGGED and the session carries on with
 * whatever rule files are already on disk. Sync is not part of — and cannot fail — the three
 * readiness steps above.
 *
 * Why readiness lives here at all: this package declares NO npm dependencies on purpose, because a
 * hard dependency makes the whole `dsh plugin add` fail whenever the registry mirror lags — an
 * install-time failure the user cannot act on. dsh runs no install scripts either, so the package
 * always installs and the CLI it needs is obtained when the plugin loads instead.
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
import { appendFileSync, existsSync, readFileSync, readdirSync } from "node:fs";
import { createRequire } from "node:module";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

export const name = "rulemux";
export const inject = ["agents"];

/**
 * 插件详情页的说明面板（`plugins.bundle.config` 槽位）要显示，宿主必须登记本插件的
 * 设置命名空间 —— 命名空间是 profile entry id（`rulemux`，见 cordis.patch.yml），
 * 不是包名（`rulemux-dsh`）。
 *
 * ⚠ 这里**只有一个占位字段**，界面既不渲染它、也不往里写任何东西：
 * 规则配置的真源始终是 `~/.rulemux/config.toml`（用户自己改），
 * dsh 的设置文档不是它的第二份拷贝。
 */

/**
 * schemastery 只服务于说明面板：拿不到就退化成「没有面板」，核心注入绝不受影响
 * （本包刻意没有硬依赖 —— 依赖装不上不该让整个插件挂掉）。
 */
let Schema = null;
try {
  Schema = (await import("@deepseek-ai/schemastery")).default;
} catch {
  Schema = null;
}

export const Config = Schema
  ? Schema.object({ notice: Schema.string().default("").volatile() })
  : undefined;

const RULES_SUBDIR = ".dsh/rules"; // where `rulemux sync` drops the copies (workspace-relative)
const FILE_PREFIX = "__rulemux__"; // rulemux's ownership prefix: other files are never read
const MANAGED_HEADER = "<!-- rulemux:managed -->"; // marks our injected block (re-injection check)
const SOURCE_KIND = "plugin:rulemux"; // producer-owned source kind dsh expects for injected messages
const CONFIG_REL = [".rulemux", "config.toml"]; // mirrors config.DefaultPath() in the Go CLI

// The CLI version this plugin needs. Kept here — NOT in package.json's dependencies (that would make
// the plugin itself uninstallable when the mirror lags) and NOT in peerDependencies (pnpm may then
// refuse to install it). `>=` means "this or newer"; an exact pin ("0.3.1" / "=0.3.1") means "exactly
// this". Either way an unsatisfied version is upgraded, then re-checked, and failing that we throw.
//
// ⚠ It must be the FIRST RELEASED version whose registry entry ships the dsh adapter: a version floor
// only means anything if everything below it genuinely cannot do the job. The dsh adapter landed
// after v0.3.0, so npm's 0.3.0 (and anything older, e.g. 0.2.7) answers "unknown agent dsh" for
// `--agent dsh` — hence 0.3.1. It also decides what we upgrade *to* (see installTarget), so it is
// bumped in the same change that cuts the release, never afterwards.
const REQUIRED_CLI = ">=0.3.1";

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

/** A version triple as text, for messages ("unknown" when it could not be parsed). */
function versionText(v) {
  return v ? `${v.major}.${v.minor}.${v.patch}` : "unknown version";
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
 *   - a copy installed alongside us as an npm dependency (legacy layout; this package declares none), else
 *   - `rulemux` on PATH.
 */
async function resolveCli() {
  try {
    const pkg = require_.resolve("rulemux/package.json");
    const shim = join(dirname(pkg), "bin", "rulemux.js");
    if (existsSync(shim)) return { cmd: process.execPath, args: [shim], how: `dependency (${pkg})` };
  } catch {
    /* not installed as a dependency (the normal case: this package has no dependencies) */
  }
  const probe = await run("rulemux", ["--version"], { timeoutMs: PROBE_TIMEOUT_MS });
  if (probe.code === 0) {
    return { cmd: "rulemux", args: [], how: "PATH", probeOut: probe.out };
  }
  return null;
}

/** The version the resolved CLI reports (`rulemux X.Y.Z`), or null when it cannot be read. */
async function probeCliVersion(cli) {
  const out =
    cli.probeOut ?? (await run(cli.cmd, [...cli.args, "--version"], { timeoutMs: PROBE_TIMEOUT_MS })).out;
  const m = String(out || "").match(/(\d+)\.(\d+)\.(\d+)/);
  return m ? { major: +m[1], minor: +m[2], patch: +m[3] } : null;
}

/**
 * Does this CLI actually know the `dsh` adapter? A version floor alone is not enough: a binary built
 * with plain `go build` / `go install` reports main.go's hardcoded default version no matter which
 * code it contains, so a perfectly capable CLI can look too old. `--help` lists the supported agent
 * ids straight from the registry, so that list is the honest answer. (Help text is human-facing, but
 * the id column is machine-stable, and the version check stays the fast path.)
 */
async function knowsDsh(cli) {
  const r = await run(cli.cmd, [...cli.args, "--help"], { timeoutMs: PROBE_TIMEOUT_MS });
  return r.code === 0 && /^\s*dsh\s/m.test(`${r.out}${r.err}`);
}

/** Can this CLI do our job? Either its version is new enough, or it demonstrably knows `dsh`. */
async function cliOk(cli) {
  if (!cli) return false;
  if (satisfies(await probeCliVersion(cli), REQUIRED_CLI)) return true;
  return knowsDsh(cli);
}

/** Compare two version triples: negative / 0 / positive, like a comparator. */
function compareVersions(a, b) {
  return a.major - b.major || a.minor - b.minor || a.patch - b.patch;
}

/**
 * Does `v` satisfy `spec`? Supported shapes: ">=1.2.3", ">1.2.3", or an exact pin ("1.2.3" / "=1.2.3").
 * Anything else is NOT satisfied: an unrecognised requirement must fail loudly, never be guessed at.
 * (No semver dependency — three numbers compared pairwise is the whole job here.)
 */
function satisfies(v, spec = REQUIRED_CLI) {
  if (!v) return false;
  const m = String(spec).trim().match(/^(>=|>|=)?\s*(\d+)\.(\d+)\.(\d+)$/);
  if (!m) return false;
  const want = { major: +m[2], minor: +m[3], patch: +m[4] };
  const cmp = compareVersions(v, want);
  if (m[1] === ">=") return cmp >= 0;
  if (m[1] === ">") return cmp > 0;
  return cmp === 0; // "=1.2.3" and a bare "1.2.3" both mean exactly this version
}

/**
 * The package spec to install. A range (">=x.y.z") means "anything at least this new", so we ask for
 * `latest`; an exact pin means that exact version. Either way the version is re-read afterwards and
 * the result decides success — we never assume the install did what we asked.
 */
function installTarget() {
  const m = String(REQUIRED_CLI).trim().match(/^(\d+)\.(\d+)\.(\d+)$/);
  return m ? `rulemux@${m[0]}` : "rulemux@latest";
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
async function installCli(reasons, spec) {
  for (const pm of ["pnpm", "npm"]) {
    const args = pm === "npm" ? ["install", "-g", spec] : ["add", "-g", spec];
    const r = await run(pm, args);
    if (r.error) {
      reasons.push(`${pm}: cannot run (${r.error.message})`);
      continue;
    }
    if (r.code !== 0) {
      reasons.push(`${pm} ${args.join(" ")} → exit ${r.code}: ${tail(r.err || r.out) || "no output"}`);
      continue;
    }
    // Look in both places: PATH may still hold an older copy that shadows what we just installed.
    for (const cli of [await resolveCli(), await cliInGlobalBin(pm)]) {
      if (await cliOk(cli)) return { cli, installedBy: cli.how === "PATH" ? `${pm} -g` : cli.how };
    }
    reasons.push(`${pm}: reported success but no usable rulemux is resolvable afterwards`);
  }
  return null;
}

/**
 * Readiness step ①: a CLI that can actually do the job — its version satisfies REQUIRED_CLI, or it
 * demonstrably knows the `dsh` adapter (see cliOk). A CLI that can do neither is upgraded, never
 * accepted; if the upgrade leaves an unusable one in effect we throw rather than report success.
 */
async function ensureCli(reasons) {
  let cli = await resolveCli();
  if (await cliOk(cli)) return cli;

  const found = cli ? await probeCliVersion(cli) : null;
  if (cli) {
    reasons.push(
      `found an existing rulemux ${versionText(found)} at ${cli.how}, which satisfies neither ` +
        `${REQUIRED_CLI} nor a \`dsh\`-aware CLI`,
    );
  }

  const spec = installTarget();
  const installed = await installCli(reasons, spec);
  if (!installed) {
    throw new Error(
      `rulemux-dsh: the \`rulemux\` CLI (${REQUIRED_CLI}) is required but could not be installed, so ` +
        "this session cannot sync your rules.\n" +
        reasons.map((x) => `  - ${x}`).join("\n") +
        "\n  If you want to install it yourself instead, start a new dsh session afterwards:\n" +
        `    npm install -g ${spec}        # or: pnpm add -g ${spec}`,
    );
  }

  cli = installed.cli;
  if (!(await cliOk(cli))) {
    throw new Error(
      `rulemux-dsh: rulemux ${versionText(await probeCliVersion(cli))} is still the CLI in effect ` +
        `after installing ${spec}: it satisfies neither ${REQUIRED_CLI} nor a \`dsh\`-aware CLI.\n` +
        `  in effect: ${cli.how} (${[cli.cmd, ...cli.args].join(" ")})\n` +
        "  An outdated copy is shadowing the upgrade — remove or upgrade that copy, then start a " +
        "new dsh session.",
    );
  }
  return cli;
}

/**
 * Readiness step ③: the config exists. Missing ⇒ `rulemux init --agent dsh` writes a commented
 * sample; present ⇒ nothing happens (an existing config is never overwritten).
 *
 * (Step ② — "this plugin is loaded" — is implied by this code running at all; there is nothing to
 * check, and no way for it to fail here.)
 */
async function ensureConfig(cli) {
  const cfg = configPath();
  if (existsSync(cfg)) return;
  const r = await run(cli.cmd, [...cli.args, "init", "--agent", "dsh"]);
  if (r.error || r.code !== 0) {
    throw new Error(
      `rulemux-dsh: could not create ${cfg} (\`rulemux init --agent dsh\`).\n` +
        `  ${tail(r.err || r.out) || r.error?.message || `exit ${r.code}`}`,
    );
  }
}

/**
 * The one-time, process-wide readiness chain: ① CLI (with version check) → ③ config. Idempotent by
 * construction (a module-level singleton + an existsSync guard), and it either completes or rejects —
 * there is no partial success to report.
 */
let provisioning = null;

function provisionOnce() {
  if (!provisioning) {
    provisioning = (async () => {
      const reasons = [];
      const cli = await ensureCli(reasons); // ① (throws when unusable)
      await ensureConfig(cli); // ③ (throws when it cannot be created)
      return { cli };
    })();
  }
  return provisioning;
}

/**
 * Sync once for this workspace — NOT part of readiness. Syncing is the same concern for every agent,
 * and it is not ours to fail the session over: a non-zero exit is logged (with the CLI's own stderr)
 * and the session carries on with whatever is already on disk. This is also where a config with no
 * [[source]] yet lands: the CLI exits non-zero, we log it, and nothing is injected.
 */
async function syncOnce(root, cli) {
  const r = await run(cli.cmd, [...cli.args, "sync", "--hook", "--agent", "dsh"], { cwd: root });
  if (r.code !== 0) {
    console.error(
      `rulemux-dsh: \`rulemux sync --hook --agent dsh\` failed in ${root} ` +
        `(exit ${r.code ?? "n/a"}). Continuing with the rule files already on disk.\n` +
        `  ${tail(r.err || r.out) || r.error?.message || "no output"}`,
    );
  }
}

/** Readiness + one sync. Throws only for the three readiness steps — never for sync. */
async function ensureReady(root) {
  const prov = await provisionOnce();
  await syncOnce(root, prov.cli);
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
 * Kick off (or reuse) the one readiness chain for a session, and sync once for its workspace.
 * Awaited — with no upper bound — at the first turn. The copy started here must not reject
 * unobserved, so a no-op catch is attached: the real error is surfaced where it matters, at the
 * first pre-step.
 */
function startReady(state) {
  if (!state.ready) {
    state.ready = ensureReady(state.root);
    state.ready.catch(() => {});
  }
  return state.ready;
}

// TEMP canary — drop once the dsh integration is confirmed on a real machine. It answers the one
// question that cannot be answered from a machine without dsh: does dsh actually call apply()?
// (no file = never called), and it records a load-time readiness failure, which would otherwise stay
// silent until a session's first turn.
function loadCanary(line) {
  try {
    appendFileSync(join(homedir(), ".rulemux-dsh-load.log"), `${new Date().toISOString()} ${line}\n`);
  } catch {
    /* diagnostics must never break the host */
  }
}

export function apply(ctx) {
  // Start the workspace-independent half of readiness NOW. dsh calls apply() at startup — well before
  // any session — so ~/.rulemux/config.toml is already there by the time the user goes to edit it.
  loadCanary("apply() called");
  provisionOnce().catch((err) => {
    // A readiness failure is real and must not be silent: record it where it can be seen. (An
    // unhandled rejection at load could disturb dsh's startup, so we log rather than rethrow; the
    // first pre-step still throws the same error, which is the session-visible half.)
    loadCanary(`readiness failed: ${err?.message ?? err}`);
    console.error(`rulemux-dsh: readiness failed at plugin load: ${err?.message ?? err}`);
  });

  ctx.on("agent/session-start", ({ agent }) => {
    loadCanary("agent/session-start");
    const state = stateFor(agent);
    if (state) startReady(state); // fallback (if apply was never called) + the per-session sync
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

    // The three readiness steps are a hard prerequisite. A rejection here is meant to surface as a
    // visible session error (see the file header) — so it is deliberately not caught.
    await startReady(state);

    const text = readRules(state.root);
    if (!text) {
      state.injected = true; // nothing to inject: do not re-scan on every later turn
      return decision;
    }
    state.injected = true;
    return { kind: "enter", messages: [...decision.messages, injectionMessage(text)] };
  });

  ctx.on("agent/disposed", ({ agent }) => {
    const id = agent?.session?.header?.id;
    if (id) sessions.delete(id);
  });
}

export default { name, inject, apply };
