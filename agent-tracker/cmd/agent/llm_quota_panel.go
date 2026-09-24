package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultZAIQuotaURL        = "https://api.z.ai/api/monitor/usage/quota/limit"
	defaultCLIProxyManagement = "https://cp.asurada.dev/v0/management"
	zaiKeychainService        = "zai-api"
	zaiKeychainAccount        = "api.z.ai"
	cliproxyKeychainService   = "cliproxy-management"
	cliproxyKeychainAccount   = "azwestus.asurada.dev"
	codexUsageURL             = "https://chatgpt.com/backend-api/wham/usage"
	codexUsageUserAgent       = "codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal"
	claudeUsageURL            = "https://api.anthropic.com/api/oauth/usage"
	claudeUsageUserAgent      = "claude-cli/2.1.280 (external, cli)"
	quotaRequestTimeout       = 18 * time.Second
	codexQuotaRequestTimeout  = 65 * time.Second
	llmQuotaCacheTTL          = 5 * time.Minute

	llmQuotaSpreadPriority = 50
)

type llmQuotaWindow struct {
	Label       string    `json:"label"`
	UsedPercent float64   `json:"used_percent"`
	ResetAt     time.Time `json:"reset_at"`
}

type llmQuotaProvider struct {
	Windows []llmQuotaWindow `json:"windows"`
	Error   string           `json:"error,omitempty"`
}

type quotaAccount struct {
	Label    string           `json:"label"`
	Plan     string           `json:"plan,omitempty"`
	Name     string           `json:"name,omitempty"`
	Disabled bool             `json:"disabled,omitempty"`
	Priority *int             `json:"priority,omitempty"`
	Windows  []llmQuotaWindow `json:"windows"`
	Error    string           `json:"error,omitempty"`
}

type llmQuotaSnapshot struct {
	ZAI             llmQuotaProvider `json:"zai"`
	Codex           []quotaAccount   `json:"codex"`
	Claude          []quotaAccount   `json:"claude"`
	CodexError      string           `json:"codex_error,omitempty"`
	ClaudeError     string           `json:"claude_error,omitempty"`
	RoutingStrategy string           `json:"routing_strategy,omitempty"`
	FetchedAt       time.Time        `json:"fetched_at"`
}

type llmQuotaResultMsg struct {
	snapshot llmQuotaSnapshot
	err      error
}

type llmQuotaRouting struct {
	Claude   []quotaAccount
	Codex    []quotaAccount
	Strategy string
}

type llmQuotaRoutingMsg struct {
	routing llmQuotaRouting
	err     error
}

type llmQuotaMutationMsg struct {
	label string
	err   error
}

type llmQuotaRowKind int

const (
	llmQuotaRowHeader llmQuotaRowKind = iota
	llmQuotaRowAccount
	llmQuotaRowStatic
)

type llmQuotaRow struct {
	kind     llmQuotaRowKind
	provider string
	account  *quotaAccount
}

type llmQuotaPanelModel struct {
	width            int
	height           int
	snapshot         llmQuotaSnapshot
	loaded           bool
	message          string
	refreshInFlight  bool
	pendingRefresh   bool
	pendingForce     bool
	showAltHints     bool
	requestBack      bool
	rows             []llmQuotaRow
	cursor           int
	mutationInFlight bool
}

