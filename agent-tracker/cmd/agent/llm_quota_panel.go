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
	defaultCLIProxyManagement = "https://azwestus.asurada.dev/v0/management"
	zaiKeychainService        = "zai-api"
	zaiKeychainAccount        = "api.z.ai"
	cliproxyKeychainService   = "cliproxy-management"
	cliproxyKeychainAccount   = "azwestus.asurada.dev"
	codexUsageURL             = "https://chatgpt.com/backend-api/wham/usage"
	codexUsageUserAgent       = "codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal"
	quotaRequestTimeout       = 18 * time.Second
	codexQuotaRequestTimeout  = 65 * time.Second
	llmQuotaCacheTTL          = 5 * time.Minute
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

type codexQuotaAccount struct {
	Label   string           `json:"label"`
	Plan    string           `json:"plan,omitempty"`
	Windows []llmQuotaWindow `json:"windows"`
	Error   string           `json:"error,omitempty"`
}

type llmQuotaSnapshot struct {
	ZAI        llmQuotaProvider    `json:"zai"`
	Codex      []codexQuotaAccount `json:"codex"`
	CodexError string              `json:"codex_error,omitempty"`
	FetchedAt  time.Time           `json:"fetched_at"`
}

type llmQuotaResultMsg struct {
	snapshot llmQuotaSnapshot
	err      error
}

type llmQuotaPanelModel struct {
	width           int
	height          int
	snapshot        llmQuotaSnapshot
	loaded          bool
	message         string
	refreshInFlight bool
	pendingRefresh  bool
	pendingForce    bool
	showAltHints    bool
	requestBack     bool
}

