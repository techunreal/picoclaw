package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func setupAgentModelTestConfig(t *testing.T) (string, func()) {
	t.Helper()
	configPath, cleanup := setupOAuthTestEnv(t)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		cleanup()
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Agents.Defaults.ModelName = "model-a"
	cfg.ModelList = []*config.ModelConfig{
		{ModelName: "model-a", Provider: "openai", Model: "openai/a", APIBase: "http://127.0.0.1:1/v1"},
		{ModelName: "model-b", Provider: "openai", Model: "openai/b", APIBase: "http://127.0.0.1:2/v1"},
		{ModelName: "model-c", Provider: "openai", Model: "openai/c", APIBase: "http://127.0.0.1:3/v1"},
	}
	cfg.Agents.List = []config.AgentConfig{
		{ID: "inherit", Workspace: t.TempDir()},
		{ID: "override", Workspace: t.TempDir(), Model: &config.AgentModelConfig{Primary: "model-a"}},
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		cleanup()
		t.Fatalf("SaveConfig() error = %v", err)
	}
	return configPath, cleanup
}

func putAgentModel(t *testing.T, mux *http.ServeMux, agentID, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/agents/"+agentID+"/model",
		bytes.NewBufferString(body),
	)
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSetAgentModelPersistsAndClearsOverride(t *testing.T) {
	configPath, cleanup := setupAgentModelTestConfig(t)
	defer cleanup()
	mux := agentsTestMux(t, configPath)

	rec := putAgentModel(t, mux, "inherit", `{"model":"model-b"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp["effective_model"] != "model-b" {
		t.Fatalf("effective_model = %v, want model-b", resp["effective_model"])
	}
	if resp["restart_required"] != false {
		t.Fatalf("restart_required = %v, want false", resp["restart_required"])
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	agentCfg := findAgentConfigForAPITest(cfg, "inherit")
	if agentCfg == nil || agentCfg.Model == nil || agentCfg.Model.Primary != "model-b" {
		t.Fatalf("persisted model = %#v, want model-b override", agentCfg)
	}

	rec = putAgentModel(t, mux, "inherit", `{"use_default":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() after clear error = %v", err)
	}
	agentCfg = findAgentConfigForAPITest(cfg, "inherit")
	if agentCfg == nil || agentCfg.Model != nil {
		t.Fatalf("model after clear = %#v, want nil", agentCfg)
	}
}

func TestSetMainModelUpdatesGlobalDefaultAndInheritance(t *testing.T) {
	configPath, cleanup := setupAgentModelTestConfig(t)
	defer cleanup()
	mux := agentsTestMux(t, configPath)

	rec := putAgentModel(t, mux, "main", `{"model":"model-b"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Agents.Defaults.GetModelName() != "model-b" {
		t.Fatalf("default model = %q, want model-b", cfg.Agents.Defaults.GetModelName())
	}
	if got := findAgentConfigForAPITest(cfg, "inherit"); got == nil || got.Model != nil {
		t.Fatalf("inheriting agent model changed: %#v", got)
	}
	override := findAgentConfigForAPITest(cfg, "override")
	if override == nil || override.Model == nil || override.Model.Primary != "model-a" {
		t.Fatalf("explicit override changed: %#v", override)
	}
}

func TestGlobalModelRequestsSkipOverridesAndFrontmatter(t *testing.T) {
	configPath, cleanup := setupAgentModelTestConfig(t)
	defer cleanup()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	frontWorkspace := t.TempDir()
	writeAgentFrontmatterModel(t, frontWorkspace, "model-c")
	cfg.Agents.List = append(cfg.Agents.List, config.AgentConfig{
		ID: "front", Workspace: frontWorkspace,
	})
	h := NewHandler(configPath)
	requests := h.globalModelRequests(cfg, "model-b")

	if _, ok := requests["main"]; !ok {
		t.Fatal("global requests must include main")
	}
	if _, ok := requests["inherit"]; !ok {
		t.Fatal("global requests must include an inheriting agent")
	}
	if _, ok := requests["override"]; ok {
		t.Fatal("global requests must skip an explicit override")
	}
	if _, ok := requests["front"]; ok {
		t.Fatal("global requests must skip an AGENT.md frontmatter model")
	}
}

func TestEffectiveAgentModelSource(t *testing.T) {
	configPath, cleanup := setupAgentModelTestConfig(t)
	defer cleanup()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	frontWorkspace := t.TempDir()
	writeAgentFrontmatterModel(t, frontWorkspace, "model-c")
	cfg.Agents.List = append(cfg.Agents.List, config.AgentConfig{
		ID: "front", Workspace: frontWorkspace,
	})

	model, source := effectiveAgentModel(cfg, config.AgentConfig{ID: "inherit", Workspace: t.TempDir()})
	if model != "model-a" || source != "global" {
		t.Fatalf("inherit = %q/%q, want model-a/global", model, source)
	}
	model, source = effectiveAgentModel(cfg, config.AgentConfig{
		ID: "override", Model: &config.AgentModelConfig{Primary: "model-b"},
	})
	if model != "model-b" || source != "override" {
		t.Fatalf("override = %q/%q, want model-b/override", model, source)
	}
	model, source = effectiveAgentModel(cfg, config.AgentConfig{ID: "front", Workspace: frontWorkspace})
	if model != "model-c" || source != "frontmatter" {
		t.Fatalf("frontmatter = %q/%q, want model-c/frontmatter", model, source)
	}
}

func findAgentConfigForAPITest(cfg *config.Config, id string) *config.AgentConfig {
	for i := range cfg.Agents.List {
		if cfg.Agents.List[i].ID == id {
			return &cfg.Agents.List[i]
		}
	}
	return nil
}

func writeAgentFrontmatterModel(t *testing.T, workspace, model string) {
	t.Helper()
	content := "---\nmodel: " + model + "\n---\n\n# Agent\n"
	if err := os.WriteFile(filepath.Join(workspace, "AGENT.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENT.md) error = %v", err)
	}
}
