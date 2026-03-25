#!/usr/bin/env bash
set -euo pipefail

F="$HOME/.config/agent-tracker/run/jump_back.txt"
if [[ ! -f "$F" ]]; then
  exit 0
fi

resolve_by_locator() {
  local session_name="$1"
  local window_index="$2"
  local pane_index="$3"
  tmux list-panes -a -F "#{session_name}:::#{window_index}:::#{pane_index}:::#{session_id}:::#{window_id}:::#{pane_id}" \
    | awk -F ':::' -v s="$session_name" -v w="$window_index" -v p="$pane_index" '$1==s && $2==w && $3==p {print $4":::"$5":::"$6; exit}'
}

target_exists() {
  local pane="$1"
  tmux display-message -p -t "$pane" '#{pane_id}' >/dev/null 2>&1
}

resolve_by_pane() {
  local pane="$1"
  tmux display-message -p -t "$pane" '#{session_id}:::#{window_id}:::#{pane_id}' 2>/dev/null | tr -d '\r\n'
}

sid=$(awk -F ':::' 'NR==1{print $1}' "$F" | tr -d '\r\n')
wid=$(awk -F ':::' 'NR==1{print $2}' "$F" | tr -d '\r\n')
pid=$(awk -F ':::' 'NR==1{print $3}' "$F" | tr -d '\r\n')
session_name=$(awk -F ':::' 'NR==1{print $4}' "$F" | tr -d '\r\n')
window_index=$(awk -F ':::' 'NR==1{print $5}' "$F" | tr -d '\r\n')
pane_index=$(awk -F ':::' 'NR==1{print $6}' "$F" | tr -d '\r\n')

if [[ -z "${sid:-}" || -z "${wid:-}" || -z "${pid:-}" ]]; then
  exit 0
fi

if target_exists "$pid"; then
  resolved=$(resolve_by_pane "$pid")
  if [[ -n "$resolved" ]]; then
    sid=$(printf '%s' "$resolved" | awk -F ':::' '{print $1}')
    wid=$(printf '%s' "$resolved" | awk -F ':::' '{print $2}')
    pid=$(printf '%s' "$resolved" | awk -F ':::' '{print $3}')
  fi
else
  if [[ -n "${session_name:-}" && -n "${window_index:-}" && -n "${pane_index:-}" ]]; then
    resolved=$(resolve_by_locator "$session_name" "$window_index" "$pane_index")
    if [[ -z "$resolved" ]]; then
      exit 0
    fi
    sid=$(printf '%s' "$resolved" | awk -F ':::' '{print $1}')
    wid=$(printf '%s' "$resolved" | awk -F ':::' '{print $2}')
    pid=$(printf '%s' "$resolved" | awk -F ':::' '{print $3}')
  else
    exit 0
  fi
fi

tmux switch-client -t "$sid" \; select-window -t "$wid" \; select-pane -t "$pid"
