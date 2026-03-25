package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
)

type dashboardMode int

const (
	modeView dashboardMode = iota
	modeEditNotes
	modeEditTask
	modeTodoPrompt
	modeConfirmDestroy
)

type todoPromptMode int

const (
	todoPromptAdd todoPromptMode = iota
	todoPromptEdit
)

type todoPrompt struct {
	active bool
	mode   todoPromptMode
	text   []rune
	cursor int
	index  int
}

func runDashboard(args []string) error {
	fs := flag.NewFlagSet("agent dashboard", flag.ContinueOnError)
	var agentID string
	fs.StringVar(&agentID, "agent-id", "", "agent id")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if agentID == "" {
		return fmt.Errorf("agent-id is required")
	}
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	record := reg.Agents[agentID]
	if record == nil {
		return fmt.Errorf("unknown agent: %s", agentID)
	}
	if record.Dashboard.Todos == nil {
		record.Dashboard.Todos = []todoItem{}
	}

	cfg := loadAppConfig()
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.Clear()

	selectedCol := 0
	todoIndex := 0
	noteScroll := 0
	taskCursor := utf8.RuneCountInString(record.Dashboard.CurrentTask)
	noteCursor := len([]rune(record.Dashboard.Notes))
	mode := modeView
	prompt := todoPrompt{}
	helpVisible := false

	persist := func() {
		fresh, err := loadRegistry()
		if err != nil {
			return
		}
		record.UpdatedAt = time.Now()
		fresh.Agents[record.ID] = record
		_ = saveRegistry(fresh)
	}

	ensureTodoIndex := func() {
		if len(record.Dashboard.Todos) == 0 {
			todoIndex = 0
			return
		}
		if todoIndex < 0 {
			todoIndex = 0
		}
		if todoIndex >= len(record.Dashboard.Todos) {
			todoIndex = len(record.Dashboard.Todos) - 1
		}
	}

	draw := func() {
		screen.Clear()
		w, h := screen.Size()
		if w <= 0 || h <= 0 {
			return
		}

		headerStyle := tcell.StyleDefault.Foreground(tcell.ColorLightCyan).Bold(true)
		selectedHeaderStyle := headerStyle.Background(tcell.ColorDarkSlateGray)
		selectedStyle := tcell.StyleDefault.Background(tcell.ColorDarkSlateGray)
		normalStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite)
		subtleStyle := tcell.StyleDefault.Foreground(tcell.ColorLightSlateGray)
		actionStyle := tcell.StyleDefault.Foreground(tcell.ColorLightSalmon)

		colWidths := []int{
			maxInt(18, w*30/100),
			maxInt(24, w*38/100),
			maxInt(18, w*22/100),
		}
		actionsWidth := maxInt(10, w-colWidths[0]-colWidths[1]-colWidths[2]-3)
		used := colWidths[0] + colWidths[1] + colWidths[2] + actionsWidth + 3
		if used > w {
			actionsWidth -= used - w
		}
		if actionsWidth < 8 {
			actionsWidth = 8
		}
		contentHeight := h - 1
		if contentHeight < 2 {
			contentHeight = h
		}

		columns := []struct {
			title string
			x     int
			w     int
		}{
			{title: "TODOS", x: 0, w: colWidths[0]},
			{title: "NOTES", x: colWidths[0] + 1, w: colWidths[1]},
			{title: "CURRENT TASK", x: colWidths[0] + colWidths[1] + 2, w: colWidths[2]},
			{title: "ACTIONS", x: colWidths[0] + colWidths[1] + colWidths[2] + 3, w: actionsWidth},
		}

		for idx, col := range columns {
			style := headerStyle
			if idx == selectedCol && mode == modeView {
				style = selectedHeaderStyle
			}
			writeStyledLine(screen, col.x, 0, padRight(truncate(col.title, col.w), col.w), style)
		}
		for _, sepX := range []int{colWidths[0], colWidths[0] + colWidths[1] + 1, colWidths[0] + colWidths[1] + colWidths[2] + 2} {
			if sepX >= 0 && sepX < w {
				for row := 0; row < h; row++ {
					screen.SetContent(sepX, row, '│', nil, subtleStyle)
				}
			}
		}

		for row := 1; row < contentHeight; row++ {
			for x := 0; x < w; x++ {
				screen.SetContent(x, row, ' ', nil, tcell.StyleDefault)
			}
		}

		ensureTodoIndex()
		visibleTodoRows := contentHeight - 1
		if visibleTodoRows < 1 {
			visibleTodoRows = 1
		}
		todoOffset := 0
		if todoIndex >= visibleTodoRows {
			todoOffset = todoIndex - visibleTodoRows + 1
		}
		for row := 0; row < visibleTodoRows; row++ {
			idx := todoOffset + row
			lineStyle := normalStyle
			if selectedCol == 0 && idx == todoIndex && mode == modeView {
				lineStyle = selectedStyle
			}
			text := ""
			if idx < len(record.Dashboard.Todos) {
				box := "[ ]"
				if record.Dashboard.Todos[idx].Done {
					box = "[x]"
				}
				text = box + " " + record.Dashboard.Todos[idx].Title
			} else if len(record.Dashboard.Todos) == 0 && row == 0 {
				text = cfg.Keys.AddTodo + " add todo"
				lineStyle = subtleStyle
			}
			writeStyledLine(screen, columns[0].x, row+1, padRight(truncate(text, columns[0].w), columns[0].w), lineStyle)
		}

		noteLines := strings.Split(record.Dashboard.Notes, "\n")
		if len(noteLines) == 0 {
			noteLines = []string{""}
		}
		if noteScroll < 0 {
			noteScroll = 0
		}
		maxScroll := maxInt(0, len(noteLines)-visibleTodoRows)
		if noteScroll > maxScroll {
			noteScroll = maxScroll
		}
		for row := 0; row < visibleTodoRows; row++ {
			idx := noteScroll + row
			style := normalStyle
			if selectedCol == 1 && mode == modeView {
				style = selectedStyle
			}
			line := ""
			if idx < len(noteLines) {
				line = noteLines[idx]
			} else if record.Dashboard.Notes == "" && row == 0 {
				line = "Enter to edit notes"
				style = subtleStyle
			}
			if selectedCol == 1 && mode == modeEditNotes {
				style = selectedStyle
			}
			writeStyledLine(screen, columns[1].x, row+1, padRight(truncate(line, columns[1].w), columns[1].w), style)
		}

		taskLines := wrapText(record.Dashboard.CurrentTask, maxInt(1, columns[2].w))
		if len(taskLines) == 0 {
			taskLines = []string{""}
		}
		for row := 0; row < visibleTodoRows; row++ {
			style := normalStyle
			if selectedCol == 2 && mode == modeView {
				style = selectedStyle
			}
			if selectedCol == 2 && mode == modeEditTask {
				style = selectedStyle
			}
			line := ""
			if row < len(taskLines) {
				line = taskLines[row]
			} else if record.Dashboard.CurrentTask == "" && row == 0 {
				line = "Enter to set current task"
				style = subtleStyle
			}
			writeStyledLine(screen, columns[2].x, row+1, padRight(truncate(line, columns[2].w), columns[2].w), style)
		}

		actionLines := []string{"[D] Destroy"}
		for row := 0; row < visibleTodoRows; row++ {
			style := actionStyle
			if selectedCol == 3 && mode == modeView {
				style = selectedStyle.Foreground(tcell.ColorLightSalmon)
			}
			line := ""
			if row < len(actionLines) {
				line = actionLines[row]
			}
			writeStyledLine(screen, columns[3].x, row+1, padRight(truncate(line, columns[3].w), columns[3].w), style)
		}

		if helpVisible && h > 1 {
			help := fmt.Sprintf("%s/%s/%s/%s move  %s edit  %s cancel  %s add todo  %s toggle  %s destroy", cfg.Keys.MoveLeft, cfg.Keys.MoveRight, cfg.Keys.MoveUp, cfg.Keys.MoveDown, cfg.Keys.Edit, cfg.Keys.Cancel, cfg.Keys.AddTodo, cfg.Keys.ToggleTodo, cfg.Keys.Destroy)
			writeStyledLine(screen, 0, h-1, padRight(truncate(help, w), w), subtleStyle)
		}

		if prompt.active {
			drawPopup(screen, w, h, promptTitle(prompt.mode), string(prompt.text), prompt.cursor)
		}
		if mode == modeConfirmDestroy {
			drawPopup(screen, w, h, "Destroy agent", "Destroy this agent? [y/N]", len([]rune("Destroy this agent? [y/N]")))
		}

		if selectedCol == 1 && mode == modeEditNotes {
			line, col := cursorLineCol(record.Dashboard.Notes, noteCursor)
			displayLine := line - noteScroll + 1
			if displayLine >= 1 && displayLine < h {
				x := columns[1].x + minInt(col, columns[1].w-1)
				screen.ShowCursor(x, displayLine)
			}
		} else if selectedCol == 2 && mode == modeEditTask {
			screen.ShowCursor(columns[2].x+minInt(taskCursor, columns[2].w-1), 1)
		} else {
			screen.HideCursor()
		}
		screen.Show()
	}

	draw()
	for {
		ev := screen.PollEvent()
		switch tev := ev.(type) {
		case *tcell.EventResize:
			screen.Sync()
			draw()
		case *tcell.EventKey:
			if matchesKey(tev, cfg.Keys.Help) {
				helpVisible = !helpVisible
				draw()
				continue
			}

			if prompt.active {
				handled := handleSingleLineEditKey(tev, &prompt.text, &prompt.cursor, true)
				if tev.Key() == tcell.KeyEnter {
					text := strings.TrimSpace(string(prompt.text))
					if prompt.mode == todoPromptAdd && text != "" {
						record.Dashboard.Todos = append(record.Dashboard.Todos, todoItem{Title: text})
						todoIndex = len(record.Dashboard.Todos) - 1
						persist()
					} else if prompt.mode == todoPromptEdit && prompt.index >= 0 && prompt.index < len(record.Dashboard.Todos) {
						record.Dashboard.Todos[prompt.index].Title = text
						persist()
					}
					prompt.active = false
					draw()
					continue
				}
				if tev.Key() == tcell.KeyEscape {
					prompt.active = false
					draw()
					continue
				}
				if handled {
					draw()
					continue
				}
			}

			if mode == modeConfirmDestroy {
				if tev.Key() == tcell.KeyEscape || matchesKey(tev, cfg.Keys.Back) {
					mode = modeView
					draw()
					continue
				}
				if matchesKey(tev, cfg.Keys.Confirm) {
					screen.Fini()
					return runDestroy([]string{"--id", record.ID})
				}
				if tev.Key() == tcell.KeyRune {
					mode = modeView
					draw()
				}
				continue
			}

			switch mode {
			case modeEditNotes:
				if tev.Key() == tcell.KeyEscape || matchesKey(tev, cfg.Keys.Cancel) {
					mode = modeView
					draw()
					continue
				}
				if handleMultilineEditKey(tev, &record.Dashboard.Notes, &noteCursor) {
					persist()
					draw()
				}
				continue
			case modeEditTask:
				if tev.Key() == tcell.KeyEscape || matchesKey(tev, cfg.Keys.Cancel) {
					mode = modeView
					draw()
					continue
				}
				if tev.Key() == tcell.KeyEnter {
					mode = modeView
					persist()
					draw()
					continue
				}
				if handleSingleLineEditKey(tev, (*[]rune)(nil), nil, false) {
				}
				runes := []rune(record.Dashboard.CurrentTask)
				if handleSingleLineEditKey(tev, &runes, &taskCursor, false) {
					record.Dashboard.CurrentTask = string(runes)
					persist()
					draw()
				}
				continue
			}

			if tev.Key() == tcell.KeyCtrlC {
				continue
			}
			if matchesKey(tev, cfg.Keys.MoveLeft) {
				selectedCol = maxInt(0, selectedCol-1)
				draw()
				continue
			}
			if matchesKey(tev, cfg.Keys.MoveRight) {
				selectedCol = minInt(3, selectedCol+1)
				draw()
				continue
			}
			switch selectedCol {
			case 0:
				if matchesKey(tev, cfg.Keys.MoveUp) {
					todoIndex--
					ensureTodoIndex()
					draw()
					continue
				}
				if matchesKey(tev, cfg.Keys.MoveDown) {
					todoIndex++
					ensureTodoIndex()
					draw()
					continue
				}
				if matchesKey(tev, cfg.Keys.AddTodo) {
					prompt = todoPrompt{active: true, mode: todoPromptAdd, text: []rune{}, cursor: 0, index: -1}
					draw()
					continue
				}
				if matchesKey(tev, cfg.Keys.ToggleTodo) && len(record.Dashboard.Todos) > 0 {
					record.Dashboard.Todos[todoIndex].Done = !record.Dashboard.Todos[todoIndex].Done
					persist()
					draw()
					continue
				}
				if matchesKey(tev, cfg.Keys.DeleteTodo) && len(record.Dashboard.Todos) > 0 {
					record.Dashboard.Todos = append(record.Dashboard.Todos[:todoIndex], record.Dashboard.Todos[todoIndex+1:]...)
					ensureTodoIndex()
					persist()
					draw()
					continue
				}
				if tev.Key() == tcell.KeyEnter && len(record.Dashboard.Todos) > 0 {
					text := []rune(record.Dashboard.Todos[todoIndex].Title)
					prompt = todoPrompt{active: true, mode: todoPromptEdit, text: text, cursor: len(text), index: todoIndex}
					draw()
					continue
				}
			case 1:
				if matchesKey(tev, cfg.Keys.MoveUp) {
					noteScroll--
					draw()
					continue
				}
				if matchesKey(tev, cfg.Keys.MoveDown) {
					noteScroll++
					draw()
					continue
				}
				if tev.Key() == tcell.KeyEnter || matchesKey(tev, cfg.Keys.Edit) {
					mode = modeEditNotes
					noteCursor = len([]rune(record.Dashboard.Notes))
					draw()
					continue
				}
			case 2:
				if tev.Key() == tcell.KeyEnter || matchesKey(tev, cfg.Keys.Edit) {
					mode = modeEditTask
					taskCursor = len([]rune(record.Dashboard.CurrentTask))
					draw()
					continue
				}
			case 3:
				if matchesKey(tev, cfg.Keys.Destroy) || tev.Key() == tcell.KeyEnter {
					mode = modeConfirmDestroy
					draw()
					continue
				}
			}
			if matchesKey(tev, cfg.Keys.Destroy) {
				mode = modeConfirmDestroy
				draw()
			}
		}
	}
}

