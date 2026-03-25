package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/david/agent-tracker/internal/ipc"
)

const (
	trackerTaskStatusInProgress = "in_progress"
	trackerTaskStatusCompleted  = "completed"
)

type trackerPanelTickMsg struct{}

type trackerPanelStateMsg struct {
	env *ipc.Envelope
	err error
}

type trackerPanelCommandMsg struct {
	message string
	err     error
	close   bool
}

type trackerPanelListState struct {
	selected int
	offset   int
}

type trackerPanelContext struct {
	SessionName string
	SessionID   string
	WindowName  string
	WindowID    string
	PaneID      string
}

type trackerPanelModel struct {
	runtime         *paletteRuntime
	currentCtx      trackerPanelContext
	width           int
	height          int
	taskList        trackerPanelListState
	state           ipc.Envelope
	loaded          bool
	message         string
	refreshedAt     time.Time
	refreshInFlight bool
	pendingRefresh  bool
	showAltHints    bool
	helpVisible     bool
	requestBack     bool
	requestClose    bool
}

func newTrackerPanelModel(runtime *paletteRuntime) *trackerPanelModel {
	model := &trackerPanelModel{runtime: runtime, message: "Loading tracker..."}
	model.syncCurrentContext()
	return model
}

func (m *trackerPanelModel) activate() tea.Cmd {
	m.requestBack = false
	m.requestClose = false
	m.syncCurrentContext()
	if !m.loaded {
		m.message = "Loading tracker..."
	}
	return tea.Batch(trackerPanelTickCmd(), m.requestRefreshCmd())
}

func (m *trackerPanelModel) Init() tea.Cmd {
	return m.activate()
}

func (m *trackerPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case trackerPanelTickMsg:
		cmds := []tea.Cmd{trackerPanelTickCmd()}
		if !m.loaded || time.Since(m.refreshedAt) >= time.Second {
			cmds = append(cmds, m.requestRefreshCmd())
		}
		return m, tea.Batch(cmds...)
	case trackerPanelStateMsg:
		m.refreshInFlight = false
		if msg.err != nil {
			m.message = msg.err.Error()
		} else if msg.env != nil {
			m.state = *msg.env
			m.loaded = true
			m.refreshedAt = time.Now()
			if text := strings.TrimSpace(msg.env.Message); text != "" {
				m.message = text
			}
			m.syncCurrentContext()
			m.clampSelections()
		}
		if m.pendingRefresh {
			m.pendingRefresh = false
			return m, m.requestRefreshCmd()
		}
		return m, nil
	case trackerPanelCommandMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		if text := strings.TrimSpace(msg.message); text != "" {
			m.message = text
		}
		if msg.close {
			m.requestClose = true
			return m, nil
		}
		return m, m.requestRefreshCmd()
	case tea.KeyMsg:
		if isAltFooterToggleKey(msg) {
			m.showAltHints = !m.showAltHints
			return m, nil
		}
		m.showAltHints = false
		return m.updateNormal(msg.String())
	}
	return m, nil
}

func (m *trackerPanelModel) updateNormal(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "ctrl+c":
		m.requestBack = true
		return m, nil
	case "?":
		m.helpVisible = !m.helpVisible
		return m, nil
	}
	if m.helpVisible {
		return m, nil
	}
	switch key {
	case "u", "up", "ctrl+u":
		m.moveSelection(-1)
		return m, nil
	case "e", "down", "ctrl+e":
		m.moveSelection(1)
		return m, nil
	case "enter", "p":
		return m.runPrimaryAction()
	case "c":
		return m.toggleSelected()
	case "D":
		return m.deleteSelected()
	}
	return m, nil
}

