import { launcherFetch } from "@/api/http"

export type CronScheduleKind = "at" | "every" | "cron"

export interface CronSchedule {
  kind: CronScheduleKind
  atMs?: number
  everyMs?: number
  expr?: string
  tz?: string
}

export interface CronJobPayload {
  kind: string
  message: string
  command?: string
  channel?: string
  to?: string
}

export interface CronJobState {
  nextRunAtMs?: number
  lastRunAtMs?: number
  lastStatus?: string
  lastError?: string
}

export interface CronJob {
  id: string
  name: string
  enabled: boolean
  schedule: CronSchedule
  payload: CronJobPayload
  state: CronJobState
  createdAtMs: number
  updatedAtMs: number
  deleteAfterRun?: boolean
}

export interface CronStatus {
  running: boolean
  tool_enabled: boolean
  jobs: number
  next_wake_at_ms?: number
  gateway_time_ms: number
}

export interface CronJobInput {
  name: string
  message: string
  schedule: {
    kind: CronScheduleKind
    at_ms?: number
    every_ms?: number
    expr?: string
    tz?: string
  }
  channel?: string
  to?: string
  enabled?: boolean
}

export interface CronJobList {
  jobs: CronJob[]
  status: CronStatus
}

export class CronApiError extends Error {
  code: string
  status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = "CronApiError"
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
    throw new CronApiError(code, message, res.status)
  }
  return (await res.json()) as T
}

export function listCronJobs(): Promise<CronJobList> {
  return request<CronJobList>("/api/cron/jobs")
}

export function createCronJob(input: CronJobInput): Promise<CronJob> {
  return request<CronJob>("/api/cron/jobs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  })
}

export function updateCronJob(
  id: string,
  input: CronJobInput,
): Promise<CronJob> {
  return request<CronJob>(`/api/cron/jobs/${encodeURIComponent(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  })
}

export function deleteCronJob(id: string): Promise<unknown> {
  return request(`/api/cron/jobs/${encodeURIComponent(id)}`, {
    method: "DELETE",
  })
}

export function setCronJobEnabled(
  id: string,
  enabled: boolean,
): Promise<CronJob> {
  return request<CronJob>(`/api/cron/jobs/${encodeURIComponent(id)}/enable`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  })
}

export function runCronJobNow(id: string): Promise<unknown> {
  return request(`/api/cron/jobs/${encodeURIComponent(id)}/run`, {
    method: "POST",
  })
}
