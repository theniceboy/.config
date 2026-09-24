package main

// Timeline view of the board — the DEV-28 horizontal bar view with DEV-47 live
// work-state markers. Ported from the approved 5-TT prototype; renders as tab 0
// of the board panel, the pre-overhaul tree browser stays as tab 1.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const (
	dayW   = 4
	labelW = 39
	panelW = 52
)

const (
	fTitle = iota
	fStatus
	fPrio
	fOwner
	fStart
	fDue
	fNote
	fCount
)

type tlItem struct {
	ID     string
	Title  string
	Desc   string
	Stream string
	Folder string
	Status string
	Prio   string
	Owner  string
	Start  *time.Time
	Due    *time.Time
	Ses    []string
}

func (d *tlItem) adjust(field, dd int, today time.Time) {
	if field == 0 {
		if d.Start == nil {
			base := today
			if d.Due != nil {
				base = *d.Due
			}
			t := base.AddDate(0, 0, dd)
			d.Start = &t
			return
		}
		t := d.Start.AddDate(0, 0, dd)
		d.Start = &t
		return
	}
	if d.Due == nil {
		base := today
		if d.Start != nil {
			base = *d.Start
		}
		t := base.AddDate(0, 0, dd)
		d.Due = &t
		return
	}
	t := d.Due.AddDate(0, 0, dd)
	d.Due = &t
}

func (d *tlItem) slide(dd int) {
	if d.Start == nil && d.Due == nil {
		return
	}
	if d.Start != nil {
		t := d.Start.AddDate(0, 0, dd)
		d.Start = &t
	}
	if d.Due != nil {
		t := d.Due.AddDate(0, 0, dd)
		d.Due = &t
	}
}

type tlModel struct {
	items      []tlItem
	streams    []string
	selID      string
	origin     time.Time
	today      time.Time
	editing    bool
	typing     bool
	focus      int
	draft      tlItem
	input      string
	msg        string
	calOpen    bool
	calMonth   time.Time
	calSel     time.Time
	vscroll    int
	showAll    bool
	pickOpen   bool
	pickIdx    int
	dateMode   bool
	dateID     string
	sesPane    map[string]string
	tasks      map[string]liveTask
	paneWin    map[string]winInfo
	jumpOpen   bool
	jumpIdx    int
	lastPoll   time.Time
	orig       tlItem
	back       bool
	wantReload bool
	dStart     *time.Time
	dDue       *time.Time
	width      int
	height     int
}

var (
	stOverdue = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	stActive  = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	stSoon    = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	stBold    = lipgloss.NewStyle().Bold(true)
	stToday   = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	stWork    = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	stID      = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	selBg     = lipgloss.NewStyle().Background(lipgloss.Color("236"))
)

func dayOff(base time.Time, n int) *time.Time {
	t := base.AddDate(0, 0, n)
	return &t
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

type liveTask struct {
	Session      string    `json:"session"`
	WindowID     string    `json:"window_id"`
	Window       string    `json:"window"`
	Pane         string    `json:"pane"`
	Status       string    `json:"status"`
	Phase        string    `json:"phase"`
	Acknowledged bool      `json:"acknowledged"`
	StartedAt    time.Time `json:"started_at"`
}

type winInfo struct {
	WindowID string
	Window   string
	Session  string
	Index    string
}

type tlLiveMsg struct {
	sesPane map[string]string
	tasks   map[string]liveTask
	paneWin map[string]winInfo
}

func tlPollLive() tea.Msg {
	sesPane := map[string]string{}
	if ms, err := filepath.Glob(os.Getenv("HOME") + "/.local/state/op/ses_ses_*"); err == nil {
		cut := time.Now().Add(-90 * time.Second).UnixMilli()
		for _, p := range ms {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var d struct {
				SessionID string `json:"sessionID"`
				Heartbeat int64  `json:"heartbeat"`
				Pane      struct {
					PaneId string `json:"paneId"`
				} `json:"pane"`
			}
			if json.Unmarshal(b, &d) != nil || d.Heartbeat < cut || d.Pane.PaneId == "" {
				continue
			}
			sesPane[d.SessionID] = d.Pane.PaneId
		}
	}
	tasks := map[string]liveTask{}
	if out, err := exec.Command("agent", "tracker", "state").Output(); err == nil {
		var arr []liveTask
		if json.Unmarshal(out, &arr) == nil {
			for _, t := range arr {
				tasks[t.Pane] = t
			}
		} else {
			var w struct {
				Tasks []liveTask `json:"tasks"`
			}
			if json.Unmarshal(out, &w) == nil {
				for _, t := range w.Tasks {
					tasks[t.Pane] = t
				}
			}
		}
	}
	paneWin := map[string]winInfo{}
	if out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id}\t#{window_id}\t#{window_name}\t#{session_name}\t#{window_index}").Output(); err == nil {
		for _, ln := range strings.Split(string(out), "\n") {
			f := strings.Split(ln, "\t")
			if len(f) == 5 {
				paneWin[f[0]] = winInfo{WindowID: f[1], Window: f[2], Session: f[3], Index: f[4]}
			}
		}
	}
	return tlLiveMsg{sesPane: sesPane, tasks: tasks, paneWin: paneWin}
}

func jumpCmd(w winInfo) tea.Cmd {
	target := w.Session + ":" + w.Index
	return tea.Batch(
		func() tea.Msg {
			exec.Command("tmux", "switch-client", "-t", target).Run()
			return nil
		},
		tea.Quit,
	)
}

type linkInfo struct {
	pane    string
	task    liveTask
	hasTask bool
	win     winInfo
}

func (m tlModel) itemLinks(it tlItem) []linkInfo {
	var ls []linkInfo
	seen := map[string]bool{}
	for _, sid := range it.Ses {
		pane, ok := m.sesPane[sid]
		if !ok || seen[pane] {
			continue
		}
		seen[pane] = true
		w, ok := m.paneWin[pane]
		if !ok {
			continue
		}
		t, has := m.tasks[pane]
		ls = append(ls, linkInfo{pane: pane, task: t, hasTask: has, win: w})
	}
	return ls
}

func linkState(l linkInfo) string {
	if !l.hasTask {
		return "i"
	}
	switch {
	case l.task.Phase == "question":
		return "q"
	case l.task.Status == "in_progress":
		return "w"
	case l.task.Status == "completed" && !l.task.Acknowledged:
		return "n"
	}
	return "i"
}

