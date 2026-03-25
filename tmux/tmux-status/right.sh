#!/usr/bin/env bash
set -euo pipefail

min_width=${TMUX_RIGHT_MIN_WIDTH:-90}
width="${1:-}"
status_bg="${2:-}"
current_session="${3:-}"
current_window_index="${4:-}"
current_pane_id="${5:-}"
current_window_id="${6:-}"

if [[ -z "${width:-}" || "$width" == "0" ]]; then
  width=${COLUMNS:-}
fi
if [[ -n "${width:-}" && "$width" =~ ^[0-9]+$ ]]; then
  if (( width < min_width )); then
    exit 0
  fi
fi

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
mem_sess_bg="#434c5e"
mem_sess_fg="#eceff4"
mem_total_bg="#3b4252"
mem_total_fg="#eceff4"
mem_pane_val=""
mem_win_val=""
mem_sess_val=""
mem_total_val=""
mem_script="$HOME/.config/tmux/tmux-status/mem_usage.sh"
memory_toggle="${TMUX_STATUS_MEMORY:-1}"
case "$memory_toggle" in
  0|false|FALSE|off|OFF|no|NO) memory_toggle="0" ;;
  *) memory_toggle="1" ;;
esac
if [[ "$memory_toggle" == "1" ]] && [[ -x "$mem_script" ]]; then
  if [[ -n "$current_session" && -n "$current_window_index" && -n "$current_pane_id" ]]; then
    mem_output=$("$mem_script" "$current_session" "$current_window_index" "$current_pane_id" 2>/dev/null || true)
    mem_pane_val=$(sed -n '1p' <<< "$mem_output")
    mem_win_val=$(sed -n '2p' <<< "$mem_output")
    mem_sess_val=$(sed -n '3p' <<< "$mem_output")
    mem_total_val=$(sed -n '4p' <<< "$mem_output")
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
  notes_output=$("$notes_count_script" "$current_window_id" 2>/dev/null || true)
fi

# Flash-MoE
flashmoe_phase=""
flashmoe_tok_s=""
flashmoe_prompt_tokens=""
flashmoe_updated_ms=""
flashmoe_metrics="$HOME/.flash-moe/tmux_metrics"
if [[ -r "$flashmoe_metrics" ]]; then
  while IFS='=' read -r key value; do
    case "$key" in
      phase) flashmoe_phase="$value" ;;
      tok_s) flashmoe_tok_s="$value" ;;
      prompt_tokens) flashmoe_prompt_tokens="$value" ;;
      updated_ms) flashmoe_updated_ms="$value" ;;
    esac
  done < "$flashmoe_metrics"
fi
if [[ -n "$flashmoe_updated_ms" && "$flashmoe_updated_ms" =~ ^[0-9]+$ ]]; then
  now_ms=$(( $(date +%s) * 1000 ))
  age_ms=$(( now_ms - flashmoe_updated_ms ))
  if (( age_ms > 10000 )) && [[ "$flashmoe_phase" == "gen" || "$flashmoe_phase" == "prefill" ]]; then
    flashmoe_phase="idle"
  fi
fi

# --- Segment building (left to right: rainbarf | pane_mem | win_mem | notes | flashmoe | host) ---

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

mem_sess_segment=""
if [[ -n "$mem_sess_val" ]]; then
  mem_sess_segment=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s] %s ' \
    "$mem_sess_bg" "$prev_bg" "$separator" \
    "$mem_sess_fg" "$mem_sess_bg" "$mem_sess_val")
  prev_bg="$mem_sess_bg"
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

flashmoe_segment=""
flashmoe_bg="#88c0d0"
flashmoe_fg="#1d1f21"
flashmoe_label=""
if [[ "$flashmoe_phase" == "prefill" ]]; then
  flashmoe_bg="#ebcb8b"
  if [[ "$flashmoe_prompt_tokens" =~ ^[0-9]+$ ]] && (( flashmoe_prompt_tokens > 0 )); then
    flashmoe_label=" FM prefill:${flashmoe_prompt_tokens} "
  else
    flashmoe_label=" FM prefill "
  fi
elif [[ "$flashmoe_phase" == "gen" ]]; then
  flashmoe_bg="#a3be8c"
  if [[ -n "$flashmoe_tok_s" && "$flashmoe_tok_s" != "0.00" ]]; then
    flashmoe_label=" FM ${flashmoe_tok_s} tok/s "
  else
    flashmoe_label=" FM gen "
  fi
fi
if [[ -n "$flashmoe_label" ]]; then
  flashmoe_segment=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s,bold]%s' \
    "$flashmoe_bg" "$prev_bg" "$separator" \
    "$flashmoe_fg" "$flashmoe_bg" "$flashmoe_label")
  prev_bg="$flashmoe_bg"
fi

host_prefix=$(printf '#[fg=%s,bg=%s]%s#[fg=%s,bg=%s] ' \
  "$host_bg" "$prev_bg" "$separator" \
  "$host_fg" "$host_bg")

printf '%s%s%s%s%s%s%s%s%s #[fg=%s,bg=%s]%s' \
  "$rainbarf_segment" \
  "$mem_pane_segment" \
  "$mem_win_segment" \
  "$mem_sess_segment" \
  "$mem_total_segment" \
  "$notes_segment" \
  "$flashmoe_segment" \
  "$host_prefix" \
  "$hostname" \
  "$host_bg" "$status_bg" "$right_cap"
