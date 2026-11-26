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
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/audio/tts"
	ttscache "github.com/sipeed/picoclaw/pkg/audio/tts/cache"
	"github.com/sipeed/picoclaw/pkg/audio/tts/local"
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
	TTSModelName     string              `json:"tts_model_name"`
	TTSVoice         string              `json:"tts_voice,omitempty"`
	TTSSpeed         float64             `json:"tts_speed"`
	Voices           []local.VoiceOption `json:"voices,omitempty"`
	Resolved         bool                `json:"resolved"`
	ResolvedName     string              `json:"resolved_name,omitempty"`
	ResolvedProvider string              `json:"resolved_provider,omitempty"`
	ResolvedModel    string              `json:"resolved_model,omitempty"`
	Models           []voiceModelOption  `json:"models"`
	RestartRequired  bool                `json:"restart_required"`
}

type voiceConfigRequest struct {
	TTSModelName string   `json:"tts_model_name"`
	TTSVoice     *string  `json:"tts_voice,omitempty"`
	TTSSpeed     *float64 `json:"tts_speed,omitempty"`
}

type voiceTestRequest struct {
	Text         string  `json:"text"`
	TTSModelName string  `json:"tts_model_name,omitempty"`
	TTSVoice     string  `json:"tts_voice,omitempty"`
	Speed        float64 `json:"speed,omitempty"`
}

type voiceSpeakRequest struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
}

const (
	voiceTestDefaultText = "Hello from PicoClaw. This is a text to speech test."
	voiceTestMaxChars    = 400
	voiceTestMaxBytes    = 20 << 20

	voiceSpeakMaxChars = 12000
	voiceSpeakMaxBytes = 60 << 20
)

func (h *Handler) registerVoiceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/voice/config", h.handleGetVoiceConfig)
	mux.HandleFunc("PUT /api/voice/config", h.handleUpdateVoiceConfig)
	mux.HandleFunc("POST /api/voice/test", h.handleTestVoice)
	mux.HandleFunc("POST /api/voice/speak", h.handleSpeakVoice)
	mux.HandleFunc("GET /api/voice/cache", h.handleVoiceCacheStats)
	mux.HandleFunc("DELETE /api/voice/cache", h.handleVoiceCacheClear)
	h.registerLocalTTSRoutes(mux)
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
	if name != "" && req.TTSVoice != nil {
		voice := strings.TrimSpace(*req.TTSVoice)
		if cfg.Voice.TTSVoices == nil {
			cfg.Voice.TTSVoices = make(map[string]string)
		}
		if voice == "" {
			delete(cfg.Voice.TTSVoices, name)
		} else {
			cfg.Voice.TTSVoices[name] = voice
		}
	}
	if req.TTSSpeed != nil {
		speed := *req.TTSSpeed
		if speed < 0.5 || speed > 2.0 {
			writeBrowserSettingsError(w, http.StatusBadRequest, "speed_out_of_range",
				"Speech speed must be between 0.5 and 2.0.")
			return
		}
		cfg.Voice.TTSSpeed = speed
	}
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
	if name := strings.TrimSpace(req.TTSModelName); name != "" {
		provider = tts.ProviderForModel(cfg, name, strings.TrimSpace(req.TTSVoice))
		if provider == nil {
			writeBrowserSettingsError(w, http.StatusBadRequest, "model_not_found",
				fmt.Sprintf("TTS model %q was not found or has no API key.", name))
			return
		}
	}
	if provider == nil {
		writeBrowserSettingsError(w, http.StatusBadRequest, "tts_not_configured",
			"No text-to-speech model is ready. Add a TTS model with an API key and select it here.")
		return
	}
	tts.ApplySpeed(provider, normalizeVoiceSpeed(req.Speed, cfg.Voice.TTSSpeed))

	h.writeSynthesizedVoice(w, r, provider, text, 30*time.Second, voiceTestMaxBytes, "tts-test", "")
}

