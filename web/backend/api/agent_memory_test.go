package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentpkg "github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/config"
)

func TestHandleAgentMemoryAutoWrite(t *testing.T) {
	defer agentpkg.SetMemoryEnabled(true)
	defer agentpkg.SetMemoryAutoWrite(false)

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
	if resp.AutoWrite || resp.AutoWriteRuntime {
		t.Fatalf("auto-write should default to disabled, got %#v", resp)
	}
	if resp.LastWriteAt != nil {
		t.Fatalf("last_write_at = %v, want nil before any note", resp.LastWriteAt)
	}

	// Create today's note so the GET response reports the last write.
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	now := time.Now()
	dateStr := now.Format("20060102")
	notePath := filepath.Join(cfg.WorkspacePath(), "memory", dateStr[:6], dateStr+".md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, []byte("# note\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp = get()
	if resp.LastWriteAt == nil || *resp.LastWriteAt == 0 {
		t.Fatalf("last_write_at = %v, want a timestamp", resp.LastWriteAt)
	}

	put := func(body string) memorySettingsResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/agent/memory", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s status = %d body=%s", body, rec.Code, rec.Body.String())
		}
		var resp memorySettingsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		return resp
	}

	resp = put(`{"auto_write":true}`)
	if !resp.AutoWrite || !resp.AutoWriteRuntime {
		t.Fatalf("auto-write was not enabled: %#v", resp)
	}
	if !agentpkg.MemoryAutoWriteEnabled() {
		t.Fatal("runtime auto-write switch should be enabled immediately")
	}
	if !resp.Enabled {
		t.Fatal("auto_write-only update must not change the memory master switch")
	}

	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !cfg.Agents.Defaults.Memory.IsAutoWriteEnabled() {
		t.Fatal("enabled auto-write setting was not persisted")
	}

	resp = put(`{"enabled":false,"auto_write":false}`)
	if resp.Enabled || resp.AutoWrite || agentpkg.MemoryEnabled() || agentpkg.MemoryAutoWriteEnabled() {
		t.Fatalf("both switches should be off: %#v", resp)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/agent/memory", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT status = %d, want 400", rec.Code)
	}
}

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
