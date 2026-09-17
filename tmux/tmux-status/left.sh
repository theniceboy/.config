#!/usr/bin/env bash
set -euo pipefail

current_session_id="${1:-}"
current_session_name="${2:-}"
term_width="${3:-}"
status_bg="${4:-}"

[[ -z "$status_bg" || "$status_bg" == "default" ]] && status_bg=black
[[ ! "$term_width" =~ ^[0-9]+$ ]] && term_width=100

inactive_bg="#373b41"
inactive_fg="#c5c8c6"
active_bg="${TMUX_THEME_COLOR:-#b294bb}"
active_fg="#1d1f21"
separator=""
left_cap="█"
max_width=18

left_narrow_width=${TMUX_LEFT_NARROW_WIDTH:-80}
is_narrow=0
[[ "$term_width" =~ ^[0-9]+$ ]] && (( term_width < left_narrow_width )) && is_narrow=1

normalize_session_id() {
  local value="$1"
  value="${value#\$}"
  printf '%s' "$value"
}

trim_label() {
  local value="$1"
  if [[ "$value" =~ ^[0-9]+-(.*)$ ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
  else
    printf '%s' "$value"
  fi
}

extract_index() {
  local value="$1"
  if [[ "$value" =~ ^([0-9]+)-.*$ ]]; then
    printf '%s' "${BASH_REMATCH[1]}"
  else
    printf ''
  fi
}




sessions=$(tmux list-sessions -F '#{session_id}::#{session_name}' 2>/dev/null || true)
if [[ -z "$sessions" ]]; then
  exit 0
fi

sessions=$(printf '%s\n' "$sessions" | grep -Evi '^([^:]+)::([0-9]+-)?scratch$' || true)
if [[ -z "$sessions" ]]; then
  exit 0
fi

"$HOME/.config/tmux/tmux-status/tracker_cache.sh" 2>/dev/null || true

CACHE_FILE="/tmp/tmux-tracker-cache.json"
tracker_state=""
if [[ -f "$CACHE_FILE" ]]; then
  tracker_state=$(cat "$CACHE_FILE" 2>/dev/null || true)
fi

question_state=$(tmux list-panes -a -F '#{session_id}::#{@op_question_pending}' 2>/dev/null || true)
window_state=$(tmux list-windows -a -F '#{session_id}::#{@unread}::#{@watching}::#{@watch_failed}' 2>/dev/null || true)

tracker_icons=""
if [[ -n "$tracker_state" ]]; then
  tracker_icons=$(printf '%s' "$tracker_state" | jq -r '
    (.tasks // []) | group_by(.session_id // "")[]
    | (.[0].session_id // "") as $sid | select($sid != "")
    | if any(.[]; .status == "completed" and .acknowledged != true) then [$sid, "waiting"]
      elif any(.[]; .status == "in_progress") then [$sid, "in_progress"]
      else empty end | @tsv' 2>/dev/null || true)
fi

sep=$'\037'
q_enc=${question_state//$'\n'/$sep}
w_enc=${window_state//$'\n'/$sep}
t_enc=${tracker_icons//$'\n'/$sep}
s_enc=${sessions//$'\n'/$sep}

session_icons=$(awk -v q="$q_enc" -v w="$w_enc" -v t="$t_enc" -v sess="$s_enc" 'BEGIN {
  FS = "::"
  nq = split(q, L, "\037"); for (i = 1; i <= nq; i++) { split(L[i], a, "::"); if (a[2] == "1") Q[a[1]] = 1 }
  nw = split(w, L, "\037"); for (i = 1; i <= nw; i++) {
    split(L[i], b, "::"); s = b[1]
    if (b[2] == "1") { U[s] = 1; if (b[4] == "1") F[s] = 1 }
    if (b[3] == "1") W[s] = 1
  }
  nt = split(t, L, "\037"); for (i = 1; i <= nt; i++) {
    split(L[i], c, "\t")
    if (c[2] == "waiting") U[c[1]] = 1
    else if (c[2] == "in_progress") W[c[1]] = 1
  }
  ns = split(sess, L, "\037")
  for (i = 1; i <= ns; i++) {
    split(L[i], d, "::"); sid = d[1]; icon = ""
    if (Q[sid]) icon = "❓"
    else if (F[sid]) icon = "❌"
    else if (U[sid]) icon = "🔔"
    else if (W[sid]) icon = "⏳"
    print sid "\t" icon
  }
}' 2>/dev/null || true)
unset sep q_enc w_enc t_enc s_enc

rendered=""
prev_bg=""
current_session_id_norm=$(normalize_session_id "$current_session_id")
while IFS= read -r entry; do
  [[ -z "$entry" ]] && continue
  session_id="${entry%%::*}"
  name="${entry#*::}"
  [[ -z "$session_id" ]] && continue

  session_id_norm=$(normalize_session_id "$session_id")
  segment_bg="$inactive_bg"
  segment_fg="$inactive_fg"
  trimmed_name=$(trim_label "$name")
  is_current=0
  if [[ "$session_id" == "$current_session_id" || "$session_id_norm" == "$current_session_id_norm" ]]; then
    is_current=1
    segment_bg="$active_bg"
    segment_fg="$active_fg"
  fi

  if (( is_narrow == 1 )); then
    if (( is_current == 1 )); then
      label="$trimmed_name"
    else
      idx=$(extract_index "$name")
      if [[ -n "$idx" ]]; then
        label="$idx"
      else
        label="$trimmed_name"
      fi
    fi
  else
    label="$trimmed_name"
  fi
  if (( ${#label} > max_width )); then
    label="${label:0:max_width-1}…"
  fi

  task_icon=$(grep -m1 -F "$session_id"$'\t' <<< "$session_icons" 2>/dev/null | cut -f2) || task_icon=""

  if [[ -z "$prev_bg" ]]; then
    rendered+="#[fg=${segment_bg},bg=${status_bg}]${left_cap}"
  else
    rendered+="#[fg=${prev_bg},bg=${segment_bg}]${separator}"
  fi
  rendered+="#[fg=${segment_fg},bg=${segment_bg}] ${label}${task_icon} "
  prev_bg="$segment_bg"
done <<< "$sessions"

if [[ -n "$prev_bg" ]]; then
  rendered+="#[fg=${prev_bg},bg=${status_bg}]${separator}"
fi

printf '%s' "$rendered"
