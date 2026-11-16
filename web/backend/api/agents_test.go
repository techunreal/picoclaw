package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func agentsTestMux(t *testing.T, configPath string) *http.ServeMux {
	t.Helper()
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func decodeAgentsResponse(t *testing.T, rec *httptest.ResponseRecorder) agentsResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp agentsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return resp
}

func validAgent(t *testing.T, id string, isDefault bool) config.AgentConfig {
	t.Helper()
	return config.AgentConfig{
		ID:        id,
		Name:      "Agent " + id,
		Default:   isDefault,
		Workspace: t.TempDir(),
		Model:     &config.AgentModelConfig{Primary: "custom-default"},
		Skills:    []string{"weather"},
		Subagents: &config.SubagentsConfig{AllowAgents: []string{"beta"}},
	}
}

func TestListAgents_ImplicitMain(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	agentsTestMux(t, configPath).ServeHTTP(rec, req)

	resp := decodeAgentsResponse(t, rec)
	if !resp.Implicit {
		t.Fatal("implicit = false, want true for an empty agents.list")
	}
	if len(resp.Agents) != 0 {
		t.Fatalf("agents = %d, want 0", len(resp.Agents))
	}
	if resp.Defaults.ModelName != "custom-default" {
		t.Fatalf("default model = %q, want custom-default", resp.Defaults.ModelName)
	}
}

func TestEffectiveAgentWorkspace(t *testing.T) {
	root := t.TempDir()
	defaultsWorkspace := filepath.Join(root, "workspace")
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = defaultsWorkspace

	tests := []struct {
		name       string
		agent      config.AgentConfig
		want       string
		wantSource string
	}{
		{
			name:       "main keeps the shared workspace",
			agent:      config.AgentConfig{ID: "main", Default: true},
			want:       defaultsWorkspace,
			wantSource: "default",
		},
		{
			name:       "named agent gets agents directory",
			agent:      config.AgentConfig{ID: "alpha"},
			want:       filepath.Join(root, "agents", "alpha"),
			wantSource: "derived",
		},
		{
			name:       "default flag does not share the workspace",
			agent:      config.AgentConfig{ID: "alpha", Default: true},
			want:       filepath.Join(root, "agents", "alpha"),
			wantSource: "derived",
		},
		{
			name:       "explicit workspace wins",
			agent:      config.AgentConfig{ID: "alpha", Workspace: filepath.Join(root, "custom")},
			want:       filepath.Join(root, "custom"),
			wantSource: "config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, source := effectiveAgentWorkspace(cfg, tt.agent)
			if got != tt.want {
				t.Fatalf("workspace = %q, want %q", got, tt.want)
			}
			if source != tt.wantSource {
				t.Fatalf("source = %q, want %q", source, tt.wantSource)
			}
		})
	}
}

