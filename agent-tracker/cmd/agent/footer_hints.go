package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const footerHintToggleKey = "?"

func isAltFooterToggleKey(msg tea.KeyMsg) bool {
	return msg.String() == footerHintToggleKey || (msg.Alt && msg.Type == tea.KeyEscape)
}

func renderShortcutPairs(renderKey func(string) string, renderText func(string) string, gap string, pairs [][2]string) string {
	segments := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		segments = append(segments, renderKey(pair[0])+renderText(" "+pair[1]))
	}
	return strings.Join(segments, gap)
}

func pickRenderedShortcutFooter(width int, render func([][2]string) string, candidates ...[][2]string) string {
	if len(candidates) == 0 {
		return ""
	}
	footer := render(candidates[len(candidates)-1])
	for _, candidate := range candidates {
		rendered := render(candidate)
		if lipgloss.Width(rendered) <= maxInt(1, width) {
			return rendered
		}
		footer = rendered
	}
	return footer
}