func stateRank(s string) int {
	switch s {
	case "q":
		return 3
	case "w":
		return 2
	case "n":
		return 1
	}
	return 0
}

func bestLink(ls []linkInfo) *linkInfo {
	bi := 0
	for i := range ls {
		if stateRank(linkState(ls[i])) > stateRank(linkState(ls[bi])) {
			bi = i
		}
	}
	return &ls[bi]
}

func stateEmoji(s string) string {
	switch s {
	case "q":
		return "❓"
	case "w":
		return "⏳"
	case "n":
		return "🔔"
	}
	return "·"
}

func (m tlModel) selLinks() []linkInfo {
	if it := m.selItem(); it != nil {
		return m.itemLinks(*it)
	}
	return nil
}

func sinceShort(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 0:
		d = 0
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

type tlEditDoneMsg struct {
	path  string
	field int
}

func pickVals(f int) []string {
	switch f {
	case fStatus:
		return []string{"todo", "doing", "done"}
	case fPrio:
		return []string{"urgent", "high", "normal", "low"}
	case fOwner:
		return []string{"operator", "engineers", "-"}
	}
	return nil
}

func (m *tlModel) applyPickVal(v string) {
	switch m.focus {
	case fStatus:
		m.draft.Status = v
	case fPrio:
		m.draft.Prio = v
	case fOwner:
		m.draft.Owner = v
	}
}

func (m tlModel) curPickVal() string {
	switch m.focus {
	case fStatus:
		return m.draft.Status
	case fPrio:
		return m.draft.Prio
	case fOwner:
		return m.draft.Owner
	}
	return ""
}

func (m *tlModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tlEditDoneMsg:
		if b, err := os.ReadFile(msg.path); err == nil {
			txt := strings.TrimSpace(string(b))
			if msg.field == fTitle {
				m.draft.Title = strings.Split(txt, "\n")[0]
			} else if msg.field == fNote {
				m.draft.Desc = txt
			}
		}
		os.Remove(msg.path)
		return nil
	case tlLiveMsg:
		m.sesPane, m.tasks, m.paneWin = msg.sesPane, msg.tasks, msg.paneWin
		return nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return nil
	case tea.KeyMsg:
		return m.keyTlk(msg.String(), msg.Runes)
	}
	return nil
}

func (m *tlModel) enterDateMode() {
	it := m.selItem()
	if it == nil {
		return
	}
	m.dateMode = true
	m.dateID = it.ID
	m.dStart, m.dDue = it.Start, it.Due
	if m.dStart == nil && m.dDue == nil {
		a, b := m.today, m.today
		m.dStart, m.dDue = &a, &b
	} else if m.dStart == nil {
		t := *m.dDue
		m.dStart = &t
	} else if m.dDue == nil {
		t := *m.dStart
		m.dDue = &t
	}
}

func (m *tlModel) shiftDates(ds, dd int) {
	if ds != 0 && m.dStart != nil {
		t := m.dStart.AddDate(0, 0, ds)
		m.dStart = &t
	}
	if dd != 0 && m.dDue != nil {
		t := m.dDue.AddDate(0, 0, dd)
		m.dDue = &t
	}
	m.ensureDraftVisible()
}

func (m *tlModel) ensureDraftVisible() {
	vis := (m.bodyWidth() - labelW - 4) / dayW
	if vis < 7 {
		vis = 7
	}
	if m.dDue != nil {
		off := int(m.dDue.Sub(m.origin).Hours() / 24)
		if off > vis-1 {
			m.origin = m.origin.AddDate(0, 0, off-(vis-1))
		}
	}
	if m.dStart != nil {
		off := int(m.dStart.Sub(m.origin).Hours() / 24)
		if off < 0 {
			m.origin = m.origin.AddDate(0, 0, off)
		}
	}
}

func (m *tlModel) commitDates() {
	args := []string{"edit", m.dateID, "start=" + tlDateStr(m.dStart), "due=" + tlDateStr(m.dDue)}
	if out, err := boardRun(args...); err != nil {
		m.msg = "commit failed: " + strings.TrimSpace(string(out))
		return
	}
	for i := range m.items {
		if m.items[i].ID == m.dateID {
			m.items[i].Start, m.items[i].Due = m.dStart, m.dDue
			break
		}
	}
	m.dateMode, m.msg = false, ""
	m.wantReload = true
}

func (m *tlModel) syncCal() {
	if m.calSel.Month() != m.calMonth.Month() || m.calSel.Year() != m.calMonth.Year() {
		m.calMonth = time.Date(m.calSel.Year(), m.calSel.Month(), 1, 0, 0, 0, 0, m.calSel.Location())
	}
}

func (m *tlModel) moveFocus(dr, dc int) {
	rows := [][]int{
		{fTitle},
		{fStatus, fPrio, fOwner},
		{fStart, fDue},
		{fNote},
	}
	r, c := 0, 0
	for ri, row := range rows {
		for ci, f := range row {
			if f == m.focus {
				r, c = ri, ci
			}
		}
	}
	nr, nc := r+dr, c+dc
	if nr < 0 {
		nr = 0
	}
	if nr >= len(rows) {
		nr = len(rows) - 1
	}
	if nc < 0 {
		nc = 0
	}
	if nc >= len(rows[nr]) {
		nc = len(rows[nr]) - 1
	}
	m.focus = rows[nr][nc]
}

func (m *tlModel) commit() {
	args := []string{"edit", m.draft.ID}
	if m.draft.Title != m.orig.Title {
		args = append(args, "title="+m.draft.Title)
	}
	if m.draft.Status != m.orig.Status {
		args = append(args, "status="+m.draft.Status)
	}
	if m.draft.Prio != m.orig.Prio {
		args = append(args, "prio="+m.draft.Prio)
	}
	if m.draft.Owner != m.orig.Owner {
		args = append(args, "owner="+m.draft.Owner)
	}
	if tlDateStr(m.draft.Start) != tlDateStr(m.orig.Start) {
		args = append(args, "start="+tlDateStr(m.draft.Start))
	}
	if tlDateStr(m.draft.Due) != tlDateStr(m.orig.Due) {
		args = append(args, "due="+tlDateStr(m.draft.Due))
	}
	if m.draft.Desc != m.orig.Desc {
		args = append(args, "body="+m.draft.Desc)
	}
	if len(args) == 2 {
		m.editing, m.msg = false, ""
		return
	}
	if out, err := boardRun(args...); err != nil {
		m.msg = "commit failed: " + strings.TrimSpace(string(out))
		return
	}
	m.editing, m.msg = false, ""
	m.wantReload = true
}

