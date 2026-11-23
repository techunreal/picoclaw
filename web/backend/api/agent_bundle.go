package api

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/routing"
)

const (
	agentBundleFormat       = "picoclaw-agent-bundle"
	agentBundleFormatVer    = 1
	maxAgentBundleZipBytes  = 100 << 20
	maxAgentBundleEntries   = 20000
	maxAgentBundleFileBytes = 20 << 20
	maxAgentBundleTotal     = 300 << 20
)

type agentBundleManifest struct {
	Format            string `json:"format"`
	FormatVersion     int    `json:"format_version"`
	CreatedAt         string `json:"created_at"`
	AppVersion        string `json:"app_version"`
	AgentID           string `json:"agent_id"`
	AgentName         string `json:"agent_name,omitempty"`
	OriginalWorkspace string `json:"original_workspace,omitempty"`
	Files             int    `json:"files"`
	Bytes             int64  `json:"bytes"`
}

func (h *Handler) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := routing.NormalizeAgentID(strings.TrimSpace(r.PathValue("id")))
	if id == routing.DefaultAgentID {
		writeAgentsAPIError(w, http.StatusBadRequest, "default_agent",
			"The default agent cannot be deleted.")
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}

	index := -1
	for i := range cfg.Agents.List {
		if routing.NormalizeAgentID(strings.TrimSpace(cfg.Agents.List[i].ID)) == id {
			index = i
			break
		}
	}
	if index < 0 {
		writeAgentsAPIError(w, http.StatusNotFound, "agent_not_found", "Agent not found.")
		return
	}
	agentCfg := cfg.Agents.List[index]
	if agentCfg.Default {
		writeAgentsAPIError(w, http.StatusBadRequest, "default_agent",
			"The default agent cannot be deleted.")
		return
	}

	workspace, source := effectiveAgentWorkspace(cfg, agentCfg)
	managedRoot := filepath.Dir(derivedAgentWorkspace(cfg, "probe"))
	canDeleteFiles := source != "default" && pathInsideDir(managedRoot, workspace)

	cfg.Agents.List = append(cfg.Agents.List[:index], cfg.Agents.List[index+1:]...)
	removeAgentReferences(cfg, id)

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_save_failed", err.Error())
		return
	}

	filesDeleted := false
	warning := ""
	if canDeleteFiles {
		if err := os.RemoveAll(workspace); err != nil {
			warning = "The agent was removed from config, but its files could not be deleted: " + err.Error()
		} else {
			filesDeleted = true
		}
	} else {
		warning = "The agent was removed from config, but its workspace is outside the managed agents folder and was left on disk."
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"id":               id,
		"workspace":        workspace,
		"files_deleted":    filesDeleted,
		"warning":          warning,
		"restart_required": true,
	})
}

func removeAgentReferences(cfg *config.Config, id string) {
	target := routing.NormalizeAgentID(strings.TrimSpace(id))
	for i := range cfg.Agents.List {
		sub := cfg.Agents.List[i].Subagents
		if sub == nil || len(sub.AllowAgents) == 0 {
			continue
		}
		allowed := sub.AllowAgents[:0]
		for _, candidate := range sub.AllowAgents {
			if routing.NormalizeAgentID(strings.TrimSpace(candidate)) != target {
				allowed = append(allowed, candidate)
			}
		}
		sub.AllowAgents = allowed
	}
	if cfg.Agents.Dispatch != nil {
		rules := cfg.Agents.Dispatch.Rules[:0]
		for _, rule := range cfg.Agents.Dispatch.Rules {
			if routing.NormalizeAgentID(strings.TrimSpace(rule.Agent)) != target {
				rules = append(rules, rule)
			}
		}
		cfg.Agents.Dispatch.Rules = rules
		if len(rules) == 0 {
			cfg.Agents.Dispatch = nil
		}
	}
}

