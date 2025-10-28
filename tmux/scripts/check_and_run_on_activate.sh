#!/usr/bin/env bash
# Check for on-tmux-window-activate.sh in current dir or parent dir and run it

pane_dir="${1:-$PWD}"

# Check current directory
if [[ -x "$pane_dir/on-tmux-window-activate.sh" ]]; then
  exec "$pane_dir/on-tmux-window-activate.sh"
fi

# Check parent directory
parent_dir=$(dirname "$pane_dir")
if [[ -x "$parent_dir/on-tmux-window-activate.sh" ]]; then
  exec "$parent_dir/on-tmux-window-activate.sh"
fi

exit 0
