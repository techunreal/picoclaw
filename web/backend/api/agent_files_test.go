package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
)

func setupAgentFilesTestEnv(t *testing.T) (configPath, workspace string) {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("PICOCLAW_HOME", filepath.Join(tmp, ".picoclaw"))

	workspace = filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace) error = %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = workspace
	configPath = filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	return configPath, workspace
}

func newAgentFilesTestMux(t *testing.T, configPath string) *http.ServeMux {
	t.Helper()
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func doAgentFileRequest(t *testing.T, mux *http.ServeMux, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeAgentFileResponse[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response error = %v body=%s", err, rec.Body.String())
	}
	return out
}

func writeAgentFileFixture(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func TestAgentFileListGroups(t *testing.T) {
	configPath, workspace := setupAgentFilesTestEnv(t)

	today := time.Now().Format("20060102")
	writeAgentFileFixture(t, filepath.Join(workspace, "AGENT.md"), "# Agent\nhello")
	writeAgentFileFixture(t, filepath.Join(workspace, "memory", "MEMORY.md"), "# Memory\nlong term")
	writeAgentFileFixture(
		t,
		filepath.Join(workspace, "memory", today[:6], today+".md"),
		"# Today\ndaily note",
	)
	writeAgentFileFixture(t, filepath.Join(workspace, "HEARTBEAT.md"), "# Heartbeat")
	writeAgentFileFixture(
		t,
		filepath.Join(workspace, "skills", "demo-skill", "SKILL.md"),
		"---\nname: demo-skill\ndescription: Demo skill\n---\n# Demo",
	)

	mux := newAgentFilesTestMux(t, configPath)
	rec := doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	resp := decodeAgentFileResponse[agentFileListResponse](t, rec)
	if len(resp.Groups) != 4 {
		t.Fatalf("expected 4 groups, got %d", len(resp.Groups))
	}

	items := map[string]agentFileItem{}
	for _, group := range resp.Groups {
		for _, item := range group.Files {
			items[item.ID] = item
			if item.Group != group.ID {
				t.Errorf("item %q group = %q, want %q", item.ID, item.Group, group.ID)
			}
		}
	}

	agent, ok := items["agent"]
	if !ok {
		t.Fatal("list should contain AGENT.md")
	}
	if !agent.Exists || agent.Size == 0 || agent.ModifiedAt == 0 || !agent.Editable {
		t.Errorf("unexpected AGENT.md item: %+v", agent)
	}
	if _, ok := items["soul"]; !ok {
		t.Error("list should contain SOUL.md even when missing (create flow)")
	}
	if item, ok := items["daily:"+today]; !ok || !item.Exists {
		t.Errorf("today's daily note should be listed as existing, got %+v", items["daily:"+today])
	}
	if item, ok := items["memory"]; !ok || !item.Exists {
		t.Errorf("MEMORY.md should be listed, got %+v", items["memory"])
	}
	if item, ok := items["heartbeat"]; !ok || !item.Exists {
		t.Errorf("HEARTBEAT.md should be listed, got %+v", items["heartbeat"])
	}
	skill, ok := items["skill:workspace:demo-skill"]
	if !ok {
		t.Fatal("workspace skill should be listed")
	}
	if !skill.Editable || skill.Path != "skills/demo-skill/SKILL.md" {
		t.Errorf("unexpected workspace skill item: %+v", skill)
	}
}

func TestAgentFileGetAndUpdateRevisionFlow(t *testing.T) {
	configPath, workspace := setupAgentFilesTestEnv(t)
	agentPath := filepath.Join(workspace, "AGENT.md")
	writeAgentFileFixture(t, agentPath, "v1")

	mux := newAgentFilesTestMux(t, configPath)

	rec := doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files/agent", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", rec.Code, rec.Body.String())
	}
	detail := decodeAgentFileResponse[agentFileDetail](t, rec)
	if detail.Content != "v1" || detail.Revision == "" || !detail.Exists {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	rev1 := detail.Revision

	rec = doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/agent", agentFileWriteRequest{
		Content:  "v2",
		Revision: rev1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	saved := decodeAgentFileResponse[agentFileWriteResponse](t, rec)
	if saved.Revision == rev1 || saved.Size != 2 || saved.Created {
		t.Fatalf("unexpected save response: %+v", saved)
	}

	content, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "v2" {
		t.Fatalf("file content = %q, want %q", string(content), "v2")
	}

	// Stale revision must be rejected and must not modify the file.
	rec = doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/agent", agentFileWriteRequest{
		Content:  "v3",
		Revision: rev1,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	conflict := decodeAgentFileResponse[map[string]any](t, rec)
	if conflict["code"] != "revision_conflict" {
		t.Errorf("conflict code = %v", conflict["code"])
	}
	content, err = os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "v2" {
		t.Fatalf("file content after conflict = %q, want %q", string(content), "v2")
	}

	// Updating an existing file without a revision must be rejected.
	rec = doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/agent", agentFileWriteRequest{
		Content: "v4",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing revision PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentFileCreateDailyNote(t *testing.T) {
	configPath, workspace := setupAgentFilesTestEnv(t)

	day := time.Now().AddDate(0, 0, -2).Format("20060102")
	mux := newAgentFilesTestMux(t, configPath)

	rec := doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/daily:"+day, agentFileWriteRequest{
		Content: "# Note\nfirst entry",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	saved := decodeAgentFileResponse[agentFileWriteResponse](t, rec)
	if !saved.Created || saved.Revision == "" {
		t.Fatalf("unexpected create response: %+v", saved)
	}

	notePath := filepath.Join(workspace, "memory", day[:6], day+".md")
	content, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", notePath, err)
	}
	if string(content) != "# Note\nfirst entry" {
		t.Fatalf("daily note content = %q", string(content))
	}

	rec = doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files/daily:"+day, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", rec.Code, rec.Body.String())
	}
	detail := decodeAgentFileResponse[agentFileDetail](t, rec)
	if !detail.Exists || detail.Revision != saved.Revision {
		t.Fatalf("unexpected daily note detail: %+v", detail)
	}
}

