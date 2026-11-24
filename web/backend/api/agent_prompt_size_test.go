package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	agentpkg "github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/config"
)

type promptSizeResponse struct {
	Agent        string                    `json:"agent"`
	Workspace    string                    `json:"workspace"`
	Persona      bool                      `json:"persona"`
	Skills       []string                  `json:"skills"`
	SkillsSource string                    `json:"skills_source"`
	Report       agentpkg.PromptSizeReport `json:"report"`
}

func TestAgentPromptSizePersonaAgent(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Shared self-management skill in the default workspace.
	sharedSkill := filepath.Join(cfg.Agents.Defaults.Workspace, "skills", "picoclaw-agent", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(sharedSkill), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sharedSkill,
		[]byte("---\nname: picoclaw-agent\ndescription: PicoClaw self-management\n---\n\nBODY\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENT.md"),
		[]byte("---\nname: Hero\nskills: [picoclaw-agent]\n---\n\nYou are Hero.\n"), 0o644); err != nil {
		t.Fatalf("write AGENT.md: %v", err)
	}
	cfg.Agents.List = []config.AgentConfig{{
		ID:        "hero",
		Name:      "Hero",
		Workspace: workspace,
		Skills:    []string{"picoclaw-agent"},
	}}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	mux := agentsTestMux(t, configPath)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents/hero/prompt-size", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp promptSizeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !resp.Persona || resp.Report.IdentityMode != "agent-files" {
		t.Fatalf("persona = %v, identity_mode = %q", resp.Persona, resp.Report.IdentityMode)
	}
	if resp.SkillsSource != "agent.md" {
		t.Fatalf("skills_source = %q, want agent.md", resp.SkillsSource)
	}
	if resp.Report.TotalChars == 0 {
		t.Fatalf("report is empty: %+v", resp.Report)
	}
	// The self-management skill body is catalog-only for a persona agent.
	if len(resp.Report.ActiveSkills) != 0 {
		t.Fatalf("active skills = %v, want none", resp.Report.ActiveSkills)
	}
	found := false
	for _, name := range resp.Report.CatalogSkills {
		if name == agentpkg.AgentSelfSkillName {
			found = true
		}
	}
	if !found {
		t.Fatalf("catalog skills = %v, want %s", resp.Report.CatalogSkills, agentpkg.AgentSelfSkillName)
	}

	// The default agent keeps the generic identity mode.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/agents/main/prompt-size", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("main status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var mainResp promptSizeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &mainResp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if mainResp.Persona || mainResp.Report.IdentityMode != "default" {
		t.Fatalf("main persona = %v, identity_mode = %q", mainResp.Persona, mainResp.Report.IdentityMode)
	}
}

func TestAgentPromptSizeUnknownAgent(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents/nope/prompt-size", nil)
	agentsTestMux(t, configPath).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}
