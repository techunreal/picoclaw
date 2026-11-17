package api

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestLauncherConfigRestartRequiredAfterLANChange(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)
	h.SetServerOptions(18800, false, false, nil)
	h.SetServerAccessOptions(true, nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := strings.NewReader(
		`{"port":18800,"public":true,"allowed_cidrs":[],"allow_localhost_bypass":true,"trusted_proxy_cidrs":[]}`,
	)
	req := httptest.NewRequest(http.MethodPut, "/api/system/launcher-config", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var saved launcherConfigPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("unmarshal PUT response: %v", err)
	}
	if !saved.Public || !saved.RestartRequired {
		t.Fatalf("PUT response = %+v, want public=true restart_required=true", saved)
	}

	recGet := httptest.NewRecorder()
	mux.ServeHTTP(recGet, httptest.NewRequest(http.MethodGet, "/api/system/launcher-config", nil))
	if recGet.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s", recGet.Code, recGet.Body.String())
	}
	var got launcherConfigPayload
	if err := json.Unmarshal(recGet.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal GET response: %v", err)
	}
	if !got.RestartRequired {
		t.Fatalf("GET restart_required = false, want true after public change")
	}
}

func TestFirewallScripts(t *testing.T) {
	allow := buildFirewallScript("PicoClaw WebUI (18800)", 18800, "allow")
	if !strings.Contains(allow, "New-NetFirewallRule") {
		t.Fatalf("allow script missing New-NetFirewallRule: %s", allow)
	}
	if !strings.Contains(allow, "-LocalPort 18800") {
		t.Fatalf("allow script missing port: %s", allow)
	}
	if !strings.Contains(allow, "LocalSubnet") {
		t.Fatalf("allow script missing LocalSubnet restriction: %s", allow)
	}

	remove := buildFirewallScript("PicoClaw WebUI (18800)", 18800, "remove")
	if !strings.Contains(remove, "Remove-NetFirewallRule") {
		t.Fatalf("remove script missing Remove-NetFirewallRule: %s", remove)
	}
}

func TestFirewallPSQuote(t *testing.T) {
	if got := psQuote("O'Brien"); got != "'O''Brien'" {
		t.Fatalf("psQuote = %q, want escaped quotes", got)
	}
}

func TestEncodePowerShellRoundTrip(t *testing.T) {
	script := "Write-Output 'merhaba dünya ✔'"
	encoded := encodePowerShell(script)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if len(raw)%2 != 0 {
		t.Fatalf("decoded length is not a multiple of 2")
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	if got := string(utf16.Decode(units)); got != script {
		t.Fatalf("round trip = %q, want %q", got, script)
	}
}

func TestFirewallActionValidation(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/system/firewall",
		bytes.NewBufferString(`{"action":"nuke"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}
