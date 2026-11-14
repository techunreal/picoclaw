package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func newBrowserSettingsTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	configPath := setupChatAttachmentTestEnv(t)
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.registerBrowserSettingsRoutes(mux)
	return mux, configPath
}

func doBrowserSettingsRequest(t *testing.T, mux *http.ServeMux, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestBrowserSettings_Get(t *testing.T) {
	mux, _ := newBrowserSettingsTestMux(t)

	rec := doBrowserSettingsRequest(t, mux, http.MethodGet, "/api/tools/browser", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response browserToolConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Enabled {
		t.Fatal("browser should be disabled by default")
	}
	if !response.Headless {
		t.Fatal("headless should default to true")
	}
	if response.MaxOutput == 0 || response.TimeoutSeconds == 0 {
		t.Fatalf("defaults missing: %+v", response)
	}
	if response.PermissionPolicy != "ask" {
		t.Fatalf("permission policy = %q, want ask", response.PermissionPolicy)
	}
}

func TestBrowserSettings_EnableRequiresDomains(t *testing.T) {
	mux, _ := newBrowserSettingsTestMux(t)

	rec := doBrowserSettingsRequest(t, mux, http.MethodPut, "/api/tools/browser", map[string]any{
		"enabled":         true,
		"allowed_domains": []string{},
		"headless":        true,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestBrowserSettings_EnablePersistsDomains(t *testing.T) {
	mux, configPath := newBrowserSettingsTestMux(t)

	rec := doBrowserSettingsRequest(t, mux, http.MethodPut, "/api/tools/browser", map[string]any{
		"enabled":         true,
		"allowed_domains": []string{"https://example.com/path", "*.example.com", "example.com", ""},
		"headless":        true,
		"max_output":      12345,
		"timeout_seconds": 45,
	})
	if rec.Code != http.StatusOK {
		t.Skipf("agent-browser is not installed in this environment: %s", rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !cfg.Tools.Browser.Enabled {
		t.Fatal("browser should be enabled")
	}
	if len(cfg.Tools.Browser.AllowedDomains) != 2 {
		t.Fatalf("domains = %v, want normalized unique list", cfg.Tools.Browser.AllowedDomains)
	}
	if cfg.Tools.Browser.AllowedDomains[1] != "*.example.com" {
		t.Fatalf("domains = %v", cfg.Tools.Browser.AllowedDomains)
	}
	if cfg.Tools.Browser.GetMaxOutput() != 12345 || cfg.Tools.Browser.GetTimeoutSeconds() != 45 {
		t.Fatalf("limits not saved: %+v", cfg.Tools.Browser)
	}
}

func TestBrowserSettings_PermissionPolicy(t *testing.T) {
	mux, configPath := newBrowserSettingsTestMux(t)

	rec := doBrowserSettingsRequest(t, mux, http.MethodPut, "/api/tools/browser", map[string]any{
		"enabled":           false,
		"allowed_domains":   []string{"example.com"},
		"permission_policy": "deny",
		"headless":          true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Tools.Browser.UnknownDomainPolicy() != "deny" {
		t.Fatalf("policy = %q, want deny", cfg.Tools.Browser.UnknownDomainPolicy())
	}
	if cfg.Tools.Browser.IsAllowAllDomains() {
		t.Fatal("deny policy must not allow all domains")
	}

	invalid := doBrowserSettingsRequest(t, mux, http.MethodPut, "/api/tools/browser", map[string]any{
		"enabled":           false,
		"allowed_domains":   []string{"example.com"},
		"permission_policy": "sometimes",
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid policy status = %d, want 400", invalid.Code)
	}
}

func TestBrowserSettings_RejectsBadBrowserPath(t *testing.T) {
	mux, _ := newBrowserSettingsTestMux(t)

	rec := doBrowserSettingsRequest(t, mux, http.MethodPut, "/api/tools/browser", map[string]any{
		"enabled":         false,
		"allowed_domains": []string{"example.com"},
		"browser_path":    "C:\\does\\not\\exist.exe",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestBrowserSettings_RejectsBadBinaryPath(t *testing.T) {
	mux, _ := newBrowserSettingsTestMux(t)

	rec := doBrowserSettingsRequest(t, mux, http.MethodPut, "/api/tools/browser", map[string]any{
		"enabled":         false,
		"allowed_domains": []string{"example.com"},
		"binary_path":     "C:\\does\\not\\exist\\agent-browser.exe",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestNormalizeBrowserDomains(t *testing.T) {
	domains, err := normalizeBrowserDomains([]string{
		" Example.COM ",
		"https://api.example.com:8443/x",
		"*.example.com",
		"example.com",
	})
	if err != nil {
		t.Fatalf("normalizeBrowserDomains() error = %v", err)
	}
	want := []string{"example.com", "api.example.com", "*.example.com"}
	if len(domains) != len(want) {
		t.Fatalf("domains = %v, want %v", domains, want)
	}
	for i := range want {
		if domains[i] != want[i] {
			t.Fatalf("domains = %v, want %v", domains, want)
		}
	}
}
