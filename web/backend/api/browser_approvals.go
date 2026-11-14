package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/browser"
	"github.com/sipeed/picoclaw/pkg/config"
	ppid "github.com/sipeed/picoclaw/pkg/pid"
)

func (h *Handler) registerBrowserApprovalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/browser/approvals", h.handleListBrowserApprovals)
	mux.HandleFunc("POST /api/browser/approvals/{id}", h.handleResolveBrowserApproval)
}

func (h *Handler) handleListBrowserApprovals(w http.ResponseWriter, r *http.Request) {
	body, status, err := h.callGatewayApprovalAPI(http.MethodGet, "/browser/approvals", nil)
	if err != nil {
		writeBrowserSettingsError(w, status, "gateway_unavailable", err.Error())
		return
	}
	writeRawBrowserSettingsJSON(w, status, body)
}

func (h *Handler) handleResolveBrowserApproval(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_request", "Approval id is required.")
		return
	}

	var body struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}

	decision := browser.ApprovalDecision(strings.ToLower(strings.TrimSpace(body.Decision)))
	switch decision {
	case browser.AllowOnce, browser.AllowAlways, browser.Deny:
	default:
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_decision",
			"decision must be once, always or deny.")
		return
	}

	payload, _ := json.Marshal(map[string]string{"decision": string(decision)})
	responseBody, status, err := h.callGatewayApprovalAPI(
		http.MethodPost,
		"/browser/approvals/"+id,
		payload,
	)
	if err != nil {
		writeBrowserSettingsError(w, status, "gateway_unavailable", err.Error())
		return
	}

	if status == http.StatusOK && decision == browser.AllowAlways {
		var resolved struct {
			Domain string `json:"domain"`
		}
		if err := json.Unmarshal(responseBody, &resolved); err == nil && resolved.Domain != "" {
			if err := h.persistBrowserDomain(resolved.Domain); err != nil {
				writeBrowserSettingsError(w, http.StatusInternalServerError, "save_failed",
					"Access was granted, but saving the domain failed: "+err.Error())
				return
			}
		}
	}

	writeRawBrowserSettingsJSON(w, status, responseBody)
}

func (h *Handler) callGatewayApprovalAPI(method, path string, body []byte) ([]byte, int, error) {
	if !h.gatewayAvailableForProxy() {
		return nil, http.StatusServiceUnavailable, errGatewayNotRunning
	}
	pidData := ppid.ReadPidFileWithCheck(globalConfigDir())
	if pidData == nil || strings.TrimSpace(pidData.Token) == "" {
		return nil, http.StatusServiceUnavailable, errGatewayTokenMissing
	}

	target := h.gatewayProxyURL()
	target.Path = path
	req, err := http.NewRequest(method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Set("Authorization", "Bearer "+pidData.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return responseBody, resp.StatusCode, nil
}

func (h *Handler) persistBrowserDomain(domain string) error {
	domain = browser.NormalizeDomainPattern(domain)
	if domain == "" {
		return nil
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return err
	}
	for _, existing := range cfg.Tools.Browser.AllowedDomains {
		if browser.NormalizeDomainPattern(existing) == domain {
			return nil
		}
	}
	cfg.Tools.Browser.AllowedDomains = append(cfg.Tools.Browser.AllowedDomains, domain)
	return config.SaveConfig(h.configPath, cfg)
}

func writeRawBrowserSettingsJSON(w http.ResponseWriter, status int, body []byte) {
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

var (
	errGatewayNotRunning   = &gatewayUnavailableError{"The gateway is not running."}
	errGatewayTokenMissing = &gatewayUnavailableError{"The gateway session token is unavailable."}
)

type gatewayUnavailableError struct{ message string }

func (e *gatewayUnavailableError) Error() string { return e.message }
