#!/usr/bin/env bash
set -euo pipefail

# Config
TTL_SECONDS=${TMUX_CCUSAGE_TTL:-10}
LOCK_WAIT=${TMUX_CCUSAGE_LOCK_WAIT:-2}
TODAY=$(date +%F)

# Cache location (default to XDG cache)
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/tmux-ccusage"
mkdir -p "$CACHE_DIR"
CACHE_FILE="$CACHE_DIR/today-$TODAY.txt"
LOCK_DIR="$CACHE_DIR/today-$TODAY.lock"

get_mtime() {
  local f="$1"
  if command -v stat >/dev/null 2>&1; then
    # macOS: -f %m, GNU: -c %Y
    stat -f %m "$f" 2>/dev/null || stat -c %Y "$f" 2>/dev/null || echo 0
  else
    echo 0
  fi
}

now=$(date +%s)

# Serve cached value if fresh
if [[ -f "$CACHE_FILE" ]]; then
  mtime=$(get_mtime "$CACHE_FILE")
  if [[ -n "${mtime:-}" && $(( now - mtime )) -lt $TTL_SECONDS ]]; then
    cat "$CACHE_FILE"
    exit 0
  fi
fi

# Build command
cmd=(ccusage-codex daily -j -O -s "$TODAY" -u "$TODAY")
if [[ -n "${TMUX_CCUSAGE_TZ:-}" ]]; then
  cmd+=(-z "$TMUX_CCUSAGE_TZ")
fi

# Attempt to acquire lock; if locked, wait briefly and serve stale
if mkdir "$LOCK_DIR" 2>/dev/null; then
  trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

  # Fetch today's totals, offline pricing if available, tolerate failures
  json=$("${cmd[@]}" 2>/dev/null || true)

  cost=$(printf '%s' "${json:-}" | jq -r '.totals?.costUSD // 0' 2>/dev/null || printf '0')

  # Coerce to number and format to 2 decimals, with dollar sign only
  if command -v python3 >/dev/null 2>&1; then
    formatted=$(COST="$cost" python3 - << 'PY'
import os,sys
try:
    v=float(os.environ.get('COST','0'))
except Exception:
    v=0.0
print(f"${v:.2f}")
PY
    )
  else
    # Fallback shell formatting
    formatted="$(printf '$%.2f' "${cost:-0}")"
  fi

  tmp_file="$CACHE_FILE.$$"
  printf '%s\n' "$formatted" > "$tmp_file"
  mv "$tmp_file" "$CACHE_FILE"
  printf '%s\n' "$formatted"
else
  # Another process is updating; wait briefly for refresh, then serve cache
  start=$(date +%s)
  initial_mtime=""
  [[ -f "$CACHE_FILE" ]] && initial_mtime=$(get_mtime "$CACHE_FILE")
  while :; do
    sleep 0.1
    now=$(date +%s)
    (( now - start >= LOCK_WAIT )) && break
    new_mtime=""
    [[ -f "$CACHE_FILE" ]] && new_mtime=$(get_mtime "$CACHE_FILE")
    if [[ -n "$new_mtime" && "$new_mtime" != "$initial_mtime" ]]; then
      break
    fi
  done
  if [[ -f "$CACHE_FILE" ]]; then
    cat "$CACHE_FILE"
  else
    printf '$0.00\n'
  fi
fi