func parseDateRef(s string, today time.Time) (*time.Time, string) {
	v := strings.ToLower(strings.TrimSpace(s))
	if v == "" || v == "none" || v == "x" || v == "-" {
		return nil, ""
	}
	if v == "t" || v == "today" || v == "=" {
		t := today
		return &t, ""
	}
	if strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") {
		n, err := strconv.Atoi(v)
		if err != nil || n == 0 {
			return nil, "bad offset: " + s
		}
		t := today.AddDate(0, 0, n)
		return &t, ""
	}
	if len(v) == 5 && v[2] == '-' {
		v = fmt.Sprintf("%04d-%s", today.Year(), v)
	}
	t, err := time.ParseInLocation("2006-01-02", v, today.Location())
	if err != nil {
		return nil, "can't parse: " + s
	}
	return &t, ""
}

func (m tlModel) selItem() *tlItem {
	for i := range m.items {
		if m.items[i].ID == m.selID {
			return &m.items[i]
		}
	}
	return nil
}

func (m tlModel) visDays() int {
	vis := (m.bodyWidth() - labelW - 4) / dayW
	if vis < 7 {
		vis = 7
	}
	return vis
}

func (m tlModel) tlVisible(it tlItem) bool {
	if len(m.itemLinks(it)) > 0 {
		return true
	}
	if it.Due == nil && it.Start == nil && !m.showAll {
		return false
	}
	if it.Due != nil && it.Due.Before(m.today) {
		return true
	}
	lo, hi := it.Start, it.Due
	if lo == nil {
		lo = hi
	}
	if hi == nil {
		hi = lo
	}
	if lo == nil {
		return true
	}
	vis := m.visDays()
	li := int(lo.Sub(m.origin).Hours() / 24)
	hib := int(hi.Sub(m.origin).Hours() / 24)
	return hib >= -30 && li <= vis+29
}

func (m tlModel) order() []string {
	ids := []string{}
	for _, r := range m.rows() {
		if r.kind == 2 {
			ids = append(ids, r.it.ID)
		}
	}
	return ids
}

func (m *tlModel) move(delta int) {
	ids := m.order()
	for i, id := range ids {
		if id == m.selID {
			j := i + delta
			if j < 0 {
				j = 0
			}
			if j >= len(ids) {
				j = len(ids) - 1
			}
			m.selID = ids[j]
			m.followSel(delta)
			return
		}
	}
	if len(ids) > 0 {
		m.selID = ids[0]
		m.followSel(delta)
	}
}

func (m tlModel) selRow() int {
	for i, r := range m.rows() {
		if r.kind == 2 && r.it.ID == m.selID {
			return i
		}
	}
	return 0
}