func (m *trackerPanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func (m *trackerPanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	contentWidth := maxInt(16, width-2)
	contentHeight := maxInt(8, height-7)
	contextLine := "Tracker"
	if location := m.renderContextLine(); strings.TrimSpace(location) != "" {
		contextLine += "  ·  " + location
	}
	header := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("Tracker"),
		styles.meta.Render(truncate(contextLine, maxInt(10, contentWidth))),
		styles.muted.Render(truncate(m.renderMetricsLine(), maxInt(10, contentWidth))),
	)
	var body string
	if m.helpVisible {
		body = m.renderHelp(styles, contentWidth, contentHeight)
	} else {
		body = m.renderTasks(styles, contentWidth, contentHeight)
	}
	footer := m.renderFooter(styles, contentWidth)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *trackerPanelModel) renderTasks(styles paletteStyles, width, height int) string {
	list := m.visibleTasks()
	if width < 84 {
		return m.renderTaskListSection(styles, width, height, list, "Queue", "Across your active tracker feed")
	}
	leftWidth := maxInt(34, width*56/100)
	rightWidth := maxInt(24, width-leftWidth-5)
	listSection := m.renderTaskListSection(styles, leftWidth, height, list, "Queue", "What still needs attention")
	detailSection := m.renderTaskDetailSection(styles, rightWidth, height)
	divider := styles.muted.Render(renderVerticalDivider(height))
	return lipgloss.JoinHorizontal(lipgloss.Top, listSection, "  ", divider, "  ", detailSection)
}

func (m *trackerPanelModel) renderTaskListSection(styles paletteStyles, width, height int, tasks []ipc.Task, title, meta string) string {
	contentHeight := maxInt(1, height-3)
	entryHeight := 3
	entriesPerPage := maxInt(1, contentHeight/entryHeight)
	selected := clampInt(m.taskList.selected, 0, maxInt(0, len(tasks)-1))
	offset := stableListOffset(m.taskList.offset, selected, entriesPerPage, len(tasks))
	m.taskList.selected = selected
	m.taskList.offset = offset
	rows := []string{}
	if len(tasks) == 0 {
		rows = append(rows, trackerEmptyState(styles, "No tasks in motion."))
	} else {
		now := time.Now()
		for idx := offset; idx < len(tasks) && idx < offset+entriesPerPage; idx++ {
			rows = append(rows, m.renderTaskRow(styles, tasks[idx], idx == selected, width, now))
		}
	}
	sectionMeta := fmt.Sprintf("%d tasks", len(tasks))
	if strings.TrimSpace(meta) != "" {
		sectionMeta += "  ·  " + meta
	}
	return trackerRenderSection(styles, title, sectionMeta, strings.Join(rows, "\n"), width, height)
}

func (m *trackerPanelModel) renderTaskRow(styles paletteStyles, task ipc.Task, selected bool, width int, now time.Time) string {
	selectedBG := lipgloss.Color("238")
	rowWidth := maxInt(16, width)
	titleStyle := styles.itemTitle
	metaStyle := styles.itemSubtitle
	indicator := trackerTaskIndicator(task, now)
	indicatorStyle := styles.selectedLabel
	if task.Status == trackerTaskStatusCompleted {
		indicatorStyle = styles.statusBad
		titleStyle = styles.itemTitle.Copy().Foreground(lipgloss.Color("246"))
		metaStyle = styles.itemSubtitle.Copy().Foreground(lipgloss.Color("246"))
		if task.Acknowledged {
			indicatorStyle = styles.todoCheckDone
		}
	}
	padStyle := lipgloss.NewStyle()
	if selected {
		titleStyle = titleStyle.Copy().Foreground(lipgloss.Color("230")).Background(selectedBG)
		metaStyle = metaStyle.Copy().Foreground(lipgloss.Color("251")).Background(selectedBG)
		indicatorStyle = indicatorStyle.Copy().Background(selectedBG)
		padStyle = padStyle.Copy().Foreground(lipgloss.Color("230")).Background(selectedBG)
	}
	duration := trackerLiveDuration(task, now)
	meta := trackerFirstNonEmpty(strings.TrimSpace(task.Session), "Session")
	if strings.TrimSpace(task.Window) != "" {
		meta += "  /  " + strings.TrimSpace(task.Window)
	}
	if task.Status == trackerTaskStatusCompleted && !task.Acknowledged {
		meta += "  ·  awaiting review"
	}
	if duration != "" {
		meta = strings.TrimSpace(meta + "  ·  " + duration)
	}
	titleText := truncate(firstPaletteLine(task.Summary), maxInt(1, rowWidth-4-lipgloss.Width(indicator)))
	line1RawWidth := 1 + lipgloss.Width(indicator) + 1 + lipgloss.Width(titleText)
	line1Pad := maxInt(0, rowWidth-line1RawWidth)
	line1 := padStyle.Render(" ") + indicatorStyle.Render(indicator) + padStyle.Render(" ") + titleStyle.Render(titleText) + padStyle.Render(strings.Repeat(" ", line1Pad))
	metaText := truncate(meta, maxInt(1, rowWidth-3))
	line2Pad := maxInt(0, rowWidth-2-lipgloss.Width(metaText))
	line2 := padStyle.Render("  ") + metaStyle.Render(metaText) + padStyle.Render(strings.Repeat(" ", line2Pad))
	line3 := padStyle.Render(strings.Repeat(" ", rowWidth))
	return lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3)
}

