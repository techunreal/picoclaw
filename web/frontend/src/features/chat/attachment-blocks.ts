import type { ChatAttachment } from "@/store/chat"

const ATTACHMENT_BLOCK_PATTERN =
  /\[Attached file: ([^\]\n]+?)(?: \(([^)\n]+)\))?\]\n----- BEGIN ATTACHMENT -----\n([\s\S]*?)\n----- END ATTACHMENT -----/g

export interface ParsedAttachmentBlocks {
  text: string
  attachments: ChatAttachment[]
}

/**
 * Splits attachment blocks injected by the pico channel out of a stored user
 * message. The agent needs the file text in the prompt, but the chat bubble
 * should only show the user's own words plus a compact attachment chip.
 */
export function parseAttachmentBlocks(content: string): ParsedAttachmentBlocks {
  const attachments: ChatAttachment[] = []
  let text = content

  ATTACHMENT_BLOCK_PATTERN.lastIndex = 0
  const matches = Array.from(content.matchAll(ATTACHMENT_BLOCK_PATTERN))
  for (const match of matches) {
    const [block, filename, contentType, fileText] = match
    attachments.push({
      type: "file",
      url: "",
      filename: filename?.trim() || undefined,
      contentType: contentType?.trim() || undefined,
      text: fileText ?? "",
    })
    text = text.replace(block, "")
  }

  return {
    text: text.replace(/\n{3,}/g, "\n\n").trim(),
    attachments,
  }
}

export function stripAttachmentBlocks(content: string): string {
  return parseAttachmentBlocks(content).text
}
