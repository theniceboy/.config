#!/usr/bin/env bash
set -euo pipefail

export HOME=/home/testuser
AGENT_SRC="/home/testuser/agent-tracker"
AGENT_BIN="/home/testuser/bin"

mkdir -p "$AGENT_BIN"
echo "=== Building agent-tracker ==="
cd "$AGENT_SRC"
go build -o "$AGENT_BIN/agent" ./cmd/agent 2>&1
echo "Build complete."

# Seed fake data and create tmux session with drafts
bash "$AGENT_SRC/test/docker/seed-data.sh"

# Launch goals panel in the main window
MAIN_WID=$(tmux display-message -p -t test:0 "#{window_id}")
tmux send-keys -t test:0 "$AGENT_BIN/agent palette --mode goals --window=$MAIN_WID --path=/tmp --session-name=test --window-name=main" Enter

echo ""
echo "============================================"
echo "  Interactive test container ready"
echo "============================================"
echo ""
echo "tmux session 'test' is running with:"
echo "  - 4 goals (Ship V2 > Board UX, API Refactor; Marketing Site)"
echo "  - 5 threads (3 assigned, 2 standalone, 1 done)"
echo "  - 5 drafts (unassigned agent windows)"
echo ""
echo "Goals panel is open in window 0."
echo ""
echo "To interact:"
echo "  docker exec -it <container> tmux attach -t test"
echo ""
echo "To send keys from outside:"
echo "  docker exec <container> tmux send-keys -t test:0 <key>"
echo ""
echo "To capture screen:"
echo "  docker exec <container> tmux capture-pane -p -t test:0"
echo ""
echo "Keeping container alive..."
exec sleep infinity
