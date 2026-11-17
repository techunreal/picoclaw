package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/sipeed/picoclaw/web/backend/launcherconfig"
)

type launcherConfigPayload struct {
	Port                 int      `json:"port"`
	Public               bool     `json:"public"`
	AllowedCIDRs         []string `json:"allowed_cidrs"`
	AllowLocalhostBypass bool     `json:"allow_localhost_bypass"`
	TrustedProxyCIDRs    []string `json:"trusted_proxy_cidrs"`
	RestartRequired      bool     `json:"restart_required"`
}

type launcherConfigUpdatePayload struct {
	Port                 int      `json:"port"`
	Public               bool     `json:"public"`
	AllowedCIDRs         []string `json:"allowed_cidrs"`
	AllowLocalhostBypass *bool    `json:"allow_localhost_bypass"`
	TrustedProxyCIDRs    []string `json:"trusted_proxy_cidrs"`
}

func (h *Handler) registerLauncherConfigRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/system/launcher-config", h.handleGetLauncherConfig)
	mux.HandleFunc("PUT /api/system/launcher-config", h.handleUpdateLauncherConfig)
	mux.HandleFunc("POST /api/system/launcher/restart", h.handleRestartLauncher)
}

func (h *Handler) launcherConfigPath() string {
	return launcherconfig.PathForAppConfig(h.configPath)
}

func (h *Handler) launcherFallbackConfig() launcherconfig.Config {
	port := h.serverPort
	if port <= 0 {
		port = launcherconfig.DefaultPort
	}
	return launcherconfig.Config{
		Port:                 port,
		Public:               h.serverPublic,
		AllowedCIDRs:         append([]string(nil), h.serverCIDRs...),
		AllowLocalhostBypass: h.serverAllowLocalhostBypass,
		TrustedProxyCIDRs:    append([]string(nil), h.serverTrustedProxyCIDRs...),
	}
}

func (h *Handler) loadLauncherConfig() (launcherconfig.Config, error) {
	return launcherconfig.Load(h.launcherConfigPath(), h.launcherFallbackConfig())
}

func (h *Handler) handleGetLauncherConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadLauncherConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load launcher config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(launcherConfigPayload{
		Port:                 cfg.Port,
		Public:               cfg.Public,
		AllowedCIDRs:         append([]string(nil), cfg.AllowedCIDRs...),
		AllowLocalhostBypass: cfg.AllowLocalhostBypass,
		TrustedProxyCIDRs:    append([]string(nil), cfg.TrustedProxyCIDRs...),
		RestartRequired:      h.launcherRestartRequired(cfg),
	})
}

func (h *Handler) handleUpdateLauncherConfig(w http.ResponseWriter, r *http.Request) {
	var payload launcherConfigUpdatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	cfg, err := h.loadLauncherConfig()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load launcher config: %v", err), http.StatusInternalServerError)
		return
	}
	cfg.Port = payload.Port
	cfg.Public = payload.Public
	cfg.AllowedCIDRs = append([]string(nil), payload.AllowedCIDRs...)
	if payload.AllowLocalhostBypass != nil {
		cfg.AllowLocalhostBypass = *payload.AllowLocalhostBypass
	}
	cfg.TrustedProxyCIDRs = append([]string(nil), payload.TrustedProxyCIDRs...)
	cfg.LegacyLauncherToken = ""
	if err := launcherconfig.Validate(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := launcherconfig.Save(h.launcherConfigPath(), cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save launcher config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(launcherConfigPayload{
		Port:                 cfg.Port,
		Public:               cfg.Public,
		AllowedCIDRs:         append([]string(nil), cfg.AllowedCIDRs...),
		AllowLocalhostBypass: cfg.AllowLocalhostBypass,
		TrustedProxyCIDRs:    append([]string(nil), cfg.TrustedProxyCIDRs...),
		RestartRequired:      h.launcherRestartRequired(cfg),
	})
}

// launcherRestartRequired reports whether the saved launcher settings differ
// from what this process is currently listening on. Port and LAN changes are
// only applied when the launcher restarts.
func (h *Handler) launcherRestartRequired(cfg launcherconfig.Config) bool {
	if h.serverPort > 0 && cfg.Port != h.serverPort {
		return true
	}
	if cfg.Public != h.serverPublic {
		return true
	}
	if !equalStringSlices(cfg.AllowedCIDRs, h.serverCIDRs) {
		return true
	}
	if cfg.AllowLocalhostBypass != h.serverAllowLocalhostBypass {
		return true
	}
	if !equalStringSlices(cfg.TrustedProxyCIDRs, h.serverTrustedProxyCIDRs) {
		return true
	}
	return false
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// handleRestartLauncher starts a fresh launcher process and then exits this
// one so port/LAN changes take effect. The replacement is launched through the
// shell so it is not tied to this process (or its job object); the new process
// retries binding for a few seconds while this one releases the port.
func (h *Handler) handleRestartLauncher(w http.ResponseWriter, r *http.Request) {
	exe, err := os.Executable()
	if err != nil {
		http.Error(w, "cannot locate the launcher executable", http.StatusInternalServerError)
		return
	}

	writeBrowserSettingsJSON(w, http.StatusOK, map[string]string{"status": "restarting"})

	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := launchReplacement(exe); err != nil {
			// Nothing else to do: the user can still start the launcher manually.
			return
		}
		time.Sleep(900 * time.Millisecond)
		h.Shutdown()
		os.Exit(0)
	}()
}
