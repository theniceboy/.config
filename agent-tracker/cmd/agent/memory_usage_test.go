package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestMemoryUsageScrollAndResize(t *testing.T) {
	m := &memoryPanelModel{hasAgent: true, sessionID: "ses_test", directory: "/repo", agentTitle: "Test", enabled: true,
		usage: memoryUsage{Status: "ready", TotalTokens: 12345}}
	for i := 0; i < 30; i++ {
		m.usage.Files = append(m.usage.Files, memoryUsageFile{Store: "repo", Path: fmt.Sprintf(".memory/deeply/nested/reference/%02d-architecture.md", i), Tokens: 400})
	}
	for _, size := range [][2]int{{96, 28}, {40, 20}, {120, 45}} {
		m.contentOffset = 0
		view := m.render(newPaletteStyles(), size[0], size[1])
		if !strings.Contains(view, "12,345") || !strings.Contains(view, "00-architecture") {
			t.Fatalf("missing loaded files/total at %v: %s", size, view)
		}
		if lipgloss.Height(view) > size[1] {
			t.Fatalf("panel overflow at %v: height %d\n%s", size, lipgloss.Height(view), view)
		}
		for i := 0; i < len(m.contentLines); i++ {
			m.handleKey("e")
		}
		if m.contentOffset != m.maxContentOffset() {
			t.Fatal("scrolling did not reach the last content row")
		}
	}
}

func TestMemoryUsageRejectsOtherSessionAndRecovers(t *testing.T) {
	m := &memoryPanelModel{sessionID: "ses_new", usageInFlight: true}
	m.acceptUsage(memoryUsageMsg{m, memoryUsage{SessionID: "ses_old", Status: "ready", TotalTokens: 123}})
	if m.usage.TotalTokens != 0 || m.usageInFlight {
		t.Fatal("stale result applied or request remains blocked")
	}
	m.acceptUsage(memoryUsageMsg{m, memoryUsage{SessionID: "ses_new", Status: "ready", Fingerprint: "abc"}})
	m.acceptUsage(memoryUsageMsg{m, memoryUsage{SessionID: "ses_new", Status: "unchanged"}})
	if m.usage.Fingerprint != "abc" {
		t.Fatal("unchanged poll lost state")
	}
}

func TestMemoryUsageReopenIgnoresOldPoll(t *testing.T) {
	m := &memoryPanelModel{}
	m.Init()
	old := memoryPanelTickMsg{m, m.pollGeneration}
	m.Init()
	_, cmd := m.Update(old)
	if cmd != nil {
		t.Fatal("reopening kept an old refresh loop alive")
	}
}

func TestAllMemoryModeToggleWritesState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	if err := setBaseMemoryEnabled(path, "/repo", true); err != nil {
		t.Fatal(err)
	}
	if err := setAllMemoryMode(path, "ses_a", "global", true); err != nil {
		t.Fatal(err)
	}
	if err := setAllMemoryMode(path, "ses_a", "repo", true); err != nil {
		t.Fatal(err)
	}
	modes, err := loadAllMemoryModes(path)
	if err != nil || len(modes["ses_a"]) != 2 {
		t.Fatalf("all stores not persisted: %v %v", modes, err)
	}
	if err := setAllMemoryMode(path, "ses_a", "repo", false); err != nil {
		t.Fatal(err)
	}
	modes, _ = loadAllMemoryModes(path)
	if len(modes["ses_a"]) != 1 || modes["ses_a"][0] != "global" {
		t.Fatalf("repo store not removed: %v", modes)
	}
	if err := setAllMemoryMode(path, "ses_a", "global", false); err != nil {
		t.Fatal(err)
	}
	modes, _ = loadAllMemoryModes(path)
	if _, kept := modes["ses_a"]; kept {
		t.Fatal("empty session entry not cleared")
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"enabled"`) {
		t.Fatal("enabled map lost")
	}
	legacy := filepath.Join(dir, "legacy.json")
	os.WriteFile(legacy, []byte(`{"allMode": {"ses_old": true}}`), 0o644)
	modes, err = loadAllMemoryModes(legacy)
	if err != nil || len(modes["ses_old"]) != 1 || modes["ses_old"][0] != "global" {
		t.Fatalf("legacy bool not migrated: %v %v", modes, err)
	}
}

func TestMemoryPanelShowsAllMemoryBadge(t *testing.T) {
	m := &memoryPanelModel{hasAgent: true, sessionID: "ses_x", directory: "/repo", agentTitle: "t", enabled: true, allGlobal: true,
		usage: memoryUsage{Status: "ready", Files: []memoryUsageFile{{Store: "global", Path: "knowledge/a.md", Tokens: 5}}, TotalTokens: 5}}
	view := m.render(newPaletteStyles(), 96, 28)
	if !strings.Contains(view, "ALL GLOBAL") {
		t.Fatalf("all-global badge missing:\n%s", view)
	}
	if strings.Contains(view, "ALL PROJECT") {
		t.Fatal("project badge shown without repo mode")
	}
	m.allGlobal = false
	m.allRepo = true
	m.hasRepo = true
	view = m.render(newPaletteStyles(), 96, 28)
	if !strings.Contains(view, "ALL PROJECT") || strings.Contains(view, "ALL GLOBAL") {
		t.Fatalf("repo-only badges wrong:\n%s", view)
	}
	if !strings.Contains(view, "all proj") {
		t.Fatal("repo shortcut hint missing")
	}
	m.enabled = false
	m.allRepo = false
	m.allGlobal = true
	if strings.Contains(m.render(newPaletteStyles(), 96, 28), "ALL GLOBAL") {
		t.Fatal("global badge shown while global memory is off")
	}
	m.enabled = true
	m.allRepo = false
	m.allGlobal = false
	if strings.Contains(m.render(newPaletteStyles(), 96, 28), "ALL ") {
		t.Fatal("badge shown when off")
	}
}
