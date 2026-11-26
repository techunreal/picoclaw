import { launcherFetch } from "@/api/http"

export interface VoiceModelOption {
  name: string
  provider: string
  model: string
  api_base?: string
  has_key: boolean
  tts_like: boolean
}

export interface VoiceConfig {
  tts_model_name: string
  tts_voice?: string
  voices?: { id: string; label: string }[]
  resolved: boolean
  resolved_name?: string
  resolved_provider?: string
  resolved_model?: string
  models: VoiceModelOption[]
  restart_required?: boolean
}

async function readError(res: Response): Promise<string> {
  const fallback = `API error: ${res.status} ${res.statusText}`
  const raw = await res.text().catch(() => "")
  const trimmed = raw.trim()
  if (trimmed === "") return fallback
  try {
    const body = JSON.parse(trimmed) as { error?: string }
    if (typeof body.error === "string" && body.error.trim() !== "") {
      return body.error
    }
  } catch {
    // Plain-text errors carry the real reason.
  }
  return trimmed
}

export async function getVoiceConfig(): Promise<VoiceConfig> {
  const res = await launcherFetch("/api/voice/config")
  if (!res.ok) throw new Error(await readError(res))
  return (await res.json()) as VoiceConfig
}

export async function updateVoiceConfig(
  ttsModelName: string,
  ttsVoice?: string,
): Promise<VoiceConfig> {
  const body: Record<string, unknown> = { tts_model_name: ttsModelName }
  if (ttsVoice !== undefined) {
    body.tts_voice = ttsVoice
  }
  const res = await launcherFetch("/api/voice/config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await readError(res))
  return (await res.json()) as VoiceConfig
}

export async function testVoice(text: string): Promise<Blob> {
  const res = await launcherFetch("/api/voice/test", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text }),
  })
  if (!res.ok) throw new Error(await readError(res))
  return res.blob()
}

export async function speakVoice(
  text: string,
  language: string,
  signal?: AbortSignal,
): Promise<Blob> {
  const res = await launcherFetch("/api/voice/speak", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text, language }),
    signal,
  })
  if (!res.ok) throw new Error(await readError(res))
  return res.blob()
}
