package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const goalStoreVersion = 1

type Goal struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	ParentID  string    `json:"parent_id,omitempty"`
	Order     int       `json:"order,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type Thread struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	GoalID     string    `json:"goal_id,omitempty"`
	Order      int       `json:"order,omitempty"`
	BlockedBy  []string  `json:"blocked_by,omitempty"`
	WindowID   string    `json:"window_id,omitempty"`
	Done       bool      `json:"done,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

type GoalStore struct {
	Version int                `json:"version"`
	Goals   map[string]*Goal   `json:"goals"`
	Threads map[string]*Thread `json:"threads"`
}

func goalsStorePath() string {
	return filepath.Join(os.Getenv("HOME"), ".cache", "agent", "goals.json")
}

func loadGoalStore() (*GoalStore, error) {
	path := goalsStorePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return normalizeGoalStore(&GoalStore{}), nil
		}
		return nil, err
	}
	var store GoalStore
	if err := jsonUnmarshalGoalStore(data, &store); err != nil {
		return nil, err
	}
	return normalizeGoalStore(&store), nil
}

func jsonUnmarshalGoalStore(data []byte, store *GoalStore) error {
	return json.Unmarshal(data, store)
}

func normalizeGoalStore(store *GoalStore) *GoalStore {
	if store == nil {
		store = &GoalStore{}
	}
	if store.Version == 0 {
		store.Version = goalStoreVersion
	}
	if store.Goals == nil {
		store.Goals = map[string]*Goal{}
	}
	if store.Threads == nil {
		store.Threads = map[string]*Thread{}
	}
	return store
}

func saveGoalStore(store *GoalStore) error {
	store = normalizeGoalStore(store)
	path := goalsStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp.%d.%d", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ── node kinds for the display tree ─────────────────────────

type nodeKind int

const (
	nodeGoal nodeKind = iota
	nodeThread
	nodeDraft
	nodeTodo
)

type goalNode struct {
	Kind        nodeKind
	Depth       int
	Goal        *Goal
	Thread      *Thread
	Draft       *draftInfo
	Todo        *tmuxTodoItem
	TodoWindow  string
	TodoIndex   int
	HasTodos    bool
	OpenTodos   int
	TotalTodos  int
	Status      string
	Phase       string
	Unread      bool
	LastUpdate  string
	SessionName string
	WindowName  string
}

type draftInfo struct {
	WindowID    string
	Name        string
	LastUpdate  string
	SessionName string
	WindowName  string
	Running     bool
	LastActive  time.Time
}

// flatList is the ordered, depth-annotated view of goals + threads.
type flatList struct {
	Nodes []*goalNode
}

func (f *flatList) indexOfThread(threadID string) int {
	for i, n := range f.Nodes {
		if n.Kind == nodeThread && n.Thread != nil && n.Thread.ID == threadID {
			return i
		}
	}
	return -1
}

func (f *flatList) indexOfGoal(goalID string) int {
	for i, n := range f.Nodes {
		if n.Kind == nodeGoal && n.Goal != nil && n.Goal.ID == goalID {
			return i
		}
	}
	return -1
}

// buildGoalFlatList constructs the display tree from the store, merging drafts.
// draftsByWindow maps window_id -> draft info (windows with tracker activity but no thread).
func buildGoalFlatList(store *GoalStore, draftsByWindow map[string]*draftInfo, expanded map[string]bool) *flatList {
	list := &flatList{}
	appendChildren(store, "", 0, list, draftsByWindow, expanded)
	return list
}

func appendChildren(store *GoalStore, parentID string, depth int, list *flatList, drafts map[string]*draftInfo, expanded map[string]bool) {
	type child struct {
		id     string
		kind   nodeKind
		order  int
		goal   *Goal
		thread *Thread
		draft  *draftInfo
	}
	var children []child
	for _, g := range store.Goals {
		if strings.TrimSpace(g.ParentID) == strings.TrimSpace(parentID) {
			children = append(children, child{id: g.ID, kind: nodeGoal, order: g.Order, goal: g})
		}
	}
	for _, t := range store.Threads {
		if strings.TrimSpace(t.GoalID) == strings.TrimSpace(parentID) {
			children = append(children, child{id: t.ID, kind: nodeThread, order: t.Order, thread: t})
		}
	}
	if parentID == "" {
		for wid, d := range drafts {
			children = append(children, child{id: "draft:" + wid, kind: nodeDraft, order: 1 << 30, draft: d})
		}
	}
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].order != children[j].order {
			return children[i].order < children[j].order
		}
		if children[i].kind == nodeDraft && children[j].kind == nodeDraft {
			di, dj := children[i].draft.LastActive, children[j].draft.LastActive
			if !di.Equal(dj) {
				return di.After(dj)
			}
			si := children[i].draft.SessionName
			sj := children[j].draft.SessionName
			if si != sj {
				return si < sj
			}
		}
		return children[i].id < children[j].id
	})
	for _, c := range children {
		switch c.kind {
		case nodeGoal:
			list.Nodes = append(list.Nodes, &goalNode{Kind: nodeGoal, Depth: depth, Goal: c.goal})
			if expanded == nil {
				appendChildren(store, c.goal.ID, depth+1, list, nil, expanded)
			} else if v, ok := expanded[c.goal.ID]; ok && !v {
				// collapsed
			} else {
				appendChildren(store, c.goal.ID, depth+1, list, nil, expanded)
			}
		case nodeThread:
			list.Nodes = append(list.Nodes, &goalNode{Kind: nodeThread, Depth: depth, Thread: c.thread})
		case nodeDraft:
			list.Nodes = append(list.Nodes, &goalNode{Kind: nodeDraft, Depth: depth, Draft: c.draft})
		}
	}
}

