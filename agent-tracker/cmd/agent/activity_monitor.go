package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var errClosePalette = errors.New("close palette")

type activitySortKey int

const (
	activitySortCPU activitySortKey = iota
	activitySortMemory
	activitySortDownload
	activitySortUpload
	activitySortPorts
	activitySortLocation
	activitySortCommand
)

var activitySortKeys = []activitySortKey{
	activitySortCPU,
	activitySortMemory,
	activitySortDownload,
	activitySortUpload,
	activitySortCommand,
	activitySortLocation,
	activitySortPorts,
}

type activitySnapshot struct {
	Processes       map[int]*activityProcess
	MemoryByPID     map[int]activityMemory
	NetworkByPID    map[int]activityNetwork
	PortsByPID      map[int][]string
	TmuxByPanePID   map[int]*activityTmuxLocation
	LastProcessLoad time.Time
	LastMemoryLoad  time.Time
	LastNetworkLoad time.Time
	LastPortLoad    time.Time
	LastTmuxLoad    time.Time
	RefreshedAt     time.Time
	Status          string
}

type activityRefreshInput struct {
	Force           bool
	Initial         bool
	ShowAll         bool
	Processes       map[int]*activityProcess
	MemoryByPID     map[int]activityMemory
	NetworkByPID    map[int]activityNetwork
	PortsByPID      map[int][]string
	TmuxByPanePID   map[int]*activityTmuxLocation
	LastProcessLoad time.Time
	LastMemoryLoad  time.Time
	LastNetworkLoad time.Time
	LastPortLoad    time.Time
	LastTmuxLoad    time.Time
	RefreshedAt     time.Time
}

type activityMemory struct {
	ResidentMB   float64
	CompressedMB float64
}

type activityNetwork struct {
	PID      int
	BytesIn  int64
	BytesOut int64
	DownKBps float64
	UpKBps   float64
}

type activityTmuxLocation struct {
	SessionID   string
	SessionName string
	WindowID    string
	WindowIndex string
	WindowName  string
	PaneID      string
	PaneIndex   string
	RootPID     int
}

type activityProcess struct {
	PID          int
	PPID         int
	CPU          float64
	RSSKB        int64
	ResidentMB   float64
	CompressedMB float64
	BytesIn      int64
	BytesOut     int64
	DownKBps     float64
	UpKBps       float64
	State        string
	Elapsed      string
	Command      string
	ShortCommand string
	Ports        []string
	Tmux         *activityTmuxLocation
	Parent       *activityProcess
	Children     []*activityProcess
}

type activityRow struct {
	PID int
}

func collectActivitySnapshot(input activityRefreshInput) (*activitySnapshot, error) {
	now := time.Now()
	snapshot := &activitySnapshot{
		Processes:       cloneActivityProcesses(input.Processes),
		MemoryByPID:     cloneActivityMemoryMap(input.MemoryByPID),
		NetworkByPID:    cloneActivityNetworkMap(input.NetworkByPID),
		PortsByPID:      cloneActivityPortsMap(input.PortsByPID),
		TmuxByPanePID:   cloneActivityTmuxMap(input.TmuxByPanePID),
		LastProcessLoad: input.LastProcessLoad,
		LastMemoryLoad:  input.LastMemoryLoad,
		LastNetworkLoad: input.LastNetworkLoad,
		LastPortLoad:    input.LastPortLoad,
		LastTmuxLoad:    input.LastTmuxLoad,
		RefreshedAt:     input.RefreshedAt,
	}
	nonFatal := []string{}
	needProcessLoad := input.Initial || input.Force || now.Sub(input.LastProcessLoad) >= time.Second
	if input.Initial || input.Force || now.Sub(input.LastTmuxLoad) >= 4*time.Second {
		values, err := loadActivityTmuxPanes()
		if err != nil {
			nonFatal = append(nonFatal, "tmux data unavailable")
		} else {
			snapshot.TmuxByPanePID = values
			snapshot.LastTmuxLoad = now
			needProcessLoad = true
		}
	}
	if input.Initial || input.Force || now.Sub(input.LastMemoryLoad) >= 5*time.Second {
		values, err := loadActivityMemory()
		if err != nil {
			nonFatal = append(nonFatal, "memory data unavailable")
		} else {
			snapshot.MemoryByPID = values
			snapshot.LastMemoryLoad = now
			needProcessLoad = true
		}
	}
	if input.Initial || input.Force || now.Sub(input.LastNetworkLoad) >= 2*time.Second {
		values, err := loadActivityNetwork(snapshot.NetworkByPID, input.LastNetworkLoad, now)
		if err != nil {
			nonFatal = append(nonFatal, "network data unavailable")
		} else {
			snapshot.NetworkByPID = values
			snapshot.LastNetworkLoad = now
			needProcessLoad = true
		}
	}
	if needProcessLoad {
		processes, err := loadActivityProcesses(snapshot.MemoryByPID, snapshot.TmuxByPanePID)
		if err != nil {
			return nil, err
		}
		snapshot.Processes = processes
		snapshot.LastProcessLoad = now
		snapshot.RefreshedAt = now
	}
	applyActivityNetwork(snapshot.Processes, snapshot.NetworkByPID)
	applyActivityPorts(snapshot.Processes, snapshot.PortsByPID)
	if !input.Initial && (input.Force || now.Sub(input.LastPortLoad) >= 5*time.Second) {
		values, err := loadActivityPorts(activityPortTargetPIDs(snapshot.Processes, input.ShowAll))
		if err != nil {
			nonFatal = append(nonFatal, "port data unavailable")
		} else {
			snapshot.PortsByPID = values
			snapshot.LastPortLoad = now
			applyActivityPorts(snapshot.Processes, values)
		}
	}
	if len(nonFatal) > 0 {
		snapshot.Status = strings.Join(nonFatal, "; ")
	}
	return snapshot, nil
}

