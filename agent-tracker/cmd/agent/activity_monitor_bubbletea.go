package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type activityMonitorBT struct {
	windowID         string
	embedded         bool
	sortKey          activitySortKey
	sortDescending   bool
	selectedPID      int
	selectedRow      int
	rowOffset        int
	lockSelection    bool
	showAllProcesses bool
	processes        map[int]*activityProcess
	rows             []activityRow
	memoryByPID      map[int]activityMemory
	networkByPID     map[int]activityNetwork
	portsByPID       map[int][]string
	tmuxByPanePID    map[int]*activityTmuxLocation
	lastProcessLoad  time.Time
	lastMemoryLoad   time.Time
	lastNetworkLoad  time.Time
	lastPortLoad     time.Time
	lastTmuxLoad     time.Time
	refreshedAt      time.Time
	status           string
	statusUntil      time.Time
	confirmKillPID   int
	copyOptions      []activityCopyOption
	copyOptionIndex  int
	width            int
	height           int
	refreshInFlight  bool
	pendingRefresh   bool
	pendingForce     bool
	showAltHints     bool
	requestBack      bool
	requestClose     bool
	styles           activityStyles
}

type activityStyles struct {
	title        lipgloss.Style
	meta         lipgloss.Style
	header       lipgloss.Style
	headerActive lipgloss.Style
	row          lipgloss.Style
	rowSelected  lipgloss.Style
	cell         lipgloss.Style
	cellSelected lipgloss.Style
	detailTitle  lipgloss.Style
	detailLabel  lipgloss.Style
	detailValue  lipgloss.Style
	muted        lipgloss.Style
	footer       lipgloss.Style
	status       lipgloss.Style
	statusBad    lipgloss.Style
	divider      lipgloss.Style
	shortcutKey  lipgloss.Style
	shortcutText lipgloss.Style
	modal        lipgloss.Style
	modalTitle   lipgloss.Style
	modalBody    lipgloss.Style
	modalHint    lipgloss.Style
}

type activityTickMsg struct{}

type activityRefreshMsg struct {
	snapshot *activitySnapshot
	err      error
}

type activityCopyOption struct {
	Label string
	Value string
}

var activityClipboardWriter = writeActivityClipboard

func newActivityStyles() activityStyles {
	accent := lipgloss.Color("223")
	cyan := lipgloss.Color("117")
	selected := lipgloss.Color("238")
	text := lipgloss.Color("252")
	muted := lipgloss.Color("245")
	bright := lipgloss.Color("230")
	warning := lipgloss.Color("203")
	success := lipgloss.Color("150")
	return activityStyles{
		title:        lipgloss.NewStyle().Bold(true).Foreground(bright),
		meta:         lipgloss.NewStyle().Foreground(muted),
		header:       lipgloss.NewStyle().Bold(true).Foreground(cyan),
		headerActive: lipgloss.NewStyle().Bold(true).Foreground(accent).Background(selected),
		row:          lipgloss.NewStyle().Padding(0, 1),
		rowSelected:  lipgloss.NewStyle().Background(selected).Padding(0, 1),
		cell:         lipgloss.NewStyle().Foreground(text),
		cellSelected: lipgloss.NewStyle().Foreground(bright).Background(selected),
		detailTitle:  lipgloss.NewStyle().Bold(true).Foreground(accent),
		detailLabel:  lipgloss.NewStyle().Foreground(cyan),
		detailValue:  lipgloss.NewStyle().Foreground(text),
		muted:        lipgloss.NewStyle().Foreground(muted),
		footer:       lipgloss.NewStyle().Foreground(lipgloss.Color("216")),
		status:       lipgloss.NewStyle().Foreground(success),
		statusBad:    lipgloss.NewStyle().Foreground(warning),
		divider:      lipgloss.NewStyle().Foreground(lipgloss.Color("239")),
		shortcutKey:  lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(accent).Padding(0, 1).Bold(true),
		shortcutText: lipgloss.NewStyle().Foreground(muted),
		modal:        lipgloss.NewStyle().Border(paletteModalBorder).BorderForeground(accent).Padding(1, 2).Background(lipgloss.Color("235")),
		modalTitle:   lipgloss.NewStyle().Bold(true).Foreground(warning),
		modalBody:    lipgloss.NewStyle().Foreground(text),
		modalHint:    lipgloss.NewStyle().Foreground(muted),
	}
}

