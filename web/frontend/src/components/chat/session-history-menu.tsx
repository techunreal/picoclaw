import { IconHistory, IconTrash } from "@tabler/icons-react"
import dayjs from "dayjs"
import type { TFunction } from "i18next"
import type { RefObject } from "react"
import { useTranslation } from "react-i18next"

import type { SessionSummary } from "@/api/sessions"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { ScrollArea } from "@/components/ui/scroll-area"
import { parseAttachmentBlocks } from "@/features/chat/attachment-blocks"

function sessionTitle(title: string, t: TFunction): string {
  const parsed = parseAttachmentBlocks(title)
  let text = parsed.text

  // The session list truncates long previews, so an attachment block may be
  // cut off before its end marker. Drop anything from the header on.
  const headerIndex = text.indexOf("[Attached file:")
  if (headerIndex >= 0) {
    text = text.slice(0, headerIndex)
  }
  text = text.replace(/\s+/g, " ").trim()
  if (text) {
    return text
  }

  const filename = parsed.attachments[0]?.filename
  if (filename) {
    return filename
  }

  const partialFilename = title.match(/\[Attached file:\s*([^\n()\]]+)/)?.[1]
  return partialFilename?.trim() || t("chat.attachedFile")
}

interface SessionHistoryMenuProps {
  sessions: SessionSummary[]
  activeSessionId: string
  hasMore: boolean
  loadError: boolean
  loadErrorMessage: string
  observerRef: RefObject<HTMLDivElement | null>
  onOpenChange: (open: boolean) => void
  onSwitchSession: (sessionId: string) => void
  onDeleteSession: (sessionId: string) => void
}

export function SessionHistoryMenu({
  sessions,
  activeSessionId,
  hasMore,
  loadError,
  loadErrorMessage,
  observerRef,
  onOpenChange,
  onSwitchSession,
  onDeleteSession,
}: SessionHistoryMenuProps) {
  const { t } = useTranslation()

  return (
    <DropdownMenu onOpenChange={onOpenChange}>
      <DropdownMenuTrigger asChild>
        <Button variant="secondary" size="sm" className="h-9 gap-2">
          <IconHistory className="size-4" />
          <span className="hidden sm:inline">{t("chat.history")}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72">
        <ScrollArea className="max-h-[300px]">
          {loadError && (
            <DropdownMenuItem disabled>
              <span className="text-destructive text-xs">
                {loadErrorMessage}
              </span>
            </DropdownMenuItem>
          )}
          {sessions.length === 0 && !loadError ? (
            <DropdownMenuItem disabled>
              <span className="text-muted-foreground text-xs">
                {t("chat.noHistory")}
              </span>
            </DropdownMenuItem>
          ) : (
            sessions.map((session) => (
              <DropdownMenuItem
                key={session.id}
                className={`group relative my-0.5 flex flex-col items-start gap-0.5 pr-8 ${
                  session.id === activeSessionId ? "bg-accent" : ""
                }`}
                onClick={() => onSwitchSession(session.id)}
              >
                <span className="line-clamp-1 text-sm font-medium">
                  {sessionTitle(session.title, t)}
                </span>
                <span className="text-muted-foreground text-xs">
                  {t("chat.messagesCount", {
                    count: session.message_count,
                  })}{" "}
                  · {dayjs(session.updated).fromNow()}
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t("chat.deleteSession")}
                  className="text-muted-foreground hover:bg-destructive/10 hover:text-destructive absolute top-1/2 right-2 h-6 w-6 -translate-y-1/2 opacity-0 transition-opacity group-hover:opacity-100"
                  onClick={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    onDeleteSession(session.id)
                  }}
                >
                  <IconTrash className="h-4 w-4" />
                </Button>
              </DropdownMenuItem>
            ))
          )}
          {hasMore && sessions.length > 0 && (
            <div ref={observerRef} className="py-2 text-center">
              <span className="text-muted-foreground animate-pulse text-xs">
                {t("chat.loadingMore")}
              </span>
            </div>
          )}
        </ScrollArea>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
