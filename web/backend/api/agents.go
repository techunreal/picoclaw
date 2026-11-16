package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/routing"
)

const (
	maxAgents        = 50
	maxDispatchRules = 100
	maxAgentSkills   = 100
	maxAgentNameLen  = 120
	maxAvatarEmoji   = 16
	maxAvatarBytes   = 1 << 20
	// lockedAgentSkill is always present when an agent has an explicit skill
	// list. It keeps the agent able to manage PicoClaw itself and guarantees
	// that "Clear all" never produces an empty list.
	lockedAgentSkill = "picoclaw-agent"
)

var avatarImagePattern = regexp.MustCompile(`(?i)^avatar\.(png|jpe?g|webp)$`)

var agentIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type agentView struct {
	ID                 string                   `json:"id"`
	Name               string                   `json:"name,omitempty"`
	Default            bool                     `json:"default"`
	Workspace          string                   `json:"workspace,omitempty"`
	EffectiveWorkspace string                   `json:"effective_workspace"`
	WorkspaceSource    string                   `json:"workspace_source"`
	Model              *config.AgentModelConfig `json:"model,omitempty"`
	EffectiveModel     string                   `json:"effective_model,omitempty"`
	Skills             []string                 `json:"skills,omitempty"`
	Subagents          *config.SubagentsConfig  `json:"subagents,omitempty"`
	Avatar             *agentAvatarView         `json:"avatar,omitempty"`
}

type agentAvatarView struct {
	Emoji   string `json:"emoji,omitempty"`
	Image   string `json:"image,omitempty"`
	Version int64  `json:"version,omitempty"`
}

type agentsDefaultsView struct {
	Workspace string `json:"workspace"`
	Provider  string `json:"provider"`
	ModelName string `json:"model_name"`
}

type agentsResponse struct {
	Agents   []agentView            `json:"agents"`
	Defaults agentsDefaultsView     `json:"defaults"`
	Dispatch *config.DispatchConfig `json:"dispatch,omitempty"`
	Hidden   []string               `json:"hidden,omitempty"`
	Implicit bool                   `json:"implicit"`
	Warnings []string               `json:"warnings,omitempty"`
}

type agentsUpdateRequest struct {
	Agents   []config.AgentConfig   `json:"agents"`
	Dispatch *config.DispatchConfig `json:"dispatch"`
}

func (h *Handler) registerAgentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agents", h.handleListAgents)
	mux.HandleFunc("PUT /api/agents", h.handleUpdateAgents)
	mux.HandleFunc("PUT /api/agents/order", h.handleAgentOrder)
	mux.HandleFunc("PUT /api/agents/hidden", h.handleAgentHidden)
	mux.HandleFunc("GET /api/agents/{id}/avatar", h.handleGetAgentAvatar)
	mux.HandleFunc("POST /api/agents/{id}/avatar", h.handleUploadAgentAvatar)
	mux.HandleFunc("DELETE /api/agents/{id}/avatar", h.handleDeleteAgentAvatar)
	h.registerAgentImportRoutes(mux)
}

func (h *Handler) handleListAgents(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(buildAgentsResponse(cfg))
}

func (h *Handler) handleUpdateAgents(w http.ResponseWriter, r *http.Request) {
	var req agentsUpdateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}

	normalized, validationErrs := validateAgentsConfig(cfg, req.Agents, req.Dispatch)
	if len(validationErrs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":   "invalid_agents",
			"error":  strings.Join(validationErrs, "; "),
			"errors": validationErrs,
		})
		return
	}

	previousList := append([]config.AgentConfig(nil), cfg.Agents.List...)

	cfg.Agents.List = normalized
	cfg.Agents.Dispatch = req.Dispatch
	if cfg.Agents.Dispatch != nil && len(cfg.Agents.Dispatch.Rules) == 0 {
		cfg.Agents.Dispatch = nil
	}

	previousAgents := make(map[string]struct{}, len(previousList))
	for _, agentCfg := range previousList {
		previousAgents[routing.NormalizeAgentID(strings.TrimSpace(agentCfg.ID))] = struct{}{}
	}

	for _, agentCfg := range normalized {
		workspace, source := effectiveAgentWorkspace(cfg, agentCfg)
		if strings.TrimSpace(workspace) == "" {
			continue
		}
		if err := os.MkdirAll(workspace, 0o755); err != nil {
			writeAgentsAPIError(w, http.StatusBadRequest, "workspace_create_failed", err.Error())
			return
		}
		if source == "default" {
			continue
		}
		if _, existed := previousAgents[routing.NormalizeAgentID(strings.TrimSpace(agentCfg.ID))]; existed {
			continue
		}
		if err := seedAgentWorkspace(cfg, workspace); err != nil {
			writeAgentsAPIError(w, http.StatusInternalServerError, "workspace_seed_failed", err.Error())
			return
		}
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_save_failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":           "ok",
		"restart_required": true,
	})
}

