# OpenCode Memory Management

## Problem

OpenCode sessions accumulate memory over time. Each session spawns multiple processes (opencode + dart/dartaotruntime) that can grow to 500MB-3GB each. Old/orphaned sessions persist indefinitely, consuming swap.

In one case: **79 processes using 48GB** (with only 36GB physical RAM, causing 37GB swap usage).

## Memory Breakdown

Each opencode session typically includes:
- 1 main `opencode` process (500MB-3GB)
- 1+ `dart` processes for LSP/tooling (200MB-1GB each)
- 1+ `node` parent processes (small)

Use `top` with CMPRS column to see true memory (RSS only shows resident, not compressed/swapped):

```bash
top -l 1 -o cmprs -stats pid,command,mem,cmprs | grep -E "opencode|dart"
```

## Diagnostic Commands

### Check total opencode/dart memory
```bash
top -l 1 -o cmprs -n 200 -stats pid,command,mem,cmprs | grep -E "opencode|dart" | awk '
{
  mem = $3; cmprs = $4
  gsub(/M/, "", mem); gsub(/M/, "", cmprs)
  total_mem += mem
  total_cmprs += cmprs
  count++
  print $1, $2, $3, $4
}
END {
  print "-------------------------------------------"
  print "TOTAL: " count " processes"
  print "MEM:   " total_mem/1024 " GB"
  print "CMPRS: " total_cmprs/1024 " GB"
}'
```

### Check swap usage
```bash
sysctl vm.swapusage
```

### List opencode processes with tmux location
```bash
for pid in $(ps -eo pid,comm | grep opencode | awk '{print $1}'); do
  ppid=$(ps -o ppid= -p $pid 2>/dev/null | tr -d ' ')
  tty=$(ps -o tty= -p $ppid 2>/dev/null | tr -d ' ')
  tmux_info="N/A"
  if [ -n "$tty" ] && [ "$tty" != "??" ]; then
    tmux_info=$(tmux list-panes -a -F '#{pane_tty} #{session_name}:#{window_name}' 2>/dev/null | grep "/dev/$tty" | awk '{print $2}')
  fi
  etime=$(ps -o etime= -p $pid 2>/dev/null | tr -d ' ')
  mem=$(ps -o rss= -p $pid 2>/dev/null | awk '{printf "%.0fMB", $1/1024}')
  printf "PID:%-6s Age:%-15s Mem:%-8s Tmux:%s\n" "$pid" "$etime" "$mem" "${tmux_info:-N/A}"
done
```

### Find orphaned processes (PPID=1)
```bash
ps -eo pid,ppid,etime,rss,comm | grep -E "opencode|dart" | awk '$2 == 1 {printf "PID:%s Age:%s Mem:%.0fMB (orphaned)\n", $1, $3, $4/1024}'
```

## Cleanup Procedures

### Kill all opencode/dart except current session
```bash
# First, find your current session's PID
tmux list-panes -t "YOUR-SESSION:WINDOW" -F '#{pane_pid}'
# Then find the opencode child process

# Kill all except that PID
current_pid=XXXXX
ps -eo pid,comm | grep -E "opencode|dart" | awk -v cur=$current_pid '$1 != cur {print $1}' | xargs kill -9
```

### Kill only orphaned processes
```bash
ps -eo pid,ppid,comm | grep -E "opencode|dart" | awk '$2 == 1 {print $1}' | xargs kill
```

### Kill processes older than N days
```bash
# Example: kill opencode processes older than 1 day
ps -eo pid,etime,comm | grep opencode | awk '$2 ~ /-/ {print $1}' | xargs kill
```

## Session Restoration

Sessions are persisted to `~/.local/share/opencode/storage/session/`. Killing a process does NOT lose conversation history.

### Restore last session for current directory
```bash
opencode -c
# or
opencode --continue
```

### Restore specific session by ID
```bash
opencode -s ses_XXXXXX
# or
opencode --session ses_XXXXXX
```

### List available sessions
```bash
opencode session list
opencode session list --max-count 10
opencode session list --format json
```

### From within TUI
- Press `Ctrl+x l` or type `/sessions` to list and switch sessions

## Limitations

- No built-in way to map a running PID to its session ID from outside
- `opencode -c` only restores the **most recent** session for that directory
- If multiple sessions exist in the same directory, you need the session ID

## Prevention

1. Close opencode sessions when done (don't leave them running for days)
2. Use different directories for different tasks so `-c` works reliably
3. Periodically check for orphaned processes
4. Consider adding a cron job to kill old sessions:

```bash
# Add to crontab: kill opencode processes older than 2 days
0 */6 * * * ps -eo pid,etime,comm | grep opencode | awk '$2 ~ /^[2-9]-/ || $2 ~ /^[0-9][0-9]-/ {print $1}' | xargs kill 2>/dev/null
```

## Data Locations

- Config: `~/.config/opencode/`
- Sessions: `~/.local/share/opencode/storage/session/`
- Projects: `~/.local/share/opencode/storage/project/`
- Logs: `~/.local/share/opencode/log/`
