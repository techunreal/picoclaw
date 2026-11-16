package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/routing"
)

const (
	maxProfileZipBytes   = 20 << 20
	maxProfileEntries    = 300
	maxProfileFileBytes  = 5 << 20
	maxProfileTotalBytes = 20 << 20
	profilePreviewRunes  = 1500
	maxIgnoredList       = 50
)

var identityBulletPattern = regexp.MustCompile(`(?mi)^\s*[-*]?\s*\*\*(name|emoji|vibe):\*\*\s*(.+?)\s*$`)

type profileIdentity struct {
	Name  string `json:"name,omitempty"`
	Emoji string `json:"emoji,omitempty"`
	Vibe  string `json:"vibe,omitempty"`
}

type profileFilePreview struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Size    int    `json:"size"`
	Preview string `json:"preview"`
}

type profilePreview struct {
	Root         string               `json:"root"`
	Files        []profileFilePreview `json:"files"`
	Identity     profileIdentity      `json:"identity"`
	SuggestedID  string               `json:"suggested_id"`
	Combined     bool                 `json:"combined"`
	Ignored      []string             `json:"ignored"`
	IgnoredCount int                  `json:"ignored_count"`
	Notes        []string             `json:"notes,omitempty"`
	targets      map[string][]byte
}

type profileImportPlan struct {
	preview   *profilePreview
	workspace string
	agent     config.AgentConfig
	existing  bool
}

func (h *Handler) registerAgentImportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/agents/import/inspect", h.handleInspectAgentProfile)
	mux.HandleFunc("POST /api/agents/import", h.handleImportAgentProfile)
}

func (h *Handler) handleInspectAgentProfile(w http.ResponseWriter, r *http.Request) {
	files, err := readProfileUpload(r)
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}
	preview, err := buildProfilePreview(files)
	if err != nil {
		writeAgentsAPIError(w, http.StatusUnprocessableEntity, "profile_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (h *Handler) handleImportAgentProfile(w http.ResponseWriter, r *http.Request) {
	h.configMu.Lock()
	defer h.configMu.Unlock()

	files, err := readProfileUpload(r)
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}
	preview, err := buildProfilePreview(files)
	if err != nil {
		writeAgentsAPIError(w, http.StatusUnprocessableEntity, "profile_not_found", err.Error())
		return
	}

	agentID := strings.TrimSpace(r.FormValue("id"))
	if !agentIDPattern.MatchString(agentID) {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_agent_id",
			"agent id must match ^[a-z0-9][a-z0-9_-]{0,63}$")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if utf8.RuneCountInString(name) > maxAgentNameLen {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_agent_name", "agent name is too long")
		return
	}
	emoji := strings.TrimSpace(r.FormValue("emoji"))
	if utf8.RuneCountInString(emoji) > maxAvatarEmoji || strings.ContainsFunc(emoji, unicode.IsControl) {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_avatar", "avatar emoji is invalid")
		return
	}
	overwrite := strings.EqualFold(strings.TrimSpace(r.FormValue("overwrite")), "true")

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}

	plan, err := buildProfileImportPlan(cfg, agentID, preview)
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "import_failed", err.Error())
		return
	}

	conflicts := profileConflicts(plan.workspace, preview.targets)
	if len(conflicts) > 0 && !overwrite {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":               "profile_conflict",
			"error":              "some profile files already exist in the target workspace",
			"files":              conflicts,
			"agent":              agentID,
			"overwrite_required": true,
		})
		return
	}

	if err := os.MkdirAll(plan.workspace, 0o755); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "workspace_create_failed", err.Error())
		return
	}
	if !plan.existing {
		if err := seedAgentWorkspace(cfg, plan.workspace); err != nil {
			writeAgentsAPIError(w, http.StatusInternalServerError, "workspace_seed_failed", err.Error())
			return
		}
	}

	written := make([]string, 0, len(preview.targets))
	for _, target := range []string{"AGENT.md", "SOUL.md", "IDENTITY.md"} {
		data, ok := preview.targets[target]
		if !ok {
			continue
		}
		if err := backupProfileTarget(agentID, plan.workspace, target); err != nil {
			writeAgentsAPIError(w, http.StatusInternalServerError, "backup_failed", err.Error())
			return
		}
		if err := writeFileAtomic(filepath.Join(plan.workspace, target), data, 0o644); err != nil {
			writeAgentsAPIError(w, http.StatusInternalServerError, "profile_write_failed", err.Error())
			return
		}
		written = append(written, target)
	}

	updateImportedAgentConfig(cfg, plan, agentID, name, emoji)
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_save_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"restart_required": true,
		"agent":            agentID,
		"workspace":        plan.workspace,
		"written":          written,
	})
}

