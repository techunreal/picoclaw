package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
)

type fakeActiveTurnLister struct {
	turns []agent.ActiveTurnInfo
}

func (f *fakeActiveTurnLister) ListActiveTurns() []agent.ActiveTurnInfo {
	return f.turns
}

func serveTurnsAPI(t *testing.T, loop *fakeActiveTurnLister, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	handler := newTurnsAPIHandler(loop, "secret")
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestTurnsAPIRequiresToken(t *testing.T) {
	loop := &fakeActiveTurnLister{}
	rec := serveTurnsAPI(t, loop, http.MethodGet, "/turns/active", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestTurnsAPIListsActiveTurns(t *testing.T) {
	start := time.Now()
	loop := &fakeActiveTurnLister{turns: []agent.ActiveTurnInfo{
		{
			TurnID:     "turn-1",
			AgentID:    "main",
			SessionKey: "sk_v1_test",
			ChatID:     "pico:session-1",
			Phase:      agent.TurnPhaseRunning,
			StartedAt:  start,
		},
	}}
	rec := serveTurnsAPI(t, loop, http.MethodGet, "/turns/active", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Turns []activeTurnDTO `json:"turns"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(payload.Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(payload.Turns))
	}
	if payload.Turns[0].SessionID != "session-1" || payload.Turns[0].AgentID != "main" {
		t.Fatalf("unexpected turn: %#v", payload.Turns[0])
	}
}

func TestTurnsAPIUnknownPath(t *testing.T) {
	loop := &fakeActiveTurnLister{}
	rec := serveTurnsAPI(t, loop, http.MethodGet, "/turns/other", "secret")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestTurnsAPIMethodNotAllowed(t *testing.T) {
	loop := &fakeActiveTurnLister{}
	rec := serveTurnsAPI(t, loop, http.MethodPost, "/turns/active", "secret")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
