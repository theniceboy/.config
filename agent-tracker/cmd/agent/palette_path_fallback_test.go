package main

import "testing"

func TestDetectPaletteAgentIDFromPath(t *testing.T) {
	got := detectPaletteAgentIDFromPath("/Users/david/Github/instaboard/.agents/icon-pad/repo")
	if got != "icon-pad" {
		t.Fatalf("expected icon-pad, got %q", got)
	}
}

func TestPaletteBuildActionsIncludesDestroyWhenAgentIDKnownWithoutRecord(t *testing.T) {
	r := &paletteRuntime{
		agentID:      "icon-pad",
		mainRepoRoot: "/tmp/repo",
		currentPath:  "/tmp/repo/.agents/icon-pad/repo",
	}
	actions := r.buildActions()
	for _, action := range actions {
		if action.Kind == paletteActionConfirmDestroy {
			return
		}
	}
	t.Fatalf("destroy action missing: %#v", actions)
}

func TestPaletteEffectiveAgentIDIgnoresLiteralAndUsesPath(t *testing.T) {
	r := &paletteRuntime{
		agentID:     "#{q:@agent_id}",
		currentPath: "/tmp/repo/.agents/icon-pad/repo",
	}
	if got := r.effectiveAgentID(); got != "icon-pad" {
		t.Fatalf("expected icon-pad, got %q", got)
	}
}
