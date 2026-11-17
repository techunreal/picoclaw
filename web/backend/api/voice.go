package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/audio/tts"
	"github.com/sipeed/picoclaw/pkg/config"
)

type voiceModelOption struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	APIBase  string `json:"api_base,omitempty"`
	HasKey   bool   `json:"has_key"`
	TTSLike  bool   `json:"tts_like"`
}

type voiceConfigResponse struct {
	TTSModelName     string             `json:"tts_model_name"`
	Resolved         bool               `json:"resolved"`
	ResolvedName     string             `json:"resolved_name,omitempty"`
	ResolvedProvider string             `json:"resolved_provider,omitempty"`
	ResolvedModel    string             `json:"resolved_model,omitempty"`
	Models           []voiceModelOption `json:"models"`
	RestartRequired  bool               `json:"restart_required"`
}

type voiceConfigRequest struct {
	TTSModelName string `json:"tts_model_name"`
}

type voiceTestRequest struct {
	Text string `json:"text"`
}

const (
	voiceTestDefaultText = "Hello from PicoClaw. This is a text to speech test."
	voiceTestMaxChars    = 400
	voiceTestMaxBytes    = 20 << 20
)

func (h *Handler) registerVoiceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/voice/config", h.handleGetVoiceConfig)
	mux.HandleFunc("PUT /api/voice/config", h.handleUpdateVoiceConfig)
	mux.HandleFunc("POST /api/voice/test", h.handleTestVoice)
}

func (h *Handler) handleGetVoiceConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "config_unreadable",
			"Failed to load config: "+err.Error())
		return
	}
	writeBrowserSettingsJSON(w, http.StatusOK, buildVoiceConfigResponse(cfg))
}

func (h *Handler) handleUpdateVoiceConfig(w http.ResponseWriter, r *http.Request) {
	var req voiceConfigRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}

	name := strings.TrimSpace(req.TTSModelName)

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "config_unreadable",
			"Failed to load config: "+err.Error())
		return
	}
	if name != "" {
		if _, err := cfg.GetModelConfig(name); err != nil {
			writeBrowserSettingsError(w, http.StatusBadRequest, "model_not_found",
				fmt.Sprintf("Model %q was not found in the model list.", name))
			return
		}
	}

	cfg.Voice.TTSModelName = name
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "save_failed",
			"Failed to save config: "+err.Error())
		return
	}

	restartRequired := false
	if h.gatewayAvailableForProxy() {
		// The TTS provider is built when the gateway starts, so apply the change
		// by restarting it now. Fall back to asking the user to restart.
		if _, err := h.RestartGateway(); err != nil {
			restartRequired = true
		}
	}

	response := buildVoiceConfigResponse(cfg)
	response.RestartRequired = restartRequired
	writeBrowserSettingsJSON(w, http.StatusOK, response)
}

func (h *Handler) handleTestVoice(w http.ResponseWriter, r *http.Request) {
	var req voiceTestRequest
	if r.Body != nil {
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		text = voiceTestDefaultText
	}
	if len(text) > voiceTestMaxChars {
		writeBrowserSettingsError(w, http.StatusBadRequest, "text_too_long",
			fmt.Sprintf("Test text must be at most %d characters.", voiceTestMaxChars))
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "config_unreadable",
			"Failed to load config: "+err.Error())
		return
	}

	provider := tts.DetectTTS(cfg)
	if provider == nil {
		writeBrowserSettingsError(w, http.StatusBadRequest, "tts_not_configured",
			"No text-to-speech model is ready. Add a TTS model with an API key and select it here.")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	stream, err := provider.Synthesize(ctx, text)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusBadGateway, "tts_failed",
			"Text-to-speech request failed: "+err.Error())
		return
	}
	defer stream.Close()

	contentType := "audio/ogg"
	ext := ".ogg"
	if metaProvider, ok := stream.(interface{ AudioFileMeta() (string, string) }); ok {
		if metaExt, metaType := metaProvider.AudioFileMeta(); metaExt != "" && metaType != "" {
			ext = metaExt
			contentType = metaType
		}
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(stream, voiceTestMaxBytes)); err != nil {
		writeBrowserSettingsError(w, http.StatusBadGateway, "tts_failed",
			"Could not read synthesized audio: "+err.Error())
		return
	}
	if buf.Len() == 0 {
		writeBrowserSettingsError(w, http.StatusBadGateway, "tts_failed",
			"The text-to-speech provider returned no audio.")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", "tts-test"+ext))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func buildVoiceConfigResponse(cfg *config.Config) voiceConfigResponse {
	response := voiceConfigResponse{
		TTSModelName: strings.TrimSpace(cfg.Voice.TTSModelName),
		Models:       make([]voiceModelOption, 0, len(cfg.ModelList)),
	}
	for _, model := range cfg.ModelList {
		name := strings.TrimSpace(model.ModelName)
		if name == "" {
			continue
		}
		response.Models = append(response.Models, voiceModelOption{
			Name:     name,
			Provider: model.Provider,
			Model:    model.Model,
			APIBase:  model.APIBase,
			HasKey:   model.APIKey() != "",
			TTSLike:  strings.Contains(strings.ToLower(model.Model), "tts"),
		})
	}

	resolvedName, resolvedProvider, resolvedModel, ok := resolveVoiceModel(cfg)
	if ok {
		response.Resolved = true
		response.ResolvedName = resolvedName
		response.ResolvedProvider = resolvedProvider
		response.ResolvedModel = resolvedModel
	}
	return response
}

// resolveVoiceModel mirrors tts.DetectTTS so the UI can show which model actually
// produces audio, including the fallback scan for entries whose id contains "tts".
func resolveVoiceModel(cfg *config.Config) (name, provider, model string, ok bool) {
	if cfg == nil {
		return "", "", "", false
	}
	if configured := strings.TrimSpace(cfg.Voice.TTSModelName); configured != "" {
		if mc, err := cfg.GetModelConfig(configured); err == nil && mc.APIKey() != "" {
			return configured, mc.Provider, mc.Model, true
		}
	}
	for _, mc := range cfg.ModelList {
		if strings.Contains(strings.ToLower(mc.Model), "tts") && mc.APIKey() != "" {
			return strings.TrimSpace(mc.ModelName), mc.Provider, mc.Model, true
		}
	}
	return "", "", "", false
}
