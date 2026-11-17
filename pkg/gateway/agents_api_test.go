package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
)

type fakeAgentModelController struct {
	model        string
	oldModel     string
	applied      bool
	switchErr    error
	lastAgent    string
	lastModel    string
	lastWait     time.Duration
	defaultModel string
	getNotFound  bool
}

func (f *fakeAgentModelController) SetDefaultModelName(modelName string) {
	f.defaultModel = modelName
}

func (f *fakeAgentModelController) GetAgentModel(agentID string) (string, bool) {
	if f.getNotFound || agentID == "missing" {
		return "", false
	}
	return f.model, true
}

func (f *fakeAgentModelController) SwitchAgentModel(
	agentID, modelName string,
	wait time.Duration,
) (string, bool, error) {
	f.lastAgent = agentID
	f.lastModel = modelName
	f.lastWait = wait
	if f.switchErr != nil {
		return "", false, f.switchErr
	}
	return f.oldModel, f.applied, nil
}

func serveAgentsAPI(t *testing.T, loop *fakeAgentModelController, method, path, body string, token string) *httptest.ResponseRecorder {
	t.Helper()
	handler := newAgentsAPIHandler(loop, "secret")
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAgentsAPIRequiresToken(t *testing.T) {
	loop := &fakeAgentModelController{}
	rec := serveAgentsAPI(t, loop, http.MethodGet, "/agents/main/model", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAgentsAPIGetModel(t *testing.T) {
	loop := &fakeAgentModelController{model: "hy4 opencode"}
	rec := serveAgentsAPI(t, loop, http.MethodGet, "/agents/main/model", "", "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["model"] != "hy4 opencode" {
		t.Fatalf("model = %v, want hy4 opencode", payload["model"])
	}
}

func TestAgentsAPIGetMissingAgent(t *testing.T) {
	loop := &fakeAgentModelController{}
	rec := serveAgentsAPI(t, loop, http.MethodGet, "/agents/missing/model", "", "secret")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAgentsAPISwitchApplied(t *testing.T) {
	loop := &fakeAgentModelController{oldModel: "old", applied: true}
	rec := serveAgentsAPI(t, loop, http.MethodPost, "/agents/deneme/model", `{"model":"new"}`, "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if loop.lastAgent != "deneme" || loop.lastModel != "new" {
		t.Fatalf("switch called with %q/%q", loop.lastAgent, loop.lastModel)
	}
	if loop.lastWait != agent.DefaultModelSwitchWait {
		t.Fatalf("wait = %v, want %v", loop.lastWait, agent.DefaultModelSwitchWait)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["old_model"] != "old" || payload["applied"] != true {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestAgentsAPISwitchPending(t *testing.T) {
	loop := &fakeAgentModelController{applied: false}
	rec := serveAgentsAPI(t, loop, http.MethodPost, "/agents/deneme/model", `{"model":"new"}`, "secret")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["status"] != "pending" || payload["applied"] != false {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestAgentsAPIMainSwitchSetsDefault(t *testing.T) {
	loop := &fakeAgentModelController{oldModel: "old", applied: true}
	rec := serveAgentsAPI(t, loop, http.MethodPost, "/agents/main/model", `{"model":"new"}`, "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if loop.defaultModel != "new" {
		t.Fatalf("default model = %q, want new", loop.defaultModel)
	}
}

func TestAgentsAPIRejectsInvalidModel(t *testing.T) {
	loop := &fakeAgentModelController{switchErr: errors.New("model \"missing\" not found")}
	rec := serveAgentsAPI(t, loop, http.MethodPost, "/agents/deneme/model", `{"model":"missing"}`, "secret")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAgentsAPIAgentNotFoundOnSwitch(t *testing.T) {
	loop := &fakeAgentModelController{switchErr: agent.ErrAgentNotFound}
	rec := serveAgentsAPI(t, loop, http.MethodPost, "/agents/missing/model", `{"model":"new"}`, "secret")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAgentsAPIMethodNotAllowed(t *testing.T) {
	loop := &fakeAgentModelController{}
	rec := serveAgentsAPI(t, loop, http.MethodDelete, "/agents/main/model", "", "secret")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