func newActivityMonitorModel(windowID string, embedded bool) *activityMonitorBT {
	model := &activityMonitorBT{
		windowID:         strings.TrimSpace(windowID),
		embedded:         embedded,
		sortKey:          activitySortCPU,
		sortDescending:   true,
		showAllProcesses: true,
		processes:        map[int]*activityProcess{},
		memoryByPID:      map[int]activityMemory{},
		networkByPID:     map[int]activityNetwork{},
		portsByPID:       map[int][]string{},
		tmuxByPanePID:    map[int]*activityTmuxLocation{},
		styles:           newActivityStyles(),
	}
	model.setStatus("Loading...", 0)
	return model
}

func runBubbleTeaActivityMonitor(windowID string) error {
	model := newActivityMonitorModel(windowID, false)
	_, err := tea.NewProgram(model).Run()
	return err
}

func (m *activityMonitorBT) Init() tea.Cmd {
	return tea.Batch(
		activityRequestRefreshBT(true, true, m),
		activityTickCmd(),
	)
}

func (m *activityMonitorBT) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case activityTickMsg:
		return m, tea.Batch(
			activityTickCmd(),
			activityRequestRefreshBT(false, false, m),
		)
	case activityRefreshMsg:
		m.refreshInFlight = false
		if msg.err != nil {
			m.setStatus(msg.err.Error(), 5*time.Second)
		} else if msg.snapshot != nil {
			m.applySnapshot(msg.snapshot)
			if strings.TrimSpace(m.status) == "Loading..." {
				m.setStatus("", 0)
			}
		}
		if m.pendingForce || m.pendingRefresh {
			force := m.pendingForce
			m.pendingForce = false
			m.pendingRefresh = false
			return m, activityRequestRefreshBT(force, false, m)
		}
		return m, nil
	case tea.KeyMsg:
		if isAltFooterToggleKey(msg) {
			m.showAltHints = !m.showAltHints
			return m, nil
		}
		m.showAltHints = false
		key := msg.String()
		if key == "alt+s" {
			if m.embedded {
				m.requestClose = true
				return m, nil
			}
			return m, tea.Quit
		}
		if len(m.copyOptions) > 0 {
			return m.updateCopyMenu(key)
		}
		if m.confirmKillPID != 0 {
			return m.updateConfirmKill(key)
		}
		return m.updateNormal(key)
	}
	return m, nil
}

func (m *activityMonitorBT) updateConfirmKill(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "n" || key == "N" {
		m.confirmKillPID = 0
		return m, nil
	}
	if key == "y" || key == "Y" || key == "enter" {
		pid := m.confirmKillPID
		m.confirmKillPID = 0
		if err := m.killProcess(pid); err != nil {
			m.setStatus(err.Error(), 5*time.Second)
		}
		return m, activityRequestRefreshBT(true, false, m)
	}
	m.confirmKillPID = 0
	return m, nil
}

func (m *activityMonitorBT) updateCopyMenu(key string) (tea.Model, tea.Cmd) {
	if len(m.copyOptions) == 0 {
		return m, nil
	}
	switch key {
	case "esc", "n", "N":
		m.copyOptions = nil
		m.copyOptionIndex = 0
		return m, nil
	case "u", "up", "ctrl+u":
		m.copyOptionIndex = clampInt(m.copyOptionIndex-1, 0, len(m.copyOptions)-1)
		return m, nil
	case "e", "down", "ctrl+e":
		m.copyOptionIndex = clampInt(m.copyOptionIndex+1, 0, len(m.copyOptions)-1)
		return m, nil
	case "enter", "y", "Y":
		option := m.copyOptions[clampInt(m.copyOptionIndex, 0, len(m.copyOptions)-1)]
		if err := activityClipboardWriter(option.Value); err != nil {
			m.setStatus(err.Error(), 5*time.Second)
			return m, nil
		}
		m.copyOptions = nil
		m.copyOptionIndex = 0
		m.setStatus("Copied "+strings.ToLower(strings.TrimSpace(option.Label)), 4*time.Second)
		return m, nil
	}
	if idx, ok := activityCopyIndexForKey(key, len(m.copyOptions)); ok {
		option := m.copyOptions[idx]
		if err := activityClipboardWriter(option.Value); err != nil {
			m.setStatus(err.Error(), 5*time.Second)
			return m, nil
		}
		m.copyOptions = nil
		m.copyOptionIndex = 0
		m.setStatus("Copied "+strings.ToLower(strings.TrimSpace(option.Label)), 4*time.Second)
	}
	return m, nil
}