type cliproxyAuthFile struct {
	Provider  string `json:"provider"`
	Type      string `json:"type"`
	Label     string `json:"label"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AuthIndex string `json:"auth_index"`
	Disabled  bool   `json:"disabled"`
	Priority  *int   `json:"priority"`
	IDToken   struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
	} `json:"id_token"`
}

type codexUsageWindow struct {
	UsedPercent        json.Number `json:"used_percent"`
	LimitWindowSeconds json.Number `json:"limit_window_seconds"`
	ResetAfterSeconds  json.Number `json:"reset_after_seconds"`
	ResetAt            json.Number `json:"reset_at"`
}

type codexRateLimit struct {
	Allowed         *bool             `json:"allowed"`
	LimitReached    bool              `json:"limit_reached"`
	PrimaryWindow   *codexUsageWindow `json:"primary_window"`
	SecondaryWindow *codexUsageWindow `json:"secondary_window"`
}

type codexUsagePayload struct {
	PlanType             string          `json:"plan_type"`
	RateLimit            *codexRateLimit `json:"rate_limit"`
	CodeReviewRateLimit  *codexRateLimit `json:"code_review_rate_limit"`
	AdditionalRateLimits []struct {
		LimitName      string          `json:"limit_name"`
		MeteredFeature string          `json:"metered_feature"`
		RateLimit      *codexRateLimit `json:"rate_limit"`
	} `json:"additional_rate_limits"`
}

type claudeUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type claudeUsagePayload struct {
	FiveHour *claudeUsageWindow `json:"five_hour"`
	SevenDay *claudeUsageWindow `json:"seven_day"`
}

func newLLMQuotaPanelModel() *llmQuotaPanelModel {
	return &llmQuotaPanelModel{message: "Loading LLM quotas..."}
}

func (m *llmQuotaPanelModel) activate() tea.Cmd {
	m.requestBack = false
	if m.loaded && time.Since(m.snapshot.FetchedAt) < llmQuotaCacheTTL {
		return nil
	}
	if !m.loaded {
		m.message = "Loading LLM quotas..."
	}
	return m.requestRefreshCmd(false)
}

func (m *llmQuotaPanelModel) Init() tea.Cmd {
	return m.activate()
}

func (m *llmQuotaPanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case llmQuotaResultMsg:
		m.refreshInFlight = false
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.snapshot = msg.snapshot
			m.loaded = true
			m.message = ""
			m.rebuildRows()
		}
		if m.pendingRefresh {
			force := m.pendingForce
			m.pendingRefresh = false
			m.pendingForce = false
			return m, m.requestRefreshCmd(force)
		}
	case llmQuotaRoutingMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.applyRouting(msg.routing)
			m.message = ""
		}
	case llmQuotaMutationMsg:
		m.mutationInFlight = false
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.message = ""
			return m, m.requestRoutingRefreshCmd()
		}
	case tea.KeyMsg:
		if isAltFooterToggleKey(msg) {
			m.showAltHints = !m.showAltHints
			return m, nil
		}
		m.showAltHints = false
		switch msg.String() {
		case "esc", "ctrl+c":
			m.requestBack = true
		case "r":
			return m, m.requestRefreshCmd(true)
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
		case "J":
			return m, m.moveAccountCmd(1)
		case "K":
			return m, m.moveAccountCmd(-1)
		case "d":
			return m, m.toggleAccountCmd()
		case "s":
			return m, m.cycleStrategyCmd()
		}
	}
	return m, nil
}

func (m *llmQuotaPanelModel) View() string {
	return m.render(newPaletteStyles(), m.width, m.height)
}

func (m *llmQuotaPanelModel) render(styles paletteStyles, width, height int) string {
	if width <= 0 {
		width = 96
	}
	if height <= 0 {
		height = 28
	}
	contentWidth := maxInt(20, width-2)
	title := styles.title.Render("LLM Quotas")
	meta := "routing " + firstNonEmpty(m.snapshot.RoutingStrategy, "—")
	if !m.snapshot.FetchedAt.IsZero() {
		meta += " · updated " + m.snapshot.FetchedAt.Local().Format("15:04")
	}
	pad := contentWidth - lipgloss.Width(title) - lipgloss.Width(meta)
	if pad > 0 {
		title = title + strings.Repeat(" ", pad) + styles.muted.Render(meta)
	}
	body := ""
	if !m.loaded {
		body = styles.muted.Render("Loading provider quotas...")
	} else {
		lines := make([]string, 0, len(m.rows)*2)
		for i := range m.rows {
			lines = append(lines, m.renderRow(styles, i, contentWidth)...)
		}
		body = strings.Join(lines, "\n")
	}
	footer := m.renderFooter(styles, contentWidth)
	bodyHeight := height - lipgloss.Height(title) - lipgloss.Height(footer) - 2
	if bodyHeight > lipgloss.Height(body) {
		body = lipgloss.NewStyle().Height(bodyHeight).Render(body)
	}
	view := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *llmQuotaPanelModel) renderRow(styles paletteStyles, index, width int) []string {
	row := m.rows[index]
	selected := index == m.cursor
	switch row.kind {
	case llmQuotaRowHeader:
		return []string{m.renderProviderHeader(styles, row.provider, selected, width)}
	case llmQuotaRowAccount:
		return m.renderAccount(styles, row, selected, width)
	default:
		switch row.provider {
		case "claude":
			if m.snapshot.ClaudeError != "" {
				return []string{styles.statusBad.Render(truncate(m.snapshot.ClaudeError, width))}
			}
		case "codex":
			if m.snapshot.CodexError != "" {
				return []string{styles.statusBad.Render(truncate(m.snapshot.CodexError, width))}
			}
		case "zai":
			if m.snapshot.ZAI.Error != "" {
				return []string{styles.statusBad.Render(truncate(m.snapshot.ZAI.Error, width))}
			}
			account := quotaAccount{Label: "Coding", Windows: m.snapshot.ZAI.Windows}
			if len(account.Windows) == 0 {
				return []string{styles.muted.Render("No quota windows returned")}
			}
			return m.renderAccountWindows(styles, &account, width, 0)
		}
		return []string{""}
	}
}

func (m *llmQuotaPanelModel) providerAccounts(provider string) []quotaAccount {
	if provider == "claude" {
		return m.snapshot.Claude
	}
	if provider == "codex" {
		return m.snapshot.Codex
	}
	return nil
}

func (m *llmQuotaPanelModel) providerTitle(provider string) string {
	if provider == "claude" {
		return "CLAUDE"
	}
	if provider == "codex" {
		return "CODEX · CHATGPT"
	}
	return "Z.AI"
}

func (m *llmQuotaPanelModel) providerMode(provider string) string {
	accounts := m.providerAccounts(provider)
	distinct := map[int]bool{}
	enabled := 0
	for i := range accounts {
		if accounts[i].Disabled {
			continue
		}
		enabled++
		if accounts[i].Priority != nil {
			distinct[*accounts[i].Priority] = true
		}
	}
	if enabled > 1 && len(distinct) <= 1 {
		return "spread"
	}
	return "drain"
}

func (m *llmQuotaPanelModel) renderProviderHeader(styles paletteStyles, provider string, selected bool, width int) string {
	name := m.providerTitle(provider)
	if provider == "zai" {
		return styles.panelTitle.Render("  " + name) + "  " + styles.muted.Render("API key · no routing")
	}
	mode := m.providerMode(provider)
	accounts := m.providerAccounts(provider)
	badge := styles.keyword.Render("drain ⇅")
	if mode == "spread" {
		badge = styles.todoCheckDone.Render("spread ⇄")
	}
	left := name
	if selected {
		left = "❯ " + name
	} else {
		left = "  " + name
	}
	label := styles.panelTitle.Render(left) + "  " + badge
	enabledCount := 0
	for i := range accounts {
		if !accounts[i].Disabled {
			enabledCount++
		}
	}
	right := styles.muted.Render(fmt.Sprintf("%d/%d on · s switch", enabledCount, len(accounts)))
	pad := width - lipgloss.Width(label) - lipgloss.Width(right) - 2
	if pad > 0 {
		label += strings.Repeat(" ", pad) + right
	}
	if selected {
		return styles.selectedItem.Render(label)
	}
	return label
}

const (
	quotaColPos     = 4
	quotaColLabel   = 24
	quotaColState   = 4
	quotaColPlan    = 6
	quotaIndent     = quotaColPos + quotaColLabel + quotaColState + quotaColPlan + 2
)

func (m *llmQuotaPanelModel) renderAccount(styles paletteStyles, row llmQuotaRow, selected bool, width int) []string {
	account := row.account
	pos := m.accountPosition(row.provider, account)
	posText := fmt.Sprintf("#%d", pos)
	if m.providerMode(row.provider) == "spread" {
		posText = "·"
	}
	stateText := "on"
	stateStyle := styles.panelTextDone
	if account.Disabled {
		stateText = "off"
		stateStyle = styles.statusBad
	}
	labelStyle := styles.itemTitle
	if account.Disabled {
		labelStyle = styles.muted
	}
	line := styles.muted.Render(fmt.Sprintf("%-*s", quotaColPos, posText)) +
		labelStyle.Render(truncate(firstNonEmpty(account.Label, account.Name, "account"), quotaColLabel)) +
		" " + stateStyle.Render(fmt.Sprintf("%-*s", quotaColState, stateText))
	if account.Plan != "" {
		line += " " + styles.keyword.Render(fmt.Sprintf("%-*s", quotaColPlan, truncate(strings.ToUpper(account.Plan), quotaColPlan)))
	} else {
		line += strings.Repeat(" ", quotaColPlan+1)
	}
	windowWidth := maxInt(0, width-quotaIndent)
	var lines []string
	if len(account.Windows) == 0 && account.Error == "" {
		line += " " + styles.muted.Render("no quota data")
		lines = []string{line}
	} else {
		chipLines := m.renderAccountWindows(styles, account, windowWidth, quotaIndent)
		line += " " + chipLines[0]
		lines = append([]string{line}, chipLines[1:]...)
	}
	if account.Error != "" && !account.Disabled {
		lines = append(lines, strings.Repeat(" ", quotaIndent)+styles.statusBad.Render(truncate(account.Error, maxInt(10, width-quotaIndent))))
	}
	if selected {
		for i := range lines {
			lines[i] = styles.selectedItem.Render(lines[i])
		}
	}
	return lines
}

func (m *llmQuotaPanelModel) renderAccountWindows(styles paletteStyles, account *quotaAccount, width, indent int) []string {
	if len(account.Windows) == 0 {
		return []string{styles.muted.Render("no quota data")}
	}
	dim := account.Disabled
	type chip struct{ text, label, bar, pct, reset string; free float64 }
	chips := make([]chip, 0, len(account.Windows))
	for _, window := range account.Windows {
		free := 100 - clampFloat(window.UsedPercent, 0, 100)
		label := shortWindowLabel(window.Label)
		reset := formatQuotaResetShort(window.ResetAt)
		pct := fmt.Sprintf("%3.0f%%", free)
		chips = append(chips, chip{label: label, pct: pct, reset: reset, free: free, bar: ""})
	}
	unit := len(chips[0].label) + 1 + 4 + 1 + len(chips[0].pct) + 1 + len(chips[0].reset)
	perChip := width
	if len(chips) > 1 {
		perChip = (width - 2*(len(chips)-1)) / len(chips)
	}
	barWidth := maxInt(6, perChip-unit)
	var rendered []string
	var current strings.Builder
	currentLen := 0
	for i := range chips {
		c := &chips[i]
		bar := renderQuotaBar(c.free, barWidth)
		pctStyle := styles.panelText
		if dim {
			pctStyle = styles.muted
		} else if c.free <= 10 {
			pctStyle = styles.statusBad
		} else if c.free >= 60 {
			pctStyle = styles.todoCheckDone
		}
		labelStyle := styles.muted
		barStyle := styles.panelText
		if dim {
			barStyle = styles.muted
		}
		text := labelStyle.Render(fmt.Sprintf("%-*s", len(chips[0].label), c.label)) + " " +
			barStyle.Render(bar) + " " + pctStyle.Render(c.pct)
		if c.reset != "" {
			text += " " + labelStyle.Render(c.reset)
		}
		if currentLen > 0 && currentLen+2+lipgloss.Width(text) > width {
			rendered = append(rendered, current.String())
			current.Reset()
			currentLen = 0
		}
		if currentLen > 0 {
			current.WriteString("  ")
			currentLen += 2
		}
		current.WriteString(text)
		currentLen += lipgloss.Width(text)
	}
	if current.Len() > 0 {
		rendered = append(rendered, current.String())
	}
	for i := 1; i < len(rendered); i++ {
		rendered[i] = strings.Repeat(" ", indent) + rendered[i]
	}
	return rendered
}

func (m *llmQuotaPanelModel) accountPosition(provider string, account *quotaAccount) int {
	accounts := m.providerAccounts(provider)
	for i := range accounts {
		if accounts[i].Name == account.Name {
			return i + 1
		}
	}
	return 0
}

func shortWindowLabel(label string) string {
	short := strings.TrimSpace(label)
	if strings.HasPrefix(short, "Code ") {
		short = strings.TrimPrefix(short, "Code ")
	}
	short = strings.ReplaceAll(short, "Review ", "rev ")
	replacer := strings.NewReplacer("5-hour", "5h", "weekly", "wk", "monthly", "mo")
	short = replacer.Replace(short)
	short = strings.TrimSpace(short)
	if short == "" {
		short = "?"
	}
	return short
}

func formatQuotaResetShort(resetAt time.Time) string {
	if resetAt.IsZero() {
		return ""
	}
	delta := time.Until(resetAt)
	if delta <= 0 {
		return "now"
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm", int(delta/time.Minute))
	}
	if delta < 10*time.Hour {
		return fmt.Sprintf("%dh%02dm", int(delta/time.Hour), int((delta%time.Hour)/time.Minute))
	}
	if delta < 48*time.Hour {
		return fmt.Sprintf("%dh", int(delta/time.Hour))
	}
	if delta < 8*24*time.Hour {
		return fmt.Sprintf("%dd%dh", int(delta/(24*time.Hour)), int((delta%(24*time.Hour))/time.Hour))
	}
	return resetAt.Local().Format("Jan 2")
}

func renderQuotaBar(remaining float64, width int) string {
	width = maxInt(1, width)
	filled := int((clampFloat(remaining, 0, 100)/100)*float64(width) + 0.5)
	return "[" + strings.Repeat("=", filled) + strings.Repeat(".", width-filled) + "]"
}

func (m *llmQuotaPanelModel) renderFooter(styles paletteStyles, width int) string {
	pairs := [][2]string{
		{"↑/↓", "select"},
		{"J/K", "order"},
		{"d", "on/off"},
		{"s", "strategy"},
		{"r", "refresh"},
		{"Esc", "back"},
		{footerHintToggleKey, "more"},
	}
	if m.showAltHints {
		pairs = [][2]string{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}}
	}
	footer := renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "   ", pairs)
	left := ""
	if m.mutationInFlight {
		left = styles.footer.Render("Applying routing change...")
	} else if m.refreshInFlight {
		left = styles.footer.Render("Refreshing...")
	} else if strings.TrimSpace(m.message) != "" {
		left = styles.statusBad.Render(truncate(m.message, maxInt(20, width-lipgloss.Width(footer)-3)))
	} else if !m.snapshot.FetchedAt.IsZero() {
		left = styles.muted.Render("Updated " + m.snapshot.FetchedAt.Local().Format("15:04:05"))
	}
	if left != "" && lipgloss.Width(left)+3+lipgloss.Width(footer) <= width {
		footer = left + "   " + footer
	}
	return lipgloss.NewStyle().Width(width).Render(footer)
}

func (m *llmQuotaPanelModel) currentStatus() string {
	if m.mutationInFlight {
		return "Applying routing change..."
	}
	if m.refreshInFlight {
		return "Refreshing LLM quotas..."
	}
	if m.loaded {
		return "LLM quotas updated"
	}
	return strings.TrimSpace(m.message)
}

func (m *llmQuotaPanelModel) rebuildRows() {
	rows := make([]llmQuotaRow, 0, 12)
	if len(m.snapshot.Claude) > 0 || m.snapshot.ClaudeError != "" {
		rows = append(rows, llmQuotaRow{kind: llmQuotaRowHeader, provider: "claude"})
		for i := range m.snapshot.Claude {
			rows = append(rows, llmQuotaRow{kind: llmQuotaRowAccount, provider: "claude", account: &m.snapshot.Claude[i]})
		}
		rows = append(rows, llmQuotaRow{kind: llmQuotaRowStatic, provider: "claude"})
	}
	if len(m.snapshot.Codex) > 0 || m.snapshot.CodexError != "" {
		rows = append(rows, llmQuotaRow{kind: llmQuotaRowHeader, provider: "codex"})
		for i := range m.snapshot.Codex {
			rows = append(rows, llmQuotaRow{kind: llmQuotaRowAccount, provider: "codex", account: &m.snapshot.Codex[i]})
		}
		rows = append(rows, llmQuotaRow{kind: llmQuotaRowStatic, provider: "codex"})
	}
	rows = append(rows, llmQuotaRow{kind: llmQuotaRowHeader, provider: "zai"})
	rows = append(rows, llmQuotaRow{kind: llmQuotaRowStatic, provider: "zai"})
	m.rows = rows
	m.cursor = clampInt(m.cursor, 0, maxInt(0, len(rows)-1))
	m.ensureCursorInteractive()
}

func (m *llmQuotaPanelModel) interactive(i int) bool {
	if i < 0 || i >= len(m.rows) {
		return false
	}
	row := m.rows[i]
	return row.provider == "claude" || row.provider == "codex"
}

func (m *llmQuotaPanelModel) ensureCursorInteractive() {
	if len(m.rows) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = clampInt(m.cursor, 0, len(m.rows)-1)
	for i := m.cursor; i < len(m.rows); i++ {
		if m.interactive(i) {
			m.cursor = i
			return
		}
	}
	for i := m.cursor; i >= 0; i-- {
		if m.interactive(i) {
			m.cursor = i
			return
		}
	}
	m.cursor = 0
}

func (m *llmQuotaPanelModel) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	step := delta
	for next := m.cursor + step; next >= 0 && next < len(m.rows); next += step {
		if m.interactive(next) {
			m.cursor = next
			return
		}
	}
}

func (m *llmQuotaPanelModel) requestRefreshCmd(force bool) tea.Cmd {
	if m.refreshInFlight {
		m.pendingRefresh = true
		m.pendingForce = m.pendingForce || force
		return nil
	}
	m.refreshInFlight = true
	if m.loaded {
		m.message = ""
	}
	return func() tea.Msg {
		snapshot, err := fetchLLMQuotaSnapshot(force)
		return llmQuotaResultMsg{snapshot: snapshot, err: err}
	}
}

func (m *llmQuotaPanelModel) requestRoutingRefreshCmd() tea.Cmd {
	return func() tea.Msg {
		routing, err := fetchLLMQuotaRouting()
		return llmQuotaRoutingMsg{routing: routing, err: err}
	}
}

func (m *llmQuotaPanelModel) mutate(label string, fn func(baseURL string, client *http.Client, key string) error) tea.Cmd {
	if m.mutationInFlight {
		return nil
	}
	m.mutationInFlight = true
	m.message = ""
	return func() tea.Msg {
		baseURL, client, key, err := cliproxyMutator()
		if err == nil {
			err = fn(baseURL, client, key)
		}
		return llmQuotaMutationMsg{label: label, err: err}
	}
}

func cliproxyMutator() (string, *http.Client, string, error) {
	key, err := loadCLIProxyManagementKey()
	if err != nil {
		return "", nil, "", err
	}
	baseURL := strings.TrimRight(firstNonEmpty(os.Getenv("CLIPROXY_MANAGEMENT_URL"), defaultCLIProxyManagement), "/")
	return baseURL, &http.Client{Timeout: quotaRequestTimeout}, key, nil
}

func (m *llmQuotaPanelModel) selectedAccount() (string, *quotaAccount) {
	if !m.interactive(m.cursor) {
		return "", nil
	}
	row := m.rows[m.cursor]
	if row.kind != llmQuotaRowAccount {
		return "", nil
	}
	return row.provider, row.account
}

func (m *llmQuotaPanelModel) moveAccountCmd(delta int) tea.Cmd {
	provider, account := m.selectedAccount()
	if account == nil {
		return nil
	}
	if m.providerMode(provider) != "drain" {
		m.message = "spread: order has no effect — press s to drain first"
		return nil
	}
	accounts := m.providerAccounts(provider)
	idx := -1
	for i := range accounts {
		if accounts[i].Name == account.Name {
			idx = i
			break
		}
	}
	target := idx + delta
	if idx < 0 || target < 0 || target >= len(accounts) {
		return nil
	}
	accounts[idx], accounts[target] = accounts[target], accounts[idx]
	m.applyDrainLadderLocal(provider)
	m.rebuildRows()
	for i := range m.rows {
		if m.rows[i].kind == llmQuotaRowAccount && m.rows[i].provider == provider && m.rows[i].account.Name == account.Name {
			m.cursor = i
			break
		}
	}
	providerCopy := append([]quotaAccount(nil), accounts...)
	strategy := m.snapshot.RoutingStrategy
	return m.applyPrioritiesCmd(provider, providerCopy, strategy)
}

func (m *llmQuotaPanelModel) applyPrioritiesCmd(provider string, accounts []quotaAccount, strategy string) tea.Cmd {
	return m.mutate(provider, func(baseURL string, client *http.Client, key string) error {
		if strategy != "round-robin" {
			if err := patchRoutingStrategy(client, key, baseURL, "round-robin"); err != nil {
				return err
			}
		}
		return patchAccountPriorities(client, key, baseURL, accounts)
	})
}

func (m *llmQuotaPanelModel) toggleAccountCmd() tea.Cmd {
	provider, account := m.selectedAccount()
	if account == nil {
		return nil
	}
	if strings.TrimSpace(account.Name) == "" {
		m.message = "missing auth filename — press r to refresh"
		return nil
	}
	newDisabled := !account.Disabled
	account.Disabled = newDisabled
	m.rebuildRows()
	name := account.Name
	return m.mutate(provider, func(baseURL string, client *http.Client, key string) error {
		return patchAuthFileStatus(client, key, baseURL, name, newDisabled)
	})
}

func (m *llmQuotaPanelModel) cycleStrategyCmd() tea.Cmd {
	if !m.interactive(m.cursor) {
		return nil
	}
	provider := m.rows[m.cursor].provider
	accounts := m.providerAccounts(provider)
	if len(accounts) == 0 {
		return nil
	}
	for i := range accounts {
		if strings.TrimSpace(accounts[i].Name) == "" {
			m.message = "missing auth filename — press r to refresh"
			return nil
		}
	}
	mode := m.providerMode(provider)
	next := "spread"
	if mode == "spread" {
		next = "drain"
	}
	if next == "spread" {
		even := llmQuotaSpreadPriority
		for i := range accounts {
			accounts[i].Priority = &even
		}
	} else {
		for i := range accounts {
			value := 100 - 10*i
			accounts[i].Priority = &value
		}
	}
	m.rebuildRows()
	providerCopy := append([]quotaAccount(nil), accounts...)
	strategy := m.snapshot.RoutingStrategy
	label := provider + " → " + next
	return m.mutate(label, func(baseURL string, client *http.Client, key string) error {
		if strategy != "round-robin" {
			if err := patchRoutingStrategy(client, key, baseURL, "round-robin"); err != nil {
				return err
			}
		}
		return patchAccountPriorities(client, key, baseURL, providerCopy)
	})
}

func (m *llmQuotaPanelModel) applyDrainLadderLocal(provider string) {
	accounts := m.providerAccounts(provider)
	for i := range accounts {
		value := 100 - 10*i
		accounts[i].Priority = &value
	}
}

func (m *llmQuotaPanelModel) applyRouting(routing llmQuotaRouting) {
	merge := func(old []quotaAccount, fresh []quotaAccount) []quotaAccount {
		for i := range fresh {
			for j := range old {
				if old[j].Name != "" && old[j].Name == fresh[i].Name {
					fresh[i].Windows = old[j].Windows
					fresh[i].Plan = old[j].Plan
					fresh[i].Error = old[j].Error
					break
				}
			}
		}
		sortQuotaAccounts(fresh)
		return fresh
	}
	if routing.Claude != nil {
		m.snapshot.Claude = merge(m.snapshot.Claude, routing.Claude)
	}
	if routing.Codex != nil {
		m.snapshot.Codex = merge(m.snapshot.Codex, routing.Codex)
	}
	if routing.Strategy != "" {
		m.snapshot.RoutingStrategy = routing.Strategy
	}
	m.rebuildRows()
}

func patchAccountPriorities(client *http.Client, key, baseURL string, accounts []quotaAccount) error {
	for i := range accounts {
		if strings.TrimSpace(accounts[i].Name) == "" || accounts[i].Priority == nil {
			continue
		}
		body := map[string]any{"name": accounts[i].Name, "priority": *accounts[i].Priority}
		var out struct {
			Status string `json:"status"`
		}
		if err := cliproxyManagementJSON(client, key, http.MethodPatch, baseURL+"/auth-files/fields", body, &out); err != nil {
			return err
		}
	}
	return nil
}

func patchAuthFileStatus(client *http.Client, key, baseURL, name string, disabled bool) error {
	body := map[string]any{"name": name, "disabled": disabled}
	var out struct {
		Status string `json:"status"`
	}
	return cliproxyManagementJSON(client, key, http.MethodPatch, baseURL+"/auth-files/status", body, &out)
}

func patchRoutingStrategy(client *http.Client, key, baseURL, value string) error {
	body := map[string]any{"value": value}
	var out struct {
		Status string `json:"status"`
	}
	return cliproxyManagementJSON(client, key, http.MethodPatch, baseURL+"/routing/strategy", body, &out)
}

func sortQuotaAccounts(accounts []quotaAccount) {
	sort.SliceStable(accounts, func(i, j int) bool {
		pi, pj := accounts[i].Priority, accounts[j].Priority
		switch {
		case pi != nil && pj != nil && *pi != *pj:
			return *pi > *pj
		case pi != nil && pj == nil:
			return true
		case pi == nil && pj != nil:
			return false
		}
		return strings.ToLower(accounts[i].Label) < strings.ToLower(accounts[j].Label)
	})
}

func fetchLLMQuotaSnapshot(force bool) (llmQuotaSnapshot, error) {
	if !force {
		if snapshot, ok := loadLLMQuotaCache(); ok {
			return snapshot, nil
		}
	}
	client := &http.Client{Timeout: quotaRequestTimeout}
	var snapshot llmQuotaSnapshot
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		snapshot.ZAI = fetchZAIQuota(client)
	}()
	go func() {
		defer wg.Done()
		snapshot.Codex, snapshot.CodexError = fetchCodexQuotas()
	}()
	go func() {
		defer wg.Done()
		snapshot.Claude, snapshot.ClaudeError = fetchClaudeQuotas()
	}()
	go func() {
		defer wg.Done()
		routing, err := fetchLLMQuotaRouting()
		if err == nil {
			snapshot.RoutingStrategy = routing.Strategy
		}
	}()
	wg.Wait()
	snapshot.FetchedAt = time.Now()
	_ = saveLLMQuotaCache(snapshot)
	return snapshot, nil
}

func fetchLLMQuotaRouting() (llmQuotaRouting, error) {
	var routing llmQuotaRouting
	key, err := loadCLIProxyManagementKey()
	if err != nil {
		return routing, err
	}
	baseURL := strings.TrimRight(firstNonEmpty(os.Getenv("CLIPROXY_MANAGEMENT_URL"), defaultCLIProxyManagement), "/")
	client := &http.Client{Timeout: quotaRequestTimeout}
	var authPayload struct {
		Files []cliproxyAuthFile `json:"files"`
	}
	if err := cliproxyManagementJSON(client, key, http.MethodGet, baseURL+"/auth-files", nil, &authPayload); err != nil {
		return routing, err
	}
	for _, file := range authPayload.Files {
		provider := firstNonEmpty(file.Provider, file.Type)
		account := quotaAccount{
			Label:    firstNonEmpty(file.Label, file.Email, file.Name, "account"),
			Name:     file.Name,
			Disabled: file.Disabled,
			Priority: file.Priority,
		}
		switch {
		case strings.EqualFold(provider, "claude"):
			routing.Claude = append(routing.Claude, account)
		case strings.EqualFold(provider, "codex"):
			routing.Codex = append(routing.Codex, account)
		}
	}
	sortQuotaAccounts(routing.Claude)
	sortQuotaAccounts(routing.Codex)
	var strategyPayload struct {
		Strategy string `json:"strategy"`
	}
	if err := cliproxyManagementJSON(client, key, http.MethodGet, baseURL+"/routing/strategy", nil, &strategyPayload); err == nil {
		routing.Strategy = strategyPayload.Strategy
	}
	return routing, nil
}

func llmQuotaCachePath() string {
	cacheRoot := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME"))
	if cacheRoot == "" {
		cacheRoot = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(cacheRoot, "agent", "llm-quotas.json")
}

func loadLLMQuotaCache() (llmQuotaSnapshot, bool) {
	data, err := os.ReadFile(llmQuotaCachePath())
	if err != nil {
		return llmQuotaSnapshot{}, false
	}
	var snapshot llmQuotaSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil || snapshot.FetchedAt.IsZero() {
		return llmQuotaSnapshot{}, false
	}
	for i := range snapshot.Codex {
		if strings.TrimSpace(snapshot.Codex[i].Name) == "" {
			return llmQuotaSnapshot{}, false
		}
	}
	for i := range snapshot.Claude {
		if strings.TrimSpace(snapshot.Claude[i].Name) == "" {
			return llmQuotaSnapshot{}, false
		}
	}
	age := time.Since(snapshot.FetchedAt)
	if age < 0 || age >= llmQuotaCacheTTL {
		return llmQuotaSnapshot{}, false
	}
	return snapshot, true
}

func saveLLMQuotaCache(snapshot llmQuotaSnapshot) error {
	path := llmQuotaCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fetchZAIQuota(client *http.Client) llmQuotaProvider {
	apiKey, err := loadSecret("ZAI_API_KEY", zaiKeychainService, zaiKeychainAccount)
	if err != nil {
		return llmQuotaProvider{Error: "Z.AI API key is missing from Keychain"}
	}
	url := firstNonEmpty(os.Getenv("ZAI_QUOTA_URL"), defaultZAIQuotaURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return llmQuotaProvider{Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return llmQuotaProvider{Error: "Z.AI: " + err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return llmQuotaProvider{Error: "Z.AI: " + err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llmQuotaProvider{Error: fmt.Sprintf("Z.AI returned HTTP %d", resp.StatusCode)}
	}
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Limits []struct {
				Type          string  `json:"type"`
				Unit          int     `json:"unit"`
				Percentage    float64 `json:"percentage"`
				NextResetTime int64   `json:"nextResetTime"`
			} `json:"limits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return llmQuotaProvider{Error: "Z.AI returned invalid JSON"}
	}
	if payload.Code != 0 && payload.Code != 200 {
		return llmQuotaProvider{Error: firstNonEmpty(payload.Msg, fmt.Sprintf("Z.AI error %d", payload.Code))}
	}
	provider := llmQuotaProvider{}
	for _, limit := range payload.Data.Limits {
		if strings.EqualFold(strings.TrimSpace(limit.Type), "TIME_LIMIT") {
			continue
		}
		window := llmQuotaWindow{Label: zaiQuotaLabel(limit.Type, limit.Unit), UsedPercent: limit.Percentage}
		if limit.NextResetTime > 0 {
			window.ResetAt = time.UnixMilli(limit.NextResetTime)
		}
		provider.Windows = append(provider.Windows, window)
	}
	sort.SliceStable(provider.Windows, func(i, j int) bool {
		return zaiQuotaOrder(provider.Windows[i].Label) < zaiQuotaOrder(provider.Windows[j].Label)
	})
	return provider
}

