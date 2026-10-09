#!/usr/bin/env bash
# rulemux smoke test: end-to-end check of init / sync / doctor / verify / uninstall.
#
# Usage: scripts/smoke.sh
#   Uses $REPO_ROOT/rulemux if present, otherwise builds it.
#
# Scope note: only *verified* agents can be installed, which today means
# codebuddy (hook ~/.codebuddy/settings.json, rules .codebuddy/rules) and
# workbuddy (separate agent since 2026-10-09: hook ~/.workbuddy/settings.json,
# rules .workbuddy/rules). Agents such as claude / trae / codex / opencode are
# registered but NOT verified yet, so they are deliberately excluded here -
# their coverage arrives with their adapter.
#
# Isolation note: HOME is redirected to a temp dir, so the user-level hook config
# written by init never touches the real ~/.codebuddy/settings.json.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

BIN="$REPO_ROOT/rulemux"
if [ ! -x "$BIN" ]; then
  go build -o "$BIN" .
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Critical: point HOME at a temp dir so init writes its user-level hook there.
export HOME="$TMP/home"
mkdir -p "$HOME"

WS="$TMP/ws"
CFG="$TMP/config.toml"
mkdir -p "$WS" "$TMP/src"

AGENT=codebuddy
RULES_REL=".codebuddy/rules"          # codebuddy only; workbuddy uses .workbuddy/rules (separate agent)
HOOK_CFG="$HOME/.codebuddy/settings.json"  # hooks live in the user-level host config

fail() { echo "FAIL: $*" >&2; exit 1; }
ok()   { echo "ok - $*"; }

# 1. Prepare source files and config
printf 'rule A v1\n' > "$TMP/src/a.md"
printf 'rule B\n'    > "$TMP/src/b.txt"
cat > "$CFG" <<EOF
[[source]]
path = ["$TMP/src/a.md", "$TMP/src/b.txt"]
EOF

# 2. init: reuse the existing config, and install the hook into the user-level host config
"$BIN" init --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/init.log" 2>&1 || fail "init exited non-zero"
grep -q "config exists" "$TMP/init.log" || fail "init did not reuse the existing config"
grep -q 'rulemux sync' "$HOOK_CFG" || fail "hook was not written to $HOOK_CFG"
ok "init installs the hook into the user-level host config"

# Idempotent: initializing again must not add a second rulemux hook
before="$(grep -o 'rulemux sync' "$HOOK_CFG" | wc -l)"
"$BIN" init --agent "$AGENT" --config "$CFG" --workspace "$WS" >/dev/null 2>&1
after="$(grep -o 'rulemux sync' "$HOOK_CFG" | wc -l)"
[ "$before" = "$after" ] || fail "init is not idempotent: $before -> $after"
ok "init is idempotent"

# 3. sync: files land in the rules dir carrying the __rulemux__ prefix
"$BIN" sync --hook --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync1.log" 2>&1 || fail "sync exited non-zero"
[ -f "$WS/$RULES_REL/__rulemux__a.md" ]  || fail "missing __rulemux__a.md"
[ -f "$WS/$RULES_REL/__rulemux__b.txt" ] || fail "missing __rulemux__b.txt"
ok "sync drops prefixed files into the rules dir"

# 4. Idempotent: a second sync skips everything
"$BIN" sync --hook --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync2.log" 2>&1
grep -q "Unchanged, skipped" "$TMP/sync2.log" || fail "second sync did not skip (not idempotent)"
if grep -q "Added:" "$TMP/sync2.log"; then fail "second sync added files again"; fi
ok "sync is idempotent (skips unchanged files)"

# 5. The user's own files are never touched
echo "mine" > "$WS/$RULES_REL/my-own.md"
"$BIN" sync --hook --agent "$AGENT" --config "$CFG" --workspace "$WS" >/dev/null 2>&1
[ -f "$WS/$RULES_REL/my-own.md" ] || fail "the user's own file was deleted"
ok "user files are left alone"

# 6. Changing the source overwrites the copy
printf 'rule A v2\n' > "$TMP/src/a.md"
"$BIN" sync --hook --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync3.log" 2>&1
grep -q "Updated:" "$TMP/sync3.log" || fail "content change did not trigger an update"
grep -q "rule A v2" "$WS/$RULES_REL/__rulemux__a.md" || fail "updated content is wrong"
ok "source change overwrites the copy"

# 7. Dropping a source removes its residue (free to add and remove)
cat > "$CFG" <<EOF
[[source]]
path = ["$TMP/src/a.md"]
EOF
"$BIN" sync --hook --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync4.log" 2>&1
grep -q "Removed residue" "$TMP/sync4.log" || fail "residue was not removed"
[ ! -f "$WS/$RULES_REL/__rulemux__b.txt" ] || fail "residue __rulemux__b.txt still present"
[ -f "$WS/$RULES_REL/__rulemux__a.md" ]    || fail "__rulemux__a.md was wrongly deleted"
ok "residue is cleaned up"

