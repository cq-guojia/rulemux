#!/usr/bin/env bash
# rulemux smoke test: end-to-end check of init / sync / doctor / verify / uninstall.
#
# Usage: scripts/smoke.sh
#   Uses $REPO_ROOT/rulemux if present, otherwise builds it.
#
# Scope note: only *verified* agents can be installed, which today means
# codebuddy (hook ~/.codebuddy/settings.json, rules .codebuddy/rules),
# workbuddy (hook ~/.workbuddy/settings.json, rules .codebuddy/rules — SHARED
# with codebuddy since the 2026-10-09 probe), trae (hook ~/.trae-cn/hooks.json,
# rules .trae/rules) and dsh (plugin-first: no hook; RULES are injection-only
# since 2026-10-14 — nothing on disk; SKILLS are real copies into .dsh/skills
# since 2026-10-14, synced by the plugin's session-start `sync --hook --agent dsh`).
# Agents such as claude / codex / opencode are registered but NOT verified yet,
# so they are deliberately excluded here - their coverage arrives with their
# adapter.
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
RULES_REL=".codebuddy/rules"          # codebuddy and workbuddy SHARE this dir (probe-verified 2026-10-09)
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
# Report format (2026-10-13 聚合版式): per-agent lines "rules N · skills M", a changed
# block gets a glyph header with a "rules +N / ~N / −N" suffix, and the closing Summary
# table carries the totals row "rules <files> <+added> <−removed> <~updated>".
# Assert on the ASCII totals row only — all four counters zero IS the proof of
# idempotency, and it keeps the check free of locale-dependent glyph matching.
"$BIN" sync --hook --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync2.log" 2>&1
grep -Eq 'rules[[:space:]]+2[[:space:]]+0[[:space:]]+0[[:space:]]+0' "$TMP/sync2.log" \
  || fail "second sync was not idempotent (totals row)"
ok "sync is idempotent (skips unchanged files)"

# 5. The user's own files are never touched
echo "mine" > "$WS/$RULES_REL/my-own.md"
"$BIN" sync --hook --agent "$AGENT" --config "$CFG" --workspace "$WS" >/dev/null 2>&1
[ -f "$WS/$RULES_REL/my-own.md" ] || fail "the user's own file was deleted"
ok "user files are left alone"

# 6. Changing the source overwrites the copy
printf 'rule A v2\n' > "$TMP/src/a.md"
"$BIN" sync --hook --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync3.log" 2>&1
# totals row: files=2 +added=0 −removed=0 ~updated=1
grep -Eq 'rules[[:space:]]+2[[:space:]]+0[[:space:]]+0[[:space:]]+1' "$TMP/sync3.log" \
  || fail "content change did not trigger an update"
grep -q "rule A v2" "$WS/$RULES_REL/__rulemux__a.md" || fail "updated content is wrong"
ok "source change overwrites the copy"

# 7. Dropping a source removes its residue (free to add and remove)
cat > "$CFG" <<EOF
[[source]]
path = ["$TMP/src/a.md"]
EOF
"$BIN" sync --hook --agent "$AGENT" --config "$CFG" --workspace "$WS" >"$TMP/sync4.log" 2>&1
# totals row: files=1 +added=0 −removed=1 ~updated=0
grep -Eq 'rules[[:space:]]+1[[:space:]]+0[[:space:]]+1[[:space:]]+0' "$TMP/sync4.log" \
  || fail "residue was not removed"
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
# and therefore skipped => never recorded). Make that precondition explicit instead of
# relying on earlier steps having recorded them.
grep -qF "$WS" "$HOME/.rulemux/workspaces.json" || fail "precondition: $WS was not recorded in the ledger"
grep -qF "$TMP/wsA" "$HOME/.rulemux/workspaces.json" || fail "precondition: wsA was not recorded in the ledger"
# --config is required here: pruning needs the config to know what is still in use, and an
# unreadable config deliberately leaves every file untouched (safety guard, see uninstall.go).
"$BIN" uninstall --agent "$AGENT" --config "$CFG" --workspace "$TMP/wsB" --yes >"$TMP/uninstall.log" 2>&1 || fail "uninstall exited non-zero"
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
grep -q "hook OUTDATED" "$TMP/doctor2.log" || fail "doctor did not flag the outdated hook"
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

