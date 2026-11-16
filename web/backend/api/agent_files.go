package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/fileutil"
	"github.com/sipeed/picoclaw/pkg/routing"
)

// Agent memory/definition files exposed to the WebUI.
//
// Clients only send stable IDs. The server maps IDs to paths inside the
// workspace (or a read-only skill root); no client supplied path is ever used,
// which removes the path traversal surface entirely.

const (
	maxAgentFileSize       = 2 << 20
	agentFileBackupLimit   = 20
	agentFileDailyNoteDays = 90
)

const (
	agentFileGroupIdentity   = "identity"
	agentFileGroupMemory     = "memory"
	agentFileGroupAutomation = "automation"
	agentFileGroupSkills     = "skills"
)

var (
	errAgentFileInvalidID   = errors.New("invalid agent file id")
	errAgentFileNotFound    = errors.New("agent file not found")
	errAgentFileReadOnly    = errors.New("agent file is read-only")
	errAgentFileTooLarge    = errors.New("agent file exceeds the size limit")
	errAgentFileNotText     = errors.New("agent file is not valid UTF-8 text")
	errAgentFileInvalidPath = errors.New("agent file path is not allowed")

	agentFileSkillDirPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	agentFileDatePattern     = regexp.MustCompile(`^[0-9]{8}$`)
	agentFileRevisionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type agentFileStaticDef struct {
	relPath   string
	group     string
	label     string
	legacy    bool
	creatable bool
}

// agentFileStaticDefs maps stable IDs to workspace files. Paths are relative to
// the workspace root and are never derived from request input.
var agentFileStaticDefs = map[string]agentFileStaticDef{
	"agent":     {relPath: "AGENT.md", group: agentFileGroupIdentity, label: "Agent", creatable: true},
	"agents":    {relPath: "AGENTS.md", group: agentFileGroupIdentity, label: "Agent (legacy)", legacy: true},
	"soul":      {relPath: "SOUL.md", group: agentFileGroupIdentity, label: "Soul", creatable: true},
	"user":      {relPath: "USER.md", group: agentFileGroupIdentity, label: "User", creatable: true},
	"identity":  {relPath: "IDENTITY.md", group: agentFileGroupIdentity, label: "Identity (legacy)", legacy: true},
	"memory":    {relPath: "memory/MEMORY.md", group: agentFileGroupMemory, label: "Long-term memory", creatable: true},
	"heartbeat": {relPath: "HEARTBEAT.md", group: agentFileGroupAutomation, label: "Heartbeat", creatable: true},
}

type agentFileTarget struct {
	id        string
	agentID   string
	group     string
	label     string
	relPath   string // slash separated, relative to root
	root      string // absolute directory the file is confined to
	source    string
	editable  bool
	legacy    bool
	creatable bool
}

type agentFileItem struct {
	ID         string `json:"id"`
	Group      string `json:"group"`
	Label      string `json:"label"`
	Path       string `json:"path"`
	Source     string `json:"source,omitempty"`
	Exists     bool   `json:"exists"`
	Editable   bool   `json:"editable"`
	Legacy     bool   `json:"legacy,omitempty"`
	Creatable  bool   `json:"creatable"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modified_at,omitempty"`
}

type agentFileDetail struct {
	agentFileItem
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

type agentFileGroupResponse struct {
	ID    string          `json:"id"`
	Label string          `json:"label"`
	Files []agentFileItem `json:"files"`
}

type agentFileListResponse struct {
	Groups []agentFileGroupResponse `json:"groups"`
}

type agentFileWriteRequest struct {
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

type agentFileRestoreRequest struct {
	Revision string `json:"revision"`
}

type agentFileWriteResponse struct {
	Status     string `json:"status"`
	ID         string `json:"id"`
	Revision   string `json:"revision"`
	Size       int64  `json:"size"`
	Created    bool   `json:"created"`
	ModifiedAt int64  `json:"modified_at"`
}

type agentFileHistoryEntry struct {
	Revision  string `json:"revision"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
}

type agentFileHistoryResponse struct {
	ID      string                  `json:"id"`
	History []agentFileHistoryEntry `json:"history"`
}

type agentFileHistoryContentResponse struct {
	Revision  string `json:"revision"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

func (h *Handler) registerAgentFileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agent/files", h.handleListAgentFiles)
	mux.HandleFunc("GET /api/agent/files/{id}", h.handleGetAgentFile)
	mux.HandleFunc("PUT /api/agent/files/{id}", h.handlePutAgentFile)
	mux.HandleFunc("GET /api/agent/files/{id}/history", h.handleAgentFileHistory)
	mux.HandleFunc("GET /api/agent/files/{id}/history/{revision}", h.handleAgentFileHistoryContent)
	mux.HandleFunc("POST /api/agent/files/{id}/restore", h.handleRestoreAgentFile)
}

func (h *Handler) handleListAgentFiles(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadConfigForAgentFiles(w)
	if err != nil {
		return
	}

	workspace, agentID, err := agentFileRequestScope(cfg, r)
	if err != nil {
		writeAgentFileJSON(w, http.StatusBadRequest, map[string]any{
			"code":  "invalid_agent",
			"error": err.Error(),
		})
		return
	}

	groups, err := buildAgentFileGroups(cfg, workspace, agentID)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}
	writeAgentFileJSON(w, http.StatusOK, agentFileListResponse{Groups: groups})
}

func (h *Handler) handleGetAgentFile(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadConfigForAgentFiles(w)
	if err != nil {
		return
	}

	workspace, agentID, err := agentFileRequestScope(cfg, r)
	if err != nil {
		writeAgentFileJSON(w, http.StatusBadRequest, map[string]any{
			"code":  "invalid_agent",
			"error": err.Error(),
		})
		return
	}

	target, err := resolveAgentFileTarget(cfg, r.PathValue("id"), workspace, agentID)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}

	detail, err := readAgentFileDetail(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && target.creatable {
			writeAgentFileJSON(w, http.StatusOK, agentFileDetail{
				agentFileItem: buildAgentFileItem(target, nil),
			})
			return
		}
		writeAgentFileError(w, err)
		return
	}
	writeAgentFileJSON(w, http.StatusOK, detail)
}

func (h *Handler) handlePutAgentFile(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadConfigForAgentFiles(w)
	if err != nil {
		return
	}

	workspace, agentID, err := agentFileRequestScope(cfg, r)
	if err != nil {
		writeAgentFileJSON(w, http.StatusBadRequest, map[string]any{
			"code":  "invalid_agent",
			"error": err.Error(),
		})
		return
	}

	target, err := resolveAgentFileTarget(cfg, r.PathValue("id"), workspace, agentID)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}
	if !target.editable {
		writeAgentFileError(w, errAgentFileReadOnly)
		return
	}

	var req agentFileWriteRequest
	if err := decodeAgentFileJSON(r, &req); err != nil {
		writeAgentFileError(w, err)
		return
	}
	content := []byte(req.Content)
	if err := validateAgentFileContent(content); err != nil {
		writeAgentFileError(w, err)
		return
	}

	h.agentFilesMu.Lock()
	defer h.agentFilesMu.Unlock()

	info, statErr := statAgentFileTarget(target)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		writeAgentFileError(w, statErr)
		return
	}

	perm := os.FileMode(0o644)
	var current []byte
	if exists {
		if req.Revision == "" {
			writeAgentFileJSON(w, http.StatusBadRequest, map[string]any{
				"code":  "revision_required",
				"error": "revision is required to update an existing file",
			})
			return
		}
		current, _, err = readAgentFileTarget(target)
		if err != nil {
			writeAgentFileError(w, err)
			return
		}
		currentRevision := agentFileRevision(current)
		if currentRevision != req.Revision {
			writeAgentFileJSON(w, http.StatusConflict, map[string]any{
				"code":     "revision_conflict",
				"error":    "the file was changed by someone else",
				"revision": currentRevision,
			})
			return
		}
		if perm = info.Mode().Perm(); perm == 0 {
			perm = 0o644
		}
		if err := backupAgentFile(target, current); err != nil {
			writeAgentFileError(w, err)
			return
		}
	} else if !target.creatable {
		writeAgentFileError(w, errAgentFileNotFound)
		return
	}

	if err := writeAgentFileContent(target, content, perm); err != nil {
		writeAgentFileError(w, err)
		return
	}

	newInfo, err := statAgentFileTarget(target)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}
	writeAgentFileJSON(w, http.StatusOK, agentFileWriteResponse{
		Status:     "saved",
		ID:         target.id,
		Revision:   agentFileRevision(content),
		Size:       int64(len(content)),
		Created:    !exists,
		ModifiedAt: newInfo.ModTime().Unix(),
	})
}

