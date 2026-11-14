import type { TFunction } from "i18next"
import { toast } from "sonner"

import {
  ChatAttachmentError,
  extractChatAttachment,
} from "@/api/chat-attachments"
import type { ChatAttachment } from "@/store/chat"

const CHAT_IMAGE_MIME_TYPES = [
  "image/jpeg",
  "image/png",
  "image/gif",
  "image/webp",
  "image/bmp",
] as const

const CHAT_IMAGE_MIME_TYPE_SET = new Set<string>(CHAT_IMAGE_MIME_TYPES)
const CHAT_IMAGE_EXTENSION_BY_MIME: Record<string, string> = {
  "image/jpeg": ".jpg",
  "image/png": ".png",
  "image/gif": ".gif",
  "image/webp": ".webp",
  "image/bmp": ".bmp",
}
const CHAT_IMAGE_MIME_BY_EXTENSION: Record<string, string> = {
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".png": "image/png",
  ".gif": "image/gif",
  ".webp": "image/webp",
  ".bmp": "image/bmp",
}

// Keep in sync with web/backend/api/chat_attachments.go.
const CHAT_TEXT_EXTENSIONS = [
  ".txt",
  ".text",
  ".md",
  ".markdown",
  ".csv",
  ".tsv",
  ".json",
  ".jsonl",
  ".ndjson",
  ".log",
  ".yaml",
  ".yml",
  ".xml",
  ".html",
  ".htm",
  ".css",
  ".js",
  ".mjs",
  ".cjs",
  ".ts",
  ".tsx",
  ".jsx",
  ".go",
  ".py",
  ".rb",
  ".rs",
  ".java",
  ".kt",
  ".c",
  ".h",
  ".cpp",
  ".hpp",
  ".cs",
  ".sh",
  ".ps1",
  ".sql",
  ".ini",
  ".toml",
  ".cfg",
  ".conf",
  ".env",
  ".srt",
  ".vtt",
] as const

const CHAT_AUDIO_EXTENSIONS = [
  ".mp3",
  ".wav",
  ".m4a",
  ".ogg",
  ".oga",
  ".opus",
  ".flac",
  ".aac",
  ".wma",
  ".amr",
] as const

const CHAT_TEXT_EXTENSION_SET = new Set<string>(CHAT_TEXT_EXTENSIONS)
const CHAT_AUDIO_EXTENSION_SET = new Set<string>(CHAT_AUDIO_EXTENSIONS)

export const CHAT_ATTACHMENT_ACCEPT = [
  ...CHAT_IMAGE_MIME_TYPES,
  ".pdf",
  ...CHAT_TEXT_EXTENSIONS,
  ...CHAT_AUDIO_EXTENSIONS,
].join(",")

export const MAX_CHAT_ATTACHMENTS = 5

const MAX_CHAT_IMAGE_SIZE_BYTES = 7 * 1024 * 1024
const MAX_CHAT_TEXT_FILE_SIZE_BYTES = 2 * 1024 * 1024
const MAX_CHAT_DOCUMENT_SIZE_BYTES = 10 * 1024 * 1024
const MAX_CHAT_AUDIO_SIZE_BYTES = 10 * 1024 * 1024

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      if (typeof reader.result === "string") {
        resolve(reader.result)
        return
      }
      reject(new Error("Failed to read file"))
    }
    reader.onerror = () =>
      reject(reader.error || new Error("Failed to read file"))
    reader.readAsDataURL(file)
  })
}

function getFileExtension(fileName: string): string {
  const lastDotIndex = fileName.lastIndexOf(".")
  if (lastDotIndex === -1) {
    return ""
  }
  return fileName.slice(lastDotIndex).toLowerCase()
}

function getSupportedImageMimeType(file: File): string | null {
  const normalizedType = file.type.trim().toLowerCase()
  if (normalizedType && CHAT_IMAGE_MIME_TYPE_SET.has(normalizedType)) {
    return normalizedType
  }

  const extension = getFileExtension(file.name)
  return CHAT_IMAGE_MIME_BY_EXTENSION[extension] ?? null
}

function normalizeImageFileForDataUrl(file: File, filename: string): File {
  const mimeType = getSupportedImageMimeType(file)
  if (!mimeType || file.type.trim().toLowerCase() === mimeType) {
    return file
  }

  const normalizedName = file.name.trim() || filename
  return new File([file], normalizedName, { type: mimeType })
}

function getAttachmentFilename(file: File, index: number): string {
  const trimmedName = file.name.trim()
  if (trimmedName) {
    return trimmedName
  }

  const mimeType = getSupportedImageMimeType(file)
  const extension = mimeType ? CHAT_IMAGE_EXTENSION_BY_MIME[mimeType] : ".png"
  return `image-${index + 1}${extension}`
}

