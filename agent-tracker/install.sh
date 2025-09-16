#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

go build -o bin/tracker-client ./cmd/tracker-client

go build -o bin/tracker-server ./cmd/tracker-server

go build -o bin/tracker-mcp ./cmd/tracker-mcp

echo "Built tracker client, server, and MCP binaries into bin/"