func buildProfileImportPlan(cfg *config.Config, agentID string, preview *profilePreview) (*profileImportPlan, error) {
	plan := &profileImportPlan{preview: preview, agent: config.AgentConfig{ID: agentID}}
	id := routing.NormalizeAgentID(agentID)
	for i := range cfg.Agents.List {
		agentCfg := cfg.Agents.List[i]
		if routing.NormalizeAgentID(strings.TrimSpace(agentCfg.ID)) != id {
			continue
		}
		workspace, _ := effectiveAgentWorkspace(cfg, agentCfg)
		plan.workspace = workspace
		plan.agent = agentCfg
		plan.existing = true
		return plan, nil
	}
	if id == routing.DefaultAgentID {
		plan.workspace = expandHomePath(cfg.Agents.Defaults.Workspace)
		return plan, nil
	}
	plan.workspace = derivedAgentWorkspace(cfg, id)
	return plan, nil
}

func updateImportedAgentConfig(
	cfg *config.Config,
	plan *profileImportPlan,
	agentID, name, emoji string,
) {
	if name == "" {
		name = plan.preview.Identity.Name
	}
	avatar := config.AgentAvatar{Emoji: emoji}
	if plan.existing {
		for i := range cfg.Agents.List {
			agentCfg := &cfg.Agents.List[i]
			if routing.NormalizeAgentID(strings.TrimSpace(agentCfg.ID)) != routing.NormalizeAgentID(agentID) {
				continue
			}
			if name != "" {
				agentCfg.Name = name
			}
			if avatar.Emoji != "" {
				if agentCfg.Avatar == nil {
					agentCfg.Avatar = &config.AgentAvatar{}
				}
				agentCfg.Avatar.Emoji = avatar.Emoji
			}
			return
		}
		return
	}

	agentCfg := config.AgentConfig{
		ID:      agentID,
		Name:    name,
		Default: len(cfg.Agents.List) == 0,
	}
	if avatar.Emoji != "" {
		agentCfg.Avatar = &avatar
	}
	if items, err := buildSkillSupportItemsForWorkspace(cfg, plan.workspace); err == nil {
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.Name)
		}
		if len(names) > 0 {
			agentCfg.Skills = withLockedAgentSkill(normalizeStringList(names, maxAgentSkills))
		}
	}
	cfg.Agents.List = append(cfg.Agents.List, agentCfg)
}

func profileConflicts(workspace string, targets map[string][]byte) []string {
	conflicts := make([]string, 0)
	for _, target := range []string{"AGENT.md", "SOUL.md", "IDENTITY.md"} {
		if _, ok := targets[target]; !ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(workspace, target)); err == nil {
			conflicts = append(conflicts, target)
		}
	}
	return conflicts
}

func backupProfileTarget(agentID, workspace, target string) error {
	current, err := os.ReadFile(filepath.Join(workspace, target))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	dir := filepath.Join(
		config.GetHome(),
		"backups",
		"agent-imports",
		safeAgentFileBackupName(agentID),
		time.Now().UTC().Format("20060102-150405"),
	)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, target), current, 0o644)
}

func readProfileUpload(r *http.Request) (map[string][]byte, error) {
	if err := r.ParseMultipartForm(maxProfileZipBytes + 5<<20); err != nil {
		return nil, fmt.Errorf("expected a folder or ZIP upload up to 20 MB: %w", err)
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	} else {
		return nil, errors.New("expected a multipart profile upload")
	}
	if headers := r.MultipartForm.File["zip"]; len(headers) > 0 {
		return readProfileZip(headers[0])
	}

	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		return nil, errors.New("no profile files were uploaded")
	}
	if len(headers) > maxProfileEntries {
		return nil, fmt.Errorf("too many files (max %d)", maxProfileEntries)
	}
	paths := r.MultipartForm.Value["paths"]
	files := make(map[string][]byte, len(headers))
	var total int64
	for i, header := range headers {
		name := header.Filename
		if i < len(paths) && strings.TrimSpace(paths[i]) != "" {
			name = paths[i]
		}
		clean, err := cleanProfilePath(name)
		if err != nil {
			return nil, err
		}
		if clean == "" {
			continue
		}
		file, err := header.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(file, maxProfileFileBytes+1))
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > maxProfileFileBytes {
			return nil, fmt.Errorf("%s is larger than 5 MB", clean)
		}
		total += int64(len(data))
		if total > maxProfileTotalBytes {
			return nil, errors.New("profile exceeds the 20 MB limit")
		}
		files[clean] = data
	}
	return files, nil
}

