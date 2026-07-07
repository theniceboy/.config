#!/usr/bin/env bash
# TUI test: drives the goals panel via tmux send-keys + capture-pane.
set -uo pipefail

AGENT="/home/testuser/bin/agent"
PASS=0; FAIL=0

check() {
    if echo "$2" | grep -q -- "$3"; then
        echo "  PASS: $1"; PASS=$((PASS+1))
    else
        echo "  FAIL: $1 (expected '$3')"
        echo "$2" | head -24 | sed 's/^/    | /'
        FAIL=$((FAIL+1))
    fi
}

tk()   { tmux send-keys -t test:0.0 "$@"; sleep 0.4; }
cap()  { tmux capture-pane -p -t test:0.0 2>/dev/null; }
type_() { printf '%s' "$1" | while IFS= read -r -n1 ch; do tmux send-keys -t test:0.0 -l "$ch"; sleep 0.03; done; sleep 0.2; }

# ── Setup ───────────────────────────────────────────────────
tmux kill-session -t test 2>/dev/null || true
tmux new-session -d -s test -x 200 -y 60
sleep 0.3
rm -rf ~/.cache/agent; mkdir -p ~/.cache/agent

$AGENT goal add-goal --title "Ship V2" >/dev/null
$AGENT goal add-goal --title "Board UX" --parent ship-v2 >/dev/null
$AGENT goal add-thread --name "drag-drop" --goal board-ux >/dev/null
$AGENT goal add-thread --name "notif" --goal board-ux >/dev/null

# ── 1. Palette + Goals view ─────────────────────────────────
echo "=== 1. Palette + Goals view ==="
tmux send-keys -t test:0.0 "$AGENT palette --window=@0 --path=/tmp --session-name=test --window-name=test0" Enter
sleep 1.5
OUT=$(cap); check "palette shows" "$OUT" "Command Palette"

tk M-r
OUT=$(cap)
check "goals header" "$OUT" "Goals"
check "goal Ship V2" "$OUT" "Ship V2"
check "sub-goal Board UX" "$OUT" "Board UX"
check "thread drag-drop" "$OUT" "drag-drop"
check "thread notif" "$OUT" "notif"

# ── 2. Navigation ───────────────────────────────────────────
echo "=== 2. Navigation ==="
tk e; OUT=$(cap); check "e moves down" "$OUT" "Goals"
tk e; OUT=$(cap); check "e again" "$OUT" "Goals"
tk e; OUT=$(cap); check "e to thread" "$OUT" "Goals"
tk u; OUT=$(cap); check "u moves up" "$OUT" "Goals"
tk n; OUT=$(cap); check "n parent jump" "$OUT" "Goals"

# ── 3. Expand thread ────────────────────────────────────────
echo "=== 3. Expand thread ==="
tk e; tk e   # navigate to a thread
tk Right
OUT=$(cap); check "expand keeps goals" "$OUT" "Goals"
tk Right     # collapse
OUT=$(cap); check "collapse keeps goals" "$OUT" "Goals"

# ── 4. More options (o) ─────────────────────────────────────
echo "=== 4. More options ==="
tk o
OUT=$(cap)
check "more-options opens" "$OUT" "[Aa]ction"
# filter for "add goal"
type_ "add goal"
OUT=$(cap); check "filter works" "$OUT" "[Aa]dd [Gg]oal"
tk Enter
OUT=$(cap); check "add-goal input opens" "$OUT" "[Nn]ew"

# ── 5. Create goal via input ────────────────────────────────
echo "=== 5. Create goal ==="
type_ "NewGoal"
tk Enter
OUT=$(cap); check "new goal in view" "$OUT" "NewGoal"

# ── 6. Rename ───────────────────────────────────────────────
echo "=== 6. Rename ==="
tk u; tk u   # move up to a goal
tk r
OUT=$(cap); check "rename input opens" "$OUT" "[Nn]ame\|[Tt]itle"
tk C-u   # clear existing text
type_ "RenamedGoal"
tk Enter
OUT=$(cap); check "rename shows in view" "$OUT" "RenamedGoal"

# ── 7. Done toggle (D) ──────────────────────────────────────
echo "=== 7. Done toggle ==="
tk e; tk e   # move to a thread
tk S-D       # shift-D
OUT=$(cap); check "D confirm opens" "$OUT" "[Dd]elete\|[Dd]one\|[Tt]oggle"
tk y         # confirm
OUT=$(cap); check "D action applied" "$OUT" "Goals"

# ── 8. Close + persist check ────────────────────────────────
echo "=== 8. Persist ==="
sleep 0.5
tk M-s
sleep 0.5
LIST=$($AGENT goal list 2>/dev/null)
check "new goal persisted" "$LIST" "NewGoal"
check "rename persisted" "$LIST" "RenamedGoal"

# ── Summary ─────────────────────────────────────────────────
echo ""
echo "========================================"
echo "TUI Results: $PASS passed, $FAIL failed"
echo "========================================"
tmux kill-session -t test 2>/dev/null || true
exit $FAIL
