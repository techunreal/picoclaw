import type { SkillSupportItem } from "@/api/skills"

export const LOCKED_AGENT_SKILL = "picoclaw-agent"

export function defaultSkillSelection(skills: SkillSupportItem[]): string[] {
  // Only the locked runtime skill is selected by default. Every active skill is
  // injected in full on each turn, so selecting all of them can consume most of
  // a local model's context window.
  return skills.some((skill) => skill.name === LOCKED_AGENT_SKILL)
    ? [LOCKED_AGENT_SKILL]
    : []
}
