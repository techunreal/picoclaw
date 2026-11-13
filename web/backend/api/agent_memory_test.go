package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentpkg "github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestHandleAgentMemoryToggle(t *testing.T) {
	defer agentpkg.SetMemoryEnabled(true)

	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	get := func() memorySettingsResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/agent/memory", nil)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET status = %d body=%s", rec.Code, rec.Body.String())
		}
		var resp memorySettingsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		return resp
	}

	resp := get()
	if !resp.Enabled || !resp.Runtime {
		t.Fatalf("memory should default to enabled, got %#v", resp)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/agent/memory",
		strings.NewReader(`{"enabled":false}`),
	)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	if agentpkg.MemoryEnabled() {
		t.Fatal("runtime memory switch should be disabled immediately")
	}

	resp = get()
	if resp.Enabled || resp.Runtime {
		t.Fatalf("memory should be disabled, got %#v", resp)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Agents.Defaults.Memory.IsEnabled() {
		t.Fatal("disabled memory setting was not persisted")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPut,
		"/api/agent/memory",
		strings.NewReader(`{"enabled":true}`),
	)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-enable status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !agentpkg.MemoryEnabled() {
		t.Fatal("runtime memory switch should be enabled again")
	}

	// Invalid payloads are rejected.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/agent/memory", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT status = %d, want 400", rec.Code)
	}
}
