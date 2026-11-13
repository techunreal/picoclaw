import {
  IconHistory,
  IconLoader2,
  IconRotateClockwise,
} from "@tabler/icons-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import type { AgentFileHistoryEntry } from "@/api/agent-files"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"

import { formatBytes, formatTimestamp } from "./format"

interface HistorySheetProps {
  open: boolean
  fileName: string
  history: AgentFileHistoryEntry[]
  isLoading: boolean
  hasError: boolean
  isRestoring: boolean
  onOpenChange: (open: boolean) => void
  onRestore: (revision: string, createdAt: number) => void
}

export function HistorySheet({
  open,
  fileName,
  history,
  isLoading,
  hasError,
  isRestoring,
  onOpenChange,
  onRestore,
}: HistorySheetProps) {
  const { t } = useTranslation()
  const [confirming, setConfirming] = useState<string | null>(null)

  useEffect(() => {
    if (!open) {
      setConfirming(null)
    }
  }, [open])

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-0 p-0 data-[side=right]:sm:!w-[480px] data-[side=right]:sm:!max-w-[480px]"
      >
        <SheetHeader className="border-b px-6 py-5">
          <SheetTitle className="flex items-center gap-2">
            <IconHistory className="size-4" />
            {t("pages.agent.memory.history_title")}
          </SheetTitle>
          <SheetDescription>
            {t("pages.agent.memory.history_description", { name: fileName })}
          </SheetDescription>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto px-6 py-4">
          {isLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 4 }).map((_, index) => (
                <Skeleton key={index} className="h-14 w-full rounded-md" />
              ))}
            </div>
          ) : hasError ? (
            <p className="text-destructive text-sm">
              {t("pages.agent.memory.history_load_error")}
            </p>
          ) : history.length === 0 ? (
            <p className="text-muted-foreground py-8 text-center text-sm">
              {t("pages.agent.memory.history_empty")}
            </p>
          ) : (
            <ul className="space-y-2">
              {history.map((entry) => (
                <li
                  key={entry.revision}
                  className="border-border/50 flex items-center justify-between gap-3 rounded-md border px-3 py-2.5"
                >
                  <div className="min-w-0">
                    <div className="text-sm">
                      {formatTimestamp(entry.created_at)}
                    </div>
                    <div className="text-muted-foreground font-mono text-[11px]">
                      {entry.revision.slice(0, 12)} · {formatBytes(entry.size)}
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    {confirming === entry.revision ? (
                      <>
                        <Button
                          size="sm"
                          variant="destructive"
                          disabled={isRestoring}
                          onClick={() => {
                            setConfirming(null)
                            onRestore(entry.revision, entry.created_at)
                          }}
                        >
                          {isRestoring ? (
                            <IconLoader2 className="size-4 animate-spin" />
                          ) : (
                            <IconRotateClockwise className="size-4" />
                          )}
                          {t("pages.agent.memory.history_confirm_restore")}
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={isRestoring}
                          onClick={() => setConfirming(null)}
                        >
                          {t("pages.agent.memory.history_cancel")}
                        </Button>
                      </>
                    ) : (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={isRestoring}
                        onClick={() => setConfirming(entry.revision)}
                      >
                        <IconRotateClockwise className="size-4" />
                        {t("pages.agent.memory.history_restore")}
                      </Button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
