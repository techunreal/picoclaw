import { launcherFetch } from "@/api/http"

export type BrowserApprovalDecision = "once" | "always" | "deny"

export interface BrowserApproval {
  id: string
  domain: string
  created_at: number
}

export async function listBrowserApprovals(): Promise<BrowserApproval[]> {
  const res = await launcherFetch("/api/browser/approvals")
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`)
  }
  const body = (await res.json()) as { approvals?: BrowserApproval[] }
  return body.approvals ?? []
}

export async function resolveBrowserApproval(
  id: string,
  decision: BrowserApprovalDecision,
): Promise<void> {
  const res = await launcherFetch(
    `/api/browser/approvals/${encodeURIComponent(id)}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ decision }),
    },
  )
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
}
