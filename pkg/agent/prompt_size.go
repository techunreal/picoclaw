package agent

import "unicode/utf8"

// PromptSizeSection describes one section of the system prompt.
type PromptSizeSection struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Chars  int    `json:"chars"`
	Tokens int    `json:"tokens"`
}

// PromptSizeReport is a read-only breakdown of what a turn would send in the
// system prompt. Tool schemas and conversation history are not included.
type PromptSizeReport struct {
	Persona        bool                `json:"persona"`
	IdentityMode   string              `json:"identity_mode"`
	ActiveSkills   []string            `json:"active_skills,omitempty"`
	CatalogSkills  []string            `json:"catalog_skills,omitempty"`
	Sections       []PromptSizeSection `json:"sections"`
	TotalChars     int                 `json:"total_chars"`
	TotalTokens    int                 `json:"total_tokens"`
	EstimateMethod string              `json:"estimate_method"`
}

// PromptSizeReport builds the same system prompt parts a turn would use and
// reports their size. activeSkills is the agent's effective skill list (the
// SkillsFilter). The token figure uses the same 2/5 heuristic as
// EstimateSystemTokens so before/after numbers stay comparable.
func (cb *ContextBuilder) PromptSizeReport(activeSkills []string) PromptSizeReport {
	report := PromptSizeReport{
		Persona:        cb.IsPersonaMode(),
		IdentityMode:   "default",
		EstimateMethod: "chars * 2 / 5 (PicoClaw EstimateSystemTokens heuristic)",
	}
	if report.Persona {
		report.IdentityMode = "agent-files"
	}

	for _, part := range cb.buildSystemPromptParts(systemPromptBuildOptions{
		IncludeSkillCatalog: true,
		IncludeToolUseRule:  true,
	}) {
		chars := utf8.RuneCountInString(part.Content)
		if chars == 0 {
			continue
		}
		report.Sections = append(report.Sections, PromptSizeSection{
			ID:     part.ID,
			Title:  part.Title,
			Chars:  chars,
			Tokens: chars * 2 / 5,
		})
		report.TotalChars += chars
		report.TotalTokens += chars * 2 / 5
	}

	active := cb.FilterPersonaActiveSkills(activeSkills, nil)
	report.ActiveSkills = append([]string(nil), active...)
	if text := cb.buildActiveSkillsContext(active); text != "" {
		chars := utf8.RuneCountInString(text)
		report.Sections = append(report.Sections, PromptSizeSection{
			ID:     "capability.active_skills",
			Title:  "active skills",
			Chars:  chars,
			Tokens: chars * 2 / 5,
		})
		report.TotalChars += chars
		report.TotalTokens += chars * 2 / 5
	}

	if cb.IsPersonaMode() {
		report.CatalogSkills = cb.personaCatalogAllowed()
	} else {
		for _, skill := range cb.ListSkillNames() {
			report.CatalogSkills = append(report.CatalogSkills, skill)
		}
	}

	return report
}
