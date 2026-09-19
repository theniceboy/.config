package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func withTempJobsStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jobs.json")
	original := jobsStorePath
	jobsStorePath = func() string { return path }
	t.Cleanup(func() { jobsStorePath = original })
	return path
}

func TestPruneJobsMarksStaleRunning(t *testing.T) {
	withTempJobsStore(t)
	now := time.Now()
	jobs := []backgroundJob{
		{ID: "stale", Kind: "clip-pull", Title: "clip pull", Status: jobStatusRunning, Started: now.Add(-31 * time.Minute).UnixMilli()},
		{ID: "fresh", Kind: "clip-pull", Title: "clip pull", Status: jobStatusRunning, Started: now.Add(-2 * time.Second).UnixMilli()},
		{ID: "old-done", Kind: "clip-pull", Title: "clip pull", Status: jobStatusDone, Started: now.Add(-2 * time.Hour).UnixMilli(), Ended: now.Add(-90 * time.Minute).UnixMilli()},
	}
	pruned := pruneJobsLocked(jobs)
	if len(pruned) != 2 {
		t.Fatalf("expected 2 jobs kept, got %d: %+v", len(pruned), pruned)
	}
	if pruned[0].ID != "stale" || pruned[0].Status != jobStatusError {
		t.Fatalf("expected stale job marked error, got %+v", pruned[0])
	}
	if pruned[1].ID != "fresh" || pruned[1].Status != jobStatusRunning {
		t.Fatalf("expected fresh running kept, got %+v", pruned[1])
	}
}

func TestListBackgroundJobsOrdering(t *testing.T) {
	withTempJobsStore(t)
	now := time.Now()
	store := jobsStoreFile{Version: 1, Jobs: []backgroundJob{
		{ID: "done-old", Status: jobStatusDone, Started: now.Add(-3 * time.Minute).UnixMilli(), Ended: now.Add(-2 * time.Minute).UnixMilli()},
		{ID: "running", Status: jobStatusRunning, Started: now.Add(-4 * time.Second).UnixMilli()},
		{ID: "done-new", Status: jobStatusDone, Started: now.Add(-2 * time.Minute).UnixMilli(), Ended: now.Add(-1 * time.Minute).UnixMilli()},
	}}
	if err := withJobsLock(func() { _ = writeJobsStore(store) }); err != nil {
		t.Fatal(err)
	}
	got := listBackgroundJobs()
	want := []string{"running", "done-new", "done-old"}
	if len(got) != len(want) {
		t.Fatalf("expected %d jobs, got %d: %+v", len(want), len(got), got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("position %d: expected %s, got %s", i, id, got[i].ID)
		}
	}
}

func TestJobsStartDetachedUnknownKind(t *testing.T) {
	withTempJobsStore(t)
	if _, err := jobsStartDetached("nope"); err == nil {
		t.Fatal("expected error for unknown kind")
	} else {
		want := fmt.Sprintf("unknown job kind")
		if len(err.Error()) < len(want) || err.Error()[:len(want)] != want {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}