func (m *trackerPanelModel) renderTaskDetailSection(styles paletteStyles, width, height int) string {
	task := m.selectedTask()
	if task == nil {
		return trackerRenderSection(styles, "Selected", "Open a task to jump into its tmux pane", trackerEmptyState(styles, "Nothing is selected."), width, height)
	}
	status := "Live"
	if task.Status == trackerTaskStatusCompleted {
		if task.Acknowledged {
			status = "Done"
		} else {
			status = "Needs review"
		}
	}
	meta := trackerFirstNonEmpty(strings.TrimSpace(task.Session), task.SessionID)
	if strings.TrimSpace(task.Window) != "" {
		meta += " / " + strings.TrimSpace(task.Window)
	}
	lines := []string{
		trackerRenderWrappedText(styles.panelText.Copy().Bold(true), firstPaletteLine(task.Summary), maxInt(10, width)),
		"",
		trackerDetailLine(styles, "status", status, width),
		trackerDetailLine(styles, "window", meta, width),
		trackerDetailLine(styles, "elapsed", trackerLiveDuration(*task, time.Now()), width),
	}
	if note := strings.TrimSpace(task.CompletionNote); note != "" {
		lines = append(lines, "", styles.muted.Render("note"), trackerRenderWrappedText(styles.panelTextDone, note, maxInt(10, width)))
	}
	return trackerRenderSection(styles, "Selected", "Enter opens the highlighted pane", strings.Join(lines, "\n"), width, height)
}

func (m *trackerPanelModel) renderHelp(styles paletteStyles, width, height int) string {
	lines := []string{
		"u and e move through tasks. enter opens the highlighted tmux pane.",
		"c settles a task. shift-d deletes it. esc returns to the command palette.",
	}
	styled := make([]string, 0, len(lines))
	for _, line := range lines {
		styled = append(styled, styles.panelText.Render(truncate(line, maxInt(10, width-4))))
	}
	return trackerRenderSection(styles, "Guide", "Task-only tracker", strings.Join(styled, "\n\n"), width, height)
}