# 8. Unverified agents stay switched off (the feature switch must hold)
if "$BIN" init --agent codex --config "$CFG" --workspace "$WS" >"$TMP/codex.log" 2>&1; then
  fail "init should refuse the unverified codex agent"
fi
grep -q "not installable yet" "$TMP/codex.log" || fail "init did not explain why codex was refused"
if "$BIN" inject --agent codex --config "$CFG" >"$TMP/inject.log" 2>&1; then
  fail "inject should refuse the unverified codex agent"
fi
ok "unverified agents are refused"

# 9. doctor runs and lists every registered agent
"$BIN" doctor --config "$CFG" --workspace "$WS" >"$TMP/doctor.log" 2>&1 || fail "doctor exited non-zero"
grep -q "$AGENT" "$TMP/doctor.log" || fail "doctor did not list $AGENT"
ok "doctor self-check"

# 10. verify canary: written into the rules dir, then removed by --clean
"$BIN" verify --agent "$AGENT" --workspace "$WS" >"$TMP/verify.log" 2>&1
[ -f "$WS/$RULES_REL/__rulemux__canary.md" ] || fail "canary was not written"
"$BIN" verify --agent "$AGENT" --clean --workspace "$WS" >/dev/null 2>&1
[ ! -f "$WS/$RULES_REL/__rulemux__canary.md" ] || fail "canary was not cleaned"
ok "verify writes and cleans the canary"

# 11. Workspace matching: only workspaces declared in the config receive files
mkdir -p "$TMP/wsA" "$TMP/wsB"
cat > "$TMP/ws.toml" <<EOF
[[source]]
path = ["$TMP/src/a.md"]
workspace = ["$TMP/wsA"]
EOF
# wsB is not in the declared list => nothing should land there
"$BIN" sync --hook --agent "$AGENT" --config "$TMP/ws.toml" --workspace "$TMP/wsB" >/dev/null 2>&1
[ ! -f "$TMP/wsB/$RULES_REL/__rulemux__a.md" ] || fail "an undeclared workspace received files"
# ...and it must NOT wipe what is already there: an undeclared workspace is skipped
# entirely (no create / no delete / no ledger write). Safety regression check.
mkdir -p "$TMP/wsB/$RULES_REL"
echo "stale" > "$TMP/wsB/$RULES_REL/__rulemux__stale.md"
"$BIN" sync --hook --agent "$AGENT" --config "$TMP/ws.toml" --workspace "$TMP/wsB" >/dev/null 2>&1
[ -f "$TMP/wsB/$RULES_REL/__rulemux__stale.md" ] || fail "an undeclared workspace was wiped"
# wsA is declared => files land there
"$BIN" sync --hook --agent "$AGENT" --config "$TMP/ws.toml" --workspace "$TMP/wsA" >/dev/null 2>&1
[ -f "$TMP/wsA/$RULES_REL/__rulemux__a.md" ] || fail "the declared workspace did not receive files"
ok "workspace matching"

# 12. Ledger: uninstalling from one workspace also sweeps the other recorded ones.
# Prior steps synced into $WS and wsA, so both are in the ledger (wsB is undeclared
# and therefore skipped => never recorded).
"$BIN" uninstall --agent "$AGENT" --workspace "$TMP/wsB" --yes >"$TMP/uninstall.log" 2>&1 || fail "uninstall exited non-zero"
[ ! -f "$TMP/wsB/$RULES_REL/__rulemux__a.md" ] || fail "wsB residue was not removed"
[ ! -f "$TMP/wsA/$RULES_REL/__rulemux__a.md" ] || fail "ledger sweep missed wsA"
[ ! -f "$WS/$RULES_REL/__rulemux__a.md" ]     || fail "ledger sweep missed $WS"
[ -f "$WS/$RULES_REL/my-own.md" ]            || fail "uninstall deleted the user's own file"
ok "uninstall sweeps every recorded workspace (user files kept)"

# 13. Refresh (the upgrade self-heal): rewrite ONLY rulemux's own hook entry.
# This is what makes "upgrade the binary" bring old hooks up to date, and it is
# where the hard rule lives: nothing but our own entry may change.
HOOKHOME="$TMP/hookhome"
mkdir -p "$HOOKHOME/.codebuddy"
cat > "$HOOKHOME/.codebuddy/settings.json" <<'EOF'
{
  "theme": "dark",
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "node \"/other-tool.js\""}]},
      {"matcher": "", "hooks": [{"type": "command", "command": "rulemux sync --agent codebuddy"}]}
    ]
  }
}
EOF
HOME="$HOOKHOME" "$BIN" init --refresh >"$TMP/refresh.log" 2>&1 || fail "init --refresh exited non-zero"
REFRESHED="$HOOKHOME/.codebuddy/settings.json"
grep -qF 'rulemux sync --hook --agent codebuddy' "$REFRESHED" || fail "old hook was not refreshed to the current format"
grep -qF 'node \"/other-tool.js\"' "$REFRESHED" || fail "another tool's hook was damaged"
grep -qF '"theme": "dark"' "$REFRESHED" || fail "an unrelated key was dropped"
ok "refresh upgrades only rulemux's own hook entry"

