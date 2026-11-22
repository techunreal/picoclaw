package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEstimateSystemContextTokensIncludesSkills(t *testing.T) {
	shared := t.TempDir()
	skillDir := filepath.Join(shared, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	body := "---\nname: test-skill\ndescription: A test skill\n---\n\n# Test skill\n\n" +
		strings.Repeat("Follow this rule. ", 200)
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	builder := NewContextBuilderWithShared(t.TempDir(), shared)
	agent := &AgentInstance{ContextBuilder: builder, SkillsFilter: []string{"test-skill"}}

	system, skills := estimateSystemContextTokens(agent, "")
	if skills <= 0 {
		t.Fatalf("skill tokens = %d, want > 0", skills)
	}
	if system <= skills {
		t.Fatalf("system = %d, skills = %d; the system prompt must include its base", system, skills)
	}

	agent.SkillsFilter = nil
	base, noSkills := estimateSystemContextTokens(agent, "")
	if noSkills != 0 {
		t.Fatalf("skill tokens = %d, want 0 without skills", noSkills)
	}
	if system <= base {
		t.Fatalf("system with skills (%d) = base (%d), want a larger value", system, base)
	}
}

func TestEstimateSystemContextTokensWithoutBuilder(t *testing.T) {
	system, skills := estimateSystemContextTokens(&AgentInstance{}, "")
	if system != 0 || skills != 0 {
		t.Fatalf("estimateSystemContextTokens() = (%d, %d), want (0, 0)", system, skills)
	}
}
