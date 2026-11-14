import { launcherFetch } from "@/api/http"

export interface ChatAttachmentExtraction {
  filename: string
  content_type: string
  kind: "text" | "pdf" | "audio"
  size: number
  text?: string
  truncated?: boolean
  transcript?: boolean
}

export class ChatAttachmentError extends Error {
  code: string

  constructor(code: string, message: string) {
    super(message)
    this.name = "ChatAttachmentError"
    this.code = code
  }
}

/** Uploads one file and returns the text that should be injected into the chat. */
export async function extractChatAttachment(
  file: File,
): Promise<ChatAttachmentExtraction> {
  const form = new FormData()
  form.append("file", file, file.name)

  const res = await launcherFetch("/api/chat/attachments", {
    method: "POST",
    body: form,
  })

  if (!res.ok) {
    let code = "upload_failed"
    let message = `Could not read this file (${res.status}).`
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
    throw new ChatAttachmentError(code, message)
  }

  return (await res.json()) as ChatAttachmentExtraction
}
