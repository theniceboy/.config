package main

import (
	"strings"
)

// collectDrafts returns windows that have tracker activity but no thread.
// Only windows that still exist in tmux are included.
func collectDrafts() (map[string]*draftInfo, error) {
	drafts := map[string]*draftInfo{}
	store, err := loadGoalStore()
	if err != nil {
		return drafts, err
	}
	env, err := trackerLoadState("")
	if err != nil || env == nil {
		return drafts, nil
	}
	existing := collectWindowInfo()
	claimed := map[string]bool{}
	for _, t := range store.Threads {
		if wid := strings.TrimSpace(t.WindowID); wid != "" {
			claimed[wid] = true
		}
	}
	for _, task := range env.Tasks {
		wid := strings.TrimSpace(task.WindowID)
		if wid == "" || claimed[wid] {
			continue
		}
		if _, ok := existing[wid]; !ok {
			continue
		}
		running := task.Status == trackerTaskStatusInProgress
		summary := firstLine(strings.TrimSpace(task.Summary))
		existingDraft := drafts[wid]
		if existingDraft == nil {
			drafts[wid] = &draftInfo{
				WindowID:    wid,
				LastUpdate:  summary,
				SessionName: strings.TrimSpace(task.Session),
				WindowName:  strings.TrimSpace(task.Window),
				Running:     running,
			}
			existingDraft = drafts[wid]
		} else {
			if running {
				existingDraft.Running = true
			}
			if summary != "" && existingDraft.LastUpdate == "" {
				existingDraft.LastUpdate = summary
			}
		}
		for _, ts := range []string{task.StartedAt, task.CompletedAt} {
			if parsed, ok := trackerParseTimestamp(ts); ok && parsed.After(existingDraft.LastActive) {
				existingDraft.LastActive = parsed
			}
		}
	}
	return drafts, nil
}

// reapOrphanedThreads deletes threads bound to windows that no longer exist.
// Planned (window-less) threads are preserved. Safe to call after tmux-resurrect
// has re-pointed threads to new window ids via the restore migration.
func reapOrphanedThreads() {
	store, err := loadGoalStore()
	if err != nil || store == nil {
		return
	}
	existing := collectWindowInfo()
	changed := false
	for id, t := range store.Threads {
		wid := strings.TrimSpace(t.WindowID)
		if wid == "" {
			continue
		}
		if _, ok := existing[wid]; ok {
			continue
		}
		delete(store.Threads, id)
		for _, other := range store.Threads {
			if containsString(other.BlockedBy, id) {
				other.BlockedBy = removeString(other.BlockedBy, id)
			}
		}
		_ = removeWindowTodos(wid)
		changed = true
	}
	if changed {
		_ = saveGoalStore(store)
	}
}

// removeWindowTodos deletes a window's todo list from the todo store.
func removeWindowTodos(windowID string) error {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return nil
	}
	store, err := loadTmuxTodoStore()
	if err != nil {
		return err
	}
	if store.Windows == nil {
		return nil
	}
	if _, ok := store.Windows[windowID]; !ok {
		return nil
	}
	delete(store.Windows, windowID)
	return saveTmuxTodoStore(store)
}

type taskInfo struct {
	phase      string
	unread     bool
	lastUpdate string
}

func taskInfoByWindow() map[string]taskInfo {
	out := map[string]taskInfo{}
	env, err := trackerLoadState("")
	if err != nil || env == nil {
		return out
	}
	for _, task := range env.Tasks {
		wid := strings.TrimSpace(task.WindowID)
		if wid == "" {
			continue
		}
		info := taskInfo{
			phase:      strings.TrimSpace(task.Phase),
			unread:     task.Status == "completed" && !task.Acknowledged,
			lastUpdate: firstLine(strings.TrimSpace(task.Summary)),
		}
		out[wid] = info
	}
	return out
}

// runningWindows returns the set of window ids with an in-progress tracker task.
func runningWindows() map[string]bool {
	out := map[string]bool{}
	env, err := trackerLoadState("")
	if err != nil || env == nil {
		return out
	}
	for _, task := range env.Tasks {
		if task.Status == trackerTaskStatusInProgress {
			if wid := strings.TrimSpace(task.WindowID); wid != "" {
				out[wid] = true
			}
		}
	}
	return out
}

