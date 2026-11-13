package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	agentpkg "github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/config"
)

type memorySettingsResponse struct {
	Enabled bool `json:"enabled"`
	Runtime bool `json:"runtime"`
	Default bool `json:"default"`
}

type memorySettingsRequest struct {
	Enabled *bool `json:"enabled"`
}

func (h *Handler) registerAgentMemoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agent/memory", h.handleGetAgentMemory)
	mux.HandleFunc("PUT /api/agent/memory", h.handleSetAgentMemory)
}

func (h *Handler) handleGetAgentMemory(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeSkillJSONError(
			w,
			http.StatusInternalServerError,
			"config_error",
			"Failed to load configuration.",
			nil,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(memorySettingsResponse{
		Enabled: cfg.Agents.Defaults.Memory.IsEnabled(),
		Runtime: agentpkg.MemoryEnabled(),
		Default: true,
	})
}

func (h *Handler) handleSetAgentMemory(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var req memorySettingsRequest
	if err := decoder.Decode(&req); err != nil || req.Enabled == nil {
		writeSkillJSONError(
			w,
			http.StatusBadRequest,
			"invalid_request",
			`Expected {"enabled": true|false}.`,
			nil,
		)
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeSkillJSONError(
			w,
			http.StatusInternalServerError,
			"config_error",
			"Failed to load configuration.",
			nil,
		)
		return
	}
	cfg.Agents.Defaults.Memory.SetEnabled(*req.Enabled)
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeSkillJSONError(
			w,
			http.StatusInternalServerError,
			"config_error",
			fmt.Sprintf("Failed to save configuration: %v", err),
			nil,
		)
		return
	}

	agentpkg.SetMemoryEnabled(*req.Enabled)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(memorySettingsResponse{
		Enabled: *req.Enabled,
		Runtime: agentpkg.MemoryEnabled(),
		Default: true,
	})
}
