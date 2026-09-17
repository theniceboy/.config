package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type memoryUsageFile struct {
	Store  string `json:"store"`
	Path   string `json:"path"`
	Tokens int    `json:"tokens"`
}

type memoryUsage struct {
	Status        string            `json:"status"`
	SessionID     string            `json:"sessionID"`
	Directory     string            `json:"directory"`
	GlobalEnabled bool              `json:"globalEnabled"`
	Fingerprint   string            `json:"fingerprint"`
	AllMode       bool              `json:"allMode"`
	AllStores     []string          `json:"allStores"`
	HasRepo       bool              `json:"hasRepo"`
	Files         []memoryUsageFile `json:"files"`
	TotalTokens   int               `json:"totalTokens"`
	Error         string            `json:"error"`
}

type memoryUsageMsg struct {
	panel *memoryPanelModel
	usage memoryUsage
}
type memoryPanelTickMsg struct {
	panel      *memoryPanelModel
	generation uint64
}

func (m *memoryPanelModel) tick() tea.Cmd {
	generation := m.pollGeneration
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return memoryPanelTickMsg{m, generation} })
}

func (m *memoryPanelModel) requestUsage() tea.Cmd {
	if !m.hasAgent || m.usageInFlight {
		return nil
	}
	m.usageInFlight = true
	sessionID, fingerprint := m.sessionID, m.usage.Fingerprint
	return func() tea.Msg {
		result := memoryUsage{SessionID: sessionID, Status: "error"}
		home, err := os.UserHomeDir()
		if err != nil {
			result.Error = err.Error()
			return memoryUsageMsg{m, result}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "python3", "-B", filepath.Join(home, "base", "scripts", "memory-usage.py"),
			sessionID, "--fingerprint", fingerprint).Output()
		if err == nil {
			err = json.Unmarshal(out, &result)
		}
		if err != nil {
			result.Error = err.Error()
		}
		return memoryUsageMsg{m, result}
	}
}

func (m *memoryPanelModel) acceptUsage(msg memoryUsageMsg) {
	if msg.panel != m {
		return
	}
	m.usageInFlight = false
	if msg.usage.SessionID != m.sessionID {
		return
	}
	if msg.usage.Status != "unchanged" {
		m.usage = msg.usage
	}
	if msg.usage.Status == "ready" {
		m.directory = msg.usage.Directory
		m.enabled = msg.usage.GlobalEnabled
		m.hasRepo = msg.usage.HasRepo
		m.allGlobal = false
		m.allRepo = false
		for _, store := range msg.usage.AllStores {
			if store == "global" {
				m.allGlobal = true
			}
			if store == "repo" {
				m.allRepo = true
			}
		}
	}
}
