package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type todoScope int

const (
	todoScopeGlobal todoScope = iota
	todoScopeSession
	todoScopeWindow
)

const tmuxTodoStoreVersion = 1

type tmuxTodoItem struct {
	Title     string    `json:"title" yaml:"title"`
	Done      bool      `json:"done" yaml:"done"`
	Priority  int       `json:"priority,omitempty" yaml:"priority,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty" yaml:"created_at,omitempty"`
}

type tmuxTodoStore struct {
	Version  int                       `json:"version"`
	Global   []tmuxTodoItem            `json:"global,omitempty"`
	Sessions map[string][]tmuxTodoItem `json:"sessions,omitempty"`
	Windows  map[string][]tmuxTodoItem `json:"windows,omitempty"`
}

type tmuxTodoListFile struct {
	Todos []tmuxTodoItem `json:"todos" yaml:"todos"`
}

type tmuxTodoEntry struct {
	Title     string
	Done      bool
	Priority  int
	Scope     todoScope
	ScopeID   string
	ScopeName string
	IsCurrent bool
	ItemIndex int
}

func tmuxTodoStorePath() string {
	return filepath.Join(os.Getenv("HOME"), ".cache", "agent", "todos.json")
}

func legacyTmuxTodosDir() string {
	return filepath.Join(os.Getenv("HOME"), ".tmux-todos")
}

func normalizeTodoPriority(priority int) int {
	if priority < 1 || priority > 3 {
		return 2
	}
	return priority
}

func normalizeTmuxTodoStore(store *tmuxTodoStore) *tmuxTodoStore {
	if store == nil {
		store = &tmuxTodoStore{}
	}
	if store.Version == 0 {
		store.Version = tmuxTodoStoreVersion
	}
	if store.Global == nil {
		store.Global = []tmuxTodoItem{}
	}
	if store.Sessions == nil {
		store.Sessions = map[string][]tmuxTodoItem{}
	}
	if store.Windows == nil {
		store.Windows = map[string][]tmuxTodoItem{}
	}
	for i := range store.Global {
		store.Global[i].Priority = normalizeTodoPriority(store.Global[i].Priority)
	}
	for key := range store.Sessions {
		for i := range store.Sessions[key] {
			store.Sessions[key][i].Priority = normalizeTodoPriority(store.Sessions[key][i].Priority)
		}
	}
	for key := range store.Windows {
		for i := range store.Windows[key] {
			store.Windows[key][i].Priority = normalizeTodoPriority(store.Windows[key][i].Priority)
		}
	}
	return store
}

func saveTmuxTodoStore(store *tmuxTodoStore) error {
	store = normalizeTmuxTodoStore(store)
	path := tmuxTodoStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func loadTmuxTodoStore() (*tmuxTodoStore, error) {
	path := tmuxTodoStorePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return bootstrapTmuxTodoStore()
		}
		return nil, err
	}
	var store tmuxTodoStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return normalizeTmuxTodoStore(&store), nil
}

func bootstrapTmuxTodoStore() (*tmuxTodoStore, error) {
	store := normalizeTmuxTodoStore(&tmuxTodoStore{Version: tmuxTodoStoreVersion})
	if err := importLegacyYamlTodos(store); err != nil {
		return nil, err
	}
	if err := saveTmuxTodoStore(store); err != nil {
		return nil, err
	}
	return store, nil
}

func todoItemsForScope(store *tmuxTodoStore, scope todoScope, scopeID string) []tmuxTodoItem {
	store = normalizeTmuxTodoStore(store)
	switch scope {
	case todoScopeSession:
		return store.Sessions[scopeID]
	case todoScopeWindow:
		return store.Windows[scopeID]
	default:
		return store.Global
	}
}

func setTodoItemsForScope(store *tmuxTodoStore, scope todoScope, scopeID string, items []tmuxTodoItem) {
	store = normalizeTmuxTodoStore(store)
	switch scope {
	case todoScopeSession:
		store.Sessions[scopeID] = items
	case todoScopeWindow:
		store.Windows[scopeID] = items
	default:
		store.Global = items
	}
}

