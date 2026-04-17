package main

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var todoPanelClipboardWriter = writeTodoClipboard

type todoPanelMode int

const (
	todoPanelModeList todoPanelMode = iota
	todoPanelModeAdd
	todoPanelModeEdit
	todoPanelModeConfirmDelete
)

type todoPanelPane int

const (
	todoPanelPaneWindow todoPanelPane = iota
	todoPanelPaneAllWindows
	todoPanelPaneGlobal
)

type todoPanelModel struct {
	entries         []tmuxTodoEntry
	focusedPane     todoPanelPane
	lastWindowPane  todoPanelPane
	selectedWindow  int
	selectedAllWin  int
	selectedGlobal  int
	windowOffset    int
	allWinOffset    int
	globalOffset    int
	mode            todoPanelMode
	sessionID       string
	windowID        string
	width           int
	height          int
	status          string
	statusUntil     time.Time
	addText         []rune
	addCursor       int
	addScope        todoScope
	deleteEntry     *tmuxTodoEntry
	editEntry       *tmuxTodoEntry
	closePalette    bool
	showAltHints    bool
	showCompleted   bool
	keepVisibleDone map[string]bool
	styles          todoPanelStyles
}

type todoPanelStyles struct {
	title        lipgloss.Style
	meta         lipgloss.Style
	subtle       lipgloss.Style
	input        lipgloss.Style
	inputCursor  lipgloss.Style
	item         lipgloss.Style
	itemSelected lipgloss.Style
	itemMuted    lipgloss.Style
	itemTitle    lipgloss.Style
	itemTitleDim lipgloss.Style
	itemMeta     lipgloss.Style
	checkBox     lipgloss.Style
	checkDone    lipgloss.Style
	scopeLabel   lipgloss.Style
	currentLabel lipgloss.Style
	mutedLabel   lipgloss.Style
	divider      lipgloss.Style
	footer       lipgloss.Style
	status       lipgloss.Style
	statusBad    lipgloss.Style
	shortcutKey  lipgloss.Style
	shortcutText lipgloss.Style
	modal        lipgloss.Style
	modalTitle   lipgloss.Style
	modalBody    lipgloss.Style
	modalHint    lipgloss.Style
}

func newTodoPanelStyles() todoPanelStyles {
	accent := lipgloss.Color("223")
	cyan := lipgloss.Color("117")
	selected := lipgloss.Color("238")
	text := lipgloss.Color("252")
	muted := lipgloss.Color("245")
	bright := lipgloss.Color("230")
	warning := lipgloss.Color("203")
	success := lipgloss.Color("150")
	return todoPanelStyles{
		title:        lipgloss.NewStyle().Bold(true).Foreground(bright),
		meta:         lipgloss.NewStyle().Foreground(muted),
		subtle:       lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		input:        lipgloss.NewStyle().Foreground(text),
		inputCursor:  lipgloss.NewStyle().Foreground(lipgloss.Color("16")).Background(accent).Bold(true),
		item:         lipgloss.NewStyle().Padding(0, 1),
		itemSelected: lipgloss.NewStyle().Background(selected).Padding(0, 1),
		itemMuted:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1),
		itemTitle:    lipgloss.NewStyle().Foreground(text).Bold(true),
		itemTitleDim: lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Bold(true),
		itemMeta:     lipgloss.NewStyle().Foreground(muted),
		checkBox:     lipgloss.NewStyle().Foreground(muted),
		checkDone:    lipgloss.NewStyle().Foreground(success),
		scopeLabel:   lipgloss.NewStyle().Foreground(cyan).Background(lipgloss.Color("237")).Padding(0, 1).Bold(true),
		currentLabel: lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(success).Padding(0, 1).Bold(true),
		mutedLabel:   lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("241")).Padding(0, 1).Bold(true),
		divider:      lipgloss.NewStyle().Foreground(lipgloss.Color("239")),
		footer:       lipgloss.NewStyle().Foreground(lipgloss.Color("216")),
		status:       lipgloss.NewStyle().Foreground(success),
		statusBad:    lipgloss.NewStyle().Foreground(warning),
		shortcutKey:  lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(accent).Padding(0, 1).Bold(true),
		shortcutText: lipgloss.NewStyle().Foreground(muted),
		modal:        lipgloss.NewStyle().Border(paletteModalBorder).BorderForeground(accent).Padding(1, 2).Background(lipgloss.Color("235")),
		modalTitle:   lipgloss.NewStyle().Bold(true).Foreground(warning),
		modalBody:    lipgloss.NewStyle().Foreground(text),
		modalHint:    lipgloss.NewStyle().Foreground(muted),
	}
}

func runTodoPanel() error {
	sessionID, windowID := getCurrentTmuxScopeInfo()
	model, err := newTodoPanelModel(sessionID, windowID)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(model).Run()
	if err != nil {
		return err
	}
	if model.closePalette {
		return errClosePalette
	}
	return nil
}

