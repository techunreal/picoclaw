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
		"office-excel",
		"office-pdf",
		"office-powerpoint",
		"office-word",
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

func TestOnboardWorkspaceShipsOfficeHelper(t *testing.T) {
	data, err := fs.ReadFile(OnboardWorkspace, "workspace/tools/office_cli.py")
	if err != nil {
		t.Fatalf("read embedded office helper: %v", err)
	}
	for _, command := range []string{
		"docx-create",
		"xlsx-create",
		"pptx-create",
		"pdf-create",
		"pdf-merge",
		"pdf-fill",
	} {
		if !strings.Contains(string(data), command) {
			t.Errorf("office helper is missing command %q", command)
		}
	}

	for _, name := range []string{"office-word", "office-excel", "office-powerpoint", "office-pdf"} {
		content, err := fs.ReadFile(OnboardWorkspace, path.Join("workspace/skills", name, "SKILL.md"))
		if err != nil {
			t.Errorf("read %s/SKILL.md: %v", name, err)
			continue
		}
		if !strings.Contains(string(content), "PICOCLAW_OFFICE_PYTHON") {
			t.Errorf("%s/SKILL.md does not document the bundled runtime", name)
		}
	}
}
