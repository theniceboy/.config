package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type backgroundJob struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Started int64  `json:"started_ms"`
	Ended   int64  `json:"ended_ms,omitempty"`
	Output  string `json:"output,omitempty"`
}

type jobsStoreFile struct {
	Version int             `json:"version"`
	Jobs    []backgroundJob `json:"jobs"`
}

const (
	jobStatusRunning = "running"
	jobStatusDone    = "done"
	jobStatusError   = "error"

	jobsFinishedKeep  = 30
	jobsFinishedTTL   = time.Hour
	jobsRunningStale  = 30 * time.Minute
)

type jobKindDef struct {
	title string
	run   func() (string, error)
}

var jobKindRegistry = map[string]jobKindDef{
	"clip-pull": {title: "clip pull", run: func() (string, error) { return clipboardSyncPull(defaultClipboardSyncHost) }},
	"clip-push": {title: "clip push", run: func() (string, error) { return clipboardSyncPush(defaultClipboardSyncHost) }},
}

var jobsStorePath = func() string {
	return filepath.Join(os.Getenv("HOME"), ".cache", "agent", "jobs.json")
}

func jobsLogDir() string {
	return filepath.Join(os.Getenv("HOME"), ".cache", "agent", "jobs")
}

func withJobsLock(fn func()) error {
	path := jobsStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lockFile, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()
	fn()
	return nil
}

func readJobsStore() jobsStoreFile {
	data, err := os.ReadFile(jobsStorePath())
	if err != nil {
		return jobsStoreFile{Version: 1}
	}
	var store jobsStoreFile
	if err := json.Unmarshal(data, &store); err != nil {
		return jobsStoreFile{Version: 1}
	}
	if store.Version == 0 {
		store.Version = 1
	}
	return store
}

func writeJobsStore(store jobsStoreFile) error {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tmp := jobsStorePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, jobsStorePath())
}

func pruneJobsLocked(jobs []backgroundJob) []backgroundJob {
	now := time.Now()
	ttlCutoff := now.Add(-jobsFinishedTTL).UnixMilli()
	staleCutoff := now.Add(-jobsRunningStale).UnixMilli()
	kept := make([]backgroundJob, 0, len(jobs))
	finished := 0
	for _, job := range jobs {
		if job.Status == jobStatusRunning && job.Started > 0 && job.Started < staleCutoff {
			job.Status = jobStatusError
			job.Ended = now.UnixMilli()
			job.Output = firstNonEmpty(job.Output, "timed out (stale)")
		}
		switch {
		case job.Status == jobStatusRunning:
			kept = append(kept, job)
		case job.Ended >= ttlCutoff:
			kept = append(kept, job)
			finished++
		}
	}
	if finished > jobsFinishedKeep {
		over := finished - jobsFinishedKeep
		out := make([]backgroundJob, 0, len(kept))
		for _, job := range kept {
			if job.Status != jobStatusRunning && over > 0 {
				over--
				continue
			}
			out = append(out, job)
		}
		return out
	}
	return kept
}

