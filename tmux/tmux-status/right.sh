#!/usr/bin/env bash
set -euo pipefail

min_width=${TMUX_RIGHT_MIN_WIDTH:-90}
width=$(tmux display-message -p '#{client_width}' 2>/dev/null || true)
if [[ -z "${width:-}" || "$width" == "0" ]]; then
  width=$(tmux display-message -p '#{window_width}' 2>/dev/null || true)
fi
if [[ -z "${width:-}" || "$width" == "0" ]]; then
  width=${COLUMNS:-}
fi
if [[ -n "${width:-}" && "$width" =~ ^[0-9]+$ ]]; then
  if (( width < min_width )); then
    exit 0
  fi
fi

status_bg=$(tmux show -gqv status-bg)
if [[ -z "$status_bg" || "$status_bg" == "default" ]]; then
  status_bg=black
fi

segment_fg="#eceff4"
host_bg="${TMUX_THEME_COLOR:-#b294bb}"
host_fg="#1d1f21"
separator=""
right_cap="█"
hostname=$(hostname -s 2>/dev/null || hostname 2>/dev/null || printf 'host')

# --- Data gathering ---

# Memory usage
mem_pane_bg="#5e81ac"
mem_pane_fg="#eceff4"
mem_win_bg="#4c566a"
mem_win_fg="#eceff4"
mem_total_bg="#3b4252"
mem_total_fg="#eceff4"
mem_pane_val=""
mem_win_val=""
mem_total_val=""
mem_script="$HOME/.config/tmux/tmux-status/mem_usage.sh"
if [[ -x "$mem_script" ]]; then
  IFS=$'\t' read -r _sess _widx _pid < <(
    tmux display-message -p '#{session_name}	#{window_index}	#{pane_id}' 2>/dev/null || echo ""
  )
  if [[ -n "${_sess:-}" && -n "${_widx:-}" && -n "${_pid:-}" ]]; then
    mem_output=$("$mem_script" "$_sess" "$_widx" "$_pid" 2>/dev/null || true)
    mem_pane_val=$(sed -n '1p' <<< "$mem_output")
    mem_win_val=$(sed -n '2p' <<< "$mem_output")
    mem_total_val=$(sed -n '3p' <<< "$mem_output")
  fi
fi

# Rainbarf
rainbarf_bg="#2e3440"
rainbarf_segment=""
rainbarf_toggle="${TMUX_RAINBARF:-1}"
case "$rainbarf_toggle" in
  0|false|FALSE|off|OFF|no|NO) rainbarf_toggle="0" ;;
  *) rainbarf_toggle="1" ;;
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

# Notes
notes_output=""
notes_count_script="$HOME/.config/tmux/tmux-status/notes_count.sh"
if [[ -x "$notes_count_script" ]]; then
  notes_output=$("$notes_count_script" 2>/dev/null || true)
fi

# --- Segment building (left to right: rainbarf | pane_mem | win_mem | notes | host) ---

# Track the rightmost background for connector chaining
prev_bg="$status_bg"
[[ -n "$rainbarf_segment" ]] && prev_bg="$rainbarf_bg"

mem_pane_segment=""
if [[ -n "$mem_pane_val" ]]; then
  mem_pane_segment=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s] %s ' \
    "$mem_pane_bg" "$prev_bg" "$separator" \
    "$mem_pane_fg" "$mem_pane_bg" "$mem_pane_val")
  prev_bg="$mem_pane_bg"
fi

mem_win_segment=""
if [[ -n "$mem_win_val" ]]; then
  mem_win_segment=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s] %s ' \
    "$mem_win_bg" "$prev_bg" "$separator" \
    "$mem_win_fg" "$mem_win_bg" "$mem_win_val")
  prev_bg="$mem_win_bg"
fi

mem_total_segment=""
if [[ -n "$mem_total_val" ]]; then
  mem_total_segment=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s] %s ' \
    "$mem_total_bg" "$prev_bg" "$separator" \
    "$mem_total_fg" "$mem_total_bg" "$mem_total_val")
  prev_bg="$mem_total_bg"
fi

notes_segment=""
if [[ -n "$notes_output" ]]; then
  notes_bg="#cc6666"
  notes_fg="#1d1f21"
  notes_segment=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s,bold]%s#[default]' \
    "$notes_bg" "$prev_bg" "$separator" \
    "$notes_fg" "$notes_bg" "$notes_output")
  prev_bg="$notes_bg"
fi

host_prefix=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s] ' \
  "$host_bg" "$prev_bg" "$separator" \
  "$host_fg" "$host_bg")

printf '%s%s%s%s%s%s%s #[fg=%s,bg=%s]%s' \
  "$rainbarf_segment" \
  "$mem_pane_segment" \
  "$mem_win_segment" \
  "$mem_total_segment" \
  "$notes_segment" \
  "$host_prefix" \
  "$hostname" \
  "$host_bg" "$status_bg" "$right_cap"
