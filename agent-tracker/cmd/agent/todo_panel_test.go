package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func writeTestTodoStore(t *testing.T, home string, store *tmuxTodoStore) {
	t.Helper()
	t.Setenv("HOME", home)
	if err := saveTmuxTodoStore(store); err != nil {
		t.Fatalf("save todo store: %v", err)
	}
}

func TestTodoPanelViewShowsTwoColumnsWithoutPreview(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Global:  []tmuxTodoItem{{Title: "global todo", Priority: 2, CreatedAt: time.Now()}},
		Windows: map[string][]tmuxTodoItem{
			"@1": {{Title: "window todo", Priority: 1, CreatedAt: time.Now()}},
		},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	model.width = 96
	model.height = 24
	view := model.View()

	if !strings.Contains(view, "WINDOW") {
		t.Fatalf("expected window column header in view: %q", view)
	}
	if !strings.Contains(view, "GLOBAL") {
		t.Fatalf("expected global column header in view: %q", view)
	}
	if strings.Contains(view, "Overview") || strings.Contains(view, "Selected") {
		t.Fatalf("unexpected preview content in view: %q", view)
	}
	if strings.Contains(view, "SESSION") {
		t.Fatalf("unexpected session scope in view: %q", view)
	}
}

func TestTodoPanelSwitchesFocusedColumnWithNI(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Global:  []tmuxTodoItem{{Title: "global todo", Priority: 2, CreatedAt: time.Now()}},
		Windows: map[string][]tmuxTodoItem{
			"@1": {{Title: "window todo", Priority: 1, CreatedAt: time.Now()}},
		},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	if model.focusedScope != todoScopeWindow {
		t.Fatalf("expected window focus by default, got %v", model.focusedScope)
	}
	model.updateList("i")
	if model.focusedScope != todoScopeGlobal {
		t.Fatalf("expected global focus after i, got %v", model.focusedScope)
	}
	model.updateList("n")
	if model.focusedScope != todoScopeWindow {
		t.Fatalf("expected window focus after n, got %v", model.focusedScope)
	}
}

func TestTodoPanelDefaultsToWindowFocusEvenWithoutWindowTodos(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Global:  []tmuxTodoItem{{Title: "global todo", Priority: 2, CreatedAt: time.Now()}},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	if model.focusedScope != todoScopeWindow {
		t.Fatalf("expected window focus by default, got %v", model.focusedScope)
	}
}

func TestTodoPanelAddModeAcceptsNICharacters(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{Version: tmuxTodoStoreVersion})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	model.updateList("a")

	model.updateAdd("n")
	model.updateAdd("i")

	if got := string(model.addText); got != "ni" {
		t.Fatalf("expected add text to include typed letters, got %q", got)
	}
	if model.addScope != todoScopeWindow {
		t.Fatalf("expected add scope to stay window while typing, got %v", model.addScope)
	}
}

func TestTodoPanelAddModeUsesArrowsAndTabForTarget(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{Version: tmuxTodoStoreVersion})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	model.updateList("a")

	model.updateAdd("right")
	if model.addScope != todoScopeGlobal {
		t.Fatalf("expected right to switch add scope to global, got %v", model.addScope)
	}
	model.updateAdd("left")
	if model.addScope != todoScopeWindow {
		t.Fatalf("expected left to switch add scope to window, got %v", model.addScope)
	}
	model.updateAdd("tab")
	if model.addScope != todoScopeGlobal {
		t.Fatalf("expected tab to toggle add scope to global, got %v", model.addScope)
	}
}

func TestTodoPanelCanEditSelectedTodo(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Windows: map[string][]tmuxTodoItem{
			"@1": {{Title: "old title", Priority: 2, CreatedAt: time.Now()}},
		},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}

	updated, cmd := model.updateList("E")
	if cmd != nil {
		t.Fatalf("expected no tea command when opening edit mode, got %v", cmd)
	}
	panel := updated.(*todoPanelModel)
	if panel.mode != todoPanelModeEdit {
		t.Fatalf("expected edit mode, got %v", panel.mode)
	}
	if got := string(panel.addText); got != "old title" {
		t.Fatalf("expected initial edit text, got %q", got)
	}

	for range []rune("old title") {
		panel.updateEdit("backspace")
	}
	for _, key := range []string{"n", "e", "w", " ", "t", "i", "t", "l", "e"} {
		panel.updateEdit(key)
	}
	updated, cmd = panel.updateEdit("enter")
	if cmd != nil {
		t.Fatalf("expected no tea command when saving edit, got %v", cmd)
	}
	panel = updated.(*todoPanelModel)
	if panel.mode != todoPanelModeList {
		t.Fatalf("expected list mode after save, got %v", panel.mode)
	}

	store, err := loadTmuxTodoStore()
	if err != nil {
		t.Fatalf("load todo store: %v", err)
	}
	if got := store.Windows["@1"][0].Title; got != "new title" {
		t.Fatalf("expected edited title, got %q", got)
	}
}

