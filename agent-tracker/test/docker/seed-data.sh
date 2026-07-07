#!/usr/bin/env bash
set -euo pipefail

export HOME=/home/testuser
AGENT=/home/testuser/bin/agent

echo "=== Seeding fake data ==="
rm -rf ~/.cache/agent
mkdir -p ~/.cache/agent

# Goals
$AGENT goal add-goal --title "Ship V2"
$AGENT goal add-goal --title "Board UX"   --parent ship-v2
$AGENT goal add-goal --title "API Refactor" --parent ship-v2
$AGENT goal add-goal --title "Marketing Site"

# Threads (assigned to goals)
$AGENT goal add-thread --name "drag-drop upload"      --goal board-ux
$AGENT goal add-thread --name "notification system"   --goal board-ux
$AGENT goal add-thread --name "auth middleware"       --goal api-refactor

# Threads (unassigned, top-level)
$AGENT goal add-thread --name "Standalone thread A"
$AGENT goal add-thread --name "Standalone thread B"

# Mark one as done
DONE_ID=$($AGENT goal list 2>/dev/null | grep 'drag-drop' | grep -o 'th_[a-f0-9]*' | head -1)
if [ -n "$DONE_ID" ]; then
    $AGENT goal done --thread "$DONE_ID" 2>/dev/null || true
fi

echo "=== Creating fake drafts ==="
# Kill any existing tmux session
tmux kill-session -t test 2>/dev/null || true

# Create session
tmux new-session -d -s test -x 120 -y 55 -n main
sleep 0.3

# Create windows to simulate agent sessions (drafts)
declare -A WINDOWS
WINDOWS=(
    ["fix-bug"]="Fix the login page redirect bug"
    ["add-tests"]="Add unit tests for payment module"
    ["refactor-nav"]="Refactor the navigation component"
    ["docs-readme"]="Update README with new install steps"
    ["perf-query"]="Optimize the slow database query"
)

for wname in "${!WINDOWS[@]}"; do
    msg="${WINDOWS[$wname]}"
    tmux new-window -t test -n "$wname"
    sleep 0.1
    wid=$(tmux display-message -p -t test "=#{window_id}")
    /home/testuser/fake-agent.sh "$wid" "$msg" 2>/dev/null || true
done

# Go back to first window
tmux select-window -t test:0

echo ""
echo "=== Seed data complete ==="
$AGENT goal list 2>/dev/null
echo ""
echo "Drafts (unassigned windows):"
echo "  fix-bug, add-tests, refactor-nav, docs-readme, perf-query"