func jobsStartDetached(kind string) (*backgroundJob, error) {
	def, ok := jobKindRegistry[kind]
	if !ok {
		return nil, fmt.Errorf("unknown job kind: %q", kind)
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	job := backgroundJob{
		ID:      fmt.Sprintf("%s-%d", kind, time.Now().UnixMilli()),
		Kind:    kind,
		Title:   def.title,
		Status:  jobStatusRunning,
		Started: time.Now().UnixMilli(),
	}
	if err := withJobsLock(func() {
		store := readJobsStore()
		store.Jobs = append(pruneJobsLocked(store.Jobs), job)
		_ = writeJobsStore(store)
	}); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(jobsLogDir(), 0o755); err != nil {
		return nil, err
	}
	logFile, logErr := os.OpenFile(filepath.Join(jobsLogDir(), job.ID+".log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	cmd := exec.Command(exe, "job", "run", job.ID)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if logErr == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	if err := cmd.Start(); err != nil {
		if logErr == nil {
			logFile.Close()
		}
		_ = withJobsLock(func() { finishJobLocked(job.ID, jobStatusError, err.Error()) })
		return nil, err
	}
	_ = cmd.Process.Release()
	if logErr == nil {
		logFile.Close()
	}
	return &job, nil
}

func runBackgroundJob(id string) error {
	var job backgroundJob
	found := false
	_ = withJobsLock(func() {
		store := readJobsStore()
		for _, candidate := range store.Jobs {
			if candidate.ID == id {
				job = candidate
				found = true
				break
			}
		}
	})
	if !found {
		return fmt.Errorf("job not found: %s", id)
	}
	def, ok := jobKindRegistry[job.Kind]
	if !ok {
		return fmt.Errorf("unknown kind %q for job %s", job.Kind, id)
	}
	output, runErr := def.run()
	if runErr != nil {
		summary := firstNonEmpty(strings.TrimSpace(output), runErr.Error())
		_ = withJobsLock(func() { finishJobLocked(id, jobStatusError, summary) })
		return nil
	}
	summary := strings.TrimSpace(output)
	_ = withJobsLock(func() { finishJobLocked(id, jobStatusDone, summary) })
	return nil
}

func finishJobLocked(id, status, output string) {
	store := readJobsStore()
	now := time.Now().UnixMilli()
	for i := range store.Jobs {
		if store.Jobs[i].ID == id {
			store.Jobs[i].Status = status
			store.Jobs[i].Ended = now
			store.Jobs[i].Output = strings.TrimSpace(output)
			break
		}
	}
	store.Jobs = pruneJobsLocked(store.Jobs)
	_ = writeJobsStore(store)
}

func listBackgroundJobs() []backgroundJob {
	store := readJobsStore()
	jobs := pruneJobsLocked(store.Jobs)
	sort.SliceStable(jobs, func(i, j int) bool {
		ri := jobs[i].Status == jobStatusRunning
		rj := jobs[j].Status == jobStatusRunning
		if ri != rj {
			return ri
		}
		return jobs[i].recentMS() > jobs[j].recentMS()
	})
	return jobs
}

func runningBackgroundJobs() []backgroundJob {
	var out []backgroundJob
	for _, job := range listBackgroundJobs() {
		if job.running() {
			out = append(out, job)
		}
	}
	return out
}

func (j backgroundJob) recentMS() int64 {
	if j.Ended > 0 {
		return j.Ended
	}
	return j.Started
}

func (j backgroundJob) running() bool {
	return j.Status == jobStatusRunning
}

func (j backgroundJob) elapsedLabel(now time.Time) string {
	start := time.UnixMilli(j.Started)
	end := now
	if j.Ended > 0 {
		end = time.UnixMilli(j.Ended)
	}
	d := end.Sub(start)
	if d < 0 {
		d = 0
	}
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%ds", secs/60, secs%60)
}

func jobSpinnerFrame(now time.Time) string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[(now.UnixMilli()/250)%int64(len(frames))]
}

func jobStatusIcon(status string) string {
	switch status {
	case jobStatusRunning:
		return "⏳"
	case jobStatusDone:
		return "✓"
	case jobStatusError:
		return "✗"
	default:
		return "·"
	}
}

func runJobsCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent job <start <kind>|list [--json]|run <id>>")
	}
	switch args[0] {
	case "start":
		if len(args) < 2 {
			return fmt.Errorf("usage: agent job start <kind>")
		}
		job, err := jobsStartDetached(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("%s (%s) started\n", job.ID, job.Title)
		return nil
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: agent job run <id>")
		}
		return runBackgroundJob(args[1])
	case "list":
		asJSON := false
		for _, arg := range args[1:] {
			if arg == "--json" {
				asJSON = true
			}
		}
		jobs := listBackgroundJobs()
		if asJSON {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(jobs)
		}
		now := time.Now()
		if len(jobs) == 0 {
			fmt.Println("no background jobs")
			return nil
		}
		for _, job := range jobs {
			line := fmt.Sprintf("%s %-10s %-8s %-6s %s", job.ID, job.Title, job.Status, job.elapsedLabel(now), strings.TrimSpace(job.Output))
			fmt.Println(strings.TrimRight(line, " "))
		}
		return nil
	default:
		return fmt.Errorf("unknown job subcommand: %q", args[0])
	}
}