func zaiQuotaLabel(limitType string, unit int) string {
	switch strings.ToUpper(strings.TrimSpace(limitType)) {
	case "TOKENS_LIMIT":
		switch unit {
		case 3:
			return "Coding 5-hour"
		case 6:
			return "Coding weekly"
		}
	}
	return fmt.Sprintf("%s / unit %d", strings.TrimSpace(limitType), unit)
}

func zaiQuotaOrder(label string) int {
	switch label {
	case "Coding 5-hour":
		return 0
	case "Coding weekly":
		return 1
	default:
		return 2
	}
}

func fetchCLIProxyAccountQuotas(providerName string, fetchAccount func(*http.Client, string, string, cliproxyAuthFile) quotaAccount) ([]quotaAccount, string) {
	key, err := loadCLIProxyManagementKey()
	if err != nil {
		return nil, err.Error()
	}
	baseURL := strings.TrimRight(firstNonEmpty(os.Getenv("CLIPROXY_MANAGEMENT_URL"), defaultCLIProxyManagement), "/")
	client := &http.Client{Timeout: codexQuotaRequestTimeout}
	var authPayload struct {
		Files []cliproxyAuthFile `json:"files"`
	}
	if err := cliproxyManagementJSON(client, key, http.MethodGet, baseURL+"/auth-files", nil, &authPayload); err != nil {
		return nil, err.Error()
	}
	files := make([]cliproxyAuthFile, 0, len(authPayload.Files))
	for _, file := range authPayload.Files {
		provider := firstNonEmpty(file.Provider, file.Type)
		if strings.EqualFold(provider, providerName) {
			files = append(files, file)
		}
	}
	accounts := make([]quotaAccount, len(files))
	var wg sync.WaitGroup
	for idx, file := range files {
		if file.Disabled {
			accounts[idx] = quotaAccount{
				Label:    firstNonEmpty(file.Label, file.Email, file.Name, "account"),
				Name:     file.Name,
				Disabled: true,
				Priority: file.Priority,
			}
			continue
		}
		wg.Add(1)
		go func(idx int, file cliproxyAuthFile) {
			defer wg.Done()
			accounts[idx] = fetchAccount(client, baseURL, key, file)
		}(idx, file)
	}
	wg.Wait()
	sortQuotaAccounts(accounts)
	return accounts, ""
}

