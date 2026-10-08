#!/usr/bin/env bash
# rulemux smoke test: end-to-end check of init / sync / doctor / verify / uninstall.
#
# Usage: scripts/smoke.sh
#   Uses $REPO_ROOT/rulemux if present, otherwise builds it.
#
# Scope note: only *verified* agents can be installed, which today means
# codebuddy (and workbuddy, which shares the same rules directory). Agents such
# as claude / trae / codex / opencode are registered but NOT verified yet, so they
# are deliberately excluded here - their coverage arrives with their adapter.
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
RULES_REL=".codebuddy/rules"          # codebuddy & workbuddy share this directory
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
grep -q '"rulemux"' "$HOOK_CFG" || fail "hook was not written to $HOOK_CFG"
ok "init installs the hook into the user-level host config"

# Idempotent: initializing again must not add a second rulemux hook
before="$(grep -o '"rulemux"' "$HOOK_CFG" | wc -l)"
"$BIN" init --agent "$AGENT" --config "$CFG" --workspace "$WS" >/dev/null 2>&1
after="$(grep -o '"rulemux"' "$HOOK_CFG" | wc -l)"
[ "$before" = "$after" ] || fail "init is not idempotent: $before -> $after"
ok "init is idempotent"

# 3. sync: files land in the rules dir carrying the .rulemux__ prefix
"$BIN" sync --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync1.log" 2>&1 || fail "sync exited non-zero"
[ -f "$WS/$RULES_REL/.rulemux__a.md" ]  || fail "missing .rulemux__a.md"
[ -f "$WS/$RULES_REL/.rulemux__b.txt" ] || fail "missing .rulemux__b.txt"
ok "sync drops prefixed files into the rules dir"

# 4. Idempotent: a second sync skips everything
"$BIN" sync --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync2.log" 2>&1
grep -q "Unchanged, skipped" "$TMP/sync2.log" || fail "second sync did not skip (not idempotent)"
if grep -q "Added:" "$TMP/sync2.log"; then fail "second sync added files again"; fi
ok "sync is idempotent (skips unchanged files)"

# 5. The user's own files are never touched
echo "mine" > "$WS/$RULES_REL/my-own.md"
"$BIN" sync --agent "$AGENT" --config "$CFG" --workspace "$WS" >/dev/null 2>&1
[ -f "$WS/$RULES_REL/my-own.md" ] || fail "the user's own file was deleted"
ok "user files are left alone"

# 6. Changing the source overwrites the copy
printf 'rule A v2\n' > "$TMP/src/a.md"
"$BIN" sync --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync3.log" 2>&1
grep -q "Updated:" "$TMP/sync3.log" || fail "content change did not trigger an update"
grep -q "rule A v2" "$WS/$RULES_REL/.rulemux__a.md" || fail "updated content is wrong"
ok "source change overwrites the copy"

# 7. Dropping a source removes its residue (free to add and remove)
cat > "$CFG" <<EOF
[[source]]
path = ["$TMP/src/a.md"]
EOF
"$BIN" sync --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync4.log" 2>&1
grep -q "Removed residue" "$TMP/sync4.log" || fail "residue was not removed"
[ ! -f "$WS/$RULES_REL/.rulemux__b.txt" ] || fail "residue .rulemux__b.txt still present"
[ -f "$WS/$RULES_REL/.rulemux__a.md" ]    || fail ".rulemux__a.md was wrongly deleted"
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
[ -f "$WS/$RULES_REL/.rulemux__canary.md" ] || fail "canary was not written"
"$BIN" verify --agent "$AGENT" --clean --workspace "$WS" >/dev/null 2>&1
[ ! -f "$WS/$RULES_REL/.rulemux__canary.md" ] || fail "canary was not cleaned"
ok "verify writes and cleans the canary"

# 11. Workspace matching: only workspaces declared in the config receive files
mkdir -p "$TMP/wsA" "$TMP/wsB"
cat > "$TMP/ws.toml" <<EOF
[[source]]
path = ["$TMP/src/a.md"]
workspace = ["$TMP/wsA"]
EOF
# wsB is not in the declared list => nothing should land there
"$BIN" sync --agent "$AGENT" --config "$TMP/ws.toml" --workspace "$TMP/wsB" >/dev/null 2>&1
[ ! -f "$TMP/wsB/$RULES_REL/.rulemux__a.md" ] || fail "an undeclared workspace received files"
# wsA is declared => files land there
"$BIN" sync --agent "$AGENT" --config "$TMP/ws.toml" --workspace "$TMP/wsA" >/dev/null 2>&1
[ -f "$TMP/wsA/$RULES_REL/.rulemux__a.md" ] || fail "the declared workspace did not receive files"
ok "workspace matching"

# 12. Ledger: uninstalling from one workspace also sweeps the other recorded ones.
# Prior steps synced into $WS, wsA and wsB, so all three are in the ledger.
"$BIN" uninstall --agent "$AGENT" --workspace "$TMP/wsB" --yes >"$TMP/uninstall.log" 2>&1 || fail "uninstall exited non-zero"
[ ! -f "$TMP/wsB/$RULES_REL/.rulemux__a.md" ] || fail "wsB residue was not removed"
[ ! -f "$TMP/wsA/$RULES_REL/.rulemux__a.md" ] || fail "ledger sweep missed wsA"
[ ! -f "$WS/$RULES_REL/.rulemux__a.md" ]     || fail "ledger sweep missed $WS"
[ -f "$WS/$RULES_REL/my-own.md" ]            || fail "uninstall deleted the user's own file"
ok "uninstall sweeps every recorded workspace (user files kept)"

echo
echo "ALL SMOKE TESTS PASSED"