func (m *activityMonitorBT) updateNormal(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "ctrl+c" {
		if m.embedded {
			m.requestBack = true
			return m, nil
		}
		return m, tea.Quit
	}
	switch key {
	case "u", "up":
		m.moveSelection(-1)
	case "e", "down":
		m.moveSelection(1)
	case "n", "left":
		m.shiftSort(-1)
	case "i", "right":
		m.shiftSort(1)
	case "a":
		m.showAllProcesses = !m.showAllProcesses
		m.resortRows()
		return m, activityRequestRefreshBT(true, false, m)
	case "l":
		m.lockSelection = !m.lockSelection
		if !m.lockSelection {
			m.resortRows()
		}
	case "r":
		m.sortDescending = !m.sortDescending
		m.resortRows()
	case "c":
		m.setSort(activitySortCPU)
	case "m":
		m.setSort(activitySortMemory)
	case "j":
		m.setSort(activitySortDownload)
	case "k":
		m.setSort(activitySortUpload)
	case "o":
		m.setSort(activitySortPorts)
	case "t", "w":
		m.setSort(activitySortLocation)
	case "f":
		m.setSort(activitySortCommand)
	case "enter", "g":
		proc := m.selectedProcess()
		if proc == nil || proc.Tmux == nil {
			m.setStatus("Not in tmux", 3*time.Second)
			return m, nil
		}
		if err := focusActivityTmuxLocation(proc.Tmux); err != nil {
			m.setStatus(err.Error(), 4*time.Second)
			return m, nil
		}
		if m.embedded {
			m.requestClose = true
			return m, nil
		}
		return m, tea.Quit
	case "d", "D":
		if proc := m.selectedProcess(); proc != nil {
			m.confirmKillPID = proc.PID
		}
	case "y", "Y":
		proc := m.selectedProcess()
		if proc == nil {
			m.setStatus("No process selected", 3*time.Second)
			return m, nil
		}
		m.copyOptions = activityCopyOptions(proc)
		m.copyOptionIndex = 0
		if len(m.copyOptions) == 0 {
			m.setStatus("Nothing available to copy", 3*time.Second)
		}
	}
	return m, nil
}

func (m *activityMonitorBT) View() string {
	w := m.width
	h := m.height
	if w < 72 || h < 14 {
		return m.styles.title.Render("Window too small")
	}

	title := fmt.Sprintf("Activity Monitor  %d processes", len(m.rows))
	headerLine := m.styles.title.Render(title)

	scope := "tmux"
	if m.showAllProcesses {
		scope = "all"
	}
	dir := "asc"
	if m.sortDescending {
		dir = "desc"
	}
	follow := "row"
	if m.lockSelection {
		follow = "item"
	}
	metaLine := m.styles.meta.Render(fmt.Sprintf("Scope %s  Sort %s %s  Follow %s", scope, m.sortKey.label(), dir, follow))

	tableX := 1
	tableY := 3
	contentH := h - tableY - 1
	tableW := w - 2
	previewX := 0
	previewW := 0
	showPreview := w >= 108
	if showPreview {
		tableW = maxInt(70, (w-3)*68/100)
		previewX = tableX + tableW + 1
		previewW = w - previewX - 1
	}

	tableContent := m.renderTable(tableW, contentH)
	left := lipgloss.NewStyle().Width(tableW).Height(contentH).Render(tableContent)

	body := left
	if showPreview {
		divider := m.renderVerticalDivider(contentH)
		rightContent := m.renderDetails(previewW, contentH)
		right := lipgloss.NewStyle().Width(previewW).Height(contentH).Render(rightContent)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
	}

	footer := m.renderFooter(w)

	view := lipgloss.JoinVertical(lipgloss.Left,
		headerLine,
		metaLine,
		"",
		body,
		"",
		footer,
	)

	result := lipgloss.NewStyle().Width(w).Height(h).Padding(0, 1).Render(view)

	if m.confirmKillPID != 0 {
		procName := fmt.Sprintf("PID %d", m.confirmKillPID)
		if proc := m.processes[m.confirmKillPID]; proc != nil {
			procName = fmt.Sprintf("%s (PID %d)", proc.ShortCommand, proc.PID)
		}
		modal := lipgloss.JoinVertical(lipgloss.Left,
			m.styles.modalTitle.Render("Kill process"),
			m.styles.modalBody.Render(procName),
			"",
			m.styles.modalHint.Render("y confirm  n cancel"),
		)
		box := m.styles.modal.Render(modal)
		result = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box, lipgloss.WithWhitespaceChars(" "), lipgloss.WithWhitespaceBackground(lipgloss.Color("235")))
	} else if len(m.copyOptions) > 0 {
		rows := make([]string, 0, len(m.copyOptions))
		for idx, option := range m.copyOptions {
			prefix := fmt.Sprintf("%d ", idx+1)
			labelStyle := m.styles.modalBody
			rowStyle := lipgloss.NewStyle()
			if idx == clampInt(m.copyOptionIndex, 0, len(m.copyOptions)-1) {
				rowStyle = rowStyle.Background(lipgloss.Color("238"))
				labelStyle = labelStyle.Copy().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230"))
			}
			rows = append(rows, rowStyle.Render(prefix+labelStyle.Render(option.Label)))
		}
		modal := lipgloss.JoinVertical(lipgloss.Left,
			m.styles.modalTitle.Render("Copy"),
			m.styles.modalBody.Render("Choose a field from the selected process"),
			"",
			strings.Join(rows, "\n"),
			"",
			m.styles.modalHint.Render("u/e move  enter copy  1-9 direct  esc cancel"),
		)
		box := m.styles.modal.Width(minInt(48, maxInt(30, w-10))).Render(modal)
		result = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box, lipgloss.WithWhitespaceChars(" "), lipgloss.WithWhitespaceBackground(lipgloss.Color("235")))
	}

	return result
}

