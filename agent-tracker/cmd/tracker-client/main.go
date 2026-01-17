package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/david/agent-tracker/internal/ipc"
	"github.com/gdamore/tcell/v2"
)

const (
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
)

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

const spinnerInterval = 120 * time.Millisecond

type viewMode int

const (
	viewTracker viewMode = iota
	viewNotes
	viewArchive
	viewEdit
)

type noteScope string

const (
	scopeWindow  noteScope = "window"
	scopeSession noteScope = "session"
	scopeAll     noteScope = "all"
)

type listState struct {
	selected int
	offset   int
}

type promptMode string

const (
	promptAddNote  promptMode = "add_note"
	promptEditNote promptMode = "edit_note"
	promptAddGoal  promptMode = "add_goal"
)

type promptState struct {
	active bool
	mode   promptMode
	text   []rune
	cursor int
	noteID string
}

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		if err := runUI(os.Args[1:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "ui":
		if err := runUI(args); err != nil {
			log.Fatal(err)
		}
	case "command":
		if err := runCommand(args); err != nil {
			log.Fatal(err)
		}
	case "state":
		if err := runState(args); err != nil {
			log.Fatal(err)
		}
	default:
		if err := runUI(os.Args[1:]); err != nil {
			log.Fatal(err)
		}
	}
}

func runCommand(args []string) error {
	fs := flag.NewFlagSet("tracker-client command", flag.ExitOnError)
	var client, session, sessionID, window, windowID, pane, summary, scope, noteID string
	fs.StringVar(&client, "client", "", "tmux client tty")
	fs.StringVar(&session, "session", "", "tmux session name")
	fs.StringVar(&sessionID, "session-id", "", "tmux session id")
	fs.StringVar(&window, "window", "", "tmux window name")
	fs.StringVar(&windowID, "window-id", "", "tmux window id")
	fs.StringVar(&pane, "pane", "", "tmux pane id")
	fs.StringVar(&summary, "summary", "", "summary or note payload")
	fs.StringVar(&scope, "scope", "", "note scope")
	fs.StringVar(&noteID, "note-id", "", "note identifier")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("command name required")
	}
	if len(rest) > 1 {
		summary = strings.Join(rest[1:], " ")
	}

	env := ipc.Envelope{
		Kind:      "command",
		Command:   rest[0],
		Client:    client,
		Session:   strings.TrimSpace(session),
		SessionID: strings.TrimSpace(sessionID),
		Window:    strings.TrimSpace(window),
		WindowID:  strings.TrimSpace(windowID),
		Pane:      strings.TrimSpace(pane),
		Scope:     strings.TrimSpace(scope),
		NoteID:    strings.TrimSpace(noteID),
		Summary:   strings.TrimSpace(summary),
	}
	if env.Summary != "" {
		env.Message = env.Summary
	}

	switch env.Command {
	case "start_task", "finish_task", "acknowledge", "note_add", "note_archive_pane", "note_attach":
		ctx, err := resolveContext(env.Session, env.SessionID, env.Window, env.WindowID, env.Pane)
		if err != nil {
			return err
		}
		env.Session = ctx.SessionName
		env.SessionID = ctx.SessionID
		env.Window = ctx.WindowName
		env.WindowID = ctx.WindowID
		env.Pane = ctx.PaneID
	}

	conn, err := net.Dial("unix", socketPath())
	if err != nil {
		return err
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	if err := enc.Encode(&env); err != nil {
		return err
	}

	dec := json.NewDecoder(conn)
	for {
		var reply ipc.Envelope
		if err := dec.Decode(&reply); err != nil {
			return err
		}
		if reply.Kind == "ack" {
			return nil
		}
	}
}

func runState(args []string) error {
	fs := flag.NewFlagSet("tracker-client state", flag.ExitOnError)
	var client string
	fs.StringVar(&client, "client", "", "tmux client tty")
	if err := fs.Parse(args); err != nil {
		return err
	}

	conn, err := net.Dial("unix", socketPath())
	if err != nil {
		return err
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReader(conn))

	if err := enc.Encode(&ipc.Envelope{Kind: "ui-register", Client: client}); err != nil {
		return err
	}

	for {
		var env ipc.Envelope
		if err := dec.Decode(&env); err != nil {
			return err
		}
		if env.Kind == "state" {
			out := json.NewEncoder(os.Stdout)
			out.SetEscapeHTML(false)
			if err := out.Encode(&env); err != nil {
				return err
			}
			return nil
		}
	}
}

type tmuxContext struct {
	SessionName string
	SessionID   string
	WindowName  string
	WindowID    string
	PaneID      string
}

func (c tmuxContext) complete() bool {
	return strings.TrimSpace(c.SessionID) != "" &&
		strings.TrimSpace(c.WindowID) != "" &&
		strings.TrimSpace(c.PaneID) != ""
}

func resolveContext(sessionName, sessionID, windowName, windowID, paneID string) (tmuxContext, error) {
	ctx := tmuxContext{
		SessionName: strings.TrimSpace(sessionName),
		SessionID:   strings.TrimSpace(sessionID),
		WindowName:  strings.TrimSpace(windowName),
		WindowID:    strings.TrimSpace(windowID),
		PaneID:      strings.TrimSpace(paneID),
	}

	fetchOrder := []string{}
	if ctx.PaneID != "" {
		fetchOrder = append(fetchOrder, ctx.PaneID)
	}
	fetchOrder = append(fetchOrder, "")

	for _, target := range fetchOrder {
		if ctx.complete() {
			break
		}
		info, err := detectTmuxContext(target)
		if err != nil {
			if target == "" {
				return tmuxContext{}, err
			}
			continue
		}
		ctx = ctx.merge(info)
	}

	if ctx.SessionID == "" || ctx.WindowID == "" {
		return tmuxContext{}, fmt.Errorf("session and window identifiers required")
	}

	if ctx.SessionName == "" || ctx.WindowName == "" {
		if info, err := detectTmuxContext(ctx.WindowID); err == nil {
			ctx = ctx.merge(info)
		}
	}

	if ctx.SessionName == "" {
		ctx.SessionName = ctx.SessionID
	}
	if ctx.WindowName == "" {
		ctx.WindowName = ctx.WindowID
	}

	return ctx, nil
}

func (c tmuxContext) merge(other tmuxContext) tmuxContext {
	if c.SessionName == "" {
		c.SessionName = other.SessionName
	}
	if c.SessionID == "" {
		c.SessionID = other.SessionID
	}
	if c.WindowName == "" {
		c.WindowName = other.WindowName
	}
	if c.WindowID == "" {
		c.WindowID = other.WindowID
	}
	if c.PaneID == "" {
		c.PaneID = other.PaneID
	}
	return c
}

