package api

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestModelRestartRequired(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				ModelName:     "local",
				ModelFallbacks: []string{"backup"},
			},
			List: []config.AgentConfig{
				{
					ID:    "agent-a",
					Model: &config.AgentModelConfig{Primary: "agent-model"},
				},
				{
					ID:    "agent-b",
					Model: &config.AgentModelConfig{Primary: "other", Fallbacks: []string{"agent-fallback"}},
				},
			},
		},
	}

	cases := []struct {
		name string
		want bool
	}{
		{"local", true},
		{"backup", true},
		{"agent-model", true},
		{"agent-fallback", true},
		{"unused-model", false},
		{"", false},
	}

	for _, tc := range cases {
		if got := modelRestartRequired(cfg, tc.name); got != tc.want {
			t.Errorf("modelRestartRequired(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}

	if modelRestartRequired(nil, "local") {
		t.Error("nil config must not require a restart")
	}
}
