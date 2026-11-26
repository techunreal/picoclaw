import { launcherFetch } from "@/api/http"

export interface LocalTTSVoice {
  id: string
  label: string
}

export interface LocalTTSPython {
  available: boolean
  executable?: string
  version?: string
  pip?: boolean
  venv?: boolean
  message?: string
}

export interface LocalTTSCatalogItem {
  id: string
  name: string
  language: string
  license: string
  size_bytes: number
  voices: LocalTTSVoice[]
  default_voice: string
  installed: boolean
  port?: number
  running: boolean
}

export interface LocalTTSModel {
  recipe_id: string
  name: string
  language: string
  voices: LocalTTSVoice[]
  default_voice: string
  port: number
}

export interface LocalTTSStatus {
  installing: boolean
  phase: string
  message: string
  progress: number
  error?: string
  done: boolean
  model?: LocalTTSModel
}

export interface LocalTTSModelsResponse {
  environment: LocalTTSPython
  catalog: LocalTTSCatalogItem[]
  status: LocalTTSStatus
}

async function readError(res: Response): Promise<string> {
  const raw = await res.text().catch(() => "")
  try {
    const body = JSON.parse(raw) as { error?: string; code?: string }
    if (typeof body.error === "string" && body.error.trim() !== "") {
      return body.error
    }
  } catch {
    // plain text
  }
  return raw.trim() || `API error: ${res.status} ${res.statusText}`
}

export async function getLocalTTSModels(): Promise<LocalTTSModelsResponse> {
  const res = await launcherFetch("/api/tts/local/models")
  if (!res.ok) throw new Error(await readError(res))
  return (await res.json()) as LocalTTSModelsResponse
}

export async function installLocalTTS(
  url: string,
  consent: boolean,
): Promise<void> {
  const res = await launcherFetch("/api/tts/local/install", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url, consent }),
  })
  if (!res.ok) throw new Error(await readError(res))
}

export async function getLocalTTSStatus(): Promise<LocalTTSStatus> {
  const res = await launcherFetch("/api/tts/local/status")
  if (!res.ok) throw new Error(await readError(res))
  return (await res.json()) as LocalTTSStatus
}

export async function uninstallLocalTTS(id: string): Promise<void> {
  const res = await launcherFetch(`/api/tts/local/models/${encodeURIComponent(id)}`, {
    method: "DELETE",
  })
  if (!res.ok) throw new Error(await readError(res))
}
