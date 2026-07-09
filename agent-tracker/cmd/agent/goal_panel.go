package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type goalPanelMode int

const (
	goalModeList goalPanelMode = iota
	goalModeMove
	goalModeRename
	goalModeAddGoal
	goalModeAddThread
	goalModeAddTodo
	goalModeSetGoal
	goalModeSetBlocker
	goalModePromote
	goalModePromoteGoal
	goalModeMoreOptions
	goalModeConfirmDeleteGoal
	goalModeConfirmDone
	goalModeConfirmDeletePlanned
	goalModeConfirmMerge
	goalModeHelp
)

type goalPickerItem struct {
	ID       string
	Label    string
	Subtitle string
	IsCreate bool
}

type goalActionMsg struct {
	err        error
	closeAfter bool
}

type goalPanelModel struct {
	runtime *paletteRuntime
	width   int
	height  int
	list    *flatList
	cursor  int
	expanded map[string]bool

	mode    goalPanelMode
	status  string
	statusUntil time.Time

	// move mode
	moveGhost ghostState
	moveName  string

	// text input modes (rename, add-goal, add-thread, add-todo)
	inputText   []rune
	inputCursor int
	inputLabel  string
	inputHint   string
	inputTarget string // thread id for rename / goal id parent for add-goal / window for add-todo

	// picker modes (set-goal, set-blocker, more-options)
	picker         []goalPickerItem
	pickerFilter   []rune
	pickerCursor   int
	pickerOffset   int
	pickerKind     string
	pickerSelected string // thread id the picker acts on

	// promote form
	promoteWindow  string
	promoteName    []rune
	promoteCursor  int
	promoteGoal    string
	promoteBind    string // existing thread id, or ""
	promoteTab     int    // 0=create, 1=bind
	promoteNoGoal       bool   // promote-goal mode: cursor on "no goal"
	promoteCreateGoal   bool   // creating new goal from promote mode

	// confirm modes
	confirmThread string
	confirmGoal   string
	confirmMergeSrc string
	confirmMergeTgt string

	showAssignedOnly bool
	helpVisible      bool
	showAltHints     bool
	requestBack      bool
	requestClose     bool
}

func newGoalPanelModel(runtime *paletteRuntime) *goalPanelModel {
	m := &goalPanelModel{
		runtime:   runtime,
		expanded:  map[string]bool{},
		mode:      goalModeList,
		showAssignedOnly: true,
	}
	m.reload()
	return m
}

type goalPanelTickMsg struct{}

func goalPanelTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return goalPanelTickMsg{} })
}

func (m *goalPanelModel) activate() tea.Cmd {
	m.requestBack = false
	m.requestClose = false
	reapOrphanedThreads()
	m.reload()
	m.jumpToCurrentWindow()
	return goalPanelTickCmd()
}

func (m *goalPanelModel) jumpToCurrentWindow() {
	wid := strings.TrimSpace(m.currentWindowID())
	if wid == "" || m.list == nil {
		return
	}
	for i, n := range m.list.Nodes {
		if n.Kind == nodeThread && n.Thread != nil && strings.TrimSpace(n.Thread.WindowID) == wid {
			m.cursor = i
			return
		}
		if n.Kind == nodeDraft && n.Draft != nil && strings.TrimSpace(n.Draft.WindowID) == wid {
			m.cursor = i
			return
		}
	}
}

func (m *goalPanelModel) Init() tea.Cmd { return nil }

func (m *goalPanelModel) reload() {
	list, err := buildDisplayList(m.expanded)
	if err != nil {
		m.list = &flatList{}
		m.status = err.Error()
		m.statusUntil = time.Now().Add(3 * time.Second)
		return
	}
	m.list = list
	if m.cursor >= len(list.Nodes) {
		m.cursor = len(list.Nodes) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *goalPanelModel) setStatus(text string) {
	m.status = strings.TrimSpace(text)
	m.statusUntil = time.Now().Add(2500 * time.Millisecond)
}

func (m *goalPanelModel) currentStatus() string {
	if m.status == "" || (!m.statusUntil.IsZero() && time.Now().After(m.statusUntil)) {
		return ""
	}
	return m.status
}

// ── current window context ──────────────────────────────────

func (m *goalPanelModel) currentWindowID() string {
	if m.runtime != nil && strings.TrimSpace(m.runtime.windowID) != "" {
		return strings.TrimSpace(m.runtime.windowID)
	}
	return resolveGoalWindowID("")
}

func (m *goalPanelModel) currentNode() *goalNode {
	if m.list == nil || m.cursor < 0 || m.cursor >= len(m.list.Nodes) {
		return nil
	}
	return m.list.Nodes[m.cursor]
}

// threadForNode returns the thread a node refers to (or a draft's implicit thread nil).
func (m *goalPanelModel) threadForNode(n *goalNode) *Thread {
	if n == nil {
		return nil
	}
	if n.Kind == nodeThread {
		return n.Thread
	}
	return nil
}

// ── Update ──────────────────────────────────────────────────

func (m *goalPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case goalActionMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error())
		}
		if msg.closeAfter {
			m.requestClose = true
		}
		return m, nil
	case goalPanelTickMsg:
		return m, goalPanelTickCmd()
	case tea.KeyMsg:
		if isAltFooterToggleKey(msg) {
			m.showAltHints = !m.showAltHints
			return m, nil
		}
		m.showAltHints = false
		key := msg.String()
		if key == "alt+s" {
			m.requestClose = true
			return m, nil
		}
		switch m.mode {
		case goalModeMove:
			return m.updateMove(key)
		case goalModeRename, goalModeAddGoal, goalModeAddThread, goalModeAddTodo:
			return m.updateTextInput(key)
	case goalModeSetGoal, goalModeSetBlocker, goalModeMoreOptions:
		return m.updatePicker(key)
	case goalModePromoteGoal:
		return m.updatePromoteGoal(key)
		case goalModePromote:
			return m.updatePromote(key)
		case goalModeConfirmDeleteGoal:
			return m.updateConfirmDeleteGoal(key)
		case goalModeConfirmDone:
			return m.updateConfirmDone(key)
		case goalModeConfirmDeletePlanned:
			return m.updateConfirmDeletePlanned(key)
		case goalModeConfirmMerge:
			return m.updateConfirmMerge(key)
		case goalModeHelp:
			if key == "?" || key == "esc" {
				m.mode = goalModeList
			}
			return m, nil
		default:
			return m.updateList(key)
		}
	}
	return m, nil
}

// ── list mode ───────────────────────────────────────────────

func (m *goalPanelModel) updateList(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "ctrl+c":
		m.requestBack = true
		return m, nil
	case "?":
		m.mode = goalModeHelp
		return m, nil
	case "u", "up":
		m.moveCursor(-1)
	case "e", "down":
		m.moveCursor(1)
	case "alt+u":
		return m.moveSibling(-1)
	case "alt+e":
		return m.moveSibling(1)
	case "n":
		m.jumpToParent()
	case "right", ">", "enter":
		return m.handleEnterOrExpand(key)
	case "left", "<":
		m.jumpToParent()
	case "l":
		m.showAssignedOnly = !m.showAssignedOnly
		m.setStatus(toggleViewLabel(m.showAssignedOnly))
	case "r":
		return m.beginRename()
	case "R":
		return m.beginRenameFromCurrent()
	case "g":
		return m.beginSetGoal()
	case "b":
		return m.beginSetBlocker()
	case "m":
		return m.beginMove()
	case "p":
		return m.beginPromote()
	case "t":
		return m.beginAddThread()
	case "T":
		return m.beginAddThreadTopLevel()
	case "o":
		return m.beginMoreOptions()
	case "d", "x":
		return m.handleDeleteOrRemoveTodo()
	case "D":
		return m.handleContextualD()
	case "a":
		return m.beginAddContextual()
	case "A":
		return m.beginAddGoalTopLevel()
	case "c":
		return m.toggleTodoOrExpand()
	case "y":
		return m.copyNodeTitle()
	case " ":
		return m.toggleTodoOrExpand()
	}
	return m, nil
}

func toggleViewLabel(assigned bool) string {
	if assigned {
		return "showing assigned + unassigned"
	}
	return "showing unassigned only"
}

func (m *goalPanelModel) moveCursor(delta int) {
	if m.list == nil || len(m.list.Nodes) == 0 {
		return
	}
	m.cursor = clampInt(m.cursor+delta, 0, len(m.list.Nodes)-1)
}

func (m *goalPanelModel) jumpToParent() {
	n := m.currentNode()
	if n == nil || n.Depth <= 0 {
		return
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if m.list.Nodes[i].Depth < n.Depth && m.list.Nodes[i].Kind == nodeGoal {
			m.cursor = i
			return
		}
	}
}

func (m *goalPanelModel) toggleExpand() {
	n := m.currentNode()
	if n == nil {
		return
	}
	key := expandKeyForNode(n)
	if key == "" {
		return
	}
	if n.Kind == nodeGoal {
		_, ok := m.expanded[key]
		if ok {
			delete(m.expanded, key)
		} else {
			m.expanded[key] = false
		}
	} else {
		m.expanded[key] = !m.expanded[key]
	}
	m.reload()
}

func expandKeyForNode(n *goalNode) string {
	if n == nil {
		return ""
	}
	switch n.Kind {
	case nodeGoal:
		return n.Goal.ID
	case nodeThread:
		return n.Thread.ID
	case nodeDraft:
		return "draft:" + n.Draft.WindowID
	}
	return ""
}

func (m *goalPanelModel) handleEnterOrExpand(key string) (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	if key == "enter" {
		return m.gotoOrBind()
	}
	// right / > expands threads/drafts
	if n.Kind == nodeThread || n.Kind == nodeDraft {
		m.toggleExpand()
	}
	return m, nil
}