func TestTodoPanelCtrlEReordersFocusedColumn(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Windows: map[string][]tmuxTodoItem{
			"@1": {
				{Title: "first", Priority: 2, CreatedAt: time.Now()},
				{Title: "second", Priority: 2, CreatedAt: time.Now()},
			},
		},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	if _, cmd := model.updateList("ctrl+e"); cmd != nil {
		t.Fatalf("expected no tea command from reorder")
	}

	store, err := loadTmuxTodoStore()
	if err != nil {
		t.Fatalf("load todo store: %v", err)
	}
	items := store.Windows["@1"]
	if len(items) != 2 {
		t.Fatalf("expected 2 items after reorder, got %d", len(items))
	}
	if items[0].Title != "second" || items[1].Title != "first" {
		t.Fatalf("unexpected reordered items: %#v", items)
	}
	if model.selectedWindow != 1 {
		t.Fatalf("expected moved selection to stay on reordered item, got %d", model.selectedWindow)
	}
}

func TestTodoPanelWrapsLongTodoTitles(t *testing.T) {
	home := t.TempDir()
	longTitle := "this is a very long todo title that should wrap across multiple lines in the panel"
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Windows: map[string][]tmuxTodoItem{
			"@1": {{Title: longTitle, Priority: 2, CreatedAt: time.Now()}},
		},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	column := model.renderColumn(todoScopeWindow, 28, 10)
	if !strings.Contains(column, "this is a very") || !strings.Contains(column, "long todo title") {
		t.Fatalf("expected wrapped long todo across multiple lines, got %q", column)
	}
	if !strings.Contains(column, "MED") {
		t.Fatalf("expected priority chip to remain visible, got %q", column)
	}
}

func TestTodoPanelYCopiesSelectedTodo(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Windows: map[string][]tmuxTodoItem{
			"@1": {{Title: "window todo", Priority: 1, CreatedAt: time.Now()}},
		},
	})

	prev := todoPanelClipboardWriter
	t.Cleanup(func() { todoPanelClipboardWriter = prev })
	got := ""
	todoPanelClipboardWriter = func(value string) error {
		got = value
		return nil
	}

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}

	updated, cmd := model.updateList("y")
	if cmd != nil {
		t.Fatalf("expected no tea command from copy, got %v", cmd)
	}
	panel := updated.(*todoPanelModel)
	if got != "window todo" {
		t.Fatalf("expected copied todo title, got %q", got)
	}
	if status := panel.currentStatus(); status != "Copied todo" {
		t.Fatalf("expected copied status, got %q", status)
	}
}

func TestTodoPanelFooterStaysSingleLine(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Global:  []tmuxTodoItem{{Title: "global todo", Priority: 2, CreatedAt: time.Now()}},
		Windows: map[string][]tmuxTodoItem{
			"@1": {{Title: "window todo", Priority: 1, CreatedAt: time.Now()}},
		},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}

	for _, width := range []int{72, 78, 84, 96, 120} {
		footer := model.renderFooter(width)
		if strings.Contains(footer, "\n") {
			t.Fatalf("width %d: footer wrapped into multiple lines: %q", width, footer)
		}
		if got := lipgloss.Width(footer); got > maxInt(1, width-2) {
			t.Fatalf("width %d: footer width %d exceeds content width %d", width, got, maxInt(1, width-2))
		}
	}
}

func TestTodoPanelFooterStatusStaysSingleLine(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{Version: tmuxTodoStoreVersion})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	model.setStatus("something went wrong with a very long message", time.Minute)

	for _, width := range []int{72, 78, 84, 96, 120} {
		footer := model.renderFooter(width)
		if strings.Contains(footer, "\n") {
			t.Fatalf("width %d: footer wrapped into multiple lines: %q", width, footer)
		}
		if got := lipgloss.Width(footer); got > maxInt(1, width-2) {
			t.Fatalf("width %d: footer width %d exceeds content width %d", width, got, maxInt(1, width-2))
		}
	}
}