func readProfileZip(header *multipart.FileHeader) (map[string][]byte, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxProfileZipBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxProfileZipBytes {
		return nil, errors.New("ZIP is larger than 20 MB")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid ZIP: %w", err)
	}
	if len(reader.File) > maxProfileEntries {
		return nil, fmt.Errorf("ZIP contains too many entries (max %d)", maxProfileEntries)
	}
	files := make(map[string][]byte, len(reader.File))
	var total int64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		clean, err := cleanProfilePath(entry.Name)
		if err != nil {
			continue
		}
		if clean == "" || isIgnoredProfilePath(clean) {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(io.LimitReader(rc, maxProfileFileBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if len(content) > maxProfileFileBytes {
			return nil, fmt.Errorf("%s is larger than 5 MB", clean)
		}
		total += int64(len(content))
		if total > maxProfileTotalBytes {
			return nil, errors.New("ZIP exceeds the 20 MB extracted limit")
		}
		files[clean] = content
	}
	return files, nil
}

func cleanProfilePath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	if value == "" {
		return "", nil
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe path %q", value)
	}
	if strings.Contains(clean, ":") {
		return "", fmt.Errorf("unsafe path %q", value)
	}
	return clean, nil
}

func isIgnoredProfilePath(value string) bool {
	lower := strings.ToLower(value)
	for _, part := range strings.Split(lower, "/") {
		if part == "adapters" || part == "__macosx" {
			return true
		}
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return strings.HasSuffix(lower, ".ds_store")
}

func buildProfilePreview(files map[string][]byte) (*profilePreview, error) {
	root, err := profileRoot(files)
	if err != nil {
		return nil, err
	}
	find := func(base string) (string, []byte, bool) {
		candidate := path.Join(root, base)
		if data, ok := files[candidate]; ok {
			return candidate, data, true
		}
		// Case-insensitive fallback.
		for filePath, data := range files {
			if strings.EqualFold(filePath, candidate) {
				return filePath, data, true
			}
		}
		return "", nil, false
	}

	preview := &profilePreview{Root: root, targets: make(map[string][]byte), Ignored: []string{}}
	targets := make(map[string]string)
	if source, data, ok := find("AGENTS.md"); ok {
		preview.targets["AGENT.md"] = data
		targets[source] = "AGENT.md"
	} else if source, data, ok := find("AGENT.md"); ok {
		preview.targets["AGENT.md"] = data
		targets[source] = "AGENT.md"
	} else if source, data, ok := find("COMBINED.md"); ok {
		preview.targets["AGENT.md"] = data
		targets[source] = "AGENT.md"
		preview.Combined = true
	} else {
		return nil, errors.New("profile needs AGENTS.md, AGENT.md or COMBINED.md")
	}

	if source, data, ok := find("SOUL.md"); ok {
		preview.targets["SOUL.md"] = data
		targets[source] = "SOUL.md"
	}
	var identityData []byte
	if source, data, ok := find("IDENTITY.md"); ok {
		preview.targets["IDENTITY.md"] = data
		targets[source] = "IDENTITY.md"
		identityData = data
	}
	if identityData != nil {
		preview.Identity = parseProfileIdentity(string(identityData))
	}

	for _, target := range []string{"AGENT.md", "SOUL.md", "IDENTITY.md"} {
		if data, ok := preview.targets[target]; ok {
			preview.Files = append(preview.Files, profileFilePreview{
				Source:  sourceForTarget(targets, target),
				Target:  target,
				Size:    len(data),
				Preview: truncatePreview(string(data), profilePreviewRunes),
			})
		}
	}

	preview.SuggestedID = suggestedProfileID(root, preview.Identity.Name)
	preview.Ignored, preview.IgnoredCount = ignoredProfileEntries(files, targets)
	if preview.Combined {
		preview.Notes = append(preview.Notes,
			"AGENTS.md was not found; COMBINED.md will be used as AGENT.md.")
	}
	if preview.Identity.Emoji == "" {
		preview.Notes = append(preview.Notes,
			"No emoji found in IDENTITY.md; you can pick one before importing.")
	}
	return preview, nil
}

func sourceForTarget(targets map[string]string, target string) string {
	for source, mapped := range targets {
		if mapped == target {
			return source
		}
	}
	return ""
}

func profileRoot(files map[string][]byte) (string, error) {
	type dirScore struct {
		dir   string
		score int
		count int
	}
	scores := map[string]*dirScore{}
	for filePath := range files {
		dir := path.Dir(filePath)
		if dir == "." {
			dir = ""
		}
		entry := scores[dir]
		if entry == nil {
			entry = &dirScore{dir: dir}
			scores[dir] = entry
		}
		entry.count++
		switch strings.ToLower(path.Base(filePath)) {
		case "agents.md":
			entry.score += 10
		case "agent.md":
			entry.score += 9
		case "combined.md":
			entry.score += 8
		case "soul.md":
			entry.score += 3
		case "identity.md":
			entry.score += 2
		}
	}
	if len(scores) == 0 {
		return "", errors.New("profile is empty")
	}
	best := make([]*dirScore, 0, len(scores))
	for _, entry := range scores {
		best = append(best, entry)
	}
	sort.Slice(best, func(i, j int) bool {
		if best[i].score != best[j].score {
			return best[i].score > best[j].score
		}
		depthI := strings.Count(best[i].dir, "/")
		depthJ := strings.Count(best[j].dir, "/")
		if depthI != depthJ {
			return depthI < depthJ
		}
		return best[i].dir < best[j].dir
	})
	if best[0].score == 0 {
		return "", errors.New("no AGENTS.md, AGENT.md or COMBINED.md found")
	}
	return best[0].dir, nil
}

func parseProfileIdentity(content string) profileIdentity {
	identity := profileIdentity{}
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	frontmatterDone := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "---") && !frontmatterDone {
			if inFrontmatter {
				frontmatterDone = true
			}
			inFrontmatter = !inFrontmatter
			continue
		}
		if inFrontmatter {
			key, value, ok := strings.Cut(trimmed, ":")
			if !ok {
				continue
			}
			setIdentityField(&identity, strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), `"'`))
		}
	}
	for _, match := range identityBulletPattern.FindAllStringSubmatch(content, -1) {
		setIdentityField(&identity, strings.ToLower(match[1]), strings.TrimSpace(match[2]))
	}
	return identity
}

