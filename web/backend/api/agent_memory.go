package api

import (
	"bytes"
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
	ppid "github.com/sipeed/picoclaw/pkg/pid"
)

type memorySettingsResponse struct {
	Enabled          bool   `json:"enabled"`
	Runtime          bool   `json:"runtime"`
	Default          bool   `json:"default"`
	AutoWrite        bool   `json:"auto_write"`
	AutoWriteRuntime bool   `json:"auto_write_runtime"`
	AutoWriteDefault bool   `json:"auto_write_default"`
	LastWriteAt      *int64 `json:"last_write_at,omitempty"`
	// RestartRequired is true when the running gateway could not apply the
	// change; the setting is persisted and takes effect after a restart.
	RestartRequired bool `json:"restart_required"`
}

type memorySettingsRequest struct {
	Enabled   *bool `json:"enabled"`
	AutoWrite *bool `json:"auto_write"`
}

type memoryRuntimeState struct {
	enabled   bool
	autoWrite bool
	// fromGateway is true when the values came from the running gateway.
	fromGateway bool
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

	writeMemorySettings(w, cfg, h.fetchMemoryRuntimeState(), false)
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

	restartRequired := false
	if err := h.applyMemoryRuntimeState(req.Enabled, req.AutoWrite); err != nil {
		restartRequired = true
	}

	writeMemorySettings(w, cfg, h.fetchMemoryRuntimeState(), restartRequired)
}

// applyMemoryRuntimeState forwards the switch values to the running gateway.
// The agent loop lives in a separate process, so flipping process-local flags in
// the launcher has no effect on it.
func (h *Handler) applyMemoryRuntimeState(enabled, autoWrite *bool) error {
	if enabled == nil && autoWrite == nil {
		return nil
	}
	if !h.gatewayAvailableForProxy() {
		return fmt.Errorf("gateway unavailable")
	}
	pidData := ppid.ReadPidFileWithCheck(globalConfigDir())
	if pidData == nil || strings.TrimSpace(pidData.Token) == "" {
		return fmt.Errorf("gateway token unavailable")
	}
	target := h.gatewayProxyURL()
	if target == nil {
		return fmt.Errorf("gateway URL unavailable")
	}

	payload, err := json.Marshal(map[string]*bool{
		"enabled":    enabled,
		"auto_write": autoWrite,
	})
	if err != nil {
		return err
	}

	endpoint := strings.TrimRight(target.String(), "/") + "/memory/state"
	request, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+pidData.Token)

	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway status %d", response.StatusCode)
	}
	return nil
}

// fetchMemoryRuntimeState reads the live switches from the gateway. When the
// gateway is unavailable it falls back to the launcher's process-local flags so
// the response shape stays stable.
func (h *Handler) fetchMemoryRuntimeState() memoryRuntimeState {
	if !h.gatewayAvailableForProxy() {
		return memoryRuntimeState{
			enabled:   agentpkg.MemoryEnabled(),
			autoWrite: agentpkg.MemoryAutoWriteEnabled(),
		}
	}
	pidData := ppid.ReadPidFileWithCheck(globalConfigDir())
	if pidData == nil || strings.TrimSpace(pidData.Token) == "" {
		return memoryRuntimeState{
			enabled:   agentpkg.MemoryEnabled(),
			autoWrite: agentpkg.MemoryAutoWriteEnabled(),
		}
	}
	target := h.gatewayProxyURL()
	if target == nil {
		return memoryRuntimeState{
			enabled:   agentpkg.MemoryEnabled(),
			autoWrite: agentpkg.MemoryAutoWriteEnabled(),
		}
	}

	endpoint := strings.TrimRight(target.String(), "/") + "/memory/state"
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return memoryRuntimeState{}
	}
	request.Header.Set("Authorization", "Bearer "+pidData.Token)

	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return memoryRuntimeState{}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return memoryRuntimeState{}
	}

	var state struct {
		Enabled   bool `json:"enabled"`
		AutoWrite bool `json:"auto_write"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<16)).Decode(&state); err != nil {
		return memoryRuntimeState{}
	}
	return memoryRuntimeState{
		enabled:     state.Enabled,
		autoWrite:   state.AutoWrite,
		fromGateway: true,
	}
}

func writeMemorySettings(
	w http.ResponseWriter,
	cfg *config.Config,
	runtime memoryRuntimeState,
	restartRequired bool,
) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(memorySettingsResponse{
		Enabled:          cfg.Agents.Defaults.Memory.IsEnabled(),
		Runtime:          runtime.enabled,
		Default:          true,
		AutoWrite:        cfg.Agents.Defaults.Memory.IsAutoWriteEnabled(),
		AutoWriteRuntime: runtime.autoWrite,
		AutoWriteDefault: false,
		LastWriteAt:      lastDailyMemoryWriteAt(cfg),
		RestartRequired:  restartRequired,
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