func fetchCodexQuotas() ([]quotaAccount, string) {
	return fetchCLIProxyAccountQuotas("codex", fetchCodexAccountQuota)
}

func fetchClaudeQuotas() ([]quotaAccount, string) {
	return fetchCLIProxyAccountQuotas("claude", fetchClaudeAccountQuota)
}

func fetchCodexAccountQuota(client *http.Client, baseURL, key string, file cliproxyAuthFile) quotaAccount {
	account := quotaAccount{
		Label:    firstNonEmpty(file.Label, file.Email, file.Name, "Codex account"),
		Name:     file.Name,
		Priority: file.Priority,
	}
	if strings.TrimSpace(file.AuthIndex) == "" {
		account.Error = "Missing auth index"
		return account
	}
	headers := map[string]string{
		"Authorization": "Bearer $TOKEN$",
		"Content-Type":  "application/json",
		"User-Agent":    codexUsageUserAgent,
	}
	if id := strings.TrimSpace(file.IDToken.ChatGPTAccountID); id != "" {
		headers["Chatgpt-Account-Id"] = id
	}
	requestBody := map[string]any{
		"auth_index": file.AuthIndex,
		"method":     http.MethodGet,
		"url":        codexUsageURL,
		"header":     headers,
	}
	var response struct {
		StatusCode int    `json:"status_code"`
		Body       string `json:"body"`
	}
	if err := cliproxyManagementJSON(client, key, http.MethodPost, baseURL+"/api-call", requestBody, &response); err != nil {
		account.Error = err.Error()
		return account
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		account.Error = fmt.Sprintf("Codex returned HTTP %d", response.StatusCode)
		return account
	}
	decoder := json.NewDecoder(strings.NewReader(response.Body))
	decoder.UseNumber()
	var usage codexUsagePayload
	if err := decoder.Decode(&usage); err != nil {
		account.Error = "Codex returned invalid usage data"
		return account
	}
	account.Plan = strings.TrimSpace(usage.PlanType)
	account.Windows = appendCodexRateWindows(account.Windows, "Code", usage.RateLimit)
	account.Windows = appendCodexRateWindows(account.Windows, "Review", usage.CodeReviewRateLimit)
	for _, additional := range usage.AdditionalRateLimits {
		label := firstNonEmpty(additional.LimitName, additional.MeteredFeature, "Additional")
		account.Windows = appendCodexRateWindows(account.Windows, label, additional.RateLimit)
	}
	return account
}

