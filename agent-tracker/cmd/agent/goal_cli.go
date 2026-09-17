package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

func runGoal(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent goal <add-goal|delete-goal|add-thread|rename|rename-current|set-goal|set-blocker|done|delete-thread|bind|unbind|merge|promote|revive|cleanup-window|current-thread|list>")
	}
	switch args[0] {
	case "add-goal":
		return runGoalAddGoal(args[1:])
	case "delete-goal":
		return runGoalDeleteGoal(args[1:])
	case "add-thread":
		return runGoalAddThread(args[1:])
	case "rename":
		return runGoalRename(args[1:])
	case "rename-current":
		return runGoalRenameCurrent(args[1:])
	case "set-goal":
		return runGoalSetGoal(args[1:])
	case "set-blocker":
		return runGoalSetBlocker(args[1:])
	case "done":
		return runGoalDone(args[1:])
	case "delete-thread":
		return runGoalDeleteThread(args[1:])
	case "bind":
		return runGoalBind(args[1:])
	case "unbind":
		return runGoalUnbind(args[1:])
	case "merge":
		return runGoalMerge(args[1:])
	case "promote":
		return runGoalPromote(args[1:])
	case "revive":
		return runGoalRevive(args[1:])
	case "cleanup-window":
		return runGoalCleanupWindow(args[1:])
	case "current-thread":
		return runGoalCurrentThread(args[1:])
	case "set-goal-current":
		return runGoalSetGoalCurrent(args[1:])
	case "set-blocker-current":
		return runGoalSetBlockerCurrent(args[1:])
	case "pick":
		return runGoalPick(args[1:])
	case "reap":
		reapOrphanedThreads()
		return nil
	case "list":
		return runGoalList(args[1:])
	default:
		return fmt.Errorf("unknown goal subcommand: %s", args[0])
	}
}

func runGoalAddGoal(args []string) error {
	fs := flag.NewFlagSet("agent goal add-goal", flag.ExitOnError)
	var title, parent string
	fs.StringVar(&title, "title", "", "goal title")
	fs.StringVar(&parent, "parent", "", "parent goal id")
	fs.Parse(args)
	g, err := addGoal(title, parent)
	if err != nil {
		return err
	}
	fmt.Println(g.ID)
	return nil
}

func runGoalDeleteGoal(args []string) error {
	confirm := ""
	var goalID string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--confirm" {
			if i+1 < len(args) {
				confirm = args[i+1]
				i++
			}
			continue
		}
		if strings.HasPrefix(a, "--confirm=") {
			confirm = strings.TrimPrefix(a, "--confirm=")
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		if goalID == "" {
			goalID = a
		}
	}
	if strings.TrimSpace(confirm) != "yes" {
		return fmt.Errorf("pass --confirm yes to delete a goal (cascades)")
	}
	if goalID == "" {
		return fmt.Errorf("goal id required")
	}
	return deleteGoalCascade(goalID)
}

func runGoalAddThread(args []string) error {
	fs := flag.NewFlagSet("agent goal add-thread", flag.ExitOnError)
	var name, goal string
	var blocks stringSliceFlag
	fs.StringVar(&name, "name", "", "thread name")
	fs.StringVar(&goal, "goal", "", "goal id")
	fs.Var(&blocks, "blocks", "thread id this depends on (repeatable)")
	fs.Parse(args)
	t, err := addManualThread(name, goal, blocks)
	if err != nil {
		return err
	}
	fmt.Println(t.ID)
	return nil
}

func runGoalRename(args []string) error {
	fs := flag.NewFlagSet("agent goal rename", flag.ExitOnError)
	var thread, name string
	fs.StringVar(&thread, "thread", "", "thread id")
	fs.StringVar(&name, "name", "", "new name")
	fs.Parse(args)
	if strings.TrimSpace(thread) == "" {
		return fmt.Errorf("--thread is required")
	}
	return renameThread(thread, name)
}

func runGoalRenameCurrent(args []string) error {
	fs := flag.NewFlagSet("agent goal rename-current", flag.ExitOnError)
	var window, name string
	fs.StringVar(&window, "window", "", "tmux window id (defaults to focused)")
	fs.StringVar(&name, "name", "", "new name")
	fs.Parse(args)
	window = resolveGoalWindowID(window)
	threadID, err := ensureThreadForWindow(window, name)
	if err != nil {
		return err
	}
	return renameThread(threadID, name)
}

func runGoalSetGoal(args []string) error {
	fs := flag.NewFlagSet("agent goal set-goal", flag.ExitOnError)
	var thread, goal string
	fs.StringVar(&thread, "thread", "", "thread id")
	fs.StringVar(&goal, "goal", "", "goal id (empty to unassign)")
	fs.Parse(args)
	if strings.TrimSpace(thread) == "" {
		return fmt.Errorf("--thread is required")
	}
	return setThreadGoal(thread, goal)
}