func (h *Handler) handleExportAgent(w http.ResponseWriter, r *http.Request) {
	id := routing.NormalizeAgentID(strings.TrimSpace(r.PathValue("id")))
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}
	agentCfg, ok := findAgentConfig(cfg, id)
	if !ok {
		writeAgentsAPIError(w, http.StatusNotFound, "agent_not_found", "Agent not found.")
		return
	}
	workspace, _ := effectiveAgentWorkspace(cfg, *agentCfg)
	if info, statErr := os.Stat(workspace); statErr != nil || !info.IsDir() {
		writeAgentsAPIError(w, http.StatusNotFound, "workspace_not_found",
			"The agent workspace does not exist.")
		return
	}

	files, bytes, err := countBundleFiles(workspace)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "bundle_count_failed", err.Error())
		return
	}
	if bytes > maxAgentBundleTotal {
		writeAgentsAPIError(w, http.StatusRequestEntityTooLarge, "bundle_too_large",
			"The agent workspace is too large to export.")
		return
	}

	tmp, err := os.CreateTemp("", "picoclaw-agent-export-*.zip")
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "bundle_create_failed", err.Error())
		return
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	manifest := agentBundleManifest{
		Format:            agentBundleFormat,
		FormatVersion:     agentBundleFormatVer,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
		AppVersion:        strings.TrimSpace(config.Version),
		AgentID:           id,
		AgentName:         agentCfg.Name,
		OriginalWorkspace: workspace,
		Files:             files,
		Bytes:             bytes,
	}
	if manifest.AppVersion == "" {
		manifest.AppVersion = "dev"
	}
	if err := writeAgentBundle(tmpPath, workspace, manifest, *agentCfg); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "bundle_export_failed", err.Error())
		return
	}

	name := fmt.Sprintf("picoclaw-agent-%s-%s.zip", id, time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, tmpPath)
}

func (h *Handler) handleImportAgentBundle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAgentBundleZipBytes+10<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_bundle", "Expected a ZIP upload: "+err.Error())
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		file, _, err = r.FormFile("zip")
	}
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_bundle", "Upload a ZIP file in the 'file' field.")
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "picoclaw-agent-import-*.zip")
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "bundle_create_failed", err.Error())
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	written, copyErr := io.Copy(tmp, io.LimitReader(file, maxAgentBundleZipBytes+1))
	closeErr := tmp.Close()
	if copyErr != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_bundle", copyErr.Error())
		return
	}
	if closeErr != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "bundle_read_failed", closeErr.Error())
		return
	}
	if written > maxAgentBundleZipBytes {
		writeAgentsAPIError(w, http.StatusRequestEntityTooLarge, "bundle_too_large", "The ZIP is too large.")
		return
	}

	reader, err := zip.OpenReader(tmpPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_bundle", "Invalid ZIP: "+err.Error())
		return
	}
	defer reader.Close()
	if len(reader.File) > maxAgentBundleEntries {
		writeAgentsAPIError(w, http.StatusRequestEntityTooLarge, "bundle_too_large", "The ZIP has too many entries.")
		return
	}
	var declaredTotal uint64
	for _, entry := range reader.File {
		declaredTotal += entry.UncompressedSize64
		if entry.UncompressedSize64 > maxAgentBundleFileBytes {
			writeAgentsAPIError(w, http.StatusRequestEntityTooLarge, "bundle_too_large",
				fmt.Sprintf("%s is too large.", entry.Name))
			return
		}
	}
	if declaredTotal > maxAgentBundleTotal {
		writeAgentsAPIError(w, http.StatusRequestEntityTooLarge, "bundle_too_large",
			"The extracted bundle would be too large.")
		return
	}

	var manifest agentBundleManifest
	var incoming config.AgentConfig
	hasManifest, hasAgent := false, false
	for _, entry := range reader.File {
		switch entry.Name {
		case "manifest.json":
			if err := decodeZipJSON(entry, &manifest); err != nil {
				writeAgentsAPIError(w, http.StatusBadRequest, "invalid_bundle", "manifest.json: "+err.Error())
				return
			}
			hasManifest = true
		case "agent.json":
			if err := decodeZipJSON(entry, &incoming); err != nil {
				writeAgentsAPIError(w, http.StatusBadRequest, "invalid_bundle", "agent.json: "+err.Error())
				return
			}
			hasAgent = true
		}
	}
	if !hasManifest || !hasAgent {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_bundle",
			"The bundle must contain manifest.json and agent.json.")
		return
	}
	if manifest.Format != agentBundleFormat || manifest.FormatVersion != agentBundleFormatVer {
		writeAgentsAPIError(w, http.StatusBadRequest, "unsupported_bundle",
			"Unsupported agent bundle format.")
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}
	if len(cfg.Agents.List) >= maxAgents {
		writeAgentsAPIError(w, http.StatusBadRequest, "too_many_agents", "The agent limit has been reached.")
		return
	}

	agentsRoot := filepath.Dir(derivedAgentWorkspace(cfg, "probe"))
	if err := os.MkdirAll(agentsRoot, 0o755); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "workspace_create_failed", err.Error())
		return
	}
	staging, err := os.MkdirTemp(agentsRoot, ".import-")
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "workspace_create_failed", err.Error())
		return
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(staging)
		}
	}()

	files, err := extractBundleWorkspace(reader.File, staging)
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_bundle", err.Error())
		return
	}
	if files == 0 {
		writeAgentsAPIError(w, http.StatusBadRequest, "empty_bundle", "The bundle has no workspace files.")
		return
	}

	originalID := strings.TrimSpace(incoming.ID)
	if originalID == "" {
		originalID = strings.TrimSpace(manifest.AgentID)
	}
	id := normalizeBundleAgentID(originalID, incoming.Name)
	id = uniqueBundleAgentID(cfg, agentsRoot, id)

	finalWorkspace := filepath.Join(agentsRoot, id)
	if err := os.Rename(staging, finalWorkspace); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "workspace_create_failed", err.Error())
		return
	}
	cleanupStaging = false
	rollback := true
	defer func() {
		if rollback {
			_ = os.RemoveAll(finalWorkspace)
		}
	}()

	incoming.ID = id
	incoming.Workspace = ""
	incoming.Default = false
	if strings.TrimSpace(incoming.Name) == "" {
		incoming.Name = id
	} else if !strings.EqualFold(strings.TrimSpace(originalID), id) {
		incoming.Name = strings.TrimSpace(incoming.Name) + " (imported)"
	}
	cfg.Agents.List = append(cfg.Agents.List, incoming)
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_save_failed", err.Error())
		return
	}
	rollback = false

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"agent":            id,
		"name":             incoming.Name,
		"renamed":          !strings.EqualFold(strings.TrimSpace(originalID), id),
		"restart_required": true,
	})
}

