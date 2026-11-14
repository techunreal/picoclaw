package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func saveSubagentSwitchConfig(t *testing.T, configPath string, mutate func(*config.Config)) {
	t.Helper()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	mutate(cfg)
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
}

func listToolsByName(t *testing.T, mux *http.ServeMux) map[string]toolSupportItem {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/tools status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp toolSupportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	got := make(map[string]toolSupportItem, len(resp.Tools))
	for _, tool := range resp.Tools {
		got[tool.Name] = tool
	}
	return got
}

func putToolState(t *testing.T, mux *http.ServeMux, name string, enabled bool) {
	t.Helper()
	body, err := json.Marshal(map[string]bool{"enabled": enabled})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/tools/"+name+"/state", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/tools/%s/state status = %d, body=%s", name, rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateToolState_SubagentMasterSwitch(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	saveSubagentSwitchConfig(t, configPath, func(cfg *config.Config) {
		cfg.Tools.Subagent.Enabled = false
		cfg.Tools.Spawn.Enabled = true
		cfg.Tools.SpawnStatus.Enabled = true
	})

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	tools := listToolsByName(t, mux)
	if tools["subagent"].ConfigKey != "subagent" {
		t.Fatalf("subagent config_key = %q, want subagent", tools["subagent"].ConfigKey)
	}
	if tools["subagent"].Status != "disabled" {
		t.Fatalf("subagent status = %q, want disabled", tools["subagent"].Status)
	}
	if tools["spawn"].Status != "blocked" || tools["spawn"].ReasonCode != "requires_subagent" {
		t.Fatalf("spawn = %#v, want blocked/requires_subagent", tools["spawn"])
	}

	putToolState(t, mux, "subagent", true)

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig(updated) error = %v", err)
	}
	if !updated.Tools.Subagent.Enabled {
		t.Fatal("subagent master switch should be enabled")
	}
	tools = listToolsByName(t, mux)
	if tools["spawn"].Status != "enabled" {
		t.Fatalf("spawn status = %q, want enabled after enabling subagent", tools["spawn"].Status)
	}
	if tools["spawn_status"].Status != "enabled" {
		t.Fatalf("spawn_status status = %q, want enabled after enabling subagent", tools["spawn_status"].Status)
	}

	putToolState(t, mux, "subagent", false)

	updated, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig(disabled) error = %v", err)
	}
	if updated.Tools.Subagent.Enabled {
		t.Fatal("subagent master switch should be disabled")
	}
	tools = listToolsByName(t, mux)
	if tools["spawn"].Status != "blocked" || tools["spawn_status"].Status != "blocked" {
		t.Fatalf("spawn tools should be blocked when subagent is off: %#v %#v", tools["spawn"], tools["spawn_status"])
	}
}

func TestConfigSignatureChangesWithSubagentSettings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = t.TempDir()
	cfg.Tools.Subagent.Enabled = true
	before := computeConfigSignature(cfg)

	cfg.Tools.Subagent.Enabled = false
	disabled := computeConfigSignature(cfg)
	if before == disabled {
		t.Fatal("config signature should change when tools.subagent changes")
	}

	// Per-agent subagents.enabled is part of the signature as well.
	disabledAgent := false
	cfg.Tools.Subagent.Enabled = true
	cfg.Agents.List = []config.AgentConfig{{
		ID:        "alpha",
		Subagents: &config.SubagentsConfig{Enabled: &disabledAgent},
	}}
	withDisabledAgent := computeConfigSignature(cfg)
	if before == withDisabledAgent {
		t.Fatal("config signature should change when a per-agent subagents.enabled changes")
	}
}
