#!/usr/bin/env bash
set -euo pipefail

info=$(tmux list-panes -F '#{pane_id}|#{pane_top}|#{pane_left}')

pane_ids=()
pane_tops=()
pane_lefts=()
while IFS='|' read -r pid top left; do
  [ -z "$pid" ] && continue
  pane_ids+=("$pid")
  pane_tops+=("$top")
  pane_lefts+=("$left")
done <<EOF
$info
EOF

if [ "${#pane_ids[@]}" -ne 2 ]; then
  tmux display-message "toggle-orientation: needs exactly 2 panes"
  exit 0
fi

top_a=${pane_tops[0]}
top_b=${pane_tops[1]}
left_a=${pane_lefts[0]}
left_b=${pane_lefts[1]}

if [ "$top_a" = "$top_b" ] && [ "$left_a" != "$left_b" ]; then
  tmux select-layout even-vertical
elif [ "$left_a" = "$left_b" ] && [ "$top_a" != "$top_b" ]; then
  tmux select-layout even-horizontal
else
  tmux select-layout tiled
fi
