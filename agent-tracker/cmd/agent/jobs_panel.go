package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type jobsPanelModel struct {
	jobs        []backgroundJob
	cursor      int
	offset      int
	width       int
	height      int
	loaded      bool
	requestBack bool
	generation  int
}

type jobsPanelTickMsg struct {
	panel      *jobsPanelModel
	generation int
}

func jobsPanelTickCmd(panel *jobsPanelModel) tea.Cmd {
	generation := panel.generation
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return jobsPanelTickMsg{panel: panel, generation: generation}
	})
}

func newJobsPanelModel() *jobsPanelModel {
	model := &jobsPanelModel{}
	model.reload()
	return model
}

func (m *jobsPanelModel) reload() {
	m.jobs = listBackgroundJobs()
	m.loaded = true
	if m.cursor >= len(m.jobs) {
		m.cursor = maxInt(0, len(m.jobs)-1)
	}
}

func (m *jobsPanelModel) currentStatus() string {
	running := 0
	for _, job := range m.jobs {
		if job.running() {
			running++
		}
	}
	if running == 0 && len(m.jobs) == 0 {
		return "No background jobs"
	}
	if running == 0 {
		return fmt.Sprintf("0 running · %d recent", len(m.jobs))
	}
	return fmt.Sprintf("%d running · %d recent", running, len(m.jobs)-running)
}

func (m *jobsPanelModel) handleKey(key string) {
	switch key {
	case "esc", "ctrl+c", "q":
		m.requestBack = true
		return
	case "r":
		m.reload()
		return
	case "u", "up", "ctrl+u", "alt+u":
		m.cursor = clampInt(m.cursor-1, 0, maxInt(0, len(m.jobs)-1))
		return
	case "e", "down", "ctrl+e", "alt+e":
		m.cursor = clampInt(m.cursor+1, 0, maxInt(0, len(m.jobs)-1))
		return
	case ",":
		m.cursor = clampInt(m.cursor-5, 0, maxInt(0, len(m.jobs)-1))
		return
	case ".":
		m.cursor = clampInt(m.cursor+5, 0, maxInt(0, len(m.jobs)-1))
		return
	}
}

func (m *jobsPanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func (m *jobsPanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	contentWidth := maxInt(16, width-2)
	contentHeight := maxInt(8, height-7)
	header := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("Ongoing Tasks"),
		styles.meta.Render(fmt.Sprintf("Background jobs · %s", m.currentStatus())),
	)
	var body string
	if !m.loaded {
		body = styles.muted.Render("Loading jobs...")
	} else if len(m.jobs) == 0 {
		body = styles.muted.Render("No background jobs — clipboard pull/push and future async tasks appear here")
	} else {
		body = m.renderJobs(styles, contentWidth, contentHeight)
	}
	footer := styles.muted.Render("u/e move · ,/. scroll · r reload · q/esc back")
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *jobsPanelModel) renderJobs(styles paletteStyles, width, height int) string {
	entriesPerPage := maxInt(1, (height-2)/3)
	m.offset = stableListOffset(m.offset, m.cursor, entriesPerPage, len(m.jobs))
	rows := []string{}
	now := time.Now()
	for row := 0; row < entriesPerPage; row++ {
		idx := m.offset + row
		if idx >= len(m.jobs) {
			break
		}
		job := m.jobs[idx]
		titleStyle := styles.itemTitle
		subtle := styles.itemSubtitle
		statusColor := ""
		switch {
		case idx == m.cursor:
			titleStyle = styles.itemTitle.Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230"))
			subtle = styles.selectedSubtle.Background(lipgloss.Color("238"))
		case job.Status == jobStatusError:
			statusColor = " ✗"
		}
		titleRow := fmt.Sprintf("%s %s · %s%s", jobStatusIcon(job.Status), job.Title, job.elapsedLabel(now), statusColor)
		detail := strings.TrimSpace(job.Output)
		if detail == "" && job.running() {
			detail = "running…"
		}
		block := lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render(truncate(titleRow, maxInt(10, width))),
			subtle.Render(truncate(detail, maxInt(10, width))),
		)
		rows = append(rows, styles.item.Width(width).Render(block))
	}
	content := strings.Join(rows, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Render(content)
}
