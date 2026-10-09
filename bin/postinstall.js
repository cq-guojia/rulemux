#!/usr/bin/env node
"use strict";

// npm postinstall: bring rulemux's own SessionStart hooks up to date after a
// global install/upgrade.
//
// Why: a hook command can change between releases (0.2.0 added `--hook`). Users
// who upgraded but never re-ran `init` would silently keep the old behaviour,
// so the upgrade itself refreshes what it previously wrote.
//
// Deliberate limits — this script must never surprise anyone:
//   - global installs only: a project-local `npm i rulemux` must not touch the
//     user's host config;
//   - skipped under CI, sudo and --ignore-scripts: $HOME would point at the
//     wrong user, or the run is not a real user's machine;
//   - it never installs anything new: only hooks rulemux itself already wrote;
//   - it never fails the install: every error path exits 0, silently.
//
// Non-npm installs (go install, a binary from GitHub Releases) have no
// postinstall. Those users run `rulemux init --refresh` themselves, and
// `rulemux doctor` reports hooks that are installed but outdated.

const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

function platformKey() {
  const plat = process.platform === "win32" ? "windows" : process.platform;
  let arch = process.arch;
  if (arch === "x64") arch = "amd64";
  else if (arch === "ia32") arch = "386";
  return `${plat}-${arch}`;
}

// Same resolution order as bin/rulemux.js: a bundled binary first, then a
// `rulemux` already on PATH.
function resolveBinary() {
  const ext = process.platform === "win32" ? ".exe" : "";
  const key = platformKey();
  const candidates = [
    path.join(__dirname, "..", "dist", `rulemux-${key}${ext}`),
    path.join(__dirname, "..", "dist", `rulemux-${process.platform}-${process.arch}${ext}`),
  ];
  for (const c of candidates) {
    if (fs.existsSync(c)) return c;
  }
  const which = process.platform === "win32" ? "where" : "which";
  const r = spawnSync(which, ["rulemux"], { stdio: "ignore" });
  return r.status === 0 ? "rulemux" : null;
}

function shouldRun() {
  if (process.env.npm_config_global !== "true") return false;
  if (process.env.npm_config_ignore_scripts === "true") return false;
  if (process.env.CI) return false;
  if (process.env.SUDO_USER) return false;
  return true;
}

function main() {
  if (!shouldRun()) return;

  const bin = resolveBinary();
  if (!bin) return;

  const r = spawnSync(bin, ["init", "--refresh"], { encoding: "utf8" });
  if (r.error || r.status !== 0 || !r.stdout) return;

  // Stay quiet unless something actually changed: "refreshed:" marks a rewrite.
  const changed = r.stdout
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.includes("refreshed:"));
  if (changed.length === 0) return;

  process.stdout.write(
    "rulemux: upgraded the SessionStart hook(s) rulemux had already installed\n" +
      changed.map((line) => "  " + line).join("\n") +
      "\n"
  );
}

try {
  main();
} catch (_) {
  // Never let a hook refresh break `npm install`.
}
