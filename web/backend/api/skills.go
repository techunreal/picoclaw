package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/fileutil"
	"github.com/sipeed/picoclaw/pkg/skills"
	"github.com/sipeed/picoclaw/pkg/utils"
)

const defaultInstallSkillRegistry = "github"

type skillSupportResponse struct {
	Skills []skillSupportItem `json:"skills"`
}

type skillSupportItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	Directory        string `json:"directory"`
	Source           string `json:"source"`
	Description      string `json:"description"`
	OriginKind       string `json:"origin_kind"`
	Editable         bool   `json:"editable"`
	NameMismatch     bool   `json:"name_mismatch,omitempty"`
	OwnerHandle      string `json:"owner_handle,omitempty"`
	RegistrySlug     string `json:"registry_slug,omitempty"`
	CanUpdate        bool   `json:"can_update,omitempty"`
	RegistryName     string `json:"registry_name,omitempty"`
	RegistryURL      string `json:"registry_url,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	InstalledAt      int64  `json:"installed_at,omitempty"`
}

type skillDetailResponse struct {
	skillSupportItem
	Content    string `json:"content"`
	RawContent string `json:"raw_content"`
	Revision   string `json:"revision"`
	Size       int64  `json:"size"`
}

type skillWriteRequest struct {
	Content  string `json:"content"`
	Revision string `json:"revision"`
	Force    bool   `json:"force,omitempty"`
}

type skillWriteResponse struct {
	Status   string `json:"status"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Size     int64  `json:"size"`
}

type skillSearchResultItem struct {
	Score         float64 `json:"score"`
	Slug          string  `json:"slug"`
	DisplayName   string  `json:"display_name"`
	Summary       string  `json:"summary"`
	Version       string  `json:"version"`
	RegistryName  string  `json:"registry_name"`
	URL           string  `json:"url,omitempty"`
	Installed     bool    `json:"installed"`
	InstalledName string  `json:"installed_name,omitempty"`
	InstalledID   string  `json:"installed_id,omitempty"`

	OwnerHandle      string             `json:"owner_handle,omitempty"`
	InstallKind      string             `json:"install_kind,omitempty"`
	InstallReference string             `json:"install_reference,omitempty"`
	CanonicalURL     string             `json:"canonical_url,omitempty"`
	Downloads        int64              `json:"downloads,omitempty"`
	InstallStatus    string             `json:"install_status"`
	InstallReason    string             `json:"install_reason,omitempty"`
	Security         *skillSecurityItem `json:"security,omitempty"`
}

type skillSecurityItem struct {
	Decision  string `json:"decision"`
	Status    string `json:"status"`
	RiskLevel string `json:"risk_level,omitempty"`
	Summary   string `json:"summary,omitempty"`
	AuditURL  string `json:"audit_url,omitempty"`
}

type skillSearchResponse struct {
	Results    []skillSearchResultItem `json:"results"`
	Limit      int                     `json:"limit"`
	Offset     int                     `json:"offset"`
	NextOffset int                     `json:"next_offset,omitempty"`
	NextCursor string                  `json:"next_cursor,omitempty"`
	HasMore    bool                    `json:"has_more"`
	Sort       string                  `json:"sort,omitempty"`
}

type installSkillRequest struct {
	Slug             string `json:"slug"`
	Registry         string `json:"registry"`
	Version          string `json:"version,omitempty"`
	Force            bool   `json:"force,omitempty"`
	OwnerHandle      string `json:"owner_handle,omitempty"`
	InstallKind      string `json:"install_kind,omitempty"`
	AcceptSuspicious bool   `json:"accept_suspicious,omitempty"`
}

type installSkillResponse struct {
	Status         string            `json:"status"`
	Slug           string            `json:"slug"`
	Registry       string            `json:"registry"`
	Version        string            `json:"version"`
	Summary        string            `json:"summary,omitempty"`
	IsSuspicious   bool              `json:"is_suspicious,omitempty"`
	InstalledSkill *skillSupportItem `json:"skill,omitempty"`
}

