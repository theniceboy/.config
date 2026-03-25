package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestChromeAppleEventsEnabledReadsPreference(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only preference path")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	prefsPath := filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Preferences")
	if err := os.MkdirAll(filepath.Dir(prefsPath), 0o755); err != nil {
		t.Fatalf("mkdir prefs dir: %v", err)
	}
	if err := os.WriteFile(prefsPath, []byte(`{"browser":{"allow_javascript_apple_events":true}}`), 0o644); err != nil {
		t.Fatalf("write prefs: %v", err)
	}

	enabled, err := chromeAppleEventsEnabled()
	if err != nil {
		t.Fatalf("chromeAppleEventsEnabled: %v", err)
	}
	if !enabled {
		t.Fatalf("expected allow_javascript_apple_events=true")
	}
}

func TestEnsureChromeAppleEventsEnabledErrorsWhenDisabled(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only preference path")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	prefsPath := filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Preferences")
	if err := os.MkdirAll(filepath.Dir(prefsPath), 0o755); err != nil {
		t.Fatalf("mkdir prefs dir: %v", err)
	}
	if err := os.WriteFile(prefsPath, []byte(`{"browser":{"allow_javascript_apple_events":false}}`), 0o644); err != nil {
		t.Fatalf("write prefs: %v", err)
	}

	err := ensureChromeAppleEventsEnabled()
	if err == nil {
		t.Fatalf("expected disabled preference to fail")
	}
}