func cloneActivityMemoryMap(values map[int]activityMemory) map[int]activityMemory {
	cloned := make(map[int]activityMemory, len(values))
	for pid, value := range values {
		cloned[pid] = value
	}
	return cloned
}

func cloneActivityNetworkMap(values map[int]activityNetwork) map[int]activityNetwork {
	cloned := make(map[int]activityNetwork, len(values))
	for pid, value := range values {
		cloned[pid] = value
	}
	return cloned
}

func cloneActivityPortsMap(values map[int][]string) map[int][]string {
	cloned := make(map[int][]string, len(values))
	for pid, ports := range values {
		cloned[pid] = append([]string(nil), ports...)
	}
	return cloned
}

func cloneActivityTmuxMap(values map[int]*activityTmuxLocation) map[int]*activityTmuxLocation {
	cloned := make(map[int]*activityTmuxLocation, len(values))
	for pid, value := range values {
		if value == nil {
			continue
		}
		copyValue := *value
		cloned[pid] = &copyValue
	}
	return cloned
}

func cloneActivityProcesses(values map[int]*activityProcess) map[int]*activityProcess {
	cloned := make(map[int]*activityProcess, len(values))
	for pid, proc := range values {
		if proc == nil {
			continue
		}
		copyProc := *proc
		copyProc.Parent = nil
		copyProc.Children = nil
		copyProc.Ports = append([]string(nil), proc.Ports...)
		cloned[pid] = &copyProc
	}
	return cloned
}