// handleSpeakVoice synthesizes arbitrary assistant text for the WebUI
// read-aloud button. It is intentionally separate from send_tts: reading a
// reply does not require the agent tool to be enabled.
func (h *Handler) handleSpeakVoice(w http.ResponseWriter, r *http.Request) {
	var req voiceSpeakRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeBrowserSettingsError(w, http.StatusBadRequest, "text_required", "Text is required.")
		return
	}
	if utf8.RuneCountInString(text) > voiceSpeakMaxChars {
		writeBrowserSettingsError(w, http.StatusBadRequest, "text_too_long",
			fmt.Sprintf("Text to read must be at most %d characters.", voiceSpeakMaxChars))
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "config_unreadable",
			"Failed to load config: "+err.Error())
		return
	}
	provider := tts.DetectTTS(cfg)
	if lang := strings.TrimSpace(req.Language); lang != "" {
		provider = tts.DetectTTSForLanguage(cfg, lang)
		if provider == nil {
			writeBrowserSettingsError(w, http.StatusBadRequest, "tts_language_unavailable",
				"No installed text-to-speech model supports the requested language.")
			return
		}
	}
	if provider == nil {
		writeBrowserSettingsError(w, http.StatusBadRequest, "tts_not_configured",
			"No text-to-speech model is ready.")
		return
	}

	speed := normalizeVoiceSpeed(req.Speed, cfg.Voice.TTSSpeed)
	tts.ApplySpeed(provider, speed)

	resolvedName, _, resolvedModel, _ := resolveVoiceModel(cfg)
	voice := ""
	if mc, err := cfg.GetModelConfig(resolvedName); err == nil {
		voice = strings.TrimSpace(cfg.Voice.TTSVoices[mc.ModelName])
	}
	cacheKey := ttscache.Key(
		"voice-v1",
		provider.Name(),
		resolvedName,
		resolvedModel,
		voice,
		fmt.Sprintf("%.2f", speed),
		strings.ToLower(strings.TrimSpace(req.Language)),
		text,
	)

	h.writeSynthesizedVoice(w, r, provider, text, 120*time.Second, voiceSpeakMaxBytes, "read-aloud", cacheKey)
}

func (h *Handler) writeSynthesizedVoice(
	w http.ResponseWriter,
	r *http.Request,
	provider tts.TTSProvider,
	text string,
	timeout time.Duration,
	maxBytes int64,
	filename string,
	cacheKey string,
) {
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if cacheKey != "" {
		if entry, ok := getVoiceCache().Get(cacheKey); ok {
			writeVoiceAudio(w, entry.Data, entry.Meta.ContentType, filename+entry.Meta.Ext)
			return
		}
	}

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
	if _, err := io.Copy(&buf, io.LimitReader(stream, maxBytes)); err != nil {
		writeBrowserSettingsError(w, http.StatusBadGateway, "tts_failed",
			"Could not read synthesized audio: "+err.Error())
		return
	}
	if buf.Len() == 0 {
		writeBrowserSettingsError(w, http.StatusBadGateway, "tts_failed",
			"The text-to-speech provider returned no audio.")
		return
	}

	if cacheKey != "" {
		_ = getVoiceCache().Put(cacheKey, buf.Bytes(), ttscache.Meta{
			ContentType: contentType,
			Ext:         ext,
		})
	}
	writeVoiceAudio(w, buf.Bytes(), contentType, filename+ext)
}

func writeVoiceAudio(w http.ResponseWriter, data []byte, contentType, filename string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func normalizeVoiceSpeed(requested, configured float64) float64 {
	if requested >= 0.5 && requested <= 2.0 {
		return requested
	}
	if configured >= 0.5 && configured <= 2.0 {
		return configured
	}
	return 1.0
}

func buildVoiceConfigResponse(cfg *config.Config) voiceConfigResponse {
	response := voiceConfigResponse{
		TTSModelName: strings.TrimSpace(cfg.Voice.TTSModelName),
		TTSSpeed:     normalizeVoiceSpeed(0, cfg.Voice.TTSSpeed),
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
			TTSLike:  strings.Contains(strings.ToLower(model.Model), "tts") || isLocalTTSModel(model),
		})
	}

	resolvedName, resolvedProvider, resolvedModel, ok := resolveVoiceModel(cfg)
	if ok {
		response.Resolved = true
		response.ResolvedName = resolvedName
		response.ResolvedProvider = resolvedProvider
		response.ResolvedModel = resolvedModel
		response.TTSVoice = strings.TrimSpace(cfg.Voice.TTSVoices[resolvedName])
		if mc, err := cfg.GetModelConfig(resolvedName); err == nil {
			if localID, ok := mc.ExtraBody["picoclaw_local_tts"].(string); ok &&
				strings.TrimSpace(localID) != "" {
				if installed, found := local.Default().InstalledByID(localID); found {
					response.Voices = append([]local.VoiceOption(nil), installed.Voices...)
					if response.TTSVoice == "" {
						response.TTSVoice = installed.DefaultVoice
					}
				}
			}
		}
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
		if (strings.Contains(strings.ToLower(mc.Model), "tts") || isLocalTTSModel(mc)) && mc.APIKey() != "" {
			return strings.TrimSpace(mc.ModelName), mc.Provider, mc.Model, true
		}
	}
	return "", "", "", false
}

func isLocalTTSModel(mc *config.ModelConfig) bool {
	if mc == nil {
		return false
	}
	id, ok := mc.ExtraBody["picoclaw_local_tts"].(string)
	return ok && strings.TrimSpace(id) != ""
}