func (m *activityMonitorBT) renderTable(width, height int) string {
	cpuW, memW, downW, upW, procW, sessionW, windowW, portW := activityColumnWidths(width)

	headers := []struct {
		key   activitySortKey
		label string
		w     int
	}{
		{activitySortCPU, "CPU", cpuW},
		{activitySortMemory, "MEM", memW},
		{activitySortDownload, "DOWN", downW},
		{activitySortUpload, "UP", upW},
		{activitySortCommand, "PROCESS", procW},
		{activitySortLocation, "SESSION", sessionW},
		{activitySortLocation, "WIN", windowW},
		{activitySortPorts, "PORTS", portW},
	}

	headerParts := make([]string, len(headers))
	for i, h := range headers {
		style := m.styles.header
		label := h.label
		if h.key == m.sortKey {
			style = m.styles.headerActive
			if m.sortDescending {
				label += " ▼"
			} else {
				label += " ▲"
			}
		}
		headerParts[i] = style.Width(h.w).Render(truncate(label, h.w))
	}
	headerLine := lipgloss.JoinHorizontal(lipgloss.Left, headerParts...)

	visibleRows := maxInt(1, height-1)
	selectedIndex := m.selectedIndex()
	if selectedIndex < 0 {
		selectedIndex = 0
	}
	offset := stableListOffset(m.rowOffset, selectedIndex, visibleRows, len(m.rows))
	m.rowOffset = offset

	rowLines := []string{}
	for row := 0; row < visibleRows; row++ {
		idx := offset + row
		if idx >= len(m.rows) {
			break
		}
		info := m.rows[idx]
		proc := m.processes[info.PID]
		if proc == nil {
			continue
		}
		selected := proc.PID == m.selectedPID
		rowStyle := m.styles.row
		cellStyle := m.styles.cell
		if selected {
			rowStyle = m.styles.rowSelected
			cellStyle = m.styles.cellSelected
		}

		cpuStyle := cellStyle
		if proc.CPU >= 50 {
			cpuStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Background(cellStyle.GetBackground())
		} else if proc.CPU >= 20 {
			cpuStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("216")).Background(cellStyle.GetBackground())
		}

		memStyle := cellStyle
		totalMem := activityTotalMemoryMB(proc)
		if totalMem >= 1024 {
			memStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Background(cellStyle.GetBackground())
		} else if totalMem >= 512 {
			memStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("216")).Background(cellStyle.GetBackground())
		}

		cells := []string{
			cpuStyle.Width(cpuW).Render(truncate(fmt.Sprintf("%.1f", proc.CPU), cpuW)),
			memStyle.Width(memW).Render(truncate(formatActivityMB(totalMem), memW)),
			cellStyle.Width(downW).Render(truncate(formatActivitySpeed(proc.DownKBps), downW)),
			cellStyle.Width(upW).Render(truncate(formatActivitySpeed(proc.UpKBps), upW)),
			cellStyle.Width(procW).Render(truncate(activityProcessLabel(proc), procW)),
			cellStyle.Width(sessionW).Render(truncate(activitySessionLabel(proc.Tmux), sessionW)),
			cellStyle.Width(windowW).Render(truncate(activityWindowLabel(proc.Tmux, m.windowID), windowW)),
			cellStyle.Width(portW).Render(truncate(formatActivityPorts(proc.Ports), portW)),
		}
		rowLines = append(rowLines, rowStyle.Width(width).Render(lipgloss.JoinHorizontal(lipgloss.Left, cells...)))
	}

	if len(rowLines) == 0 {
		msg := "No tmux processes"
		if m.showAllProcesses {
			msg = "No processes"
		}
		return headerLine + "\n" + m.styles.muted.Width(width).Render(msg)
	}

	return headerLine + "\n" + strings.Join(rowLines, "\n")
}

