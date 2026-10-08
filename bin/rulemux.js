#!/usr/bin/env node
"use strict";

// npm launcher for rulemux.
//
// rulemux itself is a single Go binary with no runtime dependencies. This shim
// exists only so that `npm i -g rulemux` puts a working `rulemux` command on your
// PATH. It resolves the real binary in this order:
//
//   1. a binary bundled for this platform in ../dist/ (produced by scripts/build-dist.sh)
//   2. a `rulemux` binary already on your PATH (e.g. installed from a GitHub Release)
//   3. otherwise: print install instructions and exit non-zero
//
// It never rewrites your arguments and always forwards the child's exit code.

const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawnSync } = require("child_process");

function platformKey() {
  const plat = process.platform === "win32" ? "windows" : process.platform;
  let arch = process.arch;
  if (arch === "x64") arch = "amd64";
  else if (arch === "ia32") arch = "386";
  return `${plat}-${arch}`;
}

function bundledCandidates() {
  const ext = process.platform === "win32" ? ".exe" : "";
  const key = platformKey();
  return [
    path.join(__dirname, "..", "dist", `rulemux-${key}${ext}`),
    // Some cross-builds use the Go default naming; keep a couple of aliases.
    path.join(__dirname, "..", "dist", `rulemux-${process.platform}-${process.arch}${ext}`),
  ];
}

function onPath() {
  const cmd = process.platform === "win32" ? "where" : "which";
  const r = spawnSync(cmd, ["rulemux"], { stdio: "ignore" });
  return r.status === 0 ? "rulemux" : null;
}

function resolve() {
  for (const c of bundledCandidates()) {
    if (fs.existsSync(c)) return c;
  }
  return onPath();
}

function main() {
  const bin = resolve();
  if (!bin) {
    process.stderr.write(
      "rulemux: no rulemux binary found.\n" +
        "\n" +
        "Install one of these first:\n" +
        "  go install github.com/cq-guojia/rulemux@latest\n" +
        "  or download a binary from https://github.com/cq-guojia/rulemux/releases\n" +
        "  and make sure it is on your PATH.\n"
    );
    process.exit(1);
  }

  // npm tarballs do not always preserve the executable bit; be defensive.
  if (process.platform !== "win32" && fs.existsSync(bin)) {
    try {
      fs.chmodSync(bin, 0o755);
    } catch (_) {
      /* ignore: it may already be fine, or the fs may be read-only */
    }
  }

  const r = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
  if (r.error) {
    process.stderr.write(`rulemux: failed to run ${bin}: ${r.error.message}\n`);
    process.exit(1);
  }
  process.exit(r.status === null ? 1 : r.status);
}

main();
