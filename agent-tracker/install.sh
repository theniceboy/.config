#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

if ! command -v go >/dev/null 2>&1 && [[ -x /opt/homebrew/bin/go ]]; then
	export PATH="/opt/homebrew/bin:$PATH"
fi

go build -o bin/tracker-server ./cmd/tracker-server

go build -o bin/tracker-mcp ./cmd/tracker-mcp

go build -o bin/agent ./cmd/agent

echo "Built tracker server, MCP, and agent binaries into bin/"
