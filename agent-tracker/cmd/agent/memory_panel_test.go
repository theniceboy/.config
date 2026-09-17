package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemoryStateForMatchingAndPrecedence(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	enabled := map[string]bool{
		"~/glob*":          true,
		"~/sub":            true,
		"~/e*":             true,
		"/home/tester/ex":  false,
		"~/yes":            false,
		"/home/tester/yes": true,
	}
	cases := []struct {
		dir    string
		want   bool
		viaKey string
	}{
		{dir: "/home/tester/glob-project", want: true, viaKey: "~/glob*"},
		{dir: "/home/tester/glob-project/.agents/feat/repo", want: true, viaKey: "~/glob*"},
		{dir: "/home/tester/sub/deep/nested", want: true, viaKey: "~/sub"},
		{dir: "/home/tester/ex", want: false, viaKey: "/home/tester/ex"},
		{dir: "/home/tester/yes", want: true, viaKey: "/home/tester/yes"},
		{dir: "/home/tester/other", want: false, viaKey: ""},
		{dir: "~/glob-x", want: true, viaKey: "~/glob*"},
	}
	for _, tc := range cases {
		got, via := memoryStateFor(tc.dir, enabled)
		if got != tc.want || via != tc.viaKey {
			t.Errorf("memoryStateFor(%q) = (%v, %q), want (%v, %q)", tc.dir, got, via, tc.want, tc.viaKey)
		}
	}
}

func TestMemoryKeyMatchesDir(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	cases := []struct {
		key, dir string
		want     bool
	}{
		{key: "~/Github/app*", dir: "/home/tester/Github/app", want: true},
		{key: "~/Github/app*", dir: "/home/tester/Github/app-pseo", want: true},
		{key: "~/Github/app*", dir: "/home/tester/Github/app/.agents/feat/repo", want: true},
		{key: "~/Github/app*", dir: "/home/tester/Github/other", want: false},
		{key: "~/a?c", dir: "/home/tester/abc", want: true},
		{key: "~/a?c", dir: "/home/tester/ac", want: false},
		{key: "~/plain", dir: "/home/tester/plain/x", want: true},
		{key: "/abs/dir", dir: "/abs/dir", want: true},
		{key: "/abs/dir", dir: "/abs/dirx", want: false},
	}
	for _, tc := range cases {
		if got := memoryKeyMatchesDir(tc.key, tc.dir); got != tc.want {
			t.Errorf("memoryKeyMatchesDir(%q, %q) = %v, want %v", tc.key, tc.dir, got, tc.want)
		}
	}
}

func TestSetBaseMemoryEnabledPreservesAndOverrides(t *testing.T) {
	t.Setenv("HOME", "/home/tester")
	path := filepath.Join(t.TempDir(), "memory.json")
	initial := `{"note": "keep", "enabled": {"~/glob*": true}}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := setBaseMemoryEnabled(path, "/home/tester/glob-project", false); err != nil {
		t.Fatal(err)
	}
	raw := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["note"]) != `"keep"` {
		t.Errorf("top-level key not preserved: %s", string(raw["note"]))
	}
	enabled := map[string]bool{}
	if err := json.Unmarshal(raw["enabled"], &enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled["~/glob*"] {
		t.Errorf("existing enabled entry lost: %#v", enabled)
	}
	if value := enabled["/home/tester/glob-project"]; value {
		t.Errorf("exact key should be false to override the glob, got %v", value)
	}
	if on, _ := memoryStateFor("/home/tester/glob-project", enabled); on {
		t.Error("exact false key must win over matching true glob")
	}

	if err := setBaseMemoryEnabled(path, "/home/tester/glob-project", true); err != nil {
		t.Fatal(err)
	}
	enabled, err = loadBaseMemoryEnabled(path)
	if err != nil {
		t.Fatal(err)
	}
	if on, _ := memoryStateFor("/home/tester/glob-project", enabled); !on {
		t.Error("exact true key should enable the directory")
	}

	if err := setBaseMemoryEnabled(path, "/home/tester/fresh", true); err != nil {
		t.Fatal(err)
	}
	enabled, err = loadBaseMemoryEnabled(path)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled["/home/tester/fresh"] {
		t.Errorf("missing exact key after write: %#v", enabled)
	}
}

func TestLoadBaseMemoryEnabledMissingFile(t *testing.T) {
	enabled, err := loadBaseMemoryEnabled(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 0 {
		t.Errorf("expected empty map for missing file, got %#v", enabled)
	}
}

func TestMemoryPanelLoadsRegistrationsAndToggles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	stateDir := filepath.Join(t.TempDir(), "op")
	t.Setenv("XDG_STATE_HOME", filepath.Dir(stateDir))
	if err := os.MkdirAll(filepath.Join(home, "base", "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(home, "base", "state", "memory.json")
	if err := os.WriteFile(statePath, []byte(`{"enabled": {"~/glob*": true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := `{"v":1,"sessionID":"ses_x","directory":"` + home + `/glob-app","pane":{"paneId":"%9"},"pid":1,"heartbeat":` + fmt.Sprintf("%d", time.Now().UnixMilli()) + `}`
	if err := os.WriteFile(filepath.Join(stateDir, "ses_ses_x"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	origLoader := tmuxPaneCtxLoader
	tmuxPaneCtxLoader = func() map[string]opPaneCtx {
		return map[string]opPaneCtx{
			"%9": {PaneID: "%9", WindowID: "@9", SessionName: "1-IB", WindowIndex: "3", PaneIndex: "1"},
		}
	}
	t.Cleanup(func() { tmuxPaneCtxLoader = origLoader })

	m := newMemoryPanelModel("@9")
	if !m.hasAgent {
		t.Fatal("expected the @9 window's agent to load")
	}
	if m.directory != filepath.Join(home, "glob-app") {
		t.Errorf("directory = %q", m.directory)
	}
	if !m.enabled || m.viaKey != "~/glob*" {
		t.Errorf("expected enabled via ~/glob*, got (%v, %q)", m.enabled, m.viaKey)
	}
	if m.stale {
		t.Error("fresh heartbeat should not be stale")
	}

	initialView := m.View()
	for _, want := range []string{"Base memory", "toggle", "via ~/glob*", "ON"} {
		if !strings.Contains(initialView, want) {
			t.Errorf("panel view missing %q", want)
		}
	}

	// A different window must show the no-agent state.
	other := newMemoryPanelModel("@404")
	if other.hasAgent {
		t.Error("window without registration should have no agent")
	}
	if !strings.Contains(other.View(), "No opencode session") {
		t.Error("no-agent view should explain itself")
	}

	m.handleKey(" ")
	enabled, err := loadBaseMemoryEnabled(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if value := enabled[filepath.Join(home, "glob-app")]; value {
		t.Errorf("toggle should write exact false key, got %#v", enabled)
	}
	if on, _ := memoryStateFor(m.directory, enabled); on {
		t.Error("exact false key must override the true glob")
	}
	if m.enabled {
		t.Error("panel state not refreshed after toggle")
	}

	m.handleKey("t")
	enabled, err = loadBaseMemoryEnabled(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled[filepath.Join(home, "glob-app")] {
		t.Errorf("toggle back should write exact true key, got %#v", enabled)
	}
	if !m.enabled {
		t.Error("panel state not refreshed after toggle back")
	}

	m.handleKey("esc")
	if !m.requestBack {
		t.Error("esc should request back")
	}
}