func promptTitle(mode todoPromptMode) string {
	if mode == todoPromptAdd {
		return "Add todo"
	}
	return "Edit todo"
}

func drawPopup(screen tcell.Screen, width, height int, title, text string, cursor int) {
	boxW := minInt(maxInt(24, width*60/100), width-4)
	boxH := 5
	boxX := (width - boxW) / 2
	boxY := maxInt(1, (height-boxH)/2)
	style := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	border := tcell.StyleDefault.Foreground(tcell.ColorLightCyan)
	for y := 0; y < boxH; y++ {
		for x := 0; x < boxW; x++ {
			ch := ' '
			st := style
			if y == 0 || y == boxH-1 {
				ch = '─'
				st = border
			}
			if x == 0 || x == boxW-1 {
				ch = '│'
				st = border
			}
			if (x == 0 || x == boxW-1) && (y == 0 || y == boxH-1) {
				switch {
				case x == 0 && y == 0:
					ch = '┌'
				case x == boxW-1 && y == 0:
					ch = '┐'
				case x == 0 && y == boxH-1:
					ch = '└'
				default:
					ch = '┘'
				}
			}
			screen.SetContent(boxX+x, boxY+y, ch, nil, st)
		}
	}
	writeStyledLine(screen, boxX+2, boxY+1, truncate(title, boxW-4), border.Bold(true))
	writeStyledLine(screen, boxX+2, boxY+2, padRight(truncate(text, boxW-4), boxW-4), style)
	screen.ShowCursor(boxX+2+minInt(cursor, boxW-5), boxY+2)
}

