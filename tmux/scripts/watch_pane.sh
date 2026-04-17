#!/usr/bin/env bash
set -euo pipefail

pane_id="$1"
window_id="$2"

[[ -z "$pane_id" || -z "$window_id" ]] && exit 1

pane_pid=$(tmux display-message -p -t "$pane_id" '#{pane_pid}' 2>/dev/null || true)
[[ -z "$pane_pid" ]] && exit 0

pane_shell=$(ps -o comm= -p "$pane_pid" 2>/dev/null | sed 's|.*/||; s/^-//')
[[ -z "$pane_shell" ]] && exit 0

current_cmd=$(tmux display-message -p -t "$pane_id" '#{pane_current_command}' 2>/dev/null || true)
[[ -z "$current_cmd" ]] && exit 0

clear_pane_watch() {
  tmux set-option -p -u -t "$pane_id" @pane_watching 2>/dev/null || true
}

trap clear_pane_watch EXIT

notify_completion() {
  local tracker_bin message
  tracker_bin="$HOME/.config/agent-tracker/bin/agent"
  [[ -x "$tracker_bin" ]] || return 0
  message="${current_cmd} finished"
  "$tracker_bin" tracker command -window-id "$window_id" -pane "$pane_id" -summary "$message" notify >/dev/null 2>&1 || true
}

if [[ "$current_cmd" == "$pane_shell" ]]; then
  exit 0
fi

tmux set -w -t "$window_id" @watching 1 2>/dev/null || true
tmux set-option -p -t "$pane_id" @pane_watching 1 2>/dev/null || true
tmux refresh-client -S

while true; do
  sleep 1
  watching=$(tmux show -wv -t "$window_id" @watching 2>/dev/null || true)
  [[ "$watching" != "1" ]] && exit 0
  cmd=$(tmux display-message -p -t "$pane_id" '#{pane_current_command}' 2>/dev/null || true)
  if [[ -z "$cmd" || "$cmd" == "$pane_shell" ]]; then
    break
  fi
done

tmux set -wu -t "$window_id" @watching 2>/dev/null || true
tmux set -w -t "$window_id" @unread 1 2>/dev/null || true
tmux refresh-client -S
notify_completion
