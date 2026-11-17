package agent

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func newRegistryTestConfig(t *testing.T, agents []config.AgentConfig) *config.Config {
	t.Helper()
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         t.TempDir(),
				Provider:          "openai",
				ModelName:         "default-alias",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
			List: agents,
		},
		ModelList: []*config.ModelConfig{
			{
				ModelName: "default-alias",
				Provider:  "openai",
				Model:     "openai/gpt-x",
				APIBase:   "http://127.0.0.1:1/v1",
				APIKeys:   config.SimpleSecureStrings("test-key"),
			},
		},
	}
}

func TestNewAgentRegistryCreatesImplicitMainWithConfiguredAgents(t *testing.T) {
	cfg := newRegistryTestConfig(t, []config.AgentConfig{
		{ID: "alpha", Workspace: t.TempDir()},
		{ID: "beta", Workspace: t.TempDir()},
	})
	registry := NewAgentRegistry(cfg, &mockProvider{})

	main, ok := registry.GetAgent("main")
	if !ok || main == nil {
		t.Fatal("implicit main agent was not registered")
	}
	if main.Model != "default-alias" {
		t.Fatalf("main model = %q, want default-alias", main.Model)
	}
	if main.Workspace != cfg.Agents.Defaults.Workspace {
		t.Fatalf("main workspace = %q, want %q", main.Workspace, cfg.Agents.Defaults.Workspace)
	}
	if _, ok := registry.GetAgent("alpha"); !ok {
		t.Fatal("alpha agent missing")
	}
}

func TestDefaultAgentPrefersMainWhenNoExplicitDefault(t *testing.T) {
	cfg := newRegistryTestConfig(t, []config.AgentConfig{
		{ID: "alpha", Workspace: t.TempDir()},
		{ID: "beta", Workspace: t.TempDir()},
	})
	registry := NewAgentRegistry(cfg, &mockProvider{})
	if got := registry.GetDefaultAgent(); got == nil || got.ID != "main" {
		t.Fatalf("default agent = %#v, want main", got)
	}
}

func TestDefaultAgentHonorsExplicitDefault(t *testing.T) {
	cfg := newRegistryTestConfig(t, []config.AgentConfig{
		{ID: "alpha", Workspace: t.TempDir()},
		{ID: "beta", Workspace: t.TempDir(), Default: true},
	})
	registry := NewAgentRegistry(cfg, &mockProvider{})
	if got := registry.GetDefaultAgent(); got == nil || got.ID != "beta" {
		t.Fatalf("default agent = %#v, want beta", got)
	}
	if _, ok := registry.GetAgent("main"); !ok {
		t.Fatal("implicit main agent should still exist")
	}
}
