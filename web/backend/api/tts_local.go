package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/audio/tts/local"
	"github.com/sipeed/picoclaw/pkg/config"
)

type localTTSInstallRequest struct {
	URL           string `json:"url"`
	Consent       bool   `json:"consent"`
	AcceptLicense string `json:"accept_license"`
	AcceptTools   bool   `json:"accept_tools"`
}

type localTTSCatalogItem struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Language     string              `json:"language"`
	License      string              `json:"license"`
	LicenseURL   string              `json:"license_url,omitempty"`
	SourceURL    string              `json:"source_url,omitempty"`
	HFRepo       string              `json:"hf_repo,omitempty"`
	ThirdParty   bool                `json:"third_party"`
	SizeBytes    int64               `json:"size_bytes"`
	Voices       []local.VoiceOption `json:"voices"`
	DefaultVoice string              `json:"default_voice"`
	Installed    bool                `json:"installed"`
	Port         int                 `json:"port,omitempty"`
	Running      bool                `json:"running"`
	Requires     []string            `json:"requires,omitempty"`
	Dependencies []local.Dependency  `json:"dependencies,omitempty"`
	Missing      []string            `json:"missing,omitempty"`
}

type localTTSModelsResponse struct {
	Environment   local.PythonInfo      `json:"environment"`
	Catalog       []localTTSCatalogItem `json:"catalog"`
	Status        local.Status          `json:"status"`
	UserManifest  string                `json:"user_manifest"`
	ManifestError string                `json:"manifest_error,omitempty"`
}

func (h *Handler) registerLocalTTSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tts/local/models", h.handleLocalTTSModels)
	mux.HandleFunc("GET /api/tts/local/status", h.handleLocalTTSStatus)
	mux.HandleFunc("POST /api/tts/local/install", h.handleLocalTTSInstall)
	mux.HandleFunc("DELETE /api/tts/local/models/{id}", h.handleLocalTTSUninstall)
	mux.HandleFunc("GET /api/tts/local/manifest", h.handleLocalTTSManifestGet)
	mux.HandleFunc("PUT /api/tts/local/manifest", h.handleLocalTTSManifestPut)
	mux.HandleFunc("DELETE /api/tts/local/manifest", h.handleLocalTTSManifestDelete)
}

func (h *Handler) handleLocalTTSModels(w http.ResponseWriter, r *http.Request) {
	manager := local.Default()
	installed := manager.Installed()
	byID := make(map[string]local.Installed, len(installed))
	for _, item := range installed {
		byID[item.RecipeID] = item
	}
	recipes, manifestErr := manager.Catalog()
	catalog := make([]localTTSCatalogItem, 0, len(recipes))
	for _, recipe := range recipes {
		dependencies := manager.Dependencies(recipe.Requires)
		missing := make([]string, 0, len(dependencies))
		for _, dependency := range dependencies {
			if !dependency.Available {
				missing = append(missing, dependency.Name)
			}
		}
		item := localTTSCatalogItem{
			ID:           recipe.ID,
			Name:         recipe.Name,
			Language:     recipe.Language,
			License:      recipe.License,
			LicenseURL:   recipe.LicenseURL,
			SourceURL:    recipe.SourceURL,
			HFRepo:       recipe.HFRepo,
			ThirdParty:   recipe.ThirdParty,
			SizeBytes:    recipe.SizeBytes,
			Voices:       recipe.Voices,
			DefaultVoice: recipe.DefaultVoice,
			Requires:     recipe.Requires,
			Dependencies: dependencies,
			Missing:      missing,
		}
		if got, ok := byID[recipe.ID]; ok {
			item.Installed = true
			item.Port = got.Port
			item.Running = manager.Healthy(got)
		}
		catalog = append(catalog, item)
	}
	userManifest, _ := manager.UserManifestJSON()
	response := localTTSModelsResponse{
		Environment:  manager.DetectPython(),
		Catalog:      catalog,
		Status:       redactLocalStatus(manager.Status()),
		UserManifest: userManifest,
	}
	if manifestErr != nil {
		response.ManifestError = manifestErr.Error()
	}
	writeJSON(w, http.StatusOK, response)
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
	manager := local.Default()
	recipe, err := manager.ResolveRecipe(req.URL)
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "unsupported_model", err.Error())
		return
	}
	if recipe.ThirdParty && strings.TrimSpace(req.AcceptLicense) != recipe.ID {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":    "license_acceptance_required",
			"error":   "Accept the model license before installing a third-party model.",
			"license": recipe.License,
			"source":  recipe.SourceURL,
		})
		return
	}
	dependencies := manager.Dependencies(recipe.Requires)
	missing := make([]string, 0)
	canInstall := true
	for _, dependency := range dependencies {
		if dependency.Available {
			continue
		}
		missing = append(missing, dependency.Name)
		if !dependency.CanInstall {
			canInstall = false
		}
	}
	if len(missing) > 0 {
		if !req.AcceptTools {
			code := "dependency_missing"
			message := fmt.Sprintf("Install the required tool first: %s", strings.Join(missing, ", "))
			if canInstall {
				code = "tool_license_required"
				message = "Download eSpeak NG 1.52.0 (GPL-3.0) before installing this model."
			}
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"code":         code,
				"error":        message,
				"dependencies": dependencies,
			})
			return
		}
		if !canInstall {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"code":         "dependency_missing",
				"error":        fmt.Sprintf("Install the required tool first: %s", strings.Join(missing, ", ")),
				"dependencies": dependencies,
			})
			return
		}
	}
	env := manager.DetectPython()
	if !env.Available {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code":        "python_missing",
			"error":       env.Message,
			"environment": env,
		})
		return
	}
	licenseAt := time.Time{}
	if recipe.ThirdParty {
		licenseAt = time.Now().UTC()
	}
	if err := manager.StartInstall(r.Context(), *recipe, licenseAt, req.AcceptTools, func(installed *local.Installed, installErr error) {
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

func (h *Handler) handleLocalTTSManifestGet(w http.ResponseWriter, r *http.Request) {
	manager := local.Default()
	raw, err := manager.UserManifestJSON()
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "manifest_read_failed", err.Error())
		return
	}
	models, _ := manager.Catalog()
	ids := make([]string, 0, len(models))
	for _, recipe := range models {
		ids = append(ids, recipe.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"json":   raw,
		"models": ids,
	})
}

func (h *Handler) handleLocalTTSManifestPut(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_request", "Could not read the manifest.")
		return
	}
	manifest, err := local.Default().SaveUserManifest(raw)
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_manifest", err.Error())
		return
	}
	ids := make([]string, 0, len(manifest.Models))
	for _, recipe := range manifest.Models {
		ids = append(ids, recipe.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "models": ids})
}

func (h *Handler) handleLocalTTSManifestDelete(w http.ResponseWriter, r *http.Request) {
	if err := local.Default().ClearUserManifest(); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "manifest_delete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
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
	if cfg.Voice.TTSLanguageModels == nil {
		cfg.Voice.TTSLanguageModels = make(map[string]string)
	}
	if language := strings.ToLower(strings.TrimSpace(installed.Language)); language != "" {
		cfg.Voice.TTSLanguageModels[language] = modelName
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
	for language, name := range cfg.Voice.TTSLanguageModels {
		if _, ok := removed[name]; ok {
			delete(cfg.Voice.TTSLanguageModels, language)
		}
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
