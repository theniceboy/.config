#!/usr/bin/env bash
set -euo pipefail

# Args: <pane_pid> <pane_id> <pane_tty> <pane_title> <pane_width> <pane_path> <pane_cmd>
pid="${1:-}"
pane_id="${2:-}"
pane_tty="${3:-}"
pane_title="${4:-}"
width="${5:-80}"
pane_path="${6:-$PWD}"
pane_cmd="${7:-}"
ps_line=""

# Best-effort: inherit venv/conda from the pane's process env
if [[ -n "$pid" ]]; then
  ps_line=$(ps e -p "$pid" -o command= 2>/dev/null || true)
  if [[ -n "$ps_line" ]]; then
    venv=$(printf '%s' "$ps_line" | sed -n 's/.*[[:space:]]VIRTUAL_ENV=\([^[:space:]]*\).*/\1/p' | tail -n1)
    conda_env=$(printf '%s' "$ps_line" | sed -n 's/.*[[:space:]]CONDA_DEFAULT_ENV=\([^[:space:]]*\).*/\1/p' | tail -n1)
    conda_prefix=$(printf '%s' "$ps_line" | sed -n 's/.*[[:space:]]CONDA_PREFIX=\([^[:space:]]*\).*/\1/p' | tail -n1)
    [[ -n "$venv" ]] && export VIRTUAL_ENV="$venv"
    [[ -n "$conda_env" ]] && export CONDA_DEFAULT_ENV="$conda_env"
    [[ -n "$conda_prefix" ]] && export CONDA_PREFIX="$conda_prefix"
  fi
fi

strip_wrappers() {
  # 1) strip ANSI, 2) strip bash \[\] and zsh %{ %}
  perl -pe 's/\e\[[\d;]*[[:alpha:]]//g' | sed -E 's/\\\[|\\\]//g; s/%\{|%\}//g'
}

run_starship() {
  local prompt_width cfg
  prompt_width="${1:-$width}"
  cfg="${STARSHIP_TMUX_CONFIG:-$HOME/.config/starship-tmux.toml}"
  STARSHIP_LOG=error STARSHIP_CONFIG="$cfg" \
    starship prompt --terminal-width "$prompt_width" | strip_wrappers | tr -d '\n'
}

trim_to_width() {
  local text max
  text="$1"
  max="$2"
  if (( ${#text} <= max )); then
    printf '%s' "$text"
    return
  fi
  if (( max <= 1 )); then
    printf ''
    return
  fi
  printf '%s…' "${text:0:$((max - 1))}"
}

load_option() {
  local option value
  option="$1"
  [[ -n "$pane_id" ]] || return 0
  value=$(tmux display-message -p -t "$pane_id" "#{${option}}" 2>/dev/null | tr -d '\r\n' || true)
  value=$(printf '%s' "$value" | perl -0pe 's/\s+/ /g; s/^\s+|\s+$//g')
  printf '%s' "$value"
}

clear_opencode_state() {
  [[ -n "$pane_id" ]] || return 0
  tmux set-option -p -u -t "$pane_id" @op_work_theme 2>/dev/null || true
  tmux set-option -p -u -t "$pane_id" @op_work_now 2>/dev/null || true
  tmux set-option -p -u -t "$pane_id" @op_work_summary 2>/dev/null || true
  tmux set-option -p -u -t "$pane_id" @op_question_pending 2>/dev/null || true
}

opencode_active() {
  local tty_name tty_ps
  [[ "$pane_title" == OC\ \|* ]] && return 0
  [[ "$pane_cmd" == "op" || "$pane_cmd" == "opencode" ]] && return 0
  tty_name="${pane_tty#/dev/}"
  tty_ps=""
  if [[ -n "$tty_name" ]]; then
    tty_ps=$(ps -t "$tty_name" -o command= 2>/dev/null || true)
  elif [[ -n "$ps_line" ]]; then
    tty_ps="$ps_line"
  fi
  [[ "$pane_cmd" == "node" && "$tty_ps" == *"/bin/opencode"* ]] && return 0
  [[ "$pane_cmd" == "node" && "$tty_ps" == *"opencode-ai/bin/.opencode"* ]] && return 0
  return 1
}

fallback() {
  # <cmd> — <last dir>
  local last_dir
  last_dir="${pane_path##*/}"
  printf '%s — %s' "$pane_cmd" "$last_dir"
}

if command -v starship >/dev/null 2>&1; then
  title=$(cd "$pane_path" && run_starship) || title=$(fallback)
else
  title=$(fallback)
fi

theme=$(load_option "@op_work_theme")
if [[ -z "$theme" ]]; then
  theme=$(load_option "@op_work_summary")
fi
now=$(load_option "@op_work_now")
question_pending=$(load_option "@op_question_pending")
pane_watching=$(load_option "@pane_watching")

if [[ "$pane_watching" == "1" ]]; then
  title="⏳ $title"
fi

if ! opencode_active; then
  if [[ -n "$theme" || -n "$now" || -n "$question_pending" ]]; then
    clear_opencode_state
  fi
  printf '%s' "$title"
  exit 0
fi

summary_display=""
if [[ -n "$theme" ]]; then
  summary_display="[$theme]"
fi
if [[ -n "$now" ]]; then
  if [[ -n "$summary_display" ]]; then
    summary_display="$summary_display  ↳ $now"
  else
    summary_display="↳ $now"
  fi
fi

if [[ -z "$summary_display" ]]; then
  printf '%s' "$title"
  exit 0
fi

if [[ "$pane_watching" == "1" ]]; then
  summary_display="⏳ $summary_display"
  title=${title#⏳ }
fi

reserved_width=$((${#summary_display} + 3))
prompt_width=$((width - reserved_width))
if (( prompt_width < 16 )); then
  summary_display=$(trim_to_width "$summary_display" "$width")
  printf '%s' "$summary_display"
  exit 0
fi

if command -v starship >/dev/null 2>&1; then
  title=$(cd "$pane_path" && run_starship "$prompt_width") || title=$(fallback)
fi

printf '%s · %s' "$summary_display" "$title"
