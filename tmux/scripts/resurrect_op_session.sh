#!/usr/bin/env bash
# Resurrect strategy for `op` (opencode).
# Called by tmux-resurrect as: script <pane_full_command> <dir>
# Returns the command to run in the pane.

pane_full_command="$1"

session_name=$(tmux display-message -p '#{session_name}' 2>/dev/null || true)
window_index=$(tmux display-message -p '#{window_index}' 2>/dev/null || true)
pane_index=$(tmux display-message -p '#{pane_index}' 2>/dev/null || true)

state_dir="${XDG_STATE_HOME:-$HOME/.local/state}/op"

if [[ -n "$session_name" && -n "$window_index" && -n "$pane_index" ]]; then
    locator="${session_name}:${window_index}.${pane_index}"
    key="${locator//[^a-zA-Z0-9_]/_}"
    loc_file="$state_dir/loc_${key}"
    if [[ -f "$loc_file" ]]; then
        session_id=$(cat "$loc_file")
        if [[ -n "$session_id" ]]; then
            echo "OP_TRACKER_NOTIFY=1 op -s ${session_id}"
            exit 0
        fi
    fi
fi

echo "OP_TRACKER_NOTIFY=1 op"
