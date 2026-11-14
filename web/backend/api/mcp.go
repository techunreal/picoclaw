package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/mcp"
)

type mcpServerPayload struct {
	Enabled  bool              `json:"enabled"`
	Deferred *bool             `json:"deferred,omitempty"`
	Type     string            `json:"type"`
	Command  string            `json:"command"`
	Args     []string          `json:"args"`
	Env      map[string]string `json:"env"`
	EnvFile  string            `json:"env_file"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers"`
}

type mcpServerInfo struct {
	Name string `json:"name"`
	mcpServerPayload
}

type mcpDiscoverySettings struct {
	Enabled          bool `json:"enabled"`
	TTL              int  `json:"ttl"`
	MaxSearchResults int  `json:"max_search_results"`
	UseBM25          bool `json:"use_bm25"`
	UseRegex         bool `json:"use_regex"`
}

type mcpServersResponse struct {
	Enabled   bool                 `json:"enabled"`
	Servers   []mcpServerInfo      `json:"servers"`
	Discovery mcpDiscoverySettings `json:"discovery"`
}

type mcpSettingsRequest struct {
	Enabled   bool `json:"enabled"`
	Discovery *struct {
		Enabled          bool `json:"enabled"`
		TTL              int  `json:"ttl"`
		MaxSearchResults int  `json:"max_search_results"`
		UseBM25          bool `json:"use_bm25"`
		UseRegex         bool `json:"use_regex"`
	} `json:"discovery,omitempty"`
}

type mcpTestResult struct {
	OK        bool     `json:"ok"`
	ToolCount int      `json:"tool_count"`
	Tools     []string `json:"tools,omitempty"`
	Error     string   `json:"error,omitempty"`
}

func (h *Handler) registerMCPRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/mcp/servers", h.handleListMCPServers)
	mux.HandleFunc("PUT /api/mcp/settings", h.handleUpdateMCPSettings)
	mux.HandleFunc("PUT /api/mcp/servers/{name}", h.handleUpsertMCPServer)
	mux.HandleFunc("DELETE /api/mcp/servers/{name}", h.handleDeleteMCPServer)
	mux.HandleFunc("POST /api/mcp/servers/{name}/test", h.handleTestMCPServer)
}

func (h *Handler) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeMCPError(w, http.StatusInternalServerError, "config_unreadable", "Failed to load config.")
		return
	}
	writeMCPJSON(w, http.StatusOK, buildMCPServersResponse(cfg))
}

func (h *Handler) handleUpdateMCPSettings(w http.ResponseWriter, r *http.Request) {
	var req mcpSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeMCPError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeMCPError(w, http.StatusInternalServerError, "config_unreadable", "Failed to load config.")
		return
	}

	cfg.Tools.MCP.Enabled = req.Enabled
	if req.Discovery != nil {
		cfg.Tools.MCP.Discovery.Enabled = req.Discovery.Enabled
		if req.Discovery.TTL > 0 {
			cfg.Tools.MCP.Discovery.TTL = req.Discovery.TTL
		}
		if req.Discovery.MaxSearchResults > 0 {
			cfg.Tools.MCP.Discovery.MaxSearchResults = req.Discovery.MaxSearchResults
		}
		cfg.Tools.MCP.Discovery.UseBM25 = req.Discovery.UseBM25
		cfg.Tools.MCP.Discovery.UseRegex = req.Discovery.UseRegex
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeMCPError(w, http.StatusInternalServerError, "save_failed", "Failed to save config.")
		return
	}
	writeMCPJSON(w, http.StatusOK, buildMCPServersResponse(cfg))
}

func (h *Handler) handleUpsertMCPServer(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if !validMCPServerName(name) {
		writeMCPError(w, http.StatusBadRequest, "invalid_name",
			"Server name must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,63}.")
		return
	}

	var payload mcpServerPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeMCPError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON request body.")
		return
	}
	server, err := normalizeMCPServerPayload(payload)
	if err != nil {
		writeMCPError(w, http.StatusBadRequest, "invalid_server", err.Error())
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeMCPError(w, http.StatusInternalServerError, "config_unreadable", "Failed to load config.")
		return
	}
	if cfg.Tools.MCP.Servers == nil {
		cfg.Tools.MCP.Servers = make(map[string]config.MCPServerConfig)
	}
	cfg.Tools.MCP.Servers[name] = server
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeMCPError(w, http.StatusInternalServerError, "save_failed", "Failed to save config.")
		return
	}
	writeMCPJSON(w, http.StatusOK, buildMCPServersResponse(cfg))
}

func (h *Handler) handleDeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if !validMCPServerName(name) {
		writeMCPError(w, http.StatusBadRequest, "invalid_name", "Invalid server name.")
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeMCPError(w, http.StatusInternalServerError, "config_unreadable", "Failed to load config.")
		return
	}
	if _, exists := cfg.Tools.MCP.Servers[name]; !exists {
		writeMCPError(w, http.StatusNotFound, "not_found", "MCP server not found.")
		return
	}
	delete(cfg.Tools.MCP.Servers, name)
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		writeMCPError(w, http.StatusInternalServerError, "save_failed", "Failed to save config.")
		return
	}
	writeMCPJSON(w, http.StatusOK, buildMCPServersResponse(cfg))
}

