/**
 * rulemux-dsh.js — a minimal native Cordis plugin for DeepSeek Harness (dsh).
 *
 * dsh is plugin-first: it loads this file from $DSH_HOME/cordis.patch.yml and exposes typed
 * lifecycle events. rulemux already copies the user's rule files into <cwd>/.dsh/rules/__rulemux__*.md
 * (real file copies, never symlinks); this plugin only READS them back and injects them into the
 * session's context, once, so the agent sees them exactly as a host that natively scans a rules dir
 * would. Token cost is identical to such a host: the same rule text ends up in context.
 *
 * It imports nothing from dsh: every host shape is structurally typed here, so any dsh whose event
 * names still match can load it (same approach as hindsight's coding-agents src/dsh.ts).
 *
 * Why inject ONCE, not every turn: the rules are static, so re-adding them per turn would duplicate
 * the same text in history and waste tokens. We inject on the first user-prompt step of a session and
 * leave the block in history. If compaction later drops it, we re-inject on the next user turn — but
 * only when we can POSITIVELY tell the host had been holding our block (see stillInContext).
 */

import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

export const name = "rulemux";
export const inject = ["agents"];

const RULES_SUBDIR = ".dsh/rules"; // where `rulemux sync` drops the copies (workspace-relative)
const FILE_PREFIX = "__rulemux__"; // rulemux's ownership prefix: other files are never read
const MANAGED_HEADER = "<!-- rulemux:managed -->"; // marks our injected block (re-injection check)
const SOURCE_KIND = "plugin:rulemux"; // producer-owned source kind dsh expects for injected messages

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
  if (parts.length === 0) return "";
  return `${MANAGED_HEADER}\n\n${parts.join("\n\n")}`;
}

/**
 * Run `rulemux sync` once so the on-disk copies are current before we read them.
 *
 * Fire-and-store: the returned promise is awaited by the first pre-step. Missing binary (not on
 * PATH) is not fatal — we simply read whatever is already on disk.
 */
function runSync(root) {
  return new Promise((resolve) => {
    let settled = false;
    const done = () => {
      if (!settled) {
        settled = true;
        resolve();
      }
    };
    try {
      const child = spawn("rulemux", ["sync", "--hook", "--agent", "dsh"], {
        cwd: root,
        stdio: "ignore",
      });
      child.on("error", done);
      child.on("close", done);
    } catch {
      done();
    }
  });
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

/** Per-session state, built once per session id (the plugin process serves many sessions). */
const sessions = new Map(); // sessionId -> { root, sync, injected, observed }

function stateFor(agent) {
  const id = agent?.session?.header?.id;
  if (!id) return undefined;
  let s = sessions.get(id);
  if (!s) {
    const root = workspaceRoot(agent);
    s = { root, sync: runSync(root), injected: false, observed: false };
    sessions.set(id, s);
  }
  return s;
}

export function apply(ctx) {
  // One sync per session; the first pre-step awaits it before reading.
  ctx.on("agent/session-start", ({ agent }) => {
    stateFor(agent);
  });

  ctx.on("agent/pre-step", async ({ agent, signal }, next) => {
    const decision = await next();
    if (decision.kind !== "enter" || signal?.aborted) return decision;

    const state = stateFor(agent);
    if (!state) return decision;

    // Only a turn claiming new human input is worth (re)injecting on; tool continuations carry none.
    const hasUserInput = (decision.messages || []).some(
      (m) => m?.source?.kind === "user" && (m.content || []).some((b) => b?.type === "text" && b.text)
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

    await state.sync; // ensure the files are on disk before reading
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
