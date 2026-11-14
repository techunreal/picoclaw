package api

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/fileutil"
	"github.com/sipeed/picoclaw/pkg/logger"
)

const (
	backupFormat           = "picoclaw-backup"
	backupFormatVersion    = 1
	backupManifestName     = "manifest.json"
	maxBackupRequestBytes  = 512 << 20
	maxBackupEntries       = 20000
	maxBackupEntryBytes    = 256 << 20
	maxBackupTotalBytes    = 2 << 30
	maxBackupManifestBytes = 1 << 20
	backupUploadTTL        = time.Hour
	backupSampleLimit      = 25
)

type backupSection struct {
	Name  string `json:"name"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

type backupExclusion struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type backupManifest struct {
	Format     string            `json:"format"`
	Version    int               `json:"version"`
	CreatedAt  string            `json:"created_at"`
	AppVersion string            `json:"app_version,omitempty"`
	Platform   string            `json:"platform"`
	Sections   []backupSection   `json:"sections"`
	TotalFiles int               `json:"total_files"`
	TotalBytes int64             `json:"total_bytes"`
	Excluded   []backupExclusion `json:"excluded"`
	Skipped    []backupExclusion `json:"skipped,omitempty"`
}

type backupUpload struct {
	path     string
	filename string
	expires  time.Time
}

type backupInspection struct {
	ID              string         `json:"id"`
	Filename        string         `json:"filename"`
	Manifest        backupManifest `json:"manifest"`
	Files           int            `json:"files"`
	Bytes           int64          `json:"bytes"`
	Overwrite       int            `json:"overwrite"`
	New             int            `json:"new"`
	Ignored         int            `json:"ignored"`
	OverwriteSample []string       `json:"overwrite_sample,omitempty"`
	Warnings        []string       `json:"warnings,omitempty"`
}

type backupRestoreResult struct {
	Restored        int      `json:"restored"`
	Ignored         int      `json:"ignored"`
	Warnings        []string `json:"warnings,omitempty"`
	RestartRequired bool     `json:"restart_required"`
}

func (h *Handler) registerBackupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backup/export", h.handleBackupExport)
	mux.HandleFunc("POST /api/backup/inspect", h.handleBackupInspect)
	mux.HandleFunc("POST /api/backup/restore", h.handleBackupRestore)
}

func (h *Handler) handleBackupExport(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, "Failed to load config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	workspace := cfg.WorkspacePath()
	if workspace == "" {
		http.Error(w, "No workspace configured", http.StatusBadRequest)
		return
	}

	manifest := backupManifest{
		Format:     backupFormat,
		Version:    backupFormatVersion,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		AppVersion: config.Version,
		Platform:   runtime.GOOS,
		Excluded: []backupExclusion{
			{Path: ".security.yml", Reason: "secrets"},
			{Path: "launcher-auth.db", Reason: "dashboard credentials"},
			{Path: ".picoclaw.pid", Reason: "runtime state"},
			{Path: "logs/", Reason: "logs"},
			{Path: "backups/", Reason: "previous local backups"},
		},
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		"attachment; filename=%q", "picoclaw-backup-"+time.Now().Format("20060102-150405")+".zip",
	))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	zipWriter := zip.NewWriter(w)
	sections := make([]*backupSection, 0, 3)
	sectionFor := func(name string) *backupSection {
		section := &backupSection{Name: name}
		sections = append(sections, section)
		return section
	}

	configSection := sectionFor("config")
	addBackupFile(zipWriter, h.configPath, "config/config.json", configSection, &manifest)

	workspaceSection := sectionFor("workspace")
	addBackupTree(zipWriter, workspace, "workspace", workspaceSection, &manifest)

	globalSkills := filepath.Join(config.GetHome(), "skills")
	if info, statErr := os.Stat(globalSkills); statErr == nil && info.IsDir() {
		globalSection := sectionFor("global-skills")
		addBackupTree(zipWriter, globalSkills, "global-skills", globalSection, &manifest)
	}

	manifest.Sections = make([]backupSection, 0, len(sections))
	for _, section := range sections {
		manifest.Sections = append(manifest.Sections, *section)
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		logger.Errorf("Failed to encode backup manifest: %v", err)
		_ = zipWriter.Close()
		return
	}
	manifestWriter, err := zipWriter.Create(backupManifestName)
	if err == nil {
		_, _ = manifestWriter.Write(manifestJSON)
	}
	if err := zipWriter.Close(); err != nil {
		logger.Errorf("Failed to finish backup archive: %v", err)
	}
}

func addBackupTree(
	zipWriter *zip.Writer,
	baseDir, archiveBase string,
	section *backupSection,
	manifest *backupManifest,
) {
	err := filepath.WalkDir(baseDir, func(localPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			manifest.Skipped = append(manifest.Skipped, backupExclusion{
				Path:   archivePathFor(baseDir, archiveBase, localPath),
				Reason: "unreadable",
			})
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			manifest.Skipped = append(manifest.Skipped, backupExclusion{
				Path:   archivePathFor(baseDir, archiveBase, localPath),
				Reason: "symlink",
			})
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		addBackupFile(zipWriter, localPath, archivePathFor(baseDir, archiveBase, localPath), section, manifest)
		return nil
	})
	if err != nil {
		logger.Warnf("Backup walk failed for %s: %v", baseDir, err)
	}
}

func archivePathFor(baseDir, archiveBase, localPath string) string {
	rel, err := filepath.Rel(baseDir, localPath)
	if err != nil {
		return archiveBase
	}
	return archiveBase + "/" + filepath.ToSlash(rel)
}

func addBackupFile(
	zipWriter *zip.Writer,
	localPath, archivePath string,
	section *backupSection,
	manifest *backupManifest,
) {
	info, err := os.Lstat(localPath)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	if info.Size() > maxBackupEntryBytes {
		manifest.Skipped = append(manifest.Skipped, backupExclusion{Path: archivePath, Reason: "too_large"})
		return
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return
	}
	header.Name = archivePath
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		logger.Warnf("Backup: failed to add %s: %v", archivePath, err)
		return
	}
	file, err := os.Open(localPath)
	if err != nil {
		logger.Warnf("Backup: failed to open %s: %v", localPath, err)
		return
	}
	defer file.Close()

	written, err := io.Copy(writer, file)
	if err != nil {
		logger.Warnf("Backup: failed to copy %s: %v", localPath, err)
		return
	}
	section.Files++
	section.Bytes += written
	manifest.TotalFiles++
	manifest.TotalBytes += written
}

func (h *Handler) handleBackupInspect(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBackupError(w, http.StatusInternalServerError, "config_unreadable", "Failed to read the configuration.")
		return
	}

	uploadPath, filename, err := h.readBackupUpload(w, r)
	if err != nil {
		return
	}

	inspection, err := h.inspectBackup(uploadPath, filename, cfg)
	if err != nil {
		_ = os.Remove(uploadPath)
		status := http.StatusBadRequest
		code := "invalid_backup"
		switch {
		case errors.Is(err, errBackupNotPicoClaw):
			code = "not_a_picoclaw_backup"
		case errors.Is(err, errBackupTooLarge):
			status = http.StatusRequestEntityTooLarge
			code = "too_large"
		case errors.Is(err, errBackupZipSlip):
			code = "unsafe_archive"
		case errors.Is(err, errBackupUnsupportedVersion):
			code = "unsupported_version"
		}
		writeBackupError(w, status, code, err.Error())
		return
	}

	now := time.Now()
	h.backupMu.Lock()
	h.cleanupBackupUploadsLocked(now)
	token := generateSecureToken()
	h.backupUploads[token] = &backupUpload{
		path:     uploadPath,
		filename: filename,
		expires:  now.Add(backupUploadTTL),
	}
	h.backupMu.Unlock()

	inspection.ID = token
	writeBackupJSON(w, http.StatusOK, inspection)
}

var (
	errBackupNotPicoClaw        = errors.New("this ZIP is not a PicoClaw backup")
	errBackupTooLarge           = errors.New("backup archive is too large")
	errBackupZipSlip            = errors.New("backup archive contains an unsafe path")
	errBackupUnsupportedVersion = errors.New("backup was created by an unsupported version")
)

func (h *Handler) readBackupUpload(w http.ResponseWriter, r *http.Request) (string, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupRequestBytes)

	reader, err := r.MultipartReader()
	if err != nil {
		writeBackupError(w, http.StatusBadRequest, "invalid_request", "Expected a multipart/form-data upload.")
		return "", "", err
	}

	dir := filepath.Join(os.TempDir(), "picoclaw_backup")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeBackupError(w, http.StatusInternalServerError, "temp_failed", "Failed to prepare the upload directory.")
		return "", "", err
	}

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeBackupError(w, http.StatusBadRequest, "invalid_request", "Failed to read the upload.")
			return "", "", err
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}

		filename := sanitizeBackupFilename(part.FileName())
		tmp, err := os.CreateTemp(dir, "upload-*.zip")
		if err != nil {
			_ = part.Close()
			writeBackupError(w, http.StatusInternalServerError, "temp_failed", "Failed to prepare the upload.")
			return "", "", err
		}
		written, copyErr := io.Copy(tmp, part)
		_ = part.Close()
		if closeErr := tmp.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			_ = os.Remove(tmp.Name())
			var maxErr *http.MaxBytesError
			if errors.As(copyErr, &maxErr) || written >= maxBackupRequestBytes {
				writeBackupError(w, http.StatusRequestEntityTooLarge, "too_large",
					"The backup file is too large.")
			} else {
				writeBackupError(w, http.StatusBadRequest, "read_failed", "Failed to read the backup file.")
			}
			return "", "", copyErr
		}
		if written == 0 {
			_ = os.Remove(tmp.Name())
			writeBackupError(w, http.StatusBadRequest, "empty_file", "The backup file is empty.")
			return "", "", errors.New("empty upload")
		}
		return tmp.Name(), filename, nil
	}

	writeBackupError(w, http.StatusBadRequest, "missing_file", "No backup file was uploaded.")
	return "", "", errors.New("missing file")
}

func (h *Handler) inspectBackup(uploadPath, filename string, cfg *config.Config) (backupInspection, error) {
	archive, err := zip.OpenReader(uploadPath)
	if err != nil {
		return backupInspection{}, fmt.Errorf("%w: %v", errBackupNotPicoClaw, err)
	}
	defer archive.Close()

	if len(archive.File) > maxBackupEntries {
		return backupInspection{}, errBackupTooLarge
	}

	manifest, err := readBackupManifest(archive.File)
	if err != nil {
		return backupInspection{}, err
	}
	if manifest.Version > backupFormatVersion {
		return backupInspection{}, errBackupUnsupportedVersion
	}

	inspection := backupInspection{
		Filename: filename,
		Manifest: manifest,
	}
	var totalBytes uint64
	workspace := cfg.WorkspacePath()
	home := config.GetHome()

	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || entry.Name == backupManifestName {
			continue
		}
		if entry.Flags&0x1 != 0 {
			return backupInspection{}, fmt.Errorf("%w: encrypted entries are not supported", errBackupNotPicoClaw)
		}
		if entry.UncompressedSize64 > maxBackupEntryBytes {
			return backupInspection{}, errBackupTooLarge
		}
		totalBytes += entry.UncompressedSize64
		if totalBytes > maxBackupTotalBytes {
			return backupInspection{}, errBackupTooLarge
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			inspection.Ignored++
			continue
		}

		cleanName, ok := safeBackupArchivePath(entry.Name)
		if !ok {
			return backupInspection{}, errBackupZipSlip
		}
		if isExcludedBackupEntry(cleanName) {
			inspection.Ignored++
			inspection.Warnings = appendUniqueString(inspection.Warnings,
				fmt.Sprintf("Ignored %s (secrets are never restored from a backup).", cleanName))
			continue
		}
		dest, ok := backupDestination(h.configPath, workspace, home, cleanName)
		if !ok {
			inspection.Ignored++
			continue
		}

		inspection.Files++
		inspection.Bytes += int64(entry.UncompressedSize64)
		if info, statErr := os.Stat(dest); statErr == nil && info.Mode().IsRegular() {
			inspection.Overwrite++
			if len(inspection.OverwriteSample) < backupSampleLimit {
				inspection.OverwriteSample = append(inspection.OverwriteSample, cleanName)
			}
		} else {
			inspection.New++
		}
	}

	return inspection, nil
}

func readBackupManifest(entries []*zip.File) (backupManifest, error) {
	for _, entry := range entries {
		if entry.Name != backupManifestName {
			continue
		}
		if entry.UncompressedSize64 > maxBackupManifestBytes {
			return backupManifest{}, errBackupTooLarge
		}
		reader, err := entry.Open()
		if err != nil {
			return backupManifest{}, errBackupNotPicoClaw
		}
		defer reader.Close()

		data, err := io.ReadAll(io.LimitReader(reader, maxBackupManifestBytes+1))
		if err != nil || len(data) > maxBackupManifestBytes {
			return backupManifest{}, errBackupNotPicoClaw
		}
		var manifest backupManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return backupManifest{}, errBackupNotPicoClaw
		}
		if manifest.Format != backupFormat {
			return backupManifest{}, errBackupNotPicoClaw
		}
		return manifest, nil
	}
	return backupManifest{}, errBackupNotPicoClaw
}

func (h *Handler) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID        string `json:"id"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeBackupError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}
	if !body.Overwrite {
		writeBackupError(w, http.StatusBadRequest, "confirmation_required",
			"Confirm that existing files may be overwritten before restoring.")
		return
	}

	h.backupMu.Lock()
	upload := h.backupUploads[body.ID]
	if upload != nil && time.Now().After(upload.expires) {
		_ = os.Remove(upload.path)
		delete(h.backupUploads, body.ID)
		upload = nil
	}
	h.backupMu.Unlock()
	if upload == nil {
		writeBackupError(w, http.StatusNotFound, "inspection_expired",
			"This backup upload expired. Select the file again.")
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeBackupError(w, http.StatusInternalServerError, "config_unreadable", "Failed to read the configuration.")
		return
	}

	result, err := h.restoreBackup(upload.path, cfg, h.configPath)
	if err != nil {
		status := http.StatusBadRequest
		code := "restore_failed"
		switch {
		case errors.Is(err, errBackupZipSlip):
			code = "unsafe_archive"
		case errors.Is(err, errBackupNotPicoClaw):
			code = "not_a_picoclaw_backup"
		case errors.Is(err, errBackupTooLarge):
			status = http.StatusRequestEntityTooLarge
			code = "too_large"
		case errors.Is(err, errBackupInvalidConfig):
			status = http.StatusUnprocessableEntity
			code = "invalid_config"
		}
		writeBackupError(w, status, code, err.Error())
		return
	}

	h.backupMu.Lock()
	_ = os.Remove(upload.path)
	delete(h.backupUploads, body.ID)
	h.backupMu.Unlock()

	writeBackupJSON(w, http.StatusOK, result)
}