func newTodoPanelModel(sessionID, windowID string) (*todoPanelModel, error) {
	if _, err := loadTmuxTodoStore(); err != nil {
		return nil, err
	}
	model := &todoPanelModel{
		sessionID:       strings.TrimSpace(sessionID),
		windowID:        strings.TrimSpace(windowID),
		focusedPane:     todoPanelPaneWindow,
		lastWindowPane:  todoPanelPaneWindow,
		keepVisibleDone: map[string]bool{},
		styles:          newTodoPanelStyles(),
		mode:            todoPanelModeList,
	}
	model.reloadEntries()
	return model, nil
}

func collectTodoPanelEntries(currentSessionID, currentWindowID string) []tmuxTodoEntry {
	store, err := loadTmuxTodoStore()
	if err != nil {
		return nil
	}
	windowLabels := todoPanelWindowLabels(currentSessionID)
	allWindowCount := 0
	for windowID, items := range store.Windows {
		if strings.TrimSpace(windowID) == strings.TrimSpace(currentWindowID) {
			continue
		}
		allWindowCount += len(items)
	}
	entries := make([]tmuxTodoEntry, 0, len(store.Global)+len(store.Windows[currentWindowID])+allWindowCount)
	for idx, item := range store.Windows[currentWindowID] {
		entries = append(entries, tmuxTodoEntry{
			Title:     item.Title,
			Done:      item.Done,
			Priority:  item.Priority,
			Scope:     todoScopeWindow,
			ScopeID:   currentWindowID,
			ScopeName: "Window",
			IsCurrent: true,
			ItemIndex: idx,
			PanelPane: todoPanelPaneWindow,
		})
	}
	windowIDs := make([]string, 0, len(store.Windows))
	for windowID := range store.Windows {
		if strings.TrimSpace(windowID) == strings.TrimSpace(currentWindowID) {
			continue
		}
		windowIDs = append(windowIDs, windowID)
	}
	sort.Strings(windowIDs)
	for _, windowID := range windowIDs {
		for idx, item := range store.Windows[windowID] {
			entries = append(entries, tmuxTodoEntry{
				Title:     item.Title,
				Done:      item.Done,
				Priority:  item.Priority,
				Scope:     todoScopeWindow,
				ScopeID:   windowID,
				ScopeName: "Window",
				ItemIndex: idx,
				PanelPane: todoPanelPaneAllWindows,
				Detail:    windowLabels[windowID],
			})
		}
	}
	for idx, item := range store.Global {
		entries = append(entries, tmuxTodoEntry{
			Title:     item.Title,
			Done:      item.Done,
			Priority:  item.Priority,
			Scope:     todoScopeGlobal,
			ScopeID:   "global",
			ScopeName: "Global",
			IsCurrent: true,
			ItemIndex: idx,
			PanelPane: todoPanelPaneGlobal,
		})
	}
	return entries
}

func (m *todoPanelModel) reloadEntries() {
	m.entries = collectTodoPanelEntries(m.sessionID, m.windowID)
	m.pruneKeepVisibleDone()
	m.clampSelections()
}

func todoPanelWindowLabels(currentSessionID string) map[string]string {
	labels := map[string]string{}
	out, err := runTmuxOutput("list-windows", "-a", "-F", "#{session_id}\t#{window_id}\t#{session_name}\t#{window_index}\t#{window_name}")
	if err != nil {
		return labels
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 5 {
			continue
		}
		sessionID := strings.TrimSpace(parts[0])
		windowID := strings.TrimSpace(parts[1])
		sessionName := strings.TrimSpace(parts[2])
		windowIndex := strings.TrimSpace(parts[3])
		windowName := strings.TrimSpace(parts[4])
		label := strings.TrimSpace(strings.TrimSpace(windowIndex+" ") + windowName)
		if label == "" {
			label = windowID
		}
		if sessionID != strings.TrimSpace(currentSessionID) && sessionName != "" {
			label = sessionName + " · " + label
		}
		labels[windowID] = label
	}
	return labels
}

func (m *todoPanelModel) Init() tea.Cmd {
	return nil
}

func (m *todoPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if isAltFooterToggleKey(msg) {
			m.showAltHints = !m.showAltHints
			return m, nil
		}
		m.showAltHints = false
		key := msg.String()
		if key == "alt+s" {
			m.closePalette = true
			return m, tea.Quit
		}
		if key == "esc" && m.mode == todoPanelModeList {
			return m, tea.Quit
		}
		switch m.mode {
		case todoPanelModeAdd:
			return m.updateAdd(key)
		case todoPanelModeEdit:
			return m.updateEdit(key)
		case todoPanelModeConfirmDelete:
			return m.updateConfirmDelete(key)
		default:
			return m.updateList(key)
		}
	}
	return m, nil
}

