package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestHandleAddModel_OpenAICompatibleProvider(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	resp, cfg, code := postAddModel(t, configPath, `{
		"model_name":"custom-endpoint",
		"provider":"openai-compatible",
		"model":"my-model",
		"api_base":"https://llm.internal/v1",
		"api_key":"sk-custom"
	}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if resp["status"] != "ok" {
		t.Fatalf("status field = %v, want ok", resp["status"])
	}

	added := findModelByName(t, cfg, "custom-endpoint")
	if added == nil {
		t.Fatal("custom-endpoint model was not added")
	}
	if added.Provider != "openai-compatible" {
		t.Fatalf("provider = %q, want openai-compatible", added.Provider)
	}
	if added.APIBase != "https://llm.internal/v1" {
		t.Fatalf("api_base = %q, want the custom endpoint", added.APIBase)
	}
	if added.APIKey() != "sk-custom" {
		t.Fatalf("api_key = %q, want sk-custom", added.APIKey())
	}
}

func TestHandleAddModel_OpenAICompatibleRequiresAPIBase(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models", `{
		"model_name":"custom-endpoint",
		"provider":"openai-compatible",
		"model":"my-model",
		"api_key":"sk-custom"
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "api_base is required") {
		t.Fatalf("body = %q, want api_base error", rec.Body.String())
	}
}

func TestHandleAddModel_CustomUnknownProviderWithBase(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	resp, cfg, code := postAddModel(t, configPath, `{
		"model_name":"my-llm-model",
		"provider":"my-llm",
		"model":"my-model",
		"api_base":"https://llm.internal/v1",
		"api_key":"sk-custom"
	}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%v)", code, resp)
	}

	added := findModelByName(t, cfg, "my-llm-model")
	if added == nil {
		t.Fatal("my-llm-model was not added")
	}
	if added.Provider != "my-llm" {
		t.Fatalf("provider = %q, want my-llm", added.Provider)
	}
}

func TestHandleAddModel_CustomUnknownProviderRequiresBase(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models", `{
		"model_name":"my-llm-model",
		"provider":"my-llm",
		"model":"my-model",
		"api_key":"sk-custom"
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `provider "my-llm" is not supported`) {
		t.Fatalf("body = %q, want unsupported provider error", rec.Body.String())
	}
}

func TestHandleUpdateModel_CustomUnknownProviderKeepsBaseRule(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		{
			ModelName: "my-llm-model",
			Provider:  "my-llm",
			Model:     "my-model",
			APIBase:   "https://llm.internal/v1",
			APIKeys:   config.SimpleSecureStrings("sk-custom"),
		},
	})

	// Dropping the API base from an unknown provider is rejected.
	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/my-llm-model", `{
		"model_name":"my-llm-model",
		"provider":"my-llm",
		"model":"my-model",
		"api_base":""
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}

	// Keeping a base works and preserves the custom provider.
	rec = serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/my-llm-model", `{
		"model_name":"my-llm-model",
		"provider":"my-llm",
		"model":"my-model-2",
		"api_base":"https://llm2.internal/v1"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	updated := findModelByName(t, cfg, "my-llm-model")
	if updated == nil || updated.Model != "my-model-2" || updated.APIBase != "https://llm2.internal/v1" {
		t.Fatalf("updated model = %#v, want new model id and base", updated)
	}
}

func TestHandleUpdateDefaultChain_CustomProviderAllowedAsDefault(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		{
			ModelName: "custom-endpoint",
			Provider:  "openai-compatible",
			Model:     "my-model",
			APIBase:   "https://llm.internal/v1",
			APIKeys:   config.SimpleSecureStrings("sk-custom"),
		},
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/default-chain", `{
		"default_model":"custom-endpoint",
		"fallback_chain":[]
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.Agents.Defaults.ModelName != "custom-endpoint" {
		t.Fatalf("default model = %q, want custom-endpoint", cfg.Agents.Defaults.ModelName)
	}
}