func (h *Handler) handleAgentFileHistory(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadConfigForAgentFiles(w)
	if err != nil {
		return
	}

	workspace, agentID, err := agentFileRequestScope(cfg, r)
	if err != nil {
		writeAgentFileJSON(w, http.StatusBadRequest, map[string]any{
			"code":  "invalid_agent",
			"error": err.Error(),
		})
		return
	}

	target, err := resolveAgentFileTarget(cfg, r.PathValue("id"), workspace, agentID)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}

	history, err := listAgentFileBackups(target)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}
	writeAgentFileJSON(w, http.StatusOK, agentFileHistoryResponse{ID: target.id, History: history})
}

func (h *Handler) handleAgentFileHistoryContent(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadConfigForAgentFiles(w)
	if err != nil {
		return
	}

	workspace, agentID, err := agentFileRequestScope(cfg, r)
	if err != nil {
		writeAgentFileJSON(w, http.StatusBadRequest, map[string]any{
			"code":  "invalid_agent",
			"error": err.Error(),
		})
		return
	}

	target, err := resolveAgentFileTarget(cfg, r.PathValue("id"), workspace, agentID)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}

	revision := r.PathValue("revision")
	content, createdAt, err := readAgentFileBackup(target, revision)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}
	writeAgentFileJSON(w, http.StatusOK, agentFileHistoryContentResponse{
		Revision:  revision,
		Content:   string(content),
		CreatedAt: createdAt,
	})
}

