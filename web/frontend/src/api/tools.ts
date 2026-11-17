import { launcherFetch } from "@/api/http"

export interface ToolSupportItem {
  name: string
  description: string
  category: string
  config_key: string
  status: "enabled" | "disabled" | "blocked"
  reason_code?: string
}

interface ToolsResponse {
  tools: ToolSupportItem[]
}

interface ToolActionResponse {
  status: string
}

export interface WebSearchProviderOption {
  id: string
  label: string
  configured: boolean
  current: boolean
  requires_auth: boolean
}

export interface WebSearchProviderConfig {
  enabled: boolean
  max_results: number
  base_url?: string
  api_key?: string
  api_keys?: string[]
  model?: string
  api_key_set?: boolean
}

export interface WebSearchConfigResponse {
  provider: string
  current_service: string
  prefer_native: boolean
  proxy?: string
  providers: WebSearchProviderOption[]
  settings: Record<string, WebSearchProviderConfig>
}

export interface BrowserToolStatus {
  binary_found: boolean
  binary_path?: string
  binary_version?: string
  browser_detected: boolean
  browser_path?: string
  error?: string
}

export interface BrowserToolConfig {
  enabled: boolean
  allowed_domains: string[]
  allow_all_domains: boolean
  permission_policy: "ask" | "allow" | "deny"
  binary_path: string
  browser_path: string
  headless: boolean
  max_output: number
  timeout_seconds: number
  allow_remote: boolean
  restart_required?: boolean
  status: BrowserToolStatus
}

export interface BrowserToolTestResult {
  ok: boolean
  version?: string
  error?: string
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await launcherFetch(path, options)
  if (!res.ok) {
    const fallback = `API error: ${res.status} ${res.statusText}`
    const raw = await res.text().catch(() => "")
    let message = fallback
    const trimmed = raw.trim()
    if (trimmed !== "") {
      try {
        const body = JSON.parse(trimmed) as {
          error?: string
          errors?: string[]
        }
        if (Array.isArray(body.errors) && body.errors.length > 0) {
          message = body.errors.join("; ")
        } else if (
          typeof body.error === "string" &&
          body.error.trim() !== ""
        ) {
          message = body.error
        } else {
          message = trimmed
        }
      } catch {
        // Plain-text errors (http.Error) carry the actual reason.
        message = trimmed
      }
    }
    throw new Error(message.length > 300 ? `${message.slice(0, 300)}…` : message)
  }
  return res.json() as Promise<T>
}

export async function getTools(): Promise<ToolsResponse> {
  return request<ToolsResponse>("/api/tools")
}

export async function setToolEnabled(
  name: string,
  enabled: boolean,
): Promise<ToolActionResponse> {
  return request<ToolActionResponse>(
    `/api/tools/${encodeURIComponent(name)}/state`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ enabled }),
    },
  )
}

export async function getWebSearchConfig(): Promise<WebSearchConfigResponse> {
  return request<WebSearchConfigResponse>("/api/tools/web-search-config")
}

export async function updateWebSearchConfig(
  payload: WebSearchConfigResponse,
): Promise<WebSearchConfigResponse> {
  return request<WebSearchConfigResponse>("/api/tools/web-search-config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function getBrowserToolConfig(): Promise<BrowserToolConfig> {
  return request<BrowserToolConfig>("/api/tools/browser")
}

export async function updateBrowserToolConfig(
  payload: Omit<BrowserToolConfig, "status">,
): Promise<BrowserToolConfig> {
  return request<BrowserToolConfig>("/api/tools/browser", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function testBrowserTool(): Promise<BrowserToolTestResult> {
  return request<BrowserToolTestResult>("/api/tools/browser/test", {
    method: "POST",
  })
}

export interface BrowserInstallStatus {
  running: boolean
  done: boolean
  error?: string
  last_line?: string
  started_at?: number
  finished_at?: number
}

export async function installBrowserEngine(): Promise<BrowserInstallStatus> {
  return request<BrowserInstallStatus>("/api/tools/browser/install", {
    method: "POST",
  })
}

export async function getBrowserInstallStatus(): Promise<BrowserInstallStatus> {
  return request<BrowserInstallStatus>("/api/tools/browser/install")
}
