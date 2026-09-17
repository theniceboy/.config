package main

import (
	"fmt"
	"testing"
	"time"
)

func TestClaimEventOnce(t *testing.T) {
	key := fmt.Sprintf("test-onfocus-%d", time.Now().UnixNano())

	if !claimEventOnce(500*time.Millisecond, key) {
		t.Fatal("first claim should win")
	}
	if claimEventOnce(500*time.Millisecond, key) {
		t.Fatal("duplicate claim within window should be suppressed")
	}

	other := key + "-other"
	if !claimEventOnce(500*time.Millisecond, other) {
		t.Fatal("different key should not be suppressed")
	}

	time.Sleep(30 * time.Millisecond)
	if !claimEventOnce(20*time.Millisecond, key) {
		t.Fatal("stale marker (beyond window) should be re-claimable")
	}
}

func TestSanitizeKey(t *testing.T) {
	got := sanitizeKey("a@5", "/tmp/x", "%12")
	if got != "a_5__tmp_x__12" {
		t.Fatalf("unexpected sanitize: %q", got)
	}
}
