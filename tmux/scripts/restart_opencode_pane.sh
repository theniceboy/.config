#!/usr/bin/env bash
set -euo pipefail

pane_id="${1:-}"
if [[ -z "$pane_id" ]]; then
  pane_id=$(tmux display-message -p '#{pane_id}')
fi

if [[ -z "$pane_id" ]]; then
  exit 1
fi

state_dir="${XDG_STATE_HOME:-$HOME/.local/state}/op"
pane_locator=$(tmux display-message -p -t "$pane_id" '#{session_name}:#{window_index}.#{pane_index}' 2>/dev/null || true)
state_file="$state_dir/loc_${pane_locator//[^a-zA-Z0-9_]/_}"

if [[ -z "$pane_locator" ]]; then
  tmux display-message "op-restart: unable to resolve pane locator"
  exit 0
fi

if [[ ! -f "$state_file" ]]; then
  tmux display-message "op-restart: no saved session for ${pane_locator}"
  exit 0
fi

IFS= read -r session_id < "$state_file"
if [[ -z "$session_id" ]]; then
  tmux display-message "op-restart: invalid session mapping for ${pane_locator}"
  exit 0
fi

wait_for_shell() {
  local cmd i
  for ((i = 0; i < 30; i++)); do
    cmd=$(tmux display-message -p -t "$pane_id" '#{pane_current_command}' 2>/dev/null || true)
    case "$cmd" in
      zsh|bash|sh|fish|nu)
        return 0
        ;;
    esac
    sleep 0.1
  done
  return 1
}

kill_opencode_on_tty() {
  local pane_tty tty_name pid
  local -a pids=()

  pane_tty=$(tmux display-message -p -t "$pane_id" '#{pane_tty}' 2>/dev/null || true)
  if [[ -z "$pane_tty" ]]; then
    return 0
  fi

  tty_name="${pane_tty#/dev/}"
  while IFS= read -r pid; do
    [[ -n "$pid" ]] && pids+=("$pid")
  done < <(ps -t "$tty_name" -o pid= -o args= 2>/dev/null | awk '/\/opt\/homebrew\/bin\/opencode|\/opt\/homebrew\/lib\/node_modules\/opencode-ai\/bin\/\.opencode|opencode-darwin-arm64\/bin\/opencode/ { print $1 }')

  if [[ ${#pids[@]} -eq 0 ]]; then
    return 0
  fi

  kill -TERM "${pids[@]}" 2>/dev/null || true
  sleep 0.3

  for pid in "${pids[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill -KILL "$pid" 2>/dev/null || true
    fi
  done
}

tmux send-keys -t "$pane_id" C-c

if ! wait_for_shell; then
  kill_opencode_on_tty
  if ! wait_for_shell; then
    tmux display-message "op-restart: pane ${pane_id} did not return to shell"
    exit 0
  fi
fi

tmux send-keys -t "$pane_id" "OP_TRACKER_NOTIFY=1 op -s ${session_id}" C-m