func (m *todoPanelModel) updateList(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "u", "up":
		m.moveSelection(-1)
	case "e", "down":
		m.moveSelection(1)
	case "ctrl+u":
		return m.moveSelectedTodo(-1)
	case "ctrl+e":
		return m.moveSelectedTodo(1)
	case "n", "left":
		m.setFocusedPane(m.lastWindowPane)
		m.clampSelections()
	case "i", "right":
		m.setFocusedPane(todoPanelPaneGlobal)
		m.clampSelections()
	case "tab", "shift+tab":
		m.toggleWindowPaneFocus()
		m.clampSelections()
	case "N":
		if m.focusedPane == todoPanelPaneGlobal {
			return m.transferSelectedTodo(todoScopeWindow)
		}
	case "I":
		if m.focusedPane == todoPanelPaneWindow || m.focusedPane == todoPanelPaneAllWindows {
			return m.transferSelectedTodo(todoScopeGlobal)
		}
	case "enter":
		entry, ok := m.selectedEntry(m.focusedPane)
		if !ok {
			return m, nil
		}
		if err := focusTodoEntry(entry); err != nil {
			m.setStatus(err.Error(), 1500*time.Millisecond)
			return m, nil
		}
		return m, tea.Quit
	case " ":
		entry, ok := m.selectedEntry(m.focusedPane)
		if !ok {
			return m, nil
		}
		entryKey := todoEntryKey(entry)
		toggledDone := !entry.Done
		if err := toggleTmuxTodoByIndex(entry.Scope, entry.ScopeID, entry.ItemIndex); err != nil {
			m.setStatus(err.Error(), 1500*time.Millisecond)
		} else {
			if toggledDone {
				m.keepVisibleDone[entryKey] = true
			} else {
				delete(m.keepVisibleDone, entryKey)
			}
			m.reloadEntries()
		}
	case "a":
		m.mode = todoPanelModeAdd
		m.addText = nil
		m.addCursor = 0
		m.addScope = m.defaultAddScope()
	case "E":
		if entry, ok := m.selectedEntry(m.focusedPane); ok {
			entryCopy := entry
			m.editEntry = &entryCopy
			m.mode = todoPanelModeEdit
			m.addText = []rune(entry.Title)
			m.addCursor = len(m.addText)
			m.addScope = entry.Scope
		}
	case "y":
		entry, ok := m.selectedEntry(m.focusedPane)
		if !ok {
			return m, nil
		}
		if err := todoPanelClipboardWriter(entry.Title); err != nil {
			m.setStatus(err.Error(), 1500*time.Millisecond)
		} else {
			m.setStatus("Copied todo", 1500*time.Millisecond)
		}
	case "d", "x":
		if entry, ok := m.selectedEntry(m.focusedPane); ok {
			m.deleteEntry = &entry
			m.mode = todoPanelModeConfirmDelete
		}
	case "1":
		return m.setSelectedPriority(1)
	case "2":
		return m.setSelectedPriority(2)
	case "3":
		return m.setSelectedPriority(3)
	case "c":
		m.showCompleted = !m.showCompleted
		m.clampSelections()
	}
	return m, nil
}

func (m *todoPanelModel) updateAdd(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.mode = todoPanelModeList
		return m, nil
	}
	if key == "enter" {
		title := strings.TrimSpace(string(m.addText))
		if title == "" {
			m.mode = todoPanelModeList
			return m, nil
		}
		scopeID := m.scopeID(m.addScope)
		if err := addTmuxTodo(m.addScope, scopeID, title); err != nil {
			m.setStatus(err.Error(), 1500*time.Millisecond)
		} else {
			m.reloadEntries()
			targetPane := m.paneForScope(m.addScope)
			m.setFocusedPane(targetPane)
			m.setSelectedIndex(targetPane, maxInt(0, len(m.visibleEntries(targetPane))-1))
		}
		m.mode = todoPanelModeList
		return m, nil
	}
	if key == "left" {
		m.addScope = todoScopeWindow
		return m, nil
	}
	if key == "right" {
		m.addScope = todoScopeGlobal
		return m, nil
	}
	if key == "tab" {
		if m.addScope == todoScopeWindow {
			m.addScope = todoScopeGlobal
		} else {
			m.addScope = todoScopeWindow
		}
		return m, nil
	}
	applyPaletteInputKey(key, &m.addText, &m.addCursor, true)
	return m, nil
}

func (m *todoPanelModel) updateEdit(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.editEntry = nil
		m.mode = todoPanelModeList
		return m, nil
	}
	if key == "enter" {
		if m.editEntry == nil {
			m.mode = todoPanelModeList
			return m, nil
		}
		title := strings.TrimSpace(string(m.addText))
		if title == "" {
			m.setStatus("todo title is required", 1500*time.Millisecond)
			return m, nil
		}
		if err := updateTmuxTodoTitleByIndex(m.editEntry.Scope, m.editEntry.ScopeID, m.editEntry.ItemIndex, title); err != nil {
			m.setStatus(err.Error(), 1500*time.Millisecond)
		} else {
			focusedPane := m.focusedPane
			selected := m.selectedIndex(focusedPane)
			m.reloadEntries()
			m.setSelectedIndex(focusedPane, selected)
		}
		m.editEntry = nil
		m.mode = todoPanelModeList
		return m, nil
	}
	applyPaletteInputKey(key, &m.addText, &m.addCursor, true)
	return m, nil
}

func (m *todoPanelModel) setSelectedPriority(priority int) (tea.Model, tea.Cmd) {
	entry, ok := m.selectedEntry(m.focusedPane)
	if !ok {
		return m, nil
	}
	if err := setTmuxTodoPriorityByIndex(entry.Scope, entry.ScopeID, entry.ItemIndex, priority); err != nil {
		m.setStatus(err.Error(), 1500*time.Millisecond)
		return m, nil
	}
	focusedPane := m.focusedPane
	selected := m.selectedIndex(focusedPane)
	m.reloadEntries()
	m.setSelectedIndex(focusedPane, selected)
	return m, nil
}

