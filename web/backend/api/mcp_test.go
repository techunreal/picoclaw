package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sipeed/picoclaw/pkg/config"
)

func newMCPTestMux(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	configPath := setupChatAttachmentTestEnv(t)
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.registerMCPRoutes(mux)
	return mux, configPath
}

func newMCPTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "api-test-server",
		Version: "1.0.0",
	}, nil)
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "echo",
		Description: "Echo a message",
	}, func(ctx context.Context, req *sdkmcp.CallToolRequest, args map[string]any) (*sdkmcp.CallToolResult, any, error) {
		message, _ := args["message"].(string)
		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: message}},
		}, nil, nil
	})

	handler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return server
	}, &sdkmcp.StreamableHTTPOptions{JSONResponse: true})
	return httptest.NewServer(handler)
}

func doMCPRequest(t *testing.T, mux *http.ServeMux, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeMCPServers(t *testing.T, rec *httptest.ResponseRecorder) mcpServersResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response mcpServersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return response
}

func TestMCP_CRUDAndTest(t *testing.T) {
	mux, configPath := newMCPTestMux(t)
	mcpServer := newMCPTestServer(t)
	defer mcpServer.Close()

	initial := decodeMCPServers(t, doMCPRequest(t, mux, http.MethodGet, "/api/mcp/servers", nil))
	if initial.Enabled {
		t.Fatal("MCP should be disabled by default")
	}
	if len(initial.Servers) != 0 {
		t.Fatalf("servers = %#v, want empty", initial.Servers)
	}

	rec := doMCPRequest(t, mux, http.MethodPut, "/api/mcp/servers/docs", map[string]any{
		"enabled": true,
		"type":    "http",
		"url":     mcpServer.URL,
		"headers": map[string]string{"X-Test": "1"},
	})
	response := decodeMCPServers(t, rec)
	if len(response.Servers) != 1 || response.Servers[0].Name != "docs" {
		t.Fatalf("servers = %#v", response.Servers)
	}
	if response.Servers[0].Type != "http" || response.Servers[0].URL != mcpServer.URL {
		t.Fatalf("server = %+v", response.Servers[0])
	}

	settings := decodeMCPServers(t, doMCPRequest(t, mux, http.MethodPut, "/api/mcp/settings", map[string]any{
		"enabled": true,
		"discovery": map[string]any{
			"enabled":            true,
			"ttl":                10,
			"max_search_results": 7,
			"use_bm25":           true,
			"use_regex":          false,
		},
	}))
	if !settings.Enabled || !settings.Discovery.Enabled || settings.Discovery.TTL != 10 {
		t.Fatalf("settings = %+v", settings)
	}

	testRec := doMCPRequest(t, mux, http.MethodPost, "/api/mcp/servers/docs/test", nil)
	if testRec.Code != http.StatusOK {
		t.Fatalf("test status = %d, body = %s", testRec.Code, testRec.Body.String())
	}
	var result mcpTestResult
	if err := json.Unmarshal(testRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal test result error = %v", err)
	}
	if !result.OK || result.ToolCount != 1 {
		t.Fatalf("test result = %+v", result)
	}
	if len(result.Tools) != 1 || result.Tools[0] != "echo" {
		t.Fatalf("tools = %v, want echo", result.Tools)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Tools.MCP.Servers["docs"].URL != mcpServer.URL {
		t.Fatalf("server not persisted: %+v", cfg.Tools.MCP.Servers["docs"])
	}

	deleted := decodeMCPServers(t, doMCPRequest(t, mux, http.MethodDelete, "/api/mcp/servers/docs", nil))
	if len(deleted.Servers) != 0 {
		t.Fatalf("servers after delete = %#v", deleted.Servers)
	}
}

func TestMCP_Validation(t *testing.T) {
	mux, _ := newMCPTestMux(t)

	cases := []struct {
		name   string
		target string
		body   map[string]any
	}{
		{
			name:   "invalid name",
			target: "/api/mcp/servers/bad%20name",
			body:   map[string]any{"type": "stdio", "command": "echo"},
		},
		{
			name:   "stdio without command",
			target: "/api/mcp/servers/docs",
			body:   map[string]any{"type": "stdio", "url": "http://example.com"},
		},
		{
			name:   "http without url",
			target: "/api/mcp/servers/docs",
			body:   map[string]any{"type": "http", "command": "echo"},
		},
		{
			name:   "unsupported transport",
			target: "/api/mcp/servers/docs",
			body:   map[string]any{"type": "carrier-pigeon"},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			rec := doMCPRequest(t, mux, http.MethodPut, test.target, test.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}

	rec := doMCPRequest(t, mux, http.MethodPost, "/api/mcp/servers/missing/test", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing test status = %d, want 404", rec.Code)
	}
}