type cliproxyAuthFile struct {
	Provider  string `json:"provider"`
	Type      string `json:"type"`
	Label     string `json:"label"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AuthIndex string `json:"auth_index"`
	Disabled  bool   `json:"disabled"`
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
		}
		if m.pendingRefresh {
			force := m.pendingForce
			m.pendingRefresh = false
			m.pendingForce = false
			return m, m.requestRefreshCmd(force)
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
	header := lipgloss.JoinVertical(lipgloss.Left,
		styles.title.Render("LLM Quotas"),
		styles.meta.Render("Live subscription usage from Z.AI and CLIProxyAPI"),
	)
	body := ""
	if !m.loaded {
		body = styles.muted.Render("Loading provider quotas...")
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left,
			m.renderZAI(styles, contentWidth),
			"",
			m.renderCodex(styles, contentWidth),
		)
	}
	footer := m.renderFooter(styles, contentWidth)
	bodyHeight := height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if bodyHeight > lipgloss.Height(body) {
		body = lipgloss.NewStyle().Height(bodyHeight).Render(body)
	}
	view := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
	return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(view)
}

func (m *llmQuotaPanelModel) renderZAI(styles paletteStyles, width int) string {
	lines := []string{styles.panelTitle.Render("Z.AI Coding Plan")}
	if m.snapshot.ZAI.Error != "" {
		lines = append(lines, styles.statusBad.Render(truncate(m.snapshot.ZAI.Error, width)))
	}
	if len(m.snapshot.ZAI.Windows) == 0 && m.snapshot.ZAI.Error == "" {
		lines = append(lines, styles.muted.Render("No quota windows returned"))
	}
	var coding []llmQuotaWindow
	var rest []llmQuotaWindow
	for _, window := range m.snapshot.ZAI.Windows {
		if window.Label == "Coding 5-hour" || window.Label == "Coding weekly" {
			coding = append(coding, window)
		} else {
			rest = append(rest, window)
		}
	}
	if len(coding) > 0 {
		columnWidth := maxInt(24, (width-2*(len(coding)-1))/len(coding))
		blocks := make([]string, 0, len(coding)*2-1)
		for idx, window := range coding {
			if idx > 0 {
				blocks = append(blocks, "  ")
			}
			blocks = append(blocks, renderLLMQuotaWindow(styles, window, minInt(columnWidth, width)))
		}
		lines = append(lines, "", lipgloss.JoinHorizontal(lipgloss.Top, blocks...))
	}
	for _, window := range rest {
		lines = append(lines, "", renderLLMQuotaWindow(styles, window, width))
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m *llmQuotaPanelModel) renderCodex(styles paletteStyles, width int) string {
	lines := []string{styles.panelTitle.Render("Codex via CLIProxyAPI")}
	if m.snapshot.CodexError != "" {
		lines = append(lines, styles.statusBad.Render(truncate(m.snapshot.CodexError, width)))
	}
	if len(m.snapshot.Codex) == 0 && m.snapshot.CodexError == "" {
		lines = append(lines, styles.muted.Render("No enabled Codex accounts"))
	}
	for idx, account := range m.snapshot.Codex {
		if idx > 0 {
			lines = append(lines, "")
		}
		label := styles.itemTitle.Render(truncate(account.Label, maxInt(12, width-8)))
		if account.Plan != "" {
			label += "  " + styles.keyword.Render(strings.ToUpper(account.Plan))
		}
		lines = append(lines, label)
		if account.Error != "" {
			lines = append(lines, styles.statusBad.Render(truncate(account.Error, width)))
			continue
		}
		if len(account.Windows) == 0 {
			lines = append(lines, styles.muted.Render("No quota windows returned"))
			continue
		}
		for _, window := range account.Windows {
			lines = append(lines, renderLLMQuotaWindow(styles, window, width))
		}
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func renderLLMQuotaWindow(styles paletteStyles, window llmQuotaWindow, width int) string {
	used := clampFloat(window.UsedPercent, 0, 100)
	remaining := 100 - used
	labelWidth := minInt(20, maxInt(12, width/3))
	barWidth := maxInt(8, minInt(24, width-labelWidth-18))
	bar := renderQuotaBar(remaining, barWidth)
	valueStyle := styles.panelText
	if remaining <= 10 {
		valueStyle = styles.statusBad
	} else if remaining >= 60 {
		valueStyle = styles.todoCheckDone
	}
	line := styles.muted.Copy().Width(labelWidth).Render(window.Label) + " " +
		valueStyle.Render(fmt.Sprintf("%3.0f%% free", remaining)) + "  " + styles.panelText.Render(bar)
	if !window.ResetAt.IsZero() {
		line += "\n" + strings.Repeat(" ", labelWidth+1) + styles.muted.Render(formatQuotaReset(window.ResetAt))
	}
	return line
}

func renderQuotaBar(remaining float64, width int) string {
	width = maxInt(1, width)
	filled := int((clampFloat(remaining, 0, 100)/100)*float64(width) + 0.5)
	return "[" + strings.Repeat("=", filled) + strings.Repeat(".", width-filled) + "]"
}

func formatQuotaReset(resetAt time.Time) string {
	delta := time.Until(resetAt)
	if delta <= 0 {
		return "reset due now"
	}
	delta = delta.Round(time.Minute)
	parts := []string{}
	if days := int(delta / (24 * time.Hour)); days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
		delta -= time.Duration(days) * 24 * time.Hour
	}
	if hours := int(delta / time.Hour); hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
		delta -= time.Duration(hours) * time.Hour
	}
	if len(parts) < 2 {
		if minutes := int(delta / time.Minute); minutes > 0 {
			parts = append(parts, fmt.Sprintf("%dm", minutes))
		}
	}
	return "resets in " + strings.Join(parts, " ") + "  " + resetAt.Local().Format("Jan 2 15:04")
}

func (m *llmQuotaPanelModel) renderFooter(styles paletteStyles, width int) string {
	pairs := [][2]string{{"r", "refresh"}, {"Esc", "back"}, {footerHintToggleKey, "more"}}
	if m.showAltHints {
		pairs = [][2]string{{"Alt-S", "close"}, {footerHintToggleKey, "hide"}}
	}
	footer := renderShortcutPairs(func(v string) string { return styles.shortcutKey.Render(v) }, func(v string) string { return styles.shortcutText.Render(v) }, "   ", pairs)
	if !m.snapshot.FetchedAt.IsZero() && !m.refreshInFlight {
		stamp := styles.muted.Render("Updated " + m.snapshot.FetchedAt.Local().Format("15:04:05"))
		if lipgloss.Width(stamp)+3+lipgloss.Width(footer) <= width {
			footer = stamp + "   " + footer
		}
	}
	if m.refreshInFlight {
		footer = styles.footer.Render("Refreshing...")
	}
	return lipgloss.NewStyle().Width(width).Render(footer)
}

func (m *llmQuotaPanelModel) currentStatus() string {
	if m.refreshInFlight {
		return "Refreshing LLM quotas..."
	}
	if m.loaded {
		return "LLM quotas updated"
	}
	return strings.TrimSpace(m.message)
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

func fetchLLMQuotaSnapshot(force bool) (llmQuotaSnapshot, error) {
	if !force {
		if snapshot, ok := loadLLMQuotaCache(); ok {
			return snapshot, nil
		}
	}
	client := &http.Client{Timeout: quotaRequestTimeout}
	var snapshot llmQuotaSnapshot
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		snapshot.ZAI = fetchZAIQuota(client)
	}()
	go func() {
		defer wg.Done()
		snapshot.Codex, snapshot.CodexError = fetchCodexQuotas()
	}()
	wg.Wait()
	snapshot.FetchedAt = time.Now()
	_ = saveLLMQuotaCache(snapshot)
	return snapshot, nil
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

func fetchCodexQuotas() ([]codexQuotaAccount, string) {
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
		if strings.EqualFold(provider, "codex") && !file.Disabled {
			files = append(files, file)
		}
	}
	accounts := make([]codexQuotaAccount, len(files))
	var wg sync.WaitGroup
	for idx, file := range files {
		wg.Add(1)
		go func(idx int, file cliproxyAuthFile) {
			defer wg.Done()
			accounts[idx] = fetchCodexAccountQuota(client, baseURL, key, file)
		}(idx, file)
	}
	wg.Wait()
	sort.SliceStable(accounts, func(i, j int) bool { return strings.ToLower(accounts[i].Label) < strings.ToLower(accounts[j].Label) })
	return accounts, ""
}

func fetchCodexAccountQuota(client *http.Client, baseURL, key string, file cliproxyAuthFile) codexQuotaAccount {
	account := codexQuotaAccount{Label: firstNonEmpty(file.Label, file.Email, file.Name, "Codex account")}
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