func handleSingleLineEditKey(ev *tcell.EventKey, text *[]rune, cursor *int, allowEnter bool) bool {
	if text == nil || cursor == nil {
		return false
	}
	switch ev.Key() {
	case tcell.KeyLeft:
		if *cursor > 0 {
			*cursor = *cursor - 1
		}
		return true
	case tcell.KeyRight:
		if *cursor < len(*text) {
			*cursor = *cursor + 1
		}
		return true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if *cursor > 0 {
			*text = append((*text)[:*cursor-1], (*text)[*cursor:]...)
			*cursor = *cursor - 1
		}
		return true
	case tcell.KeyCtrlA:
		*cursor = 0
		return true
	case tcell.KeyCtrlE:
		*cursor = len(*text)
		return true
	case tcell.KeyCtrlU:
		*text = (*text)[*cursor:]
		*cursor = 0
		return true
	case tcell.KeyCtrlW:
		start := previousWordBoundary(*text, *cursor)
		*text = append((*text)[:start], (*text)[*cursor:]...)
		*cursor = start
		return true
	case tcell.KeyRune:
		r := ev.Rune()
		*text = append((*text)[:*cursor], append([]rune{r}, (*text)[*cursor:]...)...)
		*cursor = *cursor + 1
		return true
	case tcell.KeyEnter:
		return allowEnter
	default:
		return false
	}
}