func TestTodoPanelFooterTogglesAltHints(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{Version: tmuxTodoStoreVersion})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}

	footer := model.renderFooter(96)
	if !strings.Contains(footer, "more") {
		t.Fatalf("expected default todo footer to advertise alt options, got %q", footer)
	}
	if !strings.Contains(footer, footerHintToggleKey) {
		t.Fatalf("expected default todo footer to show %q toggle, got %q", footerHintToggleKey, footer)
	}
	if strings.Contains(footer, "Alt-S") {
		t.Fatalf("expected default todo footer to hide alt shortcuts, got %q", footer)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	panel := updated.(*todoPanelModel)
	altFooter := panel.renderFooter(96)
	if !strings.Contains(altFooter, "Alt-S") {
		t.Fatalf("expected alt todo footer to show alt shortcuts, got %q", altFooter)
	}
	if strings.Contains(altFooter, "toggle") || strings.Contains(altFooter, "add") {
		t.Fatalf("expected alt todo footer to hide default shortcuts, got %q", altFooter)
	}
}

func TestTodoPanelViewNeverOverflowsWidth(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Global:  []tmuxTodoItem{{Title: "global todo", Priority: 2, CreatedAt: time.Now()}},
		Windows: map[string][]tmuxTodoItem{
			"@1": {
				{Title: "window todo", Priority: 1, CreatedAt: time.Now()},
				{Title: "another todo", Priority: 3, CreatedAt: time.Now()},
			},
		},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}

	for _, width := range []int{72, 78, 84, 96, 120} {
		model.width = width
		model.height = 20
		view := model.View()
		for idx, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width %d: line %d width %d exceeds viewport %d", width, idx+1, got, width)
			}
		}
	}
}

func TestTodoPanelFooterHasNoSpacerLine(t *testing.T) {
	home := t.TempDir()
	windowItems := make([]tmuxTodoItem, 0, 12)
	globalItems := make([]tmuxTodoItem, 0, 12)
	for i := 0; i < 12; i++ {
		windowItems = append(windowItems, tmuxTodoItem{Title: fmt.Sprintf("window %d", i), Priority: 1, CreatedAt: time.Now()})
		globalItems = append(globalItems, tmuxTodoItem{Title: fmt.Sprintf("global %d", i), Priority: 2, CreatedAt: time.Now()})
	}
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Global:  globalItems,
		Windows: map[string][]tmuxTodoItem{"@1": windowItems},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	for _, width := range []int{72, 78, 84, 96, 120} {
		model.width = width
		model.height = 20

		lines := strings.Split(model.View(), "\n")
		if len(lines) < 2 {
			t.Fatalf("width %d: expected multi-line view, got %q", width, model.View())
		}
		last := strings.TrimSpace(lines[len(lines)-1])
		prev := strings.TrimSpace(lines[len(lines)-2])
		if !strings.Contains(last, "Esc") {
			t.Fatalf("width %d: expected footer on last line, got %q", width, last)
		}
		if prev == "" {
			t.Fatalf("width %d: expected no blank spacer line above footer", width)
		}
	}
}

func TestTodoPanelFooterShowsCopyShortcutWhenSpaceAllows(t *testing.T) {
	home := t.TempDir()
	writeTestTodoStore(t, home, &tmuxTodoStore{
		Version: tmuxTodoStoreVersion,
		Windows: map[string][]tmuxTodoItem{
			"@1": {{Title: "window todo", Priority: 1, CreatedAt: time.Now()}},
		},
	})

	model, err := newTodoPanelModel("$1", "@1")
	if err != nil {
		t.Fatalf("new todo panel model: %v", err)
	}
	footer := model.renderFooter(96)
	if !strings.Contains(footer, "copy") || !strings.Contains(footer, "y") {
		t.Fatalf("expected footer to show copy shortcut, got %q", footer)
	}
	if !strings.Contains(footer, "edit") || !strings.Contains(footer, "E") {
		t.Fatalf("expected footer to show edit shortcut, got %q", footer)
	}
}
