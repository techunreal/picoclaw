package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/browser"
	"github.com/sipeed/picoclaw/pkg/channels"
)

const browserSettingsAPIBasePath = "/browser/"

type browserSettingsAPIHandler struct {
	token string
}

type browserRuntimeSettings struct {
	AllowedDomains   []string `json:"allowed_domains"`
	AllowAllDomains  bool     `json:"allow_all_domains"`
	PermissionPolicy string   `json:"permission_policy"`
	AllowRemote      bool     `json:"allow_remote"`
}

func newBrowserSettingsAPIHandler(token string) http.Handler {
	return &browserSettingsAPIHandler{token: token}
}

// registerBrowserSettingsAPI mounts the internal endpoint that pushes browser
// allowlist and permission changes into the running gateway process. The browser
// tool and allowlist proxy are created at startup, so a config edit alone does
// not affect them until the process is restarted.
func registerBrowserSettingsAPI(manager *channels.Manager, token string) {
	if manager == nil {
		return
	}
	manager.Mount(browserSettingsAPIBasePath, newBrowserSettingsAPIHandler(token))
}

func (h *browserSettingsAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeBrowserSettingsAPIError(w, http.StatusUnauthorized, "unauthorized", "Invalid or missing gateway token.")
		return
	}
	if r.Method != http.MethodPut || r.URL.Path != "/browser/settings" {
		writeBrowserSettingsAPIError(w, http.StatusNotFound, "not_found", "Unknown browser settings endpoint.")
		return
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	decoder.DisallowUnknownFields()
	var req browserRuntimeSettings
	if err := decoder.Decode(&req); err != nil {
		writeBrowserSettingsAPIError(w, http.StatusBadRequest, "invalid_request", "Invalid browser settings payload.")
		return
	}

	if err := browser.ApplyRuntimeSettings(
		req.AllowedDomains,
		req.AllowAllDomains,
		req.PermissionPolicy,
		req.AllowRemote,
	); err != nil {
		writeBrowserSettingsAPIError(w, http.StatusBadRequest, "invalid_settings", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *browserSettingsAPIHandler) authorized(r *http.Request) bool {
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

func writeBrowserSettingsAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "error": message})
}