func TestAgentFileRejectsInvalidIDs(t *testing.T) {
	configPath, workspace := setupAgentFilesTestEnv(t)
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	invalidIDs := []string{
		"",
		"../../outside",
		`C:\Windows\win.ini`,
		"memory/../../outside",
		"daily:2026",
		"daily:20261340",
		"daily:../../etc",
		"skill:workspace:..",
		"skill:workspace:a/b",
		"skill:unknown:demo",
		"skill:workspace",
		"unknown-file",
	}
	for _, id := range invalidIDs {
		if _, err := resolveAgentFileTarget(cfg, id); !errors.Is(err, errAgentFileInvalidID) && err == nil {
			t.Errorf("resolveAgentFileTarget(%q) should fail, got nil error", id)
		}
	}

	mux := newAgentFilesTestMux(t, configPath)
	for _, target := range []string{
		"/api/agent/files/unknown-file",
		"/api/agent/files/skill:workspace:..",
	} {
		rec := doAgentFileRequest(t, mux, http.MethodGet, target, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400", target, rec.Code)
		}
	}

	// A path traversal style request must never create a file outside the workspace.
	_ = doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/..%2Fescape", agentFileWriteRequest{
		Content: "pwned",
	})
	if _, err := os.Stat(filepath.Join(workspace, "..", "escape")); err == nil {
		t.Fatal("path traversal request created a file outside the workspace")
	}

	// Legacy files may be edited but not created.
	rec := doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/agents", agentFileWriteRequest{
		Content: "# Legacy",
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("creating AGENTS.md status = %d, want 404", rec.Code)
	}
}