func (h *Handler) handleRestoreAgentFile(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.loadConfigForAgentFiles(w)
	if err != nil {
		return
	}

	workspace, agentID, err := agentFileRequestScope(cfg, r)
	if err != nil {
		writeAgentFileJSON(w, http.StatusBadRequest, map[string]any{
			"code":  "invalid_agent",
			"error": err.Error(),
		})
		return
	}

	target, err := resolveAgentFileTarget(cfg, r.PathValue("id"), workspace, agentID)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}
	if !target.editable {
		writeAgentFileError(w, errAgentFileReadOnly)
		return
	}

	var req agentFileRestoreRequest
	if err := decodeAgentFileJSON(r, &req); err != nil {
		writeAgentFileError(w, err)
		return
	}
	content, _, err := readAgentFileBackup(target, req.Revision)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}
	if err := validateAgentFileContent(content); err != nil {
		writeAgentFileError(w, err)
		return
	}

	h.agentFilesMu.Lock()
	defer h.agentFilesMu.Unlock()

	perm := os.FileMode(0o644)
	info, statErr := statAgentFileTarget(target)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		writeAgentFileError(w, statErr)
		return
	}
	if exists {
		current, _, err := readAgentFileTarget(target)
		if err != nil {
			writeAgentFileError(w, err)
			return
		}
		if agentFileRevision(current) != req.Revision {
			if err := backupAgentFile(target, current); err != nil {
				writeAgentFileError(w, err)
				return
			}
		}
		if perm = info.Mode().Perm(); perm == 0 {
			perm = 0o644
		}
	} else if !target.creatable {
		writeAgentFileError(w, errAgentFileNotFound)
		return
	}

	if err := writeAgentFileContent(target, content, perm); err != nil {
		writeAgentFileError(w, err)
		return
	}

	newInfo, err := statAgentFileTarget(target)
	if err != nil {
		writeAgentFileError(w, err)
		return
	}
	writeAgentFileJSON(w, http.StatusOK, agentFileWriteResponse{
		Status:     "restored",
		ID:         target.id,
		Revision:   agentFileRevision(content),
		Size:       int64(len(content)),
		Created:    !exists,
		ModifiedAt: newInfo.ModTime().Unix(),
	})
}