# 17. Skills: [[skill_source]] syncs whole skill trees into .codebuddy/skills
mkdir -p "$TMP/skills/demo-skill/references"
printf 'name: demo\n'   > "$TMP/skills/demo-skill/SKILL.md"
printf 'reference\n'    > "$TMP/skills/demo-skill/references/ref.md"
mkdir -p "$TMP/skills/not-a-skill"   # no SKILL.md => not a skill, must be ignored
cat > "$TMP/skills.toml" <<EOF
[[source]]
path = ["$TMP/src/a.md"]

[[skill_source]]
path = ["$TMP/skills"]
EOF
"$BIN" sync --hook --agent "$AGENT" --config "$TMP/skills.toml" --workspace "$WS" >"$TMP/skill1.log" 2>&1 || fail "skill sync exited non-zero"
[ -f "$WS/$RULES_REL/__rulemux__a.md" ] || fail "rules missing after skill sync"
[ -f "$WS/.codebuddy/skills/__rulemux__demo-skill/SKILL.md" ] || fail "skill tree not synced"
[ -f "$WS/.codebuddy/skills/__rulemux__demo-skill/references/ref.md" ] || fail "skill附属文件未同步"
[ ! -e "$WS/.codebuddy/skills/__rulemux__not-a-skill" ] || fail "a subdir without SKILL.md was synced"
ok "[[skill_source]] syncs skill trees into .codebuddy/skills"

# 18. Skills: content updates propagate, residue is cleaned, the user's own skills stay
echo "mine" > "$WS/.codebuddy/skills/my-own.md"
"$BIN" sync --hook --agent "$AGENT" --config "$TMP/skills.toml" --workspace "$WS" >/dev/null 2>&1
printf 'name: demo v2\n' > "$TMP/skills/demo-skill/SKILL.md"
"$BIN" sync --hook --agent "$AGENT" --config "$TMP/skills.toml" --workspace "$WS" >/dev/null 2>&1
grep -q "demo v2" "$WS/.codebuddy/skills/__rulemux__demo-skill/SKILL.md" || fail "skill content was not updated"
rm -rf "$TMP/skills/demo-skill"
"$BIN" sync --hook --agent "$AGENT" --config "$TMP/skills.toml" --workspace "$WS" >/dev/null 2>&1
[ ! -e "$WS/.codebuddy/skills/__rulemux__demo-skill" ] || fail "skill residue was not cleaned"
[ -f "$WS/.codebuddy/skills/my-own.md" ] || fail "the user's own skill file was deleted"
ok "skills: updates propagate, residue is cleaned, user files are kept"

# 19. scope=user: rules AND skills land in BOTH user-level dirs (~/.codebuddy + ~/.workbuddy)
mkdir -p "$HOME/.codebuddy/rules" "$HOME/.codebuddy/skills" "$HOME/.workbuddy/rules" "$HOME/.workbuddy/skills"
mkdir -p "$TMP/skills2/global-skill"
printf 'name: global\n' > "$TMP/skills2/global-skill/SKILL.md"
cat > "$TMP/user.toml" <<EOF
[[source]]
path = ["$TMP/src/a.md"]
scope = "user"

[[skill_source]]
path = ["$TMP/skills2"]
scope = "user"
EOF
"$BIN" sync --agent "$AGENT" --config "$TMP/user.toml" --workspace "$WS" >"$TMP/user.log" 2>&1 || fail "user-scope sync exited non-zero"
for base in .codebuddy .workbuddy; do
  [ -f "$HOME/$base/rules/__rulemux__a.md" ] || fail "user-level rules missing in $HOME/$base"
  [ -f "$HOME/$base/skills/__rulemux__global-skill/SKILL.md" ] || fail "user-level skills missing in $HOME/$base"
done
ok "scope=user double-writes rules and skills to both user-level dirs"

# 20. Uninstall converges the user-level dirs (prefix content only; the dirs stay)
echo "keepme" > "$HOME/.codebuddy/rules/my-own.md"
"$BIN" uninstall --agent "$AGENT" --config "$TMP/user.toml" --workspace "$WS" --yes >"$TMP/uninstall2.log" 2>&1 || fail "uninstall exited non-zero"
for base in .codebuddy .workbuddy; do
  [ ! -e "$HOME/$base/rules/__rulemux__a.md" ] || fail "user-level rules residue survived uninstall ($base)"
  [ ! -e "$HOME/$base/skills/__rulemux__global-skill" ] || fail "user-level skills residue survived uninstall ($base)"
