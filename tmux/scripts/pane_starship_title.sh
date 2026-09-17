#!/usr/bin/env bash
set -euo pipefail

# Args: <pane_pid> <pane_tty> <pane_title> <pane_width> <pane_path> <pane_cmd> <pane_watching>
pid="${1:-}"
pane_tty="${2:-}"
pane_title="${3:-}"
width="${4:-80}"
pane_path="${5:-$PWD}"
pane_cmd="${6:-}"
pane_watching="${7:-}"
ps_line=""

cache_dir="$HOME/.cache/tmux/pane-titles"
mkdir -p "$cache_dir" 2>/dev/null || true
cache_file="$cache_dir/$pid.cache"
cache_key="$pane_tty|$pane_title|$width|$pane_path|$pane_cmd|$pane_watching"
if [[ "$pid" =~ ^[0-9]+$ ]] && [[ -s "$cache_file" ]] && [[ -n $(find "$cache_file" -mtime -15s 2>/dev/null) ]]; then
  { IFS= read -r cached_key || true
    IFS= read -r cached_title || true
  } < "$cache_file"
  if [[ "$cached_key" == "key:$cache_key" ]]; then
    printf '%s' "$cached_title"
    exit 0
  fi
fi

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

if [[ "$pane_watching" == "1" ]]; then
  title="⏳ $title"
fi

if [[ "$pid" =~ ^[0-9]+$ ]]; then
  tmp="$cache_file.tmp$$"
  { printf 'key:%s\n%s' "$cache_key" "$title"; } > "$tmp" 2>/dev/null && mv "$tmp" "$cache_file" 2>/dev/null || true
fi

printf '%s' "$title"
