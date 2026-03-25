#!/usr/bin/env bash
set -euo pipefail

session_id="$1"
[[ -z "$session_id" ]] && exit 0

agent_bin="$HOME/.config/agent-tracker/bin/agent"
[[ ! -x "$agent_bin" ]] && exit 0

state=$("$agent_bin" tracker state 2>/dev/null || true)
[[ -z "$state" ]] && exit 0

# Check for in_progress or unacknowledged completed tasks in this session
result=$(echo "$state" | jq -r --arg sid "$session_id" '
  .tasks // [] | .[] | select(.session_id == $sid) |
  if .status == "in_progress" then "in_progress"
  elif .status == "completed" and .acknowledged != true then "waiting"
  else empty end
' 2>/dev/null | head -1 || true)

case "$result" in
  in_progress) printf '⏳' ;;
  waiting) printf '🔔' ;;
esac
