package main

import (
	"reflect"
	"testing"
)

func TestPreferredNewWindowTarget(t *testing.T) {
	if got := preferredNewWindowTarget("@9", true, "@3"); got != "@9" {
		t.Fatalf("expected explicit target window, got %q", got)
	}
	if got := preferredNewWindowTarget("", true, "@3"); got != "@3" {
		t.Fatalf("expected current tmux window fallback, got %q", got)
	}
	if got := preferredNewWindowTarget("", false, "@3"); got != "" {
		t.Fatalf("expected no target outside tmux, got %q", got)
	}
}

func TestPositionedNewWindowArgs(t *testing.T) {
	got := positionedNewWindowArgs("feature-x", "/tmp/repo", "@7")
	want := []string{"new-window", "-P", "-F", "#{window_id}", "-a", "-t", "@7", "-n", "feature-x", "-c", "/tmp/repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected positioned new-window args %v, got %v", want, got)
	}

	got = positionedNewWindowArgs("feature-x", "/tmp/repo", "")
	want = []string{"new-window", "-P", "-F", "#{window_id}", "-n", "feature-x", "-c", "/tmp/repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected default new-window args %v, got %v", want, got)
	}
}
