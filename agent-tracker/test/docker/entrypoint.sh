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

echo "=== Running TUI tests ==="
bash "$AGENT_SRC/test/docker/run-tests.sh"
TUI_EXIT=$?

echo "=== Running visual tests ==="
AGENT_BIN="/home/testuser/bin/agent" python3 "$AGENT_SRC/test/docker/visual_tests.py"
VISUAL_EXIT=$?

echo ""
echo "TUI tests: $TUI_EXIT  Visual tests: $VISUAL_EXIT"
exit $((TUI_EXIT + VISUAL_EXIT))
