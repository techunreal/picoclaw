package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/browser"
	"github.com/sipeed/picoclaw/pkg/config"
	ppid "github.com/sipeed/picoclaw/pkg/pid"
)

type browserToolStatus struct {
	BinaryFound     bool   `json:"binary_found"`
	BinaryPath      string `json:"binary_path,omitempty"`
	BinaryVersion   string `json:"binary_version,omitempty"`
	BrowserDetected bool   `json:"browser_detected"`
	BrowserPath     string `json:"browser_path,omitempty"`
	Error           string `json:"error,omitempty"`
}

type browserToolConfigResponse struct {
	Enabled          bool              `json:"enabled"`
	AllowedDomains   []string          `json:"allowed_domains"`
	AllowAllDomains  bool              `json:"allow_all_domains"`
	PermissionPolicy string            `json:"permission_policy"`
	BinaryPath       string            `json:"binary_path"`
	BrowserPath      string            `json:"browser_path"`
	Headless         bool              `json:"headless"`
	MaxOutput        int               `json:"max_output"`
	TimeoutSeconds   int               `json:"timeout_seconds"`
	AllowRemote      bool              `json:"allow_remote"`
	RestartRequired  bool              `json:"restart_required"`
	Status           browserToolStatus `json:"status"`
}

type browserToolConfigRequest struct {
	Enabled          bool     `json:"enabled"`
	AllowedDomains   []string `json:"allowed_domains"`
	AllowAllDomains  bool     `json:"allow_all_domains"`
	PermissionPolicy string   `json:"permission_policy"`
	BinaryPath       string   `json:"binary_path"`
	BrowserPath      string   `json:"browser_path"`
	Headless         bool     `json:"headless"`
	MaxOutput        int      `json:"max_output"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
	AllowRemote      bool     `json:"allow_remote"`
}

func (h *Handler) registerBrowserSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tools/browser", h.handleGetBrowserConfig)
	mux.HandleFunc("PUT /api/tools/browser", h.handleUpdateBrowserConfig)
	mux.HandleFunc("POST /api/tools/browser/test", h.handleTestBrowser)
	mux.HandleFunc("POST /api/tools/browser/install", h.handleInstallBrowserEngine)
	mux.HandleFunc("GET /api/tools/browser/install", h.handleBrowserInstallStatus)
}

func (h *Handler) handleGetBrowserConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "config_unreadable",
			"Failed to load config: "+err.Error())
		return
	}
	writeBrowserSettingsJSON(w, http.StatusOK, buildBrowserConfigResponse(cfg, true))
}

func (h *Handler) handleUpdateBrowserConfig(w http.ResponseWriter, r *http.Request) {
	var req browserToolConfigRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}

	domains, err := normalizeBrowserDomains(req.AllowedDomains)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_domains", err.Error())
		return
	}
	policy := strings.ToLower(strings.TrimSpace(req.PermissionPolicy))
	if policy == "" {
		if req.AllowAllDomains {
			policy = "allow"
		} else {
			policy = "ask"
		}
	}
	switch policy {
	case "ask", "allow", "deny":
	default:
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_permission_policy",
			"permission_policy must be ask, allow or deny.")
		return
	}
	if req.Enabled && policy != "allow" && len(domains) == 0 {
		writeBrowserSettingsError(w, http.StatusBadRequest, "requires_allowed_domains",
			"Add at least one allowed domain before enabling the browser tool, or choose Allow all sites.")
		return
	}
	if req.Enabled {
		if _, err := browser.LocateBinary(req.BinaryPath); err != nil {
			writeBrowserSettingsError(w, http.StatusBadRequest, "agent_browser_missing",
				"agent-browser was not found. Install it or set the binary path.")
			return
		}
	}
	if req.BinaryPath != "" && !browser.IsExecutableFile(req.BinaryPath) {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_binary_path",
			"The configured agent-browser executable does not exist.")
		return
	}
	if req.BrowserPath != "" {
		if !browser.IsExecutableFile(req.BrowserPath) {
			writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_browser_path",
				"The configured browser executable does not exist.")
			return
		}
	}
	if req.MaxOutput < 0 || req.MaxOutput > 1_000_000 {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_max_output",
			"Max output must be between 0 and 1,000,000 characters.")
		return
	}
	if req.TimeoutSeconds < 0 || req.TimeoutSeconds > 600 {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_timeout",
			"Timeout must be between 0 and 600 seconds.")
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "config_unreadable",
			"Failed to load config: "+err.Error())
		return
	}

	cfg.Tools.Browser.Enabled = req.Enabled
	cfg.Tools.Browser.AllowedDomains = domains
	cfg.Tools.Browser.AllowAllDomains = policy == "allow"
	cfg.Tools.Browser.PermissionPolicy = policy
	cfg.Tools.Browser.BinaryPath = strings.TrimSpace(req.BinaryPath)
	cfg.Tools.Browser.BrowserPath = strings.TrimSpace(req.BrowserPath)
	cfg.Tools.Browser.Headless = boolPointer(req.Headless)
	cfg.Tools.Browser.MaxOutput = req.MaxOutput
	cfg.Tools.Browser.TimeoutSeconds = req.TimeoutSeconds
	cfg.Tools.Browser.AllowRemote = req.AllowRemote

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "save_failed",
			"Failed to save config: "+err.Error())
		return
	}

	restartRequired := false
	if h.gatewayAvailableForProxy() {
		// The browser tool and allowlist proxy are created at gateway startup and
		// a runtime push did not reliably replace the permission policy, so apply
		// the new restrictions by restarting the gateway now.
		if _, err := h.RestartGateway(); err != nil {
			restartRequired = true
		}
	}

	response := buildBrowserConfigResponse(cfg, false)
	response.RestartRequired = restartRequired
	writeBrowserSettingsJSON(w, http.StatusOK, response)
}

// applyBrowserRuntimeSettings pushes allowlist and permission changes into the
// running gateway so browser restrictions apply without a restart.
func (h *Handler) applyBrowserRuntimeSettings(cfg *config.Config) error {
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

	settings := cfg.Tools.Browser
	payload, err := json.Marshal(map[string]any{
		"allowed_domains":   settings.AllowedDomains,
		"allow_all_domains": settings.IsAllowAllDomains(),
		"permission_policy": settings.UnknownDomainPolicy(),
		"allow_remote":      settings.AllowRemote,
	})
	if err != nil {
		return err
	}

	endpoint := strings.TrimRight(target.String(), "/") + "/browser/settings"
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

func (h *Handler) handleInstallBrowserEngine(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "config_unreadable",
			"Failed to load config: "+err.Error())
		return
	}
	if err := browser.StartBrowserInstall(cfg.Tools.Browser.BinaryPath); err != nil {
		if errors.Is(err, browser.ErrInstallRunning) {
			writeBrowserSettingsError(w, http.StatusConflict, "install_running",
				"A browser download is already running.")
			return
		}
		writeBrowserSettingsError(w, http.StatusBadRequest, "install_failed", err.Error())
		return
	}
	writeBrowserSettingsJSON(w, http.StatusAccepted, browser.BrowserInstallStatus())
}

func (h *Handler) handleBrowserInstallStatus(w http.ResponseWriter, r *http.Request) {
	writeBrowserSettingsJSON(w, http.StatusOK, browser.BrowserInstallStatus())
}

func (h *Handler) handleTestBrowser(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "config_unreadable",
			"Failed to load config: "+err.Error())
		return
	}

	version, err := browser.Version(cfg.Tools.Browser.BinaryPath)
	if err != nil {
		writeBrowserSettingsJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	writeBrowserSettingsJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": version,
	})
}

func buildBrowserConfigResponse(cfg *config.Config, includeStatus bool) browserToolConfigResponse {
	settings := cfg.Tools.Browser
	response := browserToolConfigResponse{
		Enabled:          settings.Enabled,
		AllowedDomains:   append([]string(nil), settings.AllowedDomains...),
		AllowAllDomains:  settings.IsAllowAllDomains(),
		PermissionPolicy: settings.UnknownDomainPolicy(),
		BinaryPath:       settings.BinaryPath,
		BrowserPath:      settings.BrowserPath,
		Headless:         settings.IsHeadless(),
		MaxOutput:        settings.GetMaxOutput(),
		TimeoutSeconds:   settings.GetTimeoutSeconds(),
		AllowRemote:      settings.AllowRemote,
	}
	if response.AllowedDomains == nil {
		response.AllowedDomains = []string{}
	}
	if !includeStatus {
		return response
	}

	if path, err := browser.LocateBinary(settings.BinaryPath); err == nil {
		response.Status.BinaryFound = true
		response.Status.BinaryPath = path
		if version, versionErr := browser.Version(settings.BinaryPath); versionErr == nil {
			response.Status.BinaryVersion = version
		} else {
			response.Status.Error = versionErr.Error()
		}
	} else {
		response.Status.Error = err.Error()
	}

	if path := browser.LocateBrowser(settings.BrowserPath); path != "" {
		response.Status.BrowserDetected = true
		response.Status.BrowserPath = path
	}
	return response
}

func normalizeBrowserDomains(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := browser.NormalizeDomainPattern(raw)
		if value == "" {
			continue
		}
		if len(value) > 253 {
			return nil, fmt.Errorf("domain is too long: %s", value)
		}
		if strings.ContainsAny(value, " \t\r\n/") {
			return nil, fmt.Errorf("invalid domain: %s", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) > 50 {
			return nil, fmt.Errorf("at most 50 allowed domains are supported")
		}
	}
	return result, nil
}

func boolPointer(value bool) *bool {
	return &value
}

func writeBrowserSettingsJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeBrowserSettingsError(w http.ResponseWriter, status int, code, message string) {
	writeBrowserSettingsJSON(w, status, map[string]any{"code": code, "error": message})
}
