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

func setupScopedAgentFiles(t *testing.T) (configPath, alphaWS, betaWS string) {
	t.Helper()
	configPath, cleanup := setupOAuthTestEnv(t)
	t.Cleanup(cleanup)

	alphaWS = t.TempDir()
	betaWS = t.TempDir()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Agents.List = []config.AgentConfig{
		{ID: "alpha", Default: true, Workspace: alphaWS},
		{ID: "beta", Workspace: betaWS},
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	for dir, content := range map[string]string{
		alphaWS: "# Alpha personality\n",
		betaWS:  "# Beta personality\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
	return configPath, alphaWS, betaWS
}

func getScopedFile(t *testing.T, mux *http.ServeMux, path string) agentFileDetail {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body = %s", path, rec.Code, rec.Body.String())
	}
	var detail agentFileDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return detail
}

func TestAgentFiles_ScopedToAgentWorkspace(t *testing.T) {
	configPath, alphaWS, betaWS := setupScopedAgentFiles(t)
	mux := http.NewServeMux()
	NewHandler(configPath).RegisterRoutes(mux)

	alpha := getScopedFile(t, mux, "/api/agent/files/agent?agent=alpha")
	if alpha.Content != "# Alpha personality\n" {
		t.Fatalf("alpha content = %q", alpha.Content)
	}
	beta := getScopedFile(t, mux, "/api/agent/files/agent?agent=beta")
	if beta.Content != "# Beta personality\n" {
		t.Fatalf("beta content = %q", beta.Content)
	}

	body, err := json.Marshal(agentFileWriteRequest{
		Content:  "# Alpha updated\n",
		Revision: alpha.Revision,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/agent/files/agent?agent=alpha", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	alphaData, err := os.ReadFile(filepath.Join(alphaWS, "AGENT.md"))
	if err != nil {
		t.Fatalf("ReadFile(alpha) error = %v", err)
	}
	if string(alphaData) != "# Alpha updated\n" {
		t.Fatalf("alpha AGENT.md = %q", alphaData)
	}
	betaData, err := os.ReadFile(filepath.Join(betaWS, "AGENT.md"))
	if err != nil {
		t.Fatalf("ReadFile(beta) error = %v", err)
	}
	if string(betaData) != "# Beta personality\n" {
		t.Fatalf("beta AGENT.md was modified: %q", betaData)
	}

	// History is scoped per agent as well.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent/files/agent/history?agent=alpha", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("alpha history status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var alphaHistory agentFileHistoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &alphaHistory); err != nil {
		t.Fatalf("Unmarshal(alpha history) error = %v", err)
	}
	if len(alphaHistory.History) == 0 {
		t.Fatal("alpha history is empty, want a backup from the update")
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent/files/agent/history?agent=beta", nil))
	var betaHistory agentFileHistoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &betaHistory); err != nil {
		t.Fatalf("Unmarshal(beta history) error = %v", err)
	}
	if len(betaHistory.History) != 0 {
		t.Fatalf("beta history = %d, want 0", len(betaHistory.History))
	}
}

func TestAgentFiles_UnknownAgentRejected(t *testing.T) {
	configPath, _, _ := setupScopedAgentFiles(t)
	mux := http.NewServeMux()
	NewHandler(configPath).RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent/files?agent=ghost", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["code"] != "invalid_agent" {
		t.Fatalf("code = %q, want invalid_agent", payload["code"])
	}
}
