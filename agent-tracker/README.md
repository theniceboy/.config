# Agent Tracker

Tmux-aware agent task and note tracker.

## Notifications

`sendSystemNotification` tries `terminal-notifier` first (rich actions), then
falls back to `osascript display notification` on macOS if it is missing OR
exits non-zero. The fallback matters: a `brew upgrade` of terminal-notifier
replaces its unsigned binary and macOS can silently drop its notification
permission (`UNErrorDomain error 1`, exit 3) — every ping then dies unless the
osascript path catches it (2026-08-25 outage, ~4.3k lost notifications).

## Build, Install & Restart

```bash
./scripts/install_brew_service.sh
```

This script:
1. Builds `tracker-server` from source
2. Installs it via Homebrew
3. Restarts the brew service
