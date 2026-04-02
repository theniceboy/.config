package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type statusRightPanelEntry struct {
	Module   string
	Title    string
	Subtitle string
	Enabled  bool
}

type statusRightPanelModel struct {
	entries      []statusRightPanelEntry
	selected     int
	width        int
	height       int
	status       string
	statusUntil  time.Time
	showAltHints bool
	requestBack  bool
}

func newStatusRightPanelModel() *statusRightPanelModel {
	model := &statusRightPanelModel{}
	model.reload()
	return model
}

func (m *statusRightPanelModel) reload() {
	entries := make([]statusRightPanelEntry, 0, len(statusRightModules()))
	for _, module := range statusRightModules() {
		entries = append(entries, statusRightPanelEntry{
			Module:   module,
			Title:    statusRightModuleLabel(module),
			Subtitle: capitalizeStatusRightDescription(statusRightModuleDescription(module)),
			Enabled:  statusRightModuleEnabled(module),
		})
	}
	m.entries = entries
	m.selected = clampInt(m.selected, 0, maxInt(0, len(m.entries)-1))
}

func capitalizeStatusRightDescription(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (m *statusRightPanelModel) Init() tea.Cmd {
	return nil
}

func (m *statusRightPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		switch msg.String() {
		case "esc":
			m.requestBack = true
		case "ctrl+u", "alt+u", "up", "u":
			m.selected = clampInt(m.selected-1, 0, maxInt(0, len(m.entries)-1))
		case "ctrl+e", "alt+e", "down", "e":
			m.selected = clampInt(m.selected+1, 0, maxInt(0, len(m.entries)-1))
		case "enter", " ":
			m.toggleSelected()
		}
	}
	return m, nil
}

func (m *statusRightPanelModel) toggleSelected() {
	entry, ok := m.currentEntry()
	if !ok {
		return
	}
	if err := togglePaletteStatusRightModule(entry.Module); err != nil {
		m.setStatus(err.Error(), 1500*time.Millisecond)
		return
	}
	m.reload()
	updated, ok := m.currentEntry()
	if !ok {
		return
	}
	verb := "disabled"
	if updated.Enabled {
		verb = "enabled"
	}
	m.setStatus(fmt.Sprintf("%s %s", updated.Title, verb), 1500*time.Millisecond)
}

func (m *statusRightPanelModel) currentEntry() (statusRightPanelEntry, bool) {
	if len(m.entries) == 0 || m.selected < 0 || m.selected >= len(m.entries) {
		return statusRightPanelEntry{}, false
	}
	return m.entries[m.selected], true
}

func (m *statusRightPanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func (m *statusRightPanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	header := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("Bottom-right Status"),
		styles.meta.Render("Interactive control center for tmux status-right modules"),
		styles.meta.Render("Current layout: "+truncate(m.layoutSummary(), maxInt(28, width-2))),
	)

	lines := []string{styles.meta.Render(fmt.Sprintf("%d modules", len(m.entries))), ""}
	for idx, entry := range m.entries {
		rowStyle := styles.item.Width(maxInt(24, width-2))
		titleStyle := styles.itemTitle
		metaStyle := styles.itemSubtitle
		detailStyle := styles.meta
		fillStyle := lipgloss.NewStyle()
		badgeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("241")).Padding(0, 1).Bold(true)
		badgeLabel := "OFF"
		if entry.Enabled {
			badgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("150")).Padding(0, 1).Bold(true)
			badgeLabel = "ON"
		}
		if idx == m.selected {
			selectedBG := lipgloss.Color("238")
			rowStyle = styles.selectedItem.Width(maxInt(24, width-2))
			titleStyle = titleStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
			metaStyle = styles.selectedSubtle.Background(selectedBG)
			detailStyle = styles.selectedSubtle.Background(selectedBG)
			fillStyle = fillStyle.Background(selectedBG)
		}
		badge := badgeStyle.Render(badgeLabel)
		innerWidth := maxInt(22, width-2)
		titleText := truncate(entry.Title, maxInt(8, innerWidth-lipgloss.Width(badge)-1))
		gapWidth := maxInt(1, innerWidth-lipgloss.Width(titleText)-lipgloss.Width(badge))
		titleRow := lipgloss.JoinHorizontal(lipgloss.Left,
			titleStyle.Render(titleText),
			fillStyle.Render(strings.Repeat(" ", gapWidth)),
			badge,
		)
		detail := entry.Subtitle + ". " + statusRightVisibilityText(entry.Enabled)
		detailText := truncate(detail, innerWidth)
		detailGap := maxInt(0, innerWidth-lipgloss.Width(detailText))
		detailRow := lipgloss.JoinHorizontal(lipgloss.Left,
			metaStyle.Render(detailText),
			fillStyle.Render(strings.Repeat(" ", detailGap)),
		)
		orderText := truncate(statusRightOrderHint(entry.Module, entry.Enabled), innerWidth)
		orderGap := maxInt(0, innerWidth-lipgloss.Width(orderText))
		orderRow := lipgloss.JoinHorizontal(lipgloss.Left,
			detailStyle.Render(orderText),
			fillStyle.Render(strings.Repeat(" ", orderGap)),
		)
		lines = append(lines, rowStyle.Render(lipgloss.JoinVertical(lipgloss.Left, titleRow, detailRow, orderRow)))
	}
	bodyHeight := maxInt(8, height-7)
	body := lipgloss.NewStyle().Height(bodyHeight).Render(strings.Join(lines, "\n"))
	footer := m.renderFooter(styles, width)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func statusRightVisibilityText(enabled bool) string {
	if enabled {
		return "Visible in the tmux bottom-right status"
	}
	return "Hidden from the tmux bottom-right status"
}

func statusRightOrderHint(module string, enabled bool) string {
	prefix := "Order slot: " + statusRightModuleLabel(module)
	if enabled {
		return prefix + " follows the fixed module order"
	}
	return prefix + " stays reserved until re-enabled"
}

func (m *statusRightPanelModel) layoutSummary() string {
	labels := make([]string, 0, len(m.entries))
	for _, entry := range m.entries {
		if entry.Enabled {
			labels = append(labels, entry.Title)
		}
	}
	if len(labels) == 0 {
		return "nothing enabled"
	}
	return strings.Join(labels, "  ->  ")
}

func (m *statusRightPanelModel) renderFooter(styles paletteStyles, width int) string {
	status := strings.TrimSpace(m.currentStatus())
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "   ", pairs)
	}
	footer := ""
	if m.showAltHints {
		footer = pickRenderedShortcutFooter(width, renderSegments,
			[][2]string{{"Space", "toggle"}, {"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
			[][2]string{{"Space", "toggle"}, {"Alt-S", "close"}},
		)
	} else {
		footer = pickRenderedShortcutFooter(width, renderSegments,
			[][2]string{{"u/e", "move"}, {"Enter", "toggle"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
			[][2]string{{"u/e", "move"}, {"Enter", "toggle"}, {footerHintToggleKey, "more"}},
			[][2]string{{"Esc", "back"}, {footerHintToggleKey, "more"}},
		)
	}
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

func (m *statusRightPanelModel) setStatus(text string, duration time.Duration) {
	m.status = text
	m.statusUntil = time.Now().Add(duration)
}

func (m *statusRightPanelModel) currentStatus() string {
	if m.status == "" {
		return ""
	}
	if !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
		m.status = ""
		return ""
	}
	return m.status
}