func setIdentityField(identity *profileIdentity, key, value string) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "name":
		if identity.Name == "" {
			identity.Name = value
		}
	case "emoji":
		if identity.Emoji == "" {
			identity.Emoji = value
		}
	case "vibe":
		if identity.Vibe == "" {
			identity.Vibe = value
		}
	}
}

func suggestedProfileID(root, name string) string {
	for _, candidate := range []string{path.Base(root), name} {
		if id := slugifyAgentID(candidate); id != "" {
			return id
		}
	}
	return "agent"
}

func slugifyAgentID(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(builder.String(), "-")
	if id == "" {
		return ""
	}
	if id[0] < 'a' || id[0] > 'z' {
		if id[0] < '0' || id[0] > '9' {
			id = "a" + id
		}
	}
	if len(id) > 64 {
		id = id[:64]
	}
	return id
}

func ignoredProfileEntries(files map[string][]byte, mapped map[string]string) ([]string, int) {
	names := make([]string, 0)
	for filePath := range files {
		if _, ok := mapped[filePath]; ok {
			continue
		}
		// Redundant alternative profile files (for example COMBINED.md when
		// AGENTS.md exists) are not interesting to the user.
		switch strings.ToLower(path.Base(filePath)) {
		case "agents.md", "agent.md", "combined.md", "soul.md", "identity.md":
			continue
		}
		names = append(names, filePath)
	}
	sort.Strings(names)
	count := len(names)
	if len(names) > maxIgnoredList {
		names = names[:maxIgnoredList]
	}
	return names, count
}

func truncatePreview(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
