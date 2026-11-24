package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func writePromptTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writePromptTestSkill(t *testing.T, root, name, description, body string) {
	t.Helper()
	writePromptTestFile(t, filepath.Join(root, name, "SKILL.md"),
		"---\nname: "+name+"\ndescription: "+description+"\n---\n\n"+body)
}

func TestPersonaIdentityReplacesGenericAssistantLine(t *testing.T) {
	defaultWS := t.TempDir()
	agentWS := t.TempDir()

	writePromptTestFile(t, filepath.Join(agentWS, "AGENT.md"), "# AGENT\n\nYou are Sokrates.\n")
	writePromptTestFile(t, filepath.Join(agentWS, "SOUL.md"), "# SOUL\n\nQuestion everything.\n")

	defaultPrompt := NewContextBuilder(defaultWS).BuildSystemPrompt()
	if !strings.Contains(defaultPrompt, "You are picoclaw, a helpful AI assistant.") {
		t.Fatalf("default prompt lost the generic identity line")
	}

	persona := NewContextBuilder(agentWS).WithPersonaMode(true, nil)
	prompt := persona.BuildSystemPrompt()
	if strings.Contains(prompt, "You are picoclaw, a helpful AI assistant.") {
		t.Fatalf("persona prompt still contains the generic identity line")
	}
	if !strings.Contains(prompt, "Your identity, role and behavior are defined by the agent files below") {
		t.Fatalf("persona prompt is missing the neutral identity line")
	}
	if !strings.Contains(prompt, "You are Sokrates.") {
		t.Fatalf("persona prompt does not include AGENT.md body")
	}
	if !strings.Contains(prompt, "## SOUL.md") {
		t.Fatalf("persona prompt does not include SOUL.md")
	}
	// The product header, workspace section and rules must stay.
	if !strings.Contains(prompt, "# picoclaw") || !strings.Contains(prompt, "## Important Rules") {
		t.Fatalf("persona prompt lost shared kernel sections")
	}
}

func TestPersonaCatalogScopedToAgentSkills(t *testing.T) {
	shared := t.TempDir()
	writePromptTestSkill(t, shared, "picoclaw-agent", "PicoClaw self-management", "SELF_SKILL_BODY")
	writePromptTestSkill(t, shared, "weather", "Weather reports", "WEATHER_BODY")
	writePromptTestSkill(t, shared, "office-word", "Word documents", "OFFICE_BODY")

	agentWS := t.TempDir()
	writePromptTestFile(t, filepath.Join(agentWS, "AGENT.md"), "# AGENT\n\nCharacter.\n")

	builder := NewContextBuilderWithShared(agentWS, shared).
		WithPersonaMode(true, []string{"weather"})
	prompt := builder.BuildSystemPrompt()

	if !strings.Contains(prompt, "<name>weather</name>") {
		t.Fatalf("persona catalog is missing an assigned skill")
	}
	if !strings.Contains(prompt, "<name>picoclaw-agent</name>") {
		t.Fatalf("persona catalog is missing the locked self-management skill")
	}
	if strings.Contains(prompt, "<name>office-word</name>") {
		t.Fatalf("persona catalog includes an unassigned skill")
	}
}