func (h *Handler) handleTestMCPServer(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if !validMCPServerName(name) {
		writeMCPError(w, http.StatusBadRequest, "invalid_name", "Invalid server name.")
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeMCPError(w, http.StatusInternalServerError, "config_unreadable", "Failed to load config.")
		return
	}
	server, ok := cfg.Tools.MCP.Servers[name]
	if !ok {
		writeMCPError(w, http.StatusNotFound, "not_found", "MCP server not found.")
		return
	}
	server.Enabled = true
	if server.EnvFile != "" && !filepath.IsAbs(server.EnvFile) {
		server.EnvFile = filepath.Join(cfg.WorkspacePath(), server.EnvFile)
	}

	result := testMCPServerConfig(r.Context(), name, server)
	writeMCPJSON(w, http.StatusOK, result)
}

// testMCPServerConfig connects to one server with a temporary manager and
// returns the discovered tool list. It never registers tools in the agent.
func testMCPServerConfig(ctx context.Context, name string, server config.MCPServerConfig) mcpTestResult {
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	manager := mcp.NewManager()
	defer func() {
		if err := manager.Close(); err != nil {
			logger.DebugCF("mcp", "Closing test MCP manager failed", map[string]any{"error": err.Error()})
		}
	}()

	if err := manager.ConnectServer(testCtx, name, server); err != nil {
		return mcpTestResult{OK: false, Error: err.Error()}
	}

	tools := manager.GetAllTools()[name]
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool != nil {
			names = append(names, tool.Name)
		}
	}
	sort.Strings(names)
	return mcpTestResult{OK: true, ToolCount: len(names), Tools: names}
}

func buildMCPServersResponse(cfg *config.Config) mcpServersResponse {
	names := make([]string, 0, len(cfg.Tools.MCP.Servers))
	for name := range cfg.Tools.MCP.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	servers := make([]mcpServerInfo, 0, len(names))
	for _, name := range names {
		server := cfg.Tools.MCP.Servers[name]
		servers = append(servers, mcpServerInfo{
			Name: name,
			mcpServerPayload: mcpServerPayload{
				Enabled:  server.Enabled,
				Deferred: server.Deferred,
				Type:     config.EffectiveMCPTransportType(server),
				Command:  server.Command,
				Args:     append([]string{}, server.Args...),
				Env:      cloneStringMap(server.Env),
				EnvFile:  server.EnvFile,
				URL:      server.URL,
				Headers:  cloneStringMap(server.Headers),
			},
		})
	}
	if servers == nil {
		servers = []mcpServerInfo{}
	}

	return mcpServersResponse{
		Enabled: cfg.Tools.MCP.Enabled,
		Servers: servers,
		Discovery: mcpDiscoverySettings{
			Enabled:          cfg.Tools.MCP.Discovery.Enabled,
			TTL:              cfg.Tools.MCP.Discovery.TTL,
			MaxSearchResults: cfg.Tools.MCP.Discovery.MaxSearchResults,
			UseBM25:          cfg.Tools.MCP.Discovery.UseBM25,
			UseRegex:         cfg.Tools.MCP.Discovery.UseRegex,
		},
	}
}

func normalizeMCPServerPayload(payload mcpServerPayload) (config.MCPServerConfig, error) {
	transport := config.NormalizeMCPTransportType(payload.Type)
	if transport == "" {
		switch {
		case strings.TrimSpace(payload.Command) != "":
			transport = "stdio"
		case strings.TrimSpace(payload.URL) != "":
			transport = "sse"
		default:
			return config.MCPServerConfig{}, fmt.Errorf("choose stdio, http or sse and provide a command or URL")
		}
	}
	switch transport {
	case "stdio":
		if strings.TrimSpace(payload.Command) == "" {
			return config.MCPServerConfig{}, fmt.Errorf("command is required for stdio transport")
		}
		if strings.TrimSpace(payload.URL) != "" {
			return config.MCPServerConfig{}, fmt.Errorf("url is only used by http/sse transport")
		}
	case "http", "sse":
		if strings.TrimSpace(payload.URL) == "" {
			return config.MCPServerConfig{}, fmt.Errorf("url is required for %s transport", transport)
		}
		if strings.TrimSpace(payload.Command) != "" {
			return config.MCPServerConfig{}, fmt.Errorf("command is only used by stdio transport")
		}
	default:
		return config.MCPServerConfig{}, fmt.Errorf("unsupported transport: %s", transport)
	}

	args := make([]string, 0, len(payload.Args))
	for _, arg := range payload.Args {
		args = append(args, arg)
	}
	env, err := normalizeStringMap(payload.Env, 100)
	if err != nil {
		return config.MCPServerConfig{}, fmt.Errorf("env: %w", err)
	}
	headers, err := normalizeStringMap(payload.Headers, 50)
	if err != nil {
		return config.MCPServerConfig{}, fmt.Errorf("headers: %w", err)
	}

	return config.MCPServerConfig{
		Enabled:  payload.Enabled,
		Deferred: payload.Deferred,
		Type:     transport,
		Command:  strings.TrimSpace(payload.Command),
		Args:     args,
		Env:      env,
		EnvFile:  strings.TrimSpace(payload.EnvFile),
		URL:      strings.TrimSpace(payload.URL),
		Headers:  headers,
	}, nil
}

func normalizeStringMap(values map[string]string, maxEntries int) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > maxEntries {
		return nil, fmt.Errorf("at most %d entries are supported", maxEntries)
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("empty key")
		}
		if len(key) > 128 || len(value) > 4096 {
			return nil, fmt.Errorf("entry %q is too long", key)
		}
		result[key] = value
	}
	return result, nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func validMCPServerName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for index, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
			if index == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func writeMCPJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeMCPError(w http.ResponseWriter, status int, code, message string) {
	writeMCPJSON(w, status, map[string]any{"code": code, "error": message})
}