func (m *trackerPanelModel) renderFooter(styles paletteStyles, width int) string {
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "  ", pairs)
	}
	footer := ""
	if m.showAltHints {
		footer = pickRenderedShortcutFooter(width, renderSegments,
			[][2]string{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
			[][2]string{{"Alt-S", "close"}},
		)
	} else {
		footer = pickRenderedShortcutFooter(width, renderSegments,
			[][2]string{{"u/e", "move"}, {"Enter", "open"}, {"c", "settle"}, {"Shift-D", "delete"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
			[][2]string{{"u/e", "move"}, {"Enter", "open"}, {"c", "settle"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
			[][2]string{{"Esc", "back"}, {footerHintToggleKey, "more"}},
		)
	}
	if lipgloss.Width(footer) > width {
		return styles.muted.Copy().Width(width).Render(truncate(strings.TrimSpace(m.currentStatus()), width))
	}
	status := strings.TrimSpace(m.currentStatus())
	if status != "" && status != "Tracker" && status != "Loading tracker..." {
		footer = strings.TrimSpace(status) + "  " + footer
	}
	return lipgloss.NewStyle().Width(width).Render(footer)
}

func trackerRenderSection(styles paletteStyles, title, meta, content string, width, height int) string {
	header := []string{styles.panelTitle.Render(title)}
	if strings.TrimSpace(meta) != "" {
		header = append(header, styles.meta.Render(truncate(meta, maxInt(10, width))))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, strings.Join(header, "\n"), "", content)
	return lipgloss.NewStyle().Width(width).Height(height).Render(body)
}

func trackerEmptyState(styles paletteStyles, text string) string {
	return styles.muted.Render(text)
}

func trackerDetailLine(styles paletteStyles, label, value string, width int) string {
	label = strings.TrimSpace(label)
	value = strings.TrimSpace(value)
	if value == "" {
		value = "-"
	}
	labelWidth := 10
	contentWidth := maxInt(10, width-labelWidth-1)
	parts := wrapText(value, contentWidth)
	if len(parts) == 0 {
		parts = []string{value}
	}
	lines := []string{styles.muted.Copy().Width(labelWidth).Render(label+":") + styles.panelText.Render(truncate(parts[0], contentWidth))}
	indent := strings.Repeat(" ", labelWidth)
	for _, part := range parts[1:] {
		lines = append(lines, indent+styles.panelText.Render(truncate(part, contentWidth)))
	}
	return strings.Join(lines, "\n")
}

func trackerRenderWrappedText(style lipgloss.Style, text string, width int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	parts := wrapText(text, maxInt(10, width))
	if len(parts) == 0 {
		parts = []string{text}
	}
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, style.Render(truncate(part, maxInt(10, width))))
	}
	return strings.Join(lines, "\n")
}

func (m *trackerPanelModel) renderContextLine() string {
	parts := []string{}
	if strings.TrimSpace(m.currentCtx.SessionName) != "" {
		parts = append(parts, strings.TrimSpace(m.currentCtx.SessionName))
	}
	if strings.TrimSpace(m.currentCtx.WindowName) != "" {
		parts = append(parts, strings.TrimSpace(m.currentCtx.WindowName))
	}
	if len(parts) == 0 {
		return "No tmux context detected"
	}
	return strings.Join(parts, "  ·  ")
}

func (m *trackerPanelModel) renderMetricsLine() string {
	active := 0
	review := 0
	for _, task := range m.state.Tasks {
		switch task.Status {
		case trackerTaskStatusInProgress:
			active++
		case trackerTaskStatusCompleted:
			if !task.Acknowledged {
				review++
			}
		}
	}
	return fmt.Sprintf("%d live  ·  %d review", active, review)
}

func (m *trackerPanelModel) currentStatus() string {
	if text := strings.TrimSpace(m.message); text != "" {
		return text
	}
	if text := strings.TrimSpace(m.state.Message); text != "" {
		return text
	}
	return "Tracker"
}

func (m *trackerPanelModel) syncCurrentContext() {
	if m.runtime == nil {
		return
	}
	m.currentCtx = m.runtime.currentTrackerContext()
}

func (m *trackerPanelModel) requestRefreshCmd() tea.Cmd {
	if m.refreshInFlight {
		m.pendingRefresh = true
		return nil
	}
	m.refreshInFlight = true
	return func() tea.Msg {
		env, err := trackerLoadState("")
		return trackerPanelStateMsg{env: env, err: err}
	}
}

func (m *trackerPanelModel) moveSelection(delta int) {
	list := m.visibleTasks()
	m.taskList.selected = clampInt(m.taskList.selected+delta, 0, maxInt(0, len(list)-1))
}

func (m *trackerPanelModel) runPrimaryAction() (tea.Model, tea.Cmd) {
	task := m.selectedTask()
	if task == nil {
		return m, nil
	}
	return m, trackerPanelCommandFunc(func() error {
		return focusTrackerTask(*task)
	}, true)
}

func (m *trackerPanelModel) toggleSelected() (tea.Model, tea.Cmd) {
	task := m.selectedTask()
	if task == nil {
		return m, nil
	}
	return m, trackerPanelCommandCmd(m.toggleTask(*task), "Task updated")
}

func (m *trackerPanelModel) deleteSelected() (tea.Model, tea.Cmd) {
	task := m.selectedTask()
	if task == nil {
		return m, nil
	}
	return m, trackerPanelCommandCmd(m.deleteTask(*task), "Task deleted")
}

func (m *trackerPanelModel) toggleTask(task ipc.Task) error {
	env := ipc.Envelope{Session: task.Session, SessionID: task.SessionID, Window: task.Window, WindowID: task.WindowID, Pane: task.Pane}
	command := "acknowledge"
	if task.Status == trackerTaskStatusInProgress {
		command = "finish_task"
	}
	return sendTrackerCommand(command, &env)
}

func (m *trackerPanelModel) deleteTask(task ipc.Task) error {
	env := ipc.Envelope{Session: task.Session, SessionID: task.SessionID, Window: task.Window, WindowID: task.WindowID, Pane: task.Pane}
	return sendTrackerCommand("delete_task", &env)
}

func (m *trackerPanelModel) clampSelections() {
	m.taskList.selected = clampInt(m.taskList.selected, 0, maxInt(0, len(m.visibleTasks())-1))
}

func (m *trackerPanelModel) visibleTasks() []ipc.Task {
	result := append([]ipc.Task(nil), m.state.Tasks...)
	trackerSortTasks(result)
	return result
}

func (m *trackerPanelModel) selectedTask() *ipc.Task {
	tasks := m.visibleTasks()
	if len(tasks) == 0 || m.taskList.selected < 0 || m.taskList.selected >= len(tasks) {
		return nil
	}
	task := tasks[m.taskList.selected]
	return &task
}

func (r *paletteRuntime) currentTrackerContext() trackerPanelContext {
	ctx := trackerPanelContext{
		SessionName: strings.TrimSpace(r.currentSessionName),
		WindowName:  strings.TrimSpace(r.currentWindowName),
		WindowID:    strings.TrimSpace(r.windowID),
	}
	args := []string{"display-message", "-p"}
	if strings.TrimSpace(r.windowID) != "" {
		args = append(args, "-t", strings.TrimSpace(r.windowID))
	}
	args = append(args, "#{session_name}:::#{session_id}:::#{window_name}:::#{window_id}:::#{pane_id}")
	out, err := runTmuxOutput(args...)
	if err != nil {
		return ctx
	}
	parts := strings.Split(strings.TrimSpace(out), ":::")
	if len(parts) != 5 {
		return ctx
	}
	ctx.SessionName = trackerFirstNonEmpty(strings.TrimSpace(parts[0]), ctx.SessionName)
	ctx.SessionID = strings.TrimSpace(parts[1])
	ctx.WindowName = trackerFirstNonEmpty(strings.TrimSpace(parts[2]), ctx.WindowName)
	ctx.WindowID = trackerFirstNonEmpty(strings.TrimSpace(parts[3]), ctx.WindowID)
	ctx.PaneID = strings.TrimSpace(parts[4])
	return ctx
}

func trackerPanelTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return trackerPanelTickMsg{} })
}

