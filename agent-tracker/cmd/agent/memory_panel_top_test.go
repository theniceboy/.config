package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestMemoryPanelViewFitsPopup(t *testing.T) {
	styles := newPaletteStyles()
	for _, h := range []int{22, 26, 32, 40, 54} {
		for _, w := range []int{96, 120, 140, 190} {
			files := []memoryUsageFile{}
			for i := 0; i < 80; i++ {
				files = append(files, memoryUsageFile{Store: "repo", Path: fmt.Sprintf(".memory/deeply/nested/reference/%02d-architecture-and-surrounding-context.md", i), Tokens: 400})
			}
			m := &memoryPanelModel{
				windowID:   "@1",
				windowName: "w",
				agentTitle: "sess · win",
				sessionID:  "ses_x",
				directory:  "~/base",
				enabled:    true,
				hasAgent:   true,
				usage:      memoryUsage{Status: "ready", Files: files, TotalTokens: 123456},
			}
			m.width, m.height = w, h
			view := m.render(styles, w, h)
			got := lipgloss.Height(view)
			if got > h {
				t.Fatalf("height %d width %d: view height %d exceeds popup %d", h, w, got, h)
			}
			if !strings.Contains(view, "Base memory") {
				t.Fatalf("height %d width %d: header missing", h, w)
			}
		}
	}
}