// ── status computation ──────────────────────────────────────

const (
	threadStatusRunning  = "running"
	threadStatusReady    = "ready"
	threadStatusBlocked  = "blocked"
	threadStatusDone     = "done"
	threadStatusPlanned  = "planned"
	threadStatusIdle     = "idle"
)

func computeThreadStatus(store *GoalStore, t *Thread, runningWindows map[string]bool) string {
	if t.Done {
		return threadStatusDone
	}
	if strings.TrimSpace(t.WindowID) == "" {
		if hasUnfinishedBlocker(store, t) {
			return threadStatusBlocked
		}
		return threadStatusPlanned
	}
	if hasUnfinishedBlocker(store, t) {
		return threadStatusBlocked
	}
	if runningWindows[t.WindowID] {
		return threadStatusRunning
	}
	return threadStatusReady
}

func hasUnfinishedBlocker(store *GoalStore, t *Thread) bool {
	for _, bid := range t.BlockedBy {
		b := store.Threads[strings.TrimSpace(bid)]
		if b == nil || !b.Done {
			return true
		}
	}
	return false
}

// blockerNames returns display names for a thread's blockers.
func blockerNames(store *GoalStore, t *Thread) []string {
	var names []string
	for _, bid := range t.BlockedBy {
		b := store.Threads[strings.TrimSpace(bid)]
		if b == nil {
			continue
		}
		if b.Done {
			continue
		}
		names = append(names, threadDisplayName(b, ""))
	}
	return names
}

func threadDisplayName(t *Thread, fallback string) string {
	name := firstLine(strings.TrimSpace(t.Name))
	if name != "" {
		return name
	}
	if fallback != "" {
		return fallback
	}
	return "unnamed"
}