func TestUpdateAgents_AllowsNoDefault(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	body, err := json.Marshal(agentsUpdateRequest{Agents: []config.AgentConfig{
		{ID: "alpha", Workspace: t.TempDir()},
		{ID: "beta", Workspace: t.TempDir()},
	}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	agentsTestMux(t, configPath).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	for _, agent := range cfg.Agents.List {
		if agent.Default {
			t.Fatalf("agent %q became default unexpectedly", agent.ID)
		}
	}
}

func TestUpdateAgents_SeedsNewAgentWorkspace(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	root := t.TempDir()
	defaultsWorkspace := filepath.Join(root, "workspace")
	skillDir := filepath.Join(defaultsWorkspace, "skills", "demo-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(skillDir) error = %v", err)
	}
	skillContent := "---\nname: demo-skill\ndescription: demo\n---\n# demo\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	toolsDir := filepath.Join(defaultsWorkspace, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(toolsDir) error = %v", err)
	}
	officePath := filepath.Join(toolsDir, "office_cli.py")
	if err := os.WriteFile(officePath, []byte("# office helper\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(office_cli.py) error = %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Agents.Defaults.Workspace = defaultsWorkspace
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	body, err := json.Marshal(agentsUpdateRequest{Agents: []config.AgentConfig{
		{ID: "alpha", Default: true},
	}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	agentsTestMux(t, configPath).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	agentWorkspace := filepath.Join(root, "agents", "alpha")
	if _, err := os.Stat(filepath.Join(agentWorkspace, "skills", "demo-skill", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("standard skills must not be copied into the agent workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(agentWorkspace, "tools", "office_cli.py")); err != nil {
		t.Fatalf("seeded office helper missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(defaultsWorkspace, "skills", "demo-skill", "SKILL.md")); err != nil {
		t.Fatalf("default workspace skill changed: %v", err)
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	items, err := buildSkillSupportItemsForWorkspace(updated, agentWorkspace)
	if err != nil {
		t.Fatalf("buildSkillSupportItemsForWorkspace() error = %v", err)
	}
	found := false
	for _, item := range items {
		if item.Name == "demo-skill" && item.Source == "shared" {
			found = true
		}
	}
	if !found {
		t.Fatalf("agent does not see the shared skill: %#v", items)
	}
}

func TestUpdateAgents_KeepsLockedSkill(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	mux := agentsTestMux(t, configPath)
	put := func(t *testing.T, agent config.AgentConfig) {
		t.Helper()
		body, err := json.Marshal(agentsUpdateRequest{Agents: []config.AgentConfig{agent}})
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/agents", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
		}
	}

	put(t, config.AgentConfig{
		ID:        "alpha",
		Default:   true,
		Workspace: t.TempDir(),
		Skills:    []string{"weather"},
	})
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := cfg.Agents.List[0].Skills; len(got) != 2 || got[0] != lockedAgentSkill || got[1] != "weather" {
		t.Fatalf("skills = %#v, want [%s weather]", got, lockedAgentSkill)
	}

	put(t, config.AgentConfig{
		ID:        "alpha",
		Default:   true,
		Workspace: cfg.Agents.List[0].Workspace,
	})
	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Agents.List[0].Skills != nil {
		t.Fatalf("legacy nil skills became %#v", cfg.Agents.List[0].Skills)
	}
}

func TestListAgents_AssignsFallbackEmoji(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Agents.List = []config.AgentConfig{{
		ID:        "alpha",
		Default:   true,
		Workspace: t.TempDir(),
	}}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	rec := httptest.NewRecorder()
	agentsTestMux(t, configPath).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	resp := decodeAgentsResponse(t, rec)
	if len(resp.Agents) != 1 {
		t.Fatalf("agents = %d, want 1", len(resp.Agents))
	}
	if resp.Agents[0].Avatar == nil || resp.Agents[0].Avatar.Emoji == "" {
		t.Fatalf("fallback emoji missing: %#v", resp.Agents[0].Avatar)
	}
	if resp.Agents[0].Avatar.Emoji != autoAgentEmoji("alpha") {
		t.Fatalf("emoji = %q, want stable fallback", resp.Agents[0].Avatar.Emoji)
	}
}

func TestAgentAvatarLifecycle(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	workspace := t.TempDir()
	body, err := json.Marshal(agentsUpdateRequest{Agents: []config.AgentConfig{{
		ID:        "alpha",
		Default:   true,
		Workspace: workspace,
		Avatar:    &config.AgentAvatar{Emoji: "🦉"},
	}}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	mux := agentsTestMux(t, configPath)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	upload := func(t *testing.T, payload []byte) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, err := writer.CreateFormFile("file", "avatar.png")
		if err != nil {
			t.Fatalf("CreateFormFile() error = %v", err)
		}
		if _, err := part.Write(payload); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/agents/alpha/avatar", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		mux.ServeHTTP(rec, req)
		return rec
	}

	pngBytes, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
	)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}

	rec = upload(t, pngBytes)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Agents.List[0].Avatar == nil || cfg.Agents.List[0].Avatar.Image != "avatar.png" {
		t.Fatalf("avatar not saved: %#v", cfg.Agents.List[0].Avatar)
	}
	if _, err := os.Stat(filepath.Join(workspace, "avatar", "avatar.png")); err != nil {
		t.Fatalf("avatar file missing: %v", err)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agents/alpha/avatar", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", contentType)
	}
	if !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatal("served avatar differs from upload")
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	resp := decodeAgentsResponse(t, rec)
	if resp.Agents[0].Avatar == nil || resp.Agents[0].Avatar.Image != "avatar.png" {
		t.Fatalf("avatar missing from list response: %#v", resp.Agents[0].Avatar)
	}

	rec = upload(t, []byte("not an image"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid upload status = %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/agents/alpha/avatar", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "avatar", "avatar.png")); !os.IsNotExist(err) {
		t.Fatalf("avatar file still exists: %v", err)
	}
	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Agents.List[0].Avatar == nil || cfg.Agents.List[0].Avatar.Image != "" || cfg.Agents.List[0].Avatar.Emoji != "🦉" {
		t.Fatalf("emoji should survive image deletion: %#v", cfg.Agents.List[0].Avatar)
	}
}

func TestAgentAvatar_MainImplicit(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	workspace := cfg.WorkspacePath()

	pngBytes, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
	)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "avatar.png")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(pngBytes); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	mux := agentsTestMux(t, configPath)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/main/avatar", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, "avatar", "avatar.png")); err != nil {
		t.Fatalf("avatar file missing: %v", err)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agents/main/avatar", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateAgents_AvatarValidation(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	tests := []struct {
		name   string
		avatar *config.AgentAvatar
	}{
		{name: "emoji too long", avatar: &config.AgentAvatar{Emoji: strings.Repeat("🦉", maxAvatarEmoji+1)}},
		{name: "image path", avatar: &config.AgentAvatar{Image: "../avatar.png"}},
		{name: "image extension", avatar: &config.AgentAvatar{Image: "avatar.svg"}},
	}

	mux := agentsTestMux(t, configPath)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(agentsUpdateRequest{Agents: []config.AgentConfig{{
				ID:        "alpha",
				Default:   true,
				Workspace: t.TempDir(),
				Avatar:    tt.avatar,
			}}})
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/agents", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUpdateAgents_SavesValidConfig(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	alpha := validAgent(t, "alpha", true)
	alpha.Workspace = filepath.Join(t.TempDir(), "alpha-workspace")
	beta := validAgent(t, "beta", false)
	beta.Subagents = &config.SubagentsConfig{AllowAgents: []string{"*"}}

	body, err := json.Marshal(agentsUpdateRequest{
		Agents: []config.AgentConfig{alpha, beta},
		Dispatch: &config.DispatchConfig{Rules: []config.DispatchRule{{
			Name:  "telegram to beta",
			Agent: "beta",
			When:  config.DispatchSelector{Channel: "telegram"},
		}}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	mux := agentsTestMux(t, configPath)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(updated.Agents.List) != 2 {
		t.Fatalf("agents = %d, want 2", len(updated.Agents.List))
	}
	if updated.Agents.List[0].ID != "alpha" || !updated.Agents.List[0].Default {
		t.Fatalf("unexpected first agent: %#v", updated.Agents.List[0])
	}
	if updated.Agents.Dispatch == nil || len(updated.Agents.Dispatch.Rules) != 1 {
		t.Fatalf("dispatch rules not saved: %#v", updated.Agents.Dispatch)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	mux.ServeHTTP(rec, req)
	resp := decodeAgentsResponse(t, rec)
	if resp.Implicit || len(resp.Agents) != 2 {
		t.Fatalf("unexpected response: implicit=%v agents=%d", resp.Implicit, len(resp.Agents))
	}
	if resp.Agents[0].EffectiveWorkspace == "" {
		t.Fatal("effective workspace is empty")
	}
	if info, err := os.Stat(alpha.Workspace); err != nil || !info.IsDir() {
		t.Fatalf("workspace %q was not created: %v", alpha.Workspace, err)
	}
}

func TestListAgents_WarnsAboutSharedWorkspace(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	shared := t.TempDir()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Agents.List = []config.AgentConfig{
		{ID: "alpha", Default: true, Workspace: shared},
		{ID: "beta", Workspace: shared},
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	rec := httptest.NewRecorder()
	agentsTestMux(t, configPath).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	resp := decodeAgentsResponse(t, rec)
	if len(resp.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want one shared-workspace warning", resp.Warnings)
	}
}

func TestUpdateAgents_Validation(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	duplicateWorkspace := t.TempDir()

	tests := []struct {
		name string
		body agentsUpdateRequest
	}{
		{
			name: "invalid id",
			body: agentsUpdateRequest{Agents: []config.AgentConfig{{ID: "Bad ID", Default: true}}},
		},
		{
			name: "duplicate id",
			body: agentsUpdateRequest{Agents: []config.AgentConfig{
				{ID: "alpha", Default: true},
				{ID: "alpha"},
			}},
		},
		{
			name: "multiple defaults",
			body: agentsUpdateRequest{Agents: []config.AgentConfig{
				{ID: "alpha", Default: true},
				{ID: "beta", Default: true},
			}},
		},
		{
			name: "relative workspace",
			body: agentsUpdateRequest{Agents: []config.AgentConfig{
				{ID: "alpha", Default: true, Workspace: "relative/path"},
			}},
		},
		{
			name: "workspace traversal",
			body: agentsUpdateRequest{Agents: []config.AgentConfig{
				{ID: "alpha", Default: true, Workspace: t.TempDir() + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "escape"},
			}},
		},
		{
			name: "duplicate workspace",
			body: agentsUpdateRequest{Agents: []config.AgentConfig{
				{ID: "alpha", Default: true, Workspace: duplicateWorkspace},
				{ID: "beta", Workspace: duplicateWorkspace},
			}},
		},
		{
			name: "unknown model",
			body: agentsUpdateRequest{Agents: []config.AgentConfig{
				{ID: "alpha", Default: true, Model: &config.AgentModelConfig{Primary: "missing"}},
			}},
		},
		{
			name: "unknown subagent target",
			body: agentsUpdateRequest{Agents: []config.AgentConfig{
				{ID: "alpha", Default: true, Subagents: &config.SubagentsConfig{AllowAgents: []string{"ghost"}}},
			}},
		},
		{
			name: "dispatch unknown agent",
			body: agentsUpdateRequest{
				Agents: []config.AgentConfig{{ID: "alpha", Default: true}},
				Dispatch: &config.DispatchConfig{Rules: []config.DispatchRule{{
					Agent: "ghost",
					When:  config.DispatchSelector{Channel: "telegram"},
				}}},
			},
		},
		{
			name: "dispatch empty selector",
			body: agentsUpdateRequest{
				Agents: []config.AgentConfig{{ID: "alpha", Default: true}},
				Dispatch: &config.DispatchConfig{Rules: []config.DispatchRule{{
					Agent: "alpha",
				}}},
			},
		},
		{
			name: "dispatch without agents",
			body: agentsUpdateRequest{
				Dispatch: &config.DispatchConfig{Rules: []config.DispatchRule{{
					Agent: "alpha",
					When:  config.DispatchSelector{Channel: "telegram"},
				}}},
			},
		},
	}

	mux := agentsTestMux(t, configPath)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/agents", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestListSkills_ScopedToAgentWorkspace(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	alphaWS := t.TempDir()
	betaWS := t.TempDir()
	for _, item := range []struct{ ws, name string }{
		{alphaWS, "alpha-skill"},
		{betaWS, "beta-skill"},
	} {
		dir := filepath.Join(item.ws, "skills", item.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		content := "---\nname: " + item.name + "\ndescription: smoke skill\n---\n# " + item.name + "\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	body, err := json.Marshal(agentsUpdateRequest{Agents: []config.AgentConfig{
		{ID: "alpha", Default: true, Workspace: alphaWS},
		{ID: "beta", Workspace: betaWS},
	}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	mux := agentsTestMux(t, configPath)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	for agentID, both := range map[string][2]string{
		"alpha": {"alpha-skill", "beta-skill"},
		"beta":  {"beta-skill", "alpha-skill"},
	} {
		want, unwanted := both[0], both[1]
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/skills?agent="+agentID, nil)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET skills agent=%s status = %d, body = %s", agentID, rec.Code, rec.Body.String())
		}
		var resp skillSupportResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		foundWant, foundUnwanted := false, false
		for _, skill := range resp.Skills {
			if skill.Name == want {
				foundWant = true
			}
			if skill.Name == unwanted {
				foundUnwanted = true
			}
		}
		if !foundWant {
			t.Fatalf("agent %s did not list %q", agentID, want)
		}
		if foundUnwanted {
			t.Fatalf("agent %s leaked workspace skill %q", agentID, unwanted)
		}
	}
}

func TestListSkills_IncludesSharedSkills(t *testing.T) {
	configPath, cleanup := setupOAuthTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	sharedDir := filepath.Join(cfg.WorkspacePath(), "skills", "shared-skill")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := "---\nname: shared-skill\ndescription: shared skill\n---\n# shared\n"
	if err := os.WriteFile(filepath.Join(sharedDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	alphaWS := t.TempDir()
	body, err := json.Marshal(agentsUpdateRequest{Agents: []config.AgentConfig{
		{ID: "alpha", Default: true, Workspace: alphaWS},
	}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	mux := agentsTestMux(t, configPath)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/agents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/skills?agent=alpha", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET skills status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp skillSupportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	found := false
	for _, skill := range resp.Skills {
		if skill.Name == "shared-skill" && skill.Source == "shared" {
			found = true
		}
	}
	if !found {
		t.Fatalf("shared skill missing from agent list: %#v", resp.Skills)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent/files?agent=alpha", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET files status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "skill:shared:shared-skill") {
		t.Fatalf("shared skill missing from agent files: %s", rec.Body.String())
	}
}

func TestConfigSignatureChangesWithAgentsAndDispatch(t *testing.T) {
	cfg := config.DefaultConfig()
	before := computeConfigSignature(cfg)

	cfg.Agents.List = []config.AgentConfig{{ID: "alpha", Default: true}}
	withAgent := computeConfigSignature(cfg)
	if before == withAgent {
		t.Fatal("signature should change when an agent is added")
	}

	cfg.Agents.List[0].Name = "Renamed"
	if withAgent == computeConfigSignature(cfg) {
		t.Fatal("signature should change when an agent name changes")
	}

	cfg.Agents.Dispatch = &config.DispatchConfig{Rules: []config.DispatchRule{{
		Agent: "alpha",
		When:  config.DispatchSelector{Channel: "telegram"},
	}}}
	if withAgent == computeConfigSignature(cfg) {
		t.Fatal("signature should change when dispatch rules change")
	}
}
