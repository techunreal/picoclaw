package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func bundleTestEnv(t *testing.T) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("PICOCLAW_HOME", filepath.Join(tmp, ".picoclaw"))

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(tmp, ".picoclaw", "workspace")
	cfg.Agents.List = []config.AgentConfig{{ID: "alpha", Name: "Alpha"}}
	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	workspace := filepath.Join(tmp, ".picoclaw", "agents", "alpha")
	writeBundleTestFiles(t, workspace)
	return configPath, workspace
}

func writeBundleTestFiles(t *testing.T, workspace string) {
	t.Helper()
	files := map[string]string{
		"AGENT.md":          "# Alpha\n",
		"SOUL.md":           "# Alpha soul\n",
		"profiles/one.md":   "# Profile\n",
		"sessions/s1.jsonl": `{"role":"user","content":"hello"}` + "\n",
		"memory/notes.txt":  "memory\n",
	}
	for name, content := range files {
		target := filepath.Join(workspace, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
}

func bundleMultipart(t *testing.T, zipBytes []byte) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "agent.zip")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(zipBytes); err != nil {
		t.Fatalf("write form file error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	return &body, writer.FormDataContentType()
}

func TestDeleteAgentRemovesConfigAndWorkspace(t *testing.T) {
	configPath, workspace := bundleTestEnv(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/alpha", nil)
	agentsTestMux(t, configPath).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["files_deleted"] != true {
		t.Fatalf("files_deleted = %v, want true; body=%s", resp["files_deleted"], rec.Body.String())
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Agents.List) != 0 {
		t.Fatalf("agents = %d, want 0", len(cfg.Agents.List))
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists: %v", err)
	}
}

func TestDeleteAgentRefusesDefaultAgent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("PICOCLAW_HOME", filepath.Join(tmp, ".picoclaw"))
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(tmp, ".picoclaw", "workspace")
	cfg.Agents.List = []config.AgentConfig{{ID: "main", Name: "main", Default: true}}
	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/main", nil)
	agentsTestMux(t, configPath).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteAgentLeavesExternalWorkspace(t *testing.T) {
	configPath, _ := bundleTestEnv(t)
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Agents.List = []config.AgentConfig{{ID: "beta", Name: "Beta", Workspace: external}}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/beta", nil)
	agentsTestMux(t, configPath).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["files_deleted"] != false {
		t.Fatalf("files_deleted = %v, want false", resp["files_deleted"])
	}
	if _, err := os.Stat(filepath.Join(external, "keep.txt")); err != nil {
		t.Fatalf("external workspace was touched: %v", err)
	}
}

func TestExportAgentBundle(t *testing.T) {
	configPath, _ := bundleTestEnv(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents/alpha/export", nil)
	agentsTestMux(t, configPath).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, ".zip") {
		t.Fatalf("Content-Disposition = %q, want a zip filename", cd)
	}

	reader, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	names := map[string]bool{}
	var manifest agentBundleManifest
	var agentCfg config.AgentConfig
	for _, entry := range reader.File {
		names[entry.Name] = true
		switch entry.Name {
		case "manifest.json":
			if err := decodeZipJSON(entry, &manifest); err != nil {
				t.Fatal(err)
			}
		case "agent.json":
			if err := decodeZipJSON(entry, &agentCfg); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, want := range []string{
		"manifest.json", "agent.json", "README.txt",
		"workspace/AGENT.md", "workspace/SOUL.md",
		"workspace/profiles/one.md", "workspace/sessions/s1.jsonl",
	} {
		if !names[want] {
			t.Fatalf("bundle missing %q; names=%v", want, names)
		}
	}
	if manifest.Format != agentBundleFormat || manifest.FormatVersion != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	if agentCfg.ID != "alpha" {
		t.Fatalf("agent id = %q, want alpha", agentCfg.ID)
	}
	if agentCfg.Workspace != "" {
		t.Fatalf("agent workspace = %q, want empty for portability", agentCfg.Workspace)
	}
}

func TestImportAgentBundleRoundTrip(t *testing.T) {
	_, sourceWorkspace := bundleTestEnv(t)
	zipPath := filepath.Join(t.TempDir(), "bundle.zip")
	manifest := agentBundleManifest{
		Format: agentBundleFormat, FormatVersion: 1,
		AgentID: "alpha", AgentName: "Alpha",
	}
	if err := writeAgentBundle(zipPath, sourceWorkspace, manifest, config.AgentConfig{
		ID: "alpha", Name: "Alpha",
	}); err != nil {
		t.Fatalf("writeAgentBundle() error = %v", err)
	}
	zipBytes, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("PICOCLAW_HOME", filepath.Join(tmp, ".picoclaw"))
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(tmp, ".picoclaw", "workspace")
	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	mux := agentsTestMux(t, configPath)

	// First import keeps the original ID.
	body, contentType := bundleMultipart(t, zipBytes)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/import-bundle", body)
	req.Header.Set("Content-Type", contentType)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first import status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var first map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &first)
	if first["agent"] != "alpha" {
		t.Fatalf("first import agent = %v, want alpha", first["agent"])
	}
	importedWorkspace := filepath.Join(tmp, ".picoclaw", "agents", "alpha")
	if _, err := os.Stat(filepath.Join(importedWorkspace, "AGENT.md")); err != nil {
		t.Fatalf("imported AGENT.md missing: %v", err)
	}
	loaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Agents.List) != 1 || loaded.Agents.List[0].ID != "alpha" {
		t.Fatalf("agents = %+v, want alpha", loaded.Agents.List)
	}

	// A second import must not overwrite; it gets a new ID.
	body, contentType = bundleMultipart(t, zipBytes)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agents/import-bundle", body)
	req.Header.Set("Content-Type", contentType)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second import status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var second map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	if second["renamed"] != true || second["agent"] != "alpha-imported" {
		t.Fatalf("second import = %v, want alpha-imported/renamed", second)
	}
}

func TestImportAgentBundleRejectsTraversal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("PICOCLAW_HOME", filepath.Join(tmp, ".picoclaw"))
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = filepath.Join(tmp, ".picoclaw", "workspace")
	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_ = writeBundleText(zw, "manifest.json", `{"format":"picoclaw-agent-bundle","format_version":1}`)
	_ = writeBundleText(zw, "agent.json", `{"id":"evil","name":"Evil"}`)
	_ = writeBundleText(zw, "workspace/../../../evil.txt", "outside")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	body, contentType := bundleMultipart(t, buf.Bytes())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/import-bundle", body)
	req.Header.Set("Content-Type", contentType)
	agentsTestMux(t, configPath).ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("traversal import unexpectedly succeeded: %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(tmp, "evil.txt")); !os.IsNotExist(err) {
		t.Fatalf("traversal wrote outside the workspace: %v", err)
	}
}
