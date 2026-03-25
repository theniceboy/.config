package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type devicePanelMode int

const (
	devicePanelModeList devicePanelMode = iota
	devicePanelModeAdd
	devicePanelModeConfirmDelete
)

type devicePanelModel struct {
	devices      []string
	selected     int
	mode         devicePanelMode
	width        int
	height       int
	addText      []rune
	addCursor    int
	deleteDevice string
	status       string
	statusUntil  time.Time
	showAltHints bool
	requestBack  bool
}

func newDevicePanelModel() *devicePanelModel {
	model := &devicePanelModel{}
	model.reload()
	return model
}

func (m *devicePanelModel) reload() {
	m.devices = loadManagedDevices()
	if len(m.devices) == 0 {
		m.devices = []string{defaultManagedDeviceID}
	}
	m.selected = clampInt(m.selected, 0, len(m.devices)-1)
}

func (m *devicePanelModel) Init() tea.Cmd {
	return nil
}

func (m *devicePanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		switch m.mode {
		case devicePanelModeAdd:
			return m.updateAdd(key)
		case devicePanelModeConfirmDelete:
			return m.updateConfirmDelete(key)
		default:
			return m.updateList(key)
		}
	}
	return m, nil
}

func (m *devicePanelModel) updateList(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.requestBack = true
	case "ctrl+u", "alt+u", "up", "u":
		m.selected = clampInt(m.selected-1, 0, len(m.devices)-1)
	case "ctrl+e", "alt+e", "down", "e":
		m.selected = clampInt(m.selected+1, 0, len(m.devices)-1)
	case "a":
		m.mode = devicePanelModeAdd
		m.addText = nil
		m.addCursor = 0
	case "d", "x", "delete":
		deviceID := m.currentDevice()
		if deviceID == defaultManagedDeviceID {
			m.setStatus(defaultManagedDeviceID+" cannot be removed", 1500*time.Millisecond)
			return m, nil
		}
		if deviceID != "" {
			m.deleteDevice = deviceID
			m.mode = devicePanelModeConfirmDelete
		}
	}
	return m, nil
}

func (m *devicePanelModel) updateAdd(key string) (tea.Model, tea.Cmd) {
	if key == "esc" {
		m.mode = devicePanelModeList
		return m, nil
	}
	if key == "enter" {
		deviceID := normalizeManagedDeviceID(string(m.addText))
		if deviceID == "" {
			m.mode = devicePanelModeList
			return m, nil
		}
		if err := addManagedDevice(deviceID); err != nil {
			m.setStatus(err.Error(), 1500*time.Millisecond)
		} else {
			m.reload()
			for idx, existing := range m.devices {
				if existing == deviceID {
					m.selected = idx
					break
				}
			}
		}
		m.mode = devicePanelModeList
		return m, nil
	}
	applyPaletteInputKey(key, &m.addText, &m.addCursor, true)
	return m, nil
}

func (m *devicePanelModel) updateConfirmDelete(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "n" {
		m.mode = devicePanelModeList
		m.deleteDevice = ""
		return m, nil
	}
	if key == "y" || key == "enter" {
		if err := removeManagedDevice(m.deleteDevice); err != nil {
			m.setStatus(err.Error(), 1500*time.Millisecond)
		} else {
			m.reload()
		}
		m.mode = devicePanelModeList
		m.deleteDevice = ""
		return m, nil
	}
	return m, nil
}

