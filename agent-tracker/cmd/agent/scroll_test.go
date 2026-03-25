package main

import (
	"fmt"
	"testing"
	"time"
)

func TestStableListOffsetKeepsViewportStableWhenReversing(t *testing.T) {
	offset := 0
	for _, selected := range []int{0, 1, 2, 3, 4, 5, 6, 7} {
		offset = stableListOffset(offset, selected, 5, 20)
	}
	if offset != 3 {
		t.Fatalf("expected offset 3 after scrolling down, got %d", offset)
	}

	offset = stableListOffset(offset, 6, 5, 20)
	if offset != 3 {
		t.Fatalf("expected offset to stay 3 when reversing inside viewport, got %d", offset)
	}

	offset = stableListOffset(offset, 2, 5, 20)
	if offset != 2 {
		t.Fatalf("expected offset to move only once selection crosses the top edge, got %d", offset)
	}
}

func TestActivityMonitorViewportDoesNotJumpWhenReversing(t *testing.T) {
	m := newActivityMonitorModel("@1", false)
	m.processes = map[int]*activityProcess{}
	for i := 0; i < 20; i++ {
		pid := 100 + i
		m.rows = append(m.rows, activityRow{PID: pid})
		m.processes[pid] = &activityProcess{
			PID:          pid,
			CPU:          1,
			ResidentMB:   10,
			Command:      fmt.Sprintf("proc-%d", i),
			ShortCommand: fmt.Sprintf("proc-%d", i),
		}
	}
	m.selectedRow = 0
	m.selectedPID = m.rows[0].PID

	_ = m.renderTable(80, 6)
	for i := 0; i < 7; i++ {
		m.moveSelection(1)
		_ = m.renderTable(80, 6)
	}
	if m.rowOffset != 3 {
		t.Fatalf("expected activity monitor offset 3 after scrolling down, got %d", m.rowOffset)
	}

	m.moveSelection(-1)
	_ = m.renderTable(80, 6)
	if m.rowOffset != 3 {
		t.Fatalf("expected activity monitor offset to stay 3 when reversing inside viewport, got %d", m.rowOffset)
	}
}

func TestTodoPanelViewportDoesNotJumpWhenReversing(t *testing.T) {
	home := t.TempDir()
	windowItems := make([]tmuxTodoItem, 0, 20)
	for i := 0; i < 20; i++ {
		windowItems = append(windowItems, tmuxTodoItem{Title: fmt.Sprintf("window %d", i), Priority: 1, CreatedAt: time.Now()})
	}
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Windows: map[string][]tmuxTodoItem{"@1": windowItems},
	})

	m, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}

	_ = m.renderColumn(todoScopeWindow, 40, 7)
	for i := 0; i < 7; i++ {
		m.moveSelection(1)
		_ = m.renderColumn(todoScopeWindow, 40, 7)
	}
	if m.windowOffset != 3 {
		t.Fatalf("expected todo panel offset 3 after scrolling down, got %d", m.windowOffset)
	}

	m.moveSelection(-1)
	_ = m.renderColumn(todoScopeWindow, 40, 7)
	if m.windowOffset != 3 {
		t.Fatalf("expected todo panel offset to stay 3 when reversing inside viewport, got %d", m.windowOffset)
	}
}

func TestPaletteViewportDoesNotJumpWhenReversing(t *testing.T) {
	todos := make([]todoItem, 0, 10)
	for i := 0; i < 10; i++ {
		todos = append(todos, todoItem{Title: fmt.Sprintf("todo %d", i)})
	}
	runtime := &paletteRuntime{
		agentID: "agent-1",
		record: &agentRecord{
			ID:        "agent-1",
			Dashboard: dashboardDoc{Todos: todos},
		},
	}
	m := newPaletteModel(runtime, paletteUIState{})
	styles := newPaletteStyles()

	_ = m.renderListView(styles, 96, 20)
	for i := 0; i < 5; i++ {
		m.state.Selected++
		_ = m.renderListView(styles, 96, 20)
	}
	if m.state.ActionOffset != 3 {
		t.Fatalf("expected palette offset 3 after scrolling down, got %d", m.state.ActionOffset)
	}

	m.state.Selected--
	_ = m.renderListView(styles, 96, 20)
	if m.state.ActionOffset != 3 {
		t.Fatalf("expected palette offset to stay 3 when reversing inside viewport, got %d", m.state.ActionOffset)
	}
}
