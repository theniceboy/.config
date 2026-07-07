# Agent Tracker — agent notes

## Build workflow

After editing any palette/goal UI code under `cmd/agent/`, run:

```bash
./install.sh
```

This rebuilds `bin/agent` (plus `bin/tracker-server` and `bin/tracker-mcp`). The tmux
palette (Alt-S → Alt-R) execs `bin/agent palette` fresh on each open via
`display-popup -E`, so rebuilding `bin/agent` is sufficient for UI changes to show —
no service restart needed.

For `tracker-server` code changes, also restart the brew service:

```bash
./deploy   # builds tracker-server + tracker-client, installs + restarts brew service
```
