import { launcherFetch } from "@/api/http"

export type MCPTransport = "stdio" | "http" | "sse"

export interface MCPServer {
  name: string
  enabled: boolean
  deferred?: boolean | null
  type: MCPTransport
  command: string
  args: string[]
  env: Record<string, string>
  env_file: string
  url: string
  headers: Record<string, string>
}

export interface MCPDiscoverySettings {
  enabled: boolean
  ttl: number
  max_search_results: number
  use_bm25: boolean
  use_regex: boolean
}

export interface MCPServersResponse {
  enabled: boolean
  servers: MCPServer[]
  discovery: MCPDiscoverySettings
}

export interface MCPServerPayload {
  enabled: boolean
  deferred?: boolean | null
  type: MCPTransport
  command: string
  args: string[]
  env: Record<string, string>
  env_file: string
  url: string
  headers: Record<string, string>
}

export interface MCPTestResult {
  ok: boolean
  tool_count: number
  tools?: string[]
  error?: string
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
      // keep generic
    }
    throw new Error(message)
  }
  return (await res.json()) as T
}

export function getMCPServers(): Promise<MCPServersResponse> {
  return request<MCPServersResponse>("/api/mcp/servers")
}

export function updateMCPSettings(payload: {
  enabled: boolean
  discovery?: MCPDiscoverySettings
}): Promise<MCPServersResponse> {
  return request<MCPServersResponse>("/api/mcp/settings", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export function saveMCPServer(
  name: string,
  payload: MCPServerPayload,
): Promise<MCPServersResponse> {
  return request<MCPServersResponse>(
    `/api/mcp/servers/${encodeURIComponent(name)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    },
  )
}

export function deleteMCPServer(name: string): Promise<MCPServersResponse> {
  return request<MCPServersResponse>(
    `/api/mcp/servers/${encodeURIComponent(name)}`,
    { method: "DELETE" },
  )
}

export function testMCPServer(name: string): Promise<MCPTestResult> {
  return request<MCPTestResult>(
    `/api/mcp/servers/${encodeURIComponent(name)}/test`,
    { method: "POST" },
  )
}