func (h *Handler) loadConfigForAgentFiles(w http.ResponseWriter) (*config.Config, error) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentFileJSON(w, http.StatusInternalServerError, map[string]any{
			"code":  "config_error",
			"error": "failed to load configuration",
		})
		return nil, err
	}
	return cfg, nil
}

// agentFileRequestScope resolves the optional ?agent= selector to a workspace.
// An empty selector keeps the default workspace for backward compatibility.
func agentFileRequestScope(cfg *config.Config, r *http.Request) (string, string, error) {
	requested := strings.TrimSpace(r.URL.Query().Get("agent"))
	if requested == "" {
		return cfg.WorkspacePath(), "", nil
	}
	workspace, err := resolveAgentWorkspaceForID(cfg, requested)
	if err != nil {
		return "", "", err
	}
	return workspace, routing.NormalizeAgentID(requested), nil
}

// resolveAgentFileTarget maps a stable ID to a server-side path. Unknown or
// malformed IDs are rejected before any filesystem access.
func resolveAgentFileTarget(cfg *config.Config, id, workspace, agentID string) (agentFileTarget, error) {
	if id == "" || len(id) > 160 || strings.Contains(id, "..") {
		return agentFileTarget{}, errAgentFileInvalidID
	}
	if strings.ContainsAny(id, `/\`) {
		return agentFileTarget{}, errAgentFileInvalidID
	}

	if workspace == "" {
		workspace = cfg.WorkspacePath()
	}

	if datePart, ok := strings.CutPrefix(id, "daily:"); ok {
		if !agentFileDatePattern.MatchString(datePart) {
			return agentFileTarget{}, errAgentFileInvalidID
		}
		day, err := time.Parse("20060102", datePart)
		if err != nil {
			return agentFileTarget{}, errAgentFileInvalidID
		}
		return agentFileTarget{
			id:        id,
			agentID:   agentID,
			group:     agentFileGroupMemory,
			label:     day.Format("2006-01-02"),
			relPath:   path.Join("memory", datePart[:6], datePart+".md"),
			root:      workspace,
			editable:  true,
			creatable: true,
		}, nil
	}

	if rest, ok := strings.CutPrefix(id, "skill:"); ok {
		parts := strings.Split(rest, ":")
		if len(parts) != 2 {
			return agentFileTarget{}, errAgentFileInvalidID
		}
		source, dir := parts[0], parts[1]
		if !agentFileSkillDirPattern.MatchString(dir) || dir == "." || dir == ".." {
			return agentFileTarget{}, errAgentFileInvalidID
		}

		var root string
		editable := false
		switch source {
		case "workspace":
			root = filepath.Join(workspace, "skills")
			editable = true
		case "shared":
			root = filepath.Join(cfg.WorkspacePath(), "skills")
		case "global":
			root = filepath.Join(globalConfigDir(), "skills")
		case "builtin":
			root = builtinSkillsDir()
		default:
			return agentFileTarget{}, errAgentFileInvalidID
		}
		if root == "" {
			return agentFileTarget{}, errAgentFileNotFound
		}
		return agentFileTarget{
			id:       id,
			agentID:  agentID,
			group:    agentFileGroupSkills,
			label:    dir,
			relPath:  path.Join(dir, "SKILL.md"),
			root:     root,
			source:   source,
			editable: editable,
		}, nil
	}

	def, ok := agentFileStaticDefs[id]
	if !ok {
		return agentFileTarget{}, errAgentFileInvalidID
	}
	return agentFileTarget{
		id:        id,
		agentID:   agentID,
		group:     def.group,
		label:     def.label,
		relPath:   def.relPath,
		root:      workspace,
		editable:  true,
		legacy:    def.legacy,
		creatable: def.creatable,
	}, nil
}

func buildAgentFileGroups(cfg *config.Config, workspace, agentID string) ([]agentFileGroupResponse, error) {
	byGroup := map[string][]agentFileItem{
		agentFileGroupIdentity:   {},
		agentFileGroupMemory:     {},
		agentFileGroupAutomation: {},
		agentFileGroupSkills:     {},
	}

	// Identity files keep a fixed order; legacy files only appear when present.
	for _, id := range []string{"agent", "agents", "soul", "user", "identity"} {
		target, err := resolveAgentFileTarget(cfg, id, workspace, agentID)
		if err != nil {
			return nil, err
		}
		info, statErr := statAgentFileTarget(target)
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			continue
		}
		if target.legacy && statErr != nil {
			continue
		}
		byGroup[target.group] = append(byGroup[target.group], buildAgentFileItem(target, info))
	}

	memoryTarget, err := resolveAgentFileTarget(cfg, "memory", workspace, agentID)
	if err != nil {
		return nil, err
	}
	if info, statErr := statAgentFileTarget(memoryTarget); statErr == nil {
		byGroup[agentFileGroupMemory] = append(byGroup[agentFileGroupMemory], buildAgentFileItem(memoryTarget, info))
	} else {
		byGroup[agentFileGroupMemory] = append(byGroup[agentFileGroupMemory], buildAgentFileItem(memoryTarget, nil))
	}

	// Recent daily notes, newest first. Today's note is listed even when it does
	// not exist yet so the UI can offer "create".
	now := time.Now()
	for offset := 0; offset < agentFileDailyNoteDays; offset++ {
		day := now.AddDate(0, 0, -offset)
		id := "daily:" + day.Format("20060102")
		target, err := resolveAgentFileTarget(cfg, id, workspace, agentID)
		if err != nil {
			return nil, err
		}
		info, statErr := statAgentFileTarget(target)
		if statErr != nil {
			if offset != 0 {
				continue
			}
			byGroup[agentFileGroupMemory] = append(byGroup[agentFileGroupMemory], buildAgentFileItem(target, nil))
			continue
		}
		byGroup[agentFileGroupMemory] = append(byGroup[agentFileGroupMemory], buildAgentFileItem(target, info))
	}

	heartbeatTarget, err := resolveAgentFileTarget(cfg, "heartbeat", workspace, agentID)
	if err != nil {
		return nil, err
	}
	if info, statErr := statAgentFileTarget(heartbeatTarget); statErr == nil {
		byGroup[agentFileGroupAutomation] = append(
			byGroup[agentFileGroupAutomation],
			buildAgentFileItem(heartbeatTarget, info),
		)
	} else {
		byGroup[agentFileGroupAutomation] = append(
			byGroup[agentFileGroupAutomation],
			buildAgentFileItem(heartbeatTarget, nil),
		)
	}

	loader := newAgentSkillsLoader(cfg, workspace)
	for _, skill := range loader.ListSkills() {
		dir := filepath.Base(filepath.Dir(skill.Path))
		id := "skill:" + skill.Source + ":" + dir
		target, err := resolveAgentFileTarget(cfg, id, workspace, agentID)
		if err != nil {
			continue
		}
		info, statErr := statAgentFileTarget(target)
		if statErr != nil {
			continue
		}
		item := buildAgentFileItem(target, info)
		item.Path = path.Join("skills", dir, "SKILL.md")
		byGroup[agentFileGroupSkills] = append(byGroup[agentFileGroupSkills], item)
	}

	groupDefs := []struct{ id, label string }{
		{agentFileGroupIdentity, "Identity"},
		{agentFileGroupMemory, "Memory"},
		{agentFileGroupAutomation, "Automation"},
		{agentFileGroupSkills, "Skills"},
	}
	groups := make([]agentFileGroupResponse, 0, len(groupDefs))
	for _, def := range groupDefs {
		files := byGroup[def.id]
		if files == nil {
			files = []agentFileItem{}
		}
		groups = append(groups, agentFileGroupResponse{ID: def.id, Label: def.label, Files: files})
	}
	return groups, nil
}

func buildAgentFileItem(target agentFileTarget, info fs.FileInfo) agentFileItem {
	item := agentFileItem{
		ID:        target.id,
		Group:     target.group,
		Label:     target.label,
		Path:      path.Join(target.relPath),
		Source:    target.source,
		Editable:  target.editable,
		Legacy:    target.legacy,
		Creatable: target.creatable,
	}
	if target.group == agentFileGroupSkills {
		item.Path = path.Join("skills", target.relPath)
	}
	if info != nil {
		item.Exists = true
		item.Size = info.Size()
		item.ModifiedAt = info.ModTime().Unix()
	}
	return item
}

func statAgentFileTarget(target agentFileTarget) (fs.FileInfo, error) {
	root, err := os.OpenRoot(target.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Stat(filepath.FromSlash(target.relPath))
}

func readAgentFileTarget(target agentFileTarget) ([]byte, fs.FileInfo, error) {
	root, err := os.OpenRoot(target.root)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()

	file, err := root.Open(filepath.FromSlash(target.relPath))
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errAgentFileNotFound
	}
	if info.Size() > maxAgentFileSize {
		return nil, nil, errAgentFileTooLarge
	}

	data, err := io.ReadAll(io.LimitReader(file, maxAgentFileSize+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > maxAgentFileSize {
		return nil, nil, errAgentFileTooLarge
	}
	return data, info, nil
}

func readAgentFileDetail(target agentFileTarget) (agentFileDetail, error) {
	data, info, err := readAgentFileTarget(target)
	if err != nil {
		return agentFileDetail{}, err
	}
	if err := validateAgentFileContent(data); err != nil {
		return agentFileDetail{}, err
	}
	return agentFileDetail{
		agentFileItem: buildAgentFileItem(target, info),
		Content:       string(data),
		Revision:      agentFileRevision(data),
	}, nil
}

func validateAgentFileContent(data []byte) error {
	if len(data) > maxAgentFileSize {
		return errAgentFileTooLarge
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return errAgentFileNotText
	}
	return nil
}

func agentFileRevision(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// writeAgentFileContent writes atomically after proving that no path component
// is a symbolic link, so a link inside the workspace cannot redirect the write.
func writeAgentFileContent(target agentFileTarget, content []byte, perm os.FileMode) error {
	if err := ensureNoSymlinkComponents(target.root, target.relPath); err != nil {
		return err
	}
	absPath, err := agentFileAbsPath(target.root, target.relPath)
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(absPath, content, perm)
}

func ensureNoSymlinkComponents(rootDir, relPath string) error {
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return err
	}
	defer root.Close()

	current := ""
	for _, part := range strings.Split(path.Clean(relPath), "/") {
		if part == "" || part == "." {
			continue
		}
		if current == "" {
			current = part
		} else {
			current = path.Join(current, part)
		}
		info, err := root.Lstat(filepath.FromSlash(current))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symbolic links are not allowed", errAgentFileInvalidPath)
		}
	}
	return nil
}

func agentFileAbsPath(rootDir, relPath string) (string, error) {
	cleanRoot := filepath.Clean(rootDir)
	absPath := filepath.Join(cleanRoot, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(cleanRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", errAgentFileInvalidID
	}
	return absPath, nil
}

func agentFileBackupDir(target agentFileTarget) string {
	base := filepath.Join(config.GetHome(), "backups", "agent-files")
	if target.agentID != "" && target.agentID != routing.DefaultAgentID {
		base = filepath.Join(base, safeAgentFileBackupName(target.agentID))
	}
	return filepath.Join(base, safeAgentFileBackupName(target.id))
}

func safeAgentFileBackupName(value string) string {
	return strings.NewReplacer(":", "-", "/", "-", "\\", "-").Replace(value)
}

// backupAgentFile stores the current content of a file before it is overwritten.
// Backups are capped per file; the oldest entries are pruned first.
func backupAgentFile(target agentFileTarget, content []byte) error {
	revision := agentFileRevision(content)
	dir := agentFileBackupDir(target)

	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	suffix := "-" + revision + ".md"
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), suffix) {
			return nil // identical content already backed up
		}
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := fmt.Sprintf("%020d-%s.md", time.Now().UnixNano(), revision)
	if err := fileutil.WriteFileAtomic(filepath.Join(dir, name), content, 0o600); err != nil {
		return err
	}

	entries, err = os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > agentFileBackupLimit {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
	return nil
}

type agentFileBackupFile struct {
	name      string
	revision  string
	createdAt time.Time
	size      int64
}

func listAgentFileBackupFiles(target agentFileTarget) ([]agentFileBackupFile, error) {
	dir := agentFileBackupDir(target)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []agentFileBackupFile{}, nil
		}
		return nil, err
	}

	backups := make([]agentFileBackupFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		tsPart, revPart, ok := strings.Cut(name, "-")
		if !ok || !agentFileRevisionPattern.MatchString(revPart) {
			continue
		}
		nanos, err := strconv.ParseInt(strings.TrimLeft(tsPart, "0"), 10, 64)
		if err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, agentFileBackupFile{
			name:      entry.Name(),
			revision:  revPart,
			createdAt: time.Unix(0, nanos),
			size:      info.Size(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].createdAt.After(backups[j].createdAt)
	})
	return backups, nil
}

func listAgentFileBackups(target agentFileTarget) ([]agentFileHistoryEntry, error) {
	files, err := listAgentFileBackupFiles(target)
	if err != nil {
		return nil, err
	}
	history := make([]agentFileHistoryEntry, 0, len(files))
	for _, file := range files {
		history = append(history, agentFileHistoryEntry{
			Revision:  file.revision,
			Size:      file.size,
			CreatedAt: file.createdAt.Unix(),
		})
	}
	return history, nil
}

func readAgentFileBackup(target agentFileTarget, revision string) ([]byte, int64, error) {
	if !agentFileRevisionPattern.MatchString(revision) {
		return nil, 0, errAgentFileInvalidID
	}
	files, err := listAgentFileBackupFiles(target)
	if err != nil {
		return nil, 0, err
	}
	for _, file := range files {
		if file.revision != revision {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentFileBackupDir(target), file.name))
		if err != nil {
			return nil, 0, err
		}
		if err := validateAgentFileContent(data); err != nil {
			return nil, 0, err
		}
		return data, file.createdAt.Unix(), nil
	}
	return nil, 0, errAgentFileNotFound
}

func decodeAgentFileJSON(r *http.Request, dst any) error {
	// Allow for JSON escaping overhead on top of the raw content limit.
	decoder := json.NewDecoder(io.LimitReader(r.Body, int64(8*maxAgentFileSize)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return &agentFileBadRequestError{message: "invalid request body"}
	}
	if decoder.More() {
		return &agentFileBadRequestError{message: "invalid request body"}
	}
	return nil
}

type agentFileBadRequestError struct {
	message string
}

func (e *agentFileBadRequestError) Error() string { return e.message }

func writeAgentFileJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAgentFileError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	message := "agent file operation failed"

	var badRequest *agentFileBadRequestError
	switch {
	case errors.As(err, &badRequest):
		status = http.StatusBadRequest
		code = "invalid_request"
		message = err.Error()
	case errors.Is(err, errAgentFileInvalidID), errors.Is(err, errAgentFileInvalidPath):
		status = http.StatusBadRequest
		code = "invalid_id"
		message = "invalid agent file id"
	case errors.Is(err, errAgentFileNotFound), errors.Is(err, fs.ErrNotExist):
		status = http.StatusNotFound
		code = "not_found"
		message = "agent file not found"
	case errors.Is(err, errAgentFileReadOnly):
		status = http.StatusForbidden
		code = "read_only"
		message = "this file is read-only"
	case errors.Is(err, errAgentFileTooLarge):
		status = http.StatusRequestEntityTooLarge
		code = "too_large"
		message = "agent file exceeds the 2 MB limit"
	case errors.Is(err, errAgentFileNotText):
		status = http.StatusUnsupportedMediaType
		code = "not_text"
		message = "agent file must be UTF-8 text"
	}

	writeAgentFileJSON(w, status, map[string]any{"code": code, "error": message})
}