func appendUniqueTodo(store *tmuxTodoStore, scope todoScope, scopeID string, item tmuxTodoItem) bool {
	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		return false
	}
	item.Priority = normalizeTodoPriority(item.Priority)
	items := append([]tmuxTodoItem(nil), todoItemsForScope(store, scope, scopeID)...)
	for _, existing := range items {
		if strings.TrimSpace(existing.Title) == item.Title {
			return false
		}
	}
	items = append(items, item)
	setTodoItemsForScope(store, scope, scopeID, items)
	return true
}

func importLegacyYamlTodos(store *tmuxTodoStore) error {
	entries, err := os.ReadDir(legacyTmuxTodosDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(legacyTmuxTodosDir(), name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var list tmuxTodoListFile
		if err := yaml.Unmarshal(data, &list); err != nil {
			continue
		}
		scope := todoScopeGlobal
		scopeID := "global"
		switch {
		case name == "global.yaml":
			scope = todoScopeGlobal
		case strings.HasPrefix(name, "session_") && strings.HasSuffix(name, ".yaml"):
			scope = todoScopeSession
			id := strings.TrimSuffix(strings.TrimPrefix(name, "session_"), ".yaml")
			if strings.HasPrefix(id, "_") {
				scopeID = "$" + strings.TrimPrefix(id, "_")
			} else {
				scopeID = id
			}
		case strings.HasPrefix(name, "window_") && strings.HasSuffix(name, ".yaml"):
			scope = todoScopeWindow
			id := strings.TrimSuffix(strings.TrimPrefix(name, "window_"), ".yaml")
			if strings.HasPrefix(id, "_") {
				scopeID = "@" + strings.TrimPrefix(id, "_")
			} else {
				scopeID = id
			}
		default:
			continue
		}
		for _, item := range list.Todos {
			appendUniqueTodo(store, scope, scopeID, item)
		}
	}
	return nil
}

func collectAllTmuxTodos(currentSessionID, currentWindowID string) []tmuxTodoEntry {
	store, err := loadTmuxTodoStore()
	if err != nil {
		return nil
	}
	entries := make([]tmuxTodoEntry, 0, len(store.Global))
	for idx, item := range store.Global {
		entries = append(entries, tmuxTodoEntry{
			Title:     item.Title,
			Done:      item.Done,
			Priority:  item.Priority,
			Scope:     todoScopeGlobal,
			ScopeID:   "global",
			ScopeName: "Global",
			IsCurrent: true,
			ItemIndex: idx,
		})
	}
	sessionIDs := make([]string, 0, len(store.Sessions))
	for id := range store.Sessions {
		sessionIDs = append(sessionIDs, id)
	}
	sort.Strings(sessionIDs)
	for _, id := range sessionIDs {
		for idx, item := range store.Sessions[id] {
			entries = append(entries, tmuxTodoEntry{
				Title:     item.Title,
				Done:      item.Done,
				Priority:  item.Priority,
				Scope:     todoScopeSession,
				ScopeID:   id,
				ScopeName: "Session",
				IsCurrent: strings.TrimSpace(id) == strings.TrimSpace(currentSessionID),
				ItemIndex: idx,
			})
		}
	}
	windowIDs := make([]string, 0, len(store.Windows))
	for id := range store.Windows {
		windowIDs = append(windowIDs, id)
	}
	sort.Strings(windowIDs)
	for _, id := range windowIDs {
		for idx, item := range store.Windows[id] {
			entries = append(entries, tmuxTodoEntry{
				Title:     item.Title,
				Done:      item.Done,
				Priority:  item.Priority,
				Scope:     todoScopeWindow,
				ScopeID:   id,
				ScopeName: "Window",
				IsCurrent: strings.TrimSpace(id) == strings.TrimSpace(currentWindowID),
				ItemIndex: idx,
			})
		}
	}
	return entries
}

func sortTmuxTodosByScope(entries []tmuxTodoEntry, scopePriority todoScope) {
	sort.SliceStable(entries, func(i, j int) bool {
		ei, ej := entries[i], entries[j]
		if ei.IsCurrent != ej.IsCurrent {
			return ei.IsCurrent
		}
		if ei.Scope != ej.Scope {
			if ei.Scope == scopePriority {
				return true
			}
			if ej.Scope == scopePriority {
				return false
			}
			return ei.Scope < ej.Scope
		}
		return false
	})
}

func addTmuxTodo(scope todoScope, scopeID, title string) error {
	store, err := loadTmuxTodoStore()
	if err != nil {
		return err
	}
	items := append([]tmuxTodoItem(nil), todoItemsForScope(store, scope, scopeID)...)
	items = append(items, tmuxTodoItem{Title: strings.TrimSpace(title), Done: false, Priority: 2, CreatedAt: time.Now()})
	setTodoItemsForScope(store, scope, scopeID, items)
	return saveTmuxTodoStore(store)
}

func setTmuxTodoPriorityByIndex(scope todoScope, scopeID string, index int, priority int) error {
	store, err := loadTmuxTodoStore()
	if err != nil {
		return err
	}
	items := append([]tmuxTodoItem(nil), todoItemsForScope(store, scope, scopeID)...)
	if index < 0 || index >= len(items) {
		return fmt.Errorf("index out of range")
	}
	items[index].Priority = normalizeTodoPriority(priority)
	setTodoItemsForScope(store, scope, scopeID, items)
	return saveTmuxTodoStore(store)
}

func updateTmuxTodoTitleByIndex(scope todoScope, scopeID string, index int, title string) error {
	store, err := loadTmuxTodoStore()
	if err != nil {
		return err
	}
	items := append([]tmuxTodoItem(nil), todoItemsForScope(store, scope, scopeID)...)
	if index < 0 || index >= len(items) {
		return fmt.Errorf("index out of range")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("todo title is required")
	}
	items[index].Title = title
	setTodoItemsForScope(store, scope, scopeID, items)
	return saveTmuxTodoStore(store)
}

func moveTmuxTodoByIndex(scope todoScope, scopeID string, fromIndex, toIndex int) error {
	store, err := loadTmuxTodoStore()
	if err != nil {
		return err
	}
	items := append([]tmuxTodoItem(nil), todoItemsForScope(store, scope, scopeID)...)
	if fromIndex < 0 || fromIndex >= len(items) || toIndex < 0 || toIndex >= len(items) {
		return fmt.Errorf("index out of range")
	}
	if fromIndex == toIndex {
		return nil
	}
	item := items[fromIndex]
	if fromIndex < toIndex {
		copy(items[fromIndex:toIndex], items[fromIndex+1:toIndex+1])
	} else {
		copy(items[toIndex+1:fromIndex+1], items[toIndex:fromIndex])
	}
	items[toIndex] = item
	setTodoItemsForScope(store, scope, scopeID, items)
	return saveTmuxTodoStore(store)
}

func countOpenTmuxTodos(scope todoScope, scopeID string) (int, error) {
	store, err := loadTmuxTodoStore()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range todoItemsForScope(store, scope, scopeID) {
		if !item.Done {
			count++
		}
	}
	return count, nil
}

func getCurrentTmuxScopeInfo() (sessionID, windowID string) {
	sessionID, _ = runTmuxOutput("display-message", "-p", "#{session_id}")
	windowID, _ = runTmuxOutput("display-message", "-p", "#{window_id}")
	return strings.TrimSpace(sessionID), strings.TrimSpace(windowID)
}

func toggleTmuxTodoByIndex(scope todoScope, scopeID string, index int) error {
	store, err := loadTmuxTodoStore()
	if err != nil {
		return err
	}
	items := append([]tmuxTodoItem(nil), todoItemsForScope(store, scope, scopeID)...)
	if index < 0 || index >= len(items) {
		return fmt.Errorf("index out of range")
	}
	items[index].Done = !items[index].Done
	setTodoItemsForScope(store, scope, scopeID, items)
	return saveTmuxTodoStore(store)
}

func deleteTmuxTodoByIndex(scope todoScope, scopeID string, index int) error {
	store, err := loadTmuxTodoStore()
	if err != nil {
		return err
	}
	items := append([]tmuxTodoItem(nil), todoItemsForScope(store, scope, scopeID)...)
	if index < 0 || index >= len(items) {
		return fmt.Errorf("index out of range")
	}
	items = append(items[:index], items[index+1:]...)
	setTodoItemsForScope(store, scope, scopeID, items)
	return saveTmuxTodoStore(store)
}