func (m *goalPanelModel) gotoOrBind() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	switch n.Kind {
	case nodeThread:
		if strings.TrimSpace(n.Thread.WindowID) == "" {
			// planned thread -> bind to current window
			wid := m.currentWindowID()
			if wid == "" {
				m.setStatus("No current window to bind")
				return m, nil
			}
			if err := bindThreadWindow(n.Thread.ID, wid); err != nil {
				m.setStatus(err.Error())
				return m, nil
			}
			m.setStatus("Bound to window")
			m.reload()
			return m, nil
		}
		return m, m.runFunc(func() error { return focusGoalNode(n) }, true)
	case nodeDraft:
		return m, m.runFunc(func() error { return focusWindow(n.Draft.WindowID) }, true)
	case nodeGoal:
		m.toggleExpand()
		return m, nil
	case nodeTodo:
		return m, m.runFunc(func() error { return focusWindow(n.TodoWindow) }, true)
	}
	return m, nil
}

func focusGoalNode(n *goalNode) error {
	if n == nil || n.Kind != nodeThread {
		return nil
	}
	if strings.TrimSpace(n.Thread.WindowID) == "" {
		return nil
	}
	return focusWindow(n.Thread.WindowID)
}

func focusWindow(windowID string) error {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return nil
	}
	sessionID, _, err := tmuxSessionForWindow(windowID)
	if err == nil && strings.TrimSpace(sessionID) != "" {
		_ = runTmux("switch-client", "-t", strings.TrimSpace(sessionID))
	}
	return selectTmuxWindow(windowID)
}

// ── rename ──────────────────────────────────────────────────

func (m *goalPanelModel) beginRename() (tea.Model, tea.Cmd) {
	return m.beginRenameImpl(false)
}

func (m *goalPanelModel) beginRenameFromCurrent() (tea.Model, tea.Cmd) {
	return m.beginRenameImpl(true)
}

func (m *goalPanelModel) beginRenameImpl(prefill bool) (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	switch n.Kind {
	case nodeGoal:
		m.inputLabel = "Rename goal"
		m.inputTarget = "goal:" + n.Goal.ID
		if prefill {
			m.inputText = []rune(n.Goal.Title)
		} else {
			m.inputText = nil
		}
	case nodeThread:
		m.inputLabel = "Rename thread"
		m.inputTarget = "thread:" + n.Thread.ID
		if prefill {
			m.inputText = []rune(n.Thread.Name)
		} else {
			m.inputText = nil
		}
	case nodeDraft:
		m.inputLabel = "Rename thread"
		m.inputTarget = "draft:" + n.Draft.WindowID
		if prefill {
			m.inputText = []rune(draftDisplayName(n.Draft))
		} else {
			m.inputText = nil
		}
	default:
		return m, nil
	}
	m.inputCursor = len(m.inputText)
	m.inputHint = "Enter to save · Esc to cancel"
	m.mode = goalModeRename
	return m, nil
}

func (m *goalPanelModel) submitRename() {
	text := strings.TrimSpace(string(m.inputText))
	target := m.inputTarget
	m.mode = goalModeList
	if text == "" {
		m.setStatus("Name required")
		return
	}
	if strings.HasPrefix(target, "goal:") {
		if err := renameGoal(strings.TrimPrefix(target, "goal:"), text); err != nil {
			m.setStatus(err.Error())
			return
		}
		m.setStatus("Renamed")
	} else if strings.HasPrefix(target, "thread:") {
		if err := renameThread(strings.TrimPrefix(target, "thread:"), text); err != nil {
			m.setStatus(err.Error())
			return
		}
		m.setStatus("Renamed")
	} else if strings.HasPrefix(target, "draft:") {
		wid := strings.TrimPrefix(target, "draft:")
		if _, err := promoteDraftToThread(wid, text, "", ""); err != nil {
			m.setStatus(err.Error())
			return
		}
		m.setStatus("Renamed")
	}
	m.reload()
}

// ── add goal / thread / todo ────────────────────────────────

func (m *goalPanelModel) currentGoalID() string {
	n := m.currentNode()
	if n == nil {
		return ""
	}
	if n.Kind == nodeGoal {
		return n.Goal.ID
	}
	for i := m.cursor - 1; i >= 0; i-- {
		node := m.list.Nodes[i]
		if node.Kind == nodeGoal && node.Depth < n.Depth {
			return node.Goal.ID
		}
	}
	return ""
}

func (m *goalPanelModel) beginAddGoal() (tea.Model, tea.Cmd) {
	goalID := m.currentGoalID()
	m.inputLabel = "New sub-goal"
	if goalID == "" {
		m.inputLabel = "New goal"
	}
	m.inputTarget = goalID
	m.inputText = nil
	m.inputCursor = 0
	m.inputHint = "Enter to create · Esc to cancel"
	m.mode = goalModeAddGoal
	return m, nil
}

func (m *goalPanelModel) beginAddGoalTopLevel() (tea.Model, tea.Cmd) {
	m.inputLabel = "New goal"
	m.inputTarget = ""
	m.inputText = nil
	m.inputCursor = 0
	m.inputHint = "Enter to create · Esc to cancel"
	m.mode = goalModeAddGoal
	return m, nil
}

func (m *goalPanelModel) beginAddContextual() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m.beginAddGoalTopLevel()
	}
	switch n.Kind {
	case nodeGoal:
		return m.beginAddGoal()
	case nodeThread, nodeDraft, nodeTodo:
		return m.beginAddTodo()
	default:
		return m.beginAddGoalTopLevel()
	}
}

func (m *goalPanelModel) submitAddGoal() {
	title := strings.TrimSpace(string(m.inputText))
	if m.promoteCreateGoal {
		m.promoteCreateGoal = false
		if title == "" {
			m.mode = goalModePromoteGoal
			return
		}
		goal, err := addGoal(title, m.inputTarget)
		if err != nil {
			m.setStatus(err.Error())
			m.mode = goalModePromoteGoal
			return
		}
		m.setStatus("Goal created")
		m.reload()
		m.mode = goalModePromoteGoal
		m.promoteNoGoal = false
		if goal != nil {
			if idx := m.list.indexOfGoal(goal.ID); idx >= 0 {
				m.cursor = idx
			}
		}
		return
	}
	m.mode = goalModeList
	if title == "" {
		m.setStatus("Title required")
		return
	}
	if _, err := addGoal(title, m.inputTarget); err != nil {
		m.setStatus(err.Error())
		return
	}
	m.setStatus("Goal created")
	m.reload()
}

func (m *goalPanelModel) beginAddThread() (tea.Model, tea.Cmd) {
	goalID := m.currentGoalID()
	m.inputLabel = "New thread"
	m.inputTarget = goalID
	m.inputText = nil
	m.inputCursor = 0
	m.inputHint = "Enter to create · Esc to cancel"
	m.mode = goalModeAddThread
	return m, nil
}

func (m *goalPanelModel) beginAddThreadTopLevel() (tea.Model, tea.Cmd) {
	m.inputLabel = "New thread"
	m.inputTarget = ""
	m.inputText = nil
	m.inputCursor = 0
	m.inputHint = "Enter to create · Esc to cancel"
	m.mode = goalModeAddThread
	return m, nil
}

func (m *goalPanelModel) submitAddThread() {
	name := strings.TrimSpace(string(m.inputText))
	m.mode = goalModeList
	if name == "" {
		m.setStatus("Name required")
		return
	}
	if _, err := addManualThread(name, m.inputTarget, nil); err != nil {
		m.setStatus(err.Error())
		return
	}
	m.setStatus("Thread created")
	m.reload()
}

func (m *goalPanelModel) beginAddTodo() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	windowID := ""
	switch n.Kind {
	case nodeThread:
		windowID = n.Thread.WindowID
	case nodeDraft:
		windowID = n.Draft.WindowID
	case nodeTodo:
		windowID = n.TodoWindow
	default:
		m.setStatus("Select a thread first")
		return m, nil
	}
	if strings.TrimSpace(windowID) == "" {
		m.setStatus("No window bound for todos")
		return m, nil
	}
	m.inputTarget = windowID
	m.inputLabel = "New todo"
	m.inputText = nil
	m.inputCursor = 0
	m.inputHint = "Enter to add · Esc to cancel"
	m.mode = goalModeAddTodo
	return m, nil
}

func (m *goalPanelModel) submitAddTodo() {
	title := strings.TrimSpace(string(m.inputText))
	m.mode = goalModeList
	if title == "" {
		m.setStatus("Todo title required")
		return
	}
	if err := addTmuxTodo(todoScopeWindow, m.inputTarget, title); err != nil {
		m.setStatus(err.Error())
		return
	}
	m.setStatus("Todo added")
	m.reload()
}

// ── text input shared ───────────────────────────────────────

func (m *goalPanelModel) updateTextInput(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		if m.promoteCreateGoal {
			m.promoteCreateGoal = false
			m.mode = goalModePromoteGoal
		} else {
			m.mode = goalModeList
		}
		return m, nil
	}
	if key == "ctrl+c" {
		m.inputText = nil
		m.inputCursor = 0
		return m, nil
	}
	if key == "enter" && m.mode != goalModeAddTodo {
		switch m.mode {
		case goalModeRename:
			m.submitRename()
		case goalModeAddGoal:
			m.submitAddGoal()
		case goalModeAddThread:
			m.submitAddThread()
		}
		return m, nil
	}
	if key == "enter" && m.mode == goalModeAddTodo {
		m.submitAddTodo()
		return m, nil
	}
	applyPaletteInputKey(key, &m.inputText, &m.inputCursor, false)
	return m, nil
}

// ── pickers (set-goal, set-blocker, more-options) ───────────

func (m *goalPanelModel) beginSetGoal() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	tid := threadIDForNode(n)
	if tid == "" {
		m.setStatus("Select a thread first")
		return m, nil
	}
	store, err := loadGoalStore()
	if err != nil {
		m.setStatus(err.Error())
		return m, nil
	}
	items := []goalPickerItem{{ID: "", Label: "— unassign —", Subtitle: "no goal"}}
	items = appendGoalPickerItems(items, store, "", 0)
	m.picker = items
	m.pickerFilter = nil
	m.pickerCursor = 0
	m.pickerOffset = 0
	m.pickerKind = "goal"
	m.pickerSelected = tid
	m.inputCursor = 0
	m.mode = goalModeSetGoal
	return m, nil
}