func firstLine(s string) string {
	if idx := strings.IndexAny(s, "\n\r"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}

func draftDisplayName(d *draftInfo) string {
	name := strings.TrimSpace(d.Name)
	if name != "" {
		return name
	}
	if w := strings.TrimSpace(d.WindowName); w != "" {
		return w
	}
	return strings.TrimSpace(d.SessionName)
}

// ── mutations ───────────────────────────────────────────────

func newThreadID() string {
	return "th_" + randomToken(8)
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func newGoalID(seed string) string {
	base := sanitizeFeatureName(seed)
	if base == "" {
		base = "goal"
	}
	id := base
	if _, exists := loadGoalStoreID(id); exists {
		id = base + "-" + randomToken(3)
	}
	return id
}

func loadGoalStoreID(id string) (*GoalStore, bool) {
	store, err := loadGoalStore()
	if err != nil {
		return nil, false
	}
	if _, ok := store.Goals[id]; ok {
		return store, true
	}
	if _, ok := store.Threads[id]; ok {
		return store, true
	}
	return store, false
}

func mutateGoalStore(fn func(store *GoalStore) error) error {
	store, err := loadGoalStore()
	if err != nil {
		return err
	}
	if err := fn(store); err != nil {
		return err
	}
	return saveGoalStore(store)
}

func addGoal(title, parentID string) (*Goal, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("goal title is required")
	}
	var created *Goal
	err := mutateGoalStore(func(store *GoalStore) error {
		parentID = strings.TrimSpace(parentID)
		if parentID != "" {
			if _, ok := store.Goals[parentID]; !ok {
				return fmt.Errorf("parent goal not found: %s", parentID)
			}
		}
		id := newGoalID(title)
		g := &Goal{
			ID:        id,
			Title:     title,
			ParentID:  parentID,
			Order:     nextChildOrder(store, parentID),
			CreatedAt: time.Now(),
		}
		store.Goals[id] = g
		created = g
		return nil
	})
	return created, err
}

func nextChildOrder(store *GoalStore, parentID string) int {
	max := -1
	for _, g := range store.Goals {
		if strings.TrimSpace(g.ParentID) == strings.TrimSpace(parentID) && g.Order > max {
			max = g.Order
		}
	}
	for _, t := range store.Threads {
		if strings.TrimSpace(t.GoalID) == strings.TrimSpace(parentID) && t.Order > max {
			max = t.Order
		}
	}
	return max + 1
}

// deleteGoalCascade deletes the goal, its descendant goals, and unassigns
// (does not delete) all threads under the subtree.
func deleteGoalCascade(goalID string) error {
	goalID = strings.TrimSpace(goalID)
	return mutateGoalStore(func(store *GoalStore) error {
		if _, ok := store.Goals[goalID]; !ok {
			return fmt.Errorf("goal not found: %s", goalID)
		}
		toDelete := map[string]bool{goalID: true}
		changed := true
		for changed {
			changed = false
			for _, g := range store.Goals {
				if toDelete[g.ParentID] && !toDelete[g.ID] {
					toDelete[g.ID] = true
					changed = true
				}
			}
		}
		// unassign threads whose goal is being deleted
		for _, t := range store.Threads {
			if toDelete[t.GoalID] {
				t.GoalID = ""
				t.UpdatedAt = time.Now()
			}
		}
		for id := range toDelete {
			delete(store.Goals, id)
		}
		return nil
	})
}

func addManualThread(name, goalID string, blockedBy []string) (*Thread, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("thread name is required")
	}
	var created *Thread
	err := mutateGoalStore(func(store *GoalStore) error {
		goalID = strings.TrimSpace(goalID)
		if goalID != "" {
			if _, ok := store.Goals[goalID]; !ok {
				return fmt.Errorf("goal not found: %s", goalID)
			}
		}
		t := &Thread{
			ID:        newThreadID(),
			Name:      name,
			GoalID:    goalID,
			Order:     nextChildOrder(store, goalID),
			BlockedBy: cleanStringList(blockedBy),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		store.Threads[t.ID] = t
		created = t
		return nil
	})
	return created, err
}

func renameThread(threadID, name string) error {
	threadID = strings.TrimSpace(threadID)
	name = strings.TrimSpace(name)
	return mutateGoalStore(func(store *GoalStore) error {
		t, ok := store.Threads[threadID]
		if !ok {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		if name == "" {
			return fmt.Errorf("thread name is required")
		}
		t.Name = name
		t.UpdatedAt = time.Now()
		return nil
	})
}

func setThreadGoal(threadID, goalID string) error {
	threadID = strings.TrimSpace(threadID)
	goalID = strings.TrimSpace(goalID)
	return mutateGoalStore(func(store *GoalStore) error {
		t, ok := store.Threads[threadID]
		if !ok {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		if goalID != "" {
			if _, ok := store.Goals[goalID]; !ok {
				return fmt.Errorf("goal not found: %s", goalID)
			}
			if wouldCreateGoalCycle(store, goalID, threadID) {
				return fmt.Errorf("cannot move goal under its own subtree")
			}
		}
		t.GoalID = goalID
		t.Order = nextChildOrder(store, goalID)
		t.UpdatedAt = time.Now()
		return nil
	})
}

func wouldCreateGoalCycle(store *GoalStore, goalID, threadID string) bool {
	// threads can't be parents of goals, so cycles are impossible via thread move.
	return false
}

func setThreadBlocker(threadID string, blockedBy []string) error {
	threadID = strings.TrimSpace(threadID)
	return mutateGoalStore(func(store *GoalStore) error {
		t, ok := store.Threads[threadID]
		if !ok {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		cleaned := []string{}
		for _, b := range blockedBy {
			b = strings.TrimSpace(b)
			if b == "" || b == threadID {
				continue
			}
			if _, ok := store.Threads[b]; !ok {
				return fmt.Errorf("blocker thread not found: %s", b)
			}
			cleaned = append(cleaned, b)
		}
		t.BlockedBy = cleaned
		t.UpdatedAt = time.Now()
		return nil
	})
}

func setThreadDone(threadID string, done bool) error {
	threadID = strings.TrimSpace(threadID)
	return mutateGoalStore(func(store *GoalStore) error {
		t, ok := store.Threads[threadID]
		if !ok {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		t.Done = done
		t.UpdatedAt = time.Now()
		return nil
	})
}

func deleteThread(threadID string) error {
	threadID = strings.TrimSpace(threadID)
	return mutateGoalStore(func(store *GoalStore) error {
		if _, ok := store.Threads[threadID]; !ok {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		delete(store.Threads, threadID)
		for _, t := range store.Threads {
			if containsString(t.BlockedBy, threadID) {
				t.BlockedBy = removeString(t.BlockedBy, threadID)
				t.UpdatedAt = time.Now()
			}
		}
		return nil
	})
}

// bindThreadWindow attaches a window to a (usually planned) thread.
func bindThreadWindow(threadID, windowID string) error {
	threadID = strings.TrimSpace(threadID)
	windowID = strings.TrimSpace(windowID)
	return mutateGoalStore(func(store *GoalStore) error {
		t, ok := store.Threads[threadID]
		if !ok {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		// detach any other thread pointing at this window
		for _, other := range store.Threads {
			if other.ID != threadID && strings.TrimSpace(other.WindowID) == windowID {
				other.WindowID = ""
				other.UpdatedAt = time.Now()
			}
		}
		t.WindowID = windowID
		t.Done = false
		t.UpdatedAt = time.Now()
		return nil
	})
}

func unbindThreadWindow(threadID string) error {
	threadID = strings.TrimSpace(threadID)
	return mutateGoalStore(func(store *GoalStore) error {
		t, ok := store.Threads[threadID]
		if !ok {
			return fmt.Errorf("thread not found: %s", threadID)
		}
		t.WindowID = ""
		t.UpdatedAt = time.Now()
		return nil
	})
}

// mergeThreads moves source's window + todos into target, then deletes source.
func mergeThreads(sourceID, targetID string) error {
	sourceID = strings.TrimSpace(sourceID)
	targetID = strings.TrimSpace(targetID)
	if sourceID == targetID {
		return fmt.Errorf("cannot merge a thread into itself")
	}
	return mutateGoalStore(func(store *GoalStore) error {
		src, ok := store.Threads[sourceID]
		if !ok {
			return fmt.Errorf("thread not found: %s", sourceID)
		}
		tgt, ok := store.Threads[targetID]
		if !ok {
			return fmt.Errorf("thread not found: %s", targetID)
		}
		if strings.TrimSpace(src.WindowID) != "" && strings.TrimSpace(tgt.WindowID) == "" {
			tgt.WindowID = src.WindowID
		}
		tgt.Done = tgt.Done && src.Done
		// re-point blockers
		for _, t := range store.Threads {
			if t.ID == sourceID {
				continue
			}
			if containsString(t.BlockedBy, sourceID) {
				t.BlockedBy = removeString(t.BlockedBy, sourceID)
				if !containsString(t.BlockedBy, targetID) {
					t.BlockedBy = append(t.BlockedBy, targetID)
				}
				t.UpdatedAt = time.Now()
			}
		}
		delete(store.Threads, sourceID)
		tgt.UpdatedAt = time.Now()
		return nil
	})
}

// findThreadByWindow returns the explicit thread bound to a window, if any.
func findThreadByWindow(store *GoalStore, windowID string) *Thread {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return nil
	}
	for _, t := range store.Threads {
		if strings.TrimSpace(t.WindowID) == windowID {
			return t
		}
	}
	return nil
}

// promoteDraftToThread creates a new thread for a window (draft) or binds to an existing planned thread.
func promoteDraftToThread(windowID, name, goalID string, bindExistingThreadID string) (*Thread, error) {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return nil, fmt.Errorf("window id is required")
	}
	if strings.TrimSpace(bindExistingThreadID) != "" {
		if err := bindThreadWindow(bindExistingThreadID, windowID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(name) != "" {
			_ = renameThread(bindExistingThreadID, name)
		}
		store, _ := loadGoalStore()
		return store.Threads[bindExistingThreadID], nil
	}
	var created *Thread
	err := mutateGoalStore(func(store *GoalStore) error {
		// detach any existing thread on this window
		for _, other := range store.Threads {
			if strings.TrimSpace(other.WindowID) == windowID {
				other.WindowID = ""
			}
		}
		goalID = strings.TrimSpace(goalID)
		if goalID != "" {
			if _, ok := store.Goals[goalID]; !ok {
				return fmt.Errorf("goal not found: %s", goalID)
			}
		}
		t := &Thread{
			ID:        newThreadID(),
			Name:      strings.TrimSpace(name),
			GoalID:    goalID,
			Order:     nextChildOrder(store, goalID),
			WindowID:  windowID,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		store.Threads[t.ID] = t
		created = t
		return nil
	})
	return created, err
}

// reviveDoneThreadForWindow: when a new turn starts in a window whose thread is done,
// un-done it and rename it.
func reviveDoneThreadForWindow(windowID, newName string) error {
	windowID = strings.TrimSpace(windowID)
	return mutateGoalStore(func(store *GoalStore) error {
		t := findThreadByWindow(store, windowID)
		if t == nil {
			return nil
		}
		t.Done = false
		if strings.TrimSpace(newName) != "" {
			t.Name = strings.TrimSpace(newName)
		}
		t.UpdatedAt = time.Now()
		return nil
	})
}

// ── move-mode tree operations ───────────────────────────────
//
// The ghost lives at a position in the flat list with a depth. Moving it
// follows the validated semantics:
//   - toward a goal neighbor (same depth) -> nest as its first child (depth+1)
//   - toward a thread neighbor (same depth) -> swap positions, keep depth
//   - toward a shallower neighbor (edge of parent) -> pop out (depth-1, same slot)
//   - at root depth, pop out is blocked

type ghostState struct {
	ThreadID string
	GoalID   string
	IsGoal   bool
	Pos      int
	Depth    int
	IsDraft  bool
	After    bool
	Before   bool
}

// moveGhostDown moves the ghost one step down. Returns new state (unchanged if blocked).
func moveGhostDown(list *flatList, g ghostState) ghostState {
	if g.Pos >= len(list.Nodes)-1 {
		if g.Depth > 0 {
			return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: g.Pos, Depth: g.Depth - 1}
		}
		return g
	}
	below := list.Nodes[g.Pos+1]
	if below.Depth > g.Depth {
		next := skipSubtreeForward(list, g.Pos+1, g.Depth+1)
		return applyDownTarget(list, g, next)
	}
	return applyDownTarget(list, g, g.Pos+1)
}

func applyDownTarget(list *flatList, g ghostState, targetIdx int) ghostState {
	if targetIdx >= len(list.Nodes) {
		if g.Depth > 0 {
			return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: g.Pos, Depth: g.Depth - 1}
		}
		return g
	}
	below := list.Nodes[targetIdx]
	if below.Depth < g.Depth {
		return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: g.Pos, Depth: g.Depth - 1, After: true}
	}
	if below.Depth == g.Depth {
		if below.Kind == nodeGoal {
			return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: targetIdx, Depth: g.Depth + 1}
		}
		return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: targetIdx, Depth: g.Depth, After: true}
	}
	// deeper (shouldn't normally reach here) -> treat as swap
	return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: targetIdx, Depth: g.Depth}
}

func moveGhostUp(list *flatList, g ghostState) ghostState {
	if g.Pos <= 0 {
		if g.Depth > 0 {
			return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: g.Pos, Depth: g.Depth - 1}
		}
		return g
	}
	above := list.Nodes[g.Pos-1]
	if above.Depth > g.Depth {
		prev := skipSubtreeBackward(list, g.Pos-1, g.Depth+1)
		return applyUpTarget(list, g, prev)
	}
	return applyUpTarget(list, g, g.Pos-1)
}

