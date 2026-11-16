package agent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestNewAgentInstance_PerModelContextWindowWins(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-context-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: tmpDir,
				ModelName: "ctx-model",
			},
		},
		ModelList: []*config.ModelConfig{
			{
				ModelName:     "ctx-model",
				Provider:      "openai",
				Model:         "gpt-x",
				APIBase:       "http://127.0.0.1:1/v1",
				ContextWindow: 1048576,
			},
		},
	}

	instance := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})
	if instance.ContextWindow != 1048576 {
		t.Fatalf("ContextWindow = %d, want 1048576", instance.ContextWindow)
	}
}

func TestNewAgentInstance_DetectsOpenCodeContextWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"hy4-flash-free","limit":{"context":1048576,"input":983040,"output":65536}}]}`))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: tmpDir,
				ModelName: "hy4 opencode",
				MaxTokens: 32768,
			},
		},
		ModelList: []*config.ModelConfig{
			{
				ModelName: "hy4 opencode",
				Provider:  "opencode-go",
				Model:     "hy4-flash-free",
				APIBase:   server.URL,
			},
		},
	}

	instance := NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, &mockProvider{})
	if instance.ContextWindow != 1048576 {
		t.Fatalf("ContextWindow = %d, want 1048576 (auto-detected)", instance.ContextWindow)
	}
}
