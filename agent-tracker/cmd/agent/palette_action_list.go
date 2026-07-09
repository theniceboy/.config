package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type actionListPanel struct {
	actions  []paletteAction
	hotKeys  map[string]int
	filter   []rune
	cursor   int
	selected int
	offset   int
}

func newActionListPanel(actions []paletteAction, hotKeys map[string]int) *actionListPanel {
	return &actionListPanel{actions: actions, hotKeys: hotKeys, filter: []rune{}}
}

func filterActionsByQuery(actions []paletteAction, filter []rune) []paletteAction {
	query := strings.ToLower(strings.TrimSpace(string(filter)))
	if query == "" {
		return actions
	}
	parts := strings.Fields(query)
	filtered := make([]paletteAction, 0, len(actions))
	for _, a := range actions {
		haystack := strings.ToLower(a.Title)
		matched := true
		for _, part := range parts {
			if !strings.Contains(haystack, part) {
				matched = false
				break
			}
		}
		if matched {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

func (p *actionListPanel) filtered() []paletteAction {
	return filterActionsByQuery(p.actions, p.filter)
}

func (p *actionListPanel) navigate(delta int) {
	actions := p.filtered()
	if len(actions) == 0 {
		p.selected = 0
		return
	}
	next := clampInt(p.selected, 0, len(actions)-1) + delta
	if next < 0 {
		next = len(actions) - 1
	} else if next >= len(actions) {
		next = 0
	}
	p.selected = next
}

func (p *actionListPanel) handleKey(key string) (*paletteAction, bool) {
	for hk, idx := range p.hotKeys {
		if hk == key && idx >= 0 && idx < len(p.actions) {
			p.selected = idx
			action := p.actions[idx]
			return &action, true
		}
	}
	switch key {
	case "ctrl+u", "alt+u", "up":
		p.navigate(-1)
		return nil, true
	case "ctrl+e", "alt+e", "down":
		p.navigate(1)
		return nil, true
	case "ctrl+n", "left":
		p.cursor = clampInt(p.cursor-1, 0, len(p.filter))
		return nil, true
	case "ctrl+i", "tab", "right":
		p.cursor = clampInt(p.cursor+1, 0, len(p.filter))
		return nil, true
	case "enter", "alt+i":
		actions := p.filtered()
		if len(actions) > 0 && p.selected >= 0 && p.selected < len(actions) {
			action := actions[p.selected]
			return &action, true
		}
		return nil, true
	}
	if applyPaletteInputKey(key, &p.filter, &p.cursor, false) {
		p.selected = 0
		p.offset = 0
		return nil, true
	}
	return nil, false
}

func (p *actionListPanel) renderFilterLine(styles paletteStyles, width int) string {
	return styles.searchBox.Width(width).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			styles.searchPrompt.Render(">"),
			" ",
			styles.input.Render(renderInputValue(p.filter, p.cursor, styles)),
		),
	)
}

func renderActionItems(styles paletteStyles, actions []paletteAction, selectedPtr *int, offsetPtr *int, width, height int) string {
	selected := *selectedPtr
	entriesPerPage := maxInt(1, (height-2)/3)
	selected = clampInt(selected, 0, maxInt(0, len(actions)-1))
	offset := stableListOffset(*offsetPtr, selected, entriesPerPage, len(actions))
	*offsetPtr = offset
	*selectedPtr = selected
	blocks := []string{styles.meta.Render(fmt.Sprintf("%d commands", len(actions))), ""}
	if len(actions) == 0 {
		blocks = append(blocks, styles.muted.Width(width).Render("No matching commands"))
	} else {
		for row := 0; row < entriesPerPage; row++ {
			idx := offset + row
			if idx >= len(actions) {
				break
			}
			action := actions[idx]
			sectionLabel := styles.sectionLabel
			subtle := styles.itemSubtitle
			titleStyle := styles.itemTitle
			box := styles.item
			markerText := "  "
			markerStyle := styles.muted
			rowStyle := lipgloss.NewStyle().Width(maxInt(16, width-2))
			fillStyle := lipgloss.NewStyle()
			if idx == selected {
				selectedBG := lipgloss.Color("238")
				sectionLabel = styles.selectedLabel.Background(selectedBG)
				subtle = styles.selectedSubtle.Background(selectedBG)
				titleStyle = styles.itemTitle.Background(selectedBG).Foreground(lipgloss.Color("230"))
				box = styles.selectedItem
				markerText = "› "
				markerStyle = styles.selectedLabel.Background(selectedBG)
				rowStyle = rowStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
				fillStyle = fillStyle.Background(selectedBG).Foreground(lipgloss.Color("230"))
			}
			innerWidth := maxInt(16, width-2)
			labelText := strings.ToUpper(action.Section)
			labelWidth := lipgloss.Width(labelText)
			markerWidth := lipgloss.Width(markerText)
			titleWidth := maxInt(10, innerWidth-markerWidth-labelWidth-1)
			titleText := truncate(action.Title, titleWidth)
			gapWidth := maxInt(1, innerWidth-markerWidth-lipgloss.Width(titleText)-labelWidth)
			titleRow := rowStyle.Render(
				markerStyle.Render(markerText) +
					titleStyle.Render(titleText) +
					fillStyle.Render(strings.Repeat(" ", gapWidth)) +
					sectionLabel.Render(labelText),
			)
			subtitleRow := rowStyle.Render(fillStyle.Render(strings.Repeat(" ", markerWidth)) + subtle.Render(truncate(action.Subtitle, maxInt(0, innerWidth-markerWidth))))
			block := lipgloss.JoinVertical(lipgloss.Left, titleRow, subtitleRow)
			blocks = append(blocks, box.Width(width).Render(block))
		}
	}
	content := strings.Join(blocks, "\n")
	return lipgloss.NewStyle().Width(width).Height(height).Render(content)
}

func (p *actionListPanel) renderList(styles paletteStyles, width, height int) string {
	actions := p.filtered()
	return renderActionItems(styles, actions, &p.selected, &p.offset, width, height)
}
