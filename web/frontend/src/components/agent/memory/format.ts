import type { TFunction } from "i18next"

import type { AgentFileItem } from "@/api/agent-files"

const STATIC_LABEL_KEYS: Record<string, string> = {
  agent: "agent",
  agents: "agents_legacy",
  soul: "soul",
  user: "user",
  identity: "identity_legacy",
  memory: "memory",
  heartbeat: "heartbeat",
}

export const LARGE_FILE_BYTES = 32 * 1024

export function agentFileLabel(item: AgentFileItem, t: TFunction): string {
  if (item.id.startsWith("daily:") || item.id.startsWith("skill:")) {
    return item.label
  }
  const key = STATIC_LABEL_KEYS[item.id]
  return key ? t(`pages.agent.memory.files.${key}`) : item.label
}

export function agentFileGroupLabel(
  id: string,
  fallback: string,
  t: TFunction,
): string {
  return t(`pages.agent.memory.groups.${id}`, { defaultValue: fallback })
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`
  }
  const kb = bytes / 1024
  if (kb < 1024) {
    return `${kb.toFixed(kb < 10 ? 1 : 0)} KB`
  }
  return `${(kb / 1024).toFixed(1)} MB`
}

export function estimateTokens(content: string): number {
  return Math.ceil(content.length / 4)
}

export function utf8Size(content: string): number {
  return new TextEncoder().encode(content).length
}

export function formatTimestamp(seconds: number): string {
  if (!seconds) {
    return ""
  }
  return new Date(seconds * 1000).toLocaleString()
}

/**
 * Returns the raw frontmatter block (including the closing delimiter) when the
 * document starts with YAML frontmatter, otherwise null.
 */
export function extractFrontmatter(content: string): string | null {
  const normalized = content.replace(/\r\n/g, "\n")
  if (!normalized.startsWith("---\n")) {
    return null
  }
  const end = normalized.indexOf("\n---", 4)
  if (end === -1) {
    return null
  }
  return normalized.slice(0, end + 4)
}

export function frontmatterChanged(before: string, after: string): boolean {
  return extractFrontmatter(before) !== extractFrontmatter(after)
}
