#!/usr/bin/env bash
set -euo pipefail

current_session_id="${1:-}"
current_session_name="${2:-}"

detect_current_session_id=$(tmux display-message -p '#{session_id}')
detect_current_session_name=$(tmux display-message -p '#{session_name}')

if [[ -z "$current_session_id" ]]; then
  current_session_id="$detect_current_session_id"
fi

if [[ -z "$current_session_name" ]]; then
  current_session_name="$detect_current_session_name"
fi

status_bg=$(tmux show -gqv status-bg)
if [[ -z "$status_bg" || "$status_bg" == "default" ]]; then
  status_bg=black
fi

inactive_bg="#373b41"
inactive_fg="#c5c8c6"
active_bg="#b294bb"
active_fg="#1d1f21"
separator=""
left_cap="█"
max_width=18

# width-based label policy: when narrow (<80 cols by default),
# show title for active session and only the numeric index for inactive ones.
left_narrow_width=${TMUX_LEFT_NARROW_WIDTH:-80}
term_width=$(tmux display-message -p '#{client_width}' 2>/dev/null || true)
if [[ -z "${term_width:-}" || "$term_width" == "0" ]]; then
  term_width=$(tmux display-message -p '#{window_width}' 2>/dev/null || true)
fi
if [[ -z "${term_width:-}" || "$term_width" == "0" ]]; then
  term_width=${COLUMNS:-}
fi
is_narrow=0
if [[ -n "${term_width:-}" && "$term_width" =~ ^[0-9]+$ ]]; then
  if (( term_width < left_narrow_width )); then
    is_narrow=1
  fi
fi

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

tracker_pending_sessions=""
tracker_running_sessions=""
TRACKER_CACHE_DIR="$HOME/.config/tmux/cache"
TRACKER_CACHE_FILE="$TRACKER_CACHE_DIR/tracker_state_lines"
TRACKER_CACHE_TTL=1

file_mtime() {
  local path="$1"
  [[ ! -e "$path" ]] && return 1

  local mtime
  mtime=$(stat -f %m "$path" 2>/dev/null || true)
  if [[ -n "$mtime" ]]; then
    printf '%s\n' "$mtime"
    return 0
  fi

  mtime=$(stat -c %Y "$path" 2>/dev/null || true)
  if [[ -n "$mtime" ]]; then
    printf '%s\n' "$mtime"
    return 0
  fi

  return 1
}

write_tracker_cache() {
  mkdir -p "$TRACKER_CACHE_DIR"
  local tmp_file="$TRACKER_CACHE_FILE.$$"
  printf '%s\n' "$1" > "$tmp_file"
  mv "$tmp_file" "$TRACKER_CACHE_FILE"
}

reset_tracker_state() {
  tracker_pending_sessions=""
  tracker_running_sessions=""
}

load_tracker_info() {
  reset_tracker_state

  if ! command -v jq >/dev/null 2>&1; then
    return
  fi

  local tracker_lines=""
  local now
  now=$(date +%s 2>/dev/null || true)

  if [[ -f "$TRACKER_CACHE_FILE" && -n "$now" ]]; then
    local cache_mtime
    cache_mtime=$(file_mtime "$TRACKER_CACHE_FILE" 2>/dev/null || true)
    if [[ -n "$cache_mtime" ]]; then
      local age=$(( now - cache_mtime ))
      if (( age <= TRACKER_CACHE_TTL )); then
        tracker_lines=$(cat "$TRACKER_CACHE_FILE" 2>/dev/null || true)
      fi
    fi
  fi

  if [[ -z "$tracker_lines" ]]; then
    local tracker_state
    tracker_state=$(~/.config/agent-tracker/bin/tracker-client state 2>/dev/null || true)
    if [[ -z "$tracker_state" ]]; then
      rm -f "$TRACKER_CACHE_FILE" 2>/dev/null || true
      return
    fi

    tracker_lines=$(printf '%s\n' "$tracker_state" | jq -r 'select(.kind == "state") | .tasks[] | "\(.session_id)|\(.status)|\(.acknowledged)"' 2>/dev/null || true)
    if [[ -n "$tracker_lines" ]]; then
      write_tracker_cache "$tracker_lines"
    else
      rm -f "$TRACKER_CACHE_FILE" 2>/dev/null || true
      return
    fi
  fi

  while IFS='|' read -r session_id status acknowledged; do
    [[ -z "$session_id" ]] && continue
    case "$status" in
      in_progress)
        tracker_running_sessions+="$session_id"$'\n'
        ;;
      completed)
        if [[ "$acknowledged" == "false" ]]; then
          tracker_pending_sessions+="$session_id"$'\n'
        fi
        ;;
    esac
  done <<< "$tracker_lines"
}

value_in_list() {
  local needle="$1"
  local list="$2"
  if [[ -z "$needle" || -z "$list" ]]; then
    return 1
  fi
  while IFS= read -r line; do
    [[ "$line" == "$needle" ]] && return 0
  done <<< "$list"
  return 1
}

session_has_pending() {
  value_in_list "$1" "$tracker_pending_sessions"
}

session_has_running() {
  value_in_list "$1" "$tracker_running_sessions"
}

load_tracker_info

sessions=$(tmux list-sessions -F '#{session_id}::#{session_name}' 2>/dev/null || true)
if [[ -z "$sessions" ]]; then
  exit 0
fi

rendered=""
prev_bg=""
current_session_id_norm=$(normalize_session_id "$current_session_id")
current_session_trimmed=$(trim_label "$current_session_name")
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
  if [[ "$session_id" == "$current_session_id" || "$session_id_norm" == "$current_session_id_norm" || "$trimmed_name" == "$current_session_trimmed" ]]; then
    is_current=1
    segment_bg="$active_bg"
    segment_fg="$active_fg"
  fi

  if (( is_narrow == 1 )); then
    if (( is_current == 1 )); then
      label="$trimmed_name"  # active: show TITLE (trim N-)
    else
      idx=$(extract_index "$name")
      if [[ -n "$idx" ]]; then
        label="$idx"
      else
        label="$trimmed_name"
      fi
    fi
  else
    label="$trimmed_name"      # wide: current behavior (TITLE everywhere)
  fi
  if (( ${#label} > max_width )); then
    label="${label:0:max_width-1}…"
  fi

  indicator=""
  if session_has_pending "$session_id"; then
    indicator=" #[fg=#a6e3a1,bg=${segment_bg}]●#[fg=${segment_fg},bg=${segment_bg}]"
  elif session_has_running "$session_id"; then
    indicator=" #[fg=#f9e2af,bg=${segment_bg}]●#[fg=${segment_fg},bg=${segment_bg}]"
  fi

  if [[ -z "$prev_bg" ]]; then
    rendered+="#[fg=${segment_bg},bg=${status_bg}]${left_cap}"
  else
    rendered+="#[fg=${prev_bg},bg=${segment_bg}]${separator}"
  fi
  rendered+="#[fg=${segment_fg},bg=${segment_bg}] ${label}${indicator} "
  prev_bg="$segment_bg"
done <<< "$sessions"

if [[ -n "$prev_bg" ]]; then
  rendered+="#[fg=${prev_bg},bg=${status_bg}]${separator}"
fi

printf '%s' "$rendered"