func runGoalSetBlocker(args []string) error {
	fs := flag.NewFlagSet("agent goal set-blocker", flag.ExitOnError)
	var thread string
	var by stringSliceFlag
	fs.StringVar(&thread, "thread", "", "thread id")
	fs.Var(&by, "by", "thread id that blocks this one (repeatable)")
	fs.Parse(args)
	if strings.TrimSpace(thread) == "" {
		return fmt.Errorf("--thread is required")
	}
	return setThreadBlocker(thread, by)
}

func runGoalDone(args []string) error {
	fs := flag.NewFlagSet("agent goal done", flag.ExitOnError)
	var thread string
	var undo bool
	fs.StringVar(&thread, "thread", "", "thread id")
	fs.BoolVar(&undo, "undo", false, "mark not-done")
	fs.Parse(args)
	if strings.TrimSpace(thread) == "" {
		return fmt.Errorf("--thread is required")
	}
	return setThreadDone(thread, !undo)
}

func runGoalDeleteThread(args []string) error {
	fs := flag.NewFlagSet("agent goal delete-thread", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() < 1 {
		return fmt.Errorf("thread id required")
	}
	return deleteThread(fs.Arg(0))
}

func runGoalBind(args []string) error {
	fs := flag.NewFlagSet("agent goal bind", flag.ExitOnError)
	var thread, window string
	fs.StringVar(&thread, "thread", "", "thread id")
	fs.StringVar(&window, "window", "", "tmux window id")
	fs.Parse(args)
	if strings.TrimSpace(thread) == "" || strings.TrimSpace(window) == "" {
		return fmt.Errorf("--thread and --window are required")
	}
	return bindThreadWindow(thread, window)
}

func runGoalUnbind(args []string) error {
	fs := flag.NewFlagSet("agent goal unbind", flag.ExitOnError)
	var thread string
	fs.StringVar(&thread, "thread", "", "thread id")
	fs.Parse(args)
	if strings.TrimSpace(thread) == "" {
		return fmt.Errorf("--thread is required")
	}
	return unbindThreadWindow(thread)
}

func runGoalMerge(args []string) error {
	fs := flag.NewFlagSet("agent goal merge", flag.ExitOnError)
	var source, target string
	fs.StringVar(&source, "source", "", "thread to absorb and delete")
	fs.StringVar(&target, "target", "", "thread that survives")
	fs.Parse(args)
	if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
		return fmt.Errorf("--source and --target are required")
	}
	return mergeThreads(source, target)
}

func runGoalPromote(args []string) error {
	fs := flag.NewFlagSet("agent goal promote", flag.ExitOnError)
	var window, name, goal, bind string
	fs.StringVar(&window, "window", "", "tmux window id")
	fs.StringVar(&name, "name", "", "thread name")
	fs.StringVar(&goal, "goal", "", "goal id to attach to")
	fs.StringVar(&bind, "bind", "", "existing thread id to bind the window to")
	fs.Parse(args)
	window = resolveGoalWindowID(window)
	if strings.TrimSpace(window) == "" {
		return fmt.Errorf("--window is required (or run inside tmux)")
	}
	t, err := promoteDraftToThread(window, name, goal, bind)
	if err != nil {
		return err
	}
	fmt.Println(t.ID)
	return nil
}

func runGoalRevive(args []string) error {
	fs := flag.NewFlagSet("agent goal revive", flag.ExitOnError)
	var window, name string
	fs.StringVar(&window, "window", "", "tmux window id")
	fs.StringVar(&name, "name", "", "new thread name")
	fs.Parse(args)
	window = resolveGoalWindowID(window)
	if strings.TrimSpace(window) == "" {
		return fmt.Errorf("--window is required (or run inside tmux)")
	}
	return reviveDoneThreadForWindow(window, name)
}

// runGoalCleanupWindow deletes the thread (and its window todos) for a closed window.
func runGoalCleanupWindow(args []string) error {
	fs := flag.NewFlagSet("agent goal cleanup-window", flag.ExitOnError)
	var window string
	fs.StringVar(&window, "window", "", "tmux window id")
	fs.Parse(args)
	window = strings.TrimSpace(window)
	if window == "" {
		return nil
	}
	store, err := loadGoalStore()
	if err != nil {
		return err
	}
	t := findThreadByWindow(store, window)
	if t != nil {
		_ = deleteThread(t.ID)
	}
	removeWindowTodos(window)
	return nil
}

func runGoalCurrentThread(args []string) error {
	fs := flag.NewFlagSet("agent goal current-thread", flag.ExitOnError)
	var window string
	fs.StringVar(&window, "window", "", "tmux window id (defaults to focused)")
	fs.Parse(args)
	window = resolveGoalWindowID(window)
	store, err := loadGoalStore()
	if err != nil {
		return err
	}
	t := findThreadByWindow(store, window)
	if t == nil {
		return nil
	}
	fmt.Println(t.ID)
	return nil
}