function getTransferItemFiles(dataTransfer: DataTransfer | null): File[] {
  if (!dataTransfer) {
    return []
  }

  const files = Array.from(dataTransfer.files)
  if (files.length > 0) {
    return files
  }

  return Array.from(dataTransfer.items)
    .filter((item) => item.kind === "file")
    .map((item) => item.getAsFile())
    .filter((file): file is File => file !== null)
}

export function hasFileTransfer(dataTransfer: DataTransfer | null): boolean {
  if (!dataTransfer) {
    return false
  }

  if (dataTransfer.files.length > 0) {
    return true
  }

  return Array.from(dataTransfer.items).some((item) => item.kind === "file")
}

export function getTransferredFiles(dataTransfer: DataTransfer | null) {
  return getTransferItemFiles(dataTransfer)
}

type ChatFileKind = "image" | "pdf" | "text" | "audio" | "unsupported"

function classifyFile(file: File): ChatFileKind {
  if (getSupportedImageMimeType(file)) {
    return "image"
  }

  const extension = getFileExtension(file.name)
  if (extension === ".pdf") {
    return "pdf"
  }
  if (CHAT_TEXT_EXTENSION_SET.has(extension)) {
    return "text"
  }
  if (CHAT_AUDIO_EXTENSION_SET.has(extension)) {
    return "audio"
  }
  return "unsupported"
}

function fileSizeLimitForKind(kind: ChatFileKind): number {
  switch (kind) {
    case "image":
      return MAX_CHAT_IMAGE_SIZE_BYTES
    case "text":
      return MAX_CHAT_TEXT_FILE_SIZE_BYTES
    case "audio":
      return MAX_CHAT_AUDIO_SIZE_BYTES
    default:
      return MAX_CHAT_DOCUMENT_SIZE_BYTES
  }
}

function formatSizeLabel(bytes: number): string {
  return `${Math.round(bytes / (1024 * 1024))} MB`
}

async function buildImageAttachment(
  file: File,
  filename: string,
  t: TFunction,
): Promise<ChatAttachment | null> {
  const mimeType = getSupportedImageMimeType(file)
  if (!mimeType) {
    toast.error(t("chat.invalidImage", { name: filename }))
    return null
  }

  if (file.size > MAX_CHAT_IMAGE_SIZE_BYTES) {
    toast.error(
      t("chat.imageTooLarge", {
        name: filename,
        size: formatSizeLabel(MAX_CHAT_IMAGE_SIZE_BYTES),
      }),
    )
    return null
  }

  try {
    const normalizedFile = normalizeImageFileForDataUrl(file, filename)
    return {
      type: "image",
      filename,
      url: await readFileAsDataUrl(normalizedFile),
      contentType: mimeType,
    }
  } catch {
    toast.error(t("chat.imageReadFailed", { name: filename }))
    return null
  }
}

async function buildFileAttachment(
  file: File,
  kind: ChatFileKind,
  filename: string,
  t: TFunction,
): Promise<ChatAttachment | null> {
  const limit = fileSizeLimitForKind(kind)
  if (file.size > limit) {
    toast.error(
      t("chat.fileTooLarge", {
        name: filename,
        size: formatSizeLabel(limit),
      }),
    )
    return null
  }

  try {
    const extracted = await extractChatAttachment(file)
    const contentType = extracted.content_type || file.type
    const text = extracted.text ?? ""
    if (!text.trim()) {
      toast.error(t("chat.emptyFileText", { name: filename }))
      return null
    }
    if (extracted.truncated) {
      toast.warning(t("chat.fileTruncated", { name: filename }))
    }
    return {
      type: kind === "audio" ? "audio" : "file",
      url: "",
      filename: extracted.filename || filename,
      contentType,
      text,
    }
  } catch (error) {
    const message =
      error instanceof ChatAttachmentError
        ? error.message
        : t("chat.fileReadFailed", { name: filename })
    toast.error(message)
    return null
  }
}

/**
 * Turns selected/dropped files into chat attachments. Images stay inline as
 * data URLs; PDF/text/audio files are extracted on the launcher and carried as
 * text so any provider can read them.
 */
export async function buildChatAttachments(
  files: readonly File[],
  t: TFunction,
): Promise<ChatAttachment[]> {
  const attachments: ChatAttachment[] = []

  for (const [index, file] of files.entries()) {
    const filename = getAttachmentFilename(file, index)
    const kind = classifyFile(file)

    if (kind === "unsupported") {
      toast.error(t("chat.unsupportedFile", { name: filename }))
      continue
    }

    const attachment =
      kind === "image"
        ? await buildImageAttachment(file, filename, t)
        : await buildFileAttachment(file, kind, filename, t)

    if (attachment) {
      attachments.push(attachment)
    }
  }

  return attachments
}
