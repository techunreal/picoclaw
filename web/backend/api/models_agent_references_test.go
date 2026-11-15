package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func writeModelAgentConfig(
	t *testing.T,
	configPath string,
	models []*config.ModelConfig,
	mutate func(cfg *config.Config),
) {
	t.Helper()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = models
	if mutate != nil {
		mutate(cfg)
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
}

func decodeActionResponse(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("unmarshal response %q: %v", string(body), err)
	}
	return response
}

func agentIDsFromList(t *testing.T, value any) []string {
	t.Helper()

	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("agents = %#v, want array", value)
	}
	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		id, ok := item.(string)
		if !ok {
			t.Fatalf("agent id = %#v, want string", item)
		}
		ids = append(ids, id)
	}
	return ids
}

func agentByID(t *testing.T, cfg *config.Config, id string) *config.AgentConfig {
	t.Helper()

	for i := range cfg.Agents.List {
		if cfg.Agents.List[i].ID == id {
			return &cfg.Agents.List[i]
		}
	}
	t.Fatalf("agent %q not found", id)
	return nil
}

func TestModelRenameUpdatesAgentReferences(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeModelAgentConfig(t, configPath, []*config.ModelConfig{
		smokeModel("old-model", "openai", "gpt-4o-mini"),
		smokeModel("keep-model", "openai", "gpt-4o-mini"),
		smokeModel("other-model", "openai", "gpt-4o-mini"),
	}, func(cfg *config.Config) {
		cfg.Agents.Defaults.ModelName = "old-model"
		cfg.Agents.Defaults.ModelFallbacks = []string{"old-model"}
		cfg.Agents.List = []config.AgentConfig{
			{
				ID: "alpha",
				Model: &config.AgentModelConfig{
					Primary:   "old-model",
					Fallbacks: []string{"other-model", "old-model"},
				},
				Subagents: &config.SubagentsConfig{
					Model: &config.AgentModelConfig{Primary: "old-model"},
				},
			},
			{
				ID:    "beta",
				Model: &config.AgentModelConfig{Primary: "keep-model", Fallbacks: []string{"old-model"}},
			},
			{ID: "gamma"},
		}
	})

	rec := serveModelsRequest(t, configPath, http.MethodPut, "/api/models/by-name/old-model", `{
		"model_name":"new-model",
		"model":"gpt-4o-mini",
		"api_base":"https://api.openai.com/v1"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename status = %d body=%s", rec.Code, rec.Body.String())
	}

	response := decodeActionResponse(t, rec.Body.Bytes())
	rename, ok := response["reference_rename"].(map[string]any)
	if !ok {
		t.Fatalf("reference_rename missing: %#v", response)
	}
	affected := agentIDsFromList(t, rename["agents"])
	if len(affected) != 2 || affected[0] != "alpha" || affected[1] != "beta" {
		t.Fatalf("reference_rename.agents = %#v, want [alpha beta]", affected)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	alpha := agentByID(t, cfg, "alpha")
	if alpha.Model.Primary != "new-model" {
		t.Fatalf("alpha primary = %q, want new-model", alpha.Model.Primary)
	}
	if got := alpha.Model.Fallbacks; len(got) != 2 || got[0] != "other-model" || got[1] != "new-model" {
		t.Fatalf("alpha fallbacks = %#v, want [other-model new-model]", got)
	}
	if alpha.Subagents == nil || alpha.Subagents.Model == nil || alpha.Subagents.Model.Primary != "new-model" {
		t.Fatalf("alpha subagent model = %#v, want new-model", alpha.Subagents)
	}
	beta := agentByID(t, cfg, "beta")
	if beta.Model.Primary != "keep-model" {
		t.Fatalf("beta primary = %q, want keep-model", beta.Model.Primary)
	}
	if got := beta.Model.Fallbacks; len(got) != 1 || got[0] != "new-model" {
		t.Fatalf("beta fallbacks = %#v, want [new-model]", got)
	}
	if cfg.Agents.Defaults.ModelName != "new-model" {
		t.Fatalf("default model = %q, want new-model", cfg.Agents.Defaults.ModelName)
	}
	if got := cfg.Agents.Defaults.ModelFallbacks; len(got) != 0 {
		t.Fatalf("default fallbacks = %#v, want empty (default is removed from its own fallback chain)", got)
	}
}

func TestModelDeleteClearsAgentReferences(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeModelAgentConfig(t, configPath, []*config.ModelConfig{
		smokeModel("old-model", "openai", "gpt-4o-mini"),
		smokeModel("other-model", "openai", "gpt-4o-mini"),
	}, func(cfg *config.Config) {
		cfg.Agents.Defaults.ModelName = "old-model"
		cfg.Agents.List = []config.AgentConfig{
			{
				ID: "alpha",
				Model: &config.AgentModelConfig{
					Primary:   "old-model",
					Fallbacks: []string{"other-model", "old-model"},
				},
				Subagents: &config.SubagentsConfig{
					Model: &config.AgentModelConfig{Primary: "old-model"},
				},
			},
			{
				ID:    "beta",
				Model: &config.AgentModelConfig{Primary: "other-model", Fallbacks: []string{"old-model"}},
			},
		}
	})

	rec := serveModelsRequest(t, configPath, http.MethodDelete, "/api/models/by-name/old-model", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}

	response := decodeActionResponse(t, rec.Body.Bytes())
	cleanupInfo, ok := response["reference_cleanup"].(map[string]any)
	if !ok {
		t.Fatalf("reference_cleanup missing: %#v", response)
	}
	if cleanupInfo["model"] != "old-model" {
		t.Fatalf("reference_cleanup.model = %#v, want old-model", cleanupInfo["model"])
	}
	affected := agentIDsFromList(t, cleanupInfo["agents"])
	if len(affected) != 2 || affected[0] != "alpha" || affected[1] != "beta" {
		t.Fatalf("reference_cleanup.agents = %#v, want [alpha beta]", affected)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	alpha := agentByID(t, cfg, "alpha")
	if alpha.Model == nil || alpha.Model.Primary != "" {
		t.Fatalf("alpha model = %#v, want primary cleared", alpha.Model)
	}
	if got := alpha.Model.Fallbacks; len(got) != 1 || got[0] != "other-model" {
		t.Fatalf("alpha fallbacks = %#v, want [other-model]", got)
	}
	if alpha.Subagents == nil {
		t.Fatalf("alpha subagents config should be preserved")
	}
	if alpha.Subagents.Model != nil {
		t.Fatalf("alpha subagent model = %#v, want nil", alpha.Subagents.Model)
	}
	beta := agentByID(t, cfg, "beta")
	if beta.Model == nil || beta.Model.Primary != "other-model" {
		t.Fatalf("beta model = %#v, want other-model", beta.Model)
	}
	if len(beta.Model.Fallbacks) != 0 {
		t.Fatalf("beta fallbacks = %#v, want empty", beta.Model.Fallbacks)
	}
	if cfg.Agents.Defaults.ModelName != "" {
		t.Fatalf("default model = %q, want empty after delete", cfg.Agents.Defaults.ModelName)
	}
}

func TestModelDeleteKeepsAgentReferencesWhenDuplicateNameRemains(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	writeModelAgentConfig(t, configPath, []*config.ModelConfig{
		smokeModel("dup-model", "openai", "gpt-4o-mini"),
		smokeModel("dup-model", "openai", "gpt-4o-mini"),
	}, func(cfg *config.Config) {
		cfg.Agents.List = []config.AgentConfig{
			{ID: "alpha", Model: &config.AgentModelConfig{Primary: "dup-model"}},
		}
	})

	// Deleting one of two models with the same name leaves the references valid.
	rec := serveModelsRequest(t, configPath, http.MethodDelete, "/api/models/0", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
	response := decodeActionResponse(t, rec.Body.Bytes())
	if _, ok := response["reference_cleanup"]; ok {
		t.Fatalf("reference_cleanup = %#v, want absent while the name still exists", response["reference_cleanup"])
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	alpha := agentByID(t, cfg, "alpha")
	if alpha.Model == nil || alpha.Model.Primary != "dup-model" {
		t.Fatalf("alpha model = %#v, want dup-model preserved", alpha.Model)
	}
}
