package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func newVoiceTestEnv(t *testing.T) (string, *http.ServeMux) {
	t.Helper()
	configPath, cleanup := setupOAuthTestEnv(t)
	t.Cleanup(cleanup)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "local-tts",
			Provider:  "openai-compatible",
			Model:     "kokoro-tts",
			APIBase:   "http://127.0.0.1:9999/v1",
			APIKeys:   config.SimpleSecureStrings("test-key"),
		},
		{
			ModelName: "chat-only",
			Provider:  "openai-compatible",
			Model:     "gpt-test",
			APIBase:   "http://127.0.0.1:9999/v1",
		},
	}
	cfg.Voice.TTSModelName = ""
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return configPath, mux
}

func TestVoiceConfig_Get(t *testing.T) {
	_, mux := newVoiceTestEnv(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/voice/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var response voiceConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.TTSModelName != "" {
		t.Fatalf("tts_model_name = %q, want empty", response.TTSModelName)
	}
	if !response.Resolved || response.ResolvedName != "local-tts" {
		t.Fatalf("resolved = %+v, want local-tts via fallback scan", response)
	}
	if len(response.Models) != 2 {
		t.Fatalf("models = %d, want 2", len(response.Models))
	}
	if response.Models[0].TTSLike != true || !response.Models[0].HasKey {
		t.Fatalf("first model = %+v, want tts_like with key", response.Models[0])
	}
	if response.Models[1].TTSLike || response.Models[1].HasKey {
		t.Fatalf("second model = %+v, want plain model without key", response.Models[1])
	}
}

func TestVoiceConfig_Update(t *testing.T) {
	configPath, mux := newVoiceTestEnv(t)

	body := bytes.NewBufferString(`{"tts_model_name":"local-tts"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/voice/config", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Voice.TTSModelName != "local-tts" {
		t.Fatalf("tts_model_name = %q, want local-tts", cfg.Voice.TTSModelName)
	}
}

func TestVoiceConfig_UpdateUnknownModel(t *testing.T) {
	_, mux := newVoiceTestEnv(t)

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/voice/config",
		bytes.NewBufferString(`{"tts_model_name":"missing-model"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if payload.Code != "model_not_found" {
		t.Fatalf("code = %q, want model_not_found", payload.Code)
	}
}

func TestVoiceTest_RequiresProvider(t *testing.T) {
	configPath, mux := newVoiceTestEnv(t)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = nil
	cfg.Voice.TTSModelName = ""
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/voice/test", bytes.NewBufferString(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestVoiceTest_ReturnsAudio(t *testing.T) {
	ttsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/audio/speech") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("AUDIO-BYTES"))
	}))
	defer ttsServer.Close()

	configPath, mux := newVoiceTestEnv(t)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "local-tts",
		Provider:  "openai-compatible",
		Model:     "kokoro-tts",
		APIBase:   ttsServer.URL + "/v1",
		APIKeys:   config.SimpleSecureStrings("test-key"),
	}}
	cfg.Voice.TTSModelName = "local-tts"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/voice/test", bytes.NewBufferString(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "AUDIO-BYTES" {
		t.Fatalf("body = %q, want AUDIO-BYTES", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "audio/") {
		t.Fatalf("content type = %q, want audio/*", ct)
	}
}

func TestVoiceConfig_LocalModelIsTTSLike(t *testing.T) {
	configPath, mux := newVoiceTestEnv(t)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = append(cfg.ModelList, &config.ModelConfig{
		ModelName: "tts-tiny-voca",
		Provider:  "openai-compatible",
		Model:     "tiny-voca",
		APIBase:   "http://127.0.0.1:9999/v1",
		APIKeys:   config.SimpleSecureStrings("token"),
		ExtraBody: map[string]any{"picoclaw_local_tts": "tiny-voca"},
	})
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/voice/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response voiceConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, model := range response.Models {
		if model.Name == "tts-tiny-voca" {
			if !model.TTSLike {
				t.Fatal("local tiny-voca model must be marked as TTS-capable")
			}
			return
		}
	}
	t.Fatal("tts-tiny-voca model not found")
}

func TestVoiceTest_UsesUnsavedModelAndVoice(t *testing.T) {
	savedCalled := false
	savedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		savedCalled = true
		_, _ = w.Write([]byte("SAVED-AUDIO"))
	}))
	defer savedServer.Close()

	var requestedVoice string
	freshServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if voice, ok := body["voice"].(string); ok {
			requestedVoice = voice
		}
		_, _ = w.Write([]byte("FRESH-AUDIO"))
	}))
	defer freshServer.Close()

	configPath, mux := newVoiceTestEnv(t)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "saved-tts",
			Provider:  "openai-compatible",
			Model:     "saved-tts",
			APIBase:   savedServer.URL + "/v1",
			APIKeys:   config.SimpleSecureStrings("test-key"),
		},
		{
			ModelName: "fresh-tts",
			Provider:  "openai-compatible",
			Model:     "fresh-tts",
			APIBase:   freshServer.URL + "/v1",
			APIKeys:   config.SimpleSecureStrings("test-key"),
		},
	}
	cfg.Voice.TTSModelName = "saved-tts"
	cfg.Voice.TTSVoices = map[string]string{"saved-tts": "female_1"}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/voice/test",
		bytes.NewBufferString(`{"text":"hi","tts_model_name":"fresh-tts","tts_voice":"male_3"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "FRESH-AUDIO" {
		t.Fatalf("body = %q, want the unsaved model's audio", rec.Body.String())
	}
	if savedCalled {
		t.Fatal("saved TTS model must not be called when the request overrides it")
	}
	if requestedVoice != "male_3" {
		t.Fatalf("voice = %q, want male_3", requestedVoice)
	}
}

func TestVoiceSpeak_ReturnsAudio(t *testing.T) {
	ttsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/audio/speech") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("READ-ALOUD-AUDIO"))
	}))
	defer ttsServer.Close()

	configPath, mux := newVoiceTestEnv(t)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "local-tts",
		Provider:  "openai-compatible",
		Model:     "clarvia-tts-tiny",
		APIBase:   ttsServer.URL + "/v1",
		APIKeys:   config.SimpleSecureStrings("test-key"),
	}}
	cfg.Voice.TTSModelName = "local-tts"
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/voice/speak",
		bytes.NewBufferString(`{"text":"Merhaba dünya","language":"tr"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "READ-ALOUD-AUDIO" {
		t.Fatalf("body = %q, want READ-ALOUD-AUDIO", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "audio/") {
		t.Fatalf("content type = %q, want audio/*", ct)
	}
}

func TestVoiceSpeak_RequiresText(t *testing.T) {
	_, mux := newVoiceTestEnv(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/voice/speak",
		bytes.NewBufferString(`{"text":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "text_required") {
		t.Fatalf("body = %s, want text_required", rec.Body.String())
	}
}

func TestVoiceSpeak_RejectsVeryLongText(t *testing.T) {
	_, mux := newVoiceTestEnv(t)

	longText := strings.Repeat("a", voiceSpeakMaxChars+1)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/voice/speak",
		bytes.NewBufferString(fmt.Sprintf(`{"text":%q}`, longText)))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "text_too_long") {
		t.Fatalf("body = %s, want text_too_long", rec.Body.String())
	}
}
