package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func gatewayStatusBody(t *testing.T, h *Handler) (int, map[string]any) {
	t.Helper()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/gateway/status", nil)
	mux.ServeHTTP(rec, req)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal gateway status response %q: %v", rec.Body.String(), err)
	}
	return rec.Code, body
}

func TestGatewayStatusSurvivesBrokenConfig(t *testing.T) {
	cases := []struct {
		name            string
		content         string
		wantConfigError bool
		wantContains    string
	}{
		{
			name:            "utf8_bom",
			content:         "\xef\xbb\xbf{\"version\":3}",
			wantConfigError: true,
			wantContains:    "BOM",
		},
		{
			name:            "invalid_json",
			content:         "{\"version\": 3,",
			wantConfigError: true,
			wantContains:    "invalid",
		},
		{
			name:            "empty_file",
			content:         "",
			wantConfigError: true,
			wantContains:    "invalid",
		},
		{
			name:            "valid_config_without_bom",
			content:         `{"version":3}`,
			wantConfigError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetGatewayTestState(t)

			configPath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(configPath, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			h := NewHandler(configPath)
			code, body := gatewayStatusBody(t, h)

			if code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body=%v)", code, http.StatusOK, body)
			}
			if _, ok := body["gateway_status"].(string); !ok {
				t.Fatalf("gateway_status missing or not string: %#v", body["gateway_status"])
			}
			if allowed, ok := body["gateway_start_allowed"].(bool); !ok || allowed {
				t.Fatalf("gateway_start_allowed = %#v, want false", body["gateway_start_allowed"])
			}
			if restart, ok := body["gateway_restart_required"].(bool); !ok || restart {
				t.Fatalf("gateway_restart_required = %#v, want false", body["gateway_restart_required"])
			}

			if !tc.wantConfigError {
				if _, ok := body["config_error"]; ok {
					t.Fatalf("config_error = %#v, want absent", body["config_error"])
				}
				return
			}

			configErr, ok := body["config_error"].(string)
			if !ok || strings.TrimSpace(configErr) == "" {
				t.Fatalf("config_error missing or empty: %#v", body["config_error"])
			}
			if !strings.Contains(configErr, tc.wantContains) {
				t.Fatalf("config_error = %q, want contains %q", configErr, tc.wantContains)
			}
			if _, ok := body["config_default_model"]; ok {
				t.Fatalf("config_default_model should be absent with broken config")
			}
			reason, ok := body["gateway_start_reason"].(string)
			if !ok || reason != configErr {
				t.Fatalf("gateway_start_reason = %#v, want %q", body["gateway_start_reason"], configErr)
			}
		})
	}
}

func TestGatewayStatusSurvivesMissingConfig(t *testing.T) {
	resetGatewayTestState(t)

	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)
	code, body := gatewayStatusBody(t, h)

	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if _, ok := body["config_error"]; ok {
		t.Fatalf("config_error = %#v, want absent for missing file (defaults apply)", body["config_error"])
	}
	if _, ok := body["gateway_status"].(string); !ok {
		t.Fatalf("gateway_status missing or not string: %#v", body["gateway_status"])
	}
}

func TestGatewayStatusKeepsServingWhenConfigBecomesInvalid(t *testing.T) {
	resetGatewayTestState(t)

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := config.SaveConfig(configPath, config.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if code, body := gatewayStatusBody(t, h); code != http.StatusOK || body["config_error"] != nil {
		t.Fatalf("healthy config: status = %d, config_error = %#v", code, body["config_error"])
	}

	// Corrupt the file at runtime: the status endpoint must degrade instead of
	// panicking on a nil config.
	if err := os.WriteFile(configPath, []byte("\xef\xbb\xbf{"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	code, body := gatewayStatusBody(t, h)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	if _, ok := body["config_error"].(string); !ok {
		t.Fatalf("config_error missing after corruption: %#v", body)
	}
}

func TestGetConfigReportsBrokenConfigWithoutPanic(t *testing.T) {
	resetGatewayTestState(t)

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte("\xef\xbb\xbf{\"version\":3}"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "Failed to load config") {
		t.Fatalf("body = %q, want config load error", rec.Body.String())
	}
}
