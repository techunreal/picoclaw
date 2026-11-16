package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/tools/fs"
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

func TestSharedSkillsAreVisibleAndWritable(t *testing.T) {
	root := t.TempDir()
	agentWorkspace := filepath.Join(root, "agents", "alpha")
	sharedSkills := filepath.Join(root, "workspace", "skills")
	skillDir := filepath.Join(sharedSkills, "shared-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: shared-skill\ndescription: shared\n---\n# shared\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cb := NewContextBuilderWithShared(agentWorkspace, sharedSkills)
	if summary := cb.buildSkillsSummary(nil); !strings.Contains(summary, "shared-skill") {
		t.Fatalf("shared skill missing from summary: %q", summary)
	}
	if prompt := cb.getIdentity(false); !strings.Contains(prompt, sharedSkills) {
		t.Fatalf("prompt does not mention the shared skills directory: %q", prompt)
	}

	patterns := appendPathPattern(nil, sharedSkills)
	if !fstools.IsAllowedPath(filepath.Join(sharedSkills, "new-skill", "SKILL.md"), patterns) {
		t.Fatal("shared skills directory should be writable")
	}
	if fstools.IsAllowedPath(filepath.Join(root, "workspace", "memory", "MEMORY.md"), patterns) {
		t.Fatal("write pattern must stay limited to the shared skills directory")
	}
}
