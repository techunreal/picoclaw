import { launcherFetch } from "@/api/http"

export interface AgentModelConfig {
  primary?: string
  fallbacks?: string[]
}

export type AgentModelValue = string | AgentModelConfig

export function agentModelPrimary(
  value?: AgentModelValue | null,
): string {
  if (!value) return ""
  if (typeof value === "string") return value.trim()
  return value.primary?.trim() || ""
}

export function agentModelFallbacks(
  value?: AgentModelValue | null,
): string[] | undefined {
  if (!value || typeof value === "string") return undefined
  return value.fallbacks
}

export interface AgentSubagentsConfig {
  enabled?: boolean
  allow_agents?: string[]
  model?: AgentModelConfig
}

export interface AgentAvatar {
  emoji?: string
  image?: string
  version?: number
}

export interface AgentConfig {
  id: string
  name?: string
  default?: boolean
  workspace?: string
  model?: AgentModelValue
  skills?: string[]
  subagents?: AgentSubagentsConfig
  avatar?: AgentAvatar
}

export interface AgentView extends AgentConfig {
  effective_workspace: string
  workspace_source: "config" | "default" | "derived" | string
  effective_model?: string
  model_source?: "global" | "override" | "frontmatter" | string
}

export interface SetAgentModelResponse {
  status: string
  agent: string
  model: string
  effective_model?: string
  applied?: boolean
  pending_agents?: string[]
  gateway_unavailable?: boolean
  warnings?: string[]
  message?: string
  restart_required?: boolean
}

export interface DispatchSelector {
  channel?: string
  account?: string
  space?: string
  chat?: string
  topic?: string
  sender?: string
  mentioned?: boolean
}

export interface DispatchRule {
  name?: string
  agent: string
  when: DispatchSelector
  session_dimensions?: string[]
}

export interface DispatchConfig {
  rules: DispatchRule[]
}

export interface AgentDefaults {
  workspace: string
  provider: string
  model_name: string
}

export interface AgentsResponse {
  agents: AgentView[]
  defaults: AgentDefaults
  dispatch?: DispatchConfig
  hidden?: string[]
  implicit: boolean
  warnings?: string[]
}

export interface ProfileIdentity {
  name?: string
  emoji?: string
  vibe?: string
}

export interface ProfileFilePreview {
  source: string
  target: string
  size: number
  preview: string
}

export interface ProfilePreview {
  root: string
  files: ProfileFilePreview[]
  identity: ProfileIdentity
  suggested_id: string
  combined: boolean
  ignored: string[]
  ignored_count: number
  notes?: string[]
}

export interface ProfileUpload {
  files?: File[]
  zip?: File
}

export interface ProfileImportResult {
  status: string
  restart_required: boolean
  agent: string
  workspace: string
  written: string[]
}

export class AgentApiError extends Error {
  code: string
  status: number
  errors: string[]

  constructor(code: string, message: string, status: number, errors: string[] = []) {
    super(message)
    this.name = "AgentApiError"
    this.code = code
    this.status = status
    this.errors = errors
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await launcherFetch(path, options)
  if (!res.ok) {
    let code = "request_failed"
    let message = `Request failed (${res.status}).`
    let errors: string[] = []
    try {
      const body = (await res.json()) as {
        code?: string
        error?: string
        errors?: string[]
      }
      if (typeof body.code === "string" && body.code.trim() !== "") {
        code = body.code
      }
      if (typeof body.error === "string" && body.error.trim() !== "") {
        message = body.error
      }
      if (Array.isArray(body.errors)) {
        errors = body.errors.filter((err): err is string => typeof err === "string")
      }
    } catch {
      // Keep the generic message.
    }
    throw new AgentApiError(code, message, res.status, errors)
  }
  return (await res.json()) as T
}

export function getAgents(): Promise<AgentsResponse> {
  return request<AgentsResponse>("/api/agents")
}

export function updateAgents(
  agents: AgentConfig[],
  dispatch: DispatchConfig | null,
): Promise<{ status: string; restart_required: boolean }> {
  return request<{ status: string; restart_required: boolean }>("/api/agents", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ agents, dispatch }),
  })
}