func (m *todoPanelModel) moveSelectedTodo(delta int) (tea.Model, tea.Cmd) {
	if m.focusedPane == todoPanelPaneAllWindows {
		m.setStatus("Reorder unavailable in all windows", 1500*time.Millisecond)
		return m, nil
	}
	entries := m.visibleEntries(m.focusedPane)
	selected := m.selectedIndex(m.focusedPane)
	if len(entries) == 0 || selected < 0 || selected >= len(entries) {
		return m, nil
	}
	target := selected + delta
	if target < 0 || target >= len(entries) {
		return m, nil
	}
	entry := entries[selected]
	targetEntry := entries[target]
	if err := moveTmuxTodoByIndex(entry.Scope, entry.ScopeID, entry.ItemIndex, targetEntry.ItemIndex); err != nil {
		m.setStatus(err.Error(), 1500*time.Millisecond)
		return m, nil
	}
	m.reloadEntries()
	m.setSelectedIndex(m.focusedPane, target)
	return m, nil
}

func (m *todoPanelModel) transferSelectedTodo(targetScope todoScope) (tea.Model, tea.Cmd) {
	entry, ok := m.selectedEntry(m.focusedPane)
	if !ok {
		return m, nil
	}
	if entry.Scope == targetScope {
		return m, nil
	}
	targetScopeID := m.scopeID(targetScope)
	if targetScope == todoScopeWindow && strings.TrimSpace(targetScopeID) == "" {
		m.setStatus("window todo scope unavailable", 1500*time.Millisecond)
		return m, nil
	}
	sourcePane := m.focusedPane
	targetPane := m.paneForScope(targetScope)
	sourceSelected := m.selectedIndex(sourcePane)
	targetItemIndex := len(m.allEntries(targetPane))
	if err := moveTmuxTodoToScopeByIndex(entry.Scope, entry.ScopeID, entry.ItemIndex, targetScope, targetScopeID); err != nil {
		m.setStatus(err.Error(), 1500*time.Millisecond)
		return m, nil
	}
	m.reloadEntries()
	if entry.Done && !m.showCompleted {
		movedEntry := entry
		movedEntry.Scope = targetScope
		movedEntry.ScopeID = targetScopeID
		movedEntry.ItemIndex = targetItemIndex
		movedEntry.PanelPane = targetPane
		m.keepVisibleDone[todoEntryKey(movedEntry)] = true
	}
	m.setSelectedIndex(sourcePane, sourceSelected)
	targetSelected := 0
	for idx, candidate := range m.visibleEntries(targetPane) {
		if candidate.Scope == targetScope && candidate.ScopeID == targetScopeID && candidate.ItemIndex == targetItemIndex {
			targetSelected = idx
			break
		}
	}
	m.setSelectedIndex(targetPane, targetSelected)
	m.clampSelections()
	m.setFocusedPane(targetPane)
	m.setStatus(fmt.Sprintf("Moved todo to %s", strings.ToLower(todoScopeLabel(targetScope))), 1500*time.Millisecond)
	return m, nil
}

func (m *todoPanelModel) updateConfirmDelete(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "n" {
		m.deleteEntry = nil
		m.mode = todoPanelModeList
		return m, nil
	}
	if key == "y" || key == "enter" {
		if m.deleteEntry != nil {
			if err := deleteTmuxTodoByIndex(m.deleteEntry.Scope, m.deleteEntry.ScopeID, m.deleteEntry.ItemIndex); err != nil {
				m.setStatus(err.Error(), 1500*time.Millisecond)
			} else {
				m.reloadEntries()
			}
		}
		m.deleteEntry = nil
		m.mode = todoPanelModeList
		return m, nil
	}
	return m, nil
}

func (m *todoPanelModel) View() string {
	w := m.width
	h := m.height
	if w < 52 || h < 14 {
		return m.styles.title.Render("Window too small")
	}

	switch m.mode {
	case todoPanelModeAdd:
		return m.renderAddMode(w, h)
	case todoPanelModeEdit:
		return m.renderEditMode(w, h)
	case todoPanelModeConfirmDelete:
		return m.renderConfirmDelete(w, h)
	default:
		return m.renderList(w, h)
	}
}

