package config

import (
	"encoding/json"
	"testing"
)

func TestSubagentsConfigIsEnabled(t *testing.T) {
	disabled := false
	enabled := true

	tests := []struct {
		name string
		cfg  *SubagentsConfig
		want bool
	}{
		{name: "nil config defaults to enabled", cfg: nil, want: true},
		{name: "missing flag defaults to enabled", cfg: &SubagentsConfig{}, want: true},
		{name: "explicit enabled", cfg: &SubagentsConfig{Enabled: &enabled}, want: true},
		{name: "explicit disabled", cfg: &SubagentsConfig{Enabled: &disabled}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsEnabled(); got != tt.want {
				t.Fatalf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubagentsConfigEnabledJSONRoundTrip(t *testing.T) {
	var agentCfg AgentConfig
	if err := json.Unmarshal([]byte(`{"id":"alpha","subagents":{"enabled":false,"allow_agents":["*"]}}`), &agentCfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if agentCfg.Subagents == nil || agentCfg.Subagents.IsEnabled() {
		t.Fatalf("subagents config should be parsed as disabled: %#v", agentCfg.Subagents)
	}
	if len(agentCfg.Subagents.AllowAgents) != 1 || agentCfg.Subagents.AllowAgents[0] != "*" {
		t.Fatalf("allow_agents not preserved: %#v", agentCfg.Subagents.AllowAgents)
	}

	data, err := json.Marshal(agentCfg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded AgentConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal(round trip) error = %v", err)
	}
	if decoded.Subagents == nil || decoded.Subagents.IsEnabled() {
		t.Fatalf("disabled state did not survive round trip: %#v", decoded.Subagents)
	}
}

func TestSubagentsConfigOmittedByDefault(t *testing.T) {
	data, err := json.Marshal(AgentConfig{ID: "alpha"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(data) != `{"id":"alpha"}` {
		t.Fatalf("unexpected JSON without subagents config: %s", data)
	}
}