func TestAgentFileRejectsBinaryAndOversizedContent(t *testing.T) {
	configPath, workspace := setupAgentFilesTestEnv(t)
	writeAgentFileFixture(t, filepath.Join(workspace, "AGENT.md"), "v1")

	mux := newAgentFilesTestMux(t, configPath)
	rec := doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files/agent", nil)
	detail := decodeAgentFileResponse[agentFileDetail](t, rec)

	rec = doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/agent", agentFileWriteRequest{
		Content:  "bad\x00content",
		Revision: detail.Revision,
	})
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("binary content status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/agent", agentFileWriteRequest{
		Content:  strings.Repeat("a", maxAgentFileSize+1),
		Revision: detail.Revision,
	})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized content status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAgentFileHistoryAndRestore(t *testing.T) {
	configPath, workspace := setupAgentFilesTestEnv(t)
	writeAgentFileFixture(t, filepath.Join(workspace, "SOUL.md"), "one")

	mux := newAgentFilesTestMux(t, configPath)

	rec := doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files/soul", nil)
	rev1 := decodeAgentFileResponse[agentFileDetail](t, rec).Revision

	rec = doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/soul", agentFileWriteRequest{
		Content:  "two",
		Revision: rev1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files/soul/history", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("history status = %d body=%s", rec.Code, rec.Body.String())
	}
	history := decodeAgentFileResponse[agentFileHistoryResponse](t, rec)
	if len(history.History) != 1 || history.History[0].Revision != rev1 {
		t.Fatalf("unexpected history: %+v", history)
	}
	if history.History[0].CreatedAt == 0 || history.History[0].Size != 3 {
		t.Errorf("unexpected history entry: %+v", history.History[0])
	}

	rec = doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files/soul/history/"+rev1, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("history content status = %d body=%s", rec.Code, rec.Body.String())
	}
	old := decodeAgentFileResponse[agentFileHistoryContentResponse](t, rec)
	if old.Content != "one" || old.Revision != rev1 {
		t.Fatalf("unexpected history content: %+v", old)
	}

	rec = doAgentFileRequest(t, mux, http.MethodPost, "/api/agent/files/soul/restore", agentFileRestoreRequest{
		Revision: rev1,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d body=%s", rec.Code, rec.Body.String())
	}
	restored := decodeAgentFileResponse[agentFileWriteResponse](t, rec)
	if restored.Status != "restored" || restored.Revision != rev1 {
		t.Fatalf("unexpected restore response: %+v", restored)
	}

	content, err := os.ReadFile(filepath.Join(workspace, "SOUL.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "one" {
		t.Fatalf("restored content = %q, want %q", string(content), "one")
	}

	// Restoring must keep a backup of the content it replaced.
	rec = doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files/soul/history", nil)
	history = decodeAgentFileResponse[agentFileHistoryResponse](t, rec)
	if len(history.History) != 2 {
		t.Fatalf("expected 2 backups after restore, got %d: %+v", len(history.History), history.History)
	}

	// Unknown revisions are rejected.
	rec = doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files/soul/history/"+
		strings.Repeat("0", 64), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown revision status = %d, want 404", rec.Code)
	}
}

func TestAgentFileGlobalSkillIsReadOnly(t *testing.T) {
	configPath, _ := setupAgentFilesTestEnv(t)

	globalSkill := filepath.Join(globalConfigDir(), "skills", "global-demo", "SKILL.md")
	writeAgentFileFixture(t, globalSkill, "---\nname: global-demo\ndescription: Global demo\n---\n# Global")

	mux := newAgentFilesTestMux(t, configPath)

	rec := doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files", nil)
	resp := decodeAgentFileResponse[agentFileListResponse](t, rec)
	found := false
	for _, group := range resp.Groups {
		for _, item := range group.Files {
			if item.ID != "skill:global:global-demo" {
				continue
			}
			found = true
			if item.Editable {
				t.Error("global skill must not be editable")
			}
		}
	}
	if !found {
		t.Fatal("global skill should be listed")
	}

	rec = doAgentFileRequest(t, mux, http.MethodGet, "/api/agent/files/skill:global:global-demo", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("global skill detail status = %d body=%s", rec.Code, rec.Body.String())
	}
	detail := decodeAgentFileResponse[agentFileDetail](t, rec)
	if !strings.Contains(detail.Content, "Global demo") {
		t.Errorf("unexpected global skill content: %q", detail.Content)
	}

	rec = doAgentFileRequest(t, mux, http.MethodPut, "/api/agent/files/skill:global:global-demo",
		agentFileWriteRequest{Content: "# changed", Revision: detail.Revision})
	if rec.Code != http.StatusForbidden {
		t.Errorf("global skill PUT status = %d, want 403", rec.Code)
	}
}
