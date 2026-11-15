package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/channels"
)

const memoryAPIBasePath = "/memory/"

type memoryAPIHandler struct {
	token string
}

type memoryStateResponse struct {
	Enabled   bool `json:"enabled"`
	AutoWrite bool `json:"auto_write"`
}

type memoryStateRequest struct {
	Enabled   *bool `json:"enabled"`
	AutoWrite *bool `json:"auto_write"`
}

func newMemoryAPIHandler(token string) http.Handler {
	return &memoryAPIHandler{token: token}
}

// registerMemoryAPI mounts the internal runtime memory switch API on the shared
// gateway mux. The agent loop lives in the gateway process, so the launcher must
// forward memory toggles here instead of flipping process-local flags.
func registerMemoryAPI(manager *channels.Manager, token string) {
	if manager == nil {
		return
	}
	manager.Mount(memoryAPIBasePath, newMemoryAPIHandler(token))
}

func (h *memoryAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeMemoryAPIError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing gateway token.")
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/memory/state":
		writeMemoryAPIState(w)
	case r.Method == http.MethodPut && r.URL.Path == "/memory/state":
		h.handleSetState(w, r)
	default:
		writeMemoryAPIError(w, http.StatusNotFound, "not_found", "Unknown memory endpoint.")
	}
}

func (h *memoryAPIHandler) handleSetState(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	decoder.DisallowUnknownFields()
	var req memoryStateRequest
	if err := decoder.Decode(&req); err != nil || (req.Enabled == nil && req.AutoWrite == nil) {
		writeMemoryAPIError(w, http.StatusBadRequest, "invalid_request",
			`Expected {"enabled": true|false} and/or {"auto_write": true|false}.`)
		return
	}

	if req.Enabled != nil {
		agent.SetMemoryEnabled(*req.Enabled)
	}
	if req.AutoWrite != nil {
		agent.SetMemoryAutoWrite(*req.AutoWrite)
	}

	writeMemoryAPIState(w)
}

func (h *memoryAPIHandler) authorized(r *http.Request) bool {
	token := strings.TrimSpace(h.token)
	if token == "" {
		return false
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	provided := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func writeMemoryAPIState(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(memoryStateResponse{
		Enabled:   agent.MemoryEnabled(),
		AutoWrite: agent.MemoryAutoWriteEnabled(),
	})
}

func writeMemoryAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "error": message})
}