func TestPersonaActiveSkillBodyIsCatalogOnly(t *testing.T) {
	defaultWS := t.TempDir()
	shared := filepath.Join(defaultWS, "skills")
	writePromptTestSkill(t, shared, "picoclaw-agent", "PicoClaw self-management", "SELF_SKILL_BODY")
	writePromptTestSkill(t, shared, "weather", "Weather reports", "WEATHER_BODY")

	agentWS := t.TempDir()
	builder := NewContextBuilderWithShared(agentWS, shared).
		WithPersonaMode(true, []string{"picoclaw-agent", "weather"})

	names := builder.ResolveActiveSkillsForContext([]string{"picoclaw-agent", "weather"})
	if len(names) != 2 {
		t.Fatalf("resolved active skills = %v", names)
	}

	filtered := builder.FilterPersonaActiveSkills(names, nil)
	if strings.Contains(strings.ToLower(strings.Join(filtered, ",")), AgentSelfSkillName) {
		t.Fatalf("persona active skills still contain %s: %v", AgentSelfSkillName, filtered)
	}
	if text := builder.buildActiveSkillsContext(filtered); strings.Contains(text, "SELF_SKILL_BODY") {
		t.Fatalf("self-management skill body was injected for a persona agent")
	}
	if text := builder.buildActiveSkillsContext(filtered); !strings.Contains(text, "WEATHER_BODY") {
		t.Fatalf("assigned skill body was not injected")
	}

	forced := builder.FilterPersonaActiveSkills(names, []string{AgentSelfSkillName})
	if text := builder.buildActiveSkillsContext(forced); !strings.Contains(text, "SELF_SKILL_BODY") {
		t.Fatalf("forced self-management skill body was not injected")
	}

	// The default agent keeps the full body.
	defaultBuilder := NewContextBuilder(defaultWS)
	defaultNames := defaultBuilder.ResolveActiveSkillsForContext([]string{AgentSelfSkillName})
	if text := defaultBuilder.buildActiveSkillsContext(defaultNames); !strings.Contains(text, "SELF_SKILL_BODY") {
		t.Fatalf("default agent lost the self-management skill body")
	}
}

func TestPersonaPromptSizeReport(t *testing.T) {
	shared := t.TempDir()
	writePromptTestSkill(t, shared, "picoclaw-agent", "PicoClaw self-management", strings.Repeat("x", 4000))
	agentWS := t.TempDir()
	writePromptTestFile(t, filepath.Join(agentWS, "AGENT.md"), "# AGENT\n\nCharacter.\n")

	builder := NewContextBuilderWithShared(agentWS, shared).
		WithPersonaMode(true, []string{AgentSelfSkillName})
	report := builder.PromptSizeReport([]string{AgentSelfSkillName})

	if !report.Persona || report.IdentityMode != "agent-files" {
		t.Fatalf("report identity = %q persona=%v", report.IdentityMode, report.Persona)
	}
	if report.TotalChars == 0 || report.TotalTokens == 0 {
		t.Fatalf("report totals are empty: %+v", report)
	}
	if len(report.ActiveSkills) != 0 {
		t.Fatalf("persona active skills should be catalog-only, got %v", report.ActiveSkills)
	}
	for _, section := range report.Sections {
		if section.ID == "capability.active_skills" {
			t.Fatalf("self-management skill body leaked into the prompt size report")
		}
	}
}

func TestNewAgentInstancePersonaModeFollowsWorkspace(t *testing.T) {
	defaultWS := t.TempDir()
	agentWS := t.TempDir()
	writePromptTestFile(t, filepath.Join(agentWS, "AGENT.md"), "# AGENT\n\nCharacter.\n")

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: defaultWS,
				ModelName: "test-model",
			},
		},
	}
	cfg.Agents.List = []config.AgentConfig{{
		ID:        "hero",
		Workspace: agentWS,
		Skills:    []string{AgentSelfSkillName},
	}}

	hero := NewAgentInstance(&cfg.Agents.List[0], &cfg.Agents.Defaults, cfg, &mockProvider{})
	if !hero.ContextBuilder.IsPersonaMode() {
		t.Fatalf("agent with its own workspace should use persona mode")
	}
	prompt := hero.ContextBuilder.BuildSystemPrompt()
	if strings.Contains(prompt, "You are picoclaw, a helpful AI assistant.") {
		t.Fatalf("persona agent kept the generic identity line")
	}

	mainCfg := config.AgentConfig{ID: "main"}
	main := NewAgentInstance(&mainCfg, &cfg.Agents.Defaults, cfg, &mockProvider{})
	if main.ContextBuilder.IsPersonaMode() {
		t.Fatalf("default agent must not use persona mode")
	}
	if prompt := main.ContextBuilder.BuildSystemPrompt(); !strings.Contains(prompt, "You are picoclaw, a helpful AI assistant.") {
		t.Fatalf("default agent lost the generic identity line")
	}
}
