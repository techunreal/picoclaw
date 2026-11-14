package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipeed/picoclaw/pkg/agent"
)

type fakeSubagentController struct {
	runs     []agent.SubagentRun
	canceled []string
}

func (f *fakeSubagentController) ListSubagentRuns() []agent.SubagentRun {
	return f.runs
}

func (f *fakeSubagentController) CancelSubTurn(id string) bool {
	if id == "subturn-1" {
		f.canceled = append(f.canceled, id)
		return true
	}
	return false
}

func TestSubagentsAPIRequiresToken(t *testing.T) {
	h := newSubagentsAPIHandler(&fakeSubagentController{}, "secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/subagents/runs", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSubagentsAPIListRuns(t *testing.T) {
	controller := &fakeSubagentController{runs: []agent.SubagentRun{{
		ID:         "subturn-1",
		AgentID:    "main",
		Task:       "analyze logs",
		Status:     "running",
		Depth:      1,
		DurationMS: 1200,
	}}}
	h := newSubagentsAPIHandler(controller, "secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/subagents/runs", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Runs []agent.SubagentRun `json:"runs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(payload.Runs) != 1 || payload.Runs[0].ID != "subturn-1" || payload.Runs[0].Status != "running" {
		t.Fatalf("unexpected runs payload: %#v", payload.Runs)
	}
}

func TestSubagentsAPICancel(t *testing.T) {
	controller := &fakeSubagentController{}
	h := newSubagentsAPIHandler(controller, "secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/subagents/runs/subturn-1/cancel", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(controller.canceled) != 1 || controller.canceled[0] != "subturn-1" {
		t.Fatalf("canceled = %#v, want subturn-1", controller.canceled)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/subagents/runs/subturn-9/cancel", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for an inactive subagent", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["code"] != "not_active" {
		t.Fatalf("code = %q, want not_active", payload["code"])
	}
}

func TestSubagentsAPIUnknownEndpoint(t *testing.T) {
	h := newSubagentsAPIHandler(&fakeSubagentController{}, "secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/subagents/nope", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
