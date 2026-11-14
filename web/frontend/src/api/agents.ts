import { launcherFetch } from "@/api/http"

export interface AgentModelConfig {
  primary?: string
  fallbacks?: string[]
}

export interface AgentSubagentsConfig {
  enabled?: boolean
  allow_agents?: string[]
  model?: AgentModelConfig
}

export interface AgentConfig {
  id: string
  name?: string
  default?: boolean
  workspace?: string
  model?: AgentModelConfig
  skills?: string[]
  subagents?: AgentSubagentsConfig
}

export interface AgentView extends AgentConfig {
  effective_workspace: string
  workspace_source: "config" | "default" | "derived" | string
  effective_model?: string
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
  implicit: boolean
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

export function toAgentConfig(agent: AgentView): AgentConfig {
  return {
    id: agent.id,
    name: agent.name,
    default: agent.default,
    workspace: agent.workspace,
    model: agent.model,
    skills: agent.skills,
    subagents: agent.subagents,
  }
}