func applyUpTarget(list *flatList, g ghostState, targetIdx int) ghostState {
	if targetIdx < 0 {
		if g.Depth > 0 {
			return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: g.Pos, Depth: g.Depth - 1}
		}
		return g
	}
	above := list.Nodes[targetIdx]
	if above.Depth < g.Depth {
		return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: targetIdx, Depth: g.Depth - 1}
	}
	if above.Depth == g.Depth {
		if above.Kind == nodeGoal {
			subtreeEnd := skipSubtreeForward(list, targetIdx+1, g.Depth+1)
			lastInSubtree := subtreeEnd - 1
			if lastInSubtree > targetIdx {
				return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: lastInSubtree, Depth: g.Depth + 1, After: true}
			}
			return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: targetIdx, Depth: g.Depth + 1}
		}
		return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: targetIdx, Depth: g.Depth}
	}
	return ghostState{ThreadID: g.ThreadID, GoalID: g.GoalID, IsGoal: g.IsGoal, IsDraft: g.IsDraft, Pos: targetIdx, Depth: g.Depth}
}

func skipSubtreeForward(list *flatList, start, startDepth int) int {
	i := start
	for i < len(list.Nodes) && list.Nodes[i].Depth >= startDepth {
		i++
	}
	return i
}

func skipSubtreeBackward(list *flatList, start, startDepth int) int {
	i := start
	for i >= 0 && list.Nodes[i].Depth >= startDepth {
		i--
	}
	return i
}