func handleMultilineEditKey(ev *tcell.EventKey, value *string, cursor *int) bool {
	runes := []rune(*value)
	switch ev.Key() {
	case tcell.KeyLeft:
		if *cursor > 0 {
			*cursor = *cursor - 1
		}
	case tcell.KeyRight:
		if *cursor < len(runes) {
			*cursor = *cursor + 1
		}
	case tcell.KeyUp:
		line, col := cursorLineCol(*value, *cursor)
		if line > 0 {
			*cursor = lineColToIndex(*value, line-1, col)
		}
	case tcell.KeyDown:
		line, col := cursorLineCol(*value, *cursor)
		*cursor = lineColToIndex(*value, line+1, col)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if *cursor > 0 {
			runes = append(runes[:*cursor-1], runes[*cursor:]...)
			*cursor = *cursor - 1
		}
	case tcell.KeyEnter:
		runes = append(runes[:*cursor], append([]rune{'\n'}, runes[*cursor:]...)...)
		*cursor = *cursor + 1
	case tcell.KeyCtrlA:
		line, _ := cursorLineCol(*value, *cursor)
		*cursor = lineColToIndex(*value, line, 0)
	case tcell.KeyCtrlE:
		line, _ := cursorLineCol(*value, *cursor)
		lines := strings.Split(*value, "\n")
		if line >= len(lines) {
			line = len(lines) - 1
		}
		if line < 0 {
			line = 0
		}
		*cursor = lineColToIndex(*value, line, len([]rune(lines[line])))
	case tcell.KeyCtrlU:
		line, _ := cursorLineCol(*value, *cursor)
		start := lineColToIndex(*value, line, 0)
		runes = append(runes[:start], runes[*cursor:]...)
		*cursor = start
	case tcell.KeyCtrlW:
		start := previousWordBoundary(runes, *cursor)
		runes = append(runes[:start], runes[*cursor:]...)
		*cursor = start
	case tcell.KeyRune:
		r := ev.Rune()
		runes = append(runes[:*cursor], append([]rune{r}, runes[*cursor:]...)...)
		*cursor = *cursor + 1
	default:
		return false
	}
	*value = string(runes)
	return true
}

