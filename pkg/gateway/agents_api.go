// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/channels"
)

const agentsAPIBasePath = "/agents/"

// agentModelController is the part of agent.AgentLoop the targeted model API
// needs. Keeping it small makes the handler easy to test.
type agentModelController interface {
	GetAgentModel(agentID string) (string, bool)
	SwitchAgentModel(agentID, modelName string, wait time.Duration) (string, bool, error)
}

type agentsAPIHandler struct {
	loop  agentModelController
	token string
}

func newAgentsAPIHandler(loop agentModelController, token string) http.Handler {
	return &agentsAPIHandler{loop: loop, token: token}
}

// registerAgentsAPI mounts the internal per-agent model API on the shared
// gateway mux. The launcher proxies it behind dashboard auth and injects the
// process bearer token.
func registerAgentsAPI(manager *channels.Manager, loop agentModelController, token string) {
	if manager == nil || loop == nil {
		return
	}
	manager.Mount(agentsAPIBasePath, newAgentsAPIHandler(loop, token))
}

func (h *agentsAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeAgentsAPIError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing gateway token.")
		return
	}
	if h.loop == nil {
		writeAgentsAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "Agent model API is unavailable.")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(agentsAPIBasePath, "/"))
	path = strings.Trim(path, "/")
	segments := []string{}
	if path != "" {
		segments = strings.Split(path, "/")
	}

	if len(segments) != 2 || segments[1] != "model" {
		writeAgentsAPIError(w, http.StatusNotFound, "not_found", "Endpoint not found.")
		return
	}
	agentID := strings.TrimSpace(segments[0])
	if agentID == "" {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_agent", "Agent id is required.")
		return
	}

	switch r.Method {
	case http.MethodGet:
		model, ok := h.loop.GetAgentModel(agentID)
		if !ok {
			writeAgentsAPIError(w, http.StatusNotFound, "agent_not_found", "Agent not found.")
			return
		}
		writeAgentsJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"agent":  agentID,
			"model":  model,
		})
	case http.MethodPost:
		h.handleSwitch(w, r, agentID)
	default:
		writeAgentsAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
	}
}

func (h *agentsAPIHandler) handleSwitch(w http.ResponseWriter, r *http.Request, agentID string) {
	var req struct {
		Model string `json:"model"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON request body.")
		return
	}
	modelName := strings.TrimSpace(req.Model)
	if modelName == "" {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_model", "model is required.")
		return
	}

	oldModel, applied, err := h.loop.SwitchAgentModel(agentID, modelName, agent.DefaultModelSwitchWait)
	if err != nil {
		if errors.Is(err, agent.ErrAgentNotFound) {
			writeAgentsAPIError(w, http.StatusNotFound, "agent_not_found", "Agent not found.")
			return
		}
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_model", err.Error())
		return
	}

	if !applied {
		writeAgentsJSON(w, http.StatusAccepted, map[string]any{
			"status":  "pending",
			"agent":   agentID,
			"model":   modelName,
			"applied": false,
			"message": "Model switch queued; it applies when the current turn finishes.",
		})
		return
	}

	writeAgentsJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"agent":     agentID,
		"model":     modelName,
		"old_model": oldModel,
		"applied":   true,
	})
}

func (h *agentsAPIHandler) authorized(r *http.Request) bool {
	if h.token == "" {
		return false
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	presented := strings.TrimSpace(strings.TrimPrefix(auth, prefix))
	return subtle.ConstantTimeCompare([]byte(presented), []byte(h.token)) == 1
}

func writeAgentsJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAgentsAPIError(w http.ResponseWriter, status int, code, message string) {
	writeAgentsJSON(w, status, map[string]any{"code": code, "error": message})
}
