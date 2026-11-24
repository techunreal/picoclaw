import type { SkillSupportItem } from "@/api/skills"

export const LOCKED_AGENT_SKILL = "picoclaw-agent"

// Tools a profile agent gets when its AGENT.md does not declare a tools list.
// File read/write stays enabled so the agent can use its memory files; risky
// tools (exec, browser, cron, spawn, install_skill) stay opt-in.
export const SAFE_AGENT_TOOL_BASELINE = [
  "read_file",
  "list_dir",
  "write_file",
  "edit_file",
  "append_file",
  "web_search",
  "web_fetch",
  "message",
]

export const RISKY_AGENT_TOOLS = [
  "exec",
  "browser",
  "cron",
  "spawn",
  "subagent",
  "spawn_status",
  "install_skill",
]

export function defaultImportToolSelection(
  declared: string[] | undefined,
  declaredFlag: boolean,
  enabled: string[],
): string[] {
  const enabledSet = new Set(enabled.map((name) => name.toLowerCase()))
  const pick = (names: string[]) =>
    names.filter((name) => enabledSet.has(name.toLowerCase()))

  if (declaredFlag) {
    // An explicit list wins, including an explicit empty list ("no tools").
    return pick(declared ?? [])
  }
  const baseline = pick(SAFE_AGENT_TOOL_BASELINE)
  return baseline.length > 0 ? baseline : enabled
}

export function selectedRiskyTools(selected: string[]): string[] {
  const risky = new Set(RISKY_AGENT_TOOLS)
  return selected.filter((name) => risky.has(name.toLowerCase()))
}

export function defaultSkillSelection(skills: SkillSupportItem[]): string[] {
  // Only the locked runtime skill is selected by default. Every active skill is
  // injected in full on each turn, so selecting all of them can consume most of
  // a local model's context window.
  return skills.some((skill) => skill.name === LOCKED_AGENT_SKILL)
    ? [LOCKED_AGENT_SKILL]
    : []
}
