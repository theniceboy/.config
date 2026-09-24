package main

import (
	"testing"
	"time"
)

func intPtr(v int) *int { return &v }

func TestProviderMode(t *testing.T) {
	m := &llmQuotaPanelModel{}
	m.snapshot.Claude = []quotaAccount{
		{Name: "a", Priority: intPtr(100)},
		{Name: "b", Priority: intPtr(90)},
	}
	if mode := m.providerMode("claude"); mode != "drain" {
		t.Fatalf("distinct priorities should be drain, got %s", mode)
	}
	m.snapshot.Claude = []quotaAccount{
		{Name: "a", Priority: intPtr(50)},
		{Name: "b", Priority: intPtr(50)},
	}
	if mode := m.providerMode("claude"); mode != "spread" {
		t.Fatalf("equal priorities should be spread, got %s", mode)
	}
	m.snapshot.Claude = []quotaAccount{
		{Name: "a"},
		{Name: "b"},
	}
	if mode := m.providerMode("claude"); mode != "spread" {
		t.Fatalf("nil priorities should be spread, got %s", mode)
	}
	m.snapshot.Claude = []quotaAccount{
		{Name: "a", Priority: intPtr(100)},
		{Name: "b", Priority: intPtr(100), Disabled: true},
	}
	if mode := m.providerMode("claude"); mode != "drain" {
		t.Fatalf("single enabled account should be drain, got %s", mode)
	}
}

func TestSortQuotaAccounts(t *testing.T) {
	accounts := []quotaAccount{
		{Name: "nil-b", Label: "zoe"},
		{Name: "mid", Label: "mid", Priority: intPtr(50)},
		{Name: "top", Label: "top", Priority: intPtr(100)},
		{Name: "nil-a", Label: "adam"},
	}
	sortQuotaAccounts(accounts)
	want := []string{"top", "mid", "nil-a", "nil-b"}
	for i, name := range want {
		if accounts[i].Name != name {
			t.Fatalf("position %d: want %s got %s", i, name, accounts[i].Name)
		}
	}
}

func TestShortWindowLabel(t *testing.T) {
	cases := map[string]string{
		"5-hour":       "5h",
		"weekly":       "wk",
		"monthly":      "mo",
		"Review 5-hour": "rev 5h",
		"Code weekly":  "wk",
	}
	for input, want := range cases {
		if got := shortWindowLabel(input); got != want {
			t.Fatalf("shortWindowLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatQuotaResetShort(t *testing.T) {
	if got := formatQuotaResetShort(time.Time{}); got != "" {
		t.Fatalf("zero time should be empty, got %q", got)
	}
	if got := formatQuotaResetShort(time.Now().Add(42 * time.Minute)); got != "41m" && got != "42m" {
		t.Fatalf("42 minutes: got %q", got)
	}
	if got := formatQuotaResetShort(time.Now().Add(90 * time.Minute)); got != "1h29m" && got != "1h30m" {
		t.Fatalf("90 minutes: got %q", got)
	}
	if got := formatQuotaResetShort(time.Now().Add(30 * time.Hour)); got != "30h" {
		t.Fatalf("30 hours: got %q", got)
	}
	if got := formatQuotaResetShort(time.Now().Add(50 * time.Hour)); got != "2d1h" && got != "2d2h" {
		t.Fatalf("50 hours: got %q", got)
	}
}

func TestRebuildRowsCursorSkipsZAI(t *testing.T) {
	m := &llmQuotaPanelModel{}
	m.snapshot.Claude = []quotaAccount{{Name: "c1", Label: "claude one"}}
	m.snapshot.Codex = []quotaAccount{{Name: "x1", Label: "codex one"}}
	m.rebuildRows()
	for i, row := range m.rows {
		wantInteractive := row.provider == "claude" || row.provider == "codex"
		if m.interactive(i) != wantInteractive {
			t.Fatalf("row %d provider %s interactive=%v", i, row.provider, m.interactive(i))
		}
	}
	m.cursor = len(m.rows) - 1
	m.ensureCursorInteractive()
	if !m.interactive(m.cursor) {
		t.Fatal("cursor should land on an interactive row")
	}
}