func (m *tlModel) followSel(delta int) {
	row := m.selRow()
	vis := m.height - 8
	if vis < 1 {
		vis = 1
	}
	const off = 2
	if delta < 0 && row < m.vscroll+off {
		m.vscroll = row - off
	}
	if row >= m.vscroll+vis-off {
		m.vscroll = row - vis + off + 1
	}
	if row < m.vscroll {
		m.vscroll = row
	}
	if m.vscroll < 0 {
		m.vscroll = 0
	}
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func fmtDate(t time.Time) string { return t.Format("01-02") }

func trunc(s string, w int) string {
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

func pad(s string, w int) string {
	gap := w - lipgloss.Width(s)
	if gap < 0 {
		gap = 0
	}
	return s + strings.Repeat(" ", gap)
}

func boardTabBar(active int, width int, live string, showAll bool, today time.Time) string {
	l := " Board   "
	if active == 0 {
		l += "● Timeline   ○ Tree"
	} else {
		l += "○ Timeline   ● Tree"
	}
	if showAll {
		l += "   [all]"
	}
	l += live
	date := today.Format("Mon 2006-01-02")
	return stBold.Render(pad(l, width-17-len(date))) + stDim.Render(date)
}

func (m tlModel) header() string {
	cnt := map[string]int{}
	for _, it := range m.items {
		if ls := m.itemLinks(it); len(ls) > 0 {
			cnt[linkState(*bestLink(ls))]++
		}
	}
	live := ""
	for _, pair := range [][2]string{{"q", "❓"}, {"w", "⏳"}, {"n", "🔔"}} {
		if cnt[pair[0]] > 0 {
			live += fmt.Sprintf("  %s%d", pair[1], cnt[pair[0]])
		}
	}
	if cnt["i"] > 0 {
		live += stDim.Render(fmt.Sprintf("  ·%d", cnt["i"]))
	}
	return boardTabBar(0, m.width, live, m.showAll, m.today)
}

func (m tlModel) bodyWidth() int {
	if m.editing {
		return m.width - panelW - 2
	}
	return m.width
}

func (m *tlModel) render() string {
	if m.width == 0 {
		return "loading…"
	}
	var body []string
	var pre []string
	pre = m.renderRuler()
	body = m.renderTimeline()
	if m.vscroll > 0 {
		if m.vscroll >= len(body) {
			body = nil
		} else {
			body = body[m.vscroll:]
		}
	}
	if m.editing {
		h := m.height - 1 - len(pre)
		if h < 1 {
			h = 1
		}
		if len(body) > h {
			body = body[:h]
		}
		fill := ""
		fill = m.markerLine()
		for len(body) < h {
			body = append(body, fill)
		}
		left := strings.Join(append(append([]string{}, pre...), body...), "\n")
		return m.header() + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, m.renderEditPanel())
	}
	bottom := m.footer()
	avail := m.height - 1 - len(pre) - lipgloss.Height(bottom)
	if avail < 1 {
		avail = 1
	}
	if len(body) > avail {
		body = body[:avail]
	}
	fill := m.markerLine()
	for len(body) < avail {
		body = append(body, fill)
	}
	left := strings.Join(append(append([]string{}, pre...), body...), "\n")
	if m.jumpOpen {
		var pop []string
		for i, l := range m.selLinks() {
			mk := "  "
			if i == m.jumpIdx {
				mk = "▶ "
			}
			ph := ""
			switch linkState(l) {
			case "q":
				ph = "waiting on you"
			case "w":
				ph = "working " + sinceShort(l.task.StartedAt)
			case "n":
				ph = "done · unread"
			}
			row := mk + stateEmoji(linkState(l)) + " " + pad(l.win.Window, 16) + stDim.Render("@"+l.win.Session)
			if ph != "" {
				row += " " + ph
			}
			pop = append(pop, row)
		}
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			Render(strings.Join(pop, "\n"))
		lines := strings.Split(left, "\n")
		for i, pl := range strings.Split(box, "\n") {
			r := 3 + i
			for len(lines) <= r {
				lines = append(lines, "")
			}
			lines[r] = strings.Repeat(" ", 8) + pl
		}
		left = strings.Join(lines, "\n")
	}
	return m.header() + "\n" + left + "\n" + bottom
}

func (m tlModel) footer() string {
	it := m.selItem()
	dat := "no selection"
	if m.dateMode {
		s, d := "—", "—"
		if m.dStart != nil {
			s = fmtDate(*m.dStart)
		}
		if m.dDue != nil {
			d = fmtDate(*m.dDue)
		}
		dat = fmt.Sprintf("%s · %s → %s", m.dateID, s, d)
	} else if it != nil {
		seg := ""
		switch {
		case it.Start != nil && it.Due != nil:
			seg = " · " + fmtDate(*it.Start) + " → " + fmtDate(*it.Due)
		case it.Start != nil:
			seg = " · start " + fmtDate(*it.Start)
		case it.Due != nil:
			seg = " · due " + fmtDate(*it.Due)
		}
		dat = fmt.Sprintf("%s · %s · %s%s", it.ID, it.Stream, it.Prio, seg)
		if ls := m.itemLinks(*it); len(ls) > 0 {
			b := bestLink(ls)
			st := linkState(*b)
			extra := " · " + stateEmoji(st) + " " + b.win.Window + " @" + b.win.Session
			switch st {
			case "q":
				extra += " · waiting on you"
			case "w":
				ph := b.task.Phase
				if ph == "" {
					ph = "working"
				}
				extra += " · " + ph + " " + sinceShort(b.task.StartedAt)
			case "n":
				extra += " · done · unack"
			}
			if len(ls) > 1 {
				extra += fmt.Sprintf(" +%d more", len(ls)-1)
			}
			dat += stActive.Render(extra)
		}
	}
	msg := ""
	if m.msg != "" {
		msg = stOverdue.Render("  ⚠ " + m.msg)
	}
	help := " u/e sel · ,/. ±5 rows · ctrl+u/e pg · n/i scroll days · t today · a all · L live · m jump · Enter edit · d dates · q quit"
	if m.dateMode {
		dat = stToday.Render("DATE ") + dat
		help = " n/i scroll · N/I move block · [ ] extend · { } shrink · c commit · Esc cancel"
	}
	help = lipgloss.NewStyle().MaxWidth(m.width).Render(stDim.Render(help))
	head := ""
	if it := m.selItem(); it != nil {
		head = " " + stBold.Render(trunc(it.Title, m.width-2)) + "\n"
	}
	return "\n" + head + " " + dat + msg + "\n" + help
}

func (m tlModel) renderEditPanel() string {
	dateVal := func(t *time.Time) string {
		if t == nil {
			return "—"
		}
		return t.Format("Mon 2006-01-02")
	}
	focused := func(f int) bool { return m.focus == f && !m.typing && !m.calOpen }
	block := func(val string, f int) string {
		wl := strings.Split(lipgloss.NewStyle().Width(panelW-8).Render(val), "\n")
		var out []string
		for k, ln := range wl {
			switch {
			case k == 0 && focused(f):
				out = append(out, "▶ "+stBold.Render(ln))
			case k == 0:
				out = append(out, "  "+ln)
			case focused(f):
				out = append(out, "  "+stBold.Render(ln))
			default:
				out = append(out, "  "+ln)
			}
		}
		return strings.Join(out, "\n")
	}

	statusStyles := map[string]lipgloss.Style{
		"todo": stDim, "doing": stActive, "done": stSoon,
	}
	prioStyles := map[string]lipgloss.Style{
		"urgent": stOverdue, "high": stToday, "normal": stID, "low": stDim,
	}
	ownerStyles := map[string]lipgloss.Style{
		"operator":  lipgloss.NewStyle().Foreground(lipgloss.Color("75")),
		"engineers": lipgloss.NewStyle().Foreground(lipgloss.Color("183")),
	}
	statusSym := map[string]string{"todo": "○", "doing": "●", "done": "✓"}
	prioSym := map[string]string{"urgent": "‼", "high": "▲", "normal": "·", "low": "▾"}

	pick := func(mp map[string]lipgloss.Style, k string) lipgloss.Style {
		if st, ok := mp[k]; ok {
			return st
		}
		return stID
	}
	pill := func(txt string, f int, st lipgloss.Style) string {
		if focused(f) {
			st = st.Reverse(true).Bold(true)
		}
		return st.Render(" " + txt + " ")
	}
	pillSeg := func(f int, cur string, syms map[string]string, styles map[string]lipgloss.Style) string {
		txt := cur
		if s, ok := syms[cur]; ok {
			txt = s + " " + cur
		}
		return pill(txt, f, pick(styles, cur))
	}
	segs := []string{
		pillSeg(fStatus, m.draft.Status, statusSym, statusStyles),
		pillSeg(fPrio, m.draft.Prio, prioSym, prioStyles),
		pillSeg(fOwner, m.draft.Owner, nil, ownerStyles),
	}
	metaLine := strings.Join(segs, " ")
	menuLines := []string{}
	if m.pickOpen {
		var syms map[string]string
		var styles map[string]lipgloss.Style
		switch m.focus {
		case fStatus:
			syms, styles = statusSym, statusStyles
		case fPrio:
			syms, styles = prioSym, prioStyles
		case fOwner:
			syms, styles = nil, ownerStyles
		}
		for k, v := range pickVals(m.focus) {
			txt := v
			if s, ok := syms[v]; ok {
				txt = s + " " + v
			}
			st := pick(styles, v)
			if k == m.pickIdx {
				menuLines = append(menuLines, "▶ "+st.Bold(true).Render(txt))
			} else {
				menuLines = append(menuLines, "  "+st.Render(txt))
			}
		}
	}

	dateCell := func(label string, t *time.Time, f int) string {
		if focused(f) {
			return "▶ " + stBold.Render(label+" "+dateVal(t))
		}
		return "  " + stDim.Render(label) + " " + dateVal(t)
	}
	sep := stDim.Render(strings.Repeat("─", panelW-8))
	var lines []string
	addBlock := func(s string) {
		lines = append(lines, strings.Split(s, "\n")...)
	}
	lines = append(lines,
		"",
		" "+stBold.Render("EDIT")+" "+stID.Render(m.draft.ID),
		"",
	)
	addBlock(block(m.draft.Title, fTitle))
	lines = append(lines,
		"",
		" "+metaLine,
	)
	metaRow := len(lines) - 1
	lines = append(lines,
		"",
		sep,
		" "+dateCell("start", m.draft.Start, fStart)+"   "+dateCell("due", m.draft.Due, fDue),
	)
	dateRow := len(lines) - 1
	lines = append(lines,
		sep,
		"",
	)
	addBlock(block(m.draft.Desc, fNote))
	lines = append(lines, "")
	if m.typing {
		lines = append(lines, " type: "+m.input+"▏")
	}
	if m.msg != "" {
		lines = append(lines, " "+stOverdue.Render("⚠ "+m.msg))
	}
	overlay := func(row, col int, pop []string) {
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			Render(strings.Join(pop, "\n"))
		for i, pl := range strings.Split(box, "\n") {
			r := row + i
			for len(lines) <= r {
				lines = append(lines, "")
			}
			lines[r] = strings.Repeat(" ", col) + pl
		}
	}
	if m.pickOpen {
		prefix := 1
		fieldCol := map[int]int{fStatus: 0, fPrio: 1, fOwner: 2}
		for k, sg := range segs {
			if k < fieldCol[m.focus] {
				prefix += lipgloss.Width(sg) + 1
			}
		}
		overlay(metaRow+1, prefix, menuLines)
	}
	if m.calOpen {
		overlay(dateRow+2, 4, m.renderCalendar())
	}
	h := m.height - 1
	for len(lines) > h-1 {
		lines = lines[:len(lines)-1]
	}
	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	bottom := " c commit · Esc discard"
	switch {
	case m.typing:
		bottom = " Enter ✓ apply · Esc cancel"
	case m.pickOpen:
		bottom = " n/i move · Enter pick · Esc back"
	case m.calOpen:
		bottom = " u/e/n/i · ,/. month · t today · Enter pick"
	}
	out := strings.Join(lines, "\n") + "\n" + stDim.Render(trunc(bottom, panelW-5))
	return lipgloss.NewStyle().
		PaddingLeft(3).
		Width(panelW).
		Render(out)
}

func (m tlModel) renderCalendar() []string {
	first := time.Date(m.calMonth.Year(), m.calMonth.Month(), 1, 0, 0, 0, 0, m.calMonth.Location())
	lines := []string{
		" " + stBold.Render(first.Format("January 2006")) + stDim.Render("  ,/. month"),
		" " + stDim.Render("Su  Mo  Tu  We  Th  Fr  Sa"),
	}
	day := first.AddDate(0, 0, -int(first.Weekday()))
	stCalSel := lipgloss.NewStyle().Reverse(true).Bold(true)
	for w := 0; w < 6; w++ {
		row := " "
		for dcol := 0; dcol < 7; dcol++ {
			cell := fmt.Sprintf("%-4d", day.Day())
			switch {
			case sameDay(day, m.calSel):
				row += stCalSel.Render(cell)
			case sameDay(day, m.today):
				row += stToday.Render(cell)
			case day.Month() != first.Month():
				row += stDim.Render(cell)
			default:
				row += cell
			}
			day = day.AddDate(0, 0, 1)
		}
		lines = append(lines, row)
	}
	return lines
}

func selWrap(sel bool, s string) string {
	if !sel {
		return s
	}
	return selBg.Render(s)
}

func (m tlModel) todayCol() (int, bool) {
	vis := (m.bodyWidth() - labelW - 4) / dayW
	if vis < 7 {
		vis = 7
	}
	diff := int(m.today.Sub(m.origin).Hours() / 24)
	if diff < 0 || diff >= vis {
		return 0, false
	}
	return labelW + 1 + diff*dayW + 1, true
}

func (m tlModel) markerLine() string {
	col, ok := m.todayCol()
	if !ok {
		return ""
	}
	return strings.Repeat(" ", col) + stToday.Render("│")
}

func (m tlModel) renderRuler() []string {
	vis := (m.bodyWidth() - labelW - 4) / dayW
	if vis < 7 {
		vis = 7
	}
	wd := strings.Repeat(" ", labelW+1)
	dn := strings.Repeat(" ", labelW+1)
	for i := 0; i < vis; i++ {
		d := m.origin.AddDate(0, 0, i)
		w := d.Format("Mon")[:2]
		wCell := fmt.Sprintf("%-4s", w)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			wd += stDim.Render(wCell)
		} else {
			wd += wCell
		}
		numCell := fmt.Sprintf("%-4s", strconv.Itoa(d.Day()))
		switch {
		case sameDay(d, m.today):
			dn += stToday.Render(numCell)
		case d.Day() == 1:
			dn += stToday.Render(fmt.Sprintf("%-4s", d.Format("Jan")))
		default:
			dn += numCell
		}
	}
	return []string{wd, dn, m.markerLine(), m.markerLine()}
}