func fetchClaudeAccountQuota(client *http.Client, baseURL, key string, file cliproxyAuthFile) quotaAccount {
	account := quotaAccount{
		Label:    firstNonEmpty(file.Label, file.Email, file.Name, "Claude account"),
		Name:     file.Name,
		Priority: file.Priority,
	}
	if strings.TrimSpace(file.AuthIndex) == "" {
		account.Error = "Missing auth index"
		return account
	}
	requestBody := map[string]any{
		"auth_index": file.AuthIndex,
		"method":     http.MethodGet,
		"url":        claudeUsageURL,
		"header": map[string]string{
			"Authorization":  "Bearer $TOKEN$",
			"Content-Type":   "application/json",
			"User-Agent":     claudeUsageUserAgent,
			"anthropic-beta": "oauth-2025-04-20",
		},
	}
	var response struct {
		StatusCode int    `json:"status_code"`
		Body       string `json:"body"`
	}
	if err := cliproxyManagementJSON(client, key, http.MethodPost, baseURL+"/api-call", requestBody, &response); err != nil {
		account.Error = err.Error()
		return account
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		account.Error = fmt.Sprintf("Claude returned HTTP %d", response.StatusCode)
		return account
	}
	var usage claudeUsagePayload
	if err := json.NewDecoder(strings.NewReader(response.Body)).Decode(&usage); err != nil {
		account.Error = "Claude returned invalid usage data"
		return account
	}
	account.Windows = appendClaudeUsageWindow(account.Windows, "5-hour", usage.FiveHour)
	account.Windows = appendClaudeUsageWindow(account.Windows, "weekly", usage.SevenDay)
	return account
}

