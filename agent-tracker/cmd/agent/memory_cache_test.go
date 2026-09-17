package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRefreshMemoryUsageCacheGatesOnFreshCache(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "mem.json")
	script := filepath.Join(dir, "refresh.py")
	if err := os.WriteFile(cache, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath, origScript, origStart := statusMemoryCachePath, statusMemoryCacheRefreshScript, statusCommandStart
	t.Cleanup(func() {
		statusMemoryCachePath = origPath
		statusMemoryCacheRefreshScript = origScript
		statusCommandStart = origStart
	})
	statusMemoryCachePath = func() string { return cache }
	statusMemoryCacheRefreshScript = func() string { return script }

	started := 0
	statusCommandStart = func(name string, args ...string) error {
		started++
		return nil
	}

	refreshMemoryUsageCache()
	if started != 0 {
		t.Fatalf("fresh cache should skip python spawn, got %d spawns", started)
	}

	if err := os.Chtimes(cache, time.Now().Add(-2*memoryCacheFreshFor), time.Now().Add(-2*memoryCacheFreshFor)); err != nil {
		t.Fatal(err)
	}
	refreshMemoryUsageCache()
	if started != 1 {
		t.Fatalf("stale cache should spawn python once, got %d spawns", started)
	}
}
