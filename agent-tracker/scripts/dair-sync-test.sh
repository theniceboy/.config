#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE="${DAIR_REMOTE:-david@dair.local}"
REMOTE_ROOT="${DAIR_REMOTE_ROOT:-/Users/david/.config/agent-tracker}"

if [[ "${1:-}" == "--help" ]]; then
	echo "Usage: $(basename "$0") [remote command]"
	echo "Syncs $ROOT to $REMOTE:$REMOTE_ROOT and runs a command there."
	echo "Default remote command: ./install.sh && /opt/homebrew/bin/go test ./..."
	exit 0
fi

REMOTE_CMD="${*:-./install.sh && /opt/homebrew/bin/go test ./...}"

rsync -az --delete \
	--exclude '.git/' \
	--exclude '.build/' \
	--exclude 'bin/' \
	--exclude 'run/' \
	--exclude '.DS_Store' \
	"$ROOT/" "$REMOTE:$REMOTE_ROOT/"

ssh "$REMOTE" "mkdir -p '$REMOTE_ROOT' && cd '$REMOTE_ROOT' && export PATH='/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:\$PATH' && $REMOTE_CMD"