func previousWordBoundary(runes []rune, cursor int) int {
	i := cursor
	for i > 0 && unicode.IsSpace(runes[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(runes[i-1]) {
		i--
	}
	return i
}

func cursorLineCol(value string, cursor int) (line, col int) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	for i := 0; i < cursor; i++ {
		if runes[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return line, col
}

func lineColToIndex(value string, wantLine, wantCol int) int {
	lines := strings.Split(value, "\n")
	if wantLine < 0 {
		wantLine = 0
	}
	if wantLine >= len(lines) {
		wantLine = len(lines) - 1
	}
	if wantLine < 0 {
		return 0
	}
	idx := 0
	for i := 0; i < wantLine; i++ {
		idx += len([]rune(lines[i])) + 1
	}
	lineRunes := []rune(lines[wantLine])
	if wantCol > len(lineRunes) {
		wantCol = len(lineRunes)
	}
	if wantCol < 0 {
		wantCol = 0
	}
	return idx + wantCol
}

func matchesKey(ev *tcell.EventKey, binding string) bool {
	binding = strings.TrimSpace(binding)
	if binding == "" {
		return false
	}
	switch strings.ToLower(binding) {
	case "enter":
		return ev.Key() == tcell.KeyEnter
	case "escape", "esc":
		return ev.Key() == tcell.KeyEscape
	case "space":
		return ev.Key() == tcell.KeyRune && ev.Rune() == ' '
	default:
		if len([]rune(binding)) == 1 {
			return ev.Key() == tcell.KeyRune && unicode.ToLower(ev.Rune()) == unicode.ToLower([]rune(binding)[0])
		}
		return false
	}
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	if text == "" {
		return []string{""}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		candidate := current + " " + word
		if len([]rune(candidate)) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = word
	}
	lines = append(lines, current)
	return lines
}

func truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width == 1 {
		return string(runes[:1])
	}
	return string(runes[:width-1]) + "…"
}

func writeStyledLine(s tcell.Screen, x, y int, text string, style tcell.Style) {
	for idx, r := range []rune(text) {
		s.SetContent(x+idx, y, r, nil, style)
	}
}

func padRight(text string, width int) string {
	count := len([]rune(text))
	if count >= width {
		return text
	}
	return text + strings.Repeat(" ", width-count)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