func detectTmuxContext(target string) (tmuxContext, error) {
	format := "#{session_name}:::#{session_id}:::#{window_name}:::#{window_id}:::#{pane_id}"
	args := []string{"display-message", "-p"}
	if strings.TrimSpace(target) != "" {
		args = append(args, "-t", strings.TrimSpace(target))
	}
	args = append(args, format)
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return tmuxContext{}, fmt.Errorf("tmux display-message: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	parts := strings.Split(strings.TrimSpace(string(output)), ":::")
	if len(parts) != 5 {
		return tmuxContext{}, fmt.Errorf("unexpected tmux response: %s", strings.TrimSpace(string(output)))
	}
	return tmuxContext{
		SessionName: strings.TrimSpace(parts[0]),
		SessionID:   strings.TrimSpace(parts[1]),
		WindowName:  strings.TrimSpace(parts[2]),
		WindowID:    strings.TrimSpace(parts[3]),
		PaneID:      strings.TrimSpace(parts[4]),
	}, nil
}

func detectTmuxContextForClient(client string) (tmuxContext, error) {
	if pane := strings.TrimSpace(os.Getenv("TMUX_PANE")); pane != "" {
		if ctx, err := detectTmuxContext(pane); err == nil {
			return ctx, nil
		}
	}
	format := "#{session_name}:::#{session_id}:::#{window_name}:::#{window_id}:::#{pane_id}"
	args := []string{"display-message", "-p"}
	if strings.TrimSpace(client) != "" {
		args = append(args, "-c", strings.TrimSpace(client))
	}
	args = append(args, format)
	cmd := exec.Command("tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return tmuxContext{}, fmt.Errorf("tmux display-message: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	parts := strings.Split(strings.TrimSpace(string(output)), ":::")
	if len(parts) != 5 {
		return tmuxContext{}, fmt.Errorf("unexpected tmux response: %s", strings.TrimSpace(string(output)))
	}
	return tmuxContext{
		SessionName: strings.TrimSpace(parts[0]),
		SessionID:   strings.TrimSpace(parts[1]),
		WindowName:  strings.TrimSpace(parts[2]),
		WindowID:    strings.TrimSpace(parts[3]),
		PaneID:      strings.TrimSpace(parts[4]),
	}, nil
}

func runUI(args []string) error {
	fs := flag.NewFlagSet("tracker-client ui", flag.ExitOnError)
	var client string
	var originSession, originSessionID, originWindow, originWindowID, originPane string
	fs.StringVar(&client, "client", "", "tmux client tty")
	fs.StringVar(&originSession, "origin-session", "", "origin session name")
	fs.StringVar(&originSessionID, "origin-session-id", "", "origin session id")
	fs.StringVar(&originWindow, "origin-window", "", "origin window name")
	fs.StringVar(&originWindowID, "origin-window-id", "", "origin window id")
	fs.StringVar(&originPane, "origin-pane", "", "origin pane id")
	if err := fs.Parse(args); err != nil {
		return err
	}

	conn, err := net.Dial("unix", socketPath())
	if err != nil {
		return err
	}
	defer conn.Close()

	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.Clear()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(bufio.NewReader(conn))

	if err := enc.Encode(&ipc.Envelope{Kind: "ui-register", Client: client}); err != nil {
		return err
	}

	type state struct {
		message  string
		tasks    []ipc.Task
		notes    []ipc.Note
		archived []ipc.Note
		goals    []ipc.Goal
	}
	st := state{message: "Connecting to tracker…"}

	originCtx := tmuxContext{
		SessionName: strings.TrimSpace(originSession),
		SessionID:   strings.TrimSpace(originSessionID),
		WindowName:  strings.TrimSpace(originWindow),
		WindowID:    strings.TrimSpace(originWindowID),
		PaneID:      strings.TrimSpace(originPane),
	}

	currentCtx := originCtx
	refreshCtx := func() {
		if originCtx.complete() {
			currentCtx = originCtx
			return
		}
		if ctx, err := detectTmuxContextForClient(client); err == nil {
			currentCtx = ctx
		}
	}
	refreshCtx()

	incoming := make(chan ipc.Envelope)
	errCh := make(chan error, 1)
	go func() {
		for {
			var env ipc.Envelope
			if err := dec.Decode(&env); err != nil {
				errCh <- err
				close(incoming)
				return
			}
			incoming <- env
		}
	}()

	events := make(chan tcell.Event)
	go func() {
		for {
			ev := screen.PollEvent()
			if ev == nil {
				close(events)
				return
			}
			events <- ev
		}
	}()

	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()

	var encMu sync.Mutex
	sendCommand := func(name string, opts ...func(*ipc.Envelope)) error {
		encMu.Lock()
		defer encMu.Unlock()
		env := ipc.Envelope{Kind: "command", Command: name, Client: client}
		for _, opt := range opts {
			opt(&env)
		}
		return enc.Encode(&env)
	}

	mode := viewNotes
	scope := scopeWindow
	showCompletedTasks := true
	showCompletedNotes := false
	showCompletedArchive := false
	taskList := listState{}
	noteList := listState{}
	archiveList := listState{}
	goalList := listState{}
	keepTasksVisible := make(map[string]bool)
	keepNotesVisible := make(map[string]bool)
	prompt := promptState{}
	helpVisible := false
	focusGoals := false
	var editNote *ipc.Note

	cycleScope := func(forward bool, wrap bool) {
		order := []noteScope{scopeWindow, scopeSession, scopeAll}
		pos := 0
		for i, s := range order {
			if scope == s {
				pos = i
				break
			}
		}
		if forward {
			if pos < len(order)-1 {
				scope = order[pos+1]
			} else if wrap {
				scope = order[0]
			}
			return
		}
		if pos > 0 {
			scope = order[pos-1]
		} else if wrap {
			scope = order[len(order)-1]
		}
	}

	scopeStyle := func(s noteScope) tcell.Style {
		switch s {
		case scopeWindow:
			return tcell.StyleDefault.Foreground(tcell.ColorLightYellow).Bold(true)
		case scopeSession:
			return tcell.StyleDefault.Foreground(tcell.ColorFuchsia).Bold(true)
		case scopeAll:
			return tcell.StyleDefault.Foreground(tcell.ColorLightGreen).Bold(true)
		default:
			return tcell.StyleDefault.Foreground(tcell.ColorLightYellow).Bold(true)
		}
	}

	noteScopeOf := func(n ipc.Note) noteScope {
		switch strings.ToLower(strings.TrimSpace(n.Scope)) {
		case string(scopeWindow):
			return scopeWindow
		case string(scopeSession):
			return scopeSession
		case string(scopeAll):
			return scopeAll
		}
		switch {
		case strings.TrimSpace(n.WindowID) != "":
			return scopeWindow
		case strings.TrimSpace(n.SessionID) != "":
			return scopeSession
		default:
			return scopeAll
		}
	}

	scopeTag := func(s noteScope) string {
		switch s {
		case scopeWindow:
			return "W"
		case scopeSession:
			return "S"
		case scopeAll:
			return "G"
		default:
			return "?"
		}
	}

	clampList := func(state *listState, length int, rowHeight int, visibleRows int) {
		if length == 0 {
			state.selected = 0
			state.offset = 0
			return
		}
		if state.selected >= length {
			state.selected = length - 1
		}
		if state.selected < 0 {
			state.selected = 0
		}
		capacity := visibleRows / rowHeight
		if capacity < 1 {
			capacity = 1
		}
		maxOffset := length - capacity
		if maxOffset < 0 {
			maxOffset = 0
		}
		if state.offset > maxOffset {
			state.offset = maxOffset
		}
		if state.selected < state.offset {
			state.offset = state.selected
		}
		if state.selected >= state.offset+capacity {
			state.offset = state.selected - capacity + 1
		}
		if state.offset < 0 {
			state.offset = 0
		}
	}

	matchesScope := func(n ipc.Note, s noteScope, ctx tmuxContext) bool {
		ns := noteScopeOf(n)
		switch s {
		case scopeWindow:
			if ns == scopeAll {
				return true
			}
			if ns == scopeSession && strings.TrimSpace(n.SessionID) == strings.TrimSpace(ctx.SessionID) {
				return true
			}
			return ns == scopeWindow && strings.TrimSpace(n.WindowID) == strings.TrimSpace(ctx.WindowID)
		case scopeSession:
			if ns == scopeAll {
				return true
			}
			return strings.TrimSpace(n.SessionID) == strings.TrimSpace(ctx.SessionID)
		case scopeAll:
			return true
		default:
			return true
		}
	}

	sortNotes := func(notes []ipc.Note) {
		sort.SliceStable(notes, func(i, j int) bool {
			ic, hasIC := parseTimestamp(notes[i].CreatedAt)
			jc, hasJC := parseTimestamp(notes[j].CreatedAt)
			if hasIC && hasJC && !ic.Equal(jc) {
				return ic.Before(jc)
			}
			if hasIC != hasJC {
				return hasIC
			}
			return notes[i].Summary < notes[j].Summary
		})
	}

	sortGoals := func(goals []ipc.Goal) {
		sort.SliceStable(goals, func(i, j int) bool {
			ci, hasCi := parseTimestamp(goals[i].CreatedAt)
			cj, hasCj := parseTimestamp(goals[j].CreatedAt)
			if hasCi && hasCj && !ci.Equal(cj) {
				return ci.After(cj)
			}
			if hasCi != hasCj {
				return hasCi
			}
			return goals[i].Summary < goals[j].Summary
		})
	}

	textWidth := func(s string) int {
		return utf8.RuneCountInString(s)
	}

	getVisibleTasks := func() []ipc.Task {

		result := make([]ipc.Task, 0, len(st.tasks))
		for _, t := range st.tasks {
			key := fmt.Sprintf("%s|%s|%s", strings.TrimSpace(t.SessionID), strings.TrimSpace(t.WindowID), strings.TrimSpace(t.Pane))
			if !showCompletedTasks && t.Status == statusCompleted && !keepTasksVisible[key] {
				continue
			}
			result = append(result, t)
		}
		sortTasks(result)
		return result
	}

	getVisibleNotes := func() []ipc.Note {
		result := make([]ipc.Note, 0, len(st.notes))
		for _, n := range st.notes {
			if n.Archived {
				continue
			}
			if !showCompletedNotes && n.Completed && !keepNotesVisible[n.ID] {
				continue
			}
			if matchesScope(n, scope, currentCtx) {
				result = append(result, n)
			}
		}
		sortNotes(result)
		return result
	}

	getVisibleGoals := func() []ipc.Goal {
		result := make([]ipc.Goal, 0, len(st.goals))
		for _, g := range st.goals {
			result = append(result, g)
		}
		sortGoals(result)
		return result
	}

	getArchivedNotes := func() []ipc.Note {
		result := make([]ipc.Note, 0, len(st.archived))
		for _, n := range st.archived {
			if !showCompletedArchive && n.Completed {
				continue
			}
			result = append(result, n)
		}
		sortNotes(result)
		return result
	}

	setScopeFields := func(env *ipc.Envelope, s noteScope, ctx tmuxContext) {
		env.Scope = string(s)
		env.Session = ctx.SessionName
		env.SessionID = ctx.SessionID
		if ctx.WindowName != "" || ctx.WindowID != "" {
			env.Window = ctx.WindowName
			env.WindowID = ctx.WindowID
		}
		if ctx.PaneID != "" {
			env.Pane = ctx.PaneID
		}
	}

	addGoal := func(text string) error {
		text = strings.TrimSpace(text)
		if text == "" {
			return fmt.Errorf("goal text required")
		}
		refreshCtx()
		ctx := currentCtx
		if strings.TrimSpace(ctx.SessionID) == "" {
			return fmt.Errorf("session required to add goal")
		}
		return sendCommand("goal_add", func(env *ipc.Envelope) {
			env.Summary = text
			env.Session = ctx.SessionName
			env.SessionID = ctx.SessionID
		})
	}

	toggleGoal := func(id string) error {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("goal id required")
		}
		return sendCommand("goal_toggle_complete", func(env *ipc.Envelope) {
			env.GoalID = id
		})
	}

	deleteGoal := func(id string) error {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("goal id required")
		}
		return sendCommand("goal_delete", func(env *ipc.Envelope) {
			env.GoalID = id
		})
	}

	focusGoal := func(g ipc.Goal) error {
		if strings.TrimSpace(g.SessionID) == "" {
			return fmt.Errorf("session required to focus goal")
		}
		return sendCommand("goal_focus", func(env *ipc.Envelope) {
			env.SessionID = g.SessionID
			env.Session = g.Session
		})
	}

	addNote := func(text string) error {
		text = strings.TrimSpace(text)
		if text == "" {
			return fmt.Errorf("note text required")
		}
		refreshCtx()
		ctx := currentCtx
		return sendCommand("note_add", func(env *ipc.Envelope) {
			env.Summary = text
			setScopeFields(env, scope, ctx)
		})
	}

	updateNote := func(id, text, scope string) error {
		text = strings.TrimSpace(text)
		scope = strings.TrimSpace(scope)
		if text == "" && scope == "" {
			return fmt.Errorf("note text or scope required")
		}
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("note id required")
		}
		return sendCommand("note_edit", func(env *ipc.Envelope) {
			env.NoteID = id
			env.Summary = text
			env.Scope = scope
		})
	}

	cycleNoteScope := func(n ipc.Note) error {
		current := noteScopeOf(n)
		var next noteScope
		switch current {
		case scopeWindow:
			next = scopeSession
		case scopeSession:
			next = scopeAll
		case scopeAll:
			next = scopeWindow
		default:
			next = scopeWindow
		}
		return updateNote(n.ID, "", string(next))
	}

	toggleNote := func(id string) error {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("note id required")
		}
		keepNotesVisible[id] = true
		return sendCommand("note_toggle_complete", func(env *ipc.Envelope) {
			env.NoteID = id
		})
	}

	deleteNote := func(id string) error {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("note id required")
		}
		return sendCommand("note_delete", func(env *ipc.Envelope) {
			env.NoteID = id
		})
	}

	archiveNote := func(id string) error {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("note id required")
		}
		return sendCommand("note_archive", func(env *ipc.Envelope) {
			env.NoteID = id
		})
	}

	attachNote := func(id string) error {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("note id required")
		}
		refreshCtx()
		ctx := currentCtx
		return sendCommand("note_attach", func(env *ipc.Envelope) {
			env.NoteID = id
			setScopeFields(env, scopeWindow, ctx)
		})
	}

	toggleTask := func(t ipc.Task) error {
		key := fmt.Sprintf("%s|%s|%s", strings.TrimSpace(t.SessionID), strings.TrimSpace(t.WindowID), strings.TrimSpace(t.Pane))
		keepTasksVisible[key] = true
		if t.Status == statusInProgress {
			return sendCommand("finish_task", func(env *ipc.Envelope) {
				env.Session = t.Session
				env.SessionID = t.SessionID
				env.Window = t.Window
				env.WindowID = t.WindowID
				env.Pane = t.Pane
			})
		}
		return sendCommand("acknowledge", func(env *ipc.Envelope) {
			env.Session = t.Session
			env.SessionID = t.SessionID
			env.Window = t.Window
			env.WindowID = t.WindowID
			env.Pane = t.Pane
		})
	}

	deleteTask := func(t ipc.Task) error {
		return sendCommand("delete_task", func(env *ipc.Envelope) {
			env.Session = t.Session
			env.SessionID = t.SessionID
			env.Window = t.Window
			env.WindowID = t.WindowID
			env.Pane = t.Pane
		})
	}

	focusTask := func(t ipc.Task) error {
		return sendCommand("focus_task", func(env *ipc.Envelope) {
			env.Session = t.Session
			env.SessionID = t.SessionID
			env.Window = t.Window
			env.WindowID = t.WindowID
			env.Pane = t.Pane
		})
	}

	startAddPrompt := func() {
		prompt = promptState{active: true, mode: promptAddNote, text: []rune{}, cursor: 0}
	}

	startGoalPrompt := func() {
		prompt = promptState{active: true, mode: promptAddGoal, text: []rune{}, cursor: 0}
	}

	startEditPrompt := func(n ipc.Note) {
		copy := n
		editNote = &copy
		mode = viewEdit
		runes := []rune(n.Summary)
		prompt = promptState{active: true, mode: promptEditNote, text: runes, cursor: len(runes), noteID: n.ID}
	}

	handlePromptKey := func(tev *tcell.EventKey) (bool, error) {
		if !prompt.active {
			return false, nil
		}
		switch tev.Key() {
		case tcell.KeyEnter:
			text := strings.TrimSpace(string(prompt.text))
			var err error
			switch prompt.mode {
			case promptAddNote:
				err = addNote(text)
			case promptEditNote:
				err = updateNote(prompt.noteID, text, "")
			case promptAddGoal:
				err = addGoal(text)
			}
			prompt.active = false
			if prompt.mode == promptEditNote {
				mode = viewNotes
				editNote = nil
			}
			if err != nil {
				return true, err
			}
			return true, nil
		case tcell.KeyEscape:
			prompt.active = false
			if prompt.mode == promptEditNote {
				mode = viewNotes
				editNote = nil
			}
			return true, nil
		case tcell.KeyLeft:
			if prompt.cursor > 0 {
				prompt.cursor--
			}
			return true, nil
		case tcell.KeyRight:
			if prompt.cursor < len(prompt.text) {
				prompt.cursor++
			}
			return true, nil
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if prompt.cursor > 0 {
				prompt.text = append(prompt.text[:prompt.cursor-1], prompt.text[prompt.cursor:]...)
				prompt.cursor--
			}
			return true, nil
		case tcell.KeyCtrlW:
			if prompt.cursor > 0 {
				// skip trailing spaces
				i := prompt.cursor
				for i > 0 && unicode.IsSpace(prompt.text[i-1]) {
					i--
				}
				// skip non-spaces
				for i > 0 && !unicode.IsSpace(prompt.text[i-1]) {
					i--
				}
				prompt.text = append(prompt.text[:i], prompt.text[prompt.cursor:]...)
				prompt.cursor = i
			}
			return true, nil
		case tcell.KeyCtrlU:
			prompt.text = prompt.text[:0]
			prompt.cursor = 0
			return true, nil
		case tcell.KeyTab:
			if prompt.mode == promptAddNote {
				cycleScope(true, true)
			}
			return true, nil
		case tcell.KeyRune:
			r := tev.Rune()
			prompt.text = append(prompt.text[:prompt.cursor], append([]rune{r}, prompt.text[prompt.cursor:]...)...)
			prompt.cursor++
			return true, nil
		default:
			return true, nil
		}
	}

	draw := func(now time.Time) {
		screen.Clear()
		width, height := screen.Size()

		headerStyle := tcell.StyleDefault.Foreground(tcell.ColorLightCyan).Bold(true)
		subtleStyle := tcell.StyleDefault.Foreground(tcell.ColorLightSlateGray)
		infoStyle := tcell.StyleDefault.Foreground(tcell.ColorSilver)

		title := "Tracker"
		if mode == viewNotes {
			title = "Notes"
		} else if mode == viewArchive {
			title = "Archive"
		} else if mode == viewEdit {
			title = "Edit Note"
		}
		subtitle := st.message
		if mode == viewNotes {
			completedState := "hidden"
			if showCompletedNotes {
				completedState = "shown"
			}
			subtitle = fmt.Sprintf("%s · Completed: %s", st.message, completedState)
		} else if mode == viewEdit && editNote != nil {
			subtitle = fmt.Sprintf("%s · %s · %s", st.message, editNote.Session, editNote.Window)
		}

		writeStyledLine(screen, 0, 0, truncate(fmt.Sprintf("▌ %s", title), width), headerStyle)
		writeStyledLine(screen, 0, 1, truncate(subtitle, width), subtleStyle)
		if width > 0 {
			writeStyledLine(screen, 0, 2, strings.Repeat("─", width), infoStyle)
		}

		visibleRows := height - 3
		if visibleRows < 0 {
			visibleRows = 0
		}

		renderTasks := func(list []ipc.Task, state *listState) {
			clampList(state, len(list), 3, visibleRows)
			row := 3
			for idx := state.offset; idx < len(list); idx++ {
				if row >= height {
					break
				}
				t := list[idx]
				indicator := taskIndicator(t, now)
				summary := t.Summary
				if summary == "" {
					summary = "(no summary)"
				}

				// Style definitions
				baseStyle := tcell.StyleDefault
				accentStyle := tcell.StyleDefault.Foreground(tcell.ColorLightSlateGray)
				timeStyle := tcell.StyleDefault.Foreground(tcell.ColorDarkCyan)

				switch t.Status {
				case statusInProgress:
					baseStyle = baseStyle.Foreground(tcell.ColorLightGoldenrodYellow).Bold(true)
					indicator = "▶ " + indicator
				case statusCompleted:
					if t.Acknowledged {
						baseStyle = baseStyle.Foreground(tcell.ColorLightGreen)
					} else {
						baseStyle = baseStyle.Foreground(tcell.ColorFuchsia)
					}
				}

				if idx == state.selected {
					baseStyle = baseStyle.Background(tcell.ColorDarkSlateGray)
					accentStyle = accentStyle.Background(tcell.ColorDarkSlateGray)
					timeStyle = timeStyle.Background(tcell.ColorDarkSlateGray)
				}

				// Line 1: Indicator + Summary + Right-aligned Duration
				dur := liveDuration(t, now)
				availWidth := width - len(indicator) - 1 - len(dur) - 3
				if availWidth < 0 {
					availWidth = 0
				}

				line1Segs := []struct {
					text  string
					style tcell.Style
				}{
					{text: indicator + " ", style: baseStyle},
					{text: truncate(summary, availWidth), style: baseStyle},
				}

				// Fill spacing for right alignment
				usedWidth := len(indicator) + 1 + len([]rune(truncate(summary, availWidth)))
				padding := width - usedWidth - len(dur)
				if padding > 0 {
					line1Segs = append(line1Segs, struct {
						text  string
						style tcell.Style
					}{
						text:  strings.Repeat(" ", padding),
						style: baseStyle,
					})
				}
				line1Segs = append(line1Segs, struct {
					text  string
					style tcell.Style
				}{
					text:  dur,
					style: timeStyle,
				})

				writeStyledSegments(screen, row, line1Segs...)
				row++

				if row >= height {
					break
				}

				// Line 2: Meta info (Session / Window)
				meta := fmt.Sprintf("   └ %s / %s", t.Session, t.Window)
				if t.Status == statusCompleted && !t.Acknowledged {
					meta += " (awaiting review)"
				}

				writeStyledLine(screen, 0, row, truncate(meta, width), accentStyle)
				row++

				if t.CompletionNote != "" && row < height {
					note := fmt.Sprintf("     Note: %s", t.CompletionNote)
					noteStyle := tcell.StyleDefault.Foreground(tcell.ColorLightSteelBlue)
					if idx == state.selected {
						noteStyle = noteStyle.Background(tcell.ColorDarkSlateGray)
					}
					writeStyledLine(screen, 0, row, truncate(note, width), noteStyle)
					row++
				}

				// Spacer
				if row < height {
					if idx == state.selected {
						// Optional: subtle separator for selected item
					}
					row++
				}
			}
		}

		wrapText := func(text string, maxWidth int) []string {
			if maxWidth <= 0 {
				return []string{""}
			}
			runes := []rune(text)
			if len(runes) <= maxWidth {
				return []string{text}
			}
			var lines []string
			for len(runes) > maxWidth {
				split := maxWidth
				found := false
				for i := maxWidth; i > 0; i-- {
					if unicode.IsSpace(runes[i]) {
						split = i
						found = true
						break
					}
				}
				if !found {
					split = maxWidth
				}
				lines = append(lines, string(runes[:split]))
				runes = runes[split:]
				for len(runes) > 0 && unicode.IsSpace(runes[0]) {
					runes = runes[1:]
				}
			}
			if len(runes) > 0 {
				lines = append(lines, string(runes))
			}
			return lines
		}

		renderGoals := func(list []ipc.Goal, state *listState, focused bool, startRow int) int {
			gutterText := "  "
			gutterStyle := infoStyle
			if focused {
				gutterText = "│ "
				gutterStyle = tcell.StyleDefault.Foreground(tcell.ColorLightCyan)
			}
			row := startRow
			header := "Goals (all sessions)"
			if strings.TrimSpace(currentCtx.SessionName) != "" {
				header = fmt.Sprintf("Goals (all) · current: %s", currentCtx.SessionName)
			}
			headerStyle := infoStyle
			if focused {
				headerStyle = headerStyle.Bold(true)
			}
			contentWidth := width - textWidth(gutterText)
			if contentWidth < 0 {
				contentWidth = 0
			}
			writeStyledSegments(screen, row,
				struct {
					text  string
					style tcell.Style
				}{text: gutterText, style: gutterStyle},
				struct {
					text  string
					style tcell.Style
				}{text: truncate(header, contentWidth), style: headerStyle},
			)
			row++
			availableRows := height - row
			if availableRows < 0 {
				availableRows = 0
			}
			clampList(state, len(list), 3, availableRows)
			if len(list) == 0 {
				if row < height {
					writeStyledSegments(screen, row,
						struct {
							text  string
							style tcell.Style
						}{text: gutterText, style: gutterStyle},
						struct {
							text  string
							style tcell.Style
						}{text: truncate("No goals for this session.", contentWidth), style: subtleStyle},
					)
					row++
				}
				return row
			}
			for idx := state.offset; idx < len(list); idx++ {
				if row >= height {
					break
				}
				g := list[idx]
				indicator := "•"
				baseStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite)
				metaStyle := tcell.StyleDefault.Foreground(tcell.ColorLightSlateGray)
				timeStyle := tcell.StyleDefault.Foreground(tcell.ColorDarkCyan)
				if g.Completed {
					indicator = "✓"
					baseStyle = tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
					metaStyle = tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
					timeStyle = tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
				}
				if strings.TrimSpace(g.SessionID) == strings.TrimSpace(currentCtx.SessionID) && strings.TrimSpace(currentCtx.SessionID) != "" {
					metaStyle = tcell.StyleDefault.Foreground(tcell.ColorLightGreen)
				}
				itemGutterStyle := gutterStyle
				if focused && idx == state.selected {
					baseStyle = baseStyle.Background(tcell.ColorDarkSlateGray)
					metaStyle = metaStyle.Background(tcell.ColorDarkSlateGray)
					timeStyle = timeStyle.Background(tcell.ColorDarkSlateGray)
					itemGutterStyle = itemGutterStyle.Background(tcell.ColorDarkSlateGray)
				}

				created := ""
				if ts, ok := parseTimestamp(g.CreatedAt); ok {
					created = ts.Format("15:04")
				}

				summary := g.Summary
				if summary == "" {
					summary = "(no summary)"
				}
				avail := contentWidth - textWidth(indicator) - 1 - textWidth(created)
				if avail < 5 {
					avail = 5
				}
				lineText := truncate(summary, avail)
				segs := []struct {
					text  string
					style tcell.Style
				}{
					{text: gutterText, style: itemGutterStyle},
					{text: indicator + " ", style: baseStyle},
					{text: lineText, style: baseStyle},
				}
				used := textWidth(indicator) + 1 + textWidth(lineText)
				padding := contentWidth - used - textWidth(created)
				if padding > 0 {
					segs = append(segs, struct {
						text  string
						style tcell.Style
					}{
						text:  strings.Repeat(" ", padding),
						style: baseStyle,
					})
				}
				segs = append(segs, struct {
					text  string
					style tcell.Style
				}{
					text:  created,
					style: timeStyle,
				})
				writeStyledSegments(screen, row, segs...)
				row++
				if row >= height {
					break
				}

				meta := fmt.Sprintf("   └ %s", g.Session)
				metaDisplay := truncate(meta, contentWidth)
				metaPadding := contentWidth - textWidth(metaDisplay)
				writeStyledSegments(screen, row,
					struct {
						text  string
						style tcell.Style
					}{text: gutterText, style: itemGutterStyle},
					struct {
						text  string
						style tcell.Style
					}{text: metaDisplay, style: metaStyle},
					struct {
						text  string
						style tcell.Style
					}{text: strings.Repeat(" ", metaPadding), style: metaStyle},
				)
				row++
				if row >= height {
					break
				}

				spacerStyle := tcell.StyleDefault
				if focused && idx == state.selected {
					spacerStyle = spacerStyle.Background(tcell.ColorDarkSlateGray)
				}
				writeStyledSegments(screen, row,
					struct {
						text  string
						style tcell.Style
					}{text: gutterText, style: itemGutterStyle},
					struct {
						text  string
						style tcell.Style
					}{text: strings.Repeat(" ", contentWidth), style: spacerStyle},
				)
				row++
			}
			return row
		}

		renderNotes := func(list []ipc.Note, state *listState, archived bool, startRow int, focused bool) int {
			availableRows := height - startRow
			if availableRows < 0 {
				availableRows = 0
			}
			clampList(state, len(list), 3, availableRows)
			row := startRow
			gutterText := "  "
			gutterStyle := infoStyle
			if focused {
				gutterText = "│ "
				gutterStyle = tcell.StyleDefault.Foreground(tcell.ColorLightCyan)
			}
			contentWidth := width - textWidth(gutterText)
			if contentWidth < 0 {
				contentWidth = 0
			}
			for idx := state.offset; idx < len(list); idx++ {
				if row >= height {
					break
				}
				n := list[idx]
				ns := noteScopeOf(n)

				// Styles
				scopeSt := scopeStyle(ns)
				textStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite)
				metaStyle := tcell.StyleDefault.Foreground(tcell.ColorLightSlateGray)
				timeStyle := tcell.StyleDefault.Foreground(tcell.ColorDarkCyan)

				if n.Completed {
					textStyle = tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
					scopeSt = tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
					metaStyle = tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
					timeStyle = tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
				}

				if focused && idx == state.selected {
					textStyle = textStyle.Background(tcell.ColorDarkSlateGray)
					scopeSt = scopeSt.Background(tcell.ColorDarkSlateGray)
					metaStyle = metaStyle.Background(tcell.ColorDarkSlateGray)
					timeStyle = timeStyle.Background(tcell.ColorDarkSlateGray)
				}

				// Tag Logic
				tagText := " " + scopeTag(ns) + " "

				// Timestamp Logic
				tsStr := ""
				if archived {
					if n.ArchivedAt != "" {
						if ts, ok := parseTimestamp(n.ArchivedAt); ok {
							tsStr = ts.Format("15:04")
						}
					}
				} else {
					if n.CreatedAt != "" {
						if ts, ok := parseTimestamp(n.CreatedAt); ok {
							tsStr = ts.Format("15:04")
						}
					}
				}

				summary := n.Summary
				if n.Completed {
					summary = "✓ " + summary
				}

				// --- Line 1 Rendering ---
				availWidth1 := contentWidth - len(tagText) - len(tsStr) - 2
				if availWidth1 < 5 {
					availWidth1 = 5
				}

				// Split summary for first line
				line1Text := summary
				remText := ""
				runes := []rune(summary)
				if len(runes) > availWidth1 {
					split := availWidth1
					found := false
					for i := availWidth1; i > 0; i-- {
						if unicode.IsSpace(runes[i]) {
							split = i
							found = true
							break
						}
					}
					if !found {
						split = availWidth1
					}

					line1Text = string(runes[:split])
					remText = string(runes[split:])
					// trim leading space from remainder
					trimRunes := []rune(remText)
					for len(trimRunes) > 0 && unicode.IsSpace(trimRunes[0]) {
						trimRunes = trimRunes[1:]
					}
					remText = string(trimRunes)
				}

				segs := []struct {
					text  string
					style tcell.Style
				}{
					{text: gutterText, style: gutterStyle},
					{text: tagText, style: scopeSt},
					{text: line1Text, style: textStyle},
				}

				usedLen := len(tagText) + len([]rune(line1Text))
				padding := contentWidth - usedLen - len(tsStr)
				if padding > 0 {
					segs = append(segs, struct {
						text  string
						style tcell.Style
					}{
						text:  strings.Repeat(" ", padding),
						style: textStyle,
					})
				}
				segs = append(segs, struct {
					text  string
					style tcell.Style
				}{
					text:  tsStr,
					style: timeStyle,
				})

				writeStyledSegments(screen, row, segs...)
				row++
				if row >= height {
					break
				}

				// --- Wrapped Lines Rendering ---
				if remText != "" {
					indent := len(tagText)
					availWidthN := contentWidth - indent
					if availWidthN < 5 {
						availWidthN = 5
					}

					wrappedRem := wrapText(remText, availWidthN)
					for _, wLine := range wrappedRem {
						segs := []struct {
							text  string
							style tcell.Style
						}{
							{text: gutterText, style: gutterStyle},
							{text: strings.Repeat(" ", indent), style: scopeSt},
							{text: wLine, style: textStyle},
						}
						used := indent + len([]rune(wLine))
						if contentWidth > used {
							segs = append(segs, struct {
								text  string
								style tcell.Style
							}{
								text:  strings.Repeat(" ", contentWidth-used),
								style: textStyle,
							})
						}
						writeStyledSegments(screen, row, segs...)
						row++
						if row >= height {
							break
						}
					}
				}
				if row >= height {
					break
				}

				// --- Meta Line Rendering ---
				prefix := "   "
				metaText := fmt.Sprintf("%s / %s", n.Session, n.Window)

				metaSegs := []struct {
					text  string
					style tcell.Style
				}{
					{text: gutterText, style: gutterStyle},
					{text: prefix, style: metaStyle},
					{text: truncate(metaText, contentWidth-len(prefix)), style: metaStyle},
				}
				metaUsed := len(prefix) + len([]rune(truncate(metaText, contentWidth-len(prefix))))
				if contentWidth > metaUsed {
					metaSegs = append(metaSegs, struct {
						text  string
						style tcell.Style
					}{
						text:  strings.Repeat(" ", contentWidth-metaUsed),
						style: metaStyle,
					})
				}

				writeStyledSegments(screen, row, metaSegs...)
				row++
				if row >= height {
					break
				}

				// --- Spacer Rendering ---
				spacerStyle := tcell.StyleDefault
				if focused && idx == state.selected {
					spacerStyle = spacerStyle.Background(tcell.ColorDarkSlateGray)
				}
				writeStyledSegments(screen, row,
					struct {
						text  string
						style tcell.Style
					}{text: gutterText, style: gutterStyle},
					struct {
						text  string
						style tcell.Style
					}{text: strings.Repeat(" ", contentWidth), style: spacerStyle},
				)
				row++
			}
			return row
		}

		if helpVisible {
			helpLines := []string{
				"t: toggle Tracker/Notes | Tab: focus goals/notes | n/i: view scope | Alt-A: archive view",
				"Goals: a add | Enter/c: complete | Shift-D: delete (focus goals first)",
				"Notes: a add | k edit | Enter/c: complete | Shift-A: archive | Shift-D: delete | Shift-C: show/hide completed | Esc: close | ?: toggle help",
			}
			row := 3
			for _, line := range helpLines {
				if row >= height {
					break
				}
				writeStyledLine(screen, 0, row, truncate(line, width), infoStyle)
				row++
			}
			screen.Show()
			return
		}

		switch mode {
		case viewTracker:
			list := getVisibleTasks()
			if len(list) == 0 && height > 3 {
				writeStyledLine(screen, 0, 3, truncate("No tasks.", width), infoStyle)
			} else {
				renderTasks(list, &taskList)
			}
		case viewNotes:
			goals := getVisibleGoals()
			notes := getVisibleNotes()
			rowStart := 3
			rowStart = renderGoals(goals, &goalList, focusGoals, rowStart)
			if rowStart < height {
				notesHeader := "Notes"
				notesStyle := infoStyle
				gutterText := "  "
				gutterStyle := infoStyle
				scopeLabel := "Window"
				switch scope {
				case scopeSession:
					scopeLabel = "Session"
				case scopeAll:
					scopeLabel = "Global"
				}
				scopeText := fmt.Sprintf("[%s]", scopeLabel)
				scopeLabelStyle := scopeStyle(scope)
				if !focusGoals {
					notesStyle = notesStyle.Bold(true)
					gutterText = "│ "
					gutterStyle = tcell.StyleDefault.Foreground(tcell.ColorLightCyan)
				}
				contentWidth := width - textWidth(gutterText)
				if contentWidth < 0 {
					contentWidth = 0
				}
				spaceWidth := 1
				combinedWidth := textWidth(notesHeader) + spaceWidth + textWidth(scopeText)
				if combinedWidth > contentWidth {
					if contentWidth > textWidth(notesHeader)+spaceWidth {
						scopeText = truncate(scopeText, contentWidth-(textWidth(notesHeader)+spaceWidth))
					} else {
						scopeText = ""
						spaceWidth = 0
					}
				}
				writeStyledSegments(screen, rowStart,
					struct {
						text  string
						style tcell.Style
					}{text: gutterText, style: gutterStyle},
					struct {
						text  string
						style tcell.Style
					}{text: truncate(notesHeader, contentWidth), style: notesStyle},
					struct {
						text  string
						style tcell.Style
					}{text: strings.Repeat(" ", spaceWidth), style: notesStyle},
					struct {
						text  string
						style tcell.Style
					}{text: scopeText, style: scopeLabelStyle},
				)
				rowStart++
			}
			if len(notes) == 0 && height > rowStart {
				gutterText := "  "
				gutterStyle := infoStyle
				if !focusGoals {
					gutterText = "│ "
					gutterStyle = tcell.StyleDefault.Foreground(tcell.ColorLightCyan)
				}
				contentWidth := width - textWidth(gutterText)
				if contentWidth < 0 {
					contentWidth = 0
				}
				writeStyledSegments(screen, rowStart,
					struct {
						text  string
						style tcell.Style
					}{text: gutterText, style: gutterStyle},
					struct {
						text  string
						style tcell.Style
					}{text: truncate("No notes in this scope.", contentWidth), style: infoStyle},
				)
			} else {
				renderNotes(notes, &noteList, false, rowStart, !focusGoals)
			}
		case viewArchive:
			list := getArchivedNotes()
			if len(list) == 0 && height > 3 {
				writeStyledLine(screen, 0, 3, truncate("Archive is empty.", width), infoStyle)
			} else {
				renderNotes(list, &archiveList, true, 3, true)
			}
		case viewEdit:
			bodyStyle := tcell.StyleDefault.Foreground(tcell.ColorLightGreen)
			if editNote == nil {
				writeStyledLine(screen, 0, 3, truncate("No note selected.", width), infoStyle)
			} else {
				writeStyledLine(screen, 0, 3, truncate("Editing note (Enter to save, Esc to cancel):", width), infoStyle)
				writeStyledLine(screen, 0, 5, truncate(string(prompt.text), width), bodyStyle)
				if prompt.active {
					cx := prompt.cursor
					if cx > width-1 {
						cx = width - 1
					}
					screen.ShowCursor(cx, 5)
				}
			}
		}

		if prompt.active && mode != viewEdit {
			label := "Add note: "
			if prompt.mode == promptEditNote {
				label = "Edit note: "
			} else if prompt.mode == promptAddGoal {
				label = "Add goal: "
			} else if prompt.mode == promptAddNote {
				label = fmt.Sprintf("Add note (%s): ", scopeTag(scope))
			}
			line := label + string(prompt.text)
			writeStyledLine(screen, 0, height-1, truncate(line, width), tcell.StyleDefault.Foreground(tcell.ColorLightGreen))

			cx := len(label) + prompt.cursor
			if cx > width-1 {
				cx = width - 1
			}
			screen.ShowCursor(cx, height-1)
		}

		screen.Show()
	}

	draw(time.Now())

	for {
		select {
		case env, ok := <-incoming:
			if !ok {
				return <-errCh
			}
			switch env.Kind {
			case "state":
				st.message = env.Message
				st.tasks = make([]ipc.Task, len(env.Tasks))
				copy(st.tasks, env.Tasks)
				st.notes = make([]ipc.Note, len(env.Notes))
				copy(st.notes, env.Notes)
				st.archived = make([]ipc.Note, len(env.Archived))
				copy(st.archived, env.Archived)
				st.goals = make([]ipc.Goal, len(env.Goals))
				copy(st.goals, env.Goals)
				refreshCtx()
				draw(time.Now())
			case "ack":
			default:
			}
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			switch tev := ev.(type) {
			case *tcell.EventKey:
				if handled, err := handlePromptKey(tev); handled {
					if err != nil {
						st.message = err.Error()
					}
					draw(time.Now())
					continue
				}

				if tev.Key() == tcell.KeyRune && tev.Rune() == '?' {
					helpVisible = !helpVisible
					draw(time.Now())
					continue
				}

				if tev.Key() == tcell.KeyEscape {
					if prompt.active {
						prompt.active = false
						draw(time.Now())
						continue
					}
					if mode == viewEdit {
						mode = viewNotes
						editNote = nil
						draw(time.Now())
						continue
					}
					if err := sendCommand("hide"); err != nil {
						return err
					}
					return nil
				}

				if tev.Key() == tcell.KeyCtrlC {
					if err := sendCommand("hide"); err != nil {
						return err
					}
					return nil
				}

				if tev.Modifiers()&tcell.ModAlt != 0 {
					r := unicode.ToLower(tev.Rune())
					if r == 'a' {
						if mode == viewNotes {
							mode = viewArchive
						} else if mode == viewArchive {
							mode = viewNotes
						} else {
							mode = viewNotes
						}
						draw(time.Now())
						continue
					}
				}

				if tev.Key() == tcell.KeyEnter {
					switch mode {
					case viewTracker:
						tasks := getVisibleTasks()
						if len(tasks) > 0 && taskList.selected < len(tasks) {
							if err := focusTask(tasks[taskList.selected]); err != nil {
								st.message = err.Error()
							}
						}
					case viewNotes:
						if focusGoals {
							goals := getVisibleGoals()
							if len(goals) > 0 && goalList.selected < len(goals) {
								if err := focusGoal(goals[goalList.selected]); err != nil {
									st.message = err.Error()
								}
							}
						} else {
							notes := getVisibleNotes()
							if len(notes) > 0 && noteList.selected < len(notes) {
								if err := focusTask(ipc.Task{
									Session:   notes[noteList.selected].Session,
									SessionID: notes[noteList.selected].SessionID,
									Window:    notes[noteList.selected].Window,
									WindowID:  notes[noteList.selected].WindowID,
									Pane:      notes[noteList.selected].Pane,
								}); err != nil {
									st.message = err.Error()
								}
							}
						}
					case viewArchive:
						notes := getArchivedNotes()
						if len(notes) > 0 && archiveList.selected < len(notes) {
							if err := attachNote(notes[archiveList.selected].ID); err != nil {
								st.message = err.Error()
							}
						}
					}
					draw(time.Now())
					continue
				}

				if tev.Key() == tcell.KeyTab && mode == viewNotes {
					focusGoals = !focusGoals
					draw(time.Now())
					continue
				}

				if tev.Key() != tcell.KeyRune {
					continue
				}

				r := tev.Rune()
				lower := unicode.ToLower(r)
				shift := tev.Modifiers()&tcell.ModShift != 0 || unicode.IsUpper(r)

				switch lower {
				case 't':
					if mode == viewTracker {
						mode = viewNotes
					} else {
						mode = viewTracker
					}
					draw(time.Now())
				case 'n':
					if mode == viewNotes {
						cycleScope(false, false)
						draw(time.Now())
					}
				case 'i':
					if mode == viewNotes {
						cycleScope(true, false)
						draw(time.Now())
					}
				case 's':
					if mode == viewNotes {
						notes := getVisibleNotes()
						if len(notes) > 0 && noteList.selected < len(notes) {
							if err := cycleNoteScope(notes[noteList.selected]); err != nil {
								st.message = err.Error()
							}
						}
						draw(time.Now())
					}
				case 'u':
					switch mode {
					case viewTracker:
						if taskList.selected > 0 {
							taskList.selected--
						}
					case viewNotes:
						if focusGoals {
							if goalList.selected > 0 {
								goalList.selected--
							}
						} else {
							if noteList.selected > 0 {
								noteList.selected--
							}
						}
					case viewArchive:
						if archiveList.selected > 0 {
							archiveList.selected--
						}
					}
					draw(time.Now())
				case 'e':
					switch mode {
					case viewTracker:
						tasks := getVisibleTasks()
						if taskList.selected < len(tasks)-1 {
							taskList.selected++
						}
					case viewNotes:
						if focusGoals {
							goals := getVisibleGoals()
							if goalList.selected < len(goals)-1 {
								goalList.selected++
							}
						} else {
							notes := getVisibleNotes()
							if noteList.selected < len(notes)-1 {
								noteList.selected++
							}
						}
					case viewArchive:
						notes := getArchivedNotes()
						if archiveList.selected < len(notes)-1 {
							archiveList.selected++
						}
					}
					draw(time.Now())
				case 'c':
					if shift {
						switch mode {
						case viewNotes:
							showCompletedNotes = !showCompletedNotes
						case viewArchive:
							showCompletedArchive = !showCompletedArchive
						}
						draw(time.Now())
						break
					}
					switch mode {
					case viewTracker:
						tasks := getVisibleTasks()
						if len(tasks) > 0 && taskList.selected < len(tasks) {
							if err := toggleTask(tasks[taskList.selected]); err != nil {
								st.message = err.Error()
							}
						}
					case viewNotes:
						if focusGoals {
							goals := getVisibleGoals()
							if len(goals) > 0 && goalList.selected < len(goals) {
								if err := toggleGoal(goals[goalList.selected].ID); err != nil {
									st.message = err.Error()
								}
							}
						} else {
							notes := getVisibleNotes()
							if len(notes) > 0 && noteList.selected < len(notes) {
								if err := toggleNote(notes[noteList.selected].ID); err != nil {
									st.message = err.Error()
								}
							}
						}
					case viewArchive:
						notes := getArchivedNotes()
						if len(notes) > 0 && archiveList.selected < len(notes) {
							if err := attachNote(notes[archiveList.selected].ID); err != nil {
								st.message = err.Error()
							}
						}
					}
					draw(time.Now())
				case 'p':
					if mode == viewTracker {
						tasks := getVisibleTasks()
						if len(tasks) > 0 && taskList.selected < len(tasks) {
							if err := focusTask(tasks[taskList.selected]); err != nil {
								st.message = err.Error()
							}
						}
						draw(time.Now())
					} else if mode == viewNotes {
						notes := getVisibleNotes()
						if len(notes) > 0 && noteList.selected < len(notes) {
							if err := focusTask(ipc.Task{
								Session:   notes[noteList.selected].Session,
								SessionID: notes[noteList.selected].SessionID,
								Window:    notes[noteList.selected].Window,
								WindowID:  notes[noteList.selected].WindowID,
								Pane:      notes[noteList.selected].Pane,
							}); err != nil {
								st.message = err.Error()
							}
						}
						draw(time.Now())
					}
				case 'a':
					if shift && mode == viewNotes && !focusGoals {
						notes := getVisibleNotes()
						if len(notes) > 0 && noteList.selected < len(notes) {
							if err := archiveNote(notes[noteList.selected].ID); err != nil {
								st.message = err.Error()
							}
						}
						draw(time.Now())
						break
					}
					if mode == viewNotes {
						if focusGoals {
							startGoalPrompt()
						} else {
							startAddPrompt()
						}
						draw(time.Now())
					}
				case 'k':
					if mode == viewNotes {
						notes := getVisibleNotes()
						if len(notes) > 0 && noteList.selected < len(notes) {
							startEditPrompt(notes[noteList.selected])
						}
						draw(time.Now())
					}

				case 'd':
					if shift {
						switch mode {
						case viewTracker:
							tasks := getVisibleTasks()
							if len(tasks) > 0 && taskList.selected < len(tasks) {
								if err := deleteTask(tasks[taskList.selected]); err != nil {
									st.message = err.Error()
								}
							}
						case viewNotes:
							if focusGoals {
								goals := getVisibleGoals()
								if len(goals) > 0 && goalList.selected < len(goals) {
									if err := deleteGoal(goals[goalList.selected].ID); err != nil {
										st.message = err.Error()
									}
								}
							} else {
								notes := getVisibleNotes()
								if len(notes) > 0 && noteList.selected < len(notes) {
									if err := deleteNote(notes[noteList.selected].ID); err != nil {
										st.message = err.Error()
									}
								}
							}
						case viewArchive:
							notes := getArchivedNotes()
							if len(notes) > 0 && archiveList.selected < len(notes) {
								if err := deleteNote(notes[archiveList.selected].ID); err != nil {
									st.message = err.Error()
								}
							}
						}
						draw(time.Now())
					}
				}
			case *tcell.EventResize:
				draw(time.Now())
			}
		case now := <-ticker.C:
			draw(now)
		case err := <-errCh:
			if err == nil || errors.Is(err, io.EOF) || strings.Contains(err.Error(), "use of closed network connection") {
				return nil
			}
			return err
		}
	}
}

