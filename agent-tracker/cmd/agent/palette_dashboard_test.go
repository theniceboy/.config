package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func compactPaletteWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func TestPaletteViewShowsStatusDashboardWithoutSelectedPreview(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Global:  []tmuxTodoItem{{Title: "global todo", CreatedAt: time.Now()}},
		Windows: map[string][]tmuxTodoItem{
			"@9": {
				{Title: "window todo 1", CreatedAt: time.Now()},
				{Title: "window todo 2", Done: true, CreatedAt: time.Now()},
			},
		},
	})

	model := newPaletteModel(&paletteRuntime{
		windowID:           "@9",
		currentSessionName: "dev",
		currentWindowName:  "agent",
		currentPath:        "/tmp/repo",
		mainRepoRoot:       "/tmp/repo",
	}, paletteUIState{Mode: paletteModeList})
	model.width = 96
	model.height = 28

	view := model.View()
	compact := compactPaletteWhitespace(view)
	if !strings.Contains(compact, "Tracker Status") {
		t.Fatalf("expected tracker status section in view: %q", view)
	}
	if !strings.Contains(compact, "Todo Preview") {
		t.Fatalf("expected todo preview section in view: %q", view)
	}
	if !strings.Contains(compact, "No active agent") {
		t.Fatalf("expected no active agent status in view: %q", view)
	}
	if !strings.Contains(compact, "window todo 1") || !strings.Contains(compact, "global todo") {
		t.Fatalf("expected todo previews in view: %q", view)
	}
	if strings.Contains(view, "[ ]") || strings.Contains(view, "[x]") {
		t.Fatalf("expected todo panel checkbox visuals in view: %q", view)
	}
	if !strings.Contains(view, "○") {
		t.Fatalf("expected open todo checkbox visual in view: %q", view)
	}
	if strings.Contains(compact, "window todo 2") {
		t.Fatalf("expected completed window todo to stay out of preview when open work exists: %q", view)
	}
	if strings.Contains(compact, "Selected") {
		t.Fatalf("unexpected selected-action preview in view: %q", view)
	}
	if strings.Contains(compact, "Current Task") || strings.Contains(compact, "Next Todos") {
		t.Fatalf("unexpected legacy agent preview content in view: %q", view)
	}
}

func TestPaletteViewShowsBootstrapAndDashboardTodoStatus(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Global:  []tmuxTodoItem{{Title: "global todo", CreatedAt: time.Now()}},
		Windows: map[string][]tmuxTodoItem{
			"@4": {{Title: "window todo", CreatedAt: time.Now()}},
		},
	})

	workspaceRoot := t.TempDir()
	if err := os.MkdirAll(bootstrapStateDirPath(workspaceRoot), 0o755); err != nil {
		t.Fatalf("mkdir bootstrap dir: %v", err)
	}
	if err := os.WriteFile(bootstrapGitReadyPath(workspaceRoot), []byte(time.Now().Format(time.RFC3339Nano)), 0o644); err != nil {
		t.Fatalf("write git-ready marker: %v", err)
	}
	if err := os.WriteFile(bootstrapPIDPath(workspaceRoot), []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write pid marker: %v", err)
	}

	model := newPaletteModel(&paletteRuntime{
		windowID:           "@4",
		currentSessionName: "dev",
		currentWindowName:  "agent-4",
		mainRepoRoot:       "/tmp/repo",
		record: &agentRecord{
			ID:            "icon-pad",
			Branch:        "feature/icon-pad",
			WorkspaceRoot: workspaceRoot,
			Dashboard: dashboardDoc{
				CurrentTask: "Ship status dashboard",
				Todos: []todoItem{
					{Title: "wire dashboard"},
					{Title: "verify tests", Done: true},
				},
			},
		},
	}, paletteUIState{Mode: paletteModeList})
	model.width = 104
	model.height = 28

	view := model.View()
	compact := compactPaletteWhitespace(view)
	if !strings.Contains(compact, "copying repo") {
		t.Fatalf("expected bootstrap status in view: %q", view)
	}
	if !strings.Contains(compact, "icon-pad on feature/icon-pad") {
		t.Fatalf("expected agent summary in view: %q", view)
	}
	if !strings.Contains(compact, "Todo Preview") {
		t.Fatalf("expected todo preview section in view: %q", view)
	}
	if !strings.Contains(compact, "Ship status dashboard") {
		t.Fatalf("expected current task summary in view: %q", view)
	}
	if !strings.Contains(compact, "wire dashboard") || !strings.Contains(compact, "window todo") || !strings.Contains(compact, "global todo") {
		t.Fatalf("expected todo previews in view: %q", view)
	}
	if strings.Contains(view, "[ ]") || strings.Contains(view, "[x]") {
		t.Fatalf("expected todo panel checkbox visuals in view: %q", view)
	}
	if !strings.Contains(view, "○") {
		t.Fatalf("expected open todo checkbox visual in view: %q", view)
	}
	if strings.Contains(compact, "verify tests") {
		t.Fatalf("expected completed dashboard todo to stay out of preview when open work exists: %q", view)
	}
	if strings.Contains(compact, "Selected") {
		t.Fatalf("unexpected selected-action preview in view: %q", view)
	}
}

