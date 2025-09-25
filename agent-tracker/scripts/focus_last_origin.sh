#!/usr/bin/env bash
set -euo pipefail

F="$HOME/.config/agent-tracker/run/jump_back.txt"
if [[ ! -f "$F" ]]; then
  exit 0
fi

sid=$(awk -F ':::' 'NR==1{print $1}' "$F" | tr -d '\r\n')
wid=$(awk -F ':::' 'NR==1{print $2}' "$F" | tr -d '\r\n')
pid=$(awk -F ':::' 'NR==1{print $3}' "$F" | tr -d '\r\n')

if [[ -z "${sid:-}" || -z "${wid:-}" || -z "${pid:-}" ]]; then
  exit 0
fi

tmux switch-client -t "$sid" \; select-window -t "$wid" \; select-pane -t "$pid"

