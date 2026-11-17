package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// firewallRuleNamePrefix is used with the configured port, e.g.
// "PicoClaw WebUI (18800)".
const firewallRuleNamePrefix = "PicoClaw WebUI"

// firewallStatusResponse tells the UI whether the inbound rule exists.
type firewallStatusResponse struct {
	Supported bool   `json:"supported"`
	Present   bool   `json:"present"`
	RuleName  string `json:"rule_name"`
	Port      int    `json:"port"`
}

type firewallActionRequest struct {
	Action string `json:"action"`
}

var (
	firewallStatusMu      sync.Mutex
	firewallStatusCache   firewallStatusResponse
	firewallStatusExpires time.Time
)

func (h *Handler) registerFirewallRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/system/firewall", h.handleFirewallStatus)
	mux.HandleFunc("POST /api/system/firewall", h.handleFirewallAction)
}

func (h *Handler) firewallPort() int {
	if h.serverPort > 0 {
		return h.serverPort
	}
	return 18800
}

func (h *Handler) firewallRuleName() string {
	return fmt.Sprintf("%s (%d)", firewallRuleNamePrefix, h.firewallPort())
}

func (h *Handler) handleFirewallStatus(w http.ResponseWriter, r *http.Request) {
	port := h.firewallPort()
	name := h.firewallRuleName()

	firewallStatusMu.Lock()
	if time.Now().Before(firewallStatusExpires) && firewallStatusCache.Port == port {
		cached := firewallStatusCache
		firewallStatusMu.Unlock()
		cached.Supported = true
		cached.RuleName = name
		writeBrowserSettingsJSON(w, http.StatusOK, cached)
		return
	}
	firewallStatusMu.Unlock()

	present, err := queryFirewallRule(name)
	if err != nil {
		writeBrowserSettingsJSON(w, http.StatusOK, firewallStatusResponse{
			Supported: false,
			Present:   false,
			RuleName:  name,
			Port:      port,
		})
		return
	}

	response := firewallStatusResponse{
		Supported: true,
		Present:   present,
		RuleName:  name,
		Port:      port,
	}

	firewallStatusMu.Lock()
	firewallStatusCache = response
	firewallStatusExpires = time.Now().Add(3 * time.Second)
	firewallStatusMu.Unlock()

	writeBrowserSettingsJSON(w, http.StatusOK, response)
}

func (h *Handler) handleFirewallAction(w http.ResponseWriter, r *http.Request) {
	var req firewallActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "allow" && action != "remove" {
		writeBrowserSettingsError(w, http.StatusBadRequest, "invalid_action",
			"action must be allow or remove.")
		return
	}

	port := h.firewallPort()
	name := h.firewallRuleName()
	script := buildFirewallScript(name, port, action)

	// Windows Firewall changes need elevation. Launch one elevated PowerShell
	// process; the user confirms the UAC prompt and the rule is applied.
	encoded := encodePowerShell(script)
	outer := fmt.Sprintf(
		"Start-Process -FilePath powershell.exe -Verb RunAs -ArgumentList '-NoProfile','-NonInteractive','-EncodedCommand','%s'",
		encoded,
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", outer)
	if err := cmd.Start(); err != nil {
		writeBrowserSettingsError(w, http.StatusInternalServerError, "elevation_failed",
			"Could not start the Windows elevation prompt: "+err.Error())
		return
	}

	firewallStatusMu.Lock()
	firewallStatusExpires = time.Time{}
	firewallStatusMu.Unlock()

	writeBrowserSettingsJSON(w, http.StatusAccepted, map[string]string{
		"status": "prompted",
		"action": action,
		"rule":   name,
	})
}

func queryFirewallRule(name string) (bool, error) {
	script := fmt.Sprintf(
		"$r = Get-NetFirewallRule -DisplayName %s -ErrorAction SilentlyContinue; "+
			"if ($r) { 'PRESENT' } else { 'MISSING' }",
		psQuote(name),
	)
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), "PRESENT"), nil
}

func buildFirewallScript(name string, port int, action string) string {
	quotedName := psQuote(name)
	if action == "remove" {
		return fmt.Sprintf(
			"$n = %s; "+
				"$r = Get-NetFirewallRule -DisplayName $n -ErrorAction SilentlyContinue; "+
				"if ($r) { Remove-NetFirewallRule -DisplayName $n -ErrorAction SilentlyContinue }",
			quotedName,
		)
	}
	return fmt.Sprintf(
		"$n = %s; "+
			"if (-not (Get-NetFirewallRule -DisplayName $n -ErrorAction SilentlyContinue)) { "+
			"New-NetFirewallRule -DisplayName $n "+
			"-Description 'Allow PicoClaw WebUI from the local network' "+
			"-Direction Inbound -Action Allow -Protocol TCP -LocalPort %d "+
			"-RemoteAddress LocalSubnet -Profile Private,Public | Out-Null }",
		quotedName, port,
	)
}

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func encodePowerShell(script string) string {
	// -EncodedCommand expects UTF-16LE base64.
	utf16 := make([]uint16, 0, len(script))
	for _, r := range script {
		if r <= 0xFFFF {
			utf16 = append(utf16, uint16(r))
			continue
		}
		hi, lo := utf16EncodeSurrogate(r)
		utf16 = append(utf16, hi, lo)
	}
	raw := make([]byte, len(utf16)*2)
	for i, value := range utf16 {
		raw[i*2] = byte(value)
		raw[i*2+1] = byte(value >> 8)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func utf16EncodeSurrogate(r rune) (uint16, uint16) {
	r -= 0x10000
	return uint16(0xD800 + (r >> 10)), uint16(0xDC00 + (r & 0x3FF))
}
