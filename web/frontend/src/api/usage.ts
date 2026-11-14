import { launcherFetch } from "@/api/http"

export interface UsageTotals {
  input_tokens: number
  output_tokens: number
  total_tokens: number
  llm_calls: number
  last_used_ms?: number
}

export interface UsageModelSummary extends UsageTotals {
  model: string
  estimated_cost?: number | null
  currency?: string
  price_configured: boolean
}

export interface UsageSessionSummary extends UsageTotals {
  session_key: string
  agent_id?: string
  channel?: string
  chat_id?: string
  models?: UsageModelSummary[]
  estimated_cost?: number | null
  currency?: string
}

export interface UsageSummary {
  totals: UsageTotals
  models: UsageModelSummary[]
  sessions: UsageSessionSummary[]
  estimated_cost?: number | null
  currency?: string
  priced_models: number
  unpriced_models: number
  cost_complete: boolean
  started_at_ms: number
}

export class UsageApiError extends Error {
  code: string
  status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = "UsageApiError"
    this.code = code
    this.status = status
  }
}

async function request<T>(path: string): Promise<T> {
  const res = await launcherFetch(path)
  if (!res.ok) {
    let code = "request_failed"
    let message = `Request failed (${res.status}).`
    try {
      const body = (await res.json()) as { code?: string; error?: string }
      if (typeof body.code === "string" && body.code.trim() !== "") {
        code = body.code
      }
      if (typeof body.error === "string" && body.error.trim() !== "") {
        message = body.error
      }
    } catch {
      // Keep the generic message.
    }
    throw new UsageApiError(code, message, res.status)
  }
  return (await res.json()) as T
}

export function getUsageSummary(): Promise<UsageSummary> {
  return request<UsageSummary>("/api/usage/summary")
}