// applyGhostMove commits the ghost's new position+depth to the store.
// It reassigns the thread's GoalID and recomputes sibling orders so the flat
// list (with the ghost at its new slot/depth) is preserved.
type flatEntry struct {
	kind   nodeKind
	depth  int
	goal   *Goal
	thread *Thread
}

func applyGhostMove(threadID string, list *flatList, g ghostState) error {
	threadID = strings.TrimSpace(threadID)
	// Build the desired flat layout: remove the ghost's original node, then
	// re-insert it at (g.Pos, g.Depth). Recompute parent IDs + orders.
	var entries []flatEntry
	// source entries excluding the moved thread
	for _, n := range list.Nodes {
		if n.Kind == nodeThread && n.Thread != nil && n.Thread.ID == threadID {
			continue
		}
		e := flatEntry{kind: n.Kind, depth: n.Depth}
		if n.Kind == nodeGoal {
			e.goal = n.Goal
		} else if n.Kind == nodeThread {
			e.thread = n.Thread
		}
		entries = append(entries, e)
	}
	insertAt := g.Pos
	for i := 0; i < g.Pos && i < len(list.Nodes); i++ {
		n := list.Nodes[i]
		if n.Kind == nodeThread && n.Thread != nil && n.Thread.ID == threadID {
			insertAt--
		}
	}
	if insertAt < 0 {
		insertAt = 0
	}
	if insertAt > len(entries) {
		insertAt = len(entries)
	}
	if g.After {
		for insertAt < len(entries) && entries[insertAt].depth >= g.Depth {
			insertAt++
		}
	} else if insertAt < len(entries) && entries[insertAt].depth < g.Depth {
		insertAt++
	}
	// fetch the moved thread record
	store, err := loadGoalStore()
	if err != nil {
		return err
	}
	moved := store.Threads[threadID]
	if moved == nil {
		return fmt.Errorf("thread not found: %s", threadID)
	}
	for i := range entries {
		if entries[i].kind == nodeGoal && entries[i].goal != nil {
			entries[i].goal = store.Goals[entries[i].goal.ID]
		} else if entries[i].kind == nodeThread && entries[i].thread != nil {
			entries[i].thread = store.Threads[entries[i].thread.ID]
		}
	}
	ghostEntry := flatEntry{kind: nodeThread, depth: g.Depth, thread: moved}
	final := append([]flatEntry{}, entries[:insertAt]...)
	final = append(final, ghostEntry)
	final = append(final, entries[insertAt:]...)

	// recompute parent for each entry: nearest preceding entry at depth-1
	assignParentsAndOrders(store, final)
	return saveGoalStore(store)
}

