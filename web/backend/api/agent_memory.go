package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentpkg "github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/config"
)

type memorySettingsResponse struct {
	Enabled          bool   `json:"enabled"`
	Runtime          bool   `json:"runtime"`
	Default          bool   `json:"default"`
	AutoWrite        bool   `json:"auto_write"`
	AutoWriteRuntime bool   `json:"auto_write_runtime"`
	AutoWriteDefault bool   `json:"auto_write_default"`
	LastWriteAt      *int64 `json:"last_write_at,omitempty"`
}

type memorySettingsRequest struct {
	Enabled   *bool `json:"enabled"`
	AutoWrite *bool `json:"auto_write"`
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

	writeMemorySettings(w, cfg)
}

func (h *Handler) handleSetAgentMemory(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var req memorySettingsRequest
	if err := decoder.Decode(&req); err != nil ||
		(req.Enabled == nil && req.AutoWrite == nil) {
		writeSkillJSONError(
			w,
			http.StatusBadRequest,
			"invalid_request",
			`Expected {"enabled": true|false} and/or {"auto_write": true|false}.`,
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
	if req.Enabled != nil {
		cfg.Agents.Defaults.Memory.SetEnabled(*req.Enabled)
	}
	if req.AutoWrite != nil {
		cfg.Agents.Defaults.Memory.SetAutoWrite(*req.AutoWrite)
	}
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

	if req.Enabled != nil {
		agentpkg.SetMemoryEnabled(*req.Enabled)
	}
	if req.AutoWrite != nil {
		agentpkg.SetMemoryAutoWrite(*req.AutoWrite)
	}

	writeMemorySettings(w, cfg)
}

func writeMemorySettings(w http.ResponseWriter, cfg *config.Config) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(memorySettingsResponse{
		Enabled:          cfg.Agents.Defaults.Memory.IsEnabled(),
		Runtime:          agentpkg.MemoryEnabled(),
		Default:          true,
		AutoWrite:        cfg.Agents.Defaults.Memory.IsAutoWriteEnabled(),
		AutoWriteRuntime: agentpkg.MemoryAutoWriteEnabled(),
		AutoWriteDefault: false,
		LastWriteAt:      lastDailyMemoryWriteAt(cfg),
	})
}

// lastDailyMemoryWriteAt returns the modification time of today's daily note,
// or nil when the note does not exist yet.
func lastDailyMemoryWriteAt(cfg *config.Config) *int64 {
	workspace := strings.TrimSpace(cfg.WorkspacePath())
	if workspace == "" {
		return nil
	}
	dateStr := time.Now().Format("20060102")
	notePath := filepath.Join(workspace, "memory", dateStr[:6], dateStr+".md")
	info, err := os.Stat(notePath)
	if err != nil || info.IsDir() {
		return nil
	}
	unix := info.ModTime().Unix()
	return &unix
}
