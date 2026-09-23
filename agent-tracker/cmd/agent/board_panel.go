package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Board viewer panel — read-only. All board semantics (ids, nesting, status,
// scan) come from the central board API: `~/bin/board export` emits JSON and
// this panel only renders it. Never duplicate board logic here.

type boardItem struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Prio       string   `json:"prio"`
	Owner      string   `json:"owner"`
	Start      string   `json:"start"`
	Due        string   `json:"due"`
	Type       string   `json:"type"`
	Workstream string   `json:"workstream"`
	Folders    []string `json:"folders"`
	Order      int      `json:"order"`
	Path       string   `json:"path"`
	Body       string   `json:"body"`
}

type boardFolder struct {
	Workstream  string `json:"workstream"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Order       int    `json:"order"`
}

type boardExport struct {
	Count   int           `json:"count"`
	Items   []boardItem   `json:"items"`
	Folders []boardFolder `json:"folders"`
}

func boardExportCommand() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "bin", "board")
}

func loadBoardExport() (*boardExport, error) {
	cmdPath := boardExportCommand()
	if cmdPath == "" {
		return nil, fmt.Errorf("board command not found")
	}
	out, err := exec.Command(cmdPath, "export").Output()
	if err != nil {
		return nil, fmt.Errorf("board export failed: %w", err)
	}
	var data boardExport
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("board export: %w", err)
	}
	return &data, nil
}

type boardRow struct {
	ws          string
	item        *boardItem
	depth       int
	isWS        bool
	wsCount     string
	isFolder    bool
	folderPath  string
	folderDesc  string
	folderCount string
	branch      string
	pre         int
}

func boardBranch(rail string, last bool) string {
	if last {
		return rail + "└─ "
	}
	return rail + "├─ "
}

type boardPanelModel struct {
	items        []boardItem
	folders      []boardFolder
	rows         []boardRow
	cursor       int
	offset       int
	lineOf       []int
	totalLines   int
	detailOffset int
	focusDetail  bool
	collapsed    map[string]bool
	doneWS       map[string]bool
	query        []rune
	searchCursor int
	searching    bool
	width        int
	height       int
	status       string
	statusUntil  time.Time
	requestBack  bool
	loadErr      string
	loadedCount  int
	tab          int
	tl           *tlModel
	pendingCmd   tea.Cmd
}

func newBoardPanelModel() *boardPanelModel {
	m := &boardPanelModel{collapsed: map[string]bool{}, doneWS: map[string]bool{}, tab: 0, tl: newTLModel()}
	m.reload()
	return m
}

func (m *boardPanelModel) reload() {
	data, err := loadBoardExport()
	if err != nil {
		m.loadErr = err.Error()
		return
	}
	m.loadErr = ""
	m.items = data.Items
	m.folders = data.Folders
	m.loadedCount = data.Count
	m.tl.setItems(data.Items)
	m.rebuild()
}

func boardStatusRank(s string) int {
	switch s {
	case "doing":
		return 0
	case "todo", "inbox":
		return 1
	case "done":
		return 8
	}
	return 5
}

func boardPrioRank(p string) int {
	switch p {
	case "urgent":
		return 0
	case "high":
		return 1
	case "":
		return 2
	case "normal":
		return 2
	case "low":
		return 3
	}
	return 2
}

func boardWSRank(ws string) int {
	switch ws {
	case "projectone":
		return 0
	case "hq":
		return 1
	case "projecttwo":
		return 2
	case "devtools":
		return 3
	case "adhoc":
		return 4
	case "inbox":
		return 5
	}
	return 9
}

func boardLess(a, b *boardItem) bool {
	if boardStatusRank(a.Status) != boardStatusRank(b.Status) {
		return boardStatusRank(a.Status) < boardStatusRank(b.Status)
	}
	if boardPrioRank(a.Prio) != boardPrioRank(b.Prio) {
		return boardPrioRank(a.Prio) < boardPrioRank(b.Prio)
	}
	return a.ID < b.ID
}

func (m *boardPanelModel) matchesQuery(it *boardItem, q string) bool {
	if strings.Contains(strings.ToLower(it.Title), q) ||
		strings.Contains(strings.ToLower(it.ID), q) ||
		strings.Contains(strings.ToLower(it.Workstream), q) ||
		strings.Contains(strings.ToLower(it.Body), q) {
		return true
	}
	return strings.Contains(strings.ToLower(strings.Join(it.Folders, "/")), q)
}

func (m *boardPanelModel) itemVisible(it *boardItem, q string) bool {
	if q != "" {
		return m.matchesQuery(it, q)
	}
	return it.Status != "done" || m.doneWS[it.Workstream]
}

func (m *boardPanelModel) folderKey(ws, path string) string {
	return ws + "/" + path
}

func (m *boardPanelModel) directSubfolders(ws, prefix string) []boardFolder {
	out := []boardFolder{}
	for _, f := range m.folders {
		if f.Workstream != ws {
			continue
		}
		p := f.Path
		if prefix != "" {
			if !strings.HasPrefix(p, prefix+"/") || strings.Contains(p[len(prefix)+1:], "/") {
				continue
			}
		} else if strings.Contains(p, "/") {
			continue
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

type boardNode struct {
	item   *boardItem
	folder *boardFolder
	order  int
}

// siblings: ordered nodes first (items and folders interleaved by order),
// then items (status/prio), then folders (alpha)
func boardNodeLess(a, b boardNode) bool {
	if a.order != b.order {
		if a.order == 0 {
			return false
		}
		if b.order == 0 {
			return true
		}
		return a.order < b.order
	}
	if (a.item == nil) != (b.item == nil) {
		return a.item != nil
	}
	if a.item != nil {
		return boardLess(a.item, b.item)
	}
	return a.folder.Path < b.folder.Path
}

func (m *boardPanelModel) groupNodes(ws, prefix, q string) []boardNode {
	nodes := []boardNode{}
	for i := range m.items {
		it := &m.items[i]
		if it.Workstream == ws && strings.Join(it.Folders, "/") == prefix && m.itemVisible(it, q) {
			nodes = append(nodes, boardNode{item: it, order: it.Order})
		}
	}
	for _, f := range m.directSubfolders(ws, prefix) {
		hasContent := false
		for i := range m.items {
			it := &m.items[i]
			if it.Workstream == ws && strings.HasPrefix(strings.Join(it.Folders, "/")+"/", f.Path+"/") && m.itemVisible(it, q) {
				hasContent = true
				break
			}
		}
		if !hasContent && len(m.directSubfolders(ws, f.Path)) == 0 {
			continue
		}
		nodes = append(nodes, boardNode{folder: &f, order: f.Order})
	}
	sort.SliceStable(nodes, func(i, j int) bool { return boardNodeLess(nodes[i], nodes[j]) })
	return nodes
}

func (m *boardPanelModel) emitGroup(ws, prefix, rail string, depth int, rows *[]boardRow, q string) {
	today := time.Now().Format("2006-01-02")
	nodes := m.groupNodes(ws, prefix, q)
	for i, n := range nodes {
		last := i == len(nodes)-1
		br := boardBranch(rail, last)
		childRail := rail
		if last {
			childRail += "   "
		} else {
			childRail += "│  "
		}
		if n.item != nil {
			*rows = append(*rows, boardRow{item: n.item, depth: depth, branch: br})
			continue
		}
		f := n.folder
		counts := map[string]int{}
		overdue := 0
		for j := range m.items {
			it := &m.items[j]
			if it.Workstream != ws || !strings.HasPrefix(strings.Join(it.Folders, "/")+"/", f.Path+"/") {
				continue
			}
			counts[it.Status]++
			if it.Due != "" && it.Due < today && it.Status != "done" {
				overdue++
			}
		}
		parts := []string{}
		for _, s := range []string{"doing", "todo", "done"} {
			if counts[s] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
			}
		}
		if overdue > 0 {
			parts = append(parts, fmt.Sprintf("%d overdue", overdue))
		}
		*rows = append(*rows, boardRow{ws: ws, isFolder: true, folderPath: f.Path, folderDesc: f.Description,
			folderCount: strings.Join(parts, " · "), depth: depth, branch: br})
		if !m.collapsed[m.folderKey(ws, f.Path)] {
			m.emitGroup(ws, f.Path, childRail, depth+1, rows, q)
		}
	}
}

func (m *boardPanelModel) rebuild() {
	q := strings.ToLower(strings.TrimSpace(string(m.query)))
	workstreams := map[string]bool{}
	for i := range m.items {
		workstreams[m.items[i].Workstream] = true
	}
	names := make([]string, 0, len(workstreams))
	for ws := range workstreams {
		names = append(names, ws)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, rj := boardWSRank(names[i]), boardWSRank(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
	rows := []boardRow{}
	if q != "" {
		matches := []*boardItem{}
		for i := range m.items {
			if m.itemVisible(&m.items[i], q) {
				matches = append(matches, &m.items[i])
			}
		}
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].Workstream != matches[j].Workstream {
				return boardWSRank(matches[i].Workstream) < boardWSRank(matches[j].Workstream)
			}
			return boardLess(matches[i], matches[j])
		})
		for _, it := range matches {
			rows = append(rows, boardRow{item: it, depth: len(it.Folders)})
		}
	} else {
		today := time.Now().Format("2006-01-02")
		for _, ws := range names {
			counts := map[string]int{}
			overdue := 0
			anyVisible := false
			for i := range m.items {
				it := &m.items[i]
				if it.Workstream != ws {
					continue
				}
				counts[it.Status]++
				if it.Due != "" && it.Due < today && it.Status != "done" {
					overdue++
				}
				if m.itemVisible(it, "") {
					anyVisible = true
				}
			}
			if !anyVisible && len(m.directSubfolders(ws, "")) == 0 && !m.collapsed[m.folderKey(ws, "")] {
				continue
			}
			parts := []string{}
			for _, s := range []string{"doing", "todo", "done"} {
				if counts[s] > 0 {
					parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
				}
			}
			if overdue > 0 {
				parts = append(parts, fmt.Sprintf("%d overdue", overdue))
			}
			if m.doneWS[ws] {
				parts = append(parts, "done shown")
			}
			rows = append(rows, boardRow{ws: ws, isWS: true, wsCount: strings.Join(parts, " · ")})
			if len(rows) > 1 {
				rows[len(rows)-1].pre = 1
			}
			if !m.collapsed[m.folderKey(ws, "")] {
				m.emitGroup(ws, "", "", 0, &rows, "")
			}
		}
	}
	m.rows = rows
	m.cursor = clampInt(m.cursor, 0, maxInt(0, len(m.rows)-1))
	m.detailOffset = 0
	m.lineOf = make([]int, len(rows))
	line := 0
	for i := range rows {
		m.lineOf[i] = line + rows[i].pre
		line += rows[i].pre + 1
	}
	m.totalLines = line
	m.ensureVisible()
}

func (m *boardPanelModel) rowsVisible() int {
	w := m.width
	if w <= 0 {
		w = 96
	}
	h := m.height
	if h <= 0 {
		h = 28
	}
	// header(2) + two gaps(2) + footer (may wrap at narrow widths); search costs blank+line(2)
	budget := h - 4 - lipgloss.Height(m.renderFooter(newPaletteStyles(), w))
	if m.searching {
		budget -= 2
	}
	if budget < 3 {
		budget = 3
	}
	return budget
}

func (m *boardPanelModel) ensureVisible() {
	vis := m.rowsVisible()
	if m.cursor >= 0 && m.cursor < len(m.lineOf) {
		cl := m.lineOf[m.cursor]
		if cl < m.offset+1 {
			m.offset = maxInt(0, cl-1)
		}
		if cl >= m.offset+vis {
			m.offset = cl - vis + 1
		}
	}
	if maxOff := m.totalLines - vis; m.offset > maxOff {
		m.offset = maxInt(0, maxOff)
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *boardPanelModel) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	for delta != 0 {
		next := m.cursor + step
		if next < 0 || next >= len(m.rows) {
			break
		}
		m.cursor = next
		delta -= step
	}
	m.detailOffset = 0
	m.ensureVisible()
}

func (m *boardPanelModel) currentItem() *boardItem {
	if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].item != nil {
		return m.rows[m.cursor].item
	}
	return nil
}

func (m *boardPanelModel) leftWidth() int {
	w := m.width
	if w <= 0 {
		w = 96
	}
	lw := (w - 5) * 3 / 5
	if lw > w-35 {
		lw = w - 35
	}
	if lw < 20 {
		lw = 20
	}
	return lw
}

func (m *boardPanelModel) toggleFold() {
	row := m.currentRow()
	key := ""
	switch {
	case row != nil && row.isWS:
		key = m.folderKey(row.ws, "")
	case row != nil && row.isFolder:
		key = m.folderKey(row.ws, row.folderPath)
	case row != nil && row.item != nil && len(row.item.Folders) > 0:
		key = m.folderKey(row.item.Workstream, strings.Join(row.item.Folders, "/"))
	}
	if key == "" {
		return
	}
	if m.collapsed[key] {
		delete(m.collapsed, key)
	} else {
		m.collapsed[key] = true
	}
	m.rebuild()
	// parking on the header keeps t within reach to re-open it
	for i := range m.rows {
		if (m.rows[i].isWS && m.folderKey(m.rows[i].ws, "") == key) ||
			(m.rows[i].isFolder && m.folderKey(m.rows[i].ws, m.rows[i].folderPath) == key) {
			m.cursor = i
			break
		}
	}
	m.detailOffset = 0
	m.ensureVisible()
}

func (m *boardPanelModel) currentRow() *boardRow {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return &m.rows[m.cursor]
	}
	return nil
}

func (m *boardPanelModel) Init() tea.Cmd {
	return func() tea.Msg { return tlPollLive() }
}

func (m *boardPanelModel) handleKey(key string) {
	if key == "tab" && !m.searching {
		m.tab = 1 - m.tab
		return
	}
	if m.tab == 0 {
		if cmd := m.tl.keyTlk(key, []rune(key)); cmd != nil {
			m.pendingCmd = tea.Batch(m.pendingCmd, cmd)
		}
		if m.tl.back {
			m.tl.back = false
			m.requestBack = true
		}
		if m.tl.wantReload {
			m.tl.wantReload = false
			m.reload()
		}
		return
	}
	if m.searching {
		m.handleSearchKey(key)
		return
	}
	vis := m.rowsVisible()
	if m.focusDetail {
		switch key {
		case "u", "up":
			m.detailOffset = maxInt(0, m.detailOffset-1)
			return
		case "e", "down":
			m.detailOffset = minInt(m.maxDetailOffset(), m.detailOffset+1)
			return
		case ",":
			m.detailOffset = maxInt(0, m.detailOffset-5)
			return
		case ".":
			m.detailOffset = minInt(m.maxDetailOffset(), m.detailOffset+5)
			return
		case "ctrl+u":
			m.detailOffset = maxInt(0, m.detailOffset-m.detailVisibleRows()/2)
			return
		case "ctrl+e":
			m.detailOffset = minInt(m.maxDetailOffset(), m.detailOffset+m.detailVisibleRows()/2)
			return
		case "g", "home":
			m.detailOffset = 0
			return
		case "G", "end":
			m.detailOffset = m.maxDetailOffset()
			return
		}
	}
	switch key {
	case "esc", "q", "alt+n":
		m.requestBack = true
	case "r", "R":
		m.reload()
		m.setStatus("reloaded", 1200*time.Millisecond)
	case "u", "up":
		m.moveCursor(-1)
	case "e", "down":
		m.moveCursor(1)
	case ",":
		m.moveCursor(-5)
	case ".":
		m.moveCursor(5)
	case "U":
		m.detailOffset = maxInt(0, m.detailOffset-maxInt(1, m.detailVisibleRows()/2))
	case "E":
		m.detailOffset = minInt(m.maxDetailOffset(), m.detailOffset+maxInt(1, m.detailVisibleRows()/2))
	case "ctrl+u":
		m.moveCursor(-(vis + 1) / 2)
	case "ctrl+e":
		m.moveCursor((vis + 1) / 2)
	case "g", "home":
		m.cursor = 0
		m.offset = 0
		m.detailOffset = 0
	case "G", "end":
		if len(m.rows) > 0 {
			m.cursor = len(m.rows) - 1
			m.detailOffset = 0
			m.ensureVisible()
		}
	case "t", " ", "space", "enter":
		m.toggleFold()
	case "d":
		ws := ""
		if row := m.currentRow(); row != nil {
			if row.isWS {
				ws = row.ws
			} else if row.item != nil {
				ws = row.item.Workstream
			}
		}
		if ws == "" {
			break
		}
		if m.doneWS[ws] {
			delete(m.doneWS, ws)
		} else {
			m.doneWS[ws] = true
		}
		m.rebuild()
		label := "hidden"
		if m.doneWS[ws] {
			label = "shown"
		}
		m.setStatus("done "+label+" in "+ws, 1200*time.Millisecond)
	case "/":
		m.searching = true
		m.query = nil
		m.searchCursor = 0
	case "n", "left":
		m.focusDetail = false
	case "i", "right":
		if m.currentItem() != nil {
			m.focusDetail = true
		}
	}
}

func (m *boardPanelModel) handleSearchKey(key string) {
	switch key {
	case "esc":
		m.searching = false
		m.query = nil
		m.searchCursor = 0
		m.cursor = 0
		m.rebuild()
	case "enter":
		m.searching = false
	case "alt+n", "ctrl+c":
		m.searching = false
		m.query = nil
		m.searchCursor = 0
		m.cursor = 0
		m.rebuild()
	default:
		if applyPaletteInputKey(key, &m.query, &m.searchCursor, false) {
			m.cursor = 0
			m.offset = 0
			m.rebuild()
			return
		}
		runes := []rune(key)
		if len(runes) == 1 && !strings.Contains(key, "+") {
			m.query = append(m.query[:m.searchCursor], append(runes, m.query[m.searchCursor:]...)...)
			m.searchCursor++
			m.cursor = 0
			m.offset = 0
			m.rebuild()
		}
	}
}

func (m *boardPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if msg.Paste {
			for _, r := range msg.Runes {
				m.handleKey(string(r))
			}
			return m, nil
		}
		m.handleKey(msg.String())
	}
	return m, nil
}

func (m *boardPanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func boardStatusGlyph(status string) (string, string) {
	switch status {
	case "doing":
		return "●", "117"
	case "done":
		return "✓", "246"
	case "inbox":
		return "◦", "245"
	}
	return "○", "251"
}

func (m *boardPanelModel) detailLines(styles paletteStyles, it *boardItem, width int) []string {
	if width < 8 {
		width = 8
	}
	out := []string{styles.itemTitle.Render(truncate(it.Title, width))}
	meta := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			out = append(out, styles.statLabel.Render(label)+" "+styles.statValue.Render(truncate(value, maxInt(8, width-len(label)-2))))
		}
	}
	meta("id", it.ID)
	meta("status", it.Status)
	meta("prio", it.Prio)
	meta("owner", it.Owner)
	meta("due", it.Due)
	meta("type", it.Type)
	if len(it.Folders) > 0 {
		meta("folder", strings.Join(it.Folders, " / "))
	}
	meta("path", it.Path)
	body := strings.TrimSpace(it.Body)
	if body != "" {
		out = append(out, "", styles.sectionLabel.Render("Body"))
		bodyStyle := styles.panelText
		if it.Status == "done" {
			bodyStyle = styles.panelTextDone
		}
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimRight(line, " \t")
			if line == "" {
				out = append(out, "")
				continue
			}
			rendered := bodyStyle.Width(width).Render(line)
			out = append(out, strings.Split(rendered, "\n")...)
		}
	} else {
		out = append(out, "", styles.muted.Render("(no body)"))
	}
	return out
}

func (m *boardPanelModel) detailVisibleRows() int {
	return maxInt(3, m.rowsVisible()-1)
}

func (m *boardPanelModel) maxDetailOffset() int {
	if m.width <= 0 {
		return 0
	}
	it := m.currentItem()
	if it == nil {
		return 0
	}
	detailW := m.detailWidth()
	lines := m.detailLines(newPaletteStyles(), it, detailW)
	n := len(lines) - m.detailVisibleRows()
	if n < 0 {
		return 0
	}
	return n
}

func (m *boardPanelModel) detailWidth() int {
	w := m.width
	if w <= 0 {
		w = 96
	}
	left := (w - 5) * 3 / 5
	if left > w-35 {
		left = w - 35
	}
	if left < 20 {
		left = 20
	}
	return w - 5 - left
}

func (m *boardPanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	if m.tab == 0 {
		m.tl.width, m.tl.height = width, height
		return m.tl.render()
	}
	doing, done, overdue := 0, 0, 0
	today := time.Now().Format("2006-01-02")
	for i := range m.items {
		it := &m.items[i]
		switch it.Status {
		case "doing":
			doing++
		case "done":
			done++
		}
		if it.Due != "" && it.Due < today && it.Status != "done" {
			overdue++
		}
	}
	statsLine := fmt.Sprintf("%d items · %d doing · %d done · %d overdue · read-only", len(m.items), doing, done, overdue)
	header := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("Board"),
		styles.meta.Render(statsLine),
	)
	if m.width != width || m.height != height {
		m.width = width
		m.height = height
		m.rebuild()
	}

	searchLine := ""
	if m.searching {
		searchLine = lipgloss.JoinHorizontal(lipgloss.Left,
			styles.searchPrompt.Render(" find "),
			styles.searchBox.Render(renderInputValue(m.query, m.searchCursor, styles)),
			styles.meta.Render("  enter keep · esc clear"),
		)
	}

	leftW := (width - 5) * 3 / 5
	if leftW > width-35 {
		leftW = width - 35
	}
	if leftW < 20 {
		leftW = 20
	}
	detailW := width - 5 - leftW
	clip := func(s string, w int) string {
		return lipgloss.NewStyle().MaxWidth(w).Width(w).Render(s)
	}
	selStyle := styles.selectedItem.MarginBottom(0)
	vis := m.rowsVisible()
	leftLines := []string{}
	if m.loadErr != "" {
		leftLines = append(leftLines, "", styles.statusBad.Width(leftW).Render(truncate(m.loadErr, leftW)),
			styles.meta.Render("~/bin/board export — check ~/base/scripts/board-mcp.py"))
	} else if len(m.rows) == 0 {
		leftLines = append(leftLines, "", styles.muted.Render("(empty)"))
	} else {
		leftAll := make([]string, 0, m.totalLines)
		for i := 0; i < len(m.rows); i++ {
			row := m.rows[i]
			for b := 0; b < row.pre; b++ {
				leftAll = append(leftAll, "")
			}
			selected := i == m.cursor && !m.focusDetail
			if row.isWS {
				wsEmoji := "📂"
				if m.collapsed[m.folderKey(row.ws, "")] {
					wsEmoji = "📁"
				}
				if selected {
					bg := styles.selectedItem.GetBackground()
					gap := lipgloss.NewStyle().Background(bg)
					head := lipgloss.JoinHorizontal(lipgloss.Left, gap.Render(wsEmoji+" "),
						styles.sectionLabel.Background(bg).Render(strings.ToUpper(row.ws)),
						gap.Render("  "), styles.meta.Background(bg).Render(row.wsCount))
					leftAll = append(leftAll, selStyle.Width(leftW).Render(head))
				} else {
					label := styles.sectionLabel.Render(strings.ToUpper(row.ws))
					count := styles.meta.Render(row.wsCount)
					head := lipgloss.JoinHorizontal(lipgloss.Left, wsEmoji+" ", label, "  ", count)
					leftAll = append(leftAll, lipgloss.NewStyle().Padding(0, 1).MaxWidth(leftW).Width(leftW).Render(head))
				}
				continue
			}
			if row.isFolder {
				name := strings.ReplaceAll(row.folderPath[strings.LastIndex(row.folderPath, "/")+1:], "-", " ")
				branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
				emoji := "📂"
				if m.collapsed[m.folderKey(row.ws, row.folderPath)] {
					emoji = "📁"
				}
				avail := leftW - 1 - lipgloss.Width(row.branch) - lipgloss.Width(emoji) - 1 - lipgloss.Width(row.folderCount) - 4
				head := lipgloss.JoinHorizontal(lipgloss.Left, branchStyle.Render(row.branch), emoji+" ",
					styles.sectionLabel.Render(strings.ToUpper(truncate(name, maxInt(4, avail)))), "  ", styles.meta.Render(row.folderCount))
				if selected {
					bg := styles.selectedItem.GetBackground()
					gap := lipgloss.NewStyle().Background(bg)
					head := lipgloss.JoinHorizontal(lipgloss.Left, branchStyle.Background(bg).Render(row.branch),
						gap.Render(emoji+" "),
						styles.sectionLabel.Background(bg).Render(strings.ToUpper(truncate(name, maxInt(4, avail)))),
						gap.Render("  "), styles.meta.Background(bg).Render(row.folderCount))
					leftAll = append(leftAll, selStyle.Width(leftW).Render(head))
				} else {
					leftAll = append(leftAll, lipgloss.NewStyle().MaxWidth(leftW).Width(leftW).PaddingLeft(1).Render(head))
				}
				continue
			}
			it := row.item
			prefix := row.branch
			if prefix == "" {
				prefix = strings.Repeat("  ", row.depth)
			}
			glyph, color := boardStatusGlyph(it.Status)
			glyphStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
			branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
			titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
			if it.Status == "done" {
				titleStyle = styles.panelTextDone
			}
			prioStyle := lipgloss.NewStyle()
			prio := ""
			if it.Prio == "urgent" || it.Prio == "high" {
				prioColor := "180"
				if it.Prio == "urgent" {
					prioColor = "203"
				}
				prioStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(prioColor)).Bold(true)
				prio = "⚑ "
			}
			dueStyle := lipgloss.NewStyle()
			due := ""
			if it.Due != "" && it.Status != "done" {
				if it.Due < today {
					dueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
					due = " ‼" + it.Due
				} else {
					dueStyle = styles.meta
					due = " ·" + it.Due
				}
			}
			used := lipgloss.Width(prefix) + lipgloss.Width(glyph) + 1 + lipgloss.Width(prio) + lipgloss.Width(due) + 2
			title := truncate(it.Title, maxInt(4, leftW-used))
			rowStyle := titleStyle
			if selected {
				// every fragment carries the row background: inner resets would
				// otherwise punch holes in the highlight bar
				bg := styles.selectedItem.GetBackground()
				glyphStyle = glyphStyle.Background(bg)
				prioStyle = prioStyle.Background(bg)
				dueStyle = dueStyle.Background(bg)
				branchStyle = branchStyle.Background(bg)
				rowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(bg)
			}
			plain := branchStyle.Render(prefix) + glyphStyle.Render(glyph+" ") + prioStyle.Render(prio) +
				rowStyle.Render(title) + dueStyle.Render(due)
			if selected {
				leftAll = append(leftAll, selStyle.Width(leftW).Render(plain))
			} else {
				leftAll = append(leftAll, lipgloss.NewStyle().Padding(0, 1).MaxWidth(leftW).Render(plain))
			}
		}
		off := clampInt(m.offset, 0, maxInt(0, len(leftAll)-vis))
		hi := minInt(len(leftAll), off+vis)
		m.offset = off
		leftLines = leftAll[off:hi]
	}
	detailLines := []string{}
	if fr := m.currentRow(); fr != nil && (fr.isFolder || fr.isWS) {
		name := strings.ToUpper(fr.ws)
		path := ""
		if fr.isFolder {
			name = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(fr.folderPath, "/", " / "), "-", " "))
			path = fr.folderPath
		}
		meta := func(k, v string) string {
			return styles.statLabel.Render(k) + " " + styles.statValue.Render(truncate(v, maxInt(8, detailW-len(k)-2)))
		}
		detailLines = append(detailLines, styles.itemTitle.Render(truncate(name, detailW)), "",
			meta("workstream", fr.ws))
		if path != "" {
			detailLines = append(detailLines, meta("path", path))
		}
		counts := fr.wsCount
		if fr.isFolder {
			counts = fr.folderCount
		}
		detailLines = append(detailLines, meta("items", counts))
		if d := strings.TrimSpace(fr.folderDesc); d != "" {
			detailLines = append(detailLines, "", styles.panelTitle.Render("Description"))
			for _, l := range strings.Split(lipgloss.NewStyle().Width(detailW).Render(d), "\n") {
				detailLines = append(detailLines, l)
			}
		}
	} else if it := m.currentItem(); it != nil {
		detailLines = m.detailLines(styles, it, detailW)
		dv := m.detailVisibleRows()
		if m.detailOffset > len(detailLines) {
			m.detailOffset = len(detailLines)
		}
		lo := m.detailOffset
		hi := minInt(len(detailLines), lo+dv)
		detailLines = detailLines[lo:hi]
	} else {
		detailLines = []string{styles.muted.Render("select an item")}
	}
	detailTitle := styles.panelTitle.Render("Detail")
	if m.focusDetail {
		detailTitle = styles.selectedLabel.Render("Detail ▸")
	}
	sep := styles.meta.Render("│")
	bodyRows := maxInt(vis, maxInt(len(leftLines), len(detailLines)+1))
	leftCol := make([]string, bodyRows)
	copy(leftCol, leftLines)
	rightCol := make([]string, bodyRows)
	rightCol[0] = detailTitle
	copy(rightCol[1:], detailLines)
	for i := 0; i < bodyRows; i++ {
		leftCol[i] = clip(leftCol[i], leftW)
		rightCol[i] = clip(rightCol[i], detailW)
	}
	var bodyLines []string
	for i := 0; i < bodyRows; i++ {
		bodyLines = append(bodyLines, lipgloss.JoinHorizontal(lipgloss.Left, leftCol[i], " ", sep, " ", rightCol[i]))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, bodyLines...)
	footer := m.renderFooter(styles, width)
	parts := []string{header}
	if searchLine != "" {
		parts = append(parts, "", searchLine)
	}
	parts = append(parts, "", lipgloss.NewStyle().MaxWidth(width-2).Render(body), "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (m *boardPanelModel) renderFooter(styles paletteStyles, width int) string {
	status := strings.TrimSpace(m.currentStatus())
	renderSegments := func(pairs [][2]string) string {
		return renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "   ", pairs)
	}
	var pairs [][2]string
	if m.searching {
		pairs = [][2]string{{"type", "filter"}, {"enter", "keep"}, {"esc", "clear"}}
	} else if m.focusDetail {
		pairs = [][2]string{{"u/e", "scroll detail"}, {"n", "list"}, {"d", "done"}, {"/", "find"}, {"r", "reload"}, {"q", "back"}}
	} else {
		pairs = [][2]string{{"u/e", "move"}, {"ctrl+u/e", "half"}, {"U/E", "detail"}, {"t", "fold"}, {"d", "done"}, {"/", "find"}, {"r", "reload"}, {"q", "back"}}
	}
	footer := pickRenderedShortcutFooter(width, renderSegments, pairs)
	if status != "" {
		statusText := styles.statusBad.Render(truncate(status, maxInt(12, minInt(36, width/3))))
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

func (m *boardPanelModel) setStatus(text string, duration time.Duration) {
	m.status = text
	m.statusUntil = time.Now().Add(duration)
}

func (m *boardPanelModel) currentStatus() string {
	if m.status == "" {
		return ""
	}
	if !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
		m.status = ""
		return ""
	}
	return m.status
}
