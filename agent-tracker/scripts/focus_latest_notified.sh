#!/usr/bin/env bash
set -euo pipefail

F="$HOME/.config/agent-tracker/run/latest_notified.txt"
if [[ ! -f "$F" ]]; then
  exit 0
fi

# Read line and split by literal ':::' into sid, wid, pid robustly
# Extract fields robustly using awk with a literal ':::' separator
sid=$(awk -F ':::' 'NR==1{print $1}' "$F" | tr -d '\r\n')
wid=$(awk -F ':::' 'NR==1{print $2}' "$F" | tr -d '\r\n')
pid=$(awk -F ':::' 'NR==1{print $3}' "$F" | tr -d '\r\n')

if [[ -z "${sid:-}" || -z "${wid:-}" || -z "${pid:-}" ]]; then
  exit 0
fi

RUN_DIR="$HOME/.config/agent-tracker/run"
mkdir -p "$RUN_DIR"

# Record current location for jump-back
current=$(tmux display-message -p "#{session_id}:::#{window_id}:::#{pane_id}" | tr -d '\r\n')
if [[ -n "$current" ]]; then
  printf '%s\n' "$current" > "$RUN_DIR/jump_back.txt"
fi

# Mark as viewed (acknowledged) in tracker (graceful if unavailable)
CLIENT_BIN="$HOME/.config/agent-tracker/bin/tracker-client"
if [[ -x "$CLIENT_BIN" ]]; then
  "$CLIENT_BIN" command acknowledge -session-id "$sid" -window-id "$wid" -pane "$pid" >/dev/null 2>&1 || true
fi

# Focus the tmux target
tmux switch-client -t "$sid" \; select-window -t "$wid" \; select-pane -t "$pid"
