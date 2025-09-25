#!/usr/bin/env bash
set -euo pipefail

# Args: <pane_pid> <pane_width> <pane_path> <pane_cmd>
pid="${1:-}"
width="${2:-80}"
pane_path="${3:-$PWD}"
pane_cmd="${4:-}"

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
  local cfg
  cfg="${STARSHIP_TMUX_CONFIG:-$HOME/.config/starship-tmux.toml}"
  STARSHIP_LOG=error STARSHIP_CONFIG="$cfg" \
    starship prompt --terminal-width "$width" | strip_wrappers | tr -d '\n'
}

fallback() {
  # <cmd> — <last dir>
  local last_dir
  last_dir="${pane_path##*/}"
  printf '%s — %s' "$pane_cmd" "$last_dir"
}

if command -v starship >/dev/null 2>&1; then
  (cd "$pane_path" && run_starship) || fallback
else
  fallback
fi