func trackerPanelCommandCmd(err error, message string) tea.Cmd {
	return func() tea.Msg { return trackerPanelCommandMsg{message: message, err: err} }
}

func trackerPanelCommandFunc(fn func() error, close bool) tea.Cmd {
	return func() tea.Msg { return trackerPanelCommandMsg{err: fn(), close: close} }
}

func trackerLoadState(client string) (*ipc.Envelope, error) {
	conn, err := net.Dial("unix", trackerSocketPath())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReader(conn))
	if err := enc.Encode(&ipc.Envelope{Kind: "ui-register", Client: strings.TrimSpace(client)}); err != nil {
		return nil, err
	}
	for {
		var env ipc.Envelope
		if err := dec.Decode(&env); err != nil {
			return nil, err
		}
		if env.Kind == "state" {
			return &env, nil
		}
	}
}

func sendTrackerCommand(command string, env *ipc.Envelope) error {
	conn, err := net.Dial("unix", trackerSocketPath())
	if err != nil {
		return err
	}
	defer conn.Close()
	request := ipc.Envelope{Kind: "command", Command: strings.TrimSpace(command)}
	if env != nil {
		request.Client = strings.TrimSpace(env.Client)
		request.Session = strings.TrimSpace(env.Session)
		request.SessionID = strings.TrimSpace(env.SessionID)
		request.Window = strings.TrimSpace(env.Window)
		request.WindowID = strings.TrimSpace(env.WindowID)
		request.Pane = strings.TrimSpace(env.Pane)
		request.Summary = strings.TrimSpace(env.Summary)
		request.Message = strings.TrimSpace(env.Message)
	}
	enc := json.NewEncoder(conn)
	if err := enc.Encode(&request); err != nil {
		return err
	}
	dec := json.NewDecoder(bufio.NewReader(conn))
	for {
		var reply ipc.Envelope
		if err := dec.Decode(&reply); err != nil {
			return err
		}
		if reply.Kind == "ack" {
			return nil
		}
	}
}

func trackerSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "agent-tracker.sock")
	}
	return filepath.Join(os.TempDir(), "agent-tracker.sock")
}

func trackerSortTasks(tasks []ipc.Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		left, right := tasks[i], tasks[j]
		leftRank, rightRank := trackerTaskStatusRank(left.Status), trackerTaskStatusRank(right.Status)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		switch left.Status {
		case trackerTaskStatusInProgress:
			return left.StartedAt < right.StartedAt
		case trackerTaskStatusCompleted:
			if left.Acknowledged != right.Acknowledged {
				return !left.Acknowledged && right.Acknowledged
			}
			li, hasLi := trackerParseTimestamp(left.CompletedAt)
			rj, hasRj := trackerParseTimestamp(right.CompletedAt)
			if hasLi && hasRj && !li.Equal(rj) {
				return li.After(rj)
			}
			if hasLi != hasRj {
				return hasLi
			}
		}
		li, hasLi := trackerParseTimestamp(left.StartedAt)
		rj, hasRj := trackerParseTimestamp(right.StartedAt)
		if hasLi && hasRj && !li.Equal(rj) {
			return li.After(rj)
		}
		if hasLi != hasRj {
			return hasLi
		}
		return left.Summary < right.Summary
	})
}

func trackerTaskStatusRank(status string) int {
	switch status {
	case trackerTaskStatusInProgress:
		return 0
	case trackerTaskStatusCompleted:
		return 1
	default:
		return 2
	}
}

func trackerParseTimestamp(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func trackerTaskIndicator(task ipc.Task, now time.Time) string {
	switch task.Status {
	case trackerTaskStatusInProgress:
		frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
		return string(frames[int(now.UnixNano()/int64(100*time.Millisecond))%len(frames)])
	case trackerTaskStatusCompleted:
		if task.Acknowledged {
			return "✓"
		}
		return "⚑"
	default:
		return "•"
	}
}

func trackerLiveDuration(task ipc.Task, now time.Time) string {
	start, ok := trackerParseTimestamp(task.StartedAt)
	if !ok {
		return trackerFormatDuration(task.DurationSeconds)
	}
	if task.Status == trackerTaskStatusCompleted {
		if end, ok := trackerParseTimestamp(task.CompletedAt); ok {
			return trackerFormatDuration(end.Sub(start).Seconds())
		}
		return trackerFormatDuration(task.DurationSeconds)
	}
	return trackerFormatDuration(now.Sub(start).Seconds())
}

func trackerFormatDuration(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	d := time.Duration(seconds * float64(time.Second))
	if d >= 99*time.Hour {
		return ">=99h"
	}
	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	secondsPart := (d % time.Minute) / time.Second
	if hours > 0 {
		return fmt.Sprintf("%02dh%02dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%02dm%02ds", minutes, secondsPart)
	}
	return fmt.Sprintf("%02ds", secondsPart)
}

func focusTrackerTask(task ipc.Task) error {
	if strings.TrimSpace(task.SessionID) == "" {
		return fmt.Errorf("session required to focus task")
	}
	if err := runTmux("switch-client", "-t", strings.TrimSpace(task.SessionID)); err != nil {
		return err
	}
	if strings.TrimSpace(task.WindowID) != "" {
		if err := runTmux("select-window", "-t", strings.TrimSpace(task.WindowID)); err != nil {
			return err
		}
	}
	if strings.TrimSpace(task.Pane) != "" {
		if err := runTmux("select-pane", "-t", strings.TrimSpace(task.Pane)); err != nil {
			return err
		}
	}
	return nil
}

func trackerFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
