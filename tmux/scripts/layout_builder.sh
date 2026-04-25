#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  tmux display-message "layout command requires direction and path"
  exit 1
fi

dir="$1"
target_path="$2"

if [[ -z "$target_path" || ! -d "$target_path" ]]; then
  target_path="$HOME"
fi

printf -v start_cmd 'cd %q && exec ${SHELL:-/bin/zsh} -l' "$target_path"

run_tmux() {
  local output
  if ! output=$(tmux "$@" 2>&1); then
    tmux display-message "layout-${dir}: tmux $* failed: ${output}"
    exit 1
  fi
  printf '%s' "$output"
}

pane_count=0
while IFS='|' read -r pid ptop pleft; do
  case $pane_count in
    0)
      id1=$pid
      top1=$ptop
      left1=$pleft
      ;;
    1)
      id2=$pid
      top2=$ptop
      left2=$pleft
      ;;
  esac
  pane_count=$((pane_count + 1))
done < <(tmux list-panes -F "#{pane_id}|#{pane_top}|#{pane_left}")

if [ "$pane_count" -ne 2 ]; then
  tmux display-message "layout-${dir} expects exactly 2 panes"
  exit 0
fi

if [ "$top1" -le "$top2" ]; then
  top_id=$id1
  bottom_id=$id2
else
  top_id=$id2
  bottom_id=$id1
fi

if [ "$left1" -le "$left2" ]; then
  left_id=$id1
  right_id=$id2
else
  left_id=$id2
  right_id=$id1
fi

ensure_horizontal() {
  if [ "$top1" -ne "$top2" ]; then
    tmux display-message "layout-${dir} expects horizontal panes"
    exit 0
  fi
}

case "$dir" in
  right)
    new_id=$(run_tmux split-window -P -F '#{pane_id}' -h -c "$target_path" -t "$top_id" "$start_cmd")
    run_tmux join-pane -v -s "$bottom_id" -t "$top_id"
    run_tmux select-pane -t "$new_id"
    ;;
  left)
    new_id=$(run_tmux split-window -P -F '#{pane_id}' -h -b -c "$target_path" -t "$top_id" "$start_cmd")
    run_tmux join-pane -v -s "$bottom_id" -t "$top_id"
    run_tmux select-pane -t "$new_id"
    ;;
  up)
    ensure_horizontal
    run_tmux break-pane -d -s "$right_id"
    run_tmux select-pane -t "$left_id"
    new_id=$(run_tmux split-window -P -F '#{pane_id}' -v -b -c "$target_path" -t "$left_id" "$start_cmd")
    run_tmux join-pane -h -s "$right_id" -t "$left_id"
    run_tmux select-pane -t "$new_id"
    ;;
  down)
    ensure_horizontal
    run_tmux break-pane -d -s "$right_id"
    run_tmux select-pane -t "$left_id"
    new_id=$(run_tmux split-window -P -F '#{pane_id}' -v -c "$target_path" -t "$left_id" "$start_cmd")
    run_tmux join-pane -h -s "$right_id" -t "$left_id"
    run_tmux select-pane -t "$new_id"
    ;;
  *)
    tmux display-message "Unknown layout direction: ${dir}"
    exit 1
    ;;
esac