func appendGoalPickerItems(items []goalPickerItem, store *GoalStore, parentID string, depth int) []goalPickerItem {
	var children []*Goal
	for _, g := range store.Goals {
		if strings.TrimSpace(g.ParentID) == strings.TrimSpace(parentID) {
			children = append(children, g)
		}
	}
	sortGoalsByOrder(children)
	for _, g := range children {
		indent := strings.Repeat("  ", depth)
		items = append(items, goalPickerItem{ID: g.ID, Label: indent + g.Title, Subtitle: g.ID})
		items = appendGoalPickerItems(items, store, g.ID, depth+1)
	}
	return items
}

func sortGoalsByOrder(goals []*Goal) {
	for i := 1; i < len(goals); i++ {
		for j := i; j > 0 && goals[j-1].Order > goals[j].Order; j-- {
			goals[j-1], goals[j] = goals[j], goals[j-1]
		}
	}
}

func (m *goalPanelModel) submitSetGoal() {
	items := m.filteredPickerItems()
	if len(items) == 0 || m.pickerCursor >= len(items) {
		m.mode = goalModeList
		return
	}
	pick := items[m.pickerCursor]
	m.mode = goalModeList
	if err := setThreadGoal(m.pickerSelected, pick.ID); err != nil {
		m.setStatus(err.Error())
		return
	}
	m.setStatus("Goal set")
	m.reload()
}

func (m *goalPanelModel) beginSetBlocker() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	tid := threadIDForNode(n)
	if tid == "" {
		m.setStatus("Select a thread first")
		return m, nil
	}
	store, err := loadGoalStore()
	if err != nil {
		m.setStatus(err.Error())
		return m, nil
	}
	current := store.Threads[tid]
	items := []goalPickerItem{{ID: "", Label: "— clear blockers —", Subtitle: "remove all"}}
	ids := make([]string, 0, len(store.Threads))
	for id := range store.Threads {
		if id != tid {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		t := store.Threads[id]
		if t == nil {
			continue
		}
		items = append(items, goalPickerItem{ID: t.ID, Label: threadDisplayName(t, ""), Subtitle: t.ID})
	}
	m.picker = items
	m.pickerFilter = nil
	m.pickerCursor = 0
	m.pickerOffset = 0
	m.pickerKind = "blocker"
	m.pickerSelected = tid
	m.inputCursor = 0
	m.mode = goalModeSetBlocker
	_ = current
	return m, nil
}

func (m *goalPanelModel) submitSetBlocker() {
	items := m.filteredPickerItems()
	if len(items) == 0 || m.pickerCursor >= len(items) {
		m.mode = goalModeList
		return
	}
	pick := items[m.pickerCursor]
	m.mode = goalModeList
	if pick.ID == "" {
		if err := setThreadBlocker(m.pickerSelected, nil); err != nil {
			m.setStatus(err.Error())
			return
		}
		m.setStatus("Blockers cleared")
	} else {
		if err := setThreadBlocker(m.pickerSelected, []string{pick.ID}); err != nil {
			m.setStatus(err.Error())
			return
		}
		m.setStatus("Blocker set")
	}
	m.reload()
}

func (m *goalPanelModel) beginMoreOptions() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	m.picker = moreOptionsForNode(n)
	m.pickerFilter = nil
	m.pickerCursor = 0
	m.pickerOffset = 0
	m.pickerKind = "action"
	m.pickerSelected = ""
	m.inputCursor = 0
	m.mode = goalModeMoreOptions
	return m, nil
}

func moreOptionsForNode(n *goalNode) []goalPickerItem {
	base := []goalPickerItem{
		{ID: "add-goal", Label: "Add goal"},
		{ID: "add-thread", Label: "Add thread"},
	}
	if n == nil {
		return base
	}
	switch n.Kind {
	case nodeGoal:
		base = append(base,
			goalPickerItem{ID: "add-goal-child", Label: "Add sub-goal"},
			goalPickerItem{ID: "rename", Label: "Rename goal"},
			goalPickerItem{ID: "delete", Label: "Delete goal (cascade)"},
		)
	case nodeThread:
		base = append(base,
			goalPickerItem{ID: "rename", Label: "Rename thread"},
			goalPickerItem{ID: "set-goal", Label: "Set goal"},
			goalPickerItem{ID: "blocker", Label: "Set blocker"},
			goalPickerItem{ID: "done", Label: "Toggle done"},
		)
		if strings.TrimSpace(n.Thread.WindowID) == "" {
			base = append(base, goalPickerItem{ID: "bind-current", Label: "Bind to current window"})
		} else {
			base = append(base, goalPickerItem{ID: "unbind", Label: "Unbind window"})
			base = append(base, goalPickerItem{ID: "goto", Label: "Go to window"})
		}
		base = append(base, goalPickerItem{ID: "merge", Label: "Merge into another thread"})
		base = append(base, goalPickerItem{ID: "delete", Label: "Delete thread (window closes otherwise)"})
	case nodeDraft:
		base = append(base,
			goalPickerItem{ID: "rename", Label: "Name / promote"},
			goalPickerItem{ID: "set-goal", Label: "Set goal"},
			goalPickerItem{ID: "blocker", Label: "Set blocker"},
			goalPickerItem{ID: "goto", Label: "Go to window"},
		)
	case nodeTodo:
		base = append(base,
			goalPickerItem{ID: "toggle-todo", Label: "Toggle todo"},
			goalPickerItem{ID: "delete-todo", Label: "Delete todo"},
		)
	}
	return base
}

func (m *goalPanelModel) submitMoreOptions() {
	items := m.filteredPickerItems()
	if len(items) == 0 || m.pickerCursor >= len(items) {
		m.mode = goalModeList
		return
	}
	action := items[m.pickerCursor].ID
	m.mode = goalModeList
	m.applyMoreOption(action)
}

func (m *goalPanelModel) applyMoreOption(action string) {
	n := m.currentNode()
	switch action {
	case "add-goal":
		m.beginAddGoal()
	case "add-goal-child":
		if n != nil && n.Kind == nodeGoal {
			m.inputTarget = n.Goal.ID
			m.inputLabel = "New sub-goal"
			m.inputText = nil
			m.inputCursor = 0
			m.inputHint = "Enter to create · Esc to cancel"
			m.mode = goalModeAddGoal
		}
	case "add-thread":
		m.beginAddThread()
	case "rename":
		m.beginRename()
	case "set-goal":
		m.beginSetGoal()
	case "blocker":
		m.beginSetBlocker()
	case "done":
		if n != nil && n.Kind == nodeThread {
			m.confirmThread = n.Thread.ID
			m.mode = goalModeConfirmDone
		}
	case "delete":
		m.handleContextualD()
	case "bind-current":
		if n != nil && n.Kind == nodeThread {
			wid := m.currentWindowID()
			if wid != "" {
				if err := bindThreadWindow(n.Thread.ID, wid); err != nil {
					m.setStatus(err.Error())
				} else {
					m.setStatus("Bound")
					m.reload()
				}
			}
		}
	case "unbind":
		if n != nil && n.Kind == nodeThread {
			if err := unbindThreadWindow(n.Thread.ID); err != nil {
				m.setStatus(err.Error())
			} else {
				m.setStatus("unbound")
				m.reload()
			}
		}
	case "goto":
		m.gotoOrBind()
	case "merge":
		m.beginSetBlocker() // reuse picker skeleton; merge handled at submit
		_ = n
	case "toggle-todo":
		if n != nil && n.Kind == nodeTodo {
			_ = toggleTmuxTodoByIndex(todoScopeWindow, n.TodoWindow, n.TodoIndex)
			m.reload()
		}
	case "delete-todo":
		if n != nil && n.Kind == nodeTodo {
			_ = deleteTmuxTodoByIndex(todoScopeWindow, n.TodoWindow, n.TodoIndex)
			m.reload()
		}
	}
}

func (m *goalPanelModel) updatePicker(key string) (tea.Model, tea.Cmd) {
	items := m.filteredPickerItems()
	switch key {
	case "esc":
		m.mode = goalModeList
		return m, nil
	case "u", "up":
		if len(items) > 0 {
			m.pickerCursor = clampInt(m.pickerCursor-1, 0, len(items)-1)
		}
	case "e", "down":
		if len(items) > 0 {
			m.pickerCursor = clampInt(m.pickerCursor+1, 0, len(items)-1)
		}
	case "enter":
		switch m.mode {
		case goalModeSetGoal:
			m.submitSetGoal()
		case goalModeSetBlocker:
			m.submitSetBlocker()
		case goalModeMoreOptions:
			m.submitMoreOptions()
		case goalModePromoteGoal:
			m.submitPromoteGoal()
		}
		return m, nil
	default:
		applyPaletteInputKey(key, &m.pickerFilter, &m.inputCursor, false)
		m.pickerCursor = clampInt(m.pickerCursor, 0, maxInt(0, len(m.filteredPickerItems())-1))
	}
	return m, nil
}

func (m *goalPanelModel) filteredPickerItems() []goalPickerItem {
	needle := strings.ToLower(strings.TrimSpace(string(m.pickerFilter)))
	if needle == "" {
		return m.picker
	}
	var out []goalPickerItem
	for _, it := range m.picker {
		if strings.Contains(strings.ToLower(it.Label), needle) || strings.Contains(strings.ToLower(it.Subtitle), needle) {
			out = append(out, it)
		}
	}
	return out
}

// ── promote form ────────────────────────────────────────────

