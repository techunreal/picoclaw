import { launcherFetch } from "@/api/http"

export interface SkillSupportItem {
  id: string
  name: string
  path: string
  directory: string
  source: "workspace" | "global" | "builtin" | string
  description: string
  origin_kind: "builtin" | "third_party" | "manual" | string
  editable: boolean
  name_mismatch?: boolean
  owner_handle?: string
  registry_slug?: string
  can_update?: boolean
  registry_name?: string
  registry_url?: string
  installed_version?: string
  installed_at?: number
}

export interface SkillDetailResponse extends SkillSupportItem {
  content: string
  raw_content: string
  revision: string
  size: number
}

export interface SkillWriteResponse {
  status: string
  id: string
  revision: string
  size: number
}

export interface SkillSecuritySummary {
  decision: "pass" | "warn" | "fail" | "unknown" | string
  status: string
  risk_level?: string
  summary?: string
  audit_url?: string
}

export interface SkillOwnerMatch {
  owner_handle: string
  slug: string
  ref?: string
  url?: string
}

export interface SkillRegistrySearchResult {
  score: number
  slug: string
  display_name: string
  summary: string
  version: string
  registry_name: string
  url?: string
  installed: boolean
  installed_name?: string
  installed_id?: string
  owner_handle?: string
  install_kind?: string
  install_reference?: string
  canonical_url?: string
  downloads?: number
  install_status?: "installable" | "unsupported" | "blocked" | string
  install_reason?: string
  security?: SkillSecuritySummary | null
}

export type SkillSearchSort = "relevance" | "downloads"

interface SkillsResponse {
  skills: SkillSupportItem[]
}

export interface SkillSearchResponse {
  results: SkillRegistrySearchResult[]
  limit: number
  offset: number
  next_offset?: number
  next_cursor?: string
  has_more: boolean
  sort?: SkillSearchSort
}

type SkillActionResponse = Partial<SkillSupportItem> & {
  status?: string
}

export interface InstallSkillRequest {
  slug: string
  registry: string
  version?: string
  force?: boolean
  owner_handle?: string
  install_kind?: string
  accept_suspicious?: boolean
}

export class SkillApiError extends Error {
  status: number
  code?: string
  matches?: SkillOwnerMatch[]
  summary?: string

  constructor(
    message: string,
    status: number,
    code?: string,
    matches?: SkillOwnerMatch[],
    summary?: string,
  ) {
    super(message)
    this.name = "SkillApiError"
    this.status = status
    this.code = code
    this.matches = matches
    this.summary = summary
  }
}

export interface InstallSkillResponse {
  status: string
  slug: string
  registry: string
  version: string
  summary?: string
  is_suspicious?: boolean
  skill?: SkillSupportItem
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await launcherFetch(path, options)
  if (!res.ok) {
    throw await readSkillApiError(res)
  }
  return res.json() as Promise<T>
}

export async function getSkills(agentId?: string): Promise<SkillsResponse> {
  const query = agentId ? `?agent=${encodeURIComponent(agentId)}` : ""
  return request<SkillsResponse>(`/api/skills${query}`)
}

export async function getSkill(name: string): Promise<SkillDetailResponse> {
  return request<SkillDetailResponse>(`/api/skills/${encodeURIComponent(name)}`)
}

export async function getSkillById(id: string): Promise<SkillDetailResponse> {
  return request<SkillDetailResponse>(
    `/api/skills/by-id/${encodeURIComponent(id)}`,
  )
}

export async function saveSkillContent(
  id: string,
  input: { content: string; revision: string; force?: boolean },
): Promise<SkillWriteResponse> {
  return request<SkillWriteResponse>(
    `/api/skills/by-id/${encodeURIComponent(id)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    },
  )
}

export async function searchSkills(
  query: string,
  limit = 20,
  offset = 0,
  sort: SkillSearchSort = "relevance",
): Promise<SkillSearchResponse> {
  const params = new URLSearchParams({
    q: query,
    limit: String(limit),
    offset: String(offset),
    sort,
  })
  return request<SkillSearchResponse>(`/api/skills/search?${params.toString()}`)
}

export async function getPopularSkills(
  limit = 20,
  cursor = "",
): Promise<SkillSearchResponse> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (cursor) {
    params.set("cursor", cursor)
  }
  return request<SkillSearchResponse>(
    `/api/skills/popular?${params.toString()}`,
  )
}

export async function installSkill(
  input: InstallSkillRequest,
): Promise<InstallSkillResponse> {
  return request<InstallSkillResponse>("/api/skills/install", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  })
}

export async function importSkill(file: File): Promise<SkillActionResponse> {
  const formData = new FormData()
  formData.set("file", file)

  const res = await launcherFetch("/api/skills/import", {
    method: "POST",
    body: formData,
  })
  if (!res.ok) {
    throw await readSkillApiError(res)
  }
  return res.json() as Promise<SkillActionResponse>
}

export async function deleteSkill(name: string): Promise<SkillActionResponse> {
  return request<SkillActionResponse>(
    `/api/skills/${encodeURIComponent(name)}`,
    {
      method: "DELETE",
    },
  )
}

export async function deleteSkillById(
  id: string,
): Promise<SkillActionResponse> {
  return request<SkillActionResponse>(
    `/api/skills/by-id/${encodeURIComponent(id)}`,
    {
      method: "DELETE",
    },
  )
}

async function readSkillApiError(res: Response): Promise<SkillApiError> {
  try {
    const raw = await res.text()
    if (raw.trim() === "") {
      return new SkillApiError(
        `API error: ${res.status} ${res.statusText}`,
        res.status,
      )
    }
    try {
      const body = JSON.parse(raw) as {
        error?: string
        errors?: string[]
        code?: string
        matches?: SkillOwnerMatch[]
        summary?: string
      }
      let message = `API error: ${res.status} ${res.statusText}`
      if (Array.isArray(body.errors) && body.errors.length > 0) {
        message = body.errors.join("; ")
      } else if (typeof body.error === "string" && body.error.trim() !== "") {
        message = body.error
      }
      return new SkillApiError(
        message,
        res.status,
        typeof body.code === "string" ? body.code : undefined,
        Array.isArray(body.matches) ? body.matches : undefined,
        typeof body.summary === "string" ? body.summary : undefined,
      )
    } catch {
      return new SkillApiError(raw.trim(), res.status)
    }
  } catch {
    return new SkillApiError(
      `API error: ${res.status} ${res.statusText}`,
      res.status,
    )
  }
}