func sortTasks(tasks []ipc.Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		ti, tj := tasks[i], tasks[j]
		ranki := taskStatusRank(ti.Status)
		rankj := taskStatusRank(tj.Status)
		if ranki != rankj {
			return ranki < rankj
		}
		switch ti.Status {
		case statusInProgress:
			return ti.StartedAt < tj.StartedAt
		case statusCompleted:
			if ti.Acknowledged != tj.Acknowledged {
				return !ti.Acknowledged && tj.Acknowledged
			}
			if ci, hasCi := parseTimestamp(ti.CompletedAt); hasCi {
				if cj, hasCj := parseTimestamp(tj.CompletedAt); hasCj {
					if !ci.Equal(cj) {
						return ci.After(cj)
					}
				} else {
					return true
				}
			} else if _, hasCj := parseTimestamp(tj.CompletedAt); hasCj {
				return false
			}
			si, hasSi := parseTimestamp(ti.StartedAt)
			sj, hasSj := parseTimestamp(tj.StartedAt)
			if hasSi && hasSj && !si.Equal(sj) {
				return si.After(sj)
			}
			if hasSi != hasSj {
				return hasSi
			}
			return ti.StartedAt > tj.StartedAt
		default:
			return ti.StartedAt < tj.StartedAt
		}
	})
}

