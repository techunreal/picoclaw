package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestHandleSetModelEnabled_PersistsState(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/enabled/alpha", `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	model := findModelByName(t, cfg, "alpha")
	if model == nil || !model.Disabled {
		t.Fatalf("model = %#v, want disabled", model)
	}

	models := listModelsResponse(t, configPath)
	entry := findModelResponse(t, models, "alpha")
	if entry == nil || entry.Enabled {
		t.Fatalf("response model = %#v, want enabled=false", entry)
	}

	// Re-enabling flips the flag back.
	rec = serveModelsRequest(t, configPath, http.MethodPut, "/api/models/enabled/alpha", `{"enabled":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if model := findModelByName(t, cfg, "alpha"); model == nil || model.Disabled {
		t.Fatalf("model = %#v, want enabled", model)
	}
}

func TestHandleSetModelEnabled_ClearsDefaultAndFallback(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		smokeModel("primary", "openai", "gpt-4o-mini"),
		smokeModel("backup", "openai", "gpt-4o-mini"),
	}, "primary", []string{"backup"})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/enabled/primary", `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	assertDefaultChain(t, updated, "", nil)
}

func TestHandleUpdateDefaultChain_RejectsDisabledDefaultModel(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	disabled := smokeModel("alpha", "openai", "gpt-4o-mini")
	disabled.Disabled = true
	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		disabled,
		smokeModel("beta", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/default-chain", `{
		"default_model":"alpha",
		"fallback_chain":[]
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "disabled") {
		t.Fatalf("body = %q, want disabled model error", rec.Body.String())
	}
}

func TestHandleUpdateDefaultChain_RejectsDisabledFallbackModel(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	disabled := smokeModel("backup", "openai", "gpt-4o-mini")
	disabled.Disabled = true
	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("primary", "openai", "gpt-4o-mini"),
		disabled,
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/default-chain", `{
		"default_model":"primary",
		"fallback_chain":["backup"]
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "disabled") {
		t.Fatalf("body = %q, want disabled model error", rec.Body.String())
	}
}

func TestHandleUpdateModel_PreservesDisabledWhenOmitted(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	disabled := smokeModel("alpha", "openai", "gpt-4o-mini")
	disabled.Disabled = true
	writeSmokeModelList(t, configPath, []*config.ModelConfig{disabled})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/alpha", `{
		"model_name":"alpha",
		"model":"gpt-4.1-mini"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	model := findModelByName(t, cfg, "alpha")
	if model == nil || !model.Disabled {
		t.Fatalf("model = %#v, want disabled preserved", model)
	}
}

func TestHandleUpdateModel_EnabledFlagUpdatesState(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/alpha", `{
		"model_name":"alpha",
		"model":"gpt-4o-mini",
		"enabled":false
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if model := findModelByName(t, cfg, "alpha"); model == nil || !model.Disabled {
		t.Fatalf("model = %#v, want disabled", model)
	}
}

func TestHandleSetModelEnabled_RejectsVirtualModel(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		multiKeyModel("multi", "sk-1", "sk-2"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/enabled/multi__key_1", `{"enabled":false}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func listModelsResponse(t *testing.T, configPath string) []modelResponse {
	t.Helper()

	rec := serveModelsRequest(t, configPath, http.MethodGet, "/api/models", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Models []modelResponse `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.Models
}

func findModelResponse(t *testing.T, models []modelResponse, name string) *modelResponse {
	t.Helper()

	for i := range models {
		if models[i].ModelName == name {
			return &models[i]
		}
	}
	return nil
}
