// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/channels"
)

const turnsAPIBasePath = "/turns/"

type activeTurnLister interface {
	ListActiveTurns() []agent.ActiveTurnInfo
}

type turnsAPIHandler struct {
	loop  activeTurnLister
	token string
}

type activeTurnDTO struct {
	SessionID   string `json:"session_id,omitempty"`
	SessionKey  string `json:"session_key"`
	ChatID      string `json:"chat_id"`
	AgentID     string `json:"agent_id"`
	TurnID      string `json:"turn_id"`
	Phase       string `json:"phase"`
	StartedAtMS int64  `json:"started_at_ms"`
}

func newTurnsAPIHandler(loop activeTurnLister, token string) http.Handler {
	return &turnsAPIHandler{loop: loop, token: token}
}

// registerTurnsAPI mounts the internal active-turn API on the shared gateway
// mux. The launcher uses it to show which sessions are currently working.
func registerTurnsAPI(manager *channels.Manager, loop activeTurnLister, token string) {
	if manager == nil || loop == nil {
		return
	}
	manager.Mount(turnsAPIBasePath, newTurnsAPIHandler(loop, token))
}

func (h *turnsAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeTurnsAPIError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing gateway token.")
		return
	}
	if h.loop == nil {
		writeTurnsAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "Turn monitor is unavailable.")
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(turnsAPIBasePath, "/")), "/")
	if path != "active" {
		writeTurnsAPIError(w, http.StatusNotFound, "not_found", "Endpoint not found.")
		return
	}
	if r.Method != http.MethodGet {
		writeTurnsAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		return
	}

	turns := h.loop.ListActiveTurns()
	items := make([]activeTurnDTO, 0, len(turns))
	for _, turn := range turns {
		sessionID := ""
		if strings.HasPrefix(turn.ChatID, "pico:") {
			sessionID = strings.TrimSpace(strings.TrimPrefix(turn.ChatID, "pico:"))
		}
		items = append(items, activeTurnDTO{
			SessionID:   sessionID,
			SessionKey:  turn.SessionKey,
			ChatID:      turn.ChatID,
			AgentID:     turn.AgentID,
			TurnID:      turn.TurnID,
			Phase:       string(turn.Phase),
			StartedAtMS: turn.StartedAt.UnixMilli(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"turns": items})
}

func (h *turnsAPIHandler) authorized(r *http.Request) bool {
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

func writeTurnsAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "error": message})
}
