#!/usr/bin/env bash
set -euo pipefail

client_tty="${1-}"
window_id="${2-}"
agent_id="${3-}"
path_value="${4-}"
session_name="${5-}"
window_name="${6-}"

exec tmux display-popup -E -c "$client_tty" -d "$path_value" -w 78% -h 80% -T agent \
  ~/.config/agent-tracker/bin/agent palette \
  --window="$window_id" \
  --agent-id="$agent_id" \
  --path="$path_value" \
  --session-name="$session_name" \
  --window-name="$window_name"
