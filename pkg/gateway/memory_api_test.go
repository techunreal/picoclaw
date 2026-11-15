package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/agent"
)

func memoryAPITestServer(token string) *memoryAPIHandler {
	return &memoryAPIHandler{token: token}
}

func TestMemoryAPI_RequiresToken(t *testing.T) {
	handler := memoryAPITestServer("secret")

	req := httptest.NewRequest(http.MethodGet, "/memory/state", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/memory/state", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMemoryAPI_GetAndPutState(t *testing.T) {
	defer agent.SetMemoryEnabled(true)
	defer agent.SetMemoryAutoWrite(false)
	agent.SetMemoryEnabled(true)
	agent.SetMemoryAutoWrite(false)

	handler := memoryAPITestServer("secret")

	get := func() memoryStateResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/memory/state", nil)
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET status = %d body=%s", rec.Code, rec.Body.String())
		}
		var resp memoryStateResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		return resp
	}

	if resp := get(); !resp.Enabled || resp.AutoWrite {
		t.Fatalf("unexpected initial state: %#v", resp)
	}

	req := httptest.NewRequest(http.MethodPut, "/memory/state", strings.NewReader(`{"auto_write":true}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !agent.MemoryAutoWriteEnabled() {
		t.Fatal("auto-write runtime switch should be enabled")
	}

	req = httptest.NewRequest(http.MethodPut, "/memory/state", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	if agent.MemoryEnabled() {
		t.Fatal("memory runtime switch should be disabled")
	}
	if !agent.MemoryAutoWriteEnabled() {
		t.Fatal("auto-write switch must be untouched by an enabled-only update")
	}
}

func TestMemoryAPI_RejectsInvalidPayload(t *testing.T) {
	handler := memoryAPITestServer("secret")

	for _, body := range []string{`{}`, `{"enabled":"yes"}`, `{"unknown":true}`} {
		req := httptest.NewRequest(http.MethodPut, "/memory/state", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s status = %d, want 400", body, rec.Code)
		}
	}
}
