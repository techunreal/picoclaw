package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

// postAddModel posts a model configuration and returns the decoded response and
// the reloaded config on success.
func postAddModel(t *testing.T, configPath, body string) (map[string]any, *config.Config, int) {
	t.Helper()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	var resp map[string]any
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v, body=%s", err, rec.Body.String())
		}
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	return resp, cfg, rec.Code
}

func TestHandleAddModel_ReusesStoredAPIKeyForSameEndpoint(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	// The default test config has an openai model with "sk-default" and no
	// explicit api_base, so it targets the provider default endpoint.
	resp, cfg, code := postAddModel(t, configPath, `{
		"model_name":"catalog-gpt-4o-mini",
		"provider":"openai",
		"model":"gpt-4o-mini"
	}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if reused, _ := resp["api_key_reused"].(bool); !reused {
		t.Fatalf("api_key_reused = %v, want true (body=%v)", resp["api_key_reused"], resp)
	}

	added := cfg.ModelList[len(cfg.ModelList)-1]
	if added.APIKey() != "sk-default" {
		t.Fatalf("api_key = %q, want reused %q", added.APIKey(), "sk-default")
	}
}

func TestHandleAddModel_DoesNotReuseKeyForDifferentEndpoint(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	resp, cfg, code := postAddModel(t, configPath, `{
		"model_name":"other-endpoint-model",
		"provider":"openai",
		"model":"gpt-4o-mini",
		"api_base":"https://example.invalid/v1"
	}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if reused, _ := resp["api_key_reused"].(bool); reused {
		t.Fatalf("api_key_reused = true, want false for a different endpoint")
	}

	added := cfg.ModelList[len(cfg.ModelList)-1]
	if added.APIKey() != "" {
		t.Fatalf("api_key = %q, want empty for a different endpoint", added.APIKey())
	}
}

func TestHandleAddModel_ExplicitAPIKeyWinsOverStoredKey(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	resp, cfg, code := postAddModel(t, configPath, `{
		"model_name":"explicit-key-model",
		"provider":"openai",
		"model":"gpt-4o-mini",
		"api_key":"sk-explicit"
	}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if reused, _ := resp["api_key_reused"].(bool); reused {
		t.Fatalf("api_key_reused = true, want false when an explicit key is sent")
	}

	added := cfg.ModelList[len(cfg.ModelList)-1]
	if added.APIKey() != "sk-explicit" {
		t.Fatalf("api_key = %q, want %q", added.APIKey(), "sk-explicit")
	}
}

func TestListCatalogs_ReportsReusableStoredKey(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	if err := SaveCatalog("openai", "", "sk-default", []CatalogModel{
		{ID: "gpt-4o-mini"},
	}); err != nil {
		t.Fatalf("SaveCatalog error: %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models/catalog", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Entries []CatalogEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(resp.Entries))
	}

	entry := resp.Entries[0]
	if !entry.HasStoredKey {
		t.Fatalf("has_stored_key = false, want true")
	}
	if entry.StoredKeyModel != "custom-default" {
		t.Fatalf("stored_key_model = %q, want %q", entry.StoredKeyModel, "custom-default")
	}
	if entry.StoredKeyMask != maskAPIKey("sk-default") {
		t.Fatalf("stored_key_mask = %q, want %q", entry.StoredKeyMask, maskAPIKey("sk-default"))
	}
}
