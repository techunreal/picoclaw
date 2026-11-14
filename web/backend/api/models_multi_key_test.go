package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func multiKeyModel(modelName string, keys ...string) *config.ModelConfig {
	return &config.ModelConfig{
		ModelName: modelName,
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		APIBase:   "https://api.openai.com/v1",
		APIKeys:   config.SimpleSecureStrings(keys...),
	}
}

func TestHandleAddModel_MultipleAPIKeys(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models", `{
		"model_name":"multi-key",
		"provider":"openai",
		"model":"gpt-4o-mini",
		"api_keys":["sk-first","sk-second"]
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	added := findModelByName(t, cfg, "multi-key")
	if added == nil {
		t.Fatal("multi-key model was not added")
	}
	keys := added.APIKeys.Values()
	if len(keys) != 2 || keys[0] != "sk-first" || keys[1] != "sk-second" {
		t.Fatalf("api_keys = %v, want [sk-first sk-second]", keys)
	}
}

func TestHandleListModels_MasksMultipleAPIKeys(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		multiKeyModel("multi-key", "sk-first-secret", "sk-second-secret"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodGet, "/api/models", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "sk-first-secret") || strings.Contains(body, "sk-second-secret") {
		t.Fatalf("response leaked a plaintext API key: %s", body)
	}

	var resp struct {
		Models []modelResponse `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var primary *modelResponse
	for i := range resp.Models {
		if resp.Models[i].ModelName == "multi-key" {
			primary = &resp.Models[i]
			break
		}
	}
	if primary == nil {
		t.Fatal("multi-key model not found in response")
	}
	if primary.APIKeyCount != 2 || len(primary.APIKeys) != 2 {
		t.Fatalf("api_key_count = %d, api_keys = %v, want 2", primary.APIKeyCount, primary.APIKeys)
	}
	if primary.APIKeys[0] != maskAPIKey("sk-first-secret") || primary.APIKeys[1] != maskAPIKey("sk-second-secret") {
		t.Fatalf("masked keys = %v, want %q and %q", primary.APIKeys, maskAPIKey("sk-first-secret"), maskAPIKey("sk-second-secret"))
	}
}

func TestHandleUpdateModel_KeepsAllKeysWhenOmitted(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		multiKeyModel("multi-key", "sk-first", "sk-second"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/multi-key", `{
		"model_name":"multi-key",
		"model":"gpt-4.1-mini"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	updated := findModelByName(t, cfg, "multi-key")
	if updated == nil {
		t.Fatal("multi-key model missing after update")
	}
	keys := updated.APIKeys.Values()
	if len(keys) != 2 || keys[0] != "sk-first" || keys[1] != "sk-second" {
		t.Fatalf("api_keys = %v, want both keys preserved", keys)
	}
}

func TestHandleUpdateModel_KeepAndAppendAPIKeys(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		multiKeyModel("multi-key", "sk-1", "sk-2", "sk-3"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/multi-key", `{
		"model_name":"multi-key",
		"model":"gpt-4o-mini",
		"api_keys_keep":[0,2],
		"api_keys":["sk-4"]
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	updated := findModelByName(t, cfg, "multi-key")
	if updated == nil {
		t.Fatal("multi-key model missing after update")
	}
	keys := updated.APIKeys.Values()
	want := []string{"sk-1", "sk-3", "sk-4"}
	if len(keys) != len(want) {
		t.Fatalf("api_keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("api_keys = %v, want %v", keys, want)
		}
	}
}

func TestHandleUpdateModel_RemoveAPIKeys(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		multiKeyModel("multi-key", "sk-1", "sk-2"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/multi-key", `{
		"model_name":"multi-key",
		"model":"gpt-4o-mini",
		"api_keys_keep":[1]
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	updated := findModelByName(t, cfg, "multi-key")
	if updated == nil {
		t.Fatal("multi-key model missing after update")
	}
	keys := updated.APIKeys.Values()
	if len(keys) != 1 || keys[0] != "sk-2" {
		t.Fatalf("api_keys = %v, want [sk-2]", keys)
	}
}

func TestHandleUpdateModel_RejectsInvalidAPIKeyKeepIndex(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		multiKeyModel("multi-key", "sk-1", "sk-2"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/multi-key", `{
		"model_name":"multi-key",
		"model":"gpt-4o-mini",
		"api_keys_keep":[0,5]
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}