func activityPortTargetPIDs(processes map[int]*activityProcess, showAll bool) []int {
	pids := make([]int, 0, len(processes))
	for pid, proc := range processes {
		if proc == nil {
			continue
		}
		if !showAll && proc.Tmux == nil {
			continue
		}
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	return pids
}

func loadActivityProcesses(memoryByPID map[int]activityMemory, tmuxByPanePID map[int]*activityTmuxLocation) (map[int]*activityProcess, error) {
	out, err := runActivityCommand(2*time.Second, "ps", "axww", "-o", "pid=", "-o", "ppid=", "-o", "%cpu=", "-o", "rss=", "-o", "state=", "-o", "etime=", "-o", "command=")
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err
	}
	processes := map[int]*activityProcess{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 7 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		if pid == 1 {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		rss, _ := strconv.ParseInt(fields[3], 10, 64)
		command := strings.Join(fields[6:], " ")
		short := activityShortCommand(command)
		memory := memoryByPID[pid]
		processes[pid] = &activityProcess{
			PID:          pid,
			PPID:         ppid,
			CPU:          cpu,
			RSSKB:        rss,
			ResidentMB:   memory.ResidentMB,
			CompressedMB: memory.CompressedMB,
			State:        fields[4],
			Elapsed:      fields[5],
			Command:      command,
			ShortCommand: short,
		}
	}
	for _, proc := range processes {
		if parent := processes[proc.PPID]; parent != nil && parent.PID != proc.PID {
			proc.Parent = parent
			parent.Children = append(parent.Children, proc)
		}
	}
	assignActivityTmuxLocations(processes, tmuxByPanePID)
	return processes, nil
}

func applyActivityPorts(processes map[int]*activityProcess, portsByPID map[int][]string) {
	for pid, proc := range processes {
		proc.Ports = append([]string(nil), portsByPID[pid]...)
	}
}

func applyActivityNetwork(processes map[int]*activityProcess, networkByPID map[int]activityNetwork) {
	for pid, proc := range processes {
		network := networkByPID[pid]
		proc.BytesIn = network.BytesIn
		proc.BytesOut = network.BytesOut
		proc.DownKBps = network.DownKBps
		proc.UpKBps = network.UpKBps
	}
}

func assignActivityTmuxLocations(processes map[int]*activityProcess, tmuxByPanePID map[int]*activityTmuxLocation) {
	panePIDs := make([]int, 0, len(tmuxByPanePID))
	for pid := range tmuxByPanePID {
		panePIDs = append(panePIDs, pid)
	}
	sort.Ints(panePIDs)
	var assign func(int, *activityTmuxLocation)
	assign = func(pid int, loc *activityTmuxLocation) {
		proc := processes[pid]
		if proc == nil || proc.Tmux != nil {
			return
		}
		proc.Tmux = loc
		for _, child := range proc.Children {
			assign(child.PID, loc)
		}
	}
	for _, pid := range panePIDs {
		assign(pid, tmuxByPanePID[pid])
	}
}

func loadActivityMemory() (map[int]activityMemory, error) {
	out, err := runActivityCommand(2500*time.Millisecond, "top", "-l", "1", "-o", "mem", "-n", "500", "-stats", "pid,mem,cmprs")
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err
	}
	values := map[int]activityMemory{}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		values[pid] = activityMemory{
			ResidentMB:   parseActivityMemoryMB(fields[1]),
			CompressedMB: parseActivityMemoryMB(fields[2]),
		}
	}
	if len(values) == 0 && err != nil {
		return nil, err
	}
	return values, nil
}

func loadActivityTmuxPanes() (map[int]*activityTmuxLocation, error) {
	out, err := runActivityCommand(1500*time.Millisecond, "tmux", "list-panes", "-a", "-F", "#{session_id}\t#{session_name}\t#{window_id}\t#{window_index}\t#{window_name}\t#{pane_id}\t#{pane_index}\t#{pane_pid}")
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err
	}
	values := map[int]*activityTmuxLocation{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "\t")
		if len(parts) != 8 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(parts[7]))
		if err != nil || pid <= 0 {
			continue
		}
		values[pid] = &activityTmuxLocation{
			SessionID:   strings.TrimSpace(parts[0]),
			SessionName: strings.TrimSpace(parts[1]),
			WindowID:    strings.TrimSpace(parts[2]),
			WindowIndex: strings.TrimSpace(parts[3]),
			WindowName:  strings.TrimSpace(parts[4]),
			PaneID:      strings.TrimSpace(parts[5]),
			PaneIndex:   strings.TrimSpace(parts[6]),
			RootPID:     pid,
		}
	}
	return values, nil
}

func loadActivityPorts(pids []int) (map[int][]string, error) {
	values := map[int][]string{}
	if len(pids) == 0 {
		return values, nil
	}
	pidList := make([]string, 0, len(pids))
	for _, pid := range pids {
		if pid > 0 {
			pidList = append(pidList, strconv.Itoa(pid))
		}
	}
	if len(pidList) == 0 {
		return values, nil
	}
	commands := [][]string{
		{"-nP", "-a", "-p", strings.Join(pidList, ","), "-iTCP", "-sTCP:LISTEN", "-F", "pn"},
		{"-nP", "-a", "-p", strings.Join(pidList, ","), "-iUDP", "-F", "pn"},
	}
	seen := map[int]map[string]bool{}
	for _, args := range commands {
		out, err := runActivityCommand(1800*time.Millisecond, "lsof", args...)
		if err != nil && strings.TrimSpace(out) == "" {
			continue
		}
		currentPID := 0
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			switch line[0] {
			case 'p':
				pid, err := strconv.Atoi(strings.TrimSpace(line[1:]))
				if err != nil || pid <= 0 {
					currentPID = 0
					continue
				}
				currentPID = pid
			case 'n':
				if currentPID <= 0 {
					continue
				}
				label := normalizeActivityPort(strings.TrimSpace(line[1:]))
				if label == "" {
					continue
				}
				if seen[currentPID] == nil {
					seen[currentPID] = map[string]bool{}
				}
				if seen[currentPID][label] {
					continue
				}
				seen[currentPID][label] = true
				values[currentPID] = append(values[currentPID], label)
			}
		}
	}
	for pid := range values {
		sort.Strings(values[pid])
	}
	return values, nil
}