func (h *Handler) handleAgentOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}
	if len(req.IDs) != len(cfg.Agents.List) {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_order",
			"order must include every agent exactly once")
		return
	}
	byID := make(map[string]config.AgentConfig, len(cfg.Agents.List))
	for _, agentCfg := range cfg.Agents.List {
		byID[routing.NormalizeAgentID(strings.TrimSpace(agentCfg.ID))] = agentCfg
	}
	reordered := make([]config.AgentConfig, 0, len(req.IDs))
	seen := make(map[string]struct{}, len(req.IDs))
	for _, rawID := range req.IDs {
		id := routing.NormalizeAgentID(strings.TrimSpace(rawID))
		agentCfg, ok := byID[id]
		if !ok {
			writeAgentsAPIError(w, http.StatusBadRequest, "invalid_order",
				fmt.Sprintf("unknown agent %q", rawID))
			return
		}
		if _, duplicate := seen[id]; duplicate {
			writeAgentsAPIError(w, http.StatusBadRequest, "invalid_order",
				fmt.Sprintf("duplicate agent %q", rawID))
			return
		}
		seen[id] = struct{}{}
		reordered = append(reordered, agentCfg)
	}
	cfg.Agents.List = reordered
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *Handler) handleAgentHidden(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hidden []string `json:"hidden"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}
	known := map[string]struct{}{routing.DefaultAgentID: {}}
	for _, agentCfg := range cfg.Agents.List {
		known[routing.NormalizeAgentID(strings.TrimSpace(agentCfg.ID))] = struct{}{}
	}
	hidden := make([]string, 0, len(req.Hidden))
	seen := make(map[string]struct{}, len(req.Hidden))
	for _, rawID := range req.Hidden {
		id := routing.NormalizeAgentID(strings.TrimSpace(rawID))
		if _, ok := known[id]; !ok {
			writeAgentsAPIError(w, http.StatusBadRequest, "unknown_agent",
				fmt.Sprintf("unknown agent %q", rawID))
			return
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		hidden = append(hidden, id)
	}
	cfg.Agents.Hidden = hidden
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "hidden": hidden})
}

func buildAgentsResponse(cfg *config.Config) agentsResponse {
	list := cfg.Agents.List
	agents := make([]agentView, 0, len(list))
	for _, agentCfg := range list {
		workspace, source := effectiveAgentWorkspace(cfg, agentCfg)
		view := agentView{
			ID:                 agentCfg.ID,
			Name:               agentCfg.Name,
			Default:            agentCfg.Default,
			Workspace:          agentCfg.Workspace,
			EffectiveWorkspace: workspace,
			WorkspaceSource:    source,
			Model:              agentCfg.Model,
			EffectiveModel:     effectiveAgentModel(cfg, agentCfg),
			Skills:             agentCfg.Skills,
			Subagents:          agentCfg.Subagents,
			Avatar:             resolveAgentAvatarView(agentCfg, workspace),
		}
		agents = append(agents, view)
	}

	return agentsResponse{
		Agents: agents,
		Defaults: agentsDefaultsView{
			Workspace: expandHomePath(cfg.Agents.Defaults.Workspace),
			Provider:  cfg.Agents.Defaults.Provider,
			ModelName: cfg.Agents.Defaults.GetModelName(),
		},
		Dispatch: cfg.Agents.Dispatch,
		Hidden:   append([]string(nil), cfg.Agents.Hidden...),
		Implicit: len(list) == 0,
		Warnings: agentWorkspaceWarnings(cfg),
	}
}

func agentWorkspaceWarnings(cfg *config.Config) []string {
	seen := map[string]string{}
	warnings := []string{}
	for _, agentCfg := range cfg.Agents.List {
		workspace, _ := effectiveAgentWorkspace(cfg, agentCfg)
		key := workspaceKey(workspace)
		if prev, ok := seen[key]; ok {
			warnings = append(warnings, fmt.Sprintf(
				"agents %q and %q share the same workspace (%s); they will share AGENT.md, memory and skills",
				prev, agentCfg.ID, workspace,
			))
			continue
		}
		seen[key] = agentCfg.ID
	}
	return warnings
}

func (h *Handler) handleGetAgentAvatar(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}
	workspace, err := resolveAgentWorkspaceForID(cfg, r.PathValue("id"))
	if err != nil {
		writeAgentsAPIError(w, http.StatusNotFound, "agent_not_found", err.Error())
		return
	}
	path, ok := findAgentAvatarFile(workspace)
	if !ok {
		writeAgentsAPIError(w, http.StatusNotFound, "avatar_not_found", "no avatar image for this agent")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

func (h *Handler) handleUploadAgentAvatar(w http.ResponseWriter, r *http.Request) {
	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}
	agentID := r.PathValue("id")
	workspace, err := resolveAgentWorkspaceForID(cfg, agentID)
	if err != nil {
		writeAgentsAPIError(w, http.StatusNotFound, "agent_not_found", err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+512*1024)
	if err := r.ParseMultipartForm(maxAvatarBytes); err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_upload", "expected a multipart image upload up to 1 MB")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "missing_file", "form field \"file\" is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil {
		writeAgentsAPIError(w, http.StatusBadRequest, "invalid_upload", err.Error())
		return
	}
	if len(data) == 0 {
		writeAgentsAPIError(w, http.StatusBadRequest, "empty_file", "image is empty")
		return
	}
	if len(data) > maxAvatarBytes {
		writeAgentsAPIError(w, http.StatusBadRequest, "avatar_too_large", "image must be 1 MB or smaller")
		return
	}
	ext, ok := avatarExtension(http.DetectContentType(data))
	if !ok {
		writeAgentsAPIError(w, http.StatusBadRequest, "unsupported_image", "use a PNG, JPEG or WebP image")
		return
	}

	dir := agentAvatarDir(workspace)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "avatar_save_failed", err.Error())
		return
	}
	removeAgentAvatarFiles(dir)
	name := "avatar." + ext
	if err := writeFileAtomic(filepath.Join(dir, name), data, 0o644); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "avatar_save_failed", err.Error())
		return
	}

	updateAgentAvatarImage(cfg, agentID, name)
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_save_failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	version := int64(0)
	if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
		version = info.ModTime().UnixNano()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"avatar": agentAvatarView{
			Emoji:   agentAvatarEmoji(cfg, agentID),
			Image:   name,
			Version: version,
		},
	})
}

func (h *Handler) handleDeleteAgentAvatar(w http.ResponseWriter, r *http.Request) {
	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_load_failed", err.Error())
		return
	}
	agentID := r.PathValue("id")
	workspace, err := resolveAgentWorkspaceForID(cfg, agentID)
	if err != nil {
		writeAgentsAPIError(w, http.StatusNotFound, "agent_not_found", err.Error())
		return
	}
	removeAgentAvatarFiles(agentAvatarDir(workspace))
	updateAgentAvatarImage(cfg, agentID, "")
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeAgentsAPIError(w, http.StatusInternalServerError, "config_save_failed", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func agentAvatarDir(workspace string) string {
	return filepath.Join(workspace, "avatar")
}

func findAgentAvatarFile(workspace string) (string, bool) {
	for _, name := range []string{"avatar.png", "avatar.jpg", "avatar.jpeg", "avatar.webp"} {
		path := filepath.Join(agentAvatarDir(workspace), name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, true
		}
	}
	return "", false
}

func removeAgentAvatarFiles(dir string) {
	for _, name := range []string{"avatar.png", "avatar.jpg", "avatar.jpeg", "avatar.webp"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func avatarExtension(contentType string) (string, bool) {
	switch contentType {
	case "image/png":
		return "png", true
	case "image/jpeg":
		return "jpg", true
	case "image/webp":
		return "webp", true
	default:
		return "", false
	}
}

func resolveAgentAvatarView(agentCfg config.AgentConfig, workspace string) *agentAvatarView {
	view := agentAvatarView{}
	if agentCfg.Avatar != nil {
		view.Emoji = agentCfg.Avatar.Emoji
	}
	if path, ok := findAgentAvatarFile(workspace); ok {
		view.Image = filepath.Base(path)
		if info, err := os.Stat(path); err == nil {
			view.Version = info.ModTime().UnixNano()
		}
	}
	if view.Image == "" && view.Emoji == "" {
		// No image or configured emoji: assign a stable, pseudo-random emoji so
		// every agent has a visible avatar.
		view.Emoji = autoAgentEmoji(agentCfg.ID)
	}
	return &view
}

var autoAgentEmojis = []string{
	"🦉", "🦊", "🐼", "🐸", "🐙", "🦁", "🐧", "🦄",
	"🐻", "🐨", "🦋", "🐬", "🐢", "🦖", "🐳", "🦜",
}

func autoAgentEmoji(id string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.ToLower(strings.TrimSpace(id))))
	return autoAgentEmojis[int(hasher.Sum32())%len(autoAgentEmojis)]
}

func updateAgentAvatarImage(cfg *config.Config, agentID, image string) {
	id := routing.NormalizeAgentID(strings.TrimSpace(agentID))
	for i := range cfg.Agents.List {
		if routing.NormalizeAgentID(strings.TrimSpace(cfg.Agents.List[i].ID)) != id {
			continue
		}
		agent := &cfg.Agents.List[i]
		if agent.Avatar == nil {
			agent.Avatar = &config.AgentAvatar{}
		}
		agent.Avatar.Image = image
		if agent.Avatar.Image == "" && agent.Avatar.Emoji == "" {
			agent.Avatar = nil
		}
		return
	}
}

func agentAvatarEmoji(cfg *config.Config, agentID string) string {
	id := routing.NormalizeAgentID(strings.TrimSpace(agentID))
	for i := range cfg.Agents.List {
		if routing.NormalizeAgentID(strings.TrimSpace(cfg.Agents.List[i].ID)) == id && cfg.Agents.List[i].Avatar != nil {
			return cfg.Agents.List[i].Avatar.Emoji
		}
	}
	return ""
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".avatar-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func effectiveAgentWorkspace(cfg *config.Config, agentCfg config.AgentConfig) (string, string) {
	if ws := strings.TrimSpace(agentCfg.Workspace); ws != "" {
		return expandHomePath(ws), "config"
	}
	id := routing.NormalizeAgentID(agentCfg.ID)
	// The default ("main") agent keeps the shared compatibility workspace.
	// Every other agent gets its own isolated workspace under agents/<id>,
	// regardless of the default flag.
	if id == routing.DefaultAgentID {
		return expandHomePath(cfg.Agents.Defaults.Workspace), "default"
	}
	return derivedAgentWorkspace(cfg, id), "derived"
}

// derivedAgentWorkspace returns ~/.picoclaw/agents/<id> for a named agent.
// It is derived from the defaults workspace (<home>/workspace) so custom
// homes keep working.
func derivedAgentWorkspace(cfg *config.Config, id string) string {
	base := expandHomePath(cfg.Agents.Defaults.Workspace)
	return filepath.Clean(filepath.Join(base, "..", "agents", id))
}

// seedAgentWorkspace copies the Office helper into a newly created agent
// workspace. Standard skills are not copied: they live in the default
// workspace's skills folder and are shared read-only by every agent.
func seedAgentWorkspace(cfg *config.Config, workspace string) error {
	defaultsWorkspace := expandHomePath(cfg.Agents.Defaults.Workspace)
	if defaultsWorkspace == "" || filepath.Clean(defaultsWorkspace) == filepath.Clean(workspace) {
		return nil
	}
	return copyMissingFile(
		filepath.Join(defaultsWorkspace, "tools", "office_cli.py"),
		filepath.Join(workspace, "tools", "office_cli.py"),
	)
}

func copyMissingFile(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info, err := os.Stat(src); err != nil || info.IsDir() {
		return nil
	}
	return copyFileContents(src, dst)
}

func copyFileContents(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func effectiveAgentModel(cfg *config.Config, agentCfg config.AgentConfig) string {
	if agentCfg.Model != nil && strings.TrimSpace(agentCfg.Model.Primary) != "" {
		return strings.TrimSpace(agentCfg.Model.Primary)
	}
	return cfg.Agents.Defaults.GetModelName()
}

// resolveAgentWorkspaceForID resolves the workspace for an agent ID using the
// same rules as runtime resolution. An empty list resolves the implicit main
// agent to the defaults workspace.
func resolveAgentWorkspaceForID(cfg *config.Config, agentID string) (string, error) {
	id := routing.NormalizeAgentID(strings.TrimSpace(agentID))
	if id == "" {
		return "", fmt.Errorf("agent id is required")
	}
	if len(cfg.Agents.List) == 0 {
		if id != routing.DefaultAgentID {
			return "", fmt.Errorf("agent %q not found", id)
		}
		return expandHomePath(cfg.Agents.Defaults.Workspace), nil
	}
	for _, agentCfg := range cfg.Agents.List {
		if routing.NormalizeAgentID(strings.TrimSpace(agentCfg.ID)) != id {
			continue
		}
		workspace, _ := effectiveAgentWorkspace(cfg, agentCfg)
		return workspace, nil
	}
	return "", fmt.Errorf("agent %q not found", id)
}

func validateAgentsConfig(
	cfg *config.Config,
	agents []config.AgentConfig,
	dispatch *config.DispatchConfig,
) ([]config.AgentConfig, []string) {
	var errs []string
	if len(agents) > maxAgents {
		errs = append(errs, fmt.Sprintf("at most %d agents are allowed", maxAgents))
	}

	modelNames := make(map[string]struct{}, len(cfg.ModelList))
	for _, m := range cfg.ModelList {
		if m == nil {
			continue
		}
		modelNames[strings.TrimSpace(m.ModelName)] = struct{}{}
	}

	normalized := make([]config.AgentConfig, 0, len(agents))
	seenIDs := map[string]string{}
	defaultCount := 0
	validIDs := map[string]struct{}{}

	for i, agentCfg := range agents {
		rawID := strings.TrimSpace(agentCfg.ID)
		if !agentIDPattern.MatchString(rawID) {
			errs = append(errs, fmt.Sprintf(
				"agent %d: id %q must match ^[a-z0-9][a-z0-9_-]{0,63}$", i+1, rawID,
			))
			continue
		}
		normID := routing.NormalizeAgentID(rawID)
		if prev, ok := seenIDs[normID]; ok {
			errs = append(errs, fmt.Sprintf(
				"agent %q conflicts with %q after normalization", rawID, prev,
			))
		} else {
			seenIDs[normID] = rawID
			validIDs[normID] = struct{}{}
		}

		agentCfg.ID = rawID
		agentCfg.Name = strings.TrimSpace(agentCfg.Name)
		if len(agentCfg.Name) > maxAgentNameLen {
			errs = append(errs, fmt.Sprintf("agent %q: name is too long", rawID))
		}
		if agentCfg.Default {
			defaultCount++
		}

		if len(agentCfg.Skills) > maxAgentSkills {
			errs = append(errs, fmt.Sprintf("agent %q: too many skills (max %d)", rawID, maxAgentSkills))
		}
		// A nil list means "all skills" and is kept for legacy configs. An
		// explicit list always keeps the locked picoclaw-agent skill.
		if agentCfg.Skills != nil {
			agentCfg.Skills = withLockedAgentSkill(normalizeStringList(agentCfg.Skills, maxAgentSkills))
		}

		if agentCfg.Avatar != nil {
			emoji := strings.TrimSpace(agentCfg.Avatar.Emoji)
			image := strings.TrimSpace(agentCfg.Avatar.Image)
			if utf8.RuneCountInString(emoji) > maxAvatarEmoji {
				errs = append(errs, fmt.Sprintf("agent %q: avatar emoji is too long", rawID))
			}
			if strings.ContainsFunc(emoji, unicode.IsControl) {
				errs = append(errs, fmt.Sprintf("agent %q: avatar emoji contains control characters", rawID))
			}
			if image != "" && !avatarImagePattern.MatchString(image) {
				errs = append(errs, fmt.Sprintf("agent %q: avatar image must be avatar.png, avatar.jpg or avatar.webp", rawID))
			}
			if emoji == "" && image == "" {
				agentCfg.Avatar = nil
			} else {
				agentCfg.Avatar = &config.AgentAvatar{Emoji: emoji, Image: image}
			}
		}

		if strings.TrimSpace(agentCfg.Workspace) != "" {
			ws, err := validateAgentWorkspace(agentCfg.Workspace)
			if err != nil {
				errs = append(errs, fmt.Sprintf("agent %q: %v", rawID, err))
			} else {
				agentCfg.Workspace = ws
			}
		}

		if agentCfg.Model != nil {
			primary := strings.TrimSpace(agentCfg.Model.Primary)
			if primary == "" {
				errs = append(errs, fmt.Sprintf("agent %q: model.primary is required when model is set", rawID))
			} else if _, ok := modelNames[primary]; !ok {
				errs = append(errs, fmt.Sprintf("agent %q: model %q is not configured", rawID, primary))
			}
			for _, fallback := range agentCfg.Model.Fallbacks {
				name := strings.TrimSpace(fallback)
				if name == "" {
					continue
				}
				if _, ok := modelNames[name]; !ok {
					errs = append(errs, fmt.Sprintf("agent %q: fallback model %q is not configured", rawID, name))
				}
			}
		}

		if agentCfg.Subagents != nil {
			agentCfg.Subagents.AllowAgents = normalizeStringList(agentCfg.Subagents.AllowAgents, maxAgents)
		}

		normalized = append(normalized, agentCfg)
	}

	if defaultCount > 1 {
		errs = append(errs, "at most one agent may be marked as default")
	}

	// Reject duplicate effective workspaces (explicit or derived).
	seenWorkspaces := map[string]string{}
	for _, agentCfg := range normalized {
		workspace, _ := effectiveAgentWorkspace(cfg, agentCfg)
		key := workspaceKey(workspace)
		if prev, ok := seenWorkspaces[key]; ok {
			errs = append(errs, fmt.Sprintf(
				"agent %q uses the same workspace as %q", agentCfg.ID, prev,
			))
		} else {
			seenWorkspaces[key] = agentCfg.ID
		}
	}

	// Validate subagent permissions and dispatch targets against the final set.
	for _, agentCfg := range normalized {
		if agentCfg.Subagents == nil {
			continue
		}
		for _, allowed := range agentCfg.Subagents.AllowAgents {
			if allowed == "*" {
				continue
			}
			if _, ok := validIDs[routing.NormalizeAgentID(allowed)]; !ok {
				errs = append(errs, fmt.Sprintf(
					"agent %q: subagent target %q is not in the agent list", agentCfg.ID, allowed,
				))
			}
		}
	}

	if dispatch != nil {
		if len(dispatch.Rules) > maxDispatchRules {
			errs = append(errs, fmt.Sprintf("at most %d dispatch rules are allowed", maxDispatchRules))
		}
		for i := range dispatch.Rules {
			rule := &dispatch.Rules[i]
			rule.Agent = strings.TrimSpace(rule.Agent)
			rule.Name = strings.TrimSpace(rule.Name)
			if rule.Agent == "" {
				errs = append(errs, fmt.Sprintf("dispatch rule %d: agent is required", i+1))
			} else if _, ok := validIDs[routing.NormalizeAgentID(rule.Agent)]; !ok {
				errs = append(errs, fmt.Sprintf("dispatch rule %d: agent %q is not in the agent list", i+1, rule.Agent))
			}
			if !dispatchSelectorHasConstraint(rule.When) {
				errs = append(errs, fmt.Sprintf("dispatch rule %d: at least one condition is required", i+1))
			}
			rule.When.Channel = strings.TrimSpace(rule.When.Channel)
			rule.When.Account = strings.TrimSpace(rule.When.Account)
			rule.When.Space = strings.TrimSpace(rule.When.Space)
			rule.When.Chat = strings.TrimSpace(rule.When.Chat)
			rule.When.Topic = strings.TrimSpace(rule.When.Topic)
			rule.When.Sender = strings.TrimSpace(rule.When.Sender)
		}
	}
	if len(agents) == 0 && dispatch != nil && len(dispatch.Rules) > 0 {
		errs = append(errs, "dispatch rules require at least one explicit agent")
	}

	return normalized, errs
}

func validateAgentWorkspace(raw string) (string, error) {
	ws := expandHomePath(strings.TrimSpace(raw))
	if ws == "" {
		return "", nil
	}
	if !filepath.IsAbs(ws) {
		return "", fmt.Errorf("workspace %q must be an absolute path", raw)
	}
	for _, part := range strings.FieldsFunc(ws, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return "", fmt.Errorf("workspace %q must not contain '..'", raw)
		}
	}
	ws = filepath.Clean(ws)
	if info, err := os.Stat(ws); err == nil && !info.IsDir() {
		return "", fmt.Errorf("workspace %q exists but is not a directory", raw)
	}
	return ws, nil
}

func workspaceKey(path string) string {
	key := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

func dispatchSelectorHasConstraint(sel config.DispatchSelector) bool {
	return sel.Channel != "" || sel.Account != "" || sel.Space != "" ||
		sel.Chat != "" || sel.Topic != "" || sel.Sender != "" || sel.Mentioned != nil
}

func withLockedAgentSkill(skills []string) []string {
	for _, skill := range skills {
		if strings.EqualFold(skill, lockedAgentSkill) {
			return skills
		}
	}
	locked := append([]string{lockedAgentSkill}, skills...)
	if len(locked) > maxAgentSkills {
		locked = locked[:maxAgentSkills]
	}
	return locked
}

func normalizeStringList(values []string, limit int) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func expandHomePath(path string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func writeAgentsAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "error": message})
}
