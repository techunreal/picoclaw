package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/routing"
)

const (
	maxAgents        = 50
	maxDispatchRules = 100
	maxAgentSkills   = 100
	maxAgentNameLen  = 120
	// lockedAgentSkill is always present when an agent has an explicit skill
	// list. It keeps the agent able to manage PicoClaw itself and guarantees
	// that "Clear all" never produces an empty list.
	lockedAgentSkill = "picoclaw-agent"
)

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

// seedAgentWorkspace copies the standard skill set and the Office helper into a
// newly created agent workspace. The default ("main") workspace is the source,
// so a fresh agent starts with the same capabilities as main without sharing
// files with it.
func seedAgentWorkspace(cfg *config.Config, workspace string) error {
	defaultsWorkspace := expandHomePath(cfg.Agents.Defaults.Workspace)
	if defaultsWorkspace == "" || filepath.Clean(defaultsWorkspace) == filepath.Clean(workspace) {
		return nil
	}
	if err := copyMissingDir(
		filepath.Join(defaultsWorkspace, "skills"),
		filepath.Join(workspace, "skills"),
	); err != nil {
		return err
	}
	return copyMissingFile(
		filepath.Join(defaultsWorkspace, "tools", "office_cli.py"),
		filepath.Join(workspace, "tools", "office_cli.py"),
	)
}

func copyMissingDir(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	srcInfo, err := os.Stat(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return nil
	}
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		return copyFileContents(path, target)
	})
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

	if len(agents) > 0 && defaultCount != 1 {
		errs = append(errs, "exactly one agent must be marked as default")
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