var errBackupInvalidConfig = errors.New("backup contains an invalid config.json")

func (h *Handler) restoreBackup(
	uploadPath string,
	cfg *config.Config,
	configPath string,
) (backupRestoreResult, error) {
	archive, err := zip.OpenReader(uploadPath)
	if err != nil {
		return backupRestoreResult{}, errBackupNotPicoClaw
	}
	defer archive.Close()

	if len(archive.File) > maxBackupEntries {
		return backupRestoreResult{}, errBackupTooLarge
	}
	manifest, err := readBackupManifest(archive.File)
	if err != nil {
		return backupRestoreResult{}, err
	}
	if manifest.Version > backupFormatVersion {
		return backupRestoreResult{}, errBackupUnsupportedVersion
	}

	// Validate the archived config before touching the live one.
	if entry := findBackupEntry(archive.File, "config/config.json"); entry != nil {
		if err := validateArchivedConfig(entry); err != nil {
			return backupRestoreResult{}, err
		}
	}

	workspace := cfg.WorkspacePath()
	home := config.GetHome()
	result := backupRestoreResult{}
	var totalBytes uint64

	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || entry.Name == backupManifestName {
			continue
		}
		if entry.UncompressedSize64 > maxBackupEntryBytes {
			return backupRestoreResult{}, errBackupTooLarge
		}
		totalBytes += entry.UncompressedSize64
		if totalBytes > maxBackupTotalBytes {
			return backupRestoreResult{}, errBackupTooLarge
		}

		cleanName, ok := safeBackupArchivePath(entry.Name)
		if !ok {
			return backupRestoreResult{}, errBackupZipSlip
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 || isExcludedBackupEntry(cleanName) {
			result.Ignored++
			continue
		}
		dest, ok := backupDestination(configPath, workspace, home, cleanName)
		if !ok {
			result.Ignored++
			continue
		}

		if cleanName == "config/config.json" {
			if err := backupExistingConfig(configPath); err != nil {
				return backupRestoreResult{}, err
			}
			result.RestartRequired = true
		}

		data, err := readZipEntry(entry, maxBackupEntryBytes)
		if err != nil {
			return backupRestoreResult{}, err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return backupRestoreResult{}, err
		}
		if err := fileutil.WriteFileAtomic(dest, data, 0o644); err != nil {
			return backupRestoreResult{}, err
		}
		result.Restored++
	}

	if result.RestartRequired {
		if _, err := config.LoadConfig(configPath); err != nil {
			return backupRestoreResult{}, fmt.Errorf("%w: %v", errBackupInvalidConfig, err)
		}
	}

	return result, nil
}

