package agent

import (
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func newModelSwitchTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         t.TempDir(),
				Provider:          "openai",
				ModelName:         "local",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		ModelList: []*config.ModelConfig{
			{
				ModelName:     "local",
				Provider:      "openai",
				Model:         "openai/local",
				APIBase:       "http://127.0.0.1:1/v1",
				APIKeys:       config.SimpleSecureStrings("local-key"),
				ContextWindow: 65536,
			},
			{
				ModelName:     "remote",
				Provider:      "openai",
				Model:         "openai/remote",
				APIBase:       "http://127.0.0.1:2/v1",
				APIKeys:       config.SimpleSecureStrings("remote-key"),
				ContextWindow: 1048576,
			},
			{
				ModelName: "disabled",
				Provider:  "openai",
				Model:     "openai/disabled",
				APIBase:   "http://127.0.0.1:3/v1",
				APIKeys:   config.SimpleSecureStrings("disabled-key"),
				Disabled:  true,
			},
		},
	}
}

func newModelSwitchTestAgent(t *testing.T, cfg *config.Config) *AgentInstance {
	t.Helper()
	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	return NewAgentInstance(nil, &cfg.Agents.Defaults, cfg, provider)
}

func TestSwitchModelUpdatesStateAndContextWindow(t *testing.T) {
	cfg := newModelSwitchTestConfig(t)
	agent := newModelSwitchTestAgent(t, cfg)

	oldModel, err := agent.SwitchModel(cfg, "remote")
	if err != nil {
		t.Fatalf("SwitchModel() error = %v", err)
	}
	if oldModel != "local" {
		t.Fatalf("old model = %q, want local", oldModel)
	}
	if agent.Model != "remote" {
		t.Fatalf("Model = %q, want remote", agent.Model)
	}
	if agent.ContextWindow != 1048576 {
		t.Fatalf("ContextWindow = %d, want 1048576", agent.ContextWindow)
	}
	if len(agent.Candidates) == 0 || agent.Candidates[0].DisplayName != "remote" {
		t.Fatalf("Candidates = %#v, want remote primary", agent.Candidates)
	}
	if agent.Provider == nil {
		t.Fatal("Provider is nil after switch")
	}
}

func TestSwitchModelRejectsUnknownAndDisabledModels(t *testing.T) {
	cfg := newModelSwitchTestConfig(t)
	agent := newModelSwitchTestAgent(t, cfg)

	if _, err := agent.SwitchModel(cfg, "missing"); err == nil {
		t.Fatal("SwitchModel() expected an error for an unknown model")
	}
	if _, err := agent.SwitchModel(cfg, "disabled"); err == nil {
		t.Fatal("SwitchModel() expected an error for a disabled model")
	}
	if agent.Model != "local" {
		t.Fatalf("Model changed to %q after rejected switches", agent.Model)
	}
}

func TestSwitchModelWaitsForActiveTurnLock(t *testing.T) {
	cfg := newModelSwitchTestConfig(t)
	agent := newModelSwitchTestAgent(t, cfg)

	// runTurn holds this read lock for the whole turn.
	agent.modelStateMutex().RLock()
	done := make(chan error, 1)
	go func() {
		_, err := agent.SwitchModel(cfg, "remote")
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("SwitchModel completed while a turn held the read lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	agent.modelStateMutex().RUnlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SwitchModel() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SwitchModel did not complete after the turn lock was released")
	}
	if agent.Model != "remote" {
		t.Fatalf("Model = %q, want remote", agent.Model)
	}
}

func TestSwitchAgentModelReportsPendingForActiveTurn(t *testing.T) {
	cfg := newModelSwitchTestConfig(t)
	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), provider)
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("default agent is nil")
	}

	agent.modelStateMutex().RLock()
	oldModel, applied, err := al.SwitchAgentModel("main", "remote", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("SwitchAgentModel() error = %v", err)
	}
	if applied {
		t.Fatal("SwitchAgentModel() applied while the turn lock was held")
	}
	if oldModel != "" {
		t.Fatalf("oldModel = %q, want empty for pending switch", oldModel)
	}

	agent.modelStateMutex().RUnlock()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if model, ok := al.GetAgentModel("main"); ok && model == "remote" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	model, _ := al.GetAgentModel("main")
	t.Fatalf("model after pending switch = %q, want remote", model)
}

func TestSwitchAgentModelUnknownAgent(t *testing.T) {
	cfg := newModelSwitchTestConfig(t)
	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	al := NewAgentLoop(cfg, bus.NewMessageBus(), provider)

	if _, _, err := al.SwitchAgentModel("missing", "remote", time.Second); err == nil {
		t.Fatal("SwitchAgentModel() expected an error for an unknown agent")
	}
}
