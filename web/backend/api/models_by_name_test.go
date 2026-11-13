package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func writeSmokeModelList(t *testing.T, configPath string, models []*config.ModelConfig) {
	t.Helper()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	cfg.ModelList = models
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}
}

func smokeModel(modelName, provider, modelID string) *config.ModelConfig {
	return &config.ModelConfig{
		ModelName: modelName,
		Provider:  provider,
		Model:     modelID,
		APIBase:   "https://api.openai.com/v1",
		APIKeys:   config.SimpleSecureStrings("sk-" + modelName),
	}
}

func serveModelsRequest(t *testing.T, configPath, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	}
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

func findModelByName(t *testing.T, cfg *config.Config, name string) *config.ModelConfig {
	t.Helper()

	for _, model := range cfg.ModelList {
		if model != nil && model.ModelName == name {
			return model
		}
	}
	return nil
}

func TestHandleUpdateModelByName_SurvivesIndexShift(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	// The client loaded [alpha, beta] and wants to update beta (index 1).
	// Another tab then prepends gamma, so index 1 now points at alpha.
	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("gamma", "openai", "gpt-4o-mini"),
		smokeModel("alpha", "openai", "gpt-4o-mini"),
		smokeModel("beta", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/beta", `{
		"model_name":"beta",
		"model":"gpt-4.1-mini"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if beta := findModelByName(t, cfg, "beta"); beta == nil || beta.Model != "gpt-4.1-mini" {
		t.Fatalf("beta model = %#v, want updated gpt-4.1-mini", beta)
	}
	if alpha := findModelByName(t, cfg, "alpha"); alpha == nil || alpha.Model != "gpt-4o-mini" {
		t.Fatalf("alpha model = %#v, want untouched gpt-4o-mini", alpha)
	}
	if gamma := findModelByName(t, cfg, "gamma"); gamma == nil || gamma.Model != "gpt-4o-mini" {
		t.Fatalf("gamma model = %#v, want untouched gpt-4o-mini", gamma)
	}
}

func TestHandleDeleteModelByName_SurvivesIndexShift(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	// The client loaded [alpha, beta] and deletes alpha (index 0). Another tab
	// prepended gamma, so index 0 now points at gamma.
	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("gamma", "openai", "gpt-4o-mini"),
		smokeModel("alpha", "openai", "gpt-4o-mini"),
		smokeModel("beta", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodDelete, "/api/models/by-name/alpha", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if len(cfg.ModelList) != 2 {
		t.Fatalf("len(model_list) = %d, want 2", len(cfg.ModelList))
	}
	if findModelByName(t, cfg, "alpha") != nil {
		t.Fatalf("alpha still present after delete")
	}
	if findModelByName(t, cfg, "gamma") == nil || findModelByName(t, cfg, "beta") == nil {
		t.Fatalf("delete removed an unrelated model: %#v", cfg.ModelList)
	}
}

func TestHandleUpdateModelByName_NotFound(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/missing", `{
		"model_name":"missing",
		"model":"gpt-4o-mini"
	}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteModelByName_NotFound(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodDelete, "/api/models/by-name/missing", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateModelByName_AmbiguousName(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("dup", "openai", "gpt-4o-mini"),
		smokeModel("dup", "openai", "gpt-4.1-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/dup", `{
		"model_name":"dup",
		"model":"gpt-4o"
	}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateModelByName_AllowsSlashInName(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("meta/llama-3", "openai", "gpt-4o-mini"),
	})

	path := "/api/models/by-name/" + url.PathEscape("meta/llama-3")
	rec := serveModelsRequest(t, configPath, http.MethodPut, path, `{
		"model_name":"meta/llama-3",
		"model":"gpt-4.1-mini"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	model := findModelByName(t, cfg, "meta/llama-3")
	if model == nil || model.Model != "gpt-4.1-mini" {
		t.Fatalf("model = %#v, want updated gpt-4.1-mini", model)
	}
}

func TestHandleUpdateModelByName_PreservesAPIKeyWhenOmitted(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
	})

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
	if model == nil || model.APIKey() != "sk-alpha" {
		t.Fatalf("api_key = %q, want preserved sk-alpha", model.APIKey())
	}
}

func TestHandleUpdateModel_LegacyIndexStillWorks(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
		smokeModel("beta", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/0", `{
		"model_name":"alpha",
		"model":"gpt-4.1-mini"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("status field = %v, want ok", resp["status"])
	}
}