export function setAgentModel(
  agentId: string,
  model: string | null,
): Promise<SetAgentModelResponse> {
  const body = model === null ? { use_default: true } : { model }
  return request<SetAgentModelResponse>(
    `/api/agents/${encodeURIComponent(agentId)}/model`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    },
  )
}

export function updateAgentOrder(ids: string[]): Promise<{ status: string }> {  return request<{ status: string }>("/api/agents/order", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
  })
}

export function updateHiddenAgents(
  hidden: string[],
): Promise<{ status: string; hidden: string[] }> {
  return request<{ status: string; hidden: string[] }>("/api/agents/hidden", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ hidden }),
  })
}

export function toAgentConfig(agent: AgentView): AgentConfig {
  return {
    id: agent.id,
    name: agent.name,
    default: agent.default,
    workspace: agent.workspace,
    model: agent.model,
    skills: agent.skills,
    subagents: agent.subagents,
    avatar: agent.avatar,
  }
}

export function agentAvatarUrl(agentId: string, version?: number): string {
  const base = `/api/agents/${encodeURIComponent(agentId)}/avatar`
  return version ? `${base}?v=${version}` : base
}

export function uploadAgentAvatar(
  agentId: string,
  file: File,
): Promise<{ status: string; avatar: AgentAvatar }> {
  const body = new FormData()
  body.append("file", file)
  return request<{ status: string; avatar: AgentAvatar }>(
    `/api/agents/${encodeURIComponent(agentId)}/avatar`,
    { method: "POST", body },
  )
}

export function deleteAgentAvatar(agentId: string): Promise<{ status: string }> {
  return request<{ status: string }>(
    `/api/agents/${encodeURIComponent(agentId)}/avatar`,
    { method: "DELETE" },
  )
}


export interface DeleteAgentResponse {
  status: string
  id: string
  workspace?: string
  files_deleted: boolean
  warning?: string
  restart_required?: boolean
}

export function deleteAgent(agentId: string): Promise<DeleteAgentResponse> {
  return request<DeleteAgentResponse>(
    `/api/agents/${encodeURIComponent(agentId)}`,
    { method: "DELETE" },
  )
}

export async function exportAgentBundle(agentId: string): Promise<Blob> {
  const res = await launcherFetch(
    `/api/agents/${encodeURIComponent(agentId)}/export`,
  )
  if (!res.ok) {
    let message = `Export failed (${res.status}).`
    try {
      const body = (await res.json()) as { error?: string }
      if (typeof body.error === "string" && body.error.trim() !== "") {
        message = body.error
      }
    } catch {
      // keep generic
    }
    throw new AgentApiError("export_failed", message, res.status)
  }
  return res.blob()
}

export interface ImportAgentBundleResponse {
  status: string
  agent: string
  name?: string
  renamed?: boolean
  restart_required?: boolean
}

export function importAgentBundle(
  file: File,
): Promise<ImportAgentBundleResponse> {
  const body = new FormData()
  body.append("file", file)
  return request<ImportAgentBundleResponse>("/api/agents/import-bundle", {
    method: "POST",
    body,
  })
}

function buildProfileFormData(upload: ProfileUpload): FormData {
  const body = new FormData()
  if (upload.zip) {
    body.append("zip", upload.zip)
    return body
  }
  for (const file of upload.files ?? []) {
    body.append("files", file)
    body.append(
      "paths",
      (file as File & { webkitRelativePath?: string }).webkitRelativePath ||
        file.name,
    )
  }
  return body
}

export function inspectAgentProfile(upload: ProfileUpload): Promise<ProfilePreview> {
  return request<ProfilePreview>("/api/agents/import/inspect", {
    method: "POST",
    body: buildProfileFormData(upload),
  })
}

export function importAgentProfile(
  upload: ProfileUpload,
  values: {
    agent: AgentConfig
    overwrite?: boolean
    tools?: string[]
  },
): Promise<ProfileImportResult> {
  const body = buildProfileFormData(upload)
  body.append("agent", JSON.stringify(values.agent))
  body.append("overwrite", values.overwrite ? "true" : "false")
  if (values.tools !== undefined) {
    body.append("tools", JSON.stringify(values.tools))
  }
  return request<ProfileImportResult>("/api/agents/import", {
    method: "POST",
    body,
  })
}
