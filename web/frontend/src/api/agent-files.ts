import { launcherFetch } from "@/api/http"

export interface AgentFileItem {
  id: string
  group: string
  label: string
  path: string
  source?: string
  exists: boolean
  editable: boolean
  legacy?: boolean
  creatable: boolean
  size: number
  modified_at?: number
}

export interface AgentFileDetail extends AgentFileItem {
  content: string
  revision: string
}

export interface AgentFileGroup {
  id: string
  label: string
  files: AgentFileItem[]
}

export interface AgentFileListResponse {
  groups: AgentFileGroup[]
}

export interface AgentFileWriteResponse {
  status: string
  id: string
  revision: string
  size: number
  created: boolean
  modified_at: number
}

export interface AgentFileHistoryEntry {
  revision: string
  size: number
  created_at: number
}

export interface AgentFileHistoryResponse {
  id: string
  history: AgentFileHistoryEntry[]
}

export interface AgentFileHistoryContentResponse {
  revision: string
  content: string
  created_at: number
}

export interface AgentFileWriteRequest {
  content: string
  revision: string
}

export class AgentFileApiError extends Error {
  status: number
  code?: string
  revision?: string

  constructor(
    message: string,
    status: number,
    code?: string,
    revision?: string,
  ) {
    super(message)
    this.name = "AgentFileApiError"
    this.status = status
    this.code = code
    this.revision = revision
  }
}

async function parseError(res: Response): Promise<AgentFileApiError> {
  let code: string | undefined
  let revision: string | undefined
  let message = `API error: ${res.status} ${res.statusText}`

  try {
    const body = (await res.json()) as {
      error?: string
      code?: string
      revision?: string
    }
    if (typeof body.error === "string" && body.error.trim() !== "") {
      message = body.error
    }
    if (typeof body.code === "string") {
      code = body.code
    }
    if (typeof body.revision === "string") {
      revision = body.revision
    }
  } catch {
    // Keep the generic message when the body is not JSON.
  }

  return new AgentFileApiError(message, res.status, code, revision)
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await launcherFetch(path, options)
  if (!res.ok) {
    throw await parseError(res)
  }
  return res.json() as Promise<T>
}

function withAgent(path: string, agent?: string): string {
  if (!agent) return path
  const separator = path.includes("?") ? "&" : "?"
  return `${path}${separator}agent=${encodeURIComponent(agent)}`
}

function filePath(id: string, agent?: string): string {
  return withAgent(`/api/agent/files/${encodeURIComponent(id)}`, agent)
}

export async function listAgentFiles(
  agent?: string,
): Promise<AgentFileListResponse> {
  return request<AgentFileListResponse>(withAgent("/api/agent/files", agent))
}

export async function getAgentFile(
  id: string,
  agent?: string,
): Promise<AgentFileDetail> {
  return request<AgentFileDetail>(filePath(id, agent))
}

export async function saveAgentFile(
  id: string,
  input: AgentFileWriteRequest,
  agent?: string,
): Promise<AgentFileWriteResponse> {
  return request<AgentFileWriteResponse>(filePath(id, agent), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  })
}

export async function getAgentFileHistory(
  id: string,
  agent?: string,
): Promise<AgentFileHistoryResponse> {
  return request<AgentFileHistoryResponse>(
    `${filePath(id, agent)}/history`,
  )
}

export async function getAgentFileHistoryContent(
  id: string,
  revision: string,
  agent?: string,
): Promise<AgentFileHistoryContentResponse> {
  return request<AgentFileHistoryContentResponse>(
    `${filePath(id, agent)}/history/${encodeURIComponent(revision)}`,
  )
}

export async function restoreAgentFile(
  id: string,
  revision: string,
  agent?: string,
): Promise<AgentFileWriteResponse> {
  return request<AgentFileWriteResponse>(`${filePath(id, agent)}/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ revision }),
  })
}