func loadActivityNetwork(previous map[int]activityNetwork, lastLoad, now time.Time) (map[int]activityNetwork, error) {
	out, err := runActivityCommand(2*time.Second, "nettop", "-P", "-L", "1", "-x", "-J", "bytes_in,bytes_out")
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, err
	}
	values := map[int]activityNetwork{}
	elapsed := now.Sub(lastLoad).Seconds()
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ",bytes_in,") {
			continue
		}
		current, ok := parseActivityNetworkLine(line)
		if !ok {
			continue
		}
		if previousValue, ok := previous[current.PID]; ok && elapsed > 0 {
			if current.BytesIn >= previousValue.BytesIn {
				current.DownKBps = float64(current.BytesIn-previousValue.BytesIn) / 1024.0 / elapsed
			}
			if current.BytesOut >= previousValue.BytesOut {
				current.UpKBps = float64(current.BytesOut-previousValue.BytesOut) / 1024.0 / elapsed
			}
		}
		values[current.PID] = current
	}
	if len(values) == 0 && err != nil {
		return nil, err
	}
	return values, nil
}

func parseActivityNetworkLine(line string) (activityNetwork, bool) {
	parts := strings.Split(strings.TrimSpace(line), ",")
	if len(parts) < 3 {
		return activityNetwork{}, false
	}
	processField := strings.TrimSpace(parts[0])
	lastDot := strings.LastIndex(processField, ".")
	if lastDot <= 0 || lastDot >= len(processField)-1 {
		return activityNetwork{}, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(processField[lastDot+1:]))
	if err != nil || pid <= 0 {
		return activityNetwork{}, false
	}
	bytesIn, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return activityNetwork{}, false
	}
	bytesOut, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
	if err != nil {
		return activityNetwork{}, false
	}
	return activityNetwork{BytesIn: bytesIn, BytesOut: bytesOut, PID: pid}, true
}