func (m *activityMonitorBT) renderDetails(width, height int) string {
	lines := []string{m.styles.detailTitle.Render("Details")}

	proc := m.selectedProcess()
	if proc == nil {
		lines = append(lines, m.styles.muted.Render("No process selected"))
		return strings.Join(lines, "\n")
	}

	lines = append(lines,
		m.renderDetailRow("PID", fmt.Sprintf("%d", proc.PID), width),
		m.renderDetailRow("PPID", fmt.Sprintf("%d", proc.PPID), width),
		m.renderDetailRow("CPU", fmt.Sprintf("%.1f%%", proc.CPU), width),
		m.renderDetailRow("MEM", formatActivityMB(activityTotalMemoryMB(proc)), width),
		m.renderDetailRow("Download", formatActivitySpeed(proc.DownKBps), width),
		m.renderDetailRow("Upload", formatActivitySpeed(proc.UpKBps), width),
	)

	if proc.ResidentMB > 0 || proc.CompressedMB > 0 {
		lines = append(lines, m.renderDetailRow("Resident", formatActivityMB(proc.ResidentMB), width))
		if proc.CompressedMB > 0 {
			lines = append(lines, m.renderDetailRow("Compressed", formatActivityMB(proc.CompressedMB), width))
		}
	}

	lines = append(lines,
		m.renderDetailRow("State", blankIfEmpty(proc.State, "-"), width),
		m.renderDetailRow("Elapsed", blankIfEmpty(proc.Elapsed, "-"), width),
	)

	if proc.Tmux == nil {
		lines = append(lines, m.renderDetailRow("Tmux", "outside tmux", width))
	} else {
		lines = append(lines,
			m.renderDetailRow("Session", blankIfEmpty(proc.Tmux.SessionName, proc.Tmux.SessionID), width),
			m.renderDetailRow("Window", activityWindowLabel(proc.Tmux, m.windowID), width),
			m.renderDetailRow("Pane", proc.Tmux.PaneIndex, width),
		)
	}

	if len(proc.Ports) > 0 {
		lines = append(lines, m.renderDetailRow("Ports", strings.Join(proc.Ports, ", "), width))
	} else {
		lines = append(lines, m.renderDetailRow("Ports", "none", width))
	}

	lines = append(lines, "", m.styles.detailTitle.Render("Command"))
	cmdLines := wrapText(blankIfEmpty(proc.Command, proc.ShortCommand), width)
	for _, l := range cmdLines {
		lines = append(lines, m.styles.cell.Render(truncate(l, width)))
	}

	return clampActivityLines(lines, height, width, m.styles.muted)
}

func clampActivityLines(lines []string, height, width int, muted lipgloss.Style) string {
	if height <= 0 {
		return ""
	}
	if len(lines) <= height {
		return strings.Join(lines, "\n")
	}
	if height == 1 {
		return muted.Render(truncate("...", width))
	}
	clipped := append([]string(nil), lines[:height]...)
	clipped[height-1] = muted.Render(truncate("...", width))
	return strings.Join(clipped, "\n")
}

