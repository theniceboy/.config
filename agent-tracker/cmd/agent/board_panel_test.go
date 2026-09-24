package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func isLetter(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func TestBoardPanelViewFitsPopup(t *testing.T) {
	m := &boardPanelModel{collapsed: map[string]bool{}, doneWS: map[string]bool{}, tl: newTLModel()}
	long := "Long item title with CJK 中文宽字符 and trailing detail that should clip not wrap — "
	for i := 0; i < 12; i++ {
		folders := []string{}
		if i%3 != 0 {
			folders = []string{"release-1"}
		}
		m.items = append(m.items, boardItem{
			ID: "TST-" + itoa(i), Title: long + long, Status: "doing", Prio: "urgent",
			Owner: "operator", Due: "2026-01-0" + itoa(i%9+1), Workstream: "alpha",
			Body: strings.Repeat("body line ", 40), Folders: folders,
		})
	}
	m.folders = []boardFolder{{Workstream: "alpha", Path: "release-1", Description: "Release train " + strings.Repeat("desc ", 30)}}
	for _, h := range []int{24, 30, 44, 54} {
		for _, w := range []int{96, 120, 149, 190} {
			m.width, m.height = w, h
			m.rebuild()
			m.cursor = 0
			m.offset = 0
			view := m.render(newPaletteStyles(), w, h)
			if got := lipgloss.Height(view); got > h {
				t.Fatalf("h=%d w=%d: view height %d exceeds %d", h, w, got, h)
			}
			if !strings.Contains(view, "Board") {
				t.Fatalf("h=%d w=%d: header missing", h, w)
			}
		}
	}
	// every rendered row must be exactly one physical line (no wrap-induced shift)
	m.width, m.height = 149, 44
	m.rebuild()
	lines := strings.Split(m.render(newPaletteStyles(), 149, 44), "\n")
	for i, l := range lines {
		if lw := lipgloss.Width(l); lw > 149 {
			t.Fatalf("line %d width %d > 149: %q", i, lw, l)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}

func testPanelModel() *boardPanelModel {
	m := &boardPanelModel{collapsed: map[string]bool{}, doneWS: map[string]bool{}, tl: newTLModel(), tab: 1}
	m.wsOrder = []string{"alpha", "beta"}
	for i := 0; i < 3; i++ {
		m.items = append(m.items, boardItem{ID: "TST-A" + itoa(i), Title: "Alpha item " + itoa(i) + " with some length to render",
			Status: []string{"doing", "todo", "doing"}[i], Prio: "normal", Workstream: "alpha"})
		m.items = append(m.items, boardItem{ID: "TST-B" + itoa(i), Title: "Beta item " + itoa(i) + " with some length to render",
			Status: "todo", Prio: "normal", Workstream: "beta", Due: "2026-01-0" + itoa(i+1)})
	}
	m.rebuild()
	return m
}

func TestSelectedRowBackgroundUnbroken(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	m := testPanelModel()
	m.width, m.height = 149, 44
	m.cursor = 1
	view := m.render(newPaletteStyles(), 149, 44)
	for _, line := range strings.Split(view, "\n") {
		if !strings.Contains(line, "\x1b[48;5;238m") {
			continue
		}
		cell := line[:strings.Index(line, "│")]
		bgOn, tok := false, ""
		flush := func() {
			if tok == "" {
				return
			}
			if !bgOn && strings.TrimSpace(tok) != "" {
				t.Fatalf("text %q rendered without the row background: %q", tok, cell)
			}
			tok = ""
		}
		for i := 0; i < len(cell); i++ {
			if cell[i] == '\x1b' {
				flush()
				j := i
				for j < len(cell) && !isLetter(cell[j]) {
					j++
				}
				seq := cell[i : j+1]
				clears := strings.Contains(seq, "\x1b[0m") || strings.Contains(seq, "\x1b[49m")
				bgOn = strings.Contains(seq, "48;5;238") || (bgOn && !clears)
				i = j
				continue
			}
			tok += string(cell[i])
		}
		flush()
		return
	}
	t.Fatal("no selected row found")
}

func TestCursorDoesNotShiftColumns(t *testing.T) {
	ansiStrip := func(s string) string {
		re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
		return re.ReplaceAllString(s, "")
	}
	glyphCol := func(view string, title string) int {
		for _, line := range strings.Split(view, "\n") {
			clean := ansiStrip(line)
			left := clean
			if i := strings.LastIndex(left, "│"); i >= 0 {
				left = left[:i]
			}
			if strings.Contains(left, title) {
				for i, r := range left {
					if strings.ContainsRune("●○◐◌✓", r) {
						return i
					}
				}
			}
		}
		return -1
	}
	m := testPanelModel()
	m.width, m.height = 149, 44
	m.cursor = 1
	v1 := m.render(newPaletteStyles(), 149, 44)
	m.cursor = 2
	v2 := m.render(newPaletteStyles(), 149, 44)
	colSel1 := glyphCol(v1, "Alpha item 0")
	colUnsel := glyphCol(v2, "Alpha item 0")
	colSel2 := glyphCol(v2, "Alpha item 1")
	if colSel1 < 0 || colUnsel < 0 || colSel2 < 0 {
		t.Fatalf("rows not found: %d %d %d", colSel1, colUnsel, colSel2)
	}
	if colSel1 != colUnsel {
		t.Fatalf("deselect shifts glyph: selected col %d vs unselected %d", colSel1, colUnsel)
	}
	if colSel1 != colSel2 {
		t.Fatalf("selection shifts glyph: %d vs %d", colSel1, colSel2)
	}
}

func TestCursorReachesEveryRow(t *testing.T) {
	m := newBoardPanelModel()
	m.width, m.height = 149, 44
	m.cursor = 0
	for i := 0; i < len(m.rows)+10; i++ {
		m.moveCursor(1)
	}
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("cursor must reach the last row, at %d of %d", m.cursor, len(m.rows))
	}
	for i := 0; i < len(m.rows)+10; i++ {
		m.moveCursor(-1)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor must reach row 0, at %d", m.cursor)
	}
	if !m.rows[0].isWS {
		t.Fatal("row 0 should be a ws header and selectable")
	}
}

func TestWorkstreamSectionsSeparated(t *testing.T) {
	ansiStrip := func(s string) string {
		return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
	}
	m := testPanelModel()
	m.width, m.height = 149, 60
	view := ansiStrip(m.render(newPaletteStyles(), 149, 60))
	lines := strings.Split(view, "\n")
	headers := 0
	for i, line := range lines {
		left := line
		if j := strings.Index(left, "│"); j >= 0 {
			left = left[:j]
		}
		if !regexp.MustCompile(`^\s*(📂|📁)?\s*(ALPHA|BETA) +\d`).MatchString(left) {
			continue
		}
		headers++
		if i > 0 {
			prev := lines[i-1]
			if k := strings.Index(prev, "│"); k >= 0 {
				prev = prev[:k]
			}
			if strings.TrimSpace(prev) != "" && !strings.HasPrefix(strings.TrimSpace(prev), "─") {
				t.Fatalf("ws header %q not preceded by a blank separator (prev: %q)", strings.TrimSpace(left), strings.TrimSpace(prev))
			}
		}
	}
	if headers < 2 {
		t.Fatalf("expected multiple ws sections, found %d", headers)
	}
}

func TestDoneScopedToggle(t *testing.T) {
	m := &boardPanelModel{collapsed: map[string]bool{}, doneWS: map[string]bool{}, tl: newTLModel(), tab: 1}
	m.items = []boardItem{
		{ID: "A-1", Title: "alpha", Status: "doing", Workstream: "aaa"},
		{ID: "A-2", Title: "alpha done", Status: "done", Workstream: "aaa"},
		{ID: "B-1", Title: "beta", Status: "done", Workstream: "bbb"},
	}
	m.width, m.height = 120, 40
	m.rebuild()
	has := func(id string) bool {
		for _, r := range m.rows {
			if r.item != nil && r.item.ID == id {
				return true
			}
		}
		return false
	}
	if has("A-2") || has("B-1") {
		t.Fatal("done items must be hidden by default")
	}
	if !has("A-1") {
		t.Fatal("doing item missing")
	}
	m.cursor = m.rowOfItem("A-1")
	m.handleKey("d")
	m.rebuild()
	if !has("A-2") {
		t.Fatal("toggled workstream must show its done items")
	}
	if has("B-1") {
		t.Fatal("other workstream done items must stay hidden")
	}
}

func (m *boardPanelModel) rowOfItem(id string) int {
	for i, r := range m.rows {
		if r.item != nil && r.item.ID == id {
			return i
		}
	}
	return 0
}

func TestFolderFoldAndDesc(t *testing.T) {
	m := &boardPanelModel{collapsed: map[string]bool{}, doneWS: map[string]bool{}, tl: newTLModel(), tab: 1}
	m.items = []boardItem{
		{ID: "A-1", Title: "release", Status: "todo", Workstream: "aaa", Folders: []string{"ship"}},
		{ID: "A-2", Title: "followup", Status: "todo", Workstream: "aaa", Folders: []string{"ship"}},
		{ID: "A-3", Title: "loose", Status: "todo", Workstream: "aaa"},
	}
	m.folders = []boardFolder{{Workstream: "aaa", Path: "ship", Description: "Release train"}}
	m.width, m.height = 120, 40
	m.rebuild()
	has := func(id string) bool {
		for _, r := range m.rows {
			if r.item != nil && r.item.ID == id {
				return true
			}
		}
		return false
	}
	if !has("A-1") || !has("A-3") {
		t.Fatal("folder items missing before fold")
	}
	m.cursor = m.rowOfItem("A-1")
	m.handleKey("t")
	if has("A-1") || has("A-2") {
		t.Fatal("collapsed folder must hide its items")
	}
	if !has("A-3") {
		t.Fatal("top-level item must stay visible")
	}
	view := m.render(newPaletteStyles(), 120, 40)
	if !strings.Contains(view, "SHIP") {
		t.Fatal("collapsed folder header missing")
	}
	m.handleKey("t")
	if !has("A-1") {
		t.Fatal("second toggle must restore items")
	}
	view = m.render(newPaletteStyles(), 120, 40)
	for _, line := range strings.Split(view, "\n") {
		left := line
		if i := strings.LastIndex(left, "│"); i >= 0 {
			left = left[:i]
		}
		if strings.Contains(left, "Release train") {
			t.Fatal("folder description must not render in the list")
		}
	}
	for i, r := range m.rows {
		if r.isFolder {
			m.cursor = i
			break
		}
	}
	view = m.render(newPaletteStyles(), 120, 40)
	if !strings.Contains(view, "Release train") {
		t.Fatal("folder description missing from detail pane")
	}
}

func TestScrollToTopKeepsHeader(t *testing.T) {
	for _, sz := range [][2]int{{106, 25}, {96, 24}, {149, 44}, {162, 49}} {
		w, h := sz[0], sz[1]
		m := newBoardPanelModel()
		m.handleKey("G")
		for i := 0; i < len(m.rows)+50; i++ {
			m.handleKey("u")
		}
		view := m.render(newPaletteStyles(), w, h)
		lines := strings.Split(view, "\n")
		if len(lines) > h {
			t.Fatalf("%dx%d: view %d rows > %d", w, h, len(lines), h)
		}
		if !strings.Contains(lines[0], "Board") {
			t.Fatalf("%dx%d: header not on row 1: %q", w, h, lines[0])
		}
		if !strings.Contains(view, "PROJECTONE") {
			t.Fatalf("%dx%d: first ws header missing from view", w, h)
		}
	}
}

func TestSiblingOrdering(t *testing.T) {
	m := &boardPanelModel{collapsed: map[string]bool{}, doneWS: map[string]bool{}, tl: newTLModel()}
	m.items = []boardItem{
		{ID: "A-1", Title: "zeta", Status: "todo", Workstream: "aaa"},
		{ID: "A-2", Title: "alpha", Status: "todo", Workstream: "aaa", Order: 1},
		{ID: "A-3", Title: "inside", Status: "todo", Workstream: "aaa", Folders: []string{"folderx"}},
		{ID: "A-4", Title: "zz", Status: "todo", Workstream: "aaa", Folders: []string{"zfolder"}},
		{ID: "A-5", Title: "aa", Status: "todo", Workstream: "aaa", Folders: []string{"afolder"}},
	}
	m.folders = []boardFolder{
		{Workstream: "aaa", Path: "folderx"},
		{Workstream: "aaa", Path: "zfolder", Order: 2},
		{Workstream: "aaa", Path: "afolder"},
	}
	m.width, m.height = 120, 40
	m.rebuild()
	var seq []string
	for _, r := range m.rows {
		switch {
		case r.isWS:
			seq = append(seq, "WS")
		case r.isFolder:
			seq = append(seq, "F:"+r.folderPath)
		default:
			seq = append(seq, "I:"+r.item.ID)
		}
	}
	want := []string{"WS", "I:A-2", "F:zfolder", "I:A-4", "I:A-1", "F:afolder", "I:A-5", "F:folderx", "I:A-3"}
	if len(seq) != len(want) {
		t.Fatalf("rows %v", seq)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("row %d = %s, want %s (all: %v)", i, seq[i], want[i], seq)
		}
	}
}
