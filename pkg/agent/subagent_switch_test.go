package agent

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

func newSubagentSwitchLoop(t *testing.T, mutate func(*config.Config)) *AgentLoop {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = t.TempDir()
	if mutate != nil {
		mutate(cfg)
	}
	return NewAgentLoop(cfg, bus.NewMessageBus(), &mockProvider{})
}

func assertToolPresence(t *testing.T, agent *AgentInstance, name string, want bool) {
	t.Helper()
	_, ok := agent.Tools.Get(name)
	if ok != want {
		t.Fatalf("tool %q presence = %v, want %v", name, ok, want)
	}
}

func TestSubagentMasterSwitchDisabledRegistersNoSubagentTools(t *testing.T) {
	al := newSubagentSwitchLoop(t, func(cfg *config.Config) {
		cfg.Tools.Subagent.Enabled = false
		cfg.Tools.Spawn.Enabled = true
		cfg.Tools.SpawnStatus.Enabled = true
	})

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}
	for _, name := range []string{"subagent", "spawn", "spawn_status", "delegate"} {
		assertToolPresence(t, agent, name, false)
	}
}

func TestSubagentMasterSwitchEnabledWithoutSpawnRegistersSubagentTool(t *testing.T) {
	al := newSubagentSwitchLoop(t, func(cfg *config.Config) {
		cfg.Tools.Subagent.Enabled = true
		cfg.Tools.Spawn.Enabled = false
		cfg.Tools.SpawnStatus.Enabled = false
	})

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}
	assertToolPresence(t, agent, "subagent", true)
	assertToolPresence(t, agent, "spawn", false)
	assertToolPresence(t, agent, "spawn_status", false)
}

func TestSubagentMasterSwitchEnabledRegistersConfiguredTools(t *testing.T) {
	al := newSubagentSwitchLoop(t, func(cfg *config.Config) {
		cfg.Tools.Subagent.Enabled = true
		cfg.Tools.Spawn.Enabled = true
		cfg.Tools.SpawnStatus.Enabled = true
	})

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}
	for _, name := range []string{"subagent", "spawn", "spawn_status"} {
		assertToolPresence(t, agent, name, true)
	}
}

func TestPerAgentSubagentsEnabledGatesDelegate(t *testing.T) {
	disabled := false
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = t.TempDir()
	cfg.Agents.List = []config.AgentConfig{
		{
			ID:        "alpha",
			Workspace: t.TempDir(),
			Model:     &config.AgentModelConfig{Primary: "model-alpha"},
			Subagents: &config.SubagentsConfig{Enabled: &disabled},
		},
		{
			ID:        "beta",
			Workspace: t.TempDir(),
			Model:     &config.AgentModelConfig{Primary: "model-beta"},
			Subagents: &config.SubagentsConfig{AllowAgents: []string{"*"}},
		},
	}

	al := NewAgentLoop(cfg, bus.NewMessageBus(), &mockProvider{})

	alpha, ok := al.registry.GetAgent("alpha")
	if !ok {
		t.Fatal("alpha agent not found")
	}
	if alpha.SubagentsEnabled() {
		t.Fatal("alpha should report subagents disabled")
	}
	for _, name := range []string{"subagent", "spawn", "spawn_status", "delegate"} {
		assertToolPresence(t, alpha, name, false)
	}

	beta, ok := al.registry.GetAgent("beta")
	if !ok {
		t.Fatal("beta agent not found")
	}
	if !beta.SubagentsEnabled() {
		t.Fatal("beta should report subagents enabled by default")
	}
	for _, name := range []string{"subagent", "spawn", "delegate"} {
		assertToolPresence(t, beta, name, true)
	}
}