func (m *goalPanelModel) beginPromote() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil || n.Kind != nodeDraft {
		m.setStatus("Select a draft to promote")
		return m, nil
	}
	m.promoteWindow = n.Draft.WindowID
	m.promoteName = []rune(draftDisplayName(n.Draft))
	m.promoteCursor = m.cursor
	m.mode = goalModePromoteGoal
	m.cursor = 0
	m.promoteNoGoal = false
	return m, nil
}

func (m *goalPanelModel) promoteGoalIndices() []int {
	var indices []int
	if m.list == nil {
		return indices
	}
	for i, n := range m.list.Nodes {
		if n.Kind == nodeGoal {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m *goalPanelModel) promoteGoalCursorIndex(numGoals int) int {
	indices := m.promoteGoalIndices()
	for i, idx := range indices {
		if idx == m.cursor {
			return i
		}
	}
	return 0
}

func (m *goalPanelModel) beginAddGoalFromPromote() (tea.Model, tea.Cmd) {
	parentID := ""
	if !m.promoteNoGoal {
		n := m.currentNode()
		if n != nil && n.Kind == nodeGoal {
			parentID = n.Goal.ID
		}
	}
	m.inputLabel = "New sub-goal"
	if parentID == "" {
		m.inputLabel = "New goal"
	}
	m.inputTarget = parentID
	m.inputText = nil
	m.inputCursor = 0
	m.inputHint = "Enter to create · Esc to cancel"
	m.mode = goalModeAddGoal
	m.promoteCreateGoal = true
	return m, nil
}

func (m *goalPanelModel) updatePromoteGoal(key string) (tea.Model, tea.Cmd) {
	indices := m.promoteGoalIndices()
	numGoals := len(indices)
	switch key {
	case "esc":
		m.mode = goalModeList
		m.cursor = m.promoteCursor
		return m, nil
	case "u", "up":
		if m.promoteNoGoal {
			if numGoals > 0 {
				m.cursor = indices[numGoals-1]
				m.promoteNoGoal = false
			}
		} else {
			cur := m.promoteGoalCursorIndex(numGoals)
			if cur > 0 {
				m.cursor = indices[cur-1]
			}
		}
	case "e", "down":
		if m.promoteNoGoal {
			// stay at no goal
		} else {
			cur := m.promoteGoalCursorIndex(numGoals)
			if cur < numGoals-1 {
				m.cursor = indices[cur+1]
			} else {
				m.promoteNoGoal = true
			}
		}
	case "enter", "m":
		m.submitPromoteGoal()
	case "a":
		return m.beginAddGoalFromPromote()
	}
	return m, nil
}

func (m *goalPanelModel) submitPromoteGoal() {
	name := strings.TrimSpace(string(m.promoteName))
	m.mode = goalModeList
	var goalID string
	if !m.promoteNoGoal {
		n := m.currentNode()
		if n != nil && n.Kind == nodeGoal {
			goalID = n.Goal.ID
		}
	}
	if _, err := promoteDraftToThread(m.promoteWindow, name, goalID, ""); err != nil {
		m.setStatus(err.Error())
		return
	}
	m.setStatus("Promoted")
	m.reload()
}

func (m *goalPanelModel) updatePromote(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.mode = goalModeList
		return m, nil
	case "tab":
		m.promoteTab = 1 - m.promoteTab
		return m, nil
	case "enter":
		m.submitPromote()
		return m, nil
	}
	applyPaletteInputKey(key, &m.promoteName, &m.promoteCursor, false)
	return m, nil
}

func (m *goalPanelModel) submitPromote() {
	name := strings.TrimSpace(string(m.promoteName))
	m.mode = goalModeList
	if m.promoteTab == 1 && m.promoteBind != "" {
		if _, err := promoteDraftToThread(m.promoteWindow, name, "", m.promoteBind); err != nil {
			m.setStatus(err.Error())
			return
		}
	} else {
		if _, err := promoteDraftToThread(m.promoteWindow, name, m.promoteGoal, ""); err != nil {
			m.setStatus(err.Error())
			return
		}
	}
	m.setStatus("Promoted")
	m.reload()
}

// ── move mode ───────────────────────────────────────────────

// moveSibling reorders the selected node among its siblings (same parent,
// same depth) by one slot, without changing parent/depth. No-op at the
// first/last sibling boundary and for drafts (ordered by recency).
func (m *goalPanelModel) moveSibling(delta int) (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	var parentID, nodeID string
	var isGoal bool
	switch n.Kind {
	case nodeGoal:
		parentID = strings.TrimSpace(n.Goal.ParentID)
		nodeID = n.Goal.ID
		isGoal = true
	case nodeThread:
		parentID = strings.TrimSpace(n.Thread.GoalID)
		nodeID = n.Thread.ID
		isGoal = false
	default:
		return m, nil
	}
	if !reorderChildSibling(parentID, nodeID, isGoal, delta) {
		return m, nil
	}
	m.reload()
	if isGoal {
		if idx := m.list.indexOfGoal(nodeID); idx >= 0 {
			m.cursor = idx
		}
	} else {
		if idx := m.list.indexOfThread(nodeID); idx >= 0 {
			m.cursor = idx
		}
	}
	return m, nil
}

func (m *goalPanelModel) beginMove() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	if n.Kind == nodeGoal {
		idx := m.list.indexOfGoal(n.Goal.ID)
		if idx < 0 {
			return m, nil
		}
		m.moveGhost = ghostState{GoalID: n.Goal.ID, IsGoal: true, Pos: idx, Depth: n.Depth}
		m.moveName = n.Goal.Title
		m.mode = goalModeMove
		return m, nil
	}
	if n.Kind == nodeThread {
		idx := m.list.indexOfThread(n.Thread.ID)
		if idx < 0 {
			return m, nil
		}
		m.moveGhost = ghostState{ThreadID: n.Thread.ID, Pos: idx, Depth: n.Depth}
		m.moveName = threadDisplayName(n.Thread, "")
		m.mode = goalModeMove
		return m, nil
	}
	if n.Kind == nodeDraft {
		idx := -1
		for i, node := range m.list.Nodes {
			if node.Kind == nodeDraft && node.Draft != nil && node.Draft.WindowID == n.Draft.WindowID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return m, nil
		}
		m.moveGhost = ghostState{ThreadID: "draft:" + n.Draft.WindowID, Pos: idx, Depth: n.Depth, IsDraft: true}
		m.moveName = draftDisplayName(n.Draft)
		m.mode = goalModeMove
		return m, nil
	}
	return m, nil
}

func (m *goalPanelModel) updateMove(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.mode = goalModeList
		return m, nil
	case "enter", "m":
		return m.commitMove()
	case "u", "up":
		m.moveGhost = m.clampGoalGhost(m.moveGhostUp(), -1)
	case "e", "down":
		m.moveGhost = m.clampGoalGhost(m.moveGhostDown(), +1)
	case "n":
		if m.moveGhost.Depth > 0 {
			m.moveGhost = ghostState{ThreadID: m.moveGhost.ThreadID, GoalID: m.moveGhost.GoalID, IsGoal: m.moveGhost.IsGoal, Pos: m.moveGhost.Pos, Depth: m.moveGhost.Depth - 1, IsDraft: m.moveGhost.IsDraft}
		}
	}
	return m, nil
}

func (m *goalPanelModel) clampGoalGhost(g ghostState, dir int) ghostState {
	if !m.moveGhost.IsGoal {
		return g
	}
	if g.Pos >= m.separatorIndex() {
		return m.moveGhost
	}
	if g.Pos == m.moveGhost.Pos && g.Depth == m.moveGhost.Depth && g.After == m.moveGhost.After {
		return g
	}
	movedIdx := m.ghostMovedIdx()
	if movedIdx < 0 {
		return g
	}
	subtreeEnd := m.ghostSubtreeEnd(movedIdx)
	if g.Pos >= movedIdx && g.Pos < subtreeEnd {
		g.After = false
		g.Before = false
		if dir < 0 && movedIdx > 0 {
			g.Pos = movedIdx - 1
		} else {
			g.Pos = subtreeEnd
			if g.Pos >= len(m.list.Nodes) {
				g.Pos = len(m.list.Nodes) - 1
				if g.Depth > 0 {
					g.Depth--
				}
			}
			if g.Pos >= m.separatorIndex() {
				g.Pos = movedIdx
				g.Depth = 0
			}
		}
	}
	return g
}

func (m *goalPanelModel) separatorIndex() int {
	for i, n := range m.list.Nodes {
		if n.Kind == nodeDraft {
			return i
		}
	}
	return len(m.list.Nodes)
}

func (m *goalPanelModel) moveGhostUp() ghostState {
	sep := m.separatorIndex()
	g := m.moveGhost
	if g.IsDraft && g.Pos >= sep {
		return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, Pos: sep - 1, Depth: 0, IsDraft: true, After: true}
	}
	wasAfter := g.After
	if g.After {
		node := m.list.Nodes[g.Pos]
		if node.Depth == g.Depth && node.Kind != nodeGoal {
			return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, Pos: g.Pos, Depth: g.Depth, IsDraft: g.IsDraft, Before: true}
		}
		g.Pos = g.Pos + 1
		g.After = false
	}
	movedIdx := m.ghostMovedIdx()
	if movedIdx >= 0 && g.Pos == movedIdx && g.Depth < m.list.Nodes[movedIdx].Depth {
		return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, Pos: g.Pos, Depth: g.Depth + 1, IsDraft: g.IsDraft}
	}
	if !wasAfter && g.Pos >= 0 && g.Pos < len(m.list.Nodes) {
		node := m.list.Nodes[g.Pos]
		if node.Kind == nodeGoal && g.Depth == node.Depth+1 {
			return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, Pos: g.Pos, Depth: node.Depth, IsDraft: g.IsDraft}
		}
	}
	return moveGhostUp(m.list, g)
}

