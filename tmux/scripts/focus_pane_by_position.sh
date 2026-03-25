#!/usr/bin/env bash
set -euo pipefail

target="${1:-}"
window_id="${2:-}"

if [[ -z "$target" ]]; then
  echo "usage: $(basename "$0") <left|top-right|bottom-right> [window_id]" >&2
  exit 1
fi

if [[ -z "$window_id" ]]; then
  window_id="$(tmux display-message -p '#{window_id}')"
fi

tmux list-panes -t "$window_id" -F '#{pane_id} #{pane_left} #{pane_top}' | python3 -c '
import sys

target = sys.argv[1]
panes = []
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    pane_id, left, top = line.split()
    panes.append((pane_id, int(left), int(top)))

if not panes:
    sys.exit(1)

if target == "left":
    chosen = min(panes, key=lambda p: (p[1], p[2], p[0]))
elif target == "top-right":
    max_left = max(p[1] for p in panes)
    candidates = [p for p in panes if p[1] == max_left]
    chosen = min(candidates, key=lambda p: (p[2], p[0]))
elif target == "bottom-right":
    max_left = max(p[1] for p in panes)
    candidates = [p for p in panes if p[1] == max_left]
    chosen = max(candidates, key=lambda p: (p[2], p[0]))
else:
    sys.exit(2)

print(chosen[0])
' "$target"
