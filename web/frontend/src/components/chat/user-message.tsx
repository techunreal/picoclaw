import { IconCheck, IconCopy, IconPaperclip } from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { stripAttachmentBlocks } from "@/features/chat/attachment-blocks"
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard"
import { formatMessageTime } from "@/hooks/use-pico-chat"
import { cn } from "@/lib/utils"
import type { ChatAttachment } from "@/store/chat"

interface UserMessageProps {
  content: string
  attachments?: ChatAttachment[]
  timestamp?: string | number
}

export function UserMessage({
  content,
  attachments = [],
  timestamp = "",
}: UserMessageProps) {
  const { t } = useTranslation()
  const { copy, isCopied } = useCopyToClipboard()
  const displayContent = stripAttachmentBlocks(content)
  const hasText = displayContent.trim().length > 0
  const isCommand = displayContent.trim().startsWith("/")
  const imageAttachments = attachments.filter(
    (attachment) => attachment.type === "image",
  )
  const fileAttachments = attachments.filter(
    (attachment) => attachment.type !== "image",
  )
  const copyMessageLabel = isCopied
    ? t("chat.copiedLabel")
    : t("chat.copyMessage")
  const formattedTimestamp =
    timestamp !== "" ? formatMessageTime(timestamp) : ""

  return (
    <div className="group flex w-full flex-col items-end gap-1.5">
      {imageAttachments.length > 0 && (
        <div className="flex max-w-[70%] flex-wrap justify-end gap-2">
          {imageAttachments.map((attachment, index) => (
            <img
              key={`${attachment.url}-${index}`}
              src={attachment.url}
              alt={attachment.filename || t("chat.uploadedImage")}
              className="max-h-72 max-w-full object-cover"
            />
          ))}
        </div>
      )}

      {fileAttachments.length > 0 && (
        <div className="flex max-w-[70%] flex-wrap justify-end gap-2">
          {fileAttachments.map((attachment, index) => (
            <div
              key={`${attachment.filename}-${index}`}
              className="bg-background/80 flex max-w-full items-center gap-2 rounded-xl border px-3 py-2 text-[13px] shadow-xs"
              title={attachment.filename}
            >
              <IconPaperclip className="text-muted-foreground h-4 w-4 shrink-0" />
              <span className="truncate">
                {attachment.filename || t("chat.attachedFile")}
              </span>
              <span className="text-muted-foreground shrink-0 text-[11px]">
                {attachment.contentType?.startsWith("audio/")
                  ? t("chat.audioTranscript")
                  : t("chat.extractedText")}
              </span>
            </div>
          ))}
        </div>
      )}

      {hasText && (
        <div className="relative max-w-[70%]">
          <div
            className={cn(
              "wrap-break-word whitespace-pre-wrap",
              isCommand
                ? "rounded-xl border border-zinc-200 bg-transparent px-4 py-3 font-mono text-[14px] text-zinc-800 dark:border-zinc-800/60 dark:bg-[#121212] dark:text-zinc-200 dark:shadow-sm"
                : "rounded-2xl rounded-tr-sm bg-violet-500 px-5 py-3 text-[15px] leading-relaxed text-white shadow-sm",
            )}
          >
            {isCommand ? (
              <div className="flex items-start gap-2.5">
                <span className="font-bold text-emerald-600 select-none dark:text-emerald-400">
                  ❯
                </span>
                <span className="mt-[1px]">{displayContent}</span>
              </div>
            ) : (
              displayContent
            )}
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={cn(
              "bg-background/75 hover:bg-background absolute top-2 right-2 h-7 w-7 opacity-0 shadow-xs transition-opacity group-hover:opacity-100",
              isCommand
                ? "text-zinc-700 dark:text-zinc-200"
                : "text-violet-700 dark:text-violet-100",
            )}
            onClick={() => void copy(displayContent)}
            aria-label={copyMessageLabel}
            title={copyMessageLabel}
          >
            {isCopied ? (
              <IconCheck className="h-4 w-4 text-green-500" />
            ) : (
              <IconCopy className="h-4 w-4" />
            )}
          </Button>
        </div>
      )}

      {formattedTimestamp && (
        <span className="px-1 text-[12px] text-zinc-400">
          {formattedTimestamp}
        </span>
      )}
    </div>
  )
}
