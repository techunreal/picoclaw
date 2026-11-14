package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

type backupTestEnv struct {
	configPath string
	workspace  string
	home       string
	configDir  string
}

func setupBackupTestEnv(t *testing.T) backupTestEnv {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	home := filepath.Join(tmp, ".picoclaw")
	t.Setenv("PICOCLAW_HOME", home)
	t.Setenv("TMP", filepath.Join(tmp, "tmp"))
	t.Setenv("TEMP", filepath.Join(tmp, "tmp"))
	t.Setenv("TMPDIR", filepath.Join(tmp, "tmp"))

	workspace := filepath.Join(home, "workspace")
	writeTestFile(t, filepath.Join(workspace, "AGENT.md"), "# Agent\n\noriginal agent\n")
	writeTestFile(t, filepath.Join(workspace, "memory", "MEMORY.md"), "# Memory\n\nremember this\n")
	writeTestFile(t, filepath.Join(workspace, "sessions", "pico-s1.jsonl"), "{\"role\":\"user\"}\n")
	writeTestFile(t, filepath.Join(workspace, "skills", "demo", "SKILL.md"), "---\nname: demo\n---\nDemo skill.\n")
	writeTestFile(t, filepath.Join(workspace, "cron", "jobs.json"), "{\"version\":1,\"jobs\":[]}\n")
	writeTestFile(t, filepath.Join(home, "skills", "global-demo", "SKILL.md"), "---\nname: global-demo\n---\nGlobal.\n")
	writeTestFile(t, filepath.Join(home, ".security.yml"), "model_list:\n  smoke:0:\n    api_keys:\n      - sk-secret\n")
	writeTestFile(t, filepath.Join(home, "launcher-auth.db"), "password-hash")
	writeTestFile(t, filepath.Join(home, "logs", "launcher.log"), "log line\n")
	writeTestFile(t, filepath.Join(home, ".picoclaw.pid"), "{\"pid\":1}")

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = workspace
	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	return backupTestEnv{configPath: configPath, workspace: workspace, home: home, configDir: tmp}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func newBackupTestMux(t *testing.T, configPath string) *http.ServeMux {
	t.Helper()
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.registerBackupRoutes(mux)
	return mux
}

func exportBackup(t *testing.T, mux *http.ServeMux) []byte {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/backup/export", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", ct)
	}
	return rec.Body.Bytes()
}

func zipEntries(t *testing.T, data []byte) map[string]*zip.File {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	entries := make(map[string]*zip.File, len(reader.File))
	for _, entry := range reader.File {
		entries[entry.Name] = entry
	}
	return entries
}

func uploadBackup(t *testing.T, mux *http.ServeMux, data []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "backup.zip")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("part.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/backup/inspect", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestBackupExport_IncludesDataAndExcludesSecrets(t *testing.T) {
	env := setupBackupTestEnv(t)
	mux := newBackupTestMux(t, env.configPath)

	entries := zipEntries(t, exportBackup(t, mux))

	for _, name := range []string{
		"manifest.json",
		"config/config.json",
		"workspace/AGENT.md",
		"workspace/memory/MEMORY.md",
		"workspace/sessions/pico-s1.jsonl",
		"workspace/skills/demo/SKILL.md",
		"workspace/cron/jobs.json",
		"global-skills/global-demo/SKILL.md",
	} {
		if entries[name] == nil {
			t.Fatalf("archive is missing %s", name)
		}
	}

	for _, name := range []string{
		"config/.security.yml",
		"config/launcher-auth.db",
		"workspace/../.security.yml",
	} {
		if entries[name] != nil {
			t.Fatalf("archive must not contain %s", name)
		}
	}
	for name := range entries {
		if strings.Contains(name, ".security.yml") || strings.Contains(name, "launcher-auth.db") ||
			strings.Contains(name, "logs/") || strings.HasSuffix(name, ".picoclaw.pid") {
			t.Fatalf("archive leaked excluded entry %q", name)
		}
	}

	manifestFile := entries["manifest.json"]
	if manifestFile == nil {
		t.Fatal("manifest.json missing")
	}
	manifestReader, err := manifestFile.Open()
	if err != nil {
		t.Fatalf("manifest open error = %v", err)
	}
	defer manifestReader.Close()
	raw, _ := io.ReadAll(manifestReader)
	var manifest backupManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("manifest unmarshal error = %v", err)
	}
	if manifest.Format != backupFormat || manifest.Version != backupFormatVersion {
		t.Fatalf("manifest format = %q version = %d", manifest.Format, manifest.Version)
	}
	if manifest.TotalFiles < 6 {
		t.Fatalf("manifest total files = %d, want >= 6", manifest.TotalFiles)
	}
	if len(manifest.Excluded) == 0 {
		t.Fatal("manifest should list excluded secret paths")
	}
}