func (m *activityMonitorBT) renderDetailRow(label, value string, width int) string {
	labelW := 10
	return m.styles.detailLabel.Width(labelW).Render(label+":") + " " + m.styles.detailValue.Render(truncate(value, maxInt(10, width-labelW-2)))
}

func (m *activityMonitorBT) renderVerticalDivider(height int) string {
	lines := make([]string, maxInt(1, height))
	for i := range lines {
		lines[i] = m.styles.divider.Render("│")
	}
	return strings.Join(lines, "\n")
}

func (m *activityMonitorBT) renderFooter(width int) string {
	status := m.currentStatus()
	if status != "" {
		style := m.styles.status
		lower := strings.ToLower(status)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "refusing") {
			style = m.styles.statusBad
		}
		return style.Width(width).Render(truncate(status, width))
	}

	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return m.styles.shortcutKey.Render(v) }, func(v string) string { return m.styles.shortcutText.Render(v) }, "  ", pairs)
	}
	footer := ""
	if m.showAltHints {
		footer = pickRenderedShortcutFooter(width, renderSegments,
			[][2]string{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
			[][2]string{{"Alt-S", "close"}},
		)
	} else {
		footer = pickRenderedShortcutFooter(width, renderSegments,
			[][2]string{{"u/e", "move"}, {"n/i", "sort"}, {"j/k", "net"}, {"a", "scope"}, {"l", "follow"}, {"r", "reverse"}, {"Enter", "tmux"}, {"d", "kill"}, {"y", "copy"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
			[][2]string{{"u/e", "move"}, {"n/i", "sort"}, {"j/k", "net"}, {"a", "scope"}, {"Enter", "tmux"}, {"d", "kill"}, {"y", "copy"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
			[][2]string{{"u/e", "move"}, {"Enter", "tmux"}, {"y", "copy"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
		)
	}
	return lipgloss.NewStyle().Width(width).Render(footer)
}

func (m *activityMonitorBT) moveSelection(delta int) {
	if len(m.rows) == 0 {
		m.selectedPID = 0
		m.selectedRow = 0
		return
	}
	m.ensureSelection()
	idx := m.selectedRow + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.rows) {
		idx = len(m.rows) - 1
	}
	m.selectedRow = idx
	m.selectedPID = m.rows[m.selectedRow].PID
}

func (m *activityMonitorBT) shiftSort(delta int) {
	index := 0
	for idx, key := range activitySortKeys {
		if key == m.sortKey {
			index = idx
			break
		}
	}
	index += delta
	if index < 0 {
		index = len(activitySortKeys) - 1
	}
	if index >= len(activitySortKeys) {
		index = 0
	}
	key := activitySortKeys[index]
	if key == m.sortKey {
		return
	}
	m.sortKey = key
	m.sortDescending = defaultActivitySortDescending(key)
	m.resortRows()
}

func (m *activityMonitorBT) killProcess(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid")
	}
	if pid == os.Getpid() {
		return fmt.Errorf("refusing to kill self")
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return err
	}
	m.setStatus(fmt.Sprintf("Sent SIGTERM to %d", pid), 4*time.Second)
	return nil
}

func (m *activityMonitorBT) setSort(key activitySortKey) {
	if m.sortKey == key {
		m.sortDescending = !m.sortDescending
	} else {
		m.sortKey = key
		m.sortDescending = defaultActivitySortDescending(key)
	}
	m.resortRows()
}

func (m *activityMonitorBT) resortRows() {
	previousPID := m.selectedPID
	previousRow := m.selectedRow
	rows := make([]activityRow, 0, len(m.processes))
	for _, proc := range m.sortedProcesses(m.visibleProcesses()) {
		rows = append(rows, activityRow{PID: proc.PID})
	}
	m.rows = rows
	m.restoreSelection(previousPID, previousRow)
}

func (m *activityMonitorBT) visibleProcesses() []*activityProcess {
	visible := make([]*activityProcess, 0, len(m.processes))
	for _, proc := range m.processes {
		if !m.showAllProcesses && proc.Tmux == nil {
			continue
		}
		visible = append(visible, proc)
	}
	return visible
}

func (m *activityMonitorBT) sortedProcesses(values []*activityProcess) []*activityProcess {
	cloned := append([]*activityProcess(nil), values...)
	sort.SliceStable(cloned, func(i, j int) bool {
		return m.less(cloned[i], cloned[j])
	})
	return cloned
}

func (m *activityMonitorBT) less(left, right *activityProcess) bool {
	cmp := 0
	switch m.sortKey {
	case activitySortCPU:
		cmp = compareFloat64(left.CPU, right.CPU)
	case activitySortMemory:
		cmp = compareFloat64(activityTotalMemoryMB(left), activityTotalMemoryMB(right))
	case activitySortDownload:
		cmp = compareFloat64(left.DownKBps, right.DownKBps)
	case activitySortUpload:
		cmp = compareFloat64(left.UpKBps, right.UpKBps)
	case activitySortPorts:
		cmp = compareInt(len(left.Ports), len(right.Ports))
		if cmp == 0 {
			cmp = strings.Compare(strings.Join(left.Ports, ","), strings.Join(right.Ports, ","))
		}
	case activitySortLocation:
		cmp = compareActivityLocation(left.Tmux, right.Tmux)
	case activitySortCommand:
		cmp = strings.Compare(strings.ToLower(blankIfEmpty(left.ShortCommand, left.Command)), strings.ToLower(blankIfEmpty(right.ShortCommand, right.Command)))
	}
	if cmp == 0 {
		cmp = compareFloat64(left.CPU, right.CPU)
	}
	if cmp == 0 {
		cmp = compareFloat64(activityTotalMemoryMB(left), activityTotalMemoryMB(right))
	}
	if cmp == 0 {
		cmp = compareInt(left.PID, right.PID)
	}
	if m.sortDescending {
		return cmp > 0
	}
	return cmp < 0
}

func (m *activityMonitorBT) ensureSelection() {
	if len(m.rows) == 0 {
		m.selectedPID = 0
		m.selectedRow = 0
		return
	}
	if m.lockSelection && m.selectedPID != 0 {
		for idx, row := range m.rows {
			if row.PID == m.selectedPID {
				m.selectedRow = idx
				return
			}
		}
	}
	if m.selectedRow < 0 {
		m.selectedRow = 0
	}
	if m.selectedRow >= len(m.rows) {
		m.selectedRow = len(m.rows) - 1
	}
	m.selectedPID = m.rows[m.selectedRow].PID
}

func (m *activityMonitorBT) restoreSelection(previousPID, previousRow int) {
	if len(m.rows) == 0 {
		m.selectedPID = 0
		m.selectedRow = 0
		return
	}
	if m.lockSelection && previousPID != 0 {
		for idx, row := range m.rows {
			if row.PID == previousPID {
				m.selectedPID = previousPID
				m.selectedRow = idx
				return
			}
		}
	}
	if previousRow < 0 {
		previousRow = 0
	}
	if previousRow >= len(m.rows) {
		previousRow = len(m.rows) - 1
	}
	m.selectedRow = previousRow
	m.selectedPID = m.rows[m.selectedRow].PID
}

func (m *activityMonitorBT) selectedIndex() int {
	m.ensureSelection()
	if len(m.rows) == 0 {
		return -1
	}
	return m.selectedRow
}

func (m *activityMonitorBT) selectedProcess() *activityProcess {
	m.ensureSelection()
	if m.selectedPID == 0 {
		return nil
	}
	return m.processes[m.selectedPID]
}

func (m *activityMonitorBT) setStatus(text string, duration time.Duration) {
	m.status = strings.TrimSpace(text)
	if duration > 0 {
		m.statusUntil = time.Now().Add(duration)
	} else {
		m.statusUntil = time.Time{}
	}
}

func (m *activityMonitorBT) currentStatus() string {
	if m.status == "" {
		return ""
	}
	if !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
		m.status = ""
		m.statusUntil = time.Time{}
		return ""
	}
	return m.status
}

func (m *activityMonitorBT) applySnapshot(snapshot *activitySnapshot) {
	if snapshot == nil {
		return
	}
	m.processes = snapshot.Processes
	m.memoryByPID = snapshot.MemoryByPID
	m.networkByPID = snapshot.NetworkByPID
	m.portsByPID = snapshot.PortsByPID
	m.tmuxByPanePID = snapshot.TmuxByPanePID
	m.lastProcessLoad = snapshot.LastProcessLoad
	m.lastMemoryLoad = snapshot.LastMemoryLoad
	m.lastNetworkLoad = snapshot.LastNetworkLoad
	m.lastPortLoad = snapshot.LastPortLoad
	m.lastTmuxLoad = snapshot.LastTmuxLoad
	m.refreshedAt = snapshot.RefreshedAt
	m.resortRows()
	if strings.TrimSpace(snapshot.Status) != "" && m.currentStatus() == "" {
		m.setStatus(snapshot.Status, 4*time.Second)
	}
}

func activityTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return activityTickMsg{}
	})
}

