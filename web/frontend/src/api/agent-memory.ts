import { launcherFetch } from "@/api/http"

export interface MemorySettings {
  enabled: boolean
  runtime: boolean
  default: boolean
  auto_write: boolean
  auto_write_runtime: boolean
  auto_write_default: boolean
  last_write_at?: number
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await launcherFetch(path, options)
  if (!res.ok) {
    let message = `API error: ${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string }
      if (typeof body.error === "string" && body.error.trim() !== "") {
        message = body.error
      }
    } catch {
      // Keep the generic message.
    }
    throw new Error(message)
  }
  return res.json() as Promise<T>
}

export function getMemorySettings(): Promise<MemorySettings> {
  return request<MemorySettings>("/api/agent/memory")
}

export function setMemoryEnabled(enabled: boolean): Promise<MemorySettings> {
  return request<MemorySettings>("/api/agent/memory", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  })
}

export function setMemoryAutoWrite(
  autoWrite: boolean,
): Promise<MemorySettings> {
  return request<MemorySettings>("/api/agent/memory", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ auto_write: autoWrite }),
  })
}
