package cron

import (
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidateSchedule(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour).UnixMilli()
	past := now.Add(-time.Hour).UnixMilli()

	tests := []struct {
		name     string
		schedule CronSchedule
		wantErr  bool
	}{
		{name: "at future", schedule: CronSchedule{Kind: "at", AtMS: &future}},
		{name: "at past", schedule: CronSchedule{Kind: "at", AtMS: &past}, wantErr: true},
		{name: "at missing", schedule: CronSchedule{Kind: "at"}, wantErr: true},
		{name: "every valid", schedule: CronSchedule{Kind: "every", EveryMS: int64Ptr(MinEveryIntervalMS)}},
		{name: "every too small", schedule: CronSchedule{Kind: "every", EveryMS: int64Ptr(1000)}, wantErr: true},
		{name: "every missing", schedule: CronSchedule{Kind: "every"}, wantErr: true},
		{name: "cron valid", schedule: CronSchedule{Kind: "cron", Expr: "0 9 * * *"}},
		{name: "cron invalid", schedule: CronSchedule{Kind: "cron", Expr: "not a cron"}, wantErr: true},
		{name: "cron empty", schedule: CronSchedule{Kind: "cron"}, wantErr: true},
		{name: "cron bad tz", schedule: CronSchedule{Kind: "cron", Expr: "0 9 * * *", TZ: "Mars/Olympus"}, wantErr: true},
		{name: "unknown kind", schedule: CronSchedule{Kind: "sometimes"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateSchedule(test.schedule, now)
			if test.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestRunNow_ExecutesJob(t *testing.T) {
	var calls atomic.Int32

	cs := NewCronService(filepath.Join(t.TempDir(), "jobs.json"), func(job *CronJob) (string, error) {
		calls.Add(1)
		return "ok", nil
	})
	if err := cs.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cs.Stop()

	job, err := cs.AddJob("manual", CronSchedule{Kind: "every", EveryMS: int64Ptr(3600_000)}, "run me", "cli", "direct")
	if err != nil {
		t.Fatalf("AddJob() error = %v", err)
	}

	if err := cs.RunNow(job.ID); err != nil {
		t.Fatalf("RunNow() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if stored, ok := cs.GetJob(job.ID); ok && stored.State.LastStatus == "ok" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
	stored, ok := cs.GetJob(job.ID)
	if !ok {
		t.Fatal("job disappeared")
	}
	if stored.State.LastStatus != "ok" {
		t.Fatalf("last status = %q, want ok", stored.State.LastStatus)
	}
	if stored.State.LastRunAtMS == nil {
		t.Fatal("last run time was not recorded")
	}
	if stored.Schedule.Kind != "every" || stored.State.NextRunAtMS == nil {
		t.Fatal("manual run changed the recurring schedule")
	}
}

func TestRunNow_Errors(t *testing.T) {
	cs := NewCronService(filepath.Join(t.TempDir(), "jobs.json"), nil)

	job, err := cs.AddJob("manual", CronSchedule{Kind: "every", EveryMS: int64Ptr(3600_000)}, "run me", "cli", "direct")
	if err != nil {
		t.Fatalf("AddJob() error = %v", err)
	}

	if err := cs.RunNow(job.ID); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("RunNow() before Start error = %v, want ErrNotRunning", err)
	}

	if err := cs.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer cs.Stop()

	if err := cs.RunNow("missing"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("RunNow(missing) error = %v, want ErrJobNotFound", err)
	}
}

func TestListJobs_ReturnsCopies(t *testing.T) {
	cs := NewCronService(filepath.Join(t.TempDir(), "jobs.json"), nil)

	at := time.Now().Add(time.Hour).UnixMilli()
	if _, err := cs.AddJob("copy", CronSchedule{Kind: "at", AtMS: &at}, "msg", "cli", "direct"); err != nil {
		t.Fatalf("AddJob() error = %v", err)
	}

	jobs := cs.ListJobs(true)
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	jobs[0].Name = "mutated"
	jobs[0].Schedule.AtMS = nil

	stored, ok := cs.GetJob(jobs[0].ID)
	if !ok {
		t.Fatal("job not found")
	}
	if stored.Name != "copy" {
		t.Fatalf("stored name = %q, want copy", stored.Name)
	}
	if stored.Schedule.AtMS == nil {
		t.Fatal("stored schedule was mutated through ListJobs")
	}
}
