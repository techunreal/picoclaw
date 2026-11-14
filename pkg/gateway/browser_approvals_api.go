package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/browser"
	"github.com/sipeed/picoclaw/pkg/channels"
)

const browserApprovalBasePath = "/browser/approvals"

type browserApprovalAPI struct {
	store *browser.ApprovalStore
	token string
}

func registerBrowserApprovalAPI(manager *channels.Manager, token string) {
	if manager == nil {
		return
	}
	api := newBrowserApprovalAPI(browser.DefaultApprovals(), token)
	manager.Mount(browserApprovalBasePath+"/", api)
	// Exact list path (the mux supports exact entries as well as subtrees).
	manager.Mount(browserApprovalBasePath, api)
}

func newBrowserApprovalAPI(store *browser.ApprovalStore, token string) *browserApprovalAPI {
	return &browserApprovalAPI{store: store, token: token}
}

func (h *browserApprovalAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeBrowserApprovalError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing gateway token.")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, browserApprovalBasePath)
	path = strings.Trim(path, "/")

	if path == "" {
		if r.Method != http.MethodGet {
			writeBrowserApprovalError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
			return
		}
		writeBrowserApprovalJSON(w, http.StatusOK, map[string]any{"approvals": h.store.List()})
		return
	}

	if r.Method != http.MethodPost {
		writeBrowserApprovalError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		return
	}

	id := strings.Split(path, "/")[0]
	var body struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeBrowserApprovalError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}

	decision := browser.ApprovalDecision(strings.ToLower(strings.TrimSpace(body.Decision)))
	request, ok := h.store.Resolve(id, decision)
	if !ok {
		writeBrowserApprovalError(w, http.StatusNotFound, "not_found",
			"This approval request expired or was already answered.")
		return
	}
	writeBrowserApprovalJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"decision": decision,
		"domain":   request.Domain,
	})
}

func (h *browserApprovalAPI) authorized(r *http.Request) bool {
	if h.token == "" {
		return false
	}
	auth := r.Header.Get("Authorization")
	given, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok || given == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(given), []byte(h.token)) == 1
}

func writeBrowserApprovalJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeBrowserApprovalError(w http.ResponseWriter, status int, code, message string) {
	writeBrowserApprovalJSON(w, status, map[string]any{"code": code, "error": message})
}
