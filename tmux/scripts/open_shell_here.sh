#!/usr/bin/env bash
set -euo pipefail

mode="${1:-}"
target="${2:-}"
target_path="${3:-}"
shift 3 || true

if [[ -z "$target_path" || ! -d "$target_path" ]]; then
  target_path="$HOME"
fi

printf -v start_cmd 'cd %q && exec ${SHELL:-/bin/zsh} -l' "$target_path"

case "$mode" in
  split)
    exec tmux split-window "$@" -t "$target" -c "$target_path" "$start_cmd"
    ;;
  new-window)
    exec tmux new-window "$@" -t "$target" -c "$target_path" "$start_cmd"
    ;;
  *)
    printf 'open_shell_here: unknown mode %s\n' "$mode" >&2
    exit 1
    ;;
esac