func activityRequestRefreshBT(force, initial bool, m *activityMonitorBT) tea.Cmd {
	if m.refreshInFlight {
		if force || initial {
			m.pendingForce = true
		} else {
			m.pendingRefresh = true
		}
		return nil
	}
	m.refreshInFlight = true
	input := activityRefreshInput{
		Force:           force,
		Initial:         initial,
		ShowAll:         m.showAllProcesses,
		Processes:       cloneActivityProcesses(m.processes),
		MemoryByPID:     cloneActivityMemoryMap(m.memoryByPID),
		NetworkByPID:    cloneActivityNetworkMap(m.networkByPID),
		PortsByPID:      cloneActivityPortsMap(m.portsByPID),
		TmuxByPanePID:   cloneActivityTmuxMap(m.tmuxByPanePID),
		LastProcessLoad: m.lastProcessLoad,
		LastMemoryLoad:  m.lastMemoryLoad,
		LastNetworkLoad: m.lastNetworkLoad,
		LastPortLoad:    m.lastPortLoad,
		LastTmuxLoad:    m.lastTmuxLoad,
		RefreshedAt:     m.refreshedAt,
	}
	return func() tea.Msg {
		snapshot, err := collectActivitySnapshot(input)
		return activityRefreshMsg{snapshot: snapshot, err: err}
	}
}