done
[ -f "$HOME/.codebuddy/rules/my-own.md" ] || fail "uninstall deleted the user's own user-level file"
ok "uninstall converges the user-level dirs"

# 21. D1: bare sync (no --agent, not --hook) with an empty ledger writes nothing:
# exit 0, an actionable hint, no rules dirs, no ledger file.
D1A_HOME="$TMP/d1a-home"
mkdir -p "$D1A_HOME" "$TMP/d1aws"
HOME="$D1A_HOME" RULEMUX_STATE_DIR="$D1A_HOME/.rulemux" \
  "$BIN" sync --config "$CFG" --workspace "$TMP/d1aws" >"$TMP/d1a.log" 2>&1 \
  || fail "bare sync with an empty ledger must exit 0"
grep -q "sync --agent" "$TMP/d1a.log" || fail "empty-ledger bare sync did not say how to start"
for d in .codebuddy .dsh .trae; do
  [ ! -e "$TMP/d1aws/$d" ] || fail "empty-ledger bare sync created $d"
done
[ ! -e "$D1A_HOME/.rulemux/workspaces.json" ] || fail "empty-ledger bare sync created the ledger"
ok "bare sync with an empty ledger writes nothing (D1)"

# 22. D1: bare sync only touches ledger-recorded agents — this replicates the REPORTED
# user machine (only workbuddy ever used): .dsh/.trae must never be written again.
D1B_HOME="$TMP/d1b-home"
mkdir -p "$D1B_HOME" "$TMP/d1bws/.codebuddy/rules"
HOME="$D1B_HOME" RULEMUX_STATE_DIR="$D1B_HOME/.rulemux" \
  "$BIN" sync --hook --agent workbuddy --config "$CFG" --workspace "$TMP/d1bws" >/dev/null 2>&1 \
  || fail "seed sync (workbuddy) failed"
HOME="$D1B_HOME" RULEMUX_STATE_DIR="$D1B_HOME/.rulemux" \
  "$BIN" sync --config "$CFG" --workspace "$TMP/d1bws" >/dev/null 2>&1 \
  || fail "bare sync failed"
[ -f "$TMP/d1bws/.codebuddy/rules/__rulemux__a.md" ] || fail "the ledger-recorded agent was not synced"
for d in .dsh .trae; do
  [ ! -e "$TMP/d1bws/$d" ] || fail "bare sync touched $d (never recorded in the ledger)"
done
ok "bare sync only touches ledger-recorded agents (the reported user machine) (D1)"

