import { launcherFetch } from "@/api/http"

export interface BackupSection {
  name: string
  files: number
  bytes: number
}

export interface BackupExclusion {
  path: string
  reason: string
}

export interface BackupManifest {
  format: string
  version: number
  created_at: string
  app_version?: string
  platform: string
  sections: BackupSection[]
  total_files: number
  total_bytes: number
  excluded: BackupExclusion[]
  skipped?: BackupExclusion[]
}

export interface BackupInspection {
  id: string
  filename: string
  manifest: BackupManifest
  files: number
  bytes: number
  overwrite: number
  new: number
  ignored: number
  overwrite_sample?: string[]
  warnings?: string[]
}

export interface BackupRestoreResult {
  restored: number
  ignored: number
  warnings?: string[]
  restart_required: boolean
}

export class BackupApiError extends Error {
  code: string
  status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = "BackupApiError"
    this.code = code
    this.status = status
  }
}

async function parseError(res: Response): Promise<BackupApiError> {
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
  return new BackupApiError(code, message, res.status)
}

export function backupDownloadUrl(): string {
  return "/api/backup/export"
}

export async function inspectBackup(file: File): Promise<BackupInspection> {
  const form = new FormData()
  form.append("file", file, file.name)

  const res = await launcherFetch("/api/backup/inspect", {
    method: "POST",
    body: form,
  })
  if (!res.ok) {
    throw await parseError(res)
  }
  return (await res.json()) as BackupInspection
}

export async function restoreBackup(
  id: string,
): Promise<BackupRestoreResult> {
  const res = await launcherFetch("/api/backup/restore", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id, overwrite: true }),
  })
  if (!res.ok) {
    throw await parseError(res)
  }
  return (await res.json()) as BackupRestoreResult
}
