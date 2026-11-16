package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCronProxy_UnavailableWithoutGateway(t *testing.T) {
	resetGatewayProxyState(t)
	mux := http.NewServeMux()
	NewHandler(setupChatAttachmentTestEnv(t)).registerCronRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/cron/jobs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var payload struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Code != "gateway_unavailable" {
		t.Fatalf("code = %q, want gateway_unavailable", payload.Code)
	}
	if payload.Error == "" {
		t.Fatal("error message is empty")
	}
}
