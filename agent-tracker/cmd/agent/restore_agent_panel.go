package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type restoreAgentPanelModel struct {
	items        []restoreAgentItem
	repoRoot     string
	filter       []rune
	filterCursor int
	selected     int
	width        int
	height       int
	status       string
	statusUntil  time.Time
	requestBack  bool
	requestDone  bool
	chosen       string
}

func newRestoreAgentPanelModel(repoRoot string) *restoreAgentPanelModel {
	m := &restoreAgentPanelModel{repoRoot: strings.TrimSpace(repoRoot)}
	m.reload()
	return m
}

func (m *restoreAgentPanelModel) reload() {
	m.items = restorableAgentItems(m.repoRoot)
	if m.selected >= len(m.items) {
		m.selected = 0
	}
}

func (m *restoreAgentPanelModel) Init() tea.Cmd {
	return nil
}

func (m *restoreAgentPanelModel) handleKey(key string) {
	if key == "esc" {
		m.requestBack = true
		return
	}
	if key == "enter" {
		items := m.filteredItems()
		if len(items) > 0 && m.selected >= 0 && m.selected < len(items) {
			m.chosen = items[m.selected].id
			m.requestDone = true
			return
		}
		m.setStatus("No matching agent", 1500*time.Millisecond)
		return
	}
	if key == "ctrl+u" || key == "alt+u" || key == "up" {
		items := m.filteredItems()
		if len(items) > 0 {
			m.selected = clampInt(m.selected-1, 0, len(items)-1)
		}
		return
	}
	if key == "ctrl+e" || key == "alt+e" || key == "down" {
		items := m.filteredItems()
		if len(items) > 0 {
			next := m.selected + 1
			if next >= len(items) {
				next = 0
			}
			m.selected = next
		}
		return
	}
	if applyPaletteInputKey(key, &m.filter, &m.filterCursor, false) {
		m.selected = 0
	}
}

func (m *restoreAgentPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		m.handleKey(msg.String())
	}
	return m, nil
}

func (m *restoreAgentPanelModel) filteredItems() []restoreAgentItem {
	query := strings.ToLower(strings.TrimSpace(string(m.filter)))
	if query == "" {
		return m.items
	}
	var out []restoreAgentItem
	for _, it := range m.items {
		if strings.Contains(strings.ToLower(it.id), query) || strings.Contains(strings.ToLower(it.title), query) {
			out = append(out, it)
		}
	}
	return out
}

func (m *restoreAgentPanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func (m *restoreAgentPanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	header := styles.title.Render("Restore agent")

	filterLine := styles.searchBox.Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			styles.searchPrompt.Render(">"),
			" ",
			styles.input.Render(renderInputValue(m.filter, m.filterCursor, styles)),
		),
	)

	items := m.filteredItems()
	lines := []string{styles.meta.Render(fmt.Sprintf("%d agents", len(items))), ""}
	if len(items) == 0 {
		lines = append(lines, styles.muted.Width(width).Render("No restorable agents"))
	} else {
		innerWidth := maxInt(20, width-2)
		for idx, item := range items {
			rowStyle := styles.item.Width(maxInt(24, width-2))
			idStyle := styles.itemTitle
			subStyle := styles.muted
			if idx == m.selected {
				selectedBG := lipgloss.Color("238")
				rowStyle = styles.selectedItem.Width(maxInt(24, width-2))
				idStyle = idStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
				subStyle = subStyle.Background(selectedBG)
			}
			idText := truncate(item.id, innerWidth)
			row := idStyle.Render(idText)
			if strings.TrimSpace(item.title) != "" {
				subText := truncate(item.title, maxInt(8, innerWidth-2))
				row = lipgloss.JoinVertical(lipgloss.Left,
					row,
					subStyle.Render(subText),
				)
			}
			lines = append(lines, rowStyle.Render(row))
		}
	}

	bodyHeight := maxInt(8, height-8)
	body := lipgloss.NewStyle().Height(bodyHeight).Render(strings.Join(lines, "\n"))
	footer := m.renderFooter(styles, width)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", filterLine, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *restoreAgentPanelModel) renderFooter(styles paletteStyles, width int) string {
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "   ", pairs)
	}
	footer := pickRenderedShortcutFooter(width, renderSegments,
		[][2]string{{"Enter", "restore"}, {"u/e", "move"}, {"Esc", "back"}},
	)
	status := strings.TrimSpace(m.currentStatus())
	if status != "" {
		statusText := styles.statusBad.Render(truncate(status, maxInt(12, minInt(24, width/3))))
		if lipgloss.Width(footer)+2+lipgloss.Width(statusText) <= width {
			gap := width - lipgloss.Width(footer) - lipgloss.Width(statusText)
			if gap < 2 {
				gap = 2
			}
			return footer + strings.Repeat(" ", gap) + statusText
		}
		return statusText
	}
	return lipgloss.NewStyle().Width(width).Render(footer)
}

func (m *restoreAgentPanelModel) setStatus(text string, duration time.Duration) {
	m.status = text
	m.statusUntil = time.Now().Add(duration)
}

func (m *restoreAgentPanelModel) currentStatus() string {
	if m.status == "" {
		return ""
	}
	if !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
		m.status = ""
		return ""
	}
	return m.status
}