func fmtID(id string) string {
	code, num := id, ""
	if p := strings.SplitN(id, "-", 2); len(p) == 2 {
		code, num = p[0], p[1]
	}
	if len(code) > 3 {
		code = code[:3]
	}
	if len(num) > 3 {
		num = num[:3]
	}
	s := code
	if num != "" {
		s += "-" + num
	}
	return fmt.Sprintf("%-8s", s)
}

type tlRow struct {
	kind  int // 0 stream header, 1 folder header, 2 item
	depth int
	label string
	it    *tlItem
}

// rows builds the canonical display tree: stream header, then folder
// headers/items nested by folder path. Item depth = folder nesting level;
// rendering, cursor order and row tracking all consume this one sequence.
func (m tlModel) rows() []tlRow {
	var out []tlRow
	for _, s := range m.streams {
		type fnode struct {
			kids  map[string]*fnode
			order []string
			items []*tlItem
		}
		root := &fnode{kids: map[string]*fnode{}}
		count := 0
		for i := range m.items {
			it := &m.items[i]
			if it.Stream != s || !m.tlVisible(*it) {
				continue
			}
			count++
			cur := root
			if it.Folder != "" {
				for _, c := range strings.Split(it.Folder, "/") {
					if cur.kids[c] == nil {
						cur.kids[c] = &fnode{kids: map[string]*fnode{}}
						cur.order = append(cur.order, c)
					}
					cur = cur.kids[c]
				}
			}
			cur.items = append(cur.items, it)
		}
		if count == 0 {
			continue
		}
		out = append(out, tlRow{kind: 0, label: s})
		var walk func(nd *fnode, depth int)
		walk = func(nd *fnode, depth int) {
			for _, it := range nd.items {
				out = append(out, tlRow{kind: 2, depth: depth, it: it})
			}
			for _, c := range nd.order {
				k := nd.kids[c]
				out = append(out, tlRow{kind: 1, depth: depth + 1, label: c})
				walk(k, depth+1)
			}
		}
		walk(root, 0)
	}
	return out
}