func runGoalSetGoalCurrent(args []string) error {
	fs := flag.NewFlagSet("agent goal set-goal-current", flag.ExitOnError)
	var window, goal, name string
	fs.StringVar(&window, "window", "", "tmux window id (defaults to focused)")
	fs.StringVar(&goal, "goal", "", "goal id (empty to unassign)")
	fs.StringVar(&name, "name", "", "thread name (creates the thread if needed)")
	fs.Parse(args)
	window = resolveGoalWindowID(window)
	if window == "" {
		return fmt.Errorf("no current tmux window")
	}
	threadID, err := ensureThreadForWindow(window, name)
	if err != nil {
		return err
	}
	return setThreadGoal(threadID, goal)
}

func runGoalSetBlockerCurrent(args []string) error {
	fs := flag.NewFlagSet("agent goal set-blocker-current", flag.ExitOnError)
	var window, by string
	fs.StringVar(&window, "window", "", "tmux window id (defaults to focused)")
	fs.StringVar(&by, "by", "", "thread id that blocks the current thread")
	fs.Parse(args)
	window = resolveGoalWindowID(window)
	if window == "" {
		return fmt.Errorf("no current tmux window")
	}
	threadID, err := ensureThreadForWindow(window, "")
	if err != nil {
		return err
	}
	var blockers []string
	if strings.TrimSpace(by) != "" {
		blockers = []string{strings.TrimSpace(by)}
	}
	return setThreadBlocker(threadID, blockers)
}

// runGoalPick prints pickable items for fzf-based prefix dialogs.
// `agent goal pick goals` and `agent goal pick threads`.
func runGoalPick(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent goal pick <goals|threads>")
	}
	store, err := loadGoalStore()
	if err != nil {
		return err
	}
	switch args[0] {
	case "goals":
		fmt.Println("__new__\t— create new goal —")
		appendGoalPickLines(store, "", 0)
	case "threads":
		for _, id := range sortedThreadIDs(store) {
			t := store.Threads[id]
			if t == nil {
				continue
			}
			path := strings.Join(goalPathTitles(store, t.GoalID), " › ")
			label := threadDisplayName(t, "")
			if path != "" {
				label = path + " › " + label
			}
			fmt.Printf("%s\t%s\n", t.ID, label)
		}
	default:
		return fmt.Errorf("unknown pick kind: %s", args[0])
	}
	return nil
}

func appendGoalPickLines(store *GoalStore, parentID string, depth int) {
	var children []*Goal
	for _, g := range store.Goals {
		if strings.TrimSpace(g.ParentID) == strings.TrimSpace(parentID) {
			children = append(children, g)
		}
	}
	sortGoalsByOrder(children)
	for _, g := range children {
		indent := strings.Repeat("  ", depth)
		fmt.Printf("%s\t%s%s\n", g.ID, indent, g.Title)
		appendGoalPickLines(store, g.ID, depth+1)
	}
}

func sortedThreadIDs(store *GoalStore) []string {
	ids := make([]string, 0, len(store.Threads))
	for id := range store.Threads {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func runGoalList(args []string) error {
	store, err := loadGoalStore()
	if err != nil {
		return err
	}
	drafts, _ := collectDrafts()
	list := buildGoalFlatList(store, drafts, nil)
	for _, n := range list.Nodes {
		indent := strings.Repeat("  ", n.Depth)
		switch n.Kind {
		case nodeGoal:
			fmt.Printf("%s# %s (%s)\n", indent, n.Goal.Title, n.Goal.ID)
		case nodeThread:
			fmt.Printf("%s- %s [%s] (%s)\n", indent, n.Thread.Name, n.Status, n.Thread.ID)
		case nodeDraft:
			fmt.Printf("%s- %s [draft] (%s)\n", indent, n.Draft.Name, n.Draft.WindowID)
		}
	}
	return nil
}

// ── helpers shared by CLI + panel ───────────────────────────

func resolveGoalWindowID(window string) string {
	window = strings.TrimSpace(window)
	if window != "" {
		return window
	}
	if os.Getenv("TMUX") == "" {
		return ""
	}
	out, err := runTmuxOutput(append([]string{"display-message", "-p"}, append(currentTmuxPaneTarget(), "#{window_id}")...)...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// ensureThreadForWindow resolves (or promotes) the thread for a window, returning its id.
func ensureThreadForWindow(window, name string) (string, error) {
	window = strings.TrimSpace(window)
	if window == "" {
		return "", fmt.Errorf("window id is required")
	}
	store, err := loadGoalStore()
	if err != nil {
		return "", err
	}
	if t := findThreadByWindow(store, window); t != nil {
		return t.ID, nil
	}
	t, err := promoteDraftToThread(window, name, "", "")
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

// stringSliceFlag is a repeatable string flag.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}