func TestBackupInspectAndRestore(t *testing.T) {
	env := setupBackupTestEnv(t)
	mux := newBackupTestMux(t, env.configPath)
	archive := exportBackup(t, mux)

	rec := uploadBackup(t, mux, archive)
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var inspection backupInspection
	if err := json.Unmarshal(rec.Body.Bytes(), &inspection); err != nil {
		t.Fatalf("inspect unmarshal error = %v", err)
	}
	if inspection.ID == "" {
		t.Fatal("inspection has no id")
	}
	if inspection.Overwrite == 0 {
		t.Fatal("expected overwrite conflicts when restoring into the same environment")
	}
	if inspection.New != 0 {
		t.Fatalf("new files = %d, want 0", inspection.New)
	}

	agentPath := filepath.Join(env.workspace, "AGENT.md")
	writeTestFile(t, agentPath, "# Agent\n\nmodified after backup\n")
	securityPath := filepath.Join(env.configDir, ".security.yml")
	securityBefore, _ := os.ReadFile(securityPath)

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/backup/restore",
		strings.NewReader(`{"id":"`+inspection.ID+`","overwrite":true}`))
	restoreReq.Header.Set("Content-Type", "application/json")
	restoreRec := httptest.NewRecorder()
	mux.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, body = %s", restoreRec.Code, restoreRec.Body.String())
	}

	var result backupRestoreResult
	if err := json.Unmarshal(restoreRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("restore unmarshal error = %v", err)
	}
	if result.Restored == 0 {
		t.Fatal("nothing was restored")
	}
	if !result.RestartRequired {
		t.Fatal("restart_required = false, want true after config restore")
	}

	restored, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("read restored agent file error = %v", err)
	}
	if !strings.Contains(string(restored), "original agent") {
		t.Fatalf("AGENT.md = %q, want original content", restored)
	}

	securityAfter, _ := os.ReadFile(securityPath)
	if !bytes.Equal(securityBefore, securityAfter) {
		t.Fatal("restore modified the secrets file")
	}
	backups, _ := filepath.Glob(env.configPath + ".bak-restore-*")
	if len(backups) != 1 {
		t.Fatalf("config safety backups = %v, want 1", backups)
	}
}

func TestBackupRestore_RequiresConfirmation(t *testing.T) {
	env := setupBackupTestEnv(t)
	mux := newBackupTestMux(t, env.configPath)
	archive := exportBackup(t, mux)

	rec := uploadBackup(t, mux, archive)
	var inspection backupInspection
	if err := json.Unmarshal(rec.Body.Bytes(), &inspection); err != nil {
		t.Fatalf("inspect unmarshal error = %v", err)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/backup/restore",
		strings.NewReader(`{"id":"`+inspection.ID+`","overwrite":false}`))
	restoreReq.Header.Set("Content-Type", "application/json")
	restoreRec := httptest.NewRecorder()
	mux.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", restoreRec.Code)
	}
}

func TestBackupInspect_RejectsUnsafeArchive(t *testing.T) {
	env := setupBackupTestEnv(t)
	mux := newBackupTestMux(t, env.configPath)

	unsafe := buildTestBackup(t, map[string]string{
		"manifest.json":           `{"format":"picoclaw-backup","version":1}`,
		"workspace/../../evil.md": "escaped",
	})
	rec := uploadBackup(t, mux, unsafe)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(env.configDir, "evil.md")); !os.IsNotExist(err) {
		t.Fatal("zip-slip entry was written outside the workspace")
	}
}

func TestBackupInspect_RejectsNonBackupZip(t *testing.T) {
	env := setupBackupTestEnv(t)
	mux := newBackupTestMux(t, env.configPath)

	rec := uploadBackup(t, mux, buildTestBackup(t, map[string]string{
		"readme.txt": "not a backup",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestBackupRestore_IgnoresSecretsInOlderArchives(t *testing.T) {
	env := setupBackupTestEnv(t)
	mux := newBackupTestMux(t, env.configPath)
	securityPath := filepath.Join(env.configDir, ".security.yml")
	securityBefore, _ := os.ReadFile(securityPath)

	archive := buildTestBackup(t, map[string]string{
		"manifest.json":           `{"format":"picoclaw-backup","version":1}`,
		"workspace/AGENT.md":      "# Agent\n\nfrom archive\n",
		"workspace/.security.yml": "model_list: {}\n",
	})
	rec := uploadBackup(t, mux, archive)
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var inspection backupInspection
	if err := json.Unmarshal(rec.Body.Bytes(), &inspection); err != nil {
		t.Fatalf("inspect unmarshal error = %v", err)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/backup/restore",
		strings.NewReader(`{"id":"`+inspection.ID+`","overwrite":true}`))
	restoreReq.Header.Set("Content-Type", "application/json")
	restoreRec := httptest.NewRecorder()
	mux.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, body = %s", restoreRec.Code, restoreRec.Body.String())
	}

	securityAfter, _ := os.ReadFile(securityPath)
	if !bytes.Equal(securityBefore, securityAfter) {
		t.Fatal("restore overwrote the live secrets file")
	}
}

func TestBackupRestore_RejectsInvalidConfig(t *testing.T) {
	env := setupBackupTestEnv(t)
	mux := newBackupTestMux(t, env.configPath)
	configBefore, _ := os.ReadFile(env.configPath)

	archive := buildTestBackup(t, map[string]string{
		"manifest.json":      `{"format":"picoclaw-backup","version":1}`,
		"config/config.json": "{invalid json",
		"workspace/AGENT.md": "# Agent\n\nfrom archive\n",
	})
	rec := uploadBackup(t, mux, archive)
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var inspection backupInspection
	if err := json.Unmarshal(rec.Body.Bytes(), &inspection); err != nil {
		t.Fatalf("inspect unmarshal error = %v", err)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/backup/restore",
		strings.NewReader(`{"id":"`+inspection.ID+`","overwrite":true}`))
	restoreReq.Header.Set("Content-Type", "application/json")
	restoreRec := httptest.NewRecorder()
	mux.ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", restoreRec.Code, restoreRec.Body.String())
	}

	configAfter, _ := os.ReadFile(env.configPath)
	if !bytes.Equal(configBefore, configAfter) {
		t.Fatal("invalid config was written to the live config path")
	}
}

func buildTestBackup(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("zip create %s error = %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close error = %v", err)
	}
	return buf.Bytes()
}