func (m *goalPanelModel) ghostMovedIdx() int {
	if m.moveGhost.IsGoal {
		for i, n := range m.list.Nodes {
			if n.Kind == nodeGoal && n.Goal != nil && n.Goal.ID == m.moveGhost.GoalID {
				return i
			}
		}
		return -1
	}
	if m.moveGhost.IsDraft {
		wid := strings.TrimPrefix(m.moveGhost.ThreadID, "draft:")
		for i, n := range m.list.Nodes {
			if n.Kind == nodeDraft && n.Draft != nil && n.Draft.WindowID == wid {
				return i
			}
		}
	} else {
		for i, n := range m.list.Nodes {
			if n.Kind == nodeThread && n.Thread != nil && n.Thread.ID == m.moveGhost.ThreadID {
				return i
			}
		}
	}
	return -1
}

func (m *goalPanelModel) ghostSubtreeEnd(startIdx int) int {
	if startIdx < 0 || startIdx >= len(m.list.Nodes) {
		return startIdx + 1
	}
	depth := m.list.Nodes[startIdx].Depth
	end := startIdx + 1
	for end < len(m.list.Nodes) && m.list.Nodes[end].Depth > depth {
		end++
	}
	return end
}

func (m *goalPanelModel) moveGhostDown() ghostState {
	sep := m.separatorIndex()
	g := m.moveGhost
	if g.IsDraft && g.Pos >= sep {
		return g
	}
	isDraftAboveSep := g.IsDraft && g.Pos < sep
	wasAfter := g.After
	g.After = false
	g.Before = false
	movedIdx := m.ghostMovedIdx()
	if !wasAfter && movedIdx >= 0 && g.Pos < movedIdx && g.Pos < len(m.list.Nodes) {
		node := m.list.Nodes[g.Pos]
		if node.Depth == g.Depth && node.Kind == nodeGoal {
			return applyDownTarget(m.list, g, g.Pos)
		}
	}
	if !isDraftAboveSep && movedIdx >= 0 && g.Pos+1 == movedIdx {
		g.Pos = movedIdx
	}
	if isDraftAboveSep && movedIdx >= 0 && g.Pos+1 == movedIdx {
		g.Pos = movedIdx
	}
	next := moveGhostDown(m.list, g)
	if isDraftAboveSep {
		if next.Pos < sep {
			next.IsDraft = true
			return next
		}
		origIdx := m.ghostMovedIdx()
		if origIdx >= 0 {
			return ghostState{ThreadID: m.moveGhost.ThreadID, Pos: origIdx, Depth: 0, IsDraft: true}
		}
		return ghostState{ThreadID: m.moveGhost.ThreadID, Pos: sep, Depth: 0, IsDraft: true}
	}
	if !m.moveGhost.IsDraft && next.Pos >= sep && m.moveGhost.Pos < sep {
		return ghostState{ThreadID: m.moveGhost.ThreadID, Pos: sep, Depth: 0}
	}
	return next
}

func (m *goalPanelModel) commitMove() (tea.Model, tea.Cmd) {
	g := m.moveGhost
	if g.IsGoal {
		if err := applyGoalMove(g.GoalID, m.list, g); err != nil {
			m.setStatus(err.Error())
			m.mode = goalModeList
			return m, nil
		}
		m.mode = goalModeList
		m.setStatus("Goal moved")
		m.reload()
		if idx := m.list.indexOfGoal(g.GoalID); idx >= 0 {
			m.cursor = idx
		}
		return m, nil
	}
	sep := m.separatorIndex()
	if g.IsDraft && g.Pos < sep {
		wid := strings.TrimPrefix(g.ThreadID, "draft:")
		if _, err := promoteDraftToThread(wid, m.moveName, "", ""); err != nil {
			m.setStatus(err.Error())
			m.mode = goalModeList
			return m, nil
		}
		list, _ := buildDisplayList(m.expanded)
		m.list = list
		var newThreadID string
		for _, n := range list.Nodes {
			if n.Kind == nodeThread && n.Thread != nil && strings.TrimSpace(n.Thread.WindowID) == wid {
				newThreadID = n.Thread.ID
				break
			}
		}
		if newThreadID != "" {
			ghostAfter := ghostState{ThreadID: newThreadID, Pos: g.Pos, Depth: g.Depth}
			if ghostAfter.Pos >= len(list.Nodes) {
				ghostAfter.Pos = len(list.Nodes) - 1
			}
			if err := applyGhostMove(newThreadID, list, ghostAfter); err != nil {
				m.setStatus(err.Error())
				m.mode = goalModeList
				m.reload()
				return m, nil
			}
		}
		m.setStatus("Promoted")
		m.mode = goalModeList
		m.reload()
		if newThreadID != "" {
			if idx := m.list.indexOfThread(newThreadID); idx >= 0 {
				m.cursor = idx
			}
		}
		return m, nil
	}
	if !g.IsDraft && g.Pos >= sep {
		wid := ""
		if store, err := loadGoalStore(); err == nil && store != nil {
			if t := store.Threads[strings.TrimSpace(g.ThreadID)]; t != nil {
				wid = strings.TrimSpace(t.WindowID)
			}
		}
		if err := deleteThread(g.ThreadID); err != nil {
			m.setStatus(err.Error())
		} else {
			m.setStatus("Demoted to draft")
		}
		m.mode = goalModeList
		m.reload()
		if wid != "" {
			for i, n := range m.list.Nodes {
				if n.Kind == nodeDraft && n.Draft != nil && strings.TrimSpace(n.Draft.WindowID) == wid {
					m.cursor = i
					break
				}
			}
		}
		return m, nil
	}
	if err := applyGhostMove(g.ThreadID, m.list, g); err != nil {
		m.setStatus(err.Error())
		m.mode = goalModeList
		return m, nil
	}
	m.mode = goalModeList
	m.setStatus("Moved")
	m.reload()
	if idx := m.list.indexOfThread(g.ThreadID); idx >= 0 {
		m.cursor = idx
	}
	return m, nil
}

// ── contextual D ────────────────────────────────────────────

func (m *goalPanelModel) handleContextualD() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	switch n.Kind {
	case nodeGoal:
		m.confirmGoal = n.Goal.ID
		m.mode = goalModeConfirmDeleteGoal
	case nodeThread:
		m.confirmThread = n.Thread.ID
		if strings.TrimSpace(n.Thread.WindowID) == "" {
			m.mode = goalModeConfirmDeletePlanned
		} else {
			m.mode = goalModeConfirmDone
		}
	case nodeTodo:
		_ = deleteTmuxTodoByIndex(todoScopeWindow, n.TodoWindow, n.TodoIndex)
		m.reload()
	}
	return m, nil
}

func (m *goalPanelModel) updateConfirmDeleteGoal(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y":
		if err := deleteGoalCascade(m.confirmGoal); err != nil {
			m.setStatus(err.Error())
		} else {
			m.setStatus("Goal deleted")
		}
		m.mode = goalModeList
		m.reload()
	case "n", "esc":
		m.mode = goalModeList
	}
	return m, nil
}

func (m *goalPanelModel) updateConfirmDone(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "enter":
		t, _ := loadGoalStore()
		if t != nil {
			if th := t.Threads[m.confirmThread]; th != nil {
				_ = setThreadDone(m.confirmThread, !th.Done)
			}
		}
		m.mode = goalModeList
		m.reload()
	case "n", "esc":
		m.mode = goalModeList
	}
	return m, nil
}

func (m *goalPanelModel) updateConfirmDeletePlanned(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "enter":
		_ = deleteThread(m.confirmThread)
		m.mode = goalModeList
		m.reload()
	case "n", "esc":
		m.mode = goalModeList
	}
	return m, nil
}

func (m *goalPanelModel) updateConfirmMerge(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "enter":
		_ = mergeThreads(m.confirmMergeSrc, m.confirmMergeTgt)
		m.mode = goalModeList
		m.reload()
	case "n", "esc":
		m.mode = goalModeList
	}
	return m, nil
}

func (m *goalPanelModel) handleDeleteOrRemoveTodo() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	if n.Kind == nodeTodo {
		_ = deleteTmuxTodoByIndex(todoScopeWindow, n.TodoWindow, n.TodoIndex)
		m.reload()
	}
	return m, nil
}

func (m *goalPanelModel) toggleTodoOrExpand() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	if n.Kind == nodeTodo {
		_ = toggleTmuxTodoByIndex(todoScopeWindow, n.TodoWindow, n.TodoIndex)
		m.reload()
		return m, nil
	}
	if n.Kind == nodeThread || n.Kind == nodeDraft {
		m.toggleExpand()
	}
	return m, nil
}

func (m *goalPanelModel) copyNodeTitle() (tea.Model, tea.Cmd) {
	n := m.currentNode()
	if n == nil {
		return m, nil
	}
	title := ""
	switch n.Kind {
	case nodeGoal:
		title = n.Goal.Title
	case nodeThread:
		title = n.Thread.Name
	case nodeDraft:
		title = draftDisplayName(n.Draft)
	case nodeTodo:
		title = n.Todo.Title
	}
	if title == "" {
		return m, nil
	}
	if err := writeTodoClipboard(title); err != nil {
		m.setStatus(err.Error())
	} else {
		m.setStatus("copied")
	}
	return m, nil
}

func threadIDForNode(n *goalNode) string {
	if n == nil {
		return ""
	}
	if n.Kind == nodeThread {
		return n.Thread.ID
	}
	return ""
}

func renameGoal(goalID, title string) error {
	return mutateGoalStore(func(store *GoalStore) error {
		g, ok := store.Goals[strings.TrimSpace(goalID)]
		if !ok {
			return fmt.Errorf("goal not found: %s", goalID)
		}
		g.Title = strings.TrimSpace(title)
		return nil
	})
}

func (m *goalPanelModel) runFunc(fn func() error, closeAfter bool) tea.Cmd {
	return func() tea.Msg {
		return goalActionMsg{err: fn(), closeAfter: closeAfter}
	}
}

// ── View ────────────────────────────────────────────────────

