#!/usr/bin/env bash
set -euo pipefail

if ! command -v brew >/dev/null 2>&1; then
  echo "Error: Homebrew is required but not found in PATH" >&2
  exit 1
fi

if ! brew services list >/dev/null 2>&1; then
  echo "Error: brew services command is unavailable; install the homebrew/services tap" >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER_BIN="$ROOT_DIR/bin/tracker-server"

echo "Building tracker-server..." >&2
mkdir -p "$ROOT_DIR/bin"
if ! (cd "$ROOT_DIR" && go build -o bin/tracker-server ./cmd/tracker-server); then
  echo "Error: go build failed" >&2
  exit 1
fi

BREW_REPO="$(brew --repository)"
TAP_PATH="$BREW_REPO/Library/Taps/agenttracker/homebrew-agent-tracker"
FORMULA_DIR="$TAP_PATH/Formula"
FORMULA_PATH="$FORMULA_DIR/agent-tracker-server.rb"
mkdir -p "$FORMULA_DIR"

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cp "$SERVER_BIN" "$TMP_DIR/tracker-server"
TARBALL="$TMP_DIR/tracker-server.tar.gz"
tar -czf "$TARBALL" -C "$TMP_DIR" tracker-server
SHA256="$(shasum -a 256 "$TARBALL" | awk '{print $1}')"
VERSION="local-$(date +%Y%m%d%H%M%S)"

cat >"$FORMULA_PATH" <<EOF
class AgentTrackerServer < Formula
  desc "Tmux-aware agent tracker server"
  homepage "https://github.com/david/agent-tracker"
  url "file://$TARBALL"
  sha256 "$SHA256"
  version "$VERSION"

  def install
    bin.install "tracker-server"
  end

  service do
    run [opt_bin/"tracker-server"]
    keep_alive true
    working_dir var/"agent-tracker"
    environment_variables PATH: "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
    log_path var/"log/agent-tracker-server.log"
    error_log_path var/"log/agent-tracker-server.log"
  end
end
EOF

if brew list --formula agent-tracker-server >/dev/null 2>&1; then
  brew reinstall --formula "$FORMULA_PATH" >/dev/null
else
  brew install --formula "$FORMULA_PATH" >/dev/null
fi

mkdir -p "$(brew --prefix)/var/agent-tracker"

if brew services list | awk '{print $1}' | grep -qx "agent-tracker-server"; then
  brew services restart agent-tracker-server >/dev/null
else
  brew services start agent-tracker-server >/dev/null
fi

SERVICE_STATE="$(brew services list | awk '$1=="agent-tracker-server" {print $2}')"
if [[ "$SERVICE_STATE" != "started" ]]; then
  echo "Error: brew reports agent-tracker-server service in state '$SERVICE_STATE'" >&2
  exit 1
fi

echo "Agent tracker server managed by brew services (state: $SERVICE_STATE)." >&2
