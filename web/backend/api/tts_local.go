package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/audio/tts/local"
	"github.com/sipeed/picoclaw/pkg/config"
)

type localTTSInstallRequest struct {
	URL     string `json:"url"`
	Consent bool   `json:"consent"`
}

type localTTSCatalogItem struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Language     string              `json:"language"`
	License      string              `json:"license"`
	SizeBytes    int64               `json:"size_bytes"`
	Voices       []local.VoiceOption `json:"voices"`
	DefaultVoice string              `json:"default_voice"`
	Installed    bool                `json:"installed"`
	Port         int                 `json:"port,omitempty"`
	Running      bool                `json:"running"`
}

type localTTSModelsResponse struct {
	Environment local.PythonInfo      `json:"environment"`
	Catalog     []localTTSCatalogItem `json:"catalog"`
	Status      local.Status          `json:"status"`
}

func (h *Handler) registerLocalTTSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tts/local/models", h.handleLocalTTSModels)
	mux.HandleFunc("GET /api/tts/local/status", h.handleLocalTTSStatus)
	mux.HandleFunc("POST /api/tts/local/install", h.handleLocalTTSInstall)
	mux.HandleFunc("DELETE /api/tts/local/models/{id}", h.handleLocalTTSUninstall)
}

func (h *Handler) handleLocalTTSModels(w http.ResponseWriter, r *http.Request) {
	manager := local.Default()
	installed := manager.Installed()
	byID := make(map[string]local.Installed, len(installed))
	for _, item := range installed {
		byID[item.RecipeID] = item
	}
	catalog := make([]localTTSCatalogItem, 0)
	for _, recipe := range local.Catalog() {
		item := localTTSCatalogItem{
			ID:           recipe.ID,
			Name:         recipe.Name,
			Language:     recipe.Language,
			License:      recipe.License,
			SizeBytes:    recipe.SizeBytes,
			Voices:       recipe.Voices,
			DefaultVoice: recipe.DefaultVoice,
		}
		if got, ok := byID[recipe.ID]; ok {
			item.Installed = true
			item.Port = got.Port
			item.Running = manager.Healthy(got)
		}
		catalog = append(catalog, item)
	}
	writeJSON(w, http.StatusOK, localTTSModelsResponse{
		Environment: manager.DetectPython(),
		Catalog:     catalog,
		Status:      redactLocalStatus(manager.Status()),
	})
}

func (h *Handler) handleLocalTTSStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, redactLocalStatus(local.Default().Status()))
}

func (h *Handler) handleLocalTTSInstall(w http.ResponseWriter, r *http.Request) {
	var req localTTSInstallRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}
	if !req.Consent {
		writeAgentsAPIError(w, http.StatusBadRequest, "consent_required",
			"User consent is required before creating a Python environment.")
		return
	}
	recipe, err := local.ResolveRecipe(req.URL)
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "unsupported_model", err.Error())
		return
	}
	manager := local.Default()
	env := manager.DetectPython()
	if !env.Available {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":        "python_missing",
			"error":       env.Message,
			"environment": env,
		})
		return
	}
	if err := manager.StartInstall(r.Context(), *recipe, func(installed *local.Installed, installErr error) {
		if installErr != nil || installed == nil {
			return
		}
		h.registerLocalTTSModel(*installed)
	}); err != nil {
		writeAgentsAPIError(w, http.StatusConflict, "install_running", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "installing",
		"model":  recipe.ID,
	})
}

func (h *Handler) handleLocalTTSUninstall(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_model", "Model id is required.")
		return
	}
	if err := local.Default().Uninstall(id); err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "uninstall_failed", err.Error())
		return
	}
	h.removeLocalTTSModel(id)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "model": id})
}

// registerLocalTTSModel adds or updates the OpenAI-compatible model entry that
// points at the sidecar. It also selects it when no TTS model is configured.
func (h *Handler) registerLocalTTSModel(installed local.Installed) {
	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return
	}
	modelName := ""
	for _, mc := range cfg.ModelList {
		if localID, ok := mc.ExtraBody["picoclaw_local_tts"].(string); ok && localID == installed.RecipeID {
			modelName = mc.ModelName
			break
		}
	}
	if modelName == "" {
		base := "tts-" + installed.RecipeID
		modelName = base
		for suffix := 2; modelExists(cfg, modelName); suffix++ {
			modelName = fmt.Sprintf("%s-%d", base, suffix)
		}
	}

	extraBody := map[string]any{
		"voice":              installed.DefaultVoice,
		"response_format":    "wav",
		"picoclaw_local_tts": installed.RecipeID,
	}
	entry := &config.ModelConfig{
		ModelName: modelName,
		Provider:  "openai-compatible",
		Model:     installed.RecipeID,
		APIBase:   fmt.Sprintf("http://127.0.0.1:%d/v1", installed.Port),
		APIKeys:   config.SimpleSecureStrings(installed.Token),
		ExtraBody: extraBody,
	}
	replaced := false
	for i, mc := range cfg.ModelList {
		if mc.ModelName == modelName {
			cfg.ModelList[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.ModelList = append(cfg.ModelList, entry)
	}
	if strings.TrimSpace(cfg.Voice.TTSModelName) == "" {
		cfg.Voice.TTSModelName = modelName
	}
	if cfg.Voice.TTSVoices == nil {
		cfg.Voice.TTSVoices = make(map[string]string)
	}
	if strings.TrimSpace(cfg.Voice.TTSVoices[modelName]) == "" {
		cfg.Voice.TTSVoices[modelName] = installed.DefaultVoice
	}
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		return
	}
	if h.gatewayAvailableForProxy() {
		_, _ = h.RestartGateway()
	}
}

func (h *Handler) removeLocalTTSModel(id string) {
	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return
	}
	removed := make(map[string]struct{})
	kept := make([]*config.ModelConfig, 0, len(cfg.ModelList))
	for _, mc := range cfg.ModelList {
		if localID, ok := mc.ExtraBody["picoclaw_local_tts"].(string); ok && localID == id {
			removed[mc.ModelName] = struct{}{}
			continue
		}
		kept = append(kept, mc)
	}
	cfg.ModelList = kept
	if _, ok := removed[cfg.Voice.TTSModelName]; ok {
		cfg.Voice.TTSModelName = ""
	}
	for name := range removed {
		delete(cfg.Voice.TTSVoices, name)
	}
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		return
	}
	if h.gatewayAvailableForProxy() {
		_, _ = h.RestartGateway()
	}
}

func modelExists(cfg *config.Config, name string) bool {
	for _, mc := range cfg.ModelList {
		if mc.ModelName == name {
			return true
		}
	}
	return false
}

func redactLocalStatus(status local.Status) local.Status {
	if status.Model != nil {
		clone := *status.Model
		clone.Token = ""
		status.Model = &clone
	}
	return status
}
