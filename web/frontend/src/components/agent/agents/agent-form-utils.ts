import type { SkillSupportItem } from "@/api/skills"

export const LOCKED_AGENT_SKILL = "picoclaw-agent"

export function defaultSkillSelection(skills: SkillSupportItem[]): string[] {
  return skills.map((skill) => skill.name)
}
