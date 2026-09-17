droid() {
  local real_droid
  real_droid=$(whence -p droid)
  if [ -z "$real_droid" ]; then
    print -u2 "droid: executable not found"
    return 1
  fi

  local watcher="${HOME}/.factory/bin/droid_tracker_watch.py"
  local watcher_pid=""
  local started_at=""

  if [ -n "$TMUX" ] && [ -x "$watcher" ]; then
    started_at=$(python3 - <<'PY'
import time
print(time.time())
PY
)
    "$watcher" --cwd "$PWD" --started-at "$started_at" >/dev/null 2>&1 &
    watcher_pid=$!
  fi

  "$real_droid" "$@"
  local exit_code=$?

  if [ -n "$watcher_pid" ]; then
    kill -TERM "$watcher_pid" >/dev/null 2>&1 || true
    wait "$watcher_pid" 2>/dev/null || true
  fi

  return $exit_code
}