func assignParentsAndOrders(store *GoalStore, entries []flatEntry) {
	// track last goal seen at each depth
	lastGoalAt := map[int]string{}
	childCount := map[string]int{}
	increment := func(parentID string) int {
		n := childCount[parentID]
		childCount[parentID] = n + 1
		return n
	}
	for _, e := range entries {
		parentID := ""
		if e.depth > 0 {
			if pid, ok := lastGoalAt[e.depth-1]; ok {
				parentID = pid
			}
		}
		switch e.kind {
		case nodeGoal:
			e.goal.ParentID = parentID
			e.goal.Order = increment(parentID)
			lastGoalAt[e.depth] = e.goal.ID
			// clear deeper remembered goals
			for d := range lastGoalAt {
				if d > e.depth {
					delete(lastGoalAt, d)
				}
			}
		case nodeThread:
			e.thread.GoalID = parentID
			e.thread.Order = increment(parentID)
		}
	}
}

// reorderChildSibling swaps a child (goal or thread) by one slot among its
// siblings under parentID, keeping its parent and depth. Children of a goal
// are sub-goals (ParentID==parentID) and threads (GoalID==parentID) sharing a
// single Order sequence. Returns false at the first/last sibling boundary or
// if the node is not found.
func reorderChildSibling(parentID, nodeID string, isGoal bool, delta int) bool {
	store, err := loadGoalStore()
	if err != nil || store == nil {
		return false
	}
	parentID = strings.TrimSpace(parentID)
	type ch struct {
		id     string
		isGoal bool
		order  int
		goal   *Goal
		thread *Thread
	}
	var children []ch
	for _, g := range store.Goals {
		if strings.TrimSpace(g.ParentID) == parentID {
			children = append(children, ch{g.ID, true, g.Order, g, nil})
		}
	}
	for _, t := range store.Threads {
		if strings.TrimSpace(t.GoalID) == parentID {
			children = append(children, ch{t.ID, false, t.Order, nil, t})
		}
	}
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].order != children[j].order {
			return children[i].order < children[j].order
		}
		return children[i].id < children[j].id
	})
	idx := -1
	for i, c := range children {
		if c.id == nodeID && c.isGoal == isGoal {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	target := idx + delta
	if target < 0 || target >= len(children) {
		return false
	}
	moved := children[idx]
	rest := make([]ch, 0, len(children)-1)
	for i, c := range children {
		if i == idx {
			continue
		}
		rest = append(rest, c)
	}
	ordered := make([]ch, 0, len(children))
	ordered = append(ordered, rest[:target]...)
	ordered = append(ordered, moved)
	ordered = append(ordered, rest[target:]...)
	for i, c := range ordered {
		if c.isGoal && c.goal != nil {
			c.goal.Order = i
		} else if !c.isGoal && c.thread != nil {
			c.thread.Order = i
		}
	}
	return saveGoalStore(store) == nil
}

func applyGoalMove(goalID string, list *flatList, g ghostState) error {
	goalID = strings.TrimSpace(goalID)
	startIdx := -1
	for i, n := range list.Nodes {
		if n.Kind == nodeGoal && n.Goal != nil && n.Goal.ID == goalID {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return fmt.Errorf("goal not found: %s", goalID)
	}
	origDepth := list.Nodes[startIdx].Depth
	endIdx := startIdx + 1
	for endIdx < len(list.Nodes) && list.Nodes[endIdx].Depth > origDepth {
		endIdx++
	}
	depthDelta := g.Depth - origDepth

	store, err := loadGoalStore()
	if err != nil {
		return err
	}

	var entries []flatEntry
	for i := 0; i < len(list.Nodes); i++ {
		if i >= startIdx && i < endIdx {
			continue
		}
		n := list.Nodes[i]
		e := flatEntry{kind: n.Kind, depth: n.Depth}
		if n.Kind == nodeGoal {
			e.goal = store.Goals[n.Goal.ID]
		} else if n.Kind == nodeThread {
			e.thread = store.Threads[n.Thread.ID]
		}
		entries = append(entries, e)
	}

	var subtree []flatEntry
	for i := startIdx; i < endIdx; i++ {
		n := list.Nodes[i]
		e := flatEntry{kind: n.Kind, depth: n.Depth + depthDelta}
		if n.Kind == nodeGoal {
			e.goal = store.Goals[n.Goal.ID]
		} else if n.Kind == nodeThread {
			e.thread = store.Threads[n.Thread.ID]
		}
		subtree = append(subtree, e)
	}

	insertAt := g.Pos
	removed := endIdx - startIdx
	if insertAt >= endIdx {
		insertAt -= removed
	} else if insertAt > startIdx {
		insertAt = startIdx
	}
	if insertAt < 0 {
		insertAt = 0
	}
	if insertAt > len(entries) {
		insertAt = len(entries)
	}
	if g.After {
		for insertAt < len(entries) && entries[insertAt].depth >= g.Depth {
			insertAt++
		}
	} else if insertAt < len(entries) && entries[insertAt].depth < g.Depth {
		insertAt++
	}

	final := append([]flatEntry{}, entries[:insertAt]...)
	final = append(final, subtree...)
	final = append(final, entries[insertAt:]...)

	assignParentsAndOrders(store, final)
	return saveGoalStore(store)
}

// helpers

func cleanStringList(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func removeString(in []string, val string) []string {
	var out []string
	for _, v := range in {
		if v != val {
			out = append(out, v)
		}
	}
	return out
}
