package main

import (
	"strings"
	"testing"
)

func TestActivityCopyOptionsIncludeCoreFields(t *testing.T) {
	proc := &activityProcess{
		PID:          123,
		PPID:         45,
		DownKBps:     12.5,
		UpKBps:       4.25,
		ShortCommand: "node",
		Command:      "node server.js --port 3000",
		Ports:        []string{"3000", "9229"},
		Tmux: &activityTmuxLocation{
			SessionID:   "$1",
			SessionName: "3-Config",
			WindowID:    "@12",
			WindowIndex: "2",
			WindowName:  "agent",
			PaneID:      "%31",
		},
	}

	options := activityCopyOptions(proc)
	joined := make([]string, 0, len(options))
	for _, option := range options {
		joined = append(joined, option.Label+":"+option.Value)
	}
	text := strings.Join(joined, "\n")
	for _, expected := range []string{"PID:123", "Parent PID:45", "Process:node", "Command:node server.js --port 3000", "Download:12.5K", "Upload:4.25K", "Session:3-Config ($1)", "Window:2 agent (@12)", "Pane:%31", "Ports:3000, 9229"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in copy options, got %q", expected, text)
		}
	}
}

func TestParseActivityNetworkLine(t *testing.T) {
	parsed, ok := parseActivityNetworkLine("Google Chrome H.34802,54961,47736,")
	if !ok {
		t.Fatal("expected network line to parse")
	}
	if parsed.PID != 34802 || parsed.BytesIn != 54961 || parsed.BytesOut != 47736 {
		t.Fatalf("unexpected parsed network line: %#v", parsed)
	}
}

func TestActivityMonitorRenderIncludesNetworkColumns(t *testing.T) {
	model := newActivityMonitorModel("@12", true)
	model.width = 120
	model.height = 24
	model.processes = map[int]*activityProcess{123: {
		PID:          123,
		CPU:          10.5,
		ResidentMB:   128,
		DownKBps:     64,
		UpKBps:       8.5,
		ShortCommand: "node",
		Command:      "node server.js",
	}}
	model.rows = []activityRow{{PID: 123}}
	model.selectedPID = 123
	model.selectedRow = 0

	table := model.renderTable(80, 10)
	if !strings.Contains(table, "DOWN") || !strings.Contains(table, "UP") {
		t.Fatalf("expected network columns in table, got %q", table)
	}
	if !strings.Contains(table, "64.0K") || !strings.Contains(table, "8.50K") {
		t.Fatalf("expected network speeds in table, got %q", table)
	}
}

func TestActivityMonitorDefaultsToAllProcesses(t *testing.T) {
	model := newActivityMonitorModel("@12", true)
	if !model.showAllProcesses {
		t.Fatal("expected activity monitor to default to all processes")
	}
}

func TestActivityMonitorNetworkHotkeysSetSeparateSortColumns(t *testing.T) {
	model := newActivityMonitorModel("@12", true)

	updated, _ := model.updateNormal("j")
	activity := updated.(*activityMonitorBT)
	if activity.sortKey != activitySortDownload {
		t.Fatalf("expected j to sort by download, got %v", activity.sortKey)
	}
	if !activity.sortDescending {
		t.Fatal("expected download sort to default descending")
	}

	updated, _ = activity.updateNormal("k")
	activity = updated.(*activityMonitorBT)
	if activity.sortKey != activitySortUpload {
		t.Fatalf("expected k to sort by upload, got %v", activity.sortKey)
	}
	if !activity.sortDescending {
		t.Fatal("expected upload sort to default descending")
	}
}

func TestActivityMonitorYOpensCopyMenu(t *testing.T) {
	model := newActivityMonitorModel("@12", true)
	model.processes = map[int]*activityProcess{123: {PID: 123, PPID: 1, ShortCommand: "node", Command: "node server.js"}}
	model.rows = []activityRow{{PID: 123}}
	model.selectedPID = 123
	model.selectedRow = 0

	updated, _ := model.updateNormal("y")
	activity := updated.(*activityMonitorBT)
	if len(activity.copyOptions) == 0 {
		t.Fatalf("expected copy options to open")
	}
	if activity.copyOptions[0].Label != "PID" {
		t.Fatalf("expected first copy option to be PID, got %#v", activity.copyOptions[0])
	}
}

func TestActivityMonitorCopyMenuCopiesSelectedOption(t *testing.T) {
	model := newActivityMonitorModel("@12", true)
	model.copyOptions = []activityCopyOption{{Label: "PID", Value: "123"}, {Label: "Command", Value: "node server.js"}}
	model.copyOptionIndex = 1

	var copied string
	prev := activityClipboardWriter
	activityClipboardWriter = func(value string) error {
		copied = value
		return nil
	}
	defer func() { activityClipboardWriter = prev }()

	updated, _ := model.updateCopyMenu("enter")
	activity := updated.(*activityMonitorBT)
	if copied != "node server.js" {
		t.Fatalf("expected copied value %q, got %q", "node server.js", copied)
	}
	if len(activity.copyOptions) != 0 {
		t.Fatalf("expected copy menu to close after copying")
	}
	if !strings.Contains(strings.ToLower(activity.currentStatus()), "copied command") {
		t.Fatalf("expected copied status, got %q", activity.currentStatus())
	}
}
