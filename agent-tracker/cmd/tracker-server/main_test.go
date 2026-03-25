package main

import (
	"testing"
	"time"
)

func TestNormalizeTargetNamesClearsIDPlaceholders(t *testing.T) {
	target := normalizeTargetNames(tmuxTarget{
		SessionName: "$3",
		SessionID:   "$3",
		WindowName:  "@12",
		WindowID:    "@12",
		PaneID:      "%7",
	})
	if target.SessionName != "" {
		t.Fatalf("expected placeholder session name to be cleared, got %q", target.SessionName)
	}
	if target.WindowName != "" {
		t.Fatalf("expected placeholder window name to be cleared, got %q", target.WindowName)
	}
}

func TestBuildStateEnvelopeUsesStoredTaskNames(t *testing.T) {
	srv := newServer()
	started := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)
	srv.tasks[taskKey("$3", "@12", "%7")] = &taskRecord{
		SessionID:    "$3",
		SessionName:  "workbench",
		WindowID:     "@12",
		WindowName:   "agent-tracker",
		Pane:         "%7",
		Summary:      "Polish notifications",
		StartedAt:    started,
		Status:       statusInProgress,
		Acknowledged: true,
	}

	env := srv.buildStateEnvelope()
	if env == nil {
		t.Fatal("expected state envelope")
	}
	if len(env.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(env.Tasks))
	}
	task := env.Tasks[0]
	if task.Session != "workbench" {
		t.Fatalf("expected stored session name, got %q", task.Session)
	}
	if task.Window != "agent-tracker" {
		t.Fatalf("expected stored window name, got %q", task.Window)
	}
}
