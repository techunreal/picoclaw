package agent

import (
	"path/filepath"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestResolveAgentWorkspace(t *testing.T) {
	root := t.TempDir()
	defaultsWorkspace := filepath.Join(root, "workspace")
	defaults := &config.AgentDefaults{Workspace: defaultsWorkspace}

	tests := []struct {
		name  string
		agent *config.AgentConfig
		want  string
	}{
		{
			name: "nil agent uses defaults",
			want: defaultsWorkspace,
		},
		{
			name:  "main keeps the shared workspace",
			agent: &config.AgentConfig{ID: "main", Default: true},
			want:  defaultsWorkspace,
		},
		{
			name:  "named agent gets agents directory",
			agent: &config.AgentConfig{ID: "alpha"},
			want:  filepath.Join(root, "agents", "alpha"),
		},
		{
			name:  "default flag does not share the workspace",
			agent: &config.AgentConfig{ID: "alpha", Default: true},
			want:  filepath.Join(root, "agents", "alpha"),
		},
		{
			name:  "explicit workspace wins",
			agent: &config.AgentConfig{ID: "alpha", Workspace: filepath.Join(root, "custom")},
			want:  filepath.Join(root, "custom"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAgentWorkspace(tt.agent, defaults); got != tt.want {
				t.Fatalf("resolveAgentWorkspace() = %q, want %q", got, tt.want)
			}
		})
	}
}
