package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/channels"
)

const subagentsAPIBasePath = "/subagents/"

// subagentRunController is the part of agent.AgentLoop the monitor API needs.
type subagentRunController interface {
	ListSubagentRuns() []agent.SubagentRun
	CancelSubTurn(id string) bool
}

type subagentsAPIHandler struct {
	loop  subagentRunController
	token string
}

func newSubagentsAPIHandler(loop subagentRunController, token string) http.Handler {
	return &subagentsAPIHandler{loop: loop, token: token}
}

// registerSubagentsAPI mounts the internal subagent monitor API on the shared
// gateway mux. The launcher proxies these endpoints behind dashboard auth and
// injects the process bearer token.
func registerSubagentsAPI(manager *channels.Manager, loop subagentRunController, token string) {
	if manager == nil || loop == nil {
		return
	}
	manager.Mount(subagentsAPIBasePath, newSubagentsAPIHandler(loop, token))
}

func (h *subagentsAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeSubagentsAPIError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing gateway token.")
		return
	}
	if h.loop == nil {
		writeSubagentsAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "Subagent monitor is unavailable.")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(subagentsAPIBasePath, "/"))
	path = strings.Trim(path, "/")
	segments := []string{}
	if path != "" {
		segments = strings.Split(path, "/")
	}

	switch {
	case len(segments) == 1 && segments[0] == "runs":
		if r.Method != http.MethodGet {
			writeSubagentsAPIMethodNotAllowed(w)
			return
		}
		h.handleList(w)
	case len(segments) == 3 && segments[0] == "runs" && segments[2] == "cancel":
		if r.Method != http.MethodPost {
			writeSubagentsAPIMethodNotAllowed(w)
			return
		}
		h.handleCancel(w, segments[1])
	default:
		writeSubagentsAPIError(w, http.StatusNotFound, "not_found", "Endpoint not found.")
	}
}

func (h *subagentsAPIHandler) handleList(w http.ResponseWriter) {
	runs := h.loop.ListSubagentRuns()
	if runs == nil {
		runs = []agent.SubagentRun{}
	}
	writeSubagentsJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (h *subagentsAPIHandler) handleCancel(w http.ResponseWriter, id string) {
	id = strings.TrimSpace(id)
	if id == "" || !h.loop.CancelSubTurn(id) {
		writeSubagentsAPIError(w, http.StatusConflict, "not_active",
			"The subagent is no longer running. Refresh the list and try again.")
		return
	}
	writeSubagentsJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (h *subagentsAPIHandler) authorized(r *http.Request) bool {
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

func writeSubagentsJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeSubagentsAPIError(w http.ResponseWriter, status int, code, message string) {
	writeSubagentsJSON(w, status, map[string]any{"code": code, "error": message})
}

func writeSubagentsAPIMethodNotAllowed(w http.ResponseWriter) {
	writeSubagentsAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
}
