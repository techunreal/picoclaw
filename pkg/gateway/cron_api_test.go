package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/cron"
)

func newCronAPITestHandler(t *testing.T) (http.Handler, *cron.CronService, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	svc := cron.NewCronService(filepath.Join(t.TempDir(), "jobs.json"), func(job *cron.CronJob) (string, error) {
		calls.Add(1)
		return "ok", nil
	})
	if err := svc.Start(); err != nil {
		t.Fatalf("CronService.Start() error = %v", err)
	}
	t.Cleanup(svc.Stop)

	return newCronAPIHandler(svc, "test-token", true), svc, &calls
}

func doCronAPIRequest(t *testing.T, handler http.Handler, method, target, token string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
	}

	req := httptest.NewRequest(method, target, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeCronJob(t *testing.T, rec *httptest.ResponseRecorder) cron.CronJob {
	t.Helper()

	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var job cron.CronJob
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return job
}

func TestCronAPI_RequiresToken(t *testing.T) {
	handler, _, _ := newCronAPITestHandler(t)

	rec := doCronAPIRequest(t, handler, http.MethodGet, "/cron/jobs", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	rec = doCronAPIRequest(t, handler, http.MethodGet, "/cron/jobs", "wrong-token", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCronAPI_CRUD(t *testing.T) {
	handler, _, _ := newCronAPITestHandler(t)

	createBody := map[string]any{
		"name":     "Morning summary",
		"message":  "Summarize today's tasks",
		"schedule": map[string]any{"kind": "every", "every_ms": 3600_000},
	}
	job := decodeCronJob(t, doCronAPIRequest(t, handler, http.MethodPost, "/cron/jobs", "test-token", createBody))
	if job.ID == "" {
		t.Fatal("created job has no id")
	}
	if job.Name != "Morning summary" || job.Payload.Message != "Summarize today's tasks" {
		t.Fatalf("job = %#v, want created fields", job)
	}
	if job.Payload.Channel != "cli" || job.Payload.To != "direct" {
		t.Fatalf("default target = %s:%s, want cli:direct", job.Payload.Channel, job.Payload.To)
	}
	if !job.Enabled {
		t.Fatal("created job should be enabled")
	}

	listRec := doCronAPIRequest(t, handler, http.MethodGet, "/cron/jobs", "test-token", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}
	var list struct {
		Jobs   []cron.CronJob `json:"jobs"`
		Status struct {
			Running     any  `json:"running"`
			ToolEnabled bool `json:"tool_enabled"`
		} `json:"status"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(list.Jobs) != 1 || list.Jobs[0].ID != job.ID {
		t.Fatalf("jobs = %#v, want one created job", list.Jobs)
	}
	if !list.Status.ToolEnabled {
		t.Fatal("tool_enabled = false, want true")
	}

	updateBody := map[string]any{
		"name":     "Evening summary",
		"message":  "Summarize today's results",
		"schedule": map[string]any{"kind": "cron", "expr": "0 18 * * *"},
		"channel":  "pico",
		"to":       "pico:session-1",
	}
	updated := decodeCronJob(t, doCronAPIRequest(t, handler, http.MethodPut, "/cron/jobs/"+job.ID, "test-token", updateBody))
	if updated.Name != "Evening summary" {
		t.Fatalf("name = %q, want Evening summary", updated.Name)
	}
	if updated.Schedule.Kind != "cron" || updated.Schedule.Expr != "0 18 * * *" {
		t.Fatalf("schedule = %#v, want cron expression", updated.Schedule)
	}
	if updated.Payload.Channel != "pico" || updated.Payload.To != "pico:session-1" {
		t.Fatalf("target = %s:%s, want pico:pico:session-1", updated.Payload.Channel, updated.Payload.To)
	}
	if updated.State.NextRunAtMS == nil {
		t.Fatal("next run was not computed")
	}

	disabled := decodeCronJob(t, doCronAPIRequest(t, handler, http.MethodPost, "/cron/jobs/"+job.ID+"/enable", "test-token", map[string]any{"enabled": false}))
	if disabled.Enabled {
		t.Fatal("job should be disabled")
	}
	if disabled.State.NextRunAtMS != nil {
		t.Fatal("disabled job should not have a next run")
	}

	removed := doCronAPIRequest(t, handler, http.MethodDelete, "/cron/jobs/"+job.ID, "test-token", nil)
	if removed.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", removed.Code, removed.Body.String())
	}

	missing := doCronAPIRequest(t, handler, http.MethodGet, "/cron/jobs/"+job.ID, "test-token", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("get deleted status = %d, want 404", missing.Code)
	}
}

func TestCronAPI_Validation(t *testing.T) {
	handler, _, _ := newCronAPITestHandler(t)

	tests := []struct {
		name string
		body map[string]any
		code string
	}{
		{
			name: "missing message",
			body: map[string]any{"schedule": map[string]any{"kind": "every", "every_ms": 3600_000}},
			code: "invalid_request",
		},
		{
			name: "too small interval",
			body: map[string]any{"message": "x", "schedule": map[string]any{"kind": "every", "every_ms": 1000}},
			code: "invalid_schedule",
		},
		{
			name: "bad cron expression",
			body: map[string]any{"message": "x", "schedule": map[string]any{"kind": "cron", "expr": "not a cron"}},
			code: "invalid_schedule",
		},
		{
			name: "past one-time schedule",
			body: map[string]any{"message": "x", "schedule": map[string]any{"kind": "at", "at_ms": time.Now().Add(-time.Hour).UnixMilli()}},
			code: "invalid_schedule",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := doCronAPIRequest(t, handler, http.MethodPost, "/cron/jobs", "test-token", test.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
			var payload struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if payload.Code != test.code {
				t.Fatalf("code = %q, want %q", payload.Code, test.code)
			}
		})
	}
}

func TestCronAPI_RunNow(t *testing.T) {
	handler, _, calls := newCronAPITestHandler(t)

	job := decodeCronJob(t, doCronAPIRequest(t, handler, http.MethodPost, "/cron/jobs", "test-token", map[string]any{
		"name":     "manual",
		"message":  "do it now",
		"schedule": map[string]any{"kind": "every", "every_ms": 3600_000},
	}))

	rec := doCronAPIRequest(t, handler, http.MethodPost, "/cron/jobs/"+job.ID+"/run", "test-token", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("run status = %d, body = %s", rec.Code, rec.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && calls.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}

	missing := doCronAPIRequest(t, handler, http.MethodPost, "/cron/jobs/missing/run", "test-token", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("run missing status = %d, want 404", missing.Code)
	}
}

func TestCronAPI_CommandJobsAreReadOnly(t *testing.T) {
	handler, svc, _ := newCronAPITestHandler(t)

	at := time.Now().Add(time.Hour).UnixMilli()
	job, err := svc.AddJob("command job", cron.CronSchedule{Kind: "at", AtMS: &at}, "run df -h", "cli", "direct")
	if err != nil {
		t.Fatalf("AddJob() error = %v", err)
	}
	stored, ok := svc.GetJob(job.ID)
	if !ok {
		t.Fatal("job not found")
	}
	stored.Payload.Command = "df -h"
	if err := svc.UpdateJob(stored); err != nil {
		t.Fatalf("UpdateJob() error = %v", err)
	}

	rec := doCronAPIRequest(t, handler, http.MethodPut, "/cron/jobs/"+job.ID, "test-token", map[string]any{
		"name":     "edited",
		"message":  "edited",
		"schedule": map[string]any{"kind": "every", "every_ms": 3600_000},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}
