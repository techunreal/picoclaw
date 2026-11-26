package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalTTSModels_Catalog(t *testing.T) {
	_, mux := newVoiceTestEnv(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tts/local/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response localTTSModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(response.Catalog) == 0 || response.Catalog[0].ID != "clarvia-tts-tiny" {
		t.Fatalf("catalog = %#v", response.Catalog)
	}
	if len(response.Catalog) < 2 || response.Catalog[1].ID != "tiny-voca" ||
		response.Catalog[1].Language != "en" {
		t.Fatalf("catalog = %#v", response.Catalog)
	}
	if len(response.Catalog[0].Voices) != 10 {
		t.Fatalf("voices = %d, want 10", len(response.Catalog[0].Voices))
	}
	if response.Catalog[0].Installed {
		t.Fatalf("model should not be installed in a fresh test home")
	}
}

func TestLocalTTSInstall_RequiresConsent(t *testing.T) {
	_, mux := newVoiceTestEnv(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tts/local/install",
		bytes.NewBufferString(`{"url":"clarvia-tts-tiny","consent":false}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("consent_required")) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLocalTTSInstall_RejectsUnknownModel(t *testing.T) {
	_, mux := newVoiceTestEnv(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tts/local/install",
		bytes.NewBufferString(`{"url":"https://example.com/unknown","consent":true}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("unsupported_model")) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