# ...and a second refresh is a no-op: not even a byte is rewritten
sum_before="$(cksum < "$REFRESHED")"
HOME="$HOOKHOME" "$BIN" init --refresh >"$TMP/refresh2.log" 2>&1
sum_after="$(cksum < "$REFRESHED")"
[ "$sum_before" = "$sum_after" ] || fail "second refresh rewrote an already-current file"
grep -q "refreshed 0 hook(s)" "$TMP/refresh2.log" || fail "second refresh did not report 0 changes"
ok "refresh is idempotent (current hooks are never rewritten)"

# ...and it never conjures a host config where none existed
mkdir -p "$TMP/emptyhome"
HOME="$TMP/emptyhome" "$BIN" init --refresh >/dev/null 2>&1 || fail "init --refresh must always exit 0"
[ ! -e "$TMP/emptyhome/.codebuddy" ] || fail "refresh created a host config directory out of nowhere"
[ ! -e "$TMP/emptyhome/.rulemux" ]   || fail "refresh created the rulemux home directory"
ok "refresh never creates host config"

# 14. CODEBUDDY_CONFIG_DIR: a relocated config dir must be followed, not ignored
MOVED="$TMP/moved-cfg"
mkdir -p "$MOVED"
cat > "$MOVED/settings.json" <<'EOF'
{"hooks": {"SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "rulemux sync --agent codebuddy"}]}]}}
EOF
HOME="$TMP/emptyhome" CODEBUDDY_CONFIG_DIR="$MOVED" "$BIN" init --refresh >"$TMP/moved.log" 2>&1 || fail "refresh with CODEBUDDY_CONFIG_DIR exited non-zero"
grep -qF 'rulemux sync --hook --agent codebuddy' "$MOVED/settings.json" || fail "the relocated config dir was ignored"
grep -qF '[dir from $CODEBUDDY_CONFIG_DIR]' "$TMP/moved.log" || fail "refresh did not report the env-provided path"
[ ! -e "$TMP/emptyhome/.codebuddy" ] || fail "the default config dir was touched although the env var was set"
ok "CODEBUDDY_CONFIG_DIR is honoured"

# 15. doctor must flag an OUTDATED hook (it used to print a plain ✓ for any file
# that merely mentioned "rulemux", so an old-format hook looked fine)
cat > "$HOOKHOME/.codebuddy/settings.json" <<'EOF'
{"hooks": {"SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "rulemux sync --agent codebuddy"}]}]}}
EOF
HOME="$HOOKHOME" "$BIN" doctor --workspace "$WS" >"$TMP/doctor2.log" 2>&1 || fail "doctor exited non-zero"
grep -q "installed but OUTDATED" "$TMP/doctor2.log" || fail "doctor did not flag the outdated hook"
grep -q "rulemux init --refresh" "$TMP/doctor2.log" || fail "doctor did not point at the fix"
ok "doctor flags outdated hooks"

# 16. Hook self-heal — no npm install scripts involved at all: a plain `sync` must
# rewrite our own outdated hook entry, and touch nothing else.
cat > "$HOOKHOME/.codebuddy/settings.json" <<'EOF'
{
  "theme": "dark",
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "node \"/other.js\""}]},
      {"matcher": "", "hooks": [{"type": "command", "command": "rulemux sync --agent codebuddy"}]}
    ]
  }
}
EOF
HOME="$HOOKHOME" "$BIN" sync --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/heal.log" 2>&1 \
  || fail "sync exited non-zero while self-healing"
grep -qF 'rulemux sync --hook --agent codebuddy' "$HOOKHOME/.codebuddy/settings.json" \
  || fail "sync did not self-heal the outdated hook"
grep -qF 'node \"/other.js\"' "$HOOKHOME/.codebuddy/settings.json" \
  || fail "self-heal damaged another tool's hook"
grep -qF '"theme": "dark"' "$HOOKHOME/.codebuddy/settings.json" \
  || fail "self-heal dropped an unrelated key"
grep -q "updated the SessionStart hook" "$TMP/heal.log" \
  || fail "self-heal did not report the update on stderr"
ok "sync self-heals an outdated hook (no npm scripts involved)"

# ...and it never conjures a host config where none exists
rm -rf "$HOOKHOME/.codebuddy"
HOME="$HOOKHOME" "$BIN" sync --agent "$AGENT" --config "$CFG" --workspace "$WS" >/dev/null 2>&1
[ ! -e "$HOOKHOME/.codebuddy" ] || fail "self-heal created a host config out of nowhere"
ok "self-heal never creates host config"

echo
echo "ALL SMOKE TESTS PASSED"
