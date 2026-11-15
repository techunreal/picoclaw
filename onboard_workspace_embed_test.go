package picoclaw

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

func TestOnboardWorkspaceShipsCuratedSkills(t *testing.T) {
	want := []string{
		"agent-browser",
		"brainstorming",
		"humanizer",
		"picoclaw-agent",
		"repo-research",
		"skill-creator",
		"skill-safety-check",
		"weather",
	}

	entries, err := fs.ReadDir(OnboardWorkspace, "workspace/skills")
	if err != nil {
		t.Fatalf("read embedded workspace/skills: %v", err)
	}

	got := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		got[entry.Name()] = true
	}

	for _, name := range want {
		if !got[name] {
			t.Errorf("bundled skill %q is missing from workspace/skills", name)
		}
	}
	for name := range got {
		found := false
		for _, wantName := range want {
			if name == wantName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected bundled skill %q", name)
		}
	}

	for _, name := range want {
		data, err := fs.ReadFile(OnboardWorkspace, path.Join("workspace/skills", name, "SKILL.md"))
		if err != nil {
			t.Errorf("read %s/SKILL.md: %v", name, err)
			continue
		}
		content := strings.ReplaceAll(string(data), "\r\n", "\n")
		if !strings.HasPrefix(content, "---\n") {
			t.Errorf("%s/SKILL.md does not start with frontmatter", name)
		}
		if !strings.Contains(content, "name:") || !strings.Contains(content, "description:") {
			t.Errorf("%s/SKILL.md frontmatter lacks name or description", name)
		}
	}
}

func TestOnboardWorkspaceShipsNoOptionalSkills(t *testing.T) {
	for _, name := range []string{"github", "hardware", "summarize", "tmux"} {
		if _, err := fs.Stat(OnboardWorkspace, path.Join("workspace/skills", name)); err == nil {
			t.Errorf("optional skill %q must stay out of the bundled workspace", name)
		}
	}
}