func (m *todoPanelModel) renderList(w, h int) string {
	openCount := 0
	doneCount := 0
	for _, entry := range m.entries {
		if entry.Done {
			doneCount++
		} else {
			openCount++
		}
	}
	completedLabel := "hidden"
	if m.showCompleted {
		completedLabel = "shown"
	}
	headerLine := m.styles.title.Render(fmt.Sprintf("Todo Panel  %d open  %d done", openCount, doneCount))
	metaLine := m.styles.meta.Render(fmt.Sprintf("Window %d  All Windows %d  Global %d  Completed %s", len(m.visibleEntries(todoPanelPaneWindow)), len(m.visibleEntries(todoPanelPaneAllWindows)), len(m.visibleEntries(todoPanelPaneGlobal)), completedLabel))

	contentH := h - 4
	leftW := maxInt(24, (w-1)/2)
	rightW := maxInt(24, w-leftW-1)
	upperLeftH := maxInt(4, (contentH-1)/2)
	lowerLeftH := maxInt(4, contentH-upperLeftH-1)
	if upperLeftH+lowerLeftH+1 > contentH {
		lowerLeftH = maxInt(1, contentH-upperLeftH-1)
	}
	leftColumn := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Width(leftW).Height(upperLeftH).Render(m.renderPane(todoPanelPaneWindow, leftW, upperLeftH)),
		m.renderHorizontalDivider(leftW),
		lipgloss.NewStyle().Width(leftW).Height(lowerLeftH).Render(m.renderPane(todoPanelPaneAllWindows, leftW, lowerLeftH)),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftW).Height(contentH).Render(leftColumn),
		m.renderDivider(contentH),
		lipgloss.NewStyle().Width(rightW).Height(contentH).Render(m.renderPane(todoPanelPaneGlobal, rightW, contentH)),
	)
	footer := m.renderFooter(w)

	view := lipgloss.JoinVertical(lipgloss.Left,
		headerLine,
		metaLine,
		"",
		body,
		footer,
	)
	return lipgloss.NewStyle().Width(w).Height(h).Padding(0, 1).Render(view)
}