func activityCopyOptions(proc *activityProcess) []activityCopyOption {
	if proc == nil {
		return nil
	}
	options := []activityCopyOption{
		{Label: "PID", Value: fmt.Sprintf("%d", proc.PID)},
		{Label: "Parent PID", Value: fmt.Sprintf("%d", proc.PPID)},
		{Label: "Process", Value: blankIfEmpty(proc.ShortCommand, proc.Command)},
		{Label: "Command", Value: blankIfEmpty(proc.Command, proc.ShortCommand)},
		{Label: "Download", Value: formatActivitySpeed(proc.DownKBps)},
		{Label: "Upload", Value: formatActivitySpeed(proc.UpKBps)},
	}
	if proc.Tmux != nil {
		session := strings.TrimSpace(blankIfEmpty(proc.Tmux.SessionName, proc.Tmux.SessionID))
		if session != "" {
			if strings.TrimSpace(proc.Tmux.SessionID) != "" && session != strings.TrimSpace(proc.Tmux.SessionID) {
				session += " (" + strings.TrimSpace(proc.Tmux.SessionID) + ")"
			}
			options = append(options, activityCopyOption{Label: "Session", Value: session})
		}
		window := activityWindowLabel(proc.Tmux, "")
		if strings.TrimSpace(window) != "-" && strings.TrimSpace(window) != "" {
			if strings.TrimSpace(proc.Tmux.WindowID) != "" {
				window += " (" + strings.TrimSpace(proc.Tmux.WindowID) + ")"
			}
			options = append(options, activityCopyOption{Label: "Window", Value: window})
		}
		if pane := strings.TrimSpace(proc.Tmux.PaneID); pane != "" {
			options = append(options, activityCopyOption{Label: "Pane", Value: pane})
		}
	}
	if len(proc.Ports) > 0 {
		options = append(options, activityCopyOption{Label: "Ports", Value: strings.Join(proc.Ports, ", ")})
	}
	filtered := make([]activityCopyOption, 0, len(options))
	for _, option := range options {
		if strings.TrimSpace(option.Value) == "" || strings.TrimSpace(option.Value) == "-" {
			continue
		}
		filtered = append(filtered, option)
	}
	return filtered
}

func activityCopyIndexForKey(key string, total int) (int, bool) {
	if total <= 0 {
		return 0, false
	}
	runes := []rune(strings.TrimSpace(key))
	if len(runes) != 1 {
		return 0, false
	}
	if runes[0] < '1' || runes[0] > '9' {
		return 0, false
	}
	idx := int(runes[0] - '1')
	if idx < 0 || idx >= total {
		return 0, false
	}
	return idx, true
}

func writeActivityClipboard(value string) error {
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