func (m *devicePanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func (m *devicePanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	if m.mode == devicePanelModeAdd {
		return m.renderAdd(styles, width, height)
	}
	if m.mode == devicePanelModeConfirmDelete {
		return m.renderConfirmDelete(styles, width, height)
	}
	return m.renderList(styles, width, height)
}

func (m *devicePanelModel) renderList(styles paletteStyles, width, height int) string {
	header := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("Devices"),
		styles.meta.Render("Global launch devices managed by agent-tracker"),
	)

	lines := []string{styles.meta.Render(fmt.Sprintf("%d devices", len(m.devices))), ""}
	for idx, deviceID := range m.devices {
		rowStyle := styles.item.Width(maxInt(24, width-2))
		titleStyle := styles.itemTitle
		metaStyle := styles.itemSubtitle
		fillStyle := lipgloss.NewStyle()
		badgeStyle := styles.keyword
		badgeLabel := "CUSTOM"
		if idx == m.selected {
			selectedBG := lipgloss.Color("238")
			rowStyle = styles.selectedItem.Width(maxInt(24, width-2))
			titleStyle = titleStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
			metaStyle = styles.selectedSubtle.Background(selectedBG)
			fillStyle = fillStyle.Background(selectedBG)
			badgeStyle = badgeStyle.Background(lipgloss.Color("240"))
		}
		if deviceID == defaultManagedDeviceID {
			badgeLabel = "DEFAULT"
			badgeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("235")).Background(lipgloss.Color("150")).Padding(0, 1).Bold(true)
		}
		badge := badgeStyle.Render(badgeLabel)
		innerWidth := maxInt(22, width-2)
		titleWidth := maxInt(8, innerWidth-lipgloss.Width(badge)-1)
		titleText := truncate(deviceID, titleWidth)
		gapWidth := maxInt(1, innerWidth-lipgloss.Width(titleText)-lipgloss.Width(badge))
		titleRow := lipgloss.JoinHorizontal(lipgloss.Left,
			titleStyle.Render(titleText),
			fillStyle.Render(strings.Repeat(" ", gapWidth)),
			badge,
		)
		detail := "Manual launch device"
		if deviceID == defaultManagedDeviceID {
			detail = "Always available and cannot be removed"
		}
		detailText := truncate(detail, innerWidth)
		detailGap := maxInt(0, innerWidth-lipgloss.Width(detailText))
		detailRow := lipgloss.JoinHorizontal(lipgloss.Left,
			metaStyle.Render(detailText),
			fillStyle.Render(strings.Repeat(" ", detailGap)),
		)
		lines = append(lines, rowStyle.Render(lipgloss.JoinVertical(lipgloss.Left, titleRow, detailRow)))
	}

	bodyHeight := maxInt(8, height-7)
	body := lipgloss.NewStyle().Height(bodyHeight).Render(strings.Join(lines, "\n"))
	footer := m.renderFooter(styles, width)
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *devicePanelModel) renderAdd(styles paletteStyles, width, height int) string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render("Add device"),
		styles.modalBody.Render("Enter a global device id. Example: ios, macos, android."),
		"",
		styles.input.Render(renderInputValue(m.addText, m.addCursor, styles)),
		"",
		styles.modalHint.Render("Enter save  Esc back"),
	)
	box := styles.modal.Width(minInt(76, maxInt(36, width-10))).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *devicePanelModel) renderConfirmDelete(styles paletteStyles, width, height int) string {
	body := lipgloss.JoinVertical(lipgloss.Left,
		styles.modalTitle.Render("Remove device"),
		styles.modalBody.Render(fmt.Sprintf("Remove %s from the global device list?", m.deleteDevice)),
		"",
		styles.modalHint.Render("y confirm  n cancel"),
	)
	box := styles.modal.Width(minInt(64, maxInt(36, width-10))).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *devicePanelModel) renderFooter(styles paletteStyles, width int) string {
	status := strings.TrimSpace(m.currentStatus())
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "   ", pairs)
	}
	footer := ""
	if m.showAltHints {
		footer = pickRenderedShortcutFooter(width, renderSegments,
			[][2]string{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}},
			[][2]string{{"Alt-S", "close"}},
		)
	} else {
		footer = pickRenderedShortcutFooter(width, renderSegments,
			[][2]string{{"u/e", "move"}, {"a", "add"}, {"d", "remove"}, {"Esc", "back"}, {footerHintToggleKey, "more"}},
			[][2]string{{"u/e", "move"}, {"a", "add"}, {"d", "remove"}, {footerHintToggleKey, "more"}},
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

func (m *devicePanelModel) currentDevice() string {
	if len(m.devices) == 0 || m.selected < 0 || m.selected >= len(m.devices) {
		return ""
	}
	return m.devices[m.selected]
}

func (m *devicePanelModel) setStatus(text string, duration time.Duration) {
	m.status = text
	m.statusUntil = time.Now().Add(duration)
}

func (m *devicePanelModel) currentStatus() string {
	if m.status == "" {
		return ""
	}
	if !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
		m.status = ""
		return ""
	}
	return m.status
}