# 23. D1: hook self-heal is decoupled from the sync scope — an empty ledger still
# upgrades an outdated hook (otherwise bare sync would silently lose self-healing).
D1C_HOME="$TMP/d1c-home"
mkdir -p "$D1C_HOME/.codebuddy" "$TMP/d1cws"
cat > "$D1C_HOME/.codebuddy/settings.json" <<'EOF'
{"hooks": {"SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "rulemux sync --agent codebuddy"}]}]}}
EOF
HOME="$D1C_HOME" RULEMUX_STATE_DIR="$D1C_HOME/.rulemux" \
  "$BIN" sync --config "$CFG" --workspace "$TMP/d1cws" >/dev/null 2>&1 \
  || fail "bare sync failed"
grep -qF 'rulemux sync --hook --agent codebuddy' "$D1C_HOME/.codebuddy/settings.json" \
  || fail "empty-ledger bare sync stopped self-healing hooks"
[ ! -e "$TMP/d1cws/.codebuddy/rules" ] || fail "empty-ledger bare sync wrote rule files"
ok "hook self-heal is decoupled from the sync scope (D1)"

# 24. M3: a config that explicitly names an agent still syncs it on a bare sync
# (explicit intent beats the ledger gate).
cat > "$TMP/d1m.toml" <<EOF
[[source]]
path = ["$TMP/src/a.md"]
agents = ["trae"]
EOF
D1M_HOME="$TMP/d1m-home"
mkdir -p "$D1M_HOME" "$TMP/d1mws/.trae/rules"
HOME="$D1M_HOME" RULEMUX_STATE_DIR="$D1M_HOME/.rulemux" \
  "$BIN" sync --config "$TMP/d1m.toml" --workspace "$TMP/d1mws" >/dev/null 2>&1 \
  || fail "bare sync failed"
[ -f "$TMP/d1mws/.trae/rules/__rulemux__a.md" ] || fail "the config-explicit agent was not synced"
ok "config-explicit agents sync on a bare sync (M3)"

# 25. D2: uninstall drops the agent from the ledger — off gains the agent, the
# workspace's agent list loses it, other agents' entries stay.
D2_HOME="$TMP/d2-home"
mkdir -p "$D2_HOME" "$TMP/d2ws/.codebuddy/rules"
HOME="$D2_HOME" RULEMUX_STATE_DIR="$D2_HOME/.rulemux" \
  "$BIN" sync --hook --agent codebuddy --config "$CFG" --workspace "$TMP/d2ws" >/dev/null 2>&1
HOME="$D2_HOME" RULEMUX_STATE_DIR="$D2_HOME/.rulemux" \
  "$BIN" sync --hook --agent workbuddy --config "$CFG" --workspace "$TMP/d2ws" >/dev/null 2>&1
HOME="$D2_HOME" RULEMUX_STATE_DIR="$D2_HOME/.rulemux" \
  "$BIN" uninstall --agent workbuddy --config "$CFG" --workspace "$TMP/d2ws" --yes >/dev/null 2>&1 \
  || fail "uninstall exited non-zero"
LEDGER="$D2_HOME/.rulemux/workspaces.json"
# Cut the "agents" object out of the JSON by its braces (line-window greps would break
# as soon as the object's shape changes) and check the entries inside it only.
agents_block="$(awk '/^  "agents": \{/{f=1} f{print} f && /^  \},/{exit}' "$LEDGER")"
echo "$agents_block" | grep -q '"codebuddy"' || fail "uninstall dropped the wrong ledger entry"
if echo "$agents_block" | grep -q '"workbuddy"'; then fail "uninstall left the agent in the ledger"; fi
grep -A2 '"off"' "$LEDGER" | grep -q '"workbuddy"' || fail "the agent was not recorded as uninstalled (off)"
ok "uninstall drops the agent from the ledger and records it as uninstalled (D2)"

# 26. D3: uninstall removes the emptied rules dir — and never upward.
D3_HOME="$TMP/d3-home"
mkdir -p "$D3_HOME" "$TMP/d3ws/.trae/rules"
HOME="$D3_HOME" RULEMUX_STATE_DIR="$D3_HOME/.rulemux" \
  "$BIN" sync --hook --agent trae --config "$CFG" --workspace "$TMP/d3ws" >/dev/null 2>&1 \
  || fail "seed sync for trae failed"
HOME="$D3_HOME" RULEMUX_STATE_DIR="$D3_HOME/.rulemux" \
  "$BIN" uninstall --agent trae --config "$CFG" --workspace "$TMP/d3ws" --yes >/dev/null 2>&1 \
  || fail "uninstall exited non-zero"
[ ! -e "$TMP/d3ws/.trae/rules" ] || fail "the emptied rules dir survived uninstall"
[ -d "$TMP/d3ws/.trae" ] || fail "uninstall deleted above the leaf dir"
ok "uninstall removes the emptied rules dir (D3)"

# 26b. dsh split delivery (2026-10-14): RULES are injection-only — `sync --hook --agent dsh`
# writes no rules (a leftover .dsh/rules is neither rewritten nor deleted) — but SKILLS are
# real copies (source-verified against deepseek-ai/deepseek-harness): the same sync creates
# .dsh/skills / ~/.dsh/skills and drops __rulemux__* skill trees there, never .agents/skills.
# `inject --agent dsh` stays the rules delivery path; uninstall reports the legacy residue,
# converges the skills dirs, and warns that the host-side plugin keeps calling BOTH.
DSH_HOME="$TMP/dsh-home"
mkdir -p "$DSH_HOME" "$TMP/dshws/.dsh/rules"
printf 'leftover\n' > "$TMP/dshws/.dsh/rules/__rulemux__a.md"
mkdir -p "$TMP/dsh-skills/demo-skill"
printf 'name: demo\n' > "$TMP/dsh-skills/demo-skill/SKILL.md"
cat > "$TMP/dsh.toml" <<EOF
[[source]]
path = ["$TMP/src/a.md"]

[[skill_source]]
path = ["$TMP/dsh-skills"]
agents = ["dsh"]

[[skill_source]]
path = ["$TMP/dsh-skills"]
agents = ["dsh"]
scope = "user"
EOF
HOME="$DSH_HOME" RULEMUX_STATE_DIR="$DSH_HOME/.rulemux" \
  "$BIN" sync --hook --agent dsh --config "$TMP/dsh.toml" --workspace "$TMP/dshws" >/dev/null 2>&1 \
  || fail "sync --hook --agent dsh exited non-zero"
grep -q 'leftover' "$TMP/dshws/.dsh/rules/__rulemux__a.md" || fail "sync --hook --agent dsh rewrote the legacy rules dir"
[ -f "$TMP/dshws/.dsh/skills/__rulemux__demo-skill/SKILL.md" ] || fail "dsh skills did not land in .dsh/skills"
[ -f "$DSH_HOME/.dsh/skills/__rulemux__demo-skill/SKILL.md" ] || fail "dsh user-level skills did not land in ~/.dsh/skills"
[ ! -e "$TMP/dshws/.agents/skills" ] || fail "dsh wrote .agents/skills (cross-framework convention dir, must not be touched)"
HOME="$DSH_HOME" RULEMUX_STATE_DIR="$DSH_HOME/.rulemux" \
  "$BIN" inject --agent dsh --config "$TMP/dsh.toml" --workspace "$TMP/dshws" >"$TMP/dsh-inject.log" 2>&1 \
  || fail "inject --agent dsh exited non-zero"
grep -q 'rulemux:managed' "$TMP/dsh-inject.log" || fail "inject --agent dsh printed no rendered rules"
HOME="$DSH_HOME" RULEMUX_STATE_DIR="$DSH_HOME/.rulemux" \
  "$BIN" uninstall --agent dsh --config "$TMP/dsh.toml" --workspace "$TMP/dshws" --yes >"$TMP/dsh-uninstall.log" 2>&1 \
  || fail "uninstall --agent dsh exited non-zero"
grep -q 'inject --agent dsh' "$TMP/dsh-uninstall.log" || fail "uninstall did not warn that the plugin keeps calling inject"
grep -q 'sync --hook --agent dsh' "$TMP/dsh-uninstall.log" || fail "uninstall did not warn that the plugin keeps syncing skills"
grep -q 'disabled: true' "$TMP/dsh-uninstall.log" || fail "uninstall did not print the documented way to disable the plugin"
grep -q 'leftover' "$TMP/dsh-uninstall.log" || fail "uninstall did not report the legacy .dsh/rules residue"
[ -f "$TMP/dshws/.dsh/rules/__rulemux__a.md" ] || fail "uninstall deleted a legacy file it no longer owns"
[ ! -e "$TMP/dshws/.dsh/skills/__rulemux__demo-skill" ] || fail "uninstall left dsh skills residue in the workspace"
[ ! -e "$DSH_HOME/.dsh/skills/__rulemux__demo-skill" ] || fail "uninstall left user-level dsh skills residue"
ok "dsh: rules injection-only, skills real copies, uninstall converges both (D1+D3+R1)"

# 27. D3 boundaries: a dir that is not truly empty is kept — the user's own file,
# a prefixed subdir, or a symlink all count as "not empty".
D3B_HOME="$TMP/d3b-home"
mkdir -p "$D3B_HOME" "$TMP/d3bws/.dsh/rules" "$TMP/d3dws/.dsh/rules/__rulemux__x" \
         "$TMP/d3cws/.dsh" "$TMP/d3-real"
echo mine > "$TMP/d3bws/.dsh/rules/my-own.md"
printf 'x\n' > "$TMP/d3-real/__rulemux__a.md"
ln -s "$TMP/d3-real" "$TMP/d3cws/.dsh/rules"
for w in d3bws d3dws d3cws; do
  HOME="$D3B_HOME" RULEMUX_STATE_DIR="$D3B_HOME/.rulemux" \
    "$BIN" sync --hook --agent dsh --config "$CFG" --workspace "$TMP/$w" >/dev/null 2>&1 \
    || fail "seed sync for $w failed"
done
HOME="$D3B_HOME" RULEMUX_STATE_DIR="$D3B_HOME/.rulemux" \
  "$BIN" uninstall --agent dsh --config "$CFG" --workspace "$TMP/d3bws" --yes >/dev/null 2>&1
[ -f "$TMP/d3bws/.dsh/rules/my-own.md" ] || fail "uninstall deleted the user's own file"
[ -d "$TMP/d3bws/.dsh/rules" ] || fail "a non-empty rules dir was removed"
[ -d "$TMP/d3dws/.dsh/rules/__rulemux__x" ] || fail "a prefixed subdir was removed"
[ -d "$TMP/d3dws/.dsh/rules" ] || fail "a dir with a subdir entry was removed"
[ -L "$TMP/d3cws/.dsh/rules" ] || fail "a symlinked rules dir was not left alone"
[ -d "$TMP/d3-real" ] || fail "the symlink target was deleted"
ok "uninstall keeps non-empty / subdir-bearing / symlinked rules dirs (D3)"

# 28. R4: bare verify probes only in-use agents; explicit --agent still works;
# bare --clean is a superset (removes every canary, in use or not).
R4_HOME="$TMP/r4-home"
mkdir -p "$R4_HOME" "$TMP/r4ws/.codebuddy/rules" "$TMP/r4ws/.dsh/rules" "$TMP/r4ws/.trae/rules"
# Real usage installs the hook (init) before the session fires sync; the hook-gate
# (aba8440) counts an agent as in-use only when its hook is present, so install it
# here too (isolated temp HOME — never touches the real ~/.codebuddy).
HOME="$R4_HOME" RULEMUX_STATE_DIR="$R4_HOME/.rulemux" \
  "$BIN" init --agent codebuddy --config "$CFG" --workspace "$TMP/r4ws" >/dev/null 2>&1 \
  || fail "R4: init (hook install) failed"
HOME="$R4_HOME" RULEMUX_STATE_DIR="$R4_HOME/.rulemux" \
  "$BIN" sync --hook --agent codebuddy --config "$CFG" --workspace "$TMP/r4ws" >/dev/null 2>&1
HOME="$R4_HOME" RULEMUX_STATE_DIR="$R4_HOME/.rulemux" \
  "$BIN" verify --config "$CFG" --workspace "$TMP/r4ws" >/dev/null 2>&1 \
  || fail "bare verify exited non-zero"
[ -f "$TMP/r4ws/.codebuddy/rules/__rulemux__canary.md" ] || fail "bare verify did not probe the in-use agent"
for d in .dsh .trae; do
  [ ! -e "$TMP/r4ws/$d/rules/__rulemux__canary.md" ] || fail "bare verify probed $d (not in use)"
done
HOME="$R4_HOME" RULEMUX_STATE_DIR="$R4_HOME/.rulemux" \
  "$BIN" verify --agent trae --config "$CFG" --workspace "$TMP/r4ws" >/dev/null 2>&1
[ -f "$TMP/r4ws/.trae/rules/__rulemux__canary.md" ] || fail "explicit --agent verify was blocked"
HOME="$R4_HOME" RULEMUX_STATE_DIR="$R4_HOME/.rulemux" \
  "$BIN" verify --clean --config "$CFG" --workspace "$TMP/r4ws" >/dev/null 2>&1
for d in .codebuddy .trae; do
  [ ! -e "$TMP/r4ws/$d/rules/__rulemux__canary.md" ] || fail "bare --clean left a canary in $d"
done
[ ! -e "$TMP/r4ws/.dsh/rules/__rulemux__canary.md" ] || fail "bare --clean left a canary in the injection-only dsh dir"

# ...and bare --clean must still work when NOTHING is in use (empty ledger): the
# "nothing to verify" early return must not swallow the cleanup.
R4B_HOME="$TMP/r4b-home"
mkdir -p "$R4B_HOME" "$TMP/r4bws/.trae/rules"
HOME="$R4B_HOME" RULEMUX_STATE_DIR="$R4B_HOME/.rulemux" \
  "$BIN" verify --agent trae --config "$CFG" --workspace "$TMP/r4bws" >/dev/null 2>&1
[ -f "$TMP/r4bws/.trae/rules/__rulemux__canary.md" ] || fail "explicit verify did not write the canary"
HOME="$R4B_HOME" RULEMUX_STATE_DIR="$R4B_HOME/.rulemux" \
  "$BIN" verify --clean --config "$CFG" --workspace "$TMP/r4bws" >/dev/null 2>&1 \
  || fail "bare --clean exited non-zero on an empty in-use set"
[ ! -e "$TMP/r4bws/.trae/rules/__rulemux__canary.md" ] || fail "bare --clean skipped the canary (empty in-use set)"
[ ! -e "$TMP/r4bws/.trae/rules" ] || fail "the emptied rules dir survived bare --clean"
ok "verify probes only in-use agents; --clean is a superset (R4)"

# 29. D1 + legacy ledger: a v1 ledger (no "agents" key => the "unknown / all" sentinel)
# must NOT re-authorise writes to agents that never proved they read this workspace —
# i.e. a leftover .dsh/rules on such a workspace stays untouched, and no new dir appears.
L_HOME="$TMP/legacy-home"
L_WS="$TMP/legacyws"
mkdir -p "$L_HOME" "$L_HOME/.rulemux" "$L_WS/.dsh/rules"
printf 'leftover v1\n' > "$L_WS/.dsh/rules/__rulemux__a.md"
cat > "$L_HOME/.rulemux/workspaces.json" <<EOF
{"version":2,"workspaces":["$L_WS"],"agents":{"$L_WS":["*"]}}
EOF
HOME="$L_HOME" RULEMUX_STATE_DIR="$L_HOME/.rulemux" \
  "$BIN" sync --config "$CFG" --workspace "$L_WS" >"$TMP/legacy.log" 2>&1 \
  || fail "bare sync on a legacy ledger should exit 0"
grep -q 'leftover v1' "$L_WS/.dsh/rules/__rulemux__a.md" || fail "a legacy '*' ledger entry rewrote a never-used agent's rules dir"
for d in .codebuddy .trae; do
  [ ! -e "$L_WS/$d" ] || fail "a legacy '*' ledger entry created $d out of nowhere"
done
ok "a legacy '*' ledger entry does not authorise foreign agents (D1)"

# 29. Trae skills: [[skill_source]] lands in BOTH workspace .trae/skills AND user-level ~/.trae-cn/skills
mkdir -p "$TMP/trae-skills/trae-demo"
printf 'name: trae-demo\n' > "$TMP/trae-skills/trae-demo/SKILL.md"
cat > "$TMP/trae.toml" <<EOF
[[source]]
path = ["$TMP/src/a.md"]

[[skill_source]]
path = ["$TMP/trae-skills"]
agents = ["trae"]

# user-level ~/.trae-cn/skills is fed by a scope="user" source (same model as
# codebuddy's user.toml above) — a workspace-scope source never lands user-level.
[[skill_source]]
path = ["$TMP/trae-skills"]
agents = ["trae"]
scope = "user"
EOF
"$BIN" sync --hook --agent trae --config "$TMP/trae.toml" --workspace "$WS" >"$TMP/trae-skill.log" 2>&1 || fail "trae skill sync exited non-zero"
[ -f "$WS/.trae/skills/__rulemux__trae-demo/SKILL.md" ] || fail "trae workspace skills not synced"
[ -f "$HOME/.trae-cn/skills/__rulemux__trae-demo/SKILL.md" ] || fail "trae user-level skills not synced"
# the user's own skill file is never touched
echo mine > "$HOME/.trae-cn/skills/my-own.md"
"$BIN" sync --hook --agent trae --config "$TMP/trae.toml" --workspace "$WS" >/dev/null 2>&1
[ -f "$HOME/.trae-cn/skills/my-own.md" ] || fail "trae sync deleted the user's own skill"
# verify also drops a skill-shaped probe into the workspace skills dir
"$BIN" verify --agent trae --config "$TMP/trae.toml" --workspace "$WS" >"$TMP/trae-verify.log" 2>&1 || fail "trae verify exited non-zero"
[ -f "$WS/.trae/skills/__rulemux__canary-skill/SKILL.md" ] || fail "trae verify did not write skills canary"
"$BIN" verify --agent trae --config "$TMP/trae.toml" --workspace "$WS" --clean >/dev/null 2>&1
[ ! -e "$WS/.trae/skills/__rulemux__canary-skill" ] || fail "trae verify did not clean skills canary"
ok "trae: skills sync to workspace + user-level, verify probes skills"

echo
echo "ALL SMOKE TESTS PASSED"