func TestPaletteTodoPreviewItemsHideCompletedTodos(t *testing.T) {
	tmuxItems := paletteTmuxTodoPreviewItems([]tmuxTodoItem{
		{Title: "done one", Done: true},
		{Title: "open one"},
		{Title: "done two", Done: true},
	})
	if len(tmuxItems) != 1 || tmuxItems[0].Title != "open one" {
		t.Fatalf("expected only open tmux todo in preview, got %#v", tmuxItems)
	}

	tmuxDoneOnly := paletteTmuxTodoPreviewItems([]tmuxTodoItem{{Title: "done only", Done: true}})
	if len(tmuxDoneOnly) != 0 {
		t.Fatalf("expected completed tmux todos to stay out of preview, got %#v", tmuxDoneOnly)
	}

	agentItems := paletteAgentTodoPreviewItems([]todoItem{
		{Title: "done agent", Done: true},
		{Title: "open agent"},
	})
	if len(agentItems) != 1 || agentItems[0].Title != "open agent" {
		t.Fatalf("expected only open dashboard todo in preview, got %#v", agentItems)
	}

	agentDoneOnly := paletteAgentTodoPreviewItems([]todoItem{{Title: "done only", Done: true}})
	if len(agentDoneOnly) != 0 {
		t.Fatalf("expected completed dashboard todos to stay out of preview, got %#v", agentDoneOnly)
	}
}

func TestPaletteBootstrapStatusSummaries(t *testing.T) {
	if got := paletteBootstrapStatus(nil); got != "No active agent" {
		t.Fatalf("expected no active agent, got %q", got)
	}

	workspaceRoot := t.TempDir()
	record := &agentRecord{WorkspaceRoot: workspaceRoot}
	if err := os.MkdirAll(bootstrapStateDirPath(workspaceRoot), 0o755); err != nil {
		t.Fatalf("mkdir bootstrap dir: %v", err)
	}
	if err := os.WriteFile(bootstrapFailedPath(workspaceRoot), []byte("clone failed\nextra\n"), 0o644); err != nil {
		t.Fatalf("write failed marker: %v", err)
	}
	if got := paletteBootstrapStatus(record); got != "failed: clone failed" {
		t.Fatalf("expected failure summary, got %q", got)
	}
	if err := os.Remove(bootstrapFailedPath(workspaceRoot)); err != nil {
		t.Fatalf("remove failed marker: %v", err)
	}
	if err := os.WriteFile(bootstrapRepoReadyPath(workspaceRoot), []byte(time.Now().Format(time.RFC3339Nano)), 0o644); err != nil {
		t.Fatalf("write repo-ready marker: %v", err)
	}
	if got := paletteBootstrapStatus(record); got != "ready" {
		t.Fatalf("expected ready summary, got %q", got)
	}
}
