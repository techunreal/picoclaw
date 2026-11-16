package agent

import (
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
