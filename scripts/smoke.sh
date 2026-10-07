#!/usr/bin/env bash
# rulemux 冒烟自测：端到端验证 init / sync / inject / doctor / verify。
#
# 用法：scripts/smoke.sh
#   （默认用仓库根的 rulemux 二进制；不存在则先 go build）
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

BIN="$REPO_ROOT/rulemux"
if [ ! -x "$BIN" ]; then
  go build -o "$BIN" .
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 关键：把 HOME 指到临时目录，避免 init 真的去写 ~/.codex/config.toml
export HOME="$TMP/home"
mkdir -p "$HOME"

WS="$TMP/ws"
CFG="$TMP/config.toml"
mkdir -p "$WS" "$TMP/src"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok()   { echo "ok - $*"; }

# 1. 准备源文件与配置
printf 'rule A v1\n' > "$TMP/src/a.md"
printf 'rule B\n'    > "$TMP/src/b.txt"
cat > "$CFG" <<EOF
[[source]]
path = "$TMP/src/a.md"
agents = ["claude", "codebuddy", "trae"]

[[source]]
path = "$TMP/src/b.txt"
EOF

# 2. init：配置已存在则跳过生成；并为各 agent 装钩子
"$BIN" init --config "$CFG" --workspace "$WS" >"$TMP/init.log" 2>&1 || fail "init 退出码非 0"
grep -q "配置已存在" "$TMP/init.log" || fail "init 未复用已有配置"
for f in .claude/settings.json .codebuddy/settings.json hooks.json; do
  grep -q '"rulemux"' "$WS/$f" || fail "钩子未写入 $f"
done
ok "init 装钩子"

# 幂等：再 init 一次，rulemux 出现次数不应增加
before="$(grep -o '"rulemux"' "$WS/.claude/settings.json" | wc -l)"
"$BIN" init --config "$CFG" --workspace "$WS" >/dev/null 2>&1
after="$(grep -o '"rulemux"' "$WS/.claude/settings.json" | wc -l)"
[ "$before" = "$after" ] || fail "init 不幂等：$before -> $after"
ok "init 幂等"

# 3. sync：三个 Tier-1 目录都应落地带前缀文件
"$BIN" sync --config "$CFG" --workspace "$WS" >"$TMP/sync1.log" 2>&1 || fail "sync 退出码非 0"
for d in .claude/rules .codebuddy/rules .trae/rules; do
  [ -f "$WS/$d/.rulemux__a.md" ]  || fail "$d 缺少 .rulemux__a.md"
  [ -f "$WS/$d/.rulemux__b.txt" ] || fail "$d 缺少 .rulemux__b.txt"
done
ok "sync 落地三目录"

# 4. 幂等：再 sync 一次应全部跳过、不重复新增
"$BIN" sync --config "$CFG" --workspace "$WS" >"$TMP/sync2.log" 2>&1
grep -q "内容未变跳过" "$TMP/sync2.log" || fail "二次 sync 未复用（不幂等）"
if grep -q "新增:" "$TMP/sync2.log"; then fail "二次 sync 又新增了文件"; fi
ok "sync 幂等（跳过）"

# 5. 用户自己的文件不受影响
echo "mine" > "$WS/.claude/rules/my-own.md"
"$BIN" sync --config "$CFG" --workspace "$WS" >/dev/null 2>&1
[ -f "$WS/.claude/rules/my-own.md" ] || fail "用户自己的文件被删了"
ok "不碰用户文件"

# 6. 源内容变更 ⇒ 覆盖
printf 'rule A v2\n' > "$TMP/src/a.md"
"$BIN" sync --config "$CFG" --workspace "$WS" >"$TMP/sync3.log" 2>&1
grep -q "更新:" "$TMP/sync3.log" || fail "内容变更未触发更新"
grep -q "rule A v2" "$WS/.claude/rules/.rulemux__a.md" || fail "更新后内容不对"
ok "源变更覆盖"

# 7. 配置删掉一条 ⇒ 目标残留被清（加删自由）
cat > "$CFG" <<EOF
[[source]]
path = "$TMP/src/a.md"
agents = ["claude", "codebuddy", "trae"]
EOF
"$BIN" sync --config "$CFG" --workspace "$WS" >"$TMP/sync4.log" 2>&1
grep -q "删除残留" "$TMP/sync4.log" || fail "未删除残留"
for d in .claude/rules .codebuddy/rules .trae/rules; do
  [ ! -f "$WS/$d/.rulemux__b.txt" ] || fail "$d 残留未清"
  [ -f "$WS/$d/.rulemux__a.md" ]    || fail "$d 误删了 a.md"
done
ok "删残留（加删自由）"

# 8. inject（Tier-2）：输出注入内容
#    注意：第 7 步后配置里 a.md 只给 claude/codebuddy/trae，codex 命中不到源，
#    所以这里单独写一份含 codex 的配置来验证注入。
cat > "$TMP/codex.toml" <<EOF
[[source]]
path = "$TMP/src/a.md"
agents = ["codex"]
EOF
"$BIN" inject --agent codex --config "$TMP/codex.toml" >"$TMP/inject.log" 2>&1
grep -q "rulemux:managed" "$TMP/inject.log" || fail "inject 未输出托管标记"
grep -q "rule A v2"       "$TMP/inject.log" || fail "inject 未包含规则内容"

# 命中不到该 agent 时应静默输出空（不往上下文塞垃圾）
"$BIN" inject --agent codex --config "$CFG" >"$TMP/inject2.log" 2>&1
[ ! -s "$TMP/inject2.log" ] || fail "无匹配源时 inject 不应输出内容"
ok "inject 输出规则（且无匹配时静默）"

# 9. doctor 能跑通
"$BIN" doctor --config "$CFG" --workspace "$WS" >"$TMP/doctor.log" 2>&1 || fail "doctor 退出码非 0"
grep -q "claude" "$TMP/doctor.log" || fail "doctor 未列出 claude"
ok "doctor 自检"

# 10. verify canary：写入后可清理
"$BIN" verify --agent claude --workspace "$WS" >"$TMP/verify.log" 2>&1
[ -f "$WS/.claude/rules/.rulemux__canary.md" ] || fail "canary 未写入"
"$BIN" verify --agent claude --clean --workspace "$WS" >/dev/null 2>&1
[ ! -f "$WS/.claude/rules/.rulemux__canary.md" ] || fail "canary 未清理"
ok "verify canary 写入/清理"

echo
echo "ALL SMOKE TESTS PASSED"
