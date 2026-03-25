package main

import "testing"

func TestPaletteBuildActionsIncludesDestroyForAgentRecord(t *testing.T) {
	r := &paletteRuntime{
		agentID:      "memory-hack",
		mainRepoRoot: "/tmp/repo",
		currentPath:  "/tmp/repo/.agents/memory-hack/repo",
		record:       &agentRecord{ID: "memory-hack", RepoRoot: "/tmp/repo"},
	}
	actions := r.buildActions()
	for _, action := range actions {
		if action.Kind == paletteActionConfirmDestroy {
			return
		}
	}
	t.Fatalf("destroy action missing: %#v", actions)
}