func taskStatusRank(status string) int {
	switch status {
	case statusInProgress:
		return 0
	case statusCompleted:
		return 1
	default:
		return 2
	}
}

func parseTimestamp(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func taskIndicator(t ipc.Task, now time.Time) string {
	switch t.Status {
	case statusInProgress:
		idx := int(now.UnixNano()/int64(spinnerInterval)) % len(spinnerFrames)
		return string(spinnerFrames[idx])
	case statusCompleted:
		if t.Acknowledged {
			return "✓"
		}
		return "⚑"
	default:
		return "?"
	}
}

func liveDuration(t ipc.Task, now time.Time) string {
	if t.StartedAt == "" {
		return "0s"
	}
	start, err := time.Parse(time.RFC3339, t.StartedAt)
	if err != nil {
		return formatDuration(t.DurationSeconds)
	}
	if t.Status == statusCompleted {
		if t.CompletedAt != "" {
			if end, err := time.Parse(time.RFC3339, t.CompletedAt); err == nil {
				return formatDuration(end.Sub(start).Seconds())
			}
		}
		return formatDuration(t.DurationSeconds)
	}
	return formatDuration(time.Since(start).Seconds())
}

func formatDuration(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	d := time.Duration(seconds * float64(time.Second))
	if d >= 99*time.Hour {
		return ">=99h"
	}
	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	secondsPart := (d % time.Minute) / time.Second
	if hours > 0 {
		return fmt.Sprintf("%02dh%02dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%02dm%02ds", minutes, secondsPart)
	}
	return fmt.Sprintf("%02ds", secondsPart)
}

func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func writeStyledLine(s tcell.Screen, x, y int, text string, style tcell.Style) {
	width, _ := s.Size()
	if x >= width {
		return
	}
	runes := []rune(text)
	limit := width - x
	for i := 0; i < limit; i++ {
		r := rune(' ')
		if i < len(runes) {
			r = runes[i]
		}
		s.SetContent(x+i, y, r, nil, style)
	}
}

func writeStyledSegments(s tcell.Screen, y int, segments ...struct {
	text  string
	style tcell.Style
}) {
	x := 0
	width, _ := s.Size()
	for _, seg := range segments {
		runes := []rune(seg.text)
		for _, r := range runes {
			if x >= width {
				return
			}
			s.SetContent(x, y, r, nil, seg.style)
			x++
		}
	}
}

func writeStyledSegmentsPad(s tcell.Screen, y int, segments []struct {
	text  string
	style tcell.Style
}, fill tcell.Style) {
	x := 0
	width, _ := s.Size()
	for _, seg := range segments {
		runes := []rune(seg.text)
		for _, r := range runes {
			if x >= width {
				return
			}
			s.SetContent(x, y, r, nil, seg.style)
			x++
		}
	}
	for x < width {
		s.SetContent(x, y, ' ', nil, fill)
		x++
	}
}

func socketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "agent-tracker.sock")
	}
	return filepath.Join(os.TempDir(), "agent-tracker.sock")
}
