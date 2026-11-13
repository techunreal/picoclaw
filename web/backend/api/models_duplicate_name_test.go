package api

import (
	"net/http"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestHandleAddModel_RejectsDuplicateModelName(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
		smokeModel("beta", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models", `{
		"model_name":"alpha",
		"provider":"openai",
		"model":"gpt-4.1-mini",
		"api_key":"sk-new"
	}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if len(cfg.ModelList) != 2 {
		t.Fatalf("len(model_list) = %d, want unchanged 2", len(cfg.ModelList))
	}
}

func TestHandleAddModel_RejectsDuplicateNameIgnoringWhitespace(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPost, "/api/models", `{
		"model_name":"  alpha  ",
		"provider":"openai",
		"model":"gpt-4.1-mini",
		"api_key":"sk-new"
	}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if len(cfg.ModelList) != 1 {
		t.Fatalf("len(model_list) = %d, want unchanged 1", len(cfg.ModelList))
	}
}

func TestHandleUpdateModel_RejectsRenameToExistingName(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
		smokeModel("beta", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/0", `{
		"model_name":"beta",
		"model":"gpt-4o-mini"
	}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if findModelByName(t, cfg, "alpha") == nil {
		t.Fatalf("alpha was renamed despite the conflict")
	}
	if beta := findModelByName(t, cfg, "beta"); beta == nil || beta.Model != "gpt-4o-mini" {
		t.Fatalf("beta = %#v, want untouched gpt-4o-mini", beta)
	}
}

func TestHandleUpdateModelByName_RejectsRenameToExistingName(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("alpha", "openai", "gpt-4o-mini"),
		smokeModel("beta", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/alpha", `{
		"model_name":"beta",
		"model":"gpt-4o-mini"
	}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if findModelByName(t, cfg, "alpha") == nil {
		t.Fatalf("alpha was renamed despite the conflict")
	}
}

func TestHandleUpdateModel_AllowsKeepingOwnName(t *testing.T) {
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

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if alpha := findModelByName(t, cfg, "alpha"); alpha == nil || alpha.Model != "gpt-4.1-mini" {
		t.Fatalf("alpha = %#v, want updated gpt-4.1-mini", alpha)
	}
}

func TestHandleUpdateModel_ToleratesExistingDuplicateNames(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	// Older configs may already contain duplicates; editing one without
	// renaming it must keep working so users can repair the list.
	writeSmokeModelList(t, configPath, []*config.ModelConfig{
		smokeModel("dup", "openai", "gpt-4o-mini"),
		smokeModel("dup", "openai", "gpt-4o-mini"),
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/0", `{
		"model_name":"dup",
		"model":"gpt-4.1-mini"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.ModelList[0].Model != "gpt-4.1-mini" {
		t.Fatalf("model[0].model = %q, want gpt-4.1-mini", cfg.ModelList[0].Model)
	}
}