type windowInfo struct {
	sessionName string
	windowName  string
}

// collectWindowInfo returns live session/window names for every tmux window.
func collectWindowInfo() map[string]windowInfo {
	out := map[string]windowInfo{}
	lines, err := runTmuxOutput("list-windows", "-a", "-F",
		"#{window_id}\t#{session_name}\t#{window_name}")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(lines, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		wid := strings.TrimSpace(parts[0])
		if wid == "" {
			continue
		}
		out[wid] = windowInfo{
			sessionName: strings.TrimSpace(parts[1]),
			windowName:  strings.TrimSpace(parts[2]),
		}
	}
	return out
}

// buildDisplayList constructs the enriched flat list the panel renders.
// expanded maps thread ids whose todos should be shown inline.
func buildDisplayList(expanded map[string]bool) (*flatList, error) {
	store, err := loadGoalStore()
	if err != nil {
		return nil, err
	}
	drafts, _ := collectDrafts()
	running := runningWindows()
	windows := collectWindowInfo()
	todoStore, _ := loadTmuxTodoStore()
	taskByWindow := taskInfoByWindow()
	list := buildGoalFlatList(store, drafts, expanded)
	for _, n := range list.Nodes {
		switch n.Kind {
		case nodeGoal:
		case nodeThread:
			n.Status = computeThreadStatus(store, n.Thread, running)
			n.SessionName, n.WindowName = windowLabel(windows, n.Thread.WindowID)
			n.OpenTodos, n.TotalTodos = windowTodoCounts(todoStore, n.Thread.WindowID)
			n.HasTodos = n.TotalTodos > 0
			if info, ok := taskByWindow[strings.TrimSpace(n.Thread.WindowID)]; ok {
				n.Phase = info.phase
				n.Unread = info.unread
				n.LastUpdate = info.lastUpdate
			}
		case nodeDraft:
			if n.Draft.Running {
				n.Status = threadStatusRunning
			} else {
				n.Status = threadStatusReady
			}
			n.SessionName = n.Draft.SessionName
			n.WindowName = n.Draft.WindowName
			n.LastUpdate = n.Draft.LastUpdate
			n.OpenTodos, n.TotalTodos = windowTodoCounts(todoStore, n.Draft.WindowID)
			n.HasTodos = n.TotalTodos > 0
			if info, ok := taskByWindow[strings.TrimSpace(n.Draft.WindowID)]; ok {
				n.Phase = info.phase
				n.Unread = info.unread
				if info.lastUpdate != "" {
					n.LastUpdate = info.lastUpdate
				}
			}
		}
	}
	return spliceExpandedTodos(list, todoStore, expanded), nil
}

// spliceExpandedTodos inserts todo nodes under expanded threads/drafts.
func spliceExpandedTodos(list *flatList, store *tmuxTodoStore, expanded map[string]bool) *flatList {
	if store == nil {
		return list
	}
	result := &flatList{}
	for _, n := range list.Nodes {
		result.Nodes = append(result.Nodes, n)
		var windowID string
		switch n.Kind {
		case nodeThread:
			windowID = n.Thread.WindowID
		case nodeDraft:
			windowID = n.Draft.WindowID
		default:
			continue
		}
		var key string
		if n.Kind == nodeThread {
			key = n.Thread.ID
		} else {
			key = "draft:" + windowID
		}
		if !expanded[key] {
			continue
		}
		items := store.Windows[strings.TrimSpace(windowID)]
		for idx, item := range items {
			result.Nodes = append(result.Nodes, &goalNode{
				Kind:       nodeTodo,
				Depth:      n.Depth + 1,
				Todo:       &item,
				TodoWindow: windowID,
				TodoIndex:  idx,
			})
		}
	}
	return result
}

func windowLabel(windows map[string]windowInfo, windowID string) (string, string) {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		return "", ""
	}
	info, ok := windows[windowID]
	if !ok {
		return "", ""
	}
	return info.sessionName, info.windowName
}

func windowTodoCounts(store *tmuxTodoStore, windowID string) (open, total int) {
	windowID = strings.TrimSpace(windowID)
	if windowID == "" || store == nil {
		return 0, 0
	}
	for _, item := range store.Windows[windowID] {
		total++
		if !item.Done {
			open++
		}
	}
	return open, total
}