func (m *goalPanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func (m *goalPanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	switch m.mode {
	case goalModeRename, goalModeAddGoal, goalModeAddThread, goalModeAddTodo:
		return m.renderInputBox(styles, width, height)
	case goalModeSetGoal, goalModeSetBlocker, goalModeMoreOptions:
		return m.renderPicker(styles, width, height)
	case goalModePromoteGoal:
		return m.renderList(styles, width, height)
	case goalModePromote:
		return m.renderPromote(styles, width, height)
	case goalModeConfirmDeleteGoal:
		return m.renderConfirmDeleteGoal(styles, width, height)
	case goalModeConfirmDeletePlanned:
		return m.renderConfirmDone(styles, width, height)
	case goalModeConfirmDone:
		return m.renderConfirmDone(styles, width, height)
	case goalModeHelp:
		return m.renderHelp(styles, width, height)
	default:
		return m.renderList(styles, width, height)
	}
}

func (m *goalPanelModel) renderList(styles paletteStyles, width, height int) string {
	store, _ := loadGoalStore()
	drafts, _ := collectDrafts()
	goalCount := len(store.Goals)
	threadCount := len(store.Threads)
	draftCount := len(drafts)
	headerText := "Goals"
	statsText := fmt.Sprintf("%d goals · %d threads · %d unassigned", goalCount, threadCount, draftCount)
	statsLine := padBetween(styles.title.Render(headerText), styles.meta.Render(statsText), width-2)
	innerW := width - 2
	body := m.renderTree(styles, innerW, height-4)
	footer := m.renderFooter(styles, innerW)
	view := lipgloss.JoinVertical(lipgloss.Left, statsLine, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *goalPanelModel) renderTree(styles paletteStyles, width, height int) string {
	if m.list == nil || len(m.list.Nodes) == 0 {
		empty := styles.muted.Render("No goals or threads yet.")
		addHint := styles.muted.Render("Press G to add a goal, or start an opencode turn to create a draft.")
		return lipgloss.JoinVertical(lipgloss.Left, empty, "", addHint)
	}
	contentH := maxInt(4, height)
	sepLines := 0
	for _, n := range m.list.Nodes {
		if n.Kind == nodeDraft {
			sepLines = 1
			break
		}
	}
	visibleRows := maxInt(2, (contentH-sepLines)/2)

	if m.mode == goalModeMove {
		return m.renderTreeMove(styles, width, contentH, visibleRows)
	}

	offset := stableListOffset(0, m.cursor, visibleRows, len(m.list.Nodes))
	rows := []string{}
	sawDraft := false
	nodeCount := 0
	for i := offset; i < len(m.list.Nodes) && nodeCount < visibleRows; i++ {
		n := m.list.Nodes[i]
		if n.Kind == nodeDraft && !sawDraft {
			sawDraft = true
			if m.mode == goalModePromoteGoal {
				noGoalSel := m.promoteNoGoal
				noGoalStyle := styles.muted
				if noGoalSel {
					noGoalStyle = styles.itemTitle.Copy().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230"))
				}
				noGoalText := noGoalStyle.Render(fmt.Sprintf(" %s  %s", "▹", "No goal (top-level)"))
				noGoalText = fillWidthBg([]string{noGoalText}, width, lipgloss.Color("238"), noGoalSel)
				rows = append(rows, noGoalText)
				nodeCount++
			}
			rows = append(rows, styles.muted.Render(strings.Repeat("─", width)))
		}
		rows = append(rows, m.renderNode(styles, n, i, width))
		nodeCount++
	}
	body := strings.Join(rows, "\n")
	return lipgloss.NewStyle().Width(width).Height(contentH).Render(body)
}

func (m *goalPanelModel) renderTreeMove(styles paletteStyles, width, contentH, visibleRows int) string {
	movedIdx := m.ghostMovedIdx()
	skipStart := movedIdx
	skipEnd := movedIdx + 1
	if m.moveGhost.IsGoal && movedIdx >= 0 {
		skipEnd = m.ghostSubtreeEnd(movedIdx)
	}
	ghostPos := m.moveGhost.Pos
	ghostDepth := m.moveGhost.Depth
	ghostAfter := m.moveGhost.After
	ghostBefore := m.moveGhost.Before
	ghostName := m.moveName

	type displayItem struct {
		node     *goalNode
		isGhost  bool
		ghostDep int
	}
	var items []displayItem
	for i, n := range m.list.Nodes {
		if i >= skipStart && i < skipEnd {
			if ghostPos == i {
				items = append(items, displayItem{isGhost: true, ghostDep: ghostDepth})
			}
			continue
		}
		if ghostPos != i {
			items = append(items, displayItem{node: n})
			continue
		}
		if n.Depth < ghostDepth {
			if ghostAfter {
				items = append(items, displayItem{isGhost: true, ghostDep: ghostDepth})
				items = append(items, displayItem{node: n})
			} else {
				items = append(items, displayItem{node: n})
				items = append(items, displayItem{isGhost: true, ghostDep: ghostDepth})
			}
		} else if n.Depth > ghostDepth {
			if ghostAfter {
				items = append(items, displayItem{node: n})
				items = append(items, displayItem{isGhost: true, ghostDep: ghostDepth})
			} else {
				items = append(items, displayItem{isGhost: true, ghostDep: ghostDepth})
				items = append(items, displayItem{node: n})
			}
		} else {
			if ghostAfter || (ghostPos > movedIdx && !ghostBefore) {
				items = append(items, displayItem{node: n})
				items = append(items, displayItem{isGhost: true, ghostDep: ghostDepth})
			} else {
				items = append(items, displayItem{isGhost: true, ghostDep: ghostDepth})
				items = append(items, displayItem{node: n})
			}
		}
	}
	if ghostPos >= len(m.list.Nodes) || (movedIdx < len(m.list.Nodes) && ghostPos == movedIdx && ghostPos == len(m.list.Nodes)-1) {
		if ghostPos >= len(m.list.Nodes) {
			items = append(items, displayItem{isGhost: true, ghostDep: ghostDepth})
		}
	}

	totalItems := len(items)
	offset := stableListOffset(0, ghostPos, visibleRows, totalItems)
	sep := m.separatorIndex()
	rows := []string{}
	sawDraft := false
	itemCount := 0
	for i := offset; i < totalItems && itemCount < visibleRows; i++ {
		item := items[i]
		isInDraftZone := false
		if item.isGhost {
			origPos := m.moveGhost.Pos
			isInDraftZone = origPos >= sep
		} else {
			isInDraftZone = item.node.Kind == nodeDraft
		}
		if isInDraftZone && !sawDraft {
			sawDraft = true
			rows = append(rows, styles.muted.Render(strings.Repeat("─", width)))
		}
		if item.isGhost {
			indent := strings.Repeat("    ", item.ghostDep)
			maxW := width - len(indent) - 4 - 10
			if maxW < 8 {
				maxW = 8
			}
			glyph := "◦"
			if m.moveGhost.IsGoal {
				glyph = "▾"
			}
			line1 := styles.muted.Render(fmt.Sprintf("%s%s  %s (moving)", indent, glyph, truncate(firstLine(ghostName), maxW)))
			line2 := styles.muted.Render(indent + "   ")
			rows = append(rows, lipgloss.JoinVertical(lipgloss.Left,
				lipgloss.NewStyle().Width(width).Render(line1),
				lipgloss.NewStyle().Width(width).Render(line2)))
		} else {
			rows = append(rows, m.renderNode(styles, item.node, i, width))
		}
		itemCount++
	}
	body := strings.Join(rows, "\n")
	return lipgloss.NewStyle().Width(width).Height(contentH).Render(body)
}

func (m *goalPanelModel) renderNode(styles paletteStyles, n *goalNode, idx, width int) string {
	indent := strings.Repeat("    ", n.Depth)
	selected := idx == m.cursor && m.mode != goalModeMove && !(m.mode == goalModePromoteGoal && m.promoteNoGoal)
	nameStyle := styles.itemTitle
	metaStyle := styles.itemSubtitle
	if selected {
		bg := lipgloss.Color("238")
		nameStyle = nameStyle.Copy().Background(bg).Foreground(lipgloss.Color("230"))
		metaStyle = metaStyle.Copy().Background(bg).Foreground(lipgloss.Color("251"))
	}

	switch n.Kind {
	case nodeGoal:
		store, _ := loadGoalStore()
		hasChildren := false
		if store != nil {
			for _, g := range store.Goals {
				if strings.TrimSpace(g.ParentID) == strings.TrimSpace(n.Goal.ID) {
					hasChildren = true
					break
				}
			}
			if !hasChildren {
				for _, t := range store.Threads {
					if strings.TrimSpace(t.GoalID) == strings.TrimSpace(n.Goal.ID) {
						hasChildren = true
						break
					}
				}
			}
		}
		isCollapsed := false
		if v, ok := m.expanded[n.Goal.ID]; ok && !v {
			isCollapsed = true
		}
		var glyph string
		if !hasChildren {
			glyph = "▹"
		} else if isCollapsed {
			glyph = "▸"
		} else {
			glyph = "▾"
		}
		goalStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("153"))
		if selected {
			goalStyle = goalStyle.Background(lipgloss.Color("238")).Foreground(lipgloss.Color("153"))
		}
		maxW := width - len(indent) - 4
		if maxW < 8 {
			maxW = 8
		}
		label := fmt.Sprintf("%s%s  %s", indent, glyph, truncate(n.Goal.Title, maxW))
		left := goalStyle.Render(label)
		right := ""
		if isCollapsed && hasChildren {
			threadCount, subGoalCount := 0, 0
			for _, c := range m.list.Nodes {
				if c.Depth <= n.Depth {
					continue
				}
				if c.Kind == nodeThread && c.Thread != nil && strings.TrimSpace(c.Thread.GoalID) == strings.TrimSpace(n.Goal.ID) {
					threadCount++
				}
				if c.Kind == nodeGoal && c.Goal != nil && strings.TrimSpace(c.Goal.ParentID) == strings.TrimSpace(n.Goal.ID) {
					subGoalCount++
				}
			}
			parts := []string{}
			if threadCount > 0 {
				parts = append(parts, fmt.Sprintf("%d thread%s", threadCount, pluralS(threadCount)))
			}
			if subGoalCount > 0 {
				parts = append(parts, fmt.Sprintf("%d sub-goal%s", subGoalCount, pluralS(subGoalCount)))
			}
			if len(parts) > 0 {
				rm := metaStyle
				if selected {
					rm = rm.Copy().Background(lipgloss.Color("238"))
				}
				right = rm.Render(strings.Join(parts, " · "))
			}
		}
		return buildRowLine(left, right, width, lipgloss.Color("238"), selected)
	case nodeTodo:
		check := "☐"
		checkStyle := styles.todoCheck
		titleStyle := styles.itemTitle
		if n.Todo != nil && n.Todo.Done {
			check = "☑"
			checkStyle = styles.todoCheckDone
			titleStyle = styles.panelTextDone
		}
		if selected {
			bg := lipgloss.Color("238")
			checkStyle = checkStyle.Copy().Background(bg)
			titleStyle = titleStyle.Copy().Background(bg).Foreground(lipgloss.Color("230"))
		}
		title := ""
		if n.Todo != nil {
			title = n.Todo.Title
		}
		maxW := width - len(indent) - 8
		if maxW < 8 {
			maxW = 8
		}
		checkR := checkStyle.Render(check)
		titleR := titleStyle.Render(truncate(title, maxW))
		indentR := indent + "    "
		sepR := "  "
		if selected {
			bg := lipgloss.Color("238")
			indentR = lipgloss.NewStyle().Background(bg).Render(indentR)
			sepR = lipgloss.NewStyle().Background(bg).Render("  ")
		}
		return fillWidthBg([]string{indentR, checkR, sepR, titleR}, width, lipgloss.Color("238"), selected)
	case nodeDraft:
		isPromoting := m.mode == goalModePromoteGoal && strings.TrimSpace(n.Draft.WindowID) == strings.TrimSpace(m.promoteWindow)
		draftNameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("251"))
		if isPromoting {
			draftNameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("223"))
		}
		if selected {
			draftNameStyle = draftNameStyle.Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230"))
		}
		bg := lipgloss.Color("238")
		icon := threadStatusIcon(n.Status)
		iconStyle := threadStatusIconStyle(styles, n.Status)
		if isPromoting {
			iconStyle = styles.selectedLabel
		}
		if n.Unread {
			icon = "⚑"
			iconStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
		}
		if selected {
			iconStyle = iconStyle.Copy().Background(bg)
		}
		currentWID := m.currentWindowID()
		isCurrent := strings.TrimSpace(n.Draft.WindowID) == currentWID && currentWID != ""
		maxW := width - len(indent) - 6
		if isCurrent {
			maxW -= lipgloss.Width("► current") + 2
		}
		if maxW < 8 {
			maxW = 8
		}
		indentR := indent
		sepR := " "
		if selected {
			indentR = lipgloss.NewStyle().Background(bg).Render(indentR)
			sepR = lipgloss.NewStyle().Background(bg).Render(sepR)
		}
		currentTag := ""
		if isCurrent {
			tagStyle := styles.selectedLabel
			if selected {
				tagStyle = tagStyle.Copy().Background(bg)
			}
			gap := "  "
			if selected {
				gap = lipgloss.NewStyle().Background(bg).Render(gap)
			}
			currentTag = gap + tagStyle.Render("► current")
		}
		hasName := strings.TrimSpace(n.Draft.Name) != ""
		dispName := n.Draft.Name
		if !hasName {
			dispName = strings.TrimSpace(n.SessionName) + " " + strings.TrimSpace(n.WindowName)
			dispName = strings.TrimSpace(dispName)
		}
		nameR := draftNameStyle.Render(truncate(firstLine(dispName), maxW))
		left := indentR + iconStyle.Render(icon) + sepR + nameR + currentTag

		rightParts := []string{}
		if hasName {
			if s := strings.TrimSpace(n.SessionName); s != "" {
				loc := s
				if w := strings.TrimSpace(n.WindowName); w != "" {
					loc += " / " + w
				}
				rightParts = append(rightParts, loc)
			}
		}
		if n.HasTodos {
			rightParts = append(rightParts, fmt.Sprintf("%d/%d todos", n.OpenTodos, n.TotalTodos))
		}
		right := ""
		if len(rightParts) > 0 {
			rm := metaStyle
			if selected {
				rm = rm.Copy().Background(bg)
			}
			right = rm.Render(strings.Join(rightParts, " · "))
		}
		line1 := buildRowLine(left, right, width, bg, selected)
		line2 := m.renderStatusLine(styles, metaStyle, n, indent, width, selected)
		return lipgloss.JoinVertical(lipgloss.Left, line1, line2)
	case nodeThread:
		name := threadDisplayName(n.Thread, "")
		if n.Thread.Done {
			nameStyle = styles.panelTextDone
			if selected {
				nameStyle = nameStyle.Copy().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("246"))
			}
		}
		bg := lipgloss.Color("238")
		icon := threadStatusIcon(n.Status)
		iconStyle := threadStatusIconStyle(styles, n.Status)
		if n.Unread {
			icon = "⚑"
			iconStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
		}
		if selected {
			iconStyle = iconStyle.Copy().Background(bg)
		}
		maxW := width - len(indent) - 6
		if maxW < 8 {
			maxW = 8
		}
		indentR := indent
		sepR := " "
		if selected {
			indentR = lipgloss.NewStyle().Background(bg).Render(indentR)
			sepR = lipgloss.NewStyle().Background(bg).Render(sepR)
		}
		nameR := nameStyle.Render(truncate(name, maxW))
		left := indentR + iconStyle.Render(icon) + sepR + nameR

		rightParts := []string{}
		if s := strings.TrimSpace(n.SessionName); s != "" {
			loc := s
			if w := strings.TrimSpace(n.WindowName); w != "" {
				loc += " / " + w
			}
			rightParts = append(rightParts, loc)
		}
		if n.HasTodos {
			rightParts = append(rightParts, fmt.Sprintf("%d/%d todos", n.OpenTodos, n.TotalTodos))
		}
		right := ""
		if len(rightParts) > 0 {
			rm := metaStyle
			if selected {
				rm = rm.Copy().Background(bg)
			}
			right = rm.Render(strings.Join(rightParts, " · "))
		}
		line1 := buildRowLine(left, right, width, bg, selected)
		line2 := m.renderStatusLine(styles, metaStyle, n, indent, width, selected)
		return lipgloss.JoinVertical(lipgloss.Left, line1, line2)
	}
	return ""
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (m *goalPanelModel) renderStatusLine(styles paletteStyles, metaStyle lipgloss.Style, n *goalNode, indent string, width int, selected bool) string {
	bg := lipgloss.Color("238")
	indentText := indent + "  "
	indentR := indentText
	if selected {
		indentR = lipgloss.NewStyle().Background(bg).Render(indentText)
	}

	segments := []string{indentR}

	currentWID := m.currentWindowID()
	isCurrent := false
	if n.Kind == nodeThread && strings.TrimSpace(n.Thread.WindowID) == currentWID && currentWID != "" {
		isCurrent = true
	} else if n.Kind == nodeDraft && strings.TrimSpace(n.Draft.WindowID) == currentWID && currentWID != "" {
		isCurrent = true
	}
	showCurrentTag := isCurrent && n.Kind != nodeDraft
	if showCurrentTag {
		tagStyle := styles.selectedLabel
		if selected {
			tagStyle = tagStyle.Copy().Background(bg)
		}
		segments = append(segments, tagStyle.Render("● current"))
	}

	primary := ""
	primaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	if n.Status == threadStatusBlocked && n.Kind == nodeThread {
		store, _ := loadGoalStore()
		names := blockerNames(store, n.Thread)
		if len(names) > 0 {
			primary = "blocked ← " + strings.Join(names, ", ")
			primaryStyle = styles.statusBad
		}
	}
	if primary == "" && n.LastUpdate != "" {
		primary = n.LastUpdate
		if n.Phase != "" {
			primaryStyle = phaseDisplayStyle(styles, n.Phase)
		}
	}
	if primary == "" {
		if pt, phaseStyle := phaseDisplay(styles, n.Phase); pt != "" {
			primary = pt
			primaryStyle = phaseStyle
		}
	}
	if primary == "" && n.Unread {
		primary = "needs review"
		primaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
	}

	prefixW := 0
	if showCurrentTag {
		prefixW = lipgloss.Width("● current")
		if primary != "" {
			prefixW += lipgloss.Width(" · ")
		}
	}
	maxW := width - lipgloss.Width(indentText) - prefixW
	if maxW < 8 {
		maxW = 8
	}
	primary = truncate(primary, maxW)

	if selected {
		primaryStyle = primaryStyle.Copy().Background(bg)
	}
	if primary != "" {
		if showCurrentTag {
			ms := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			if selected {
				ms = ms.Copy().Background(bg)
			}
			segments = append(segments, ms.Render(" · "))
		}
		segments = append(segments, primaryStyle.Render(primary))
	}

	return fillWidthBg(segments, width, bg, selected)
}