func (m tlModel) renderTimeline() []string {
	vis := (m.bodyWidth() - labelW - 4) / dayW
	if vis < 7 {
		vis = 7
	}
	var L []string
	for _, r := range m.rows() {
		if r.kind == 0 {
			hdr := " " + stBold.Render(strings.ToUpper(r.label))
			if col, ok := m.todayCol(); ok {
				hdr = pad(hdr, col) + stToday.Render("│")
			}
			L = append(L, hdr)
			continue
		}
		if r.kind == 1 {
			sub := strings.Repeat(" ", 1+2*r.depth) + stDim.Render(strings.ToUpper(r.label)+" ▼")
			if col, ok := m.todayCol(); ok {
				sub = pad(sub, col) + stToday.Render("│")
			}
			L = append(L, sub)
			continue
		}
		{
			it := *r.it
			ind := strings.Repeat(" ", 3+2*r.depth)
			sel := it.ID == m.selID
			idTxt := fmtID(it.ID)
			mark := "  "
			if ls := m.itemLinks(it); len(ls) > 0 {
				st := linkState(*bestLink(ls))
				sty := map[string]lipgloss.Style{"q": stOverdue, "w": stWork, "n": stSoon, "i": stDim}[st]
				ch := "•"
				if sel {
					ch = "▶"
				}
				mark = sty.Render(ch) + " "
			} else if sel {
				mark = "▶ "
			}
			tw := labelW + 1 - (3 + 2*r.depth) - 10
			if tw > 24 {
				tw = 24
			}
			if tw < 8 {
				tw = 8
			}
			titTxt := trunc(it.Title, tw)
			if it.Due != nil && it.Due.Before(m.today) {
				st := stOverdue
				if sel {
					st = st.Bold(true)
				}
				idTxt = st.Render(idTxt)
				titTxt = st.Render(titTxt)
			} else if sel {
				titTxt = stBold.Render(titTxt)
			}
			grid := ""
			ghost := m.dateMode && it.ID == m.dateID
			lo, hi := it.Start, it.Due
			if ghost {
				lo, hi = m.dStart, m.dDue
			}
			spanCh, solidCh := "▓", "█"
			if ghost {
				spanCh, solidCh = "⣿", "⣿"
			}
			if lo == nil && hi != nil {
				lo = hi
			}
			loIdx, hiIdx := 1<<30, 1<<30
			if lo != nil {
				loIdx = int(lo.Sub(m.origin).Hours() / 24)
			}
			if hi != nil {
				hiIdx = int(hi.Sub(m.origin).Hours() / 24)
			}
			isMS := lo != nil && hi != nil && loIdx == hiIdx
			leftClip := lo != nil && hi != nil && !isMS && loIdx < 0 && hiIdx >= 1
			rightClip := lo != nil && hi != nil && !isMS && hiIdx > vis && loIdx <= vis-1
			isMSDay := func(d time.Time) bool {
				return isMS && d.Equal(*hi)
			}
			cellToday := func(d time.Time, cell string, sty lipgloss.Style) string {
				if !sameDay(d, m.today) || len([]rune(cell)) != 4 {
					return sty.Render(cell)
				}
				r := []rune(cell)
				return sty.Render(string(r[:1])) + stToday.Render("│") + sty.Render(string(r[2:]))
			}
			for i := 0; i < vis; i++ {
				d := m.origin.AddDate(0, 0, i)
				solid4 := strings.Repeat(solidCh, 4)
				span4 := strings.Repeat(spanCh, 4)
				switch {
				case isMSDay(d):
					grid += selWrap(sel, cellToday(d, solid4, barStyle(it, m.today, sel)))
				case lo != nil && hi != nil && !d.Before(*lo) && !d.After(*hi):
					sty := barStyle(it, m.today, sel)
					switch {
					case i == 0 && leftClip:
						grid += selWrap(sel, cellToday(d, "◀"+strings.Repeat(spanCh, 3), sty))
					case i == vis-1 && rightClip:
						grid += selWrap(sel, cellToday(d, strings.Repeat(spanCh, 3)+"▶", sty))
					default:
						grid += selWrap(sel, cellToday(d, span4, sty))
					}
				case sameDay(d, m.today):
					grid += selWrap(sel, stToday.Render(" │  "))
				default:
					grid += selWrap(sel, "    ")
				}
			}
			if !m.editing && hi != nil && !hi.After(m.origin) {
				if hi.Before(m.today) {
					late := int(m.today.Sub(*hi).Hours() / 24)
					grid += selWrap(sel, stOverdue.Render(fmt.Sprintf(" ◀ %dd late", late)))
				} else {
					grid += selWrap(sel, stDim.Render(" ◀"))
				}
			}
			if !m.editing && lo != nil && loIdx > vis-1 {
				ahead := int(lo.Sub(m.today).Hours() / 24)
				grid += selWrap(sel, stSoon.Render(fmt.Sprintf(" ▶ +%dd", ahead)))
			}
			rowLab := selWrap(sel, ind) + selWrap(sel, mark) + selWrap(sel, idTxt) + selWrap(sel, titTxt)
			gap := labelW + 1 - lipgloss.Width(ind+mark+idTxt+titTxt)
			if gap < 0 {
				gap = 0
			}
			L = append(L, rowLab+selWrap(sel, strings.Repeat(" ", gap))+grid)
		}
	}
	L = append(L, m.markerLine())
	return L
}

