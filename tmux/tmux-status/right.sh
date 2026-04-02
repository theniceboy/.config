#!/usr/bin/env bash
set -euo pipefail

agent_bin="$HOME/.config/agent-tracker/bin/agent"
[[ -x "$agent_bin" ]] || exit 0

exec "$agent_bin" tmux right-status "$@"