func (m *todoPanelModel) renderPane(pane todoPanelPane, width, height int) string {
	entries := m.visibleEntries(pane)
	selected := m.selectedIndex(pane)
	labelStyle := m.styles.mutedLabel
	if m.focusedPane == pane {
		labelStyle = m.styles.currentLabel
	}
	label := labelStyle.Render(todoPanelPaneLabel(pane))
	header := lipgloss.JoinHorizontal(lipgloss.Left, label, " ", m.styles.meta.Render(fmt.Sprintf("%d items", len(entries))))
	lines := []string{header, ""}
	if len(entries) == 0 {
		message, hint := m.emptyPaneState(pane)
		lines = append(lines, m.styles.itemMuted.Width(width).Render(message))
		if hint != "" {
			lines = append(lines, m.styles.subtle.Width(width).Render(hint))
		}
		return lipgloss.NewStyle().Width(width).Height(height).Render(strings.Join(lines, "\n"))
	}

	visibleRows := maxInt(1, height-2)
	offset := stableListOffset(m.selectedOffset(pane), selected, visibleRows, len(entries))
	m.setSelectedOffset(pane, offset)
	usedRows := 0
	for idx := offset; usedRows < visibleRows; idx++ {
		if idx >= len(entries) {
			break
		}
		entry := entries[idx]
		isSelected := m.focusedPane == pane && idx == selected
		entryLines := m.renderTodoEntryLines(entry, width, isSelected)
		remaining := visibleRows - usedRows
		if len(entryLines) > remaining {
			entryLines = entryLines[:remaining]
		}
		lines = append(lines, entryLines...)
		usedRows += len(entryLines)
	}
	return lipgloss.NewStyle().Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (m *todoPanelModel) renderTodoEntryLines(entry tmuxTodoEntry, width int, isSelected bool) []string {
	check := "○"
	checkStyle := m.styles.checkBox
	boxStyle := m.styles.item.Width(width)
	titleStyle := m.styles.itemTitle
	fillStyle := lipgloss.NewStyle()
	if entry.Done {
		check = "●"
		checkStyle = m.styles.checkDone
		titleStyle = m.styles.itemTitleDim
		boxStyle = m.styles.itemMuted.Width(width)
	}
	if isSelected {
		selectedBG := lipgloss.Color("238")
		boxStyle = m.styles.itemSelected.Width(width)
		checkStyle = checkStyle.Background(selectedBG)
		titleStyle = titleStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
		fillStyle = fillStyle.Background(selectedBG)
	}
	priorityChip := renderTodoPriorityChip(entry.Priority)
	innerWidth := maxInt(16, width-2)
	prefixWidth := lipgloss.Width(check) + 1
	chipWidth := lipgloss.Width(priorityChip)
	titleWidth := maxInt(8, innerWidth-prefixWidth-chipWidth-1)
	titleLines := wrapTodoTitle(entry.Title, titleWidth)
	if len(titleLines) == 0 {
		titleLines = []string{""}
	}
	gapWidth := maxInt(1, innerWidth-prefixWidth-lipgloss.Width(titleLines[0])-chipWidth)
	rowLines := []string{boxStyle.Render(lipgloss.JoinHorizontal(lipgloss.Left,
		checkStyle.Render(check),
		fillStyle.Render(" "),
		titleStyle.Render(titleLines[0]),
		fillStyle.Render(strings.Repeat(" ", gapWidth)),
		priorityChip,
	))}
	continuationPrefix := fillStyle.Render(strings.Repeat(" ", prefixWidth))
	for _, line := range titleLines[1:] {
		padding := maxInt(0, innerWidth-prefixWidth-lipgloss.Width(line))
		rowLines = append(rowLines, boxStyle.Render(continuationPrefix+titleStyle.Render(line)+fillStyle.Render(strings.Repeat(" ", padding))))
	}
	if detail := strings.TrimSpace(entry.Detail); detail != "" {
		metaStyle := m.styles.itemMeta
		if entry.Done {
			metaStyle = metaStyle.Foreground(lipgloss.Color("242"))
		}
		if isSelected {
			metaStyle = metaStyle.Background(lipgloss.Color("238")).Foreground(lipgloss.Color("247"))
		}
		detail = truncate(detail, maxInt(8, innerWidth-prefixWidth))
		padding := maxInt(0, innerWidth-prefixWidth-lipgloss.Width(detail))
		rowLines = append(rowLines, boxStyle.Render(continuationPrefix+metaStyle.Render(detail)+fillStyle.Render(strings.Repeat(" ", padding))))
	}
	return rowLines
}

func (m *todoPanelModel) renderAddMode(w, h int) string {
	target := todoScopeLabel(m.addScope)
	body := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.modalTitle.Render("Add Todo"),
		m.styles.modalBody.Render(fmt.Sprintf("Target: %s", target)),
		"",
		m.styles.input.Render(renderTodoInputValue(m.addText, m.addCursor, m.styles)),
		"",
		m.styles.modalHint.Render("Enter save  Tab/left/right target  Esc cancel"),
	)
	box := m.styles.modal.Width(minInt(72, maxInt(34, w-10))).Render(body)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

func (m *todoPanelModel) renderEditMode(w, h int) string {
	target := todoScopeLabel(m.addScope)
	body := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.modalTitle.Render("Edit Todo"),
		m.styles.modalBody.Render(fmt.Sprintf("Target: %s", target)),
		"",
		m.styles.input.Render(renderTodoInputValue(m.addText, m.addCursor, m.styles)),
		"",
		m.styles.modalHint.Render("Enter save  Esc cancel"),
	)
	box := m.styles.modal.Width(minInt(72, maxInt(34, w-10))).Render(body)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

func (m *todoPanelModel) renderConfirmDelete(w, h int) string {
	title := "Delete todo?"
	if m.deleteEntry != nil {
		title = fmt.Sprintf("Delete \"%s\"?", truncate(m.deleteEntry.Title, 40))
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.modalTitle.Render(title),
		m.styles.modalBody.Render("This removes it from local tmux todos."),
		"",
		m.styles.modalHint.Render("y confirm  n cancel"),
	)
	box := m.styles.modal.Width(minInt(48, w-10)).Render(body)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

func (m *todoPanelModel) renderFooter(w int) string {
	contentWidth := maxInt(1, w-2)
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return m.styles.shortcutKey.Render(v) }, func(v string) string { return m.styles.shortcutText.Render(v) }, "  ", pairs)
	}
	footer := ""
	if m.showAltHints {
		footer = pickRenderedShortcutFooter(contentWidth, renderSegments,
			[][2]string{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
			[][2]string{{"Alt-S", "close"}},
		)
	} else {
		footer = pickRenderedShortcutFooter(contentWidth, renderSegments,
			[][2]string{{"u/e", "move"}, {"Enter", "goto"}, {"Ctrl-U/E", "reorder"}, {"n/i", "column"}, {"Tab", "window"}, {"Shift-N/I", "scope"}, {"Space", "toggle"}, {"a", "add"}, {"E", "edit"}, {"y", "copy"}, {"d", "delete"}, {"1/2/3", "priority"}, {"c", "completed"}, {"Esc", "close"}, {footerHintToggleKey, "more"}},
			[][2]string{{"u/e", "move"}, {"Enter", "goto"}, {"Ctrl-U/E", "reorder"}, {"n/i", "col"}, {"Tab", "win"}, {"N/I", "scope"}, {"Space", "toggle"}, {"a", "add"}, {"E", "edit"}, {"y", "copy"}, {"d", "del"}, {"c", "done"}, {"Esc", "close"}, {footerHintToggleKey, "more"}},
			[][2]string{{"u/e", "move"}, {"Enter", "goto"}, {"Ctrl-U/E", "reorder"}, {"n/i", "col"}, {"Tab", "win"}, {"N/I", "scope"}, {"Space", "toggle"}, {"a", "add"}, {"E", "edit"}, {"y", "copy"}, {"d", "del"}, {"Esc", "close"}, {footerHintToggleKey, "more"}},
			[][2]string{{"u/e", "move"}, {"Enter", "goto"}, {"n/i", "col"}, {"Tab", "win"}, {"N/I", "scope"}, {"Space", "toggle"}, {"a", "add"}, {"E", "edit"}, {"y", "copy"}, {"d", "del"}, {"Esc", "close"}, {footerHintToggleKey, "more"}},
			[][2]string{{"u/e", "move"}, {"Enter", "goto"}, {"n/i", "col"}, {"Tab", "win"}, {"N/I", "scope"}, {"a", "add"}, {"E", "edit"}, {"y", "copy"}, {"d", "del"}, {"Esc", "close"}, {footerHintToggleKey, "more"}},
			[][2]string{{"Esc", "close"}, {footerHintToggleKey, "more"}},
		)
	}
	status := strings.TrimSpace(m.currentStatus())
	if status != "" {
		statusText := m.styles.statusBad.Render(truncate(status, maxInt(12, minInt(20, contentWidth/4))))
		if lipgloss.Width(footer)+2+lipgloss.Width(statusText) <= contentWidth {
			gap := contentWidth - lipgloss.Width(footer) - lipgloss.Width(statusText)
			if gap < 2 {
				gap = 2
			}
			return footer + strings.Repeat(" ", gap) + statusText
		}
		if lipgloss.Width(statusText) <= contentWidth {
			return lipgloss.NewStyle().Width(contentWidth).Render(statusText)
		}
	}
	return lipgloss.NewStyle().Width(contentWidth).Render(footer)
}