func barStyle(it tlItem, today time.Time, sel bool) lipgloss.Style {
	var base lipgloss.Style
	switch {
	case it.Due != nil && it.Due.Before(today):
		base = stOverdue
	case (it.Start != nil && !it.Start.After(today)) || it.Status == "doing":
		base = stActive
	default:
		base = stSoon
	}
	if sel {
		base = base.Bold(true)
	}
	return base
}

func tlDateStr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02")
}

func boardRun(args ...string) ([]byte, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, fmt.Errorf("no home")
	}
	cmdPath := filepath.Join(home, "bin", "board")
	out, err := exec.Command(cmdPath, args...).CombinedOutput()
	return out, err
}

func newTLModel() *tlModel {
	m := &tlModel{width: 120, height: 40, selID: tlLoadSel()}
	return m
}

var tlSesRe = regexp.MustCompile(`ses_[A-Za-z0-9]{16,}`)

func (m *tlModel) setItems(items []boardItem) {
	today := dateOnly(time.Now())
	m.today = today
	if m.origin.IsZero() {
		m.origin = today.AddDate(0, 0, -7)
	}
	out := make([]tlItem, 0, len(items))
	for _, b := range items {
		if b.Status == "done" {
			continue
		}
		it := tlItem{ID: b.ID, Title: b.Title, Desc: b.Body, Stream: b.Workstream,
			Folder: strings.Join(b.Folders, "/"), Status: b.Status, Prio: b.Prio, Owner: b.Owner,
			Ses: sesListFrom(tlSesRe.FindString(b.Body))}
		if s := b.Start; s != "" && s != "-" {
			if t, err := time.ParseInLocation("2006-01-02", s, today.Location()); err == nil {
				it.Start = &t
			}
		}
		if s := b.Due; s != "" && s != "-" {
			if t, err := time.ParseInLocation("2006-01-02", s, today.Location()); err == nil {
				it.Due = &t
			}
		}
		out = append(out, it)
	}
	m.items = out
	pref := []string{"life", "projecttwo", "projectone", "hq", "devtools"}
	seen := map[string]bool{}
	streams := []string{}
	for _, s := range pref {
		for _, it := range out {
			if it.Stream == s && !seen[s] {
				seen[s] = true
				streams = append(streams, s)
			}
		}
	}
	for _, it := range out {
		if !seen[it.Stream] {
			seen[it.Stream] = true
			streams = append(streams, it.Stream)
		}
	}
	m.streams = streams
	if m.selID != "" {
		for _, it := range out {
			if it.ID == m.selID {
				return
			}
		}
	}
	m.selID = ""
	for _, it := range out {
		if it.Due != nil && it.Due.Before(today) {
			m.selID = it.ID
			return
		}
	}
	for _, it := range out {
		if it.Due != nil {
			m.selID = it.ID
			return
		}
	}
	if len(out) > 0 {
		m.selID = out[0].ID
	}
}

