package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

type profileUploadFile struct {
	path    string
	content string
}

func profileMultipartBody(t *testing.T, fields map[string]string, files []profileUploadFile) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField(%s) error = %v", key, err)
		}
	}
	for _, item := range files {
		part, err := writer.CreateFormFile("files", filepath.Base(item.path))
		if err != nil {
			t.Fatalf("CreateFormFile() error = %v", err)
		}
		if _, err := part.Write([]byte(item.content)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := writer.WriteField("paths", item.path); err != nil {
			t.Fatalf("WriteField(paths) error = %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return &buf, writer.FormDataContentType()
}

func profileZipBody(t *testing.T, fields map[string]string, files []profileUploadFile) (*bytes.Buffer, string) {
	t.Helper()
	var zipBuf bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuf)
	for _, item := range files {
		entry, err := zipWriter.Create(item.path)
		if err != nil {
			t.Fatalf("zip.Create() error = %v", err)
		}
		if _, err := entry.Write([]byte(item.content)); err != nil {
			t.Fatalf("zip.Write() error = %v", err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("zip.Close() error = %v", err)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField(%s) error = %v", key, err)
		}
	}
	part, err := writer.CreateFormFile("zip", "profile.zip")
	if err != nil {
		t.Fatalf("CreateFormFile(zip) error = %v", err)
	}
	if _, err := part.Write(zipBuf.Bytes()); err != nil {
		t.Fatalf("Write(zip) error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return &buf, writer.FormDataContentType()
}

func exampleProfileFiles() []profileUploadFile {
	return []profileUploadFile{
		{path: "socrates/AGENTS.md", content: "# Sokrates\n\nMission.\n"},
		{path: "socrates/SOUL.md", content: "# SOUL\n\nCalm and curious.\n"},
		{
			path:    "socrates/IDENTITY.md",
			content: "# IDENTITY.md\n- **Name:** Sokrates\n- **Emoji:** 🦉\n- **Vibe:** Asks the hard question.\n",
		},
		{path: "socrates/COMBINED.md", content: "# Combined\n"},
		{path: "socrates/RESEARCH.md", content: "research notes\n"},
		{path: "socrates/adapters/claude-code.md", content: "adapter\n"},
	}
}

func TestBuildProfilePreview_MapsIdentity(t *testing.T) {
	files := map[string][]byte{}
	for _, item := range exampleProfileFiles() {
		files[item.path] = []byte(item.content)
	}
	preview, err := buildProfilePreview(files)
	if err != nil {
		t.Fatalf("buildProfilePreview() error = %v", err)
	}
	if preview.Root != "socrates" {
		t.Fatalf("root = %q, want socrates", preview.Root)
	}
	if preview.Combined {
		t.Fatal("AGENTS.md should win over COMBINED.md")
	}
	if preview.Identity.Name != "Sokrates" || preview.Identity.Emoji != "🦉" {
		t.Fatalf("identity = %#v", preview.Identity)
	}
	if preview.SuggestedID != "socrates" {
		t.Fatalf("suggested id = %q, want socrates", preview.SuggestedID)
	}
	if got := string(preview.targets["AGENT.md"]); !strings.Contains(got, "Mission") {
		t.Fatalf("AGENT.md target = %q", got)
	}
	if _, ok := preview.targets["SOUL.md"]; !ok {
		t.Fatal("SOUL.md was not mapped")
	}
	if _, ok := preview.targets["IDENTITY.md"]; !ok {
		t.Fatal("IDENTITY.md was not mapped")
	}
	if len(preview.Ignored) != 2 {
		t.Fatalf("ignored = %#v, want adapters and RESEARCH", preview.Ignored)
	}
}

func TestBuildProfilePreview_CombinedFallback(t *testing.T) {
	preview, err := buildProfilePreview(map[string][]byte{
		"agent/COMBINED.md": []byte("# Combined profile\n"),
		"agent/IDENTITY.md": []byte("- **Name:** Ada\n"),
	})
	if err != nil {
		t.Fatalf("buildProfilePreview() error = %v", err)
	}
	if !preview.Combined {
		t.Fatal("combined flag = false")
	}
	if got := string(preview.targets["AGENT.md"]); !strings.Contains(got, "Combined profile") {
		t.Fatalf("AGENT.md target = %q", got)
	}
}

func TestSlugifyAgentID(t *testing.T) {
	tests := map[string]string{
		"Sokrates":         "sokrates",
		"Dr. Ada Lovelace": "dr-ada-lovelace",
		"  ":               "",
		"9 Lives":          "9-lives",
		"_hidden":          "a_hidden",
	}
	for value, want := range tests {
		if got := slugifyAgentID(value); got != want {
			t.Fatalf("slugifyAgentID(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestImportAgentProfile_Folder(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	mux := agentsTestMux(t, configPath)
	files := exampleProfileFiles()

	body, contentType := profileMultipartBody(t, nil, files)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/import/inspect", body)
	req.Header.Set("Content-Type", contentType)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var preview profilePreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if preview.Root != "socrates" || len(preview.Files) != 3 {
		t.Fatalf("preview = %#v", preview)
	}

	fields := map[string]string{
		"agent": `{"id":"socrates","name":"Sokrates"}`,
	}
	body, contentType = profileMultipartBody(t, fields, files)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agents/import", body)
	req.Header.Set("Content-Type", contentType)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Agents.List) != 1 {
		t.Fatalf("agents = %d, want 1", len(cfg.Agents.List))
	}
	agent := cfg.Agents.List[0]
	if agent.ID != "socrates" || agent.Default {
		t.Fatalf("agent = %#v", agent)
	}
	if agent.Avatar == nil || agent.Avatar.Emoji != "🦉" {
		t.Fatalf("avatar = %#v", agent.Avatar)
	}
	workspace := filepath.Join(filepath.Dir(cfg.Agents.Defaults.Workspace), "agents", "socrates")
	if content, err := os.ReadFile(filepath.Join(workspace, "AGENT.md")); err != nil || !strings.Contains(string(content), "Mission") {
		t.Fatalf("AGENT.md = %q, err = %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "SOUL.md")); err != nil {
		t.Fatalf("SOUL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "adapters")); !os.IsNotExist(err) {
		t.Fatalf("adapters should not be imported: %v", err)
	}
	if agent.Skills != nil && len(agent.Skills) == 0 {
		t.Fatal("skills list should be explicit or nil, not empty")
	}

	// A second import without overwrite must fail with 409.
	body, contentType = profileMultipartBody(t, fields, files)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agents/import", body)
	req.Header.Set("Content-Type", contentType)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("overwrite status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}

	fields["overwrite"] = "true"
	body, contentType = profileMultipartBody(t, fields, files)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/agents/import", body)
	req.Header.Set("Content-Type", contentType)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("overwrite import status = %d, body = %s", rec.Code, rec.Body.String())
	}
	backups := filepath.Join(config.GetHome(), "backups", "agent-imports", "socrates")
	entries, err := os.ReadDir(backups)
	if err != nil || len(entries) == 0 {
		t.Fatalf("backups missing: %v", err)
	}
}

func TestImportAgentProfile_AppliesAgentSettings(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	workspace := t.TempDir()
	agentJSON := fmt.Sprintf(
		`{"id":"socrates","name":"Sokrates","workspace":%q,"model":{"primary":"custom-default"},"skills":["weather"],"subagents":{"enabled":false}}`,
		workspace,
	)
	fields := map[string]string{"agent": agentJSON}
	body, contentType := profileMultipartBody(t, fields, exampleProfileFiles())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/import", body)
	req.Header.Set("Content-Type", contentType)
	agentsTestMux(t, configPath).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Agents.List) != 1 {
		t.Fatalf("agents = %d, want 1", len(cfg.Agents.List))
	}
	agent := cfg.Agents.List[0]
	if agent.Workspace != workspace {
		t.Fatalf("workspace = %q, want %q", agent.Workspace, workspace)
	}
	if agent.Model == nil || agent.Model.Primary != "custom-default" {
		t.Fatalf("model = %#v", agent.Model)
	}
	if len(agent.Skills) != 2 || agent.Skills[0] != lockedAgentSkill || agent.Skills[1] != "weather" {
		t.Fatalf("skills = %#v", agent.Skills)
	}
	if agent.Subagents == nil || agent.Subagents.IsEnabled() {
		t.Fatalf("subagents = %#v, want disabled", agent.Subagents)
	}
	if _, err := os.Stat(filepath.Join(workspace, "AGENT.md")); err != nil {
		t.Fatalf("AGENT.md missing: %v", err)
	}
}

func TestImportAgentProfile_Zip(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	body, contentType := profileZipBody(t, nil, exampleProfileFiles())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/import/inspect", body)
	req.Header.Set("Content-Type", contentType)
	agentsTestMux(t, configPath).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var preview profilePreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if preview.Root != "socrates" {
		t.Fatalf("root = %q, want socrates", preview.Root)
	}
	foundAgent := false
	for _, item := range preview.Files {
		if item.Target == "AGENT.md" {
			foundAgent = true
		}
	}
	if !foundAgent {
		t.Fatal("ZIP AGENT.md target missing")
	}
}
