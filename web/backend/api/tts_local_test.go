package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/audio/tts/local"
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

func thirdPartyManifest(t *testing.T) []byte {
	t.Helper()
	manifest := local.Manifest{
		Version: 1,
		Models: []local.Recipe{{
			ID:           "tiny-voca-es-user",
			Name:         "tiny-voca (Spanish, user manifest)",
			Language:     "es",
			License:      "Apache-2.0",
			LicenseURL:   "https://huggingface.co/marcosmaceo/tiny-voca-es",
			SourceURL:    "https://huggingface.co/marcosmaceo/tiny-voca-es",
			HFRepo:       "marcosmaceo/tiny-voca-es",
			ModelFile:    "tiny-voca-es.onnx",
			ConfigFile:   "tiny-voca-es.json",
			Adapter:      "espeak",
			SizeBytes:    22 << 20,
			SampleRate:   24000,
			Voices:       []local.VoiceOption{{ID: "F2", Label: "F2"}},
			DefaultVoice: "F2",
			ThirdParty:   true,
			Checksums: map[string]string{
				"tiny-voca-es.onnx": strings.Repeat("a", 64),
				"tiny-voca-es.json": strings.Repeat("b", 64),
			},
			Frontend: &local.FrontendSpec{
				Kind:        "espeak",
				EspeakVoice: "es",
				Symbols:     []string{"<pad>", "<bos>", "<eos>", "<sp>", "<unk>", ",", ".", "a", "e", "i", "o", "u"},
				Vowels:      []string{"a", "e", "i", "o", "u"},
			},
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func putUserManifest(t *testing.T, mux *http.ServeMux, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/tts/local/manifest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

func TestLocalTTSManifest_CRUD(t *testing.T) {
	_, mux := newVoiceTestEnv(t)

	rec := putUserManifest(t, mux, thirdPartyManifest(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tts/local/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	var response localTTSModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var found *localTTSCatalogItem
	for index := range response.Catalog {
		if response.Catalog[index].ID == "tiny-voca-es-user" {
			found = &response.Catalog[index]
		}
	}
	if found == nil {
		t.Fatalf("catalog = %#v", response.Catalog)
	}
	if !found.ThirdParty {
		t.Fatal("model should be marked third-party")
	}
	if len(found.Requires) != 1 || found.Requires[0] != "espeak-ng" {
		t.Fatalf("requires = %#v", found.Requires)
	}
	if len(found.Dependencies) != 1 || found.Dependencies[0].Name != "espeak-ng" {
		t.Fatalf("dependencies = %#v", found.Dependencies)
	}
	if !strings.Contains(response.UserManifest, "tiny-voca-es-user") {
		t.Fatalf("user_manifest = %q", response.UserManifest)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/tts/local/manifest", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tts/local/models", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, item := range response.Catalog {
		if item.ID == "tiny-voca-es-user" {
			t.Fatalf("model still listed after delete: %#v", item)
		}
	}
}

func TestLocalTTSManifest_RejectsInvalid(t *testing.T) {
	_, mux := newVoiceTestEnv(t)

	rec := putUserManifest(t, mux, []byte(`{"version":1,"models":[],"evil":true}`))
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("invalid_manifest")) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var manifest local.Manifest
	if err := json.Unmarshal(thirdPartyManifest(t), &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Models[0].ID = "clarvia-tts-tiny"
	reserved, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rec = putUserManifest(t, mux, reserved)
	if rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("reserved")) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestLocalTTSInstall_RequiresLicenseAcceptance(t *testing.T) {
	_, mux := newVoiceTestEnv(t)
	if rec := putUserManifest(t, mux, thirdPartyManifest(t)); rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tts/local/install",
		bytes.NewBufferString(`{"url":"tiny-voca-es-user","consent":true}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("license_acceptance_required")) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLocalTTSInstall_DependencyMissing(t *testing.T) {
	if _, err := exec.LookPath("espeak-ng"); err == nil {
		t.Skip("espeak-ng is installed; dependency failure cannot be exercised")
	}
	_, mux := newVoiceTestEnv(t)
	if rec := putUserManifest(t, mux, thirdPartyManifest(t)); rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tts/local/install",
		bytes.NewBufferString(`{"url":"tiny-voca-es-user","consent":true,"accept_license":"tiny-voca-es-user"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("dependency_missing")) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