func appendClaudeUsageWindow(windows []llmQuotaWindow, label string, raw *claudeUsageWindow) []llmQuotaWindow {
	if raw == nil {
		return windows
	}
	window := llmQuotaWindow{Label: label, UsedPercent: raw.Utilization}
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.ResetsAt)); err == nil {
		window.ResetAt = parsed
	}
	return append(windows, window)
}

func appendCodexRateWindows(windows []llmQuotaWindow, prefix string, rateLimit *codexRateLimit) []llmQuotaWindow {
	if rateLimit == nil {
		return windows
	}
	for index, raw := range []*codexUsageWindow{rateLimit.PrimaryWindow, rateLimit.SecondaryWindow} {
		if raw == nil {
			continue
		}
		used := jsonNumberFloat(raw.UsedPercent)
		if (rateLimit.LimitReached || (rateLimit.Allowed != nil && !*rateLimit.Allowed)) && used == 0 {
			used = 100
		}
		seconds := int64(jsonNumberFloat(raw.LimitWindowSeconds))
		label := codexWindowLabel(prefix, seconds, index)
		resetAt := time.Time{}
		if value := int64(jsonNumberFloat(raw.ResetAt)); value > 0 {
			resetAt = time.Unix(value, 0)
		} else if value := int64(jsonNumberFloat(raw.ResetAfterSeconds)); value > 0 {
			resetAt = time.Now().Add(time.Duration(value) * time.Second)
		}
		windows = append(windows, llmQuotaWindow{Label: label, UsedPercent: used, ResetAt: resetAt})
	}
	return windows
}

