package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/david/agent-tracker/internal/ipc"
)

func TestPaletteAltROpensTrackerPanel(t *testing.T) {
	model := newPaletteModel(&paletteRuntime{}, paletteUIState{Mode: paletteModeList})

	updated, cmd := model.updateList("alt+r")
	if cmd == nil {
		t.Fatalf("expected tracker shortcut to return a command")
	}

	palette := updated.(*paletteModel)
	if palette.state.Mode != paletteModeTracker {
		t.Fatalf("expected tracker mode, got %v", palette.state.Mode)
	}
	if palette.tracker == nil {
		t.Fatalf("expected tracker panel to be created")
	}
}

func TestTrackerSortTasksPrioritizesActiveAndUnacknowledged(t *testing.T) {
	tasks := []ipc.Task{
		{SessionID: "$1", WindowID: "@1", Pane: "%1", Summary: "ack", Status: trackerTaskStatusCompleted, Acknowledged: true, CompletedAt: "2026-03-23T18:00:00Z"},
		{SessionID: "$1", WindowID: "@2", Pane: "%2", Summary: "active", Status: trackerTaskStatusInProgress, StartedAt: "2026-03-23T17:00:00Z"},
		{SessionID: "$1", WindowID: "@3", Pane: "%3", Summary: "review", Status: trackerTaskStatusCompleted, Acknowledged: false, CompletedAt: "2026-03-23T19:00:00Z"},
	}

	trackerSortTasks(tasks)
	if tasks[0].Summary != "active" {
		t.Fatalf("expected active task first, got %#v", tasks)
	}
	if tasks[1].Summary != "review" {
		t.Fatalf("expected unacknowledged completed task second, got %#v", tasks)
	}
}

func TestTrackerDetailLineWrapsWithinPanel(t *testing.T) {
	line := trackerDetailLine(newPaletteStyles(), "where", "session-name / a-very-long-window-name-that-needs-wrapping", 26)
	if !strings.Contains(line, "\n") {
		t.Fatalf("expected wrapped detail line, got %q", line)
	}
}

func TestTrackerTaskRowUsesIndicatorInsteadOfLiveDoneLabel(t *testing.T) {
	width := 36
	row := (&trackerPanelModel{}).renderTaskRow(newPaletteStyles(), ipc.Task{Summary: "Build release", Status: trackerTaskStatusInProgress}, true, width, mustTrackerTime(t, "2026-03-23T20:00:00Z"))
	if strings.Contains(row, "LIVE") || strings.Contains(row, "DONE") || strings.Contains(row, "REVIEW") {
		t.Fatalf("expected indicator-based tracker row, got %q", row)
	}
	for _, line := range strings.Split(row, "\n") {
		if lipgloss.Width(line) > width {
			t.Fatalf("expected rendered task row width <= %d, got %d for %q", width, lipgloss.Width(line), line)
		}
	}

	doneRow := (&trackerPanelModel{}).renderTaskRow(newPaletteStyles(), ipc.Task{Summary: "Build release", Status: trackerTaskStatusCompleted, Acknowledged: true}, false, width, mustTrackerTime(t, "2026-03-23T20:00:00Z"))
	if !strings.Contains(doneRow, "✓") {
		t.Fatalf("expected completed task row to use checkmark indicator, got %q", doneRow)
	}
}

func TestTrackerVisibleTasksReturnsSortedTasks(t *testing.T) {
	model := &trackerPanelModel{state: ipc.Envelope{Tasks: []ipc.Task{
		{Summary: "done", Status: trackerTaskStatusCompleted, Acknowledged: true, CompletedAt: "2026-03-23T18:00:00Z"},
		{Summary: "active", Status: trackerTaskStatusInProgress, StartedAt: "2026-03-23T17:00:00Z"},
		{Summary: "review", Status: trackerTaskStatusCompleted, Acknowledged: false, CompletedAt: "2026-03-23T19:00:00Z"},
	}}}
	visible := model.visibleTasks()
	if len(visible) != 3 {
		t.Fatalf("expected 3 visible tasks, got %d", len(visible))
	}
	if visible[0].Summary != "active" || visible[1].Summary != "review" {
		t.Fatalf("expected visible tasks to stay sorted by tracker priority, got %#v", visible)
	}
}

func TestTrackerTaskRowKeepsSameHeightWhenSelected(t *testing.T) {
	styles := newPaletteStyles()
	task := ipc.Task{Summary: "Build release", Status: trackerTaskStatusCompleted, Acknowledged: false, CompletionNote: "this should stay in the detail pane only"}
	selected := (&trackerPanelModel{}).renderTaskRow(styles, task, true, 36, mustTrackerTime(t, "2026-03-23T20:00:00Z"))
	unselected := (&trackerPanelModel{}).renderTaskRow(styles, task, false, 36, mustTrackerTime(t, "2026-03-23T20:00:00Z"))
	if strings.Count(selected, "\n") != strings.Count(unselected, "\n") {
		t.Fatalf("expected selected and unselected task rows to keep same height, got %q vs %q", selected, unselected)
	}
}

func mustTrackerTime(t *testing.T, value string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return ts
}