func validateArchivedConfig(entry *zip.File) error {
	data, err := readZipEntry(entry, maxBackupManifestBytes*10)
	if err != nil {
		return err
	}

	dirBase := filepath.Join(os.TempDir(), "picoclaw_backup")
	if err := os.MkdirAll(dirBase, 0o700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(dirBase, "config-check-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	candidate := filepath.Join(dir, "config.json")
	if err := os.WriteFile(candidate, data, 0o600); err != nil {
		return err
	}
	if _, err := config.LoadConfig(candidate); err != nil {
		return fmt.Errorf("%w: %v", errBackupInvalidConfig, err)
	}
	return nil
}

func backupExistingConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	backupPath := fmt.Sprintf("%s.bak-restore-%s", configPath, time.Now().Format("20060102-150405"))
	return fileutil.WriteFileAtomic(backupPath, data, 0o600)
}

func findBackupEntry(entries []*zip.File, name string) *zip.File {
	for _, entry := range entries {
		if entry.Name == name {
			return entry
		}
	}
	return nil
}

func readZipEntry(entry *zip.File, limit int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errBackupTooLarge
	}
	return data, nil
}

func safeBackupArchivePath(name string) (string, bool) {
	normalized := strings.ReplaceAll(name, "\\", "/")
	if normalized == "" || strings.HasPrefix(normalized, "/") || strings.Contains(normalized, ":") {
		return "", false
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

func isExcludedBackupEntry(name string) bool {
	base := path.Base(name)
	switch {
	case base == ".security.yml":
		return true
	case strings.HasPrefix(base, ".security.yml.") && strings.HasSuffix(base, ".bak"):
		return true
	case base == "launcher-auth.db":
		return true
	case base == ".picoclaw.pid":
		return true
	case strings.HasPrefix(name, "logs/"):
		return true
	case strings.HasPrefix(name, "backups/"):
		return true
	}
	return false
}

func backupDestination(configPath, workspace, home, name string) (string, bool) {
	switch {
	case name == "config/config.json":
		return configPath, true
	case strings.HasPrefix(name, "workspace/"):
		if workspace == "" {
			return "", false
		}
		rel := strings.TrimPrefix(name, "workspace/")
		dest, err := joinWithin(workspace, rel)
		return dest, err == nil
	case strings.HasPrefix(name, "global-skills/"):
		if home == "" {
			return "", false
		}
		rel := strings.TrimPrefix(name, "global-skills/")
		dest, err := joinWithin(filepath.Join(home, "skills"), rel)
		return dest, err == nil
	}
	return "", false
}

func joinWithin(base, rel string) (string, error) {
	dest := filepath.Join(base, filepath.FromSlash(rel))
	check, err := filepath.Rel(base, dest)
	if err != nil || check == ".." || strings.HasPrefix(check, ".."+string(os.PathSeparator)) || filepath.IsAbs(check) {
		return "", errors.New("path escapes base directory")
	}
	return dest, nil
}

func sanitizeBackupFilename(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == ':' {
			return -1
		}
		return r
	}, name)
	if name == "" {
		return "backup.zip"
	}
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func (h *Handler) cleanupBackupUploadsLocked(now time.Time) {
	for token, upload := range h.backupUploads {
		if now.After(upload.expires) {
			_ = os.Remove(upload.path)
			delete(h.backupUploads, token)
		}
	}
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func writeBackupJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeBackupError(w http.ResponseWriter, status int, code, message string) {
	writeBackupJSON(w, status, map[string]any{"code": code, "error": message})
}