func phaseDisplayStyle(styles paletteStyles, phase string) lipgloss.Style {
	switch phase {
	case "waiting":
		return styles.muted
	case "responding":
		return styles.selectedLabel
	case "tool":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
	case "question":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
	default:
		return styles.muted
	}
}

func phaseDisplay(styles paletteStyles, phase string) (string, lipgloss.Style) {
	switch phase {
	case "waiting":
		return "waiting for response…", styles.muted
	case "responding":
		return "responding…", styles.selectedLabel
	case "tool":
		return "running tool…", lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
	case "question":
		return "awaiting answer…", lipgloss.NewStyle().Foreground(lipgloss.Color("221"))
	default:
		return "", lipgloss.Style{}
	}
}

func threadStatusIcon(status string) string {
	switch status {
	case threadStatusRunning:
		frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
		now := time.Now()
		return string(frames[int(now.UnixNano()/int64(100*time.Millisecond))%len(frames)])
	case threadStatusDone:
		return "✓"
	case threadStatusBlocked:
		return "✗"
	case threadStatusReady:
		return "◦"
	default:
		return "◦"
	}
}

func threadStatusIconStyle(styles paletteStyles, status string) lipgloss.Style {
	switch status {
	case threadStatusRunning:
		return styles.selectedLabel
	case threadStatusDone:
		return styles.todoCheckDone
	case threadStatusBlocked:
		return styles.statusBad
	default:
		return styles.muted
	}
}

