import { launcherFetch } from "@/api/http"

export interface MemorySettings {
  enabled: boolean
  runtime: boolean
  default: boolean
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
