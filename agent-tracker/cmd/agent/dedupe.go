package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// eventDedupeWindow collapses identical focus events that arrive within this
// window (tmux fires pane-focus-in + client-session-changed +
// after-select-window for a single session switch).
const eventDedupeWindow = 500 * time.Millisecond

// claimEventOnce reports whether this caller should handle the event keyed by
// keyParts. Concurrent invocations race safely: O_EXCL creation guarantees
// exactly one winner; losers and near-simultaneous duplicates see a fresh
// marker and return false. Markers older than the window are re-claimable.
func claimEventOnce(window time.Duration, keyParts ...string) bool {
	dir := filepath.Join(os.TempDir(), "agent-dedupe")
	_ = os.MkdirAll(dir, 0o700)
	name := filepath.Join(dir, sanitizeKey(keyParts...)+".marker")
	if info, err := os.Stat(name); err == nil {
		if time.Since(info.ModTime()) < window {
			return false
		}
		_ = os.Remove(name)
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	_, _ = f.WriteString(time.Now().Format(time.RFC3339Nano))
	_ = f.Close()
	return true
}

func sanitizeKey(parts ...string) string {
	joined := strings.Join(parts, "-")
	var b strings.Builder
	for _, r := range joined {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
