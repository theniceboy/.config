package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

func runTodos(args []string) error {
	fs := flag.NewFlagSet("agent todos", flag.ContinueOnError)
	var windowID string
	var global, all, openOnly, asJSON bool
	fs.StringVar(&windowID, "window", "", "tmux window id (default: current window)")
	fs.BoolVar(&global, "global", false, "show global todos instead of window todos")
	fs.BoolVar(&all, "all", false, "show global todos plus every window")
	fs.BoolVar(&openOnly, "open", false, "hide done todos")
	fs.BoolVar(&asJSON, "json", false, "output JSON")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}

	store, err := loadTmuxTodoStore()
	if err != nil {
		return err
	}

	if all {
		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(store)
		}
		currentWindow, _ := getCurrentTmuxScopeInfo()
		printTodoScope("Global", "global", false, store.Global, openOnly)
		windowIDs := make([]string, 0, len(store.Windows))
		for id := range store.Windows {
			windowIDs = append(windowIDs, id)
		}
		sort.Strings(windowIDs)
		for _, id := range windowIDs {
			printTodoScope(windowScopeLabel(id), id, id == currentWindow, store.Windows[id], openOnly)
		}
		return nil
	}

	if global {
		if asJSON {
			return encodeTodosJSON("global", "", store.Global)
		}
		printTodoScope("Global", "global", false, store.Global, openOnly)
		return nil
	}

	windowID = strings.TrimSpace(windowID)
	if windowID == "" {
		if os.Getenv("TMUX") == "" {
			return fmt.Errorf("not inside tmux; pass -window <id>, -global, or -all")
		}
		_, windowID = getCurrentTmuxScopeInfo()
		if windowID == "" {
			return fmt.Errorf("could not resolve current tmux window")
		}
	}
	items := todoItemsForScope(store, todoScopeWindow, windowID)
	if asJSON {
		return encodeTodosJSON("window", windowID, items)
	}
	printTodoScope(windowScopeLabel(windowID), windowID, false, items, openOnly)
	return nil
}

func windowScopeLabel(windowID string) string {
	label := "window " + windowID
	if os.Getenv("TMUX") == "" {
		return label
	}
	if out, err := runTmuxOutput("display-message", "-p", "-t", windowID, "#{session_name}:#{window_name}"); err == nil {
		name := strings.TrimSpace(out)
		if session, win, ok := strings.Cut(name, ":"); ok && session != "" && win != "" {
			return label + " (" + session + ":" + win + ")"
		}
	}
	return label
}

func printTodoScope(label, scopeID string, current bool, items []tmuxTodoItem, openOnly bool) {
	shown, open, total := 0, 0, len(items)
	for _, item := range items {
		if !item.Done {
			open++
		}
		if openOnly && item.Done {
			continue
		}
		shown++
	}
	suffix := ""
	if current {
		suffix = " [current]"
	}
	fmt.Printf("%s — %d open / %d%s\n", label, open, total, suffix)
	for _, item := range items {
		if openOnly && item.Done {
			continue
		}
		mark := " "
		if item.Done {
			mark = "x"
		}
		fmt.Printf("  [%s] %s\n", mark, item.Title)
	}
}

type todosJSON struct {
	Scope   string        `json:"scope"`
	Window  string        `json:"window,omitempty"`
	Open    int           `json:"open"`
	Total   int           `json:"total"`
	Todos   []tmuxTodoItem `json:"todos"`
}

func encodeTodosJSON(scope, windowID string, items []tmuxTodoItem) error {
	open := 0
	for _, item := range items {
		if !item.Done {
			open++
		}
	}
	if items == nil {
		items = []tmuxTodoItem{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(todosJSON{Scope: scope, Window: windowID, Open: open, Total: len(items), Todos: items})
}