func (m *todoPanelModel) scopeID(scope todoScope) string {
	if scope == todoScopeGlobal {
		return "global"
	}
	return m.windowID
}

func (m *todoPanelModel) allEntries(pane todoPanelPane) []tmuxTodoEntry {
	entries := make([]tmuxTodoEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		if entry.PanelPane == pane {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (m *todoPanelModel) visibleEntries(pane todoPanelPane) []tmuxTodoEntry {
	entries := make([]tmuxTodoEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		if entry.PanelPane != pane {
			continue
		}
		if entry.Done && !m.showCompleted && !m.keepVisibleDone[todoEntryKey(entry)] {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func (m *todoPanelModel) selectedIndex(pane todoPanelPane) int {
	switch pane {
	case todoPanelPaneAllWindows:
		return m.selectedAllWin
	case todoPanelPaneGlobal:
		return m.selectedGlobal
	default:
		return m.selectedWindow
	}
}

func (m *todoPanelModel) setSelectedIndex(pane todoPanelPane, index int) {
	switch pane {
	case todoPanelPaneAllWindows:
		m.selectedAllWin = index
	case todoPanelPaneGlobal:
		m.selectedGlobal = index
	default:
		m.selectedWindow = index
	}
}

func (m *todoPanelModel) selectedOffset(pane todoPanelPane) int {
	switch pane {
	case todoPanelPaneAllWindows:
		return m.allWinOffset
	case todoPanelPaneGlobal:
		return m.globalOffset
	default:
		return m.windowOffset
	}
}

func (m *todoPanelModel) setSelectedOffset(pane todoPanelPane, offset int) {
	switch pane {
	case todoPanelPaneAllWindows:
		m.allWinOffset = offset
	case todoPanelPaneGlobal:
		m.globalOffset = offset
	default:
		m.windowOffset = offset
	}
}

func (m *todoPanelModel) clampSelections() {
	for _, pane := range []todoPanelPane{todoPanelPaneWindow, todoPanelPaneAllWindows, todoPanelPaneGlobal} {
		entries := m.visibleEntries(pane)
		selected := m.selectedIndex(pane)
		if len(entries) == 0 {
			m.setSelectedIndex(pane, 0)
			continue
		}
		if selected < 0 {
			selected = 0
		}
		if selected >= len(entries) {
			selected = len(entries) - 1
		}
		m.setSelectedIndex(pane, selected)
	}
}

func (m *todoPanelModel) selectedEntry(pane todoPanelPane) (tmuxTodoEntry, bool) {
	entries := m.visibleEntries(pane)
	selected := m.selectedIndex(pane)
	if len(entries) == 0 || selected < 0 || selected >= len(entries) {
		return tmuxTodoEntry{}, false
	}
	return entries[selected], true
}

func (m *todoPanelModel) moveSelection(delta int) {
	entries := m.visibleEntries(m.focusedPane)
	if len(entries) == 0 {
		return
	}
	selected := clampInt(m.selectedIndex(m.focusedPane)+delta, 0, len(entries)-1)
	m.setSelectedIndex(m.focusedPane, selected)
}

func (m *todoPanelModel) renderDivider(height int) string {
	lines := make([]string, maxInt(1, height))
	for i := range lines {
		lines[i] = m.styles.divider.Render("│")
	}
	return strings.Join(lines, "\n")
}

func (m *todoPanelModel) renderHorizontalDivider(width int) string {
	return m.styles.divider.Render(strings.Repeat("─", maxInt(1, width)))
}

func (m *todoPanelModel) emptyPaneState(pane todoPanelPane) (message, hint string) {
	if m.showCompleted {
		switch pane {
		case todoPanelPaneAllWindows:
			return "No todos in other windows", ""
		case todoPanelPaneGlobal:
			return "No todos", "Press a to add a todo here."
		default:
			return "No todos", "Press a to add a todo here."
		}
	}
	if len(m.allEntries(pane)) > 0 {
		return map[bool]string{true: "No open todos in other windows", false: "No open todos"}[pane == todoPanelPaneAllWindows], "Press c to show completed todos."
	}
	if pane == todoPanelPaneAllWindows {
		return "No todos in other windows", "Tab switches the window panes."
	}
	return "No open todos", "Press a to add a todo here."
}

func (m *todoPanelModel) setFocusedPane(pane todoPanelPane) {
	m.focusedPane = pane
	if pane == todoPanelPaneWindow || pane == todoPanelPaneAllWindows {
		m.lastWindowPane = pane
	}
}

func (m *todoPanelModel) toggleWindowPaneFocus() {
	if m.focusedPane == todoPanelPaneGlobal {
		return
	}
	if m.focusedPane == todoPanelPaneWindow {
		m.setFocusedPane(todoPanelPaneAllWindows)
		return
	}
	m.setFocusedPane(todoPanelPaneWindow)
}

func (m *todoPanelModel) defaultAddScope() todoScope {
	if m.focusedPane == todoPanelPaneGlobal {
		return todoScopeGlobal
	}
	return todoScopeWindow
}

func (m *todoPanelModel) paneForScope(scope todoScope) todoPanelPane {
	if scope == todoScopeGlobal {
		return todoPanelPaneGlobal
	}
	return todoPanelPaneWindow
}

func todoPanelPaneLabel(pane todoPanelPane) string {
	switch pane {
	case todoPanelPaneAllWindows:
		return "ALL WINDOWS"
	case todoPanelPaneGlobal:
		return "GLOBAL"
	default:
		return "WINDOW"
	}
}

func todoEntryKey(entry tmuxTodoEntry) string {
	return fmt.Sprintf("%d|%s|%d|%s", entry.Scope, entry.ScopeID, entry.ItemIndex, entry.Title)
}

func (m *todoPanelModel) pruneKeepVisibleDone() {
	if len(m.keepVisibleDone) == 0 {
		return
	}
	present := make(map[string]bool, len(m.entries))
	for _, entry := range m.entries {
		present[todoEntryKey(entry)] = true
	}
	for key := range m.keepVisibleDone {
		if !present[key] {
			delete(m.keepVisibleDone, key)
		}
	}
}

func todoScopeLabel(scope todoScope) string {
	if scope == todoScopeGlobal {
		return "GLOBAL"
	}
	return "WINDOW"
}

func todoPriorityLabel(priority int) string {
	switch normalizeTodoPriority(priority) {
	case 1:
		return "high"
	case 3:
		return "low"
	default:
		return "medium"
	}
}

func renderTodoPriorityChip(priority int) string {
	label := "MED"
	bg := lipgloss.Color("240")
	fg := lipgloss.Color("230")
	switch normalizeTodoPriority(priority) {
	case 1:
		label = "HIGH"
		bg = lipgloss.Color("203")
		fg = lipgloss.Color("235")
	case 3:
		label = "LOW"
		bg = lipgloss.Color("241")
		fg = lipgloss.Color("252")
	}
	return lipgloss.NewStyle().Foreground(fg).Background(bg).Padding(0, 1).Bold(true).Render(label)
}

func (m *todoPanelModel) setStatus(text string, duration time.Duration) {
	m.status = text
	m.statusUntil = time.Now().Add(duration)
}

func (m *todoPanelModel) currentStatus() string {
	if m.status == "" {
		return ""
	}
	if !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
		m.status = ""
		return ""
	}
	return m.status
}

func focusTodoEntry(entry tmuxTodoEntry) error {
	switch entry.Scope {
	case todoScopeGlobal:
		return fmt.Errorf("global todo has no tmux target")
	case todoScopeSession:
		sessionID := strings.TrimSpace(entry.ScopeID)
		if sessionID == "" {
			return fmt.Errorf("session todo has no tmux target")
		}
		return runTmux("switch-client", "-t", sessionID)
	case todoScopeWindow:
		windowID := strings.TrimSpace(entry.ScopeID)
		if windowID == "" {
			return fmt.Errorf("window todo has no tmux target")
		}
		sessionID, _, err := tmuxSessionForWindow(windowID)
		if err == nil && strings.TrimSpace(sessionID) != "" {
			if err := runTmux("switch-client", "-t", strings.TrimSpace(sessionID)); err != nil {
				return err
			}
		}
		return selectTmuxWindow(windowID)
	default:
		return fmt.Errorf("todo has no tmux target")
	}
}

func renderTodoInputValue(text []rune, cursor int, styles todoPanelStyles) string {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(text) {
		cursor = len(text)
	}
	left := string(text[:cursor])
	right := string(text[cursor:])
	cursorChar := " "
	if cursor < len(text) {
		cursorChar = string(text[cursor])
		right = string(text[cursor+1:])
	}
	if len(text) == 0 && cursor == 0 {
		cursorChar = " "
	}
	return left + styles.inputCursor.Render(cursorChar) + right
}

func wrapTodoTitle(text string, width int) []string {
	text = strings.TrimSpace(text)
	if width <= 0 {
		return []string{""}
	}
	if text == "" {
		return []string{""}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := ""
	appendCurrent := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}
	for _, word := range words {
		for lipgloss.Width(word) > width {
			spaceLeft := width
			if current != "" {
				spaceLeft = maxInt(1, width-lipgloss.Width(current)-1)
			}
			chunk := truncate(word, spaceLeft)
			chunk = strings.TrimSuffix(chunk, "…")
			if chunk == "" {
				chunk = string([]rune(word)[:1])
			}
			if current == "" {
				lines = append(lines, chunk)
			} else {
				lines = append(lines, current+" "+chunk)
				current = ""
			}
			word = strings.TrimPrefix(word, chunk)
		}
		if current == "" {
			current = word
			continue
		}
		candidate := current + " " + word
		if lipgloss.Width(candidate) <= width {
			current = candidate
			continue
		}
		appendCurrent()
		current = word
	}
	appendCurrent()
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func writeTodoClipboard(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("nothing to copy")
	}
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(value)
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}
		return fmt.Errorf("clipboard copy failed: %s", message)
	}
	return nil
}