func focusActivityTmuxLocation(loc *activityTmuxLocation) error {
	if loc == nil {
		return fmt.Errorf("process is not in tmux")
	}
	if strings.TrimSpace(loc.SessionID) != "" {
		if err := runTmux("switch-client", "-t", loc.SessionID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(loc.WindowID) != "" {
		if err := runTmux("select-window", "-t", loc.WindowID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(loc.PaneID) != "" {
		if err := runTmux("select-pane", "-t", loc.PaneID); err != nil {
			return err
		}
	}
	return nil
}

func runActivityCommand(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("%s timed out", name)
	}
	if err != nil {
		return string(out), fmt.Errorf("%s failed", name)
	}
	return string(out), nil
}

func activityColumnWidths(width int) (int, int, int, int, int, int, int, int) {
	cpuW := 6
	memW := 7
	downW := 9
	upW := 9
	procW := 18
	sessionW := 10
	windowW := 12
	portW := 10
	procW = width - cpuW - memW - downW - upW - sessionW - windowW - portW - 7
	for procW < 18 && windowW > 12 {
		windowW--
		procW++
	}
	for procW < 18 && sessionW > 10 {
		sessionW--
		procW++
	}
	for procW < 18 && downW > 7 {
		downW--
		procW++
	}
	for procW < 18 && upW > 7 {
		upW--
		procW++
	}
	for procW < 18 && portW > 8 {
		portW--
		procW++
	}
	for procW < 18 && memW > 6 {
		memW--
		procW++
	}
	if procW < 12 {
		procW = 12
	}
	return cpuW, memW, downW, upW, procW, sessionW, windowW, portW
}

func formatActivitySpeed(kbps float64) string {
	if kbps <= 0.05 {
		return "-"
	}
	if kbps >= 1024 {
		return fmt.Sprintf("%.1fM", kbps/1024)
	}
	if kbps >= 100 {
		return fmt.Sprintf("%.0fK", kbps)
	}
	if kbps >= 10 {
		return fmt.Sprintf("%.1fK", kbps)
	}
	return fmt.Sprintf("%.2fK", kbps)
}

func activitySessionLabel(loc *activityTmuxLocation) string {
	if loc == nil {
		return "-"
	}
	return blankIfEmpty(loc.SessionName, loc.SessionID)
}

func activityWindowLabel(loc *activityTmuxLocation, currentWindowID string) string {
	if loc == nil {
		return "-"
	}
	label := blankIfEmpty(loc.WindowIndex, "?")
	if strings.TrimSpace(loc.WindowName) != "" {
		label += " " + loc.WindowName
	}
	if currentWindowID != "" && loc.WindowID == currentWindowID {
		return "*" + label
	}
	return label
}

func activityProcessLabel(proc *activityProcess) string {
	return blankIfEmpty(proc.ShortCommand, proc.Command)
}

func activityShortCommand(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return "unknown"
	}
	base := filepath.Base(fields[0])
	if strings.TrimSpace(base) == "" {
		return fields[0]
	}
	return base
}

func formatActivityMB(mb float64) string {
	if mb <= 0 {
		return "0M"
	}
	if mb >= 100 {
		gb := mb / 1024
		if gb >= 10 {
			return fmt.Sprintf("%.0fG", gb)
		}
		return fmt.Sprintf("%.1fG", gb)
	}
	if mb >= 10 {
		return fmt.Sprintf("%.0fM", mb)
	}
	return fmt.Sprintf("%.1fM", mb)
}

func formatActivityPorts(ports []string) string {
	if len(ports) == 0 {
		return "-"
	}
	if len(ports) == 1 {
		return ports[0]
	}
	if len(ports) == 2 {
		return ports[0] + "," + ports[1]
	}
	return fmt.Sprintf("%s,%s+%d", ports[0], ports[1], len(ports)-2)
}

func normalizeActivityPort(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Split(value, "->")[0]
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	value = fields[0]
	if idx := strings.LastIndex(value, "]:"); idx >= 0 {
		return value[idx+2:]
	}
	if idx := strings.LastIndex(value, ":"); idx >= 0 && idx+1 < len(value) {
		value = value[idx+1:]
	}
	if value == "" || value == "*" {
		return ""
	}
	return value
}

func parseActivityMemoryMB(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return 0
	}
	unit := byte('M')
	last := value[len(value)-1]
	if (last >= 'A' && last <= 'Z') || (last >= 'a' && last <= 'z') {
		unit = byte(unicode.ToUpper(rune(last)))
		value = strings.TrimSpace(value[:len(value)-1])
	}
	amount, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	switch unit {
	case 'B':
		return amount / 1024 / 1024
	case 'K':
		return amount / 1024
	case 'G':
		return amount * 1024
	case 'T':
		return amount * 1024 * 1024
	default:
		return amount
	}
}

func activityResidentMemoryMB(proc *activityProcess) float64 {
	if proc == nil {
		return 0
	}
	return proc.ResidentMB
}

func activityTotalMemoryMB(proc *activityProcess) float64 {
	if proc == nil {
		return 0
	}
	return activityResidentMemoryMB(proc) + proc.CompressedMB
}

func defaultActivitySortDescending(key activitySortKey) bool {
	switch key {
	case activitySortCPU, activitySortMemory, activitySortDownload, activitySortUpload, activitySortPorts:
		return true
	default:
		return false
	}
}

func (key activitySortKey) label() string {
	switch key {
	case activitySortCPU:
		return "CPU"
	case activitySortMemory:
		return "MEM"
	case activitySortDownload:
		return "DOWN"
	case activitySortUpload:
		return "UP"
	case activitySortPorts:
		return "PORTS"
	case activitySortLocation:
		return "LOCATION"
	default:
		return "PROCESS"
	}
}

func compareActivityLocation(left, right *activityTmuxLocation) int {
	leftSession := strings.ToLower(activitySessionLabel(left))
	rightSession := strings.ToLower(activitySessionLabel(right))
	if cmp := strings.Compare(leftSession, rightSession); cmp != 0 {
		return cmp
	}
	leftIndex, leftOK := strconv.Atoi(strings.TrimSpace(blankIfEmpty(windowIndexOf(left), "")))
	rightIndex, rightOK := strconv.Atoi(strings.TrimSpace(blankIfEmpty(windowIndexOf(right), "")))
	switch {
	case leftOK == nil && rightOK == nil:
		if cmp := compareInt(leftIndex, rightIndex); cmp != 0 {
			return cmp
		}
	default:
		if cmp := strings.Compare(strings.ToLower(windowIndexOf(left)), strings.ToLower(windowIndexOf(right))); cmp != 0 {
			return cmp
		}
	}
	return strings.Compare(strings.ToLower(windowNameOf(left)), strings.ToLower(windowNameOf(right)))
}

func windowIndexOf(loc *activityTmuxLocation) string {
	if loc == nil {
		return ""
	}
	return strings.TrimSpace(loc.WindowIndex)
}

func windowNameOf(loc *activityTmuxLocation) string {
	if loc == nil {
		return ""
	}
	return strings.TrimSpace(loc.WindowName)
}

func compareFloat64(left, right float64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareInt64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func blankIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