func renderThreadStatus(styles paletteStyles, status string, selected bool) string {
	var label string
	style := styles.meta
	switch status {
	case threadStatusRunning:
		frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
		now := time.Now()
		label = string(frames[int(now.UnixNano()/int64(100*time.Millisecond))%len(frames)]) + " running"
		style = styles.selectedLabel
	case threadStatusReady:
		label = "ready"
		style = styles.selectedLabel
	case threadStatusBlocked:
		label = "⛔ blocked"
		style = styles.statusBad
	case threadStatusDone:
		label = "✓ done"
		style = styles.todoCheckDone
	case threadStatusPlanned:
		label = "planned"
		style = styles.muted
	}
	if selected {
		style = style.Copy().Background(lipgloss.Color("238"))
	}
	if label == "" {
		return ""
	}
	return style.Render(label)
}

func goalHasVisibleChildren(expanded map[string]bool, g *goalNode, list *flatList) bool {
	if list == nil {
		return false
	}
	for _, n := range list.Nodes {
		if n.Kind == nodeThread && n.Thread != nil && strings.TrimSpace(n.Thread.GoalID) == strings.TrimSpace(g.Goal.ID) {
			return true
		}
		if n.Kind == nodeGoal && n.Goal != nil && strings.TrimSpace(n.Goal.ParentID) == strings.TrimSpace(g.Goal.ID) {
			return true
		}
		if n.Kind == nodeDraft && g.Goal.ID == "" && g.Depth == 0 {
			return true
		}
	}
	return false
}

func padBetween(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func fillWidthBg(segments []string, width int, bg lipgloss.Color, selected bool) string {
	joined := strings.Join(segments, "")
	if !selected {
		trailing := width - lipgloss.Width(joined)
		if trailing > 0 {
			joined += strings.Repeat(" ", trailing)
		}
		return joined
	}
	totalUsed := lipgloss.Width(joined)
	trailing := width - totalUsed
	if trailing > 0 {
		joined += lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", trailing))
	}
	return joined
}

func buildRowLine(left, right string, width int, bg lipgloss.Color, selected bool) string {
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	if leftW+rightW+2 > width {
		right = ""
		rightW = 0
	}
	gap := width - leftW - rightW
	if gap < 0 {
		gap = 0
	}
	if selected && gap > 0 {
		gapFill := lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", gap))
		return left + gapFill + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *goalPanelModel) renderFooter(styles paletteStyles, width int) string {
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "  ", pairs)
	}
	if m.mode == goalModePromoteGoal {
		draftName := truncate(firstLine(string(m.promoteName)), 30)
		draftTag := styles.selectedLabel.Render("◇ " + draftName)
		hints := [][2]string{{"u/e", "move"}, {"a", "new goal"}, {"Enter", "promote"}, {"Esc", "cancel"}}
		hintStr := pickRenderedShortcutFooter(width-lipgloss.Width(draftTag)-4, renderSegments, hints, hints)
		return draftTag + "  " + hintStr
	}
	n := m.currentNode()

	enterLabel := "open"
	if n != nil {
		switch n.Kind {
		case nodeGoal:
			enterLabel = "expand"
		case nodeThread:
			if strings.TrimSpace(n.Thread.WindowID) == "" {
				enterLabel = "bind"
			} else {
				enterLabel = "goto"
			}
		case nodeDraft, nodeTodo:
			enterLabel = "goto"
		}
	}

	base := [][2]string{{"u/e", "move"}, {"t", "thread"}, {"a", "goal"}, {"Enter", enterLabel}, {"Esc", "back"}}
	var full [][2]string
	if n != nil {
		switch n.Kind {
		case nodeGoal:
			full = [][2]string{{"u/e", "move"}, {"Alt-U/E", "reorder"}, {"t", "thread"}, {"T", "top"}, {"a", "sub-goal"}, {"A", "top goal"}, {"r", "rename"}, {"R", "edit"}, {"Enter", enterLabel}, {"D", "delete"}, {"m", "move"}, {"o", "more"}, {"Esc", "back"}}
		case nodeThread:
			dLabel := "done"
			if strings.TrimSpace(n.Thread.WindowID) == "" {
				dLabel = "delete"
			}
			full = [][2]string{{"u/e", "move"}, {"Alt-U/E", "reorder"}, {"t", "thread"}, {"a", "goal"}, {"r", "rename"}, {"R", "edit"}, {"g", "goal"}, {"m", "move"}, {"Enter", enterLabel}, {"D", dLabel}, {"o", "more"}, {"Esc", "back"}}
		case nodeDraft:
			full = [][2]string{{"u/e", "move"}, {"a", "goal"}, {"m", "move"}, {"p", "promote"}, {"Enter", enterLabel}, {"o", "more"}, {"Esc", "back"}}
		case nodeTodo:
			full = [][2]string{{"u/e", "move"}, {"Enter", enterLabel}, {"D", "del"}, {"Esc", "back"}}
		default:
			full = base
		}
	} else {
		full = base
	}
	return pickRenderedShortcutFooter(width, renderSegments, full, base)
}

func (m *goalPanelModel) renderInputBox(styles paletteStyles, width, height int) string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render(m.inputLabel),
		"",
		styles.input.Render(renderInputValue(m.inputText, m.inputCursor, styles)),
		"",
		styles.modalHint.Render(m.inputHint),
	)
	box := styles.modal.Width(minInt(72, maxInt(34, width-10))).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *goalPanelModel) renderPicker(styles paletteStyles, width, height int) string {
	title := "Pick"
	switch m.mode {
	case goalModeSetGoal:
		title = "Set goal"
	case goalModeSetBlocker:
		title = "Set blocker"
	case goalModeMoreOptions:
		title = "Actions"
	case goalModePromoteGoal:
		title = "Promote to thread — pick goal"
	}
	items := m.filteredPickerItems()
	innerW := width - 2
	contentH := maxInt(4, height-8)
	visibleRows := contentH
	offset := stableListOffset(m.pickerOffset, m.pickerCursor, visibleRows, len(items))
	m.pickerOffset = offset
	rows := []string{}
	for i := offset; i < len(items) && len(rows) < visibleRows; i++ {
		line := items[i].Label
		if items[i].Subtitle != "" {
			line += "  " + items[i].Subtitle
		}
		style := styles.item
		if i == m.pickerCursor {
			style = styles.selectedItem
		}
		rows = append(rows, style.Width(innerW).Render(truncate(line, maxInt(8, innerW-2))))
	}
	if len(rows) == 0 {
		rows = append(rows, styles.muted.Render("no matches"))
	}
	body := strings.Join(rows, "\n")
	filterLine := styles.searchBox.Width(innerW).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			styles.searchPrompt.Render(">"),
			" ",
			styles.input.Render(renderInputValue(m.pickerFilter, m.inputCursor, styles)),
		),
	)
	view := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render(title),
		"",
		filterLine,
		"",
		body,
		"",
		styles.modalHint.Render("u/e to move · type to filter · Enter to select · Esc to cancel"),
	)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *goalPanelModel) renderPromote(styles paletteStyles, width, height int) string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render("Promote draft"),
		styles.meta.Render("name"),
		styles.input.Render(renderInputValue(m.promoteName, m.promoteCursor, styles)),
		"",
		styles.modalHint.Render("Enter promote  Esc cancel"),
	)
	box := styles.modal.Width(minInt(72, maxInt(34, width-10))).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *goalPanelModel) renderConfirmDeleteGoal(styles paletteStyles, width, height int) string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render("Delete goal?"),
		styles.modalBody.Render("Sub-goals deleted; threads unassigned."),
		"",
		styles.modalHint.Render("y to delete  n to cancel"),
	)
	box := styles.modal.Width(minInt(60, maxInt(34, width-10))).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *goalPanelModel) renderConfirmDone(styles paletteStyles, width, height int) string {
	title := "Toggle done?"
	if m.mode == goalModeConfirmDeletePlanned {
		title = "Delete thread?"
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render(title),
		styles.modalHint.Render("y to confirm · n to cancel"),
	)
	box := styles.modal.Width(minInt(40, maxInt(24, width-10))).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *goalPanelModel) renderHelp(styles paletteStyles, width, height int) string {
	lines := []string{
		"u/e move through the list. n jumps to the parent goal.",
		"→ or t expands a thread to show its todos. Enter goes to the thread's window (or binds it if planned).",
		"r renames. g sets the goal. b sets a blocker. Alt-U/E reorders the item among its siblings (no nesting). m enters move mode (u/e to reposition, Enter to commit).",
		"p promotes a draft into a named thread. D marks a thread done / deletes a goal (cascade).",
		"a adds a todo to the selected thread. c toggles a todo. l toggles the assigned/unassigned view.",
		"o opens all actions. Esc returns to the palette.",
	}
	styled := make([]string, 0, len(lines))
	for _, line := range lines {
		styled = append(styled, styles.panelText.Render(truncate(line, maxInt(10, width-4))))
	}
	return lipgloss.JoinVertical(lipgloss.Left, styles.title.Render("Guide"), "", strings.Join(styled, "\n\n"))
}