func sesListFrom(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func (m *tlModel) keyTlk(k string, r []rune) tea.Cmd {
	if m.jumpOpen {
		ls := m.selLinks()
		switch k {
		case "ctrl+c":
			m.jumpOpen = false
		case "esc":
			m.jumpOpen = false
		case "u":
			if m.jumpIdx > 0 {
				m.jumpIdx--
			}
		case "e":
			if m.jumpIdx < len(ls)-1 {
				m.jumpIdx++
			}
		case "enter":
			if m.jumpIdx < len(ls) {
				w := ls[m.jumpIdx].win
				m.jumpOpen = false
				return jumpCmd(w)
			}
		}
		return nil
	}
	if m.editing {
		if m.typing {
			switch k {
			case "enter":
				if m.focus == fStart || m.focus == fDue {
					v, errMsg := parseDateRef(m.input, m.today)
					if errMsg != "" {
						m.msg = errMsg
						return nil
					}
					if m.focus == fStart {
						m.draft.Start = v
					} else {
						m.draft.Due = v
					}
				} else if m.focus == fTitle {
					if strings.TrimSpace(m.input) != "" {
						m.draft.Title = strings.TrimSpace(m.input)
					}
				} else if m.focus == fNote {
					m.draft.Desc = m.input
				}
				m.typing, m.input, m.msg = false, "", ""
			case "esc":
				m.typing, m.input, m.msg = false, "", ""
			case "backspace":
				if r := []rune(m.input); len(r) > 0 {
					m.input = string(r[:len(r)-1])
				}
			default:
				if len(r) > 0 && k == string(r) {
					m.input += string(r)
					m.msg = ""
				}
			}
			return nil
		}
		if m.calOpen {
			switch k {
			case "enter":
				v := m.calSel
				if m.focus == fStart {
					m.draft.Start = &v
				} else {
					m.draft.Due = &v
				}
				m.calOpen = false
			case "esc":
				m.calOpen = false
			case "u", "up":
				m.calSel = m.calSel.AddDate(0, 0, -7)
				m.syncCal()
			case "e", "down":
				m.calSel = m.calSel.AddDate(0, 0, 7)
				m.syncCal()
			case "n", "left":
				m.calSel = m.calSel.AddDate(0, 0, -1)
				m.syncCal()
			case "i", "right":
				m.calSel = m.calSel.AddDate(0, 0, 1)
				m.syncCal()
			case ",":
				m.calSel = m.calSel.AddDate(0, -1, 0)
				m.syncCal()
			case ".":
				m.calSel = m.calSel.AddDate(0, 1, 0)
				m.syncCal()
			case "t":
				m.calSel = m.today
				m.syncCal()
			}
			return nil
		}
		if m.pickOpen {
			vals := pickVals(m.focus)
			switch k {
			case "enter":
				if m.pickIdx >= 0 && m.pickIdx < len(vals) {
					m.applyPickVal(vals[m.pickIdx])
				}
				m.pickOpen = false
			case "esc":
				m.pickOpen = false
			case "u", "up", "n", "left":
				m.pickIdx = (m.pickIdx - 1 + len(vals)) % len(vals)
			case "e", "down", "i", "right":
				m.pickIdx = (m.pickIdx + 1) % len(vals)
			}
			return nil
		}
		switch k {
		case "esc":
			m.editing, m.msg = false, ""
		case "c":
			m.commit()
		case "u", "up":
			m.moveFocus(-1, 0)
		case "e", "down":
			m.moveFocus(1, 0)
		case "n", "left":
			m.moveFocus(0, -1)
		case "i", "right":
			m.moveFocus(0, 1)
		case "shift+left":
			m.draft.slide(-1)
		case "shift+right":
			m.draft.slide(1)
		case "enter":
			switch m.focus {
			case fStart, fDue:
				m.calSel = m.today
				if m.focus == fStart && m.draft.Start != nil {
					m.calSel = *m.draft.Start
				}
				if m.focus == fDue && m.draft.Due != nil {
					m.calSel = *m.draft.Due
				}
				m.calMonth = time.Date(m.calSel.Year(), m.calSel.Month(), 1, 0, 0, 0, 0, m.calSel.Location())
				m.calOpen = true
			case fTitle, fNote:
				txt := m.draft.Title
				if m.focus == fNote {
					txt = m.draft.Desc
				}
				f, err := os.CreateTemp("", "asb-edit-*.md")
				if err != nil {
					m.msg = err.Error()
				} else {
					path := f.Name()
					f.WriteString(txt)
					f.Close()
					cmd := exec.Command("nvim", path)
					field := m.focus
					return tea.ExecProcess(cmd, func(error) tea.Msg {
						return tlEditDoneMsg{path: path, field: field}
					})
				}
			default:
				if vals := pickVals(m.focus); vals != nil {
					cur := m.curPickVal()
					m.pickIdx = 0
					for k, v := range vals {
						if v == cur {
							m.pickIdx = k
						}
					}
					m.pickOpen = true
				}
			}
		default:
			if len(r) > 0 && k == string(r) {
				m.typing, m.input, m.msg = true, string(r), ""
			}
		}
		return nil
	}
	m.msg = ""
	switch k {
	case "esc":
		if m.dateMode {
			m.dateMode = false
		} else {
			m.back = true
		}
	case "c":
		if m.dateMode {
			m.commitDates()
		}
	case "N":
		if m.dateMode {
			m.shiftDates(-1, -1)
		}
	case "I":
		if m.dateMode {
			m.shiftDates(1, 1)
		}
	case "[":
		if m.dateMode {
			m.shiftDates(-1, 0)
		}
	case "{":
		if m.dateMode && m.dStart.Before(*m.dDue) {
			m.shiftDates(1, 0)
		}
	case "]":
		if m.dateMode {
			m.shiftDates(0, 1)
		}
	case "}":
		if m.dateMode && m.dStart.Before(*m.dDue) {
			m.shiftDates(0, -1)
		}
	case "q", "ctrl+c":
		m.back = true
	case "up", "u":
		m.move(-1)
	case "down", "e":
		m.move(1)
	case "ctrl+u":
		m.vscroll -= 10
		if m.vscroll < 0 {
			m.vscroll = 0
		}
	case "ctrl+e":
		m.vscroll += 10
	case "n", "left":
		m.origin = m.origin.AddDate(0, 0, -1)
	case "i", "right":
		m.origin = m.origin.AddDate(0, 0, 1)
	case ",":
		m.move(-5)
	case ".":
		m.move(5)
	case "0", "t":
		m.origin = m.today.AddDate(0, 0, -7)
	case "a":
		m.showAll = !m.showAll
		m.followSel(0)
	case "enter":
		if it := m.selItem(); it != nil {
			m.draft, m.orig = *it, *it
			m.editing, m.typing, m.focus, m.input, m.msg = true, false, 0, "", ""
		}
	case "d":
		m.enterDateMode()
	case "L":
		live := []string{}
		for _, id := range m.order() {
			for _, it := range m.items {
				if it.ID == id {
					if len(m.itemLinks(it)) > 0 {
						live = append(live, id)
					}
				}
			}
		}
		if len(live) > 0 {
			i := 0
			for j, id := range live {
				if id == m.selID {
					i = (j + 1) % len(live)
				}
			}
			m.selID = live[i]
			m.followSel(1)
		}
	case "m":
		if ls := m.selLinks(); len(ls) == 1 {
			return jumpCmd(ls[0].win)
		} else if len(ls) > 1 {
			m.jumpOpen, m.jumpIdx = true, 0
		}
	}
	return nil
}

func tlSelStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".cache", "agent", "board-panel.json")
}

func tlSaveSel(id string) {
	p := tlSelStatePath()
	if p == "" || id == "" {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(`{"sel":`+strconv.Quote(id)+`}`), 0o644)
}

func tlLoadSel() string {
	p := tlSelStatePath()
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var d struct {
		Sel string `json:"sel"`
	}
	if json.Unmarshal(b, &d) != nil {
		return ""
	}
	return d.Sel
}

// applyWindowLink moves the cursor to the ticket linked to the invoking
// window's opencode session, if any. The active pane's session wins;
// otherwise sibling panes of the same window are considered in order.
func (m *tlModel) applyWindowLink(windowID string) {
	if windowID == "" {
		pane := os.Getenv("TMUX_PANE")
		if pane == "" {
			return
		}
		var err error
		windowID, err = paneWindow(pane)
		if err != nil {
			return
		}
	}
	out, err := exec.Command("tmux", "list-panes", "-t", windowID, "-F", "#{pane_active} #{pane_id}").Output()
	if err != nil {
		return
	}
	ordered := []string{}
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(ln)
		if len(f) != 2 {
			continue
		}
		if f[0] == "1" {
			ordered = append([]string{f[1]}, ordered...)
		} else {
			ordered = append(ordered, f[1])
		}
	}
	if len(ordered) == 0 {
		return
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		return
	}
	ms, err := filepath.Glob(filepath.Join(home, ".local", "state", "op", "ses_ses_*"))
	if err != nil {
		return
	}
	sesOfPane := map[string]string{}
	for _, f := range ms {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var d struct {
			SessionID string `json:"sessionID"`
			Pane      struct {
				PaneId string `json:"paneId"`
			} `json:"pane"`
		}
		if json.Unmarshal(b, &d) != nil || d.Pane.PaneId == "" {
			continue
		}
		sesOfPane[d.Pane.PaneId] = d.SessionID
	}
	for _, p := range ordered {
		sid, ok := sesOfPane[p]
		if !ok {
			continue
		}
		for _, it := range m.items {
			for _, s := range it.Ses {
				if s == sid {
					m.selID = it.ID
					m.vscroll = 0
					m.followSel(0)
					return
				}
			}
		}
	}
}

func paneWindow(pane string) (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane, "#{window_id}").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