func findAgentConfig(cfg *config.Config, id string) (*config.AgentConfig, bool) {
	id = routing.NormalizeAgentID(strings.TrimSpace(id))
	for i := range cfg.Agents.List {
		if routing.NormalizeAgentID(strings.TrimSpace(cfg.Agents.List[i].ID)) == id {
			return &cfg.Agents.List[i], true
		}
	}
	if id == routing.DefaultAgentID {
		return &config.AgentConfig{ID: id, Name: "main", Default: true}, true
	}
	return nil, false
}

func pathInsideDir(base, target string) bool {
	if strings.TrimSpace(base) == "" || strings.TrimSpace(target) == "" {
		return false
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func countBundleFiles(workspace string) (int, int64, error) {
	files := 0
	var bytes int64
	err := filepath.WalkDir(workspace, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes, err
}

func writeAgentBundle(zipPath, workspace string, manifest agentBundleManifest, agentCfg config.AgentConfig) error {
	file, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(file)
	closeAll := func() error {
		if err := zw.Close(); err != nil {
			_ = file.Close()
			return err
		}
		return file.Close()
	}

	if err := writeBundleJSON(zw, "manifest.json", manifest); err != nil {
		_ = closeAll()
		return err
	}
	agentCfg.Workspace = ""
	agentCfg.Default = false
	if err := writeBundleJSON(zw, "agent.json", agentCfg); err != nil {
		_ = closeAll()
		return err
	}
	if err := writeBundleText(zw, "README.txt", bundleReadme(manifest)); err != nil {
		_ = closeAll()
		return err
	}

	err = filepath.WalkDir(workspace, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(workspace, p)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return writeBundleFile(zw, "workspace/"+filepath.ToSlash(rel), p, info)
	})
	if err != nil {
		_ = closeAll()
		return err
	}
	return closeAll()
}

func writeBundleJSON(zw *zip.Writer, name string, value any) error {
	entry, err := zw.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(entry)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func writeBundleText(zw *zip.Writer, name, text string) error {
	entry, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(entry, text)
	return err
}

func writeBundleFile(zw *zip.Writer, name, source string, info fs.FileInfo) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: info.ModTime()}
	entry, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(entry, file)
	return err
}

func bundleReadme(manifest agentBundleManifest) string {
	return fmt.Sprintf(`PicoClaw agent bundle
=====================

Agent: %s (%s)
Created: %s
Format: %s v%d

Contents
--------
- agent.json: the agent settings snapshot
- manifest.json: bundle metadata
- workspace/: all files from the agent workspace

Import
------
PicoClaw: Agents -> Import agent, then choose this ZIP.
Other harnesses: use workspace/AGENT.md as the job file and workspace/SOUL.md
as the persona file. workspace/profiles/ contains generated profiles.

Warning
-------
workspace/sessions/, workspace/memory/ and workspace/browser/ may contain
private conversation data and screenshots. Treat this ZIP as sensitive.
`, manifest.AgentName, manifest.AgentID, manifest.CreatedAt, manifest.Format, manifest.FormatVersion)
}

func decodeZipJSON(entry *zip.File, target any) error {
	rc, err := entry.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil {
		return err
	}
	if len(data) >= 1<<20 {
		return errors.New("JSON entry is too large")
	}
	return json.Unmarshal(data, target)
}

func extractBundleWorkspace(entries []*zip.File, staging string) (int, error) {
	files := 0
	var total int64
	for _, entry := range entries {
		if entry.FileInfo().IsDir() {
			continue
		}
		clean, err := cleanProfilePath(entry.Name)
		if err != nil {
			return 0, err
		}
		if clean == "manifest.json" || clean == "agent.json" || clean == "README.txt" {
			continue
		}
		if !strings.HasPrefix(clean, "workspace/") {
			continue
		}
		rel := strings.TrimPrefix(clean, "workspace/")
		if rel == "" {
			continue
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("symlinks are not allowed: %s", entry.Name)
		}
		target := filepath.Join(staging, filepath.FromSlash(rel))
		if !pathInsideDir(staging, target) {
			return 0, fmt.Errorf("unsafe path: %s", entry.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return 0, err
		}
		rc, err := entry.Open()
		if err != nil {
			return 0, err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			_ = rc.Close()
			return 0, err
		}
		written, copyErr := io.Copy(out, io.LimitReader(rc, maxAgentBundleFileBytes+1))
		_ = rc.Close()
		_ = out.Close()
		if copyErr != nil {
			return 0, copyErr
		}
		if written > maxAgentBundleFileBytes {
			return 0, fmt.Errorf("%s is too large", entry.Name)
		}
		total += written
		if total > maxAgentBundleTotal {
			return 0, errors.New("the extracted bundle is too large")
		}
		files++
	}
	return files, nil
}

func normalizeBundleAgentID(rawID, name string) string {
	id := routing.NormalizeAgentID(strings.TrimSpace(rawID))
	if !agentIDPattern.MatchString(id) || id == routing.DefaultAgentID {
		id = slugBundleName(name)
	}
	if id == "" || id == routing.DefaultAgentID {
		id = "imported-agent"
	}
	return id
}

func slugBundleName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if utf8.RuneCountInString(b.String()) >= 60 {
			break
		}
	}
	return strings.Trim(b.String(), "-_")
}

func uniqueBundleAgentID(cfg *config.Config, agentsRoot, base string) string {
	candidate := base
	for suffix := 0; ; suffix++ {
		if suffix == 1 {
			candidate = base + "-imported"
		} else if suffix > 1 {
			candidate = fmt.Sprintf("%s-imported-%d", base, suffix)
		}
		if len(candidate) > 64 {
			candidate = candidate[:64]
		}
		exists := false
		for i := range cfg.Agents.List {
			if routing.NormalizeAgentID(strings.TrimSpace(cfg.Agents.List[i].ID)) == candidate {
				exists = true
				break
			}
		}
		if !exists {
			if _, err := os.Stat(filepath.Join(agentsRoot, candidate)); os.IsNotExist(err) {
				return candidate
			}
		}
	}
}
