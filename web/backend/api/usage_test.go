package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUsageProxy_UnavailableWithoutGateway(t *testing.T) {
	mux := http.NewServeMux()
	NewHandler(setupChatAttachmentTestEnv(t)).registerUsageRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/usage/summary", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", rec.Code, rec.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["code"] != "gateway_unavailable" {
		t.Fatalf("code = %q, want gateway_unavailable", payload["code"])
	}
}
