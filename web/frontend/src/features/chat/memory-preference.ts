const STORAGE_KEY = "picoclaw.memory-off-sessions"

function readDisabledSessions(): Set<string> {
  try {
    const raw = globalThis.localStorage?.getItem(STORAGE_KEY)
    if (!raw) {
      return new Set()
    }
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed)) {
      return new Set()
    }
    return new Set(
      parsed.filter((value): value is string => typeof value === "string"),
    )
  } catch {
    return new Set()
  }
}

function writeDisabledSessions(disabled: Set<string>) {
  try {
    if (disabled.size === 0) {
      globalThis.localStorage?.removeItem(STORAGE_KEY)
      return
    }
    globalThis.localStorage?.setItem(STORAGE_KEY, JSON.stringify([...disabled]))
  } catch {
    // Ignore storage failures; the session then follows the global switch.
  }
}

export function isSessionMemoryEnabled(sessionId: string): boolean {
  if (!sessionId) {
    return true
  }
  return !readDisabledSessions().has(sessionId)
}

export function setSessionMemoryPreference(
  sessionId: string,
  enabled: boolean,
) {
  if (!sessionId) {
    return
  }
  const disabled = readDisabledSessions()
  if (enabled) {
    disabled.delete(sessionId)
  } else {
    disabled.add(sessionId)
  }
  writeDisabledSessions(disabled)
}