type installedSkillOriginMeta struct {
	Version          int    `json:"version"`
	OriginKind       string `json:"origin_kind,omitempty"`
	Registry         string `json:"registry,omitempty"`
	Slug             string `json:"slug,omitempty"`
	OwnerHandle      string `json:"owner_handle,omitempty"`
	RegistryURL      string `json:"registry_url,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	InstalledAt      int64  `json:"installed_at"`
}

var (
	skillNameSanitizer       = regexp.MustCompile(`[^a-z0-9-]+`)
	importedSkillFrontmatter = regexp.MustCompile(`(?s)^---(?:\r\n|\n|\r)(.*?)(?:\r\n|\n|\r)---(?:\r\n|\n|\r)*`)
	skillFrontmatterStripper = regexp.MustCompile(`(?s)^---(?:\r\n|\n|\r)(.*?)(?:\r\n|\n|\r)---(?:\r\n|\n|\r)*`)
	persistSkillOriginMeta   = writeSkillOriginMeta
	workspaceSkillWriteMu    sync.Mutex
	errImportedSkillExists   = errors.New("skill already exists")
)

const (
	maxImportedSkillSize    = 1 << 20
	maxRegistrySearchFanout = 1000
)

func (h *Handler) registerSkillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/skills", h.handleListSkills)
	mux.HandleFunc("GET /api/skills/{name}", h.handleGetSkill)
	mux.HandleFunc("GET /api/skills/search", h.handleSearchSkills)
	mux.HandleFunc("GET /api/skills/popular", h.handlePopularSkills)
	mux.HandleFunc("GET /api/skills/by-id/{id}", h.handleGetSkillByID)
	mux.HandleFunc("PUT /api/skills/by-id/{id}", h.handleSaveSkillByID)
	mux.HandleFunc("POST /api/skills/install", h.handleInstallSkill)
	mux.HandleFunc("POST /api/skills/import", h.handleImportSkill)
	mux.HandleFunc("DELETE /api/skills/{name}", h.handleDeleteSkill)
	mux.HandleFunc("DELETE /api/skills/by-id/{id}", h.handleDeleteSkillByID)
}

func (h *Handler) handleListSkills(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	items, err := buildSkillSupportItems(cfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to build skill list: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(skillSupportResponse{
		Skills: items,
	})
}

func (h *Handler) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	skillItems, err := buildSkillSupportItems(cfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to build skill list: %v", err), http.StatusInternalServerError)
		return
	}
	name := r.PathValue("name")
	for _, skillItem := range skillItems {
		if skillItem.Name != name {
			continue
		}

		detail, err := buildSkillDetailResponse(skillItem)
		if err != nil {
			http.Error(w, "Skill content not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
		return
	}

	http.Error(w, "Skill not found", http.StatusNotFound)
}

var skillIDPattern = regexp.MustCompile(`^(workspace|global|builtin):([^/\\]+)$`)

func parseSkillID(id string) (source, directory string, ok bool) {
	matches := skillIDPattern.FindStringSubmatch(id)
	if len(matches) != 3 {
		return "", "", false
	}
	directory = matches[2]
	if directory == "." || directory == ".." {
		return "", "", false
	}
	return matches[1], directory, true
}

func findSkillByID(cfg *config.Config, id string) (*skillSupportItem, error) {
	source, directory, ok := parseSkillID(id)
	if !ok {
		return nil, errors.New("invalid skill id")
	}
	items, err := buildSkillSupportItems(cfg)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Source == source && item.Directory == directory {
			itemCopy := item
			return &itemCopy, nil
		}
	}
	return nil, nil
}

func (h *Handler) handleGetSkillByID(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	item, err := findSkillByID(cfg, r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if item == nil {
		http.Error(w, "Skill not found", http.StatusNotFound)
		return
	}

	content, err := buildSkillDetailResponse(*item)
	if err != nil {
		http.Error(w, "Skill content not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(content)
}

const (
	maxSkillFileSize = 2 << 20
	skillBackupLimit = 20
)

func skillRevision(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func buildSkillDetailResponse(item skillSupportItem) (skillDetailResponse, error) {
	raw, err := os.ReadFile(item.Path)
	if err != nil {
		return skillDetailResponse{}, err
	}
	return skillDetailResponse{
		skillSupportItem: item,
		Content:          skillFrontmatterStripper.ReplaceAllString(string(raw), ""),
		RawContent:       string(raw),
		Revision:         skillRevision(raw),
		Size:             int64(len(raw)),
	}, nil
}

func skillBackupDir(id string) string {
	safeID := strings.NewReplacer(":", "-", "/", "-", "\\", "-").Replace(id)
	return filepath.Join(config.GetHome(), "backups", "skills", safeID)
}

func backupSkillContent(id string, content []byte) error {
	revision := skillRevision(content)
	dir := skillBackupDir(id)

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	suffix := "-" + revision + ".md"
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), suffix) {
			return nil
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
	for len(names) > skillBackupLimit {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
	return nil
}

func (h *Handler) handleSaveSkillByID(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	item, err := findSkillByID(cfg, r.PathValue("id"))
	if err != nil {
		writeSkillJSONError(w, http.StatusBadRequest, "invalid_skill_id", "Invalid skill id.", nil)
		return
	}
	if item == nil {
		writeSkillJSONError(w, http.StatusNotFound, "not_found", "Skill not found.", nil)
		return
	}
	if item.Source != "workspace" || !item.Editable {
		writeSkillJSONError(w, http.StatusForbidden, "read_only", "Only workspace skills can be edited.", nil)
		return
	}

	decoder := json.NewDecoder(io.LimitReader(r.Body, int64(8*maxSkillFileSize)))
	decoder.DisallowUnknownFields()
	var req skillWriteRequest
	if err := decoder.Decode(&req); err != nil {
		writeSkillJSONError(w, http.StatusBadRequest, "invalid_request", "Invalid request body.", nil)
		return
	}

	content := []byte(req.Content)
	if len(content) > maxSkillFileSize {
		writeSkillJSONError(w, http.StatusRequestEntityTooLarge, "too_large", "Skill file exceeds the 2 MB limit.", nil)
		return
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		writeSkillJSONError(w, http.StatusUnsupportedMediaType, "not_text", "Skill file must be UTF-8 text.", nil)
		return
	}

	workspaceSkillsRoot := filepath.Join(cfg.WorkspacePath(), "skills")
	relPath, relErr := filepath.Rel(workspaceSkillsRoot, item.Path)
	if relErr != nil || relPath == ".." ||
		strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) ||
		filepath.IsAbs(relPath) {
		writeSkillJSONError(w, http.StatusBadRequest, "invalid_skill_id", "Invalid skill path.", nil)
		return
	}

	workspaceSkillWriteMu.Lock()
	defer workspaceSkillWriteMu.Unlock()

	current, readErr := os.ReadFile(item.Path)
	if readErr != nil {
		writeSkillJSONError(w, http.StatusNotFound, "not_found", "Skill content not found.", nil)
		return
	}
	currentRevision := skillRevision(current)
	if !req.Force {
		if req.Revision == "" {
			writeSkillJSONError(w, http.StatusBadRequest, "revision_required", "Revision is required to update an existing skill.", nil)
			return
		}
		if currentRevision != req.Revision {
			writeSkillJSONError(w, http.StatusConflict, "revision_conflict",
				"The skill was changed by someone else.",
				map[string]any{"revision": currentRevision})
			return
		}
	}

	perm := os.FileMode(0o644)
	if info, statErr := os.Stat(item.Path); statErr == nil {
		if mode := info.Mode().Perm(); mode != 0 {
			perm = mode
		}
	}
	if err := ensureNoSymlinkComponents(workspaceSkillsRoot, filepath.ToSlash(relPath)); err != nil {
		writeSkillJSONError(w, http.StatusBadRequest, "invalid_skill_id", "Skill path is not allowed.", nil)
		return
	}
	if err := backupSkillContent(item.ID, current); err != nil {
		writeSkillJSONError(w, http.StatusInternalServerError, "backup_failed", "Failed to back up the skill.", nil)
		return
	}
	if err := fileutil.WriteFileAtomic(item.Path, content, perm); err != nil {
		writeSkillJSONError(w, http.StatusInternalServerError, "save_failed", "Failed to save the skill.", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(skillWriteResponse{
		Status:   "saved",
		ID:       item.ID,
		Revision: skillRevision(content),
		Size:     int64(len(content)),
	})
}

func (h *Handler) handleSearchSkills(w http.ResponseWriter, r *http.Request) {
	cfg, loadErr := config.LoadConfig(h.configPath)
	if loadErr != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", loadErr), http.StatusInternalServerError)
		return
	}
	if registryErr := ensureSkillRegistryToolEnabled(cfg, "find_skills"); registryErr != nil {
		http.Error(w, registryErr.Error(), http.StatusBadRequest)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	limit := 20
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsedLimit, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsedLimit < 1 || parsedLimit > 50 {
			http.Error(w, "limit must be between 1 and 50", http.StatusBadRequest)
			return
		}
		limit = parsedLimit
	}
	offset := 0
	if rawOffset := strings.TrimSpace(r.URL.Query().Get("offset")); rawOffset != "" {
		parsedOffset, parseErr := strconv.Atoi(rawOffset)
		if parseErr != nil || parsedOffset < 0 {
			http.Error(w, "offset must be 0 or greater", http.StatusBadRequest)
			return
		}
		offset = parsedOffset
	}

	sortOrder := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	if sortOrder == "" {
		sortOrder = "relevance"
	}
	if sortOrder != "relevance" && sortOrder != "downloads" {
		http.Error(w, "sort must be relevance or downloads", http.StatusBadRequest)
		return
	}

	installedSkills, err := buildOccupiedWorkspaceSkillsByDirectory(cfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to inspect installed skills: %v", err), http.StatusInternalServerError)
		return
	}

	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(skillSearchResponse{
			Results: []skillSearchResultItem{},
			Limit:   limit,
			Offset:  offset,
			HasMore: false,
			Sort:    sortOrder,
		})
		return
	}

	registryMgr := newSkillsRegistryManager(cfg)
	searchLimit := offset + limit + 1
	if sortOrder == "downloads" {
		// Downloads live on each result, so a wider relevance window is needed
		// before sorting to keep the popular ordering meaningful.
		searchLimit = maxRegistrySearchFanout
	}
	if searchLimit > maxRegistrySearchFanout {
		searchLimit = maxRegistrySearchFanout
	}
	results, err := registryMgr.SearchAll(r.Context(), query, searchLimit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to search skills: %v", err), http.StatusBadGateway)
		return
	}

	if sortOrder == "downloads" {
		sort.SliceStable(results, func(i, j int) bool {
			if results[i].Downloads != results[j].Downloads {
				return results[i].Downloads > results[j].Downloads
			}
			if results[i].Score != results[j].Score {
				return results[i].Score > results[j].Score
			}
			return results[i].Slug < results[j].Slug
		})
	}

	if offset > len(results) {
		offset = len(results)
	}

	end := offset + limit
	if end > len(results) {
		end = len(results)
	}

	pageResults := results[offset:end]
	response := make([]skillSearchResultItem, 0, len(pageResults))
	for _, result := range pageResults {
		response = append(
			response,
			buildSkillSearchResultItem(cfg, registryMgr, installedSkills, result),
		)
	}

	enrichSkillSearchSecurity(r.Context(), registryMgr, pageResults, response)

	w.Header().Set("Content-Type", "application/json")
	nextOffset := 0
	hasMore := len(results) > end
	if hasMore {
		nextOffset = end
	}
	json.NewEncoder(w).Encode(skillSearchResponse{
		Results:    response,
		Limit:      limit,
		Offset:     offset,
		NextOffset: nextOffset,
		HasMore:    hasMore,
		Sort:       sortOrder,
	})
}

func (h *Handler) handlePopularSkills(w http.ResponseWriter, r *http.Request) {
	cfg, loadErr := config.LoadConfig(h.configPath)
	if loadErr != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", loadErr), http.StatusInternalServerError)
		return
	}
	if registryErr := ensureSkillRegistryToolEnabled(cfg, "find_skills"); registryErr != nil {
		http.Error(w, registryErr.Error(), http.StatusBadRequest)
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsedLimit, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsedLimit < 1 || parsedLimit > 50 {
			http.Error(w, "limit must be between 1 and 50", http.StatusBadRequest)
			return
		}
		limit = parsedLimit
	}
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if len(cursor) > 1024 {
		http.Error(w, "cursor is too long", http.StatusBadRequest)
		return
	}

	registryMgr := newSkillsRegistryManager(cfg)
	registry := registryMgr.GetRegistry("clawhub")
	catalog, ok := registry.(skills.CatalogRegistry)
	if !ok {
		http.Error(w, "popular skills are not supported by the configured registries", http.StatusBadRequest)
		return
	}

	results, nextCursor, err := catalog.PopularSkills(r.Context(), limit, cursor)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load popular skills: %v", err), http.StatusBadGateway)
		return
	}

	installedSkills, err := buildOccupiedWorkspaceSkillsByDirectory(cfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to inspect installed skills: %v", err), http.StatusInternalServerError)
		return
	}

	response := make([]skillSearchResultItem, 0, len(results))
	for _, result := range results {
		response = append(
			response,
			buildSkillSearchResultItem(cfg, registryMgr, installedSkills, result),
		)
	}
	enrichSkillSearchSecurity(r.Context(), registryMgr, results, response)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(skillSearchResponse{
		Results:    response,
		Limit:      limit,
		Offset:     0,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
		Sort:       "downloads",
	})
}

func (h *Handler) handleInstallSkill(w http.ResponseWriter, r *http.Request) {
	cfg, loadErr := config.LoadConfig(h.configPath)
	if loadErr != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", loadErr), http.StatusInternalServerError)
		return
	}
	if registryErr := ensureSkillRegistryToolEnabled(cfg, "install_skill"); registryErr != nil {
		http.Error(w, registryErr.Error(), http.StatusBadRequest)
		return
	}

	var req installSkillRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", decodeErr), http.StatusBadRequest)
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	req.Registry = strings.TrimSpace(req.Registry)
	req.Version = strings.TrimSpace(req.Version)
	req.OwnerHandle = normalizeSkillOwner(req.OwnerHandle)
	req.InstallKind = strings.ToLower(strings.TrimSpace(req.InstallKind))
	if req.Registry == "" {
		req.Registry = defaultInstallSkillRegistry
	}

	if validateErr := utils.ValidateSkillIdentifier(req.Registry); validateErr != nil {
		http.Error(
			w,
			fmt.Sprintf("invalid registry %q: error: %s", req.Registry, validateErr.Error()),
			http.StatusBadRequest,
		)
		return
	}
	if err := validateSkillOwner(req.OwnerHandle); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.InstallKind == "skills-sh" {
		writeSkillJSONError(
			w,
			http.StatusUnprocessableEntity,
			"unsupported_source",
			"skills.sh skills are not supported yet. Install it from its GitHub repository instead.",
			nil,
		)
		return
	}

	registryMgr := newSkillsRegistryManager(cfg)
	registry := registryMgr.GetRegistry(req.Registry)
	if registry == nil {
		http.Error(w, fmt.Sprintf("registry %q not found", req.Registry), http.StatusBadRequest)
		return
	}
	dirName, err := registry.ResolveInstallDirName(req.Slug)
	if err != nil {
		if registry.Name() == "github" {
			writeSkillJSONError(
				w,
				http.StatusBadRequest,
				"invalid_github_url",
				"Enter a github.com repository URL or owner/repo/path.",
				nil,
			)
			return
		}
		http.Error(w, fmt.Sprintf("invalid slug %q: error: %s", req.Slug, err.Error()), http.StatusBadRequest)
		return
	}

	workspace := cfg.WorkspacePath()
	skillsRoot := filepath.Join(workspace, "skills")
	targetDir := filepath.Join(workspace, "skills", dirName)
	workspaceSkillWriteMu.Lock()
	defer workspaceSkillWriteMu.Unlock()

	targetExists := false
	if _, statErr := os.Stat(targetDir); statErr == nil {
		targetExists = true
	} else if !os.IsNotExist(statErr) {
		http.Error(w, fmt.Sprintf("Failed to inspect install target: %v", statErr), http.StatusInternalServerError)
		return
	}

	if !req.Force && targetExists {
		http.Error(w, fmt.Sprintf("skill %q already installed at %s", dirName, targetDir), http.StatusConflict)
		return
	}
	if mkdirErr := os.MkdirAll(skillsRoot, 0o755); mkdirErr != nil {
		http.Error(w, fmt.Sprintf("Failed to create skills directory: %v", mkdirErr), http.StatusInternalServerError)
		return
	}

	stagedWorkspaceRoot, stagedTargetDir, err := createStagedSkillInstall(skillsRoot, dirName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to prepare staged install: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(stagedWorkspaceRoot)

	result, err := installSkillFromRegistry(
		r.Context(),
		registry,
		req.Slug,
		req.Version,
		req.OwnerHandle,
		stagedTargetDir,
	)
	if err != nil {
		var ambiguous *skills.AmbiguousSkillSlugError
		if errors.As(err, &ambiguous) {
			writeSkillJSONError(
				w,
				http.StatusConflict,
				"ambiguous_slug",
				ambiguous.Error(),
				map[string]any{"slug": ambiguous.Slug, "matches": ambiguous.Matches},
			)
			return
		}
		writeSkillInstallFailure(w, registry.Name(), err)
		return
	}
	if result.IsMalwareBlocked {
		writeSkillJSONError(
			w,
			http.StatusForbidden,
			"malware_blocked",
			fmt.Sprintf("Skill %q is flagged as malicious and cannot be installed.", req.Slug),
			nil,
		)
		return
	}
	if result.IsSuspicious && !req.AcceptSuspicious {
		writeSkillJSONError(
			w,
			http.StatusConflict,
			"suspicious_skill",
			"This skill is flagged as suspicious. Review it before installing.",
			map[string]any{
				"slug":     req.Slug,
				"registry": registry.Name(),
				"summary":  result.Summary,
			},
		)
		return
	}

	if findWorkspaceSkillInfoByDirectory(stagedWorkspaceRoot, dirName) == nil {
		http.Error(
			w,
			fmt.Sprintf("Failed to install skill: registry archive for %q is not a valid skill", req.Slug),
			http.StatusBadGateway,
		)
		return
	}

	installedAt := time.Now().UnixMilli()
	normalizedSlug := skills.NormalizeInstallTargetForRegistryInstance(registry, req.Slug)
	registryURL := registrySkillURLForOwner(
		cfg,
		registry.Name(),
		req.Slug,
		req.OwnerHandle,
		result.Version,
	)
	if registryURL == "" {
		registryURL = registry.SkillURL(req.Slug, result.Version)
	}
	if registryURL == "" {
		registryURL = registry.SkillURL(normalizedSlug, result.Version)
	}
	if err := persistSkillOriginMeta(stagedTargetDir, installedSkillOriginMeta{
		Version:          1,
		OriginKind:       "third_party",
		Registry:         registry.Name(),
		Slug:             normalizedSlug,
		OwnerHandle:      req.OwnerHandle,
		RegistryURL:      registryURL,
		InstalledVersion: result.Version,
		InstalledAt:      installedAt,
	}); err != nil {
		http.Error(w, fmt.Sprintf("Failed to persist skill metadata: %v", err), http.StatusInternalServerError)
		return
	}

	if err := commitStagedSkillInstall(
		stagedWorkspaceRoot,
		stagedTargetDir,
		targetDir,
		req.Force && targetExists,
	); err != nil {
		http.Error(w, fmt.Sprintf("Failed to activate installed skill: %v", err), http.StatusInternalServerError)
		return
	}

	validatedSkill := findWorkspaceSkillByDirectory(cfg, dirName)
	if validatedSkill == nil {
		http.Error(
			w,
			fmt.Sprintf("Failed to install skill: activated archive for %q is not a valid skill", req.Slug),
			http.StatusBadGateway,
		)
		return
	}

	installedSkill := *validatedSkill
	installedSkill.OriginKind = "third_party"
	installedSkill.RegistryName = registry.Name()
	installedSkill.RegistryURL = registryURL
	installedSkill.InstalledVersion = result.Version
	installedSkill.InstalledAt = installedAt
	installedSkill.OwnerHandle = req.OwnerHandle

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(installSkillResponse{
		Status:         "ok",
		Slug:           req.Slug,
		Registry:       registry.Name(),
		Version:        result.Version,
		Summary:        result.Summary,
		IsSuspicious:   result.IsSuspicious,
		InstalledSkill: &installedSkill,
	})
}

func (h *Handler) handleImportSkill(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	err = r.ParseMultipartForm(2 << 20)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid multipart form: %v", err), http.StatusBadRequest)
		return
	}

	uploadedFile, fileHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer uploadedFile.Close()

	content, err := io.ReadAll(io.LimitReader(uploadedFile, maxImportedSkillSize+1))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read file: %v", err), http.StatusBadRequest)
		return
	}
	if len(content) > maxImportedSkillSize {
		http.Error(w, "file exceeds 1MB limit", http.StatusBadRequest)
		return
	}
	workspaceSkillWriteMu.Lock()
	defer workspaceSkillWriteMu.Unlock()

	importedSkill, statusCode, err := importUploadedSkill(cfg, fileHeader.Filename, content)
	if err != nil {
		http.Error(w, err.Error(), statusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(importedSkill)
}

func (h *Handler) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	loader := newSkillsLoader(cfg.WorkspacePath())
	name := r.PathValue("name")
	workspaceSkillWriteMu.Lock()
	defer workspaceSkillWriteMu.Unlock()

	var matchedNonWorkspace bool
	for _, skill := range loader.ListSkills() {
		if skill.Name != name {
			continue
		}
		if skill.Source != "workspace" {
			matchedNonWorkspace = true
			continue
		}
		if err := os.RemoveAll(filepath.Dir(skill.Path)); err != nil {
			http.Error(w, fmt.Sprintf("Failed to delete skill: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}
	if matchedNonWorkspace {
		http.Error(w, "only workspace skills can be deleted", http.StatusBadRequest)
		return
	}

	http.Error(w, "Skill not found", http.StatusNotFound)
}

func (h *Handler) handleDeleteSkillByID(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	item, err := findSkillByID(cfg, r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if item == nil {
		http.Error(w, "Skill not found", http.StatusNotFound)
		return
	}
	if item.Source != "workspace" {
		http.Error(w, "only workspace skills can be deleted", http.StatusBadRequest)
		return
	}

	workspaceSkillWriteMu.Lock()
	defer workspaceSkillWriteMu.Unlock()

	if err := os.RemoveAll(filepath.Dir(item.Path)); err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete skill: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func newSkillsLoader(workspace string) *skills.SkillsLoader {
	return skills.NewSkillsLoader(
		workspace,
		filepath.Join(globalConfigDir(), "skills"),
		builtinSkillsDir(),
	)
}

func newSkillsRegistryManager(cfg *config.Config) *skills.RegistryManager {
	return skills.NewRegistryManagerFromToolsConfig(cfg.Tools.Skills)
}

func ensureSkillRegistryToolEnabled(cfg *config.Config, toolName string) error {
	if !cfg.Tools.IsToolEnabled("skills") {
		return fmt.Errorf("tools.skills is disabled")
	}
	if !cfg.Tools.IsToolEnabled(toolName) {
		return fmt.Errorf("%s is disabled", toolName)
	}
	return nil
}

func buildSkillSupportItems(cfg *config.Config) ([]skillSupportItem, error) {
	rawSkills := newSkillsLoader(cfg.WorkspacePath()).ListSkills()
	items := make([]skillSupportItem, 0, len(rawSkills))
	for _, skill := range rawSkills {
		item, err := enrichSkillInfo(cfg, skill)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func buildWorkspaceSkillItemsByDirectory(cfg *config.Config) (map[string]skillSupportItem, error) {
	result := make(map[string]skillSupportItem)
	items, err := buildSkillSupportItems(cfg)
	if err != nil {
		return nil, err
	}
	for _, skill := range items {
		if skill.Source != "workspace" {
			continue
		}
		dir := filepath.Base(filepath.Dir(skill.Path))
		if dir == "" {
			continue
		}
		result[dir] = skill
	}
	return result, nil
}

func buildOccupiedWorkspaceSkillsByDirectory(cfg *config.Config) (map[string]skillSupportItem, error) {
	result := make(map[string]skillSupportItem)
	items, err := buildSkillSupportItems(cfg)
	if err != nil {
		return nil, err
	}
	for _, skill := range items {
		dirName := strings.ToLower(skill.Directory)
		if dirName != "" {
			key := "dir:" + dirName
			existing, exists := result[key]
			if !exists || (existing.Source != "workspace" && skill.Source == "workspace") {
				result[key] = skill
			}
		}

		meta, err := readInstalledSkillOriginMeta(skill.Path)
		if err != nil || meta == nil || meta.Slug == "" {
			continue
		}
		registryName := strings.ToLower(strings.TrimSpace(meta.Registry))
		slug := strings.ToLower(strings.TrimSpace(meta.Slug))
		if slug == "" {
			continue
		}
		owner := normalizeSkillOwner(meta.OwnerHandle)
		if owner != "" {
			result["owner:"+registryName+"|"+owner+"|"+slug] = skill
		} else {
			result["slug:"+registryName+"|"+slug] = skill
		}
	}
	return result, nil
}

// findInstalledSkill matches a search result to an installed skill by
// registry + owner + slug first, then by legacy slug-only metadata, and
// finally by install directory (manual, global, and builtin skills).
func findInstalledSkill(
	installedSkills map[string]skillSupportItem,
	registryMgr *skills.RegistryManager,
	result skills.SearchResult,
) (skillSupportItem, bool) {
	registryName := strings.ToLower(strings.TrimSpace(result.RegistryName))
	slug := strings.ToLower(strings.TrimSpace(result.Slug))
	owner := normalizeSkillOwner(result.OwnerHandle)

	if owner != "" {
		if skill, ok := installedSkills["owner:"+registryName+"|"+owner+"|"+slug]; ok {
			return skill, true
		}
	}
	if skill, ok := installedSkills["slug:"+registryName+"|"+slug]; ok {
		return skill, true
	}

	dirCandidates := []string{slug}
	if registryMgr != nil {
		if registry := registryMgr.GetRegistry(result.RegistryName); registry != nil {
			if dirName, err := registry.ResolveInstallDirName(result.Slug); err == nil {
				dirCandidates = append([]string{strings.ToLower(dirName)}, dirCandidates...)
			}
		}
	}
	for _, dirName := range dirCandidates {
		if dirName == "" {
			continue
		}
		skill, ok := installedSkills["dir:"+dirName]
		if !ok {
			continue
		}
		// A third-party skill installed by a different publisher must not be
		// reported as the same skill just because the folder name matches.
		if skill.OwnerHandle != "" && owner != "" && !strings.EqualFold(skill.OwnerHandle, owner) {
			continue
		}
		return skill, true
	}
	return skillSupportItem{}, false
}

func findWorkspaceSkillByDirectory(cfg *config.Config, directory string) *skillSupportItem {
	items, err := buildWorkspaceSkillItemsByDirectory(cfg)
	if err != nil {
		return nil
	}
	skill, ok := items[directory]
	if !ok {
		return nil
	}
	return &skill
}

func findWorkspaceSkillInfoByDirectory(workspace, directory string) *skills.SkillInfo {
	loader := skills.NewSkillsLoader(workspace, "", "")
	for _, skill := range loader.ListSkills() {
		if skill.Source != "workspace" {
			continue
		}
		if filepath.Base(filepath.Dir(skill.Path)) != directory {
			continue
		}
		skillCopy := skill
		return &skillCopy
	}
	return nil
}

func createStagedSkillInstall(skillsRoot, slug string) (string, string, error) {
	stagedWorkspaceRoot, err := os.MkdirTemp(skillsRoot, "."+slug+"-install-*")
	if err != nil {
		return "", "", err
	}
	stagedTargetDir := filepath.Join(stagedWorkspaceRoot, "skills", slug)
	return stagedWorkspaceRoot, stagedTargetDir, nil
}

func commitStagedSkillInstall(stagedWorkspaceRoot, stagedTargetDir, targetDir string, replaceExisting bool) error {
	if !replaceExisting {
		return os.Rename(stagedTargetDir, targetDir)
	}

	backupDir, err := reserveTempDirPath(filepath.Dir(targetDir), "."+filepath.Base(targetDir)+"-backup-*")
	if err != nil {
		return err
	}

	if err := os.Rename(targetDir, backupDir); err != nil {
		return fmt.Errorf("failed to move existing skill aside: %w", err)
	}

	if err := os.Rename(stagedTargetDir, targetDir); err != nil {
		if rollbackErr := os.Rename(backupDir, targetDir); rollbackErr != nil {
			return fmt.Errorf("failed to activate replacement: %w (rollback failed: %v)", err, rollbackErr)
		}
		return fmt.Errorf("failed to activate replacement: %w", err)
	}

	_ = os.RemoveAll(backupDir)
	_ = os.RemoveAll(stagedWorkspaceRoot)
	return nil
}

func reserveTempDirPath(parent, pattern string) (string, error) {
	tempDir, err := os.MkdirTemp(parent, pattern)
	if err != nil {
		return "", err
	}
	if err := os.Remove(tempDir); err != nil {
		return "", err
	}
	return tempDir, nil
}

func enrichSkillInfo(cfg *config.Config, skill skills.SkillInfo) (skillSupportItem, error) {
	directory := filepath.Base(filepath.Dir(skill.Path))
	item := skillSupportItem{
		ID:           skill.Source + ":" + directory,
		Name:         skill.Name,
		Path:         skill.Path,
		Directory:    directory,
		Source:       skill.Source,
		Description:  skill.Description,
		OriginKind:   "builtin",
		Editable:     skill.Source == "workspace",
		NameMismatch: skill.Name != directory,
	}

	switch skill.Source {
	case "builtin":
		item.OriginKind = "builtin"
	case "global":
		item.OriginKind = "builtin"
	case "workspace":
		meta, err := readInstalledSkillOriginMeta(skill.Path)
		if err == nil && meta != nil {
			item.OwnerHandle = normalizeSkillOwner(meta.OwnerHandle)
			item.RegistrySlug = meta.Slug
			item.CanUpdate = skill.Source == "workspace" &&
				strings.TrimSpace(meta.Registry) != "" &&
				strings.TrimSpace(meta.Slug) != ""
			switch meta.OriginKind {
			case "manual":
				item.OriginKind = "manual"
				item.InstalledAt = meta.InstalledAt
			case "third_party":
				item.OriginKind = "third_party"
				item.RegistryName = meta.Registry
				item.RegistryURL = registrySkillURLFromMeta(cfg, meta)
				item.InstalledVersion = meta.InstalledVersion
				item.InstalledAt = meta.InstalledAt
			default:
				if meta.Registry != "" || meta.Slug != "" || meta.InstalledVersion != "" {
					item.OriginKind = "third_party"
					item.RegistryName = meta.Registry
					item.RegistryURL = registrySkillURLFromMeta(cfg, meta)
					item.InstalledVersion = meta.InstalledVersion
					item.InstalledAt = meta.InstalledAt
				} else {
					item.OriginKind = "builtin"
					item.InstalledAt = meta.InstalledAt
				}
			}
		} else {
			item.OriginKind = "builtin"
		}
	default:
		item.OriginKind = "builtin"
	}

	return item, nil
}

func readInstalledSkillOriginMeta(skillPath string) (*installedSkillOriginMeta, error) {
	metaPath := filepath.Join(filepath.Dir(skillPath), ".skill-origin.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var meta installedSkillOriginMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func writeSkillOriginMeta(targetDir string, meta installedSkillOriginMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(filepath.Join(targetDir, ".skill-origin.json"), data, 0o600)
}

func registrySkillURL(cfg *config.Config, registryName, slug, version string) string {
	return registrySkillURLForOwner(cfg, registryName, slug, "", version)
}

func registrySkillURLForOwner(
	cfg *config.Config,
	registryName, slug, ownerHandle, version string,
) string {
	if cfg == nil || registryName == "" || slug == "" {
		return ""
	}
	registry := skills.LookupRegistryFromToolsConfig(cfg.Tools.Skills, registryName)
	if registry == nil {
		return ""
	}
	if ownerAware, ok := registry.(skills.OwnerAwareRegistry); ok && ownerHandle != "" {
		if url := ownerAware.SkillURLForOwner(slug, ownerHandle, version); url != "" {
			return url
		}
	}
	return registry.SkillURL(slug, version)
}

func registrySkillURLFromMeta(cfg *config.Config, meta *installedSkillOriginMeta) string {
	if meta == nil || meta.Slug == "" {
		return ""
	}
	if meta.RegistryURL != "" {
		return meta.RegistryURL
	}
	if cfg == nil || meta.Registry == "" {
		return ""
	}
	return registrySkillURLForOwner(cfg, meta.Registry, meta.Slug, meta.OwnerHandle, meta.InstalledVersion)
}

var skillOwnerPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func normalizeSkillOwner(owner string) string {
	owner = strings.TrimSpace(owner)
	owner = strings.TrimLeft(owner, "@")
	return strings.ToLower(owner)
}

func validateSkillOwner(owner string) error {
	if owner == "" {
		return nil
	}
	if !skillOwnerPattern.MatchString(owner) {
		return fmt.Errorf("invalid owner_handle %q", owner)
	}
	return nil
}

func installSkillFromRegistry(
	ctx context.Context,
	registry skills.SkillRegistry,
	slug, version, ownerHandle, targetDir string,
) (*skills.InstallResult, error) {
	if ownerAware, ok := registry.(skills.OwnerAwareRegistry); ok && ownerHandle != "" {
		return ownerAware.DownloadAndInstallForOwner(ctx, slug, version, ownerHandle, targetDir)
	}
	return registry.DownloadAndInstall(ctx, slug, version, targetDir)
}

func deriveSkillInstallStatus(
	result skills.SearchResult,
	security *skills.SkillSecurity,
) (string, string) {
	if security != nil && security.Decision == "fail" {
		return "blocked", "This skill is blocked by the registry's security moderation."
	}
	if result.InstallKind == "skills-sh" {
		return "unsupported", "This skill comes from skills.sh and is not supported yet."
	}
	switch strings.ToLower(result.Installability) {
	case "", "installable":
		return "installable", ""
	case "blocked", "malware", "quarantined", "hidden", "removed":
		return "blocked", "This skill is not available for download from the registry."
	default:
		return "unsupported", "This skill cannot be installed from its current source."
	}
}

func buildSkillSearchResultItem(
	cfg *config.Config,
	registryMgr *skills.RegistryManager,
	installedSkills map[string]skillSupportItem,
	result skills.SearchResult,
) skillSearchResultItem {
	installedSkill, installed := findInstalledSkill(installedSkills, registryMgr, result)

	resultURL := result.CanonicalURL
	if resultURL == "" {
		resultURL = registrySkillURLForOwner(
			cfg,
			result.RegistryName,
			result.Slug,
			result.OwnerHandle,
			result.Version,
		)
	}
	installStatus, installReason := deriveSkillInstallStatus(result, nil)
	item := skillSearchResultItem{
		Score:            result.Score,
		Slug:             result.Slug,
		DisplayName:      result.DisplayName,
		Summary:          result.Summary,
		Version:          result.Version,
		RegistryName:     result.RegistryName,
		URL:              resultURL,
		Installed:        installed,
		OwnerHandle:      result.OwnerHandle,
		InstallKind:      result.InstallKind,
		InstallReference: result.InstallReference,
		CanonicalURL:     result.CanonicalURL,
		Downloads:        result.Downloads,
		InstallStatus:    installStatus,
		InstallReason:    installReason,
	}
	if installed {
		item.InstalledName = installedSkill.Name
		item.InstalledID = installedSkill.ID
	}
	return item
}

// enrichSkillSearchSecurity attaches ClawHub audit verdicts to the current
// result page. Failures only remove the badge; they never fail the search.
func enrichSkillSearchSecurity(
	ctx context.Context,
	registryMgr *skills.RegistryManager,
	results []skills.SearchResult,
	items []skillSearchResultItem,
) {
	type lookup struct {
		index    int
		registry skills.OwnerAwareRegistry
		slug     string
		owner    string
	}

	lookups := make([]lookup, 0, len(items))
	for i := range results {
		if i >= len(items) || results[i].InstallKind == "skills-sh" {
			continue
		}
		registry := registryMgr.GetRegistry(results[i].RegistryName)
		ownerAware, ok := registry.(skills.OwnerAwareRegistry)
		if !ok {
			continue
		}
		lookups = append(lookups, lookup{
			index:    i,
			registry: ownerAware,
			slug:     results[i].Slug,
			owner:    results[i].OwnerHandle,
		})
	}
	if len(lookups) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, item := range lookups {
		wg.Add(1)
		go func(item lookup) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			security, err := item.registry.GetSecuritySummary(ctx, item.slug, item.owner)
			if err != nil || security == nil {
				return
			}
			status, reason := deriveSkillInstallStatus(results[item.index], security)
			items[item.index].Security = &skillSecurityItem{
				Decision:  security.Decision,
				Status:    security.Status,
				RiskLevel: security.RiskLevel,
				Summary:   security.Summary,
				AuditURL:  security.AuditURL,
			}
			items[item.index].InstallStatus = status
			items[item.index].InstallReason = reason
		}(item)
	}
	wg.Wait()
}

func writeSkillJSONError(
	w http.ResponseWriter,
	status int,
	code, message string,
	extra map[string]any,
) {
	payload := map[string]any{"code": code, "error": message}
	for key, value := range extra {
		payload[key] = value
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeSkillInstallFailure translates registry errors into actionable HTTP
// responses. GitHub-specific errors are install-by-URL paths, so they get
// messages that explain how to fix the address.
func writeSkillInstallFailure(w http.ResponseWriter, registryName string, err error) {
	message := err.Error()

	if registryName == "github" {
		lower := strings.ToLower(message)
		switch {
		case strings.Contains(lower, "skill.md not found"):
			writeSkillJSONError(
				w,
				http.StatusUnprocessableEntity,
				"skill_not_found",
				"No SKILL.md was found at that GitHub location. Paste the URL of the folder that contains SKILL.md.",
				nil,
			)
			return
		case strings.Contains(lower, "http 404"):
			writeSkillJSONError(
				w,
				http.StatusUnprocessableEntity,
				"skill_not_found",
				"GitHub returned 404. Check that the repository and path are public and contain SKILL.md.",
				nil,
			)
			return
		case strings.Contains(lower, "rate limit"):
			writeSkillJSONError(
				w,
				http.StatusTooManyRequests,
				"github_rate_limited",
				"GitHub rate limit reached. Try again later.",
				nil,
			)
			return
		case strings.Contains(lower, "http 401"):
			writeSkillJSONError(
				w,
				http.StatusForbidden,
				"github_auth_required",
				"GitHub rejected the request. A valid token may be required for this repository.",
				nil,
			)
			return
		case strings.Contains(lower, "http 403"):
			writeSkillJSONError(
				w,
				http.StatusForbidden,
				"github_access_denied",
				"GitHub denied access to this repository.",
				nil,
			)
			return
		case strings.Contains(lower, "invalid github"),
			strings.Contains(lower, "invalid format"),
			strings.Contains(lower, "invalid slug"):
			writeSkillJSONError(
				w,
				http.StatusBadRequest,
				"invalid_github_url",
				"Enter a github.com repository URL or owner/repo/path.",
				nil,
			)
			return
		}
	}

	writeSkillJSONError(
		w,
		http.StatusBadGateway,
		"install_failed",
		fmt.Sprintf("Failed to install skill: %v", err),
		nil,
	)
}

func normalizeImportedSkillName(filename string, content []byte) (string, error) {
	return normalizeImportedSkillNameWithHint(filename, "", content)
}

func normalizeImportedSkillNameWithHint(filename, directoryHint string, content []byte) (string, error) {
	rawContent := strings.ReplaceAll(string(content), "\r\n", "\n")
	rawContent = strings.ReplaceAll(rawContent, "\r", "\n")
	metadata, _ := extractImportedSkillMetadata(rawContent)

	raw := strings.TrimSpace(metadata["name"])
	if raw == "" {
		raw = strings.TrimSpace(directoryHint)
	}
	if raw == "" {
		raw = strings.TrimSpace(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	}
	raw = strings.ToLower(raw)
	raw = strings.ReplaceAll(raw, "_", "-")
	raw = strings.ReplaceAll(raw, " ", "-")
	raw = skillNameSanitizer.ReplaceAllString(raw, "-")
	raw = strings.Trim(raw, "-")
	raw = strings.Join(strings.FieldsFunc(raw, func(r rune) bool { return r == '-' }), "-")

	if raw == "" {
		return "", fmt.Errorf("skill name is required in frontmatter or filename")
	}
	if len(raw) > 64 {
		return "", fmt.Errorf("skill name exceeds 64 characters")
	}
	matched, err := regexp.MatchString(`^[a-z0-9]+(-[a-z0-9]+)*$`, raw)
	if err != nil || !matched {
		return "", fmt.Errorf("skill name must be alphanumeric with hyphens")
	}
	return raw, nil
}

func normalizeImportedSkillContent(content []byte, skillName string) []byte {
	raw := strings.ReplaceAll(string(content), "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")

	metadata, body := extractImportedSkillMetadata(raw)
	description := strings.TrimSpace(metadata["description"])
	if description == "" {
		description = inferImportedSkillDescription(body)
	}
	if description == "" {
		description = "Imported skill"
	}
	if len(description) > 1024 {
		description = strings.TrimSpace(description[:1024])
	}

	body = strings.TrimLeft(body, "\n")
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.WriteString("name: ")
	builder.WriteString(skillName)
	builder.WriteString("\n")
	builder.WriteString("description: ")
	builder.WriteString(description)
	builder.WriteString("\n")
	builder.WriteString("---\n\n")
	builder.WriteString(body)
	if !strings.HasSuffix(builder.String(), "\n") {
		builder.WriteString("\n")
	}
	return []byte(builder.String())
}

func importUploadedSkill(cfg *config.Config, filename string, content []byte) (*skillSupportItem, int, error) {
	if isImportedSkillArchive(filename, content) {
		return importUploadedSkillArchive(cfg, filename, content)
	}
	return importUploadedMarkdownSkill(cfg, filename, content)
}

func importUploadedMarkdownSkill(cfg *config.Config, filename string, content []byte) (*skillSupportItem, int, error) {
	skillName, err := normalizeImportedSkillName(filename, content)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}

	normalizedContent := normalizeImportedSkillContent(content, skillName)
	workspace := cfg.WorkspacePath()
	skillDir := filepath.Join(workspace, "skills", skillName)
	skillFile := filepath.Join(skillDir, "SKILL.md")

	if err := ensureWorkspaceSkillDoesNotExist(skillDir); err != nil {
		return nil, statusCodeForImportedSkillWriteError(err), err
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("Failed to create skill directory: %v", err)
	}
	if err := fileutil.WriteFileAtomic(skillFile, normalizedContent, 0o644); err != nil {
		_ = os.RemoveAll(skillDir)
		return nil, http.StatusInternalServerError, fmt.Errorf("Failed to save skill: %v", err)
	}

	return finalizeImportedSkill(cfg, skillDir, skillName, false)
}

func importUploadedSkillArchive(cfg *config.Config, filename string, content []byte) (*skillSupportItem, int, error) {
	tmpDir, tempDirErr := os.MkdirTemp("", "picoclaw-skill-import-*")
	if tempDirErr != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("Failed to create temp directory: %v", tempDirErr)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "import.zip")
	if writeErr := fileutil.WriteFileAtomic(archivePath, content, 0o600); writeErr != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("Failed to stage uploaded archive: %v", writeErr)
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if extractErr := utils.ExtractZipFile(archivePath, extractDir); extractErr != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid ZIP archive: %w", extractErr)
	}

	skillRoot, err := findImportedSkillRoot(extractDir)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}

	skillFile := filepath.Join(skillRoot, "SKILL.md")
	skillContent, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("failed to read SKILL.md from archive: %w", err)
	}

	directoryHint := ""
	if filepath.Clean(skillRoot) != filepath.Clean(extractDir) {
		directoryHint = filepath.Base(skillRoot)
	}
	skillName, err := normalizeImportedSkillNameWithHint(filename, directoryHint, skillContent)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}

	workspace := cfg.WorkspacePath()
	skillDir := filepath.Join(workspace, "skills", skillName)
	if err := ensureWorkspaceSkillDoesNotExist(skillDir); err != nil {
		return nil, statusCodeForImportedSkillWriteError(err), err
	}
	if err := copyImportedSkillTree(skillRoot, skillDir); err != nil {
		_ = os.RemoveAll(skillDir)
		return nil, http.StatusInternalServerError, fmt.Errorf("Failed to save skill: %v", err)
	}

	normalizedContent := normalizeImportedSkillContent(skillContent, skillName)
	if err := fileutil.WriteFileAtomic(filepath.Join(skillDir, "SKILL.md"), normalizedContent, 0o644); err != nil {
		_ = os.RemoveAll(skillDir)
		return nil, http.StatusInternalServerError, fmt.Errorf("Failed to normalize skill: %v", err)
	}

	return finalizeImportedSkill(cfg, skillDir, skillName, true)
}

func isImportedSkillArchive(filename string, content []byte) bool {
	if strings.EqualFold(filepath.Ext(filename), ".zip") {
		return true
	}
	return len(content) >= 4 && bytes.HasPrefix(content, []byte("PK\x03\x04"))
}

func ensureWorkspaceSkillDoesNotExist(skillDir string) error {
	if _, err := os.Stat(skillDir); err == nil {
		return errImportedSkillExists
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect skill directory: %w", err)
	}
	return nil
}

func statusCodeForImportedSkillWriteError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, errImportedSkillExists) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func finalizeImportedSkill(
	cfg *config.Config,
	skillDir string,
	skillName string,
	requireValidatedSkill bool,
) (*skillSupportItem, int, error) {
	if err := persistSkillOriginMeta(skillDir, installedSkillOriginMeta{
		Version:     1,
		OriginKind:  "manual",
		InstalledAt: time.Now().UnixMilli(),
	}); err != nil {
		_ = os.RemoveAll(skillDir)
		return nil, http.StatusInternalServerError, fmt.Errorf("Failed to persist skill metadata: %v", err)
	}

	if importedSkill := findWorkspaceSkillByDirectory(cfg, skillName); importedSkill != nil {
		return importedSkill, http.StatusOK, nil
	}

	if requireValidatedSkill {
		_ = os.RemoveAll(skillDir)
		return nil, http.StatusBadRequest, fmt.Errorf("imported archive is not a valid skill")
	}

	return &skillSupportItem{
		ID:          "workspace:" + skillName,
		Name:        skillName,
		Path:        filepath.Join(skillDir, "SKILL.md"),
		Directory:   skillName,
		Source:      "workspace",
		Description: "Imported skill",
		OriginKind:  "manual",
		Editable:    true,
	}, http.StatusOK, nil
}

func findImportedSkillRoot(extractDir string) (string, error) {
	skillFiles := make([]string, 0, 1)
	err := filepath.WalkDir(extractDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == "SKILL.md" {
			skillFiles = append(skillFiles, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to inspect ZIP archive: %w", err)
	}

	switch len(skillFiles) {
	case 0:
		return "", fmt.Errorf("ZIP archive must contain a SKILL.md file")
	case 1:
		return filepath.Dir(skillFiles[0]), nil
	default:
		return "", fmt.Errorf("ZIP archive must contain exactly one SKILL.md file")
	}
}

func copyImportedSkillTree(srcDir, destDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return os.MkdirAll(destDir, 0o755)
		}

		destPath := filepath.Join(destDir, relPath)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("archive contains unsupported file %q", relPath)
		}
		return fileutil.CopyFile(path, destPath, info.Mode().Perm())
	})
}

func extractImportedSkillMetadata(raw string) (map[string]string, string) {
	matches := importedSkillFrontmatter.FindStringSubmatch(raw)
	if len(matches) != 2 {
		return map[string]string{}, raw
	}
	meta := parseImportedSkillYAML(matches[1])
	body := importedSkillFrontmatter.ReplaceAllString(raw, "")
	return meta, body
}

func parseImportedSkillYAML(frontmatter string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return result
}

func inferImportedSkillDescription(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "#-*0123456789. ")
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func loadSkillContent(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return skillFrontmatterStripper.ReplaceAllString(string(content), ""), nil
}

func globalConfigDir() string {
	return config.GetHome()
}

func builtinSkillsDir() string {
	if path := os.Getenv(config.EnvBuiltinSkills); path != "" {
		return path
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(wd, "skills")
}
