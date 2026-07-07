#!/usr/bin/env bash
# Fake agent: simulates an opencode turn by firing tracker hook events.
# Usage: fake-agent.sh <window_id> <user_message>
# This mimics what the real hook script does: start_task with the message as summary.
set -euo pipefail

WINDOW_ID="${1:-}"
MESSAGE="${2:-working...}"

AGENT_BIN="/home/testuser/bin/agent"
TRACKER_BIN="/home/testuser/bin/tracker-client"

# Resolve tmux context for the window
PANE=$(tmux list-panes -a -F '#{window_id}|#{pane_id}' 2>/dev/null | grep "^${WINDOW_ID}|" | head -1 | cut -d'|' -f2 || true)
if [ -z "$PANE" ]; then
    echo "fake-agent: no pane for window $WINDOW_ID" >&2
    exit 1
fi

SESSION_ID=$(tmux display-message -p -t "$PANE" "#{session_id}" 2>/dev/null || echo "")
WINDOW_NAME=$(tmux display-message -p -t "$PANE" "#{window_name}" 2>/dev/null || echo "")
SESSION_NAME=$(tmux display-message -p -t "$PANE" "#{session_name}" 2>/dev/null || echo "")

echo "fake-agent: window=$WINDOW_ID pane=$PANE session=$SESSION_ID message=$MESSAGE"

# Start a tracker task (this creates the draft thread)
$TRACKER_BIN command \
    -session-id "$SESSION_ID" \
    -window-id "$WINDOW_ID" \
    -pane "$PANE" \
    -summary "$MESSAGE" \
    start_task 2>/dev/null || true

# Revive any done thread for this window with the new name
$AGENT_BIN goal revive --window "$WINDOW_ID" --name "$MESSAGE" 2>/dev/null || true

echo "fake-agent: turn started"