func codexWindowLabel(prefix string, seconds int64, index int) string {
	window := "window"
	switch {
	case seconds == 18000:
		window = "5-hour"
	case seconds == 604800:
		window = "weekly"
	case seconds >= 28*24*60*60 && seconds <= 31*24*60*60:
		window = "monthly"
	case seconds > 0:
		window = formatWindowDuration(seconds)
	case index == 0:
		window = "primary"
	default:
		window = "secondary"
	}
	if strings.EqualFold(prefix, "Code") {
		return window
	}
	return prefix + " " + window
}

func formatWindowDuration(seconds int64) string {
	duration := time.Duration(seconds) * time.Second
	if duration%(24*time.Hour) == 0 {
		return fmt.Sprintf("%d-day", int(duration/(24*time.Hour)))
	}
	if duration%time.Hour == 0 {
		return fmt.Sprintf("%d-hour", int(duration/time.Hour))
	}
	return "window"
}

func loadCLIProxyManagementKey() (string, error) {
	return loadSecret("CLIPROXY_MANAGEMENT_KEY", cliproxyKeychainService, cliproxyKeychainAccount)
}

func loadSecret(envName, service, account string) (string, error) {
	if key := strings.TrimSpace(os.Getenv(envName)); key != "" {
		return key, nil
	}
	cmd := exec.Command("security", "find-generic-password", "-s", service, "-a", account, "-w")
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return "", fmt.Errorf("secret %s is unavailable", envName)
	}
	return strings.TrimSpace(string(output)), nil
}

func cliproxyManagementJSON(client *http.Client, key, method, url string, input any, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("CLIProxy: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("CLIProxy: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("CLIProxy management returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("CLIProxy returned invalid JSON")
	}
	return nil
}

func jsonNumberFloat(value json.Number) float64 {
	if strings.TrimSpace(value.String()) == "" {
		return 0
	}
	result, _ := strconv.ParseFloat(value.String(), 64)
	return result
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
