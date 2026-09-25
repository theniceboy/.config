package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type deviceSwitchPanelModel struct {
	devices      []string
	current      string
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

func newDeviceSwitchPanelModel(current string) *deviceSwitchPanelModel {
	m := &deviceSwitchPanelModel{
		current: strings.TrimSpace(current),
	}
	m.reload()
	return m
}

func (m *deviceSwitchPanelModel) reload() {
	m.devices = loadManagedDevices()
}

func (m *deviceSwitchPanelModel) Init() tea.Cmd {
	return nil
}

func (m *deviceSwitchPanelModel) handleKey(key string) {
	if key == "esc" {
		m.requestBack = true
		return
	}
	if key == "enter" || key == "alt+i" {
		items := m.filteredItems()
		if len(items) > 0 && m.selected >= 0 && m.selected < len(items) {
			m.chosen = items[m.selected].deviceID
			m.requestDone = true
			return
		}
		custom := normalizeManagedDeviceID(string(m.filter))
		if custom != "" {
			m.chosen = custom
			m.requestDone = true
			return
		}
		m.setStatus("Type or select a device", 1500*time.Millisecond)
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

func (m *deviceSwitchPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

type deviceSwitchItem struct {
	deviceID string
	custom   bool
}

func (m *deviceSwitchPanelModel) filteredItems() []deviceSwitchItem {
	query := strings.ToLower(strings.TrimSpace(string(m.filter)))
	var out []deviceSwitchItem
	for _, d := range m.devices {
		if query == "" || strings.Contains(strings.ToLower(d), query) {
			out = append(out, deviceSwitchItem{deviceID: d})
		}
	}
	custom := normalizeManagedDeviceID(string(m.filter))
	if custom != "" {
		found := false
		for _, d := range m.devices {
			if d == custom {
				found = true
				break
			}
		}
		if !found {
			out = append(out, deviceSwitchItem{deviceID: custom, custom: true})
		}
	}
	return out
}

func (m *deviceSwitchPanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func (m *deviceSwitchPanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	header := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("Switch device"),
		styles.meta.Render(fmt.Sprintf("Current: %s", firstNonEmpty(m.current, "(none)"))),
	)

	filterLine := styles.searchBox.Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			styles.searchPrompt.Render(">"),
			" ",
			styles.input.Render(renderInputValue(m.filter, m.filterCursor, styles)),
		),
	)

	items := m.filteredItems()
	lines := []string{styles.meta.Render(fmt.Sprintf("%d devices", len(items))), ""}
	if len(items) == 0 {
		lines = append(lines, styles.muted.Width(width).Render("No matching devices"))
	} else {
		innerWidth := maxInt(20, width-2)
		for idx, item := range items {
			rowStyle := styles.item.Width(maxInt(24, width-2))
			titleStyle := styles.itemTitle
			fillStyle := lipgloss.NewStyle()
			badgeStyle := styles.keyword
			badgeLabel := "CUSTOM"
			if !item.custom && item.deviceID == defaultManagedDeviceID {
				badgeLabel = "DEFAULT"
				badgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("150")).Padding(0, 1).Bold(true)
			} else if !item.custom {
				badgeLabel = ""
			}
			if idx == m.selected {
				selectedBG := lipgloss.Color("238")
				rowStyle = styles.selectedItem.Width(maxInt(24, width-2))
				titleStyle = titleStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
				fillStyle = fillStyle.Background(selectedBG)
				badgeStyle = badgeStyle.Background(lipgloss.Color("240"))
			}
			badge := ""
			labelText := item.deviceID
			if item.deviceID == m.current {
				labelText = labelText + "  (current)"
			}
			if badgeLabel != "" {
				badge = badgeStyle.Render(badgeLabel)
			}
			titleWidth := maxInt(8, innerWidth-lipgloss.Width(badge)-1)
			titleText := truncate(labelText, titleWidth)
			gapWidth := maxInt(1, innerWidth-lipgloss.Width(titleText)-lipgloss.Width(badge))
			titleRow := lipgloss.JoinHorizontal(lipgloss.Left,
				titleStyle.Render(titleText),
				fillStyle.Render(strings.Repeat(" ", gapWidth)),
				badge,
			)
			lines = append(lines, rowStyle.Render(titleRow))
		}
	}

	bodyHeight := maxInt(8, height-8)
	body := lipgloss.NewStyle().Height(bodyHeight).Render(strings.Join(lines, "\n"))
	footer := m.renderFooter(styles, width)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", filterLine, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *deviceSwitchPanelModel) renderFooter(styles paletteStyles, width int) string {
	status := strings.TrimSpace(m.currentStatus())
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "   ", pairs)
	}
	footer := pickRenderedShortcutFooter(width, renderSegments,
		[][2]string{{"Enter", "select"}, {"u/e", "move"}, {"Esc", "back"}},
	)
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

func (m *deviceSwitchPanelModel) setStatus(text string, duration time.Duration) {
	m.status = text
	m.statusUntil = time.Now().Add(duration)
}

func (m *deviceSwitchPanelModel) currentStatus() string {
	if m.status == "" {
		return ""
	}
	if !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
		m.status = ""
		return ""
	}
	return m.status
}
