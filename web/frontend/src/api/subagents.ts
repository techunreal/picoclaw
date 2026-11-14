import { launcherFetch } from "@/api/http"

export type SubagentStatus =
  | "running"
  | "completed"
  | "failed"
  | "canceled"
  | string

export interface SubagentRun {
  id: string
  parent_turn_id?: string
  agent_id: string
  task: string
  status: SubagentStatus
  depth: number
  started_at_ms: number
  ended_at_ms?: number
  duration_ms: number
  result_preview?: string
}

interface SubagentListResponse {
  runs?: SubagentRun[]
}

export class SubagentApiError extends Error {
  code: string
  status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = "SubagentApiError"
    this.code = code
    this.status = status
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await launcherFetch(path, options)
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
    throw new SubagentApiError(code, message, res.status)
  }
  return (await res.json()) as T
}

export async function listSubagentRuns(): Promise<SubagentRun[]> {
  const body = await request<SubagentListResponse>("/api/subagents/runs")
  return body.runs ?? []
}

export async function cancelSubagentRun(id: string): Promise<void> {
  await request<{ ok: boolean }>(
    `/api/subagents/runs/${encodeURIComponent(id)}/cancel`,
    { method: "POST" },
  )
}
