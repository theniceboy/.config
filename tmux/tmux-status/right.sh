#!/usr/bin/env bash
set -euo pipefail

status_bg=$(tmux show -gqv status-bg)
if [[ -z "$status_bg" || "$status_bg" == "default" ]]; then
  status_bg=black
fi

segment_bg="#3b4252"
segment_fg="#eceff4"
separator=""
right_cap="█"
hostname=$(hostname -s 2>/dev/null || hostname 2>/dev/null || printf 'host')
rainbarf_bg="#2e3440"
rainbarf_segment=""
rainbarf_toggle="${TMUX_RAINBARF:-1}"

case "$rainbarf_toggle" in
  0|false|FALSE|off|OFF|no|NO)
    rainbarf_toggle="0"
    ;;
  *)
    rainbarf_toggle="1"
    ;;
esac

if [[ "$rainbarf_toggle" == "1" ]] && command -v rainbarf >/dev/null 2>&1; then
  rainbarf_output=$(rainbarf --no-battery --no-remaining --no-bolt --tmux --rgb 2>/dev/null || true)
  rainbarf_output=${rainbarf_output//$'\n'/}
  if [[ -n "$rainbarf_output" ]]; then
    rainbarf_segment=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s]%s' \
      "$rainbarf_bg" "$status_bg" "$separator" \
      "$segment_fg" "$rainbarf_bg" "$rainbarf_output")
  fi
fi

host_prefix=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s] ' \
  "$segment_bg" "$status_bg" "$separator" \
  "$segment_fg" "$segment_bg")

if [[ -n "$rainbarf_segment" ]]; then
  host_prefix=$(printf '#[fg=%s,bg=%s] ' "$segment_fg" "$segment_bg")
fi

printf '%s%s%s #[fg=%s,bg=%s]%s' \
  "$rainbarf_segment" \
  "$host_prefix" \
  "$hostname" \
  "$segment_bg" "$status_bg" "$right_cap"
