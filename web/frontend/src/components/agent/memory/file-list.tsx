import { IconFileText } from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import type { AgentFileGroup } from "@/api/agent-files"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

import { agentFileGroupLabel, agentFileLabel, formatBytes } from "./format"

interface FileListProps {
  groups: AgentFileGroup[]
  selectedId: string | null
  isLoading: boolean
  hasError: boolean
  onSelect: (id: string) => void
}

export function FileList({
  groups,
  selectedId,
  isLoading,
  hasError,
  onSelect,
}: FileListProps) {
  const { t } = useTranslation()

  return (
    <aside className="border-border/40 flex w-full shrink-0 flex-col gap-4 overflow-y-auto border-b pb-4 md:w-72 md:border-r md:border-b-0 md:pr-4 md:pb-0">
      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className="h-9 w-full rounded-md" />
          ))}
        </div>
      ) : hasError ? (
        <p className="text-destructive text-sm">
          {t("pages.agent.memory.load_error")}
        </p>
      ) : (
        groups.map((group) => (
          <div key={group.id} className="space-y-1">
            <div className="text-muted-foreground px-2 text-[11px] font-medium tracking-wide uppercase">
              {agentFileGroupLabel(group.id, group.label, t)}
            </div>
            {group.files.length === 0 ? (
              <p className="text-muted-foreground/70 px-2 py-1 text-xs">
                {t("pages.agent.memory.empty_group")}
              </p>
            ) : (
              group.files.map((item) => {
                const isActive = item.id === selectedId
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => onSelect(item.id)}
                    className={cn(
                      "hover:bg-muted/60 flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm transition-colors",
                      isActive
                        ? "bg-accent text-accent-foreground font-medium"
                        : "text-muted-foreground",
                      !item.exists && "opacity-70",
                    )}
                  >
                    <IconFileText
                      className={cn(
                        "size-4 shrink-0",
                        isActive ? "opacity-100" : "opacity-60",
                      )}
                    />
                    <span className="truncate">{agentFileLabel(item, t)}</span>
                    <span className="ml-auto flex shrink-0 items-center gap-1">
                      {item.legacy && (
                        <Badge variant="secondary" className="text-[10px]">
                          {t("pages.agent.memory.legacy")}
                        </Badge>
                      )}
                      {!item.editable && (
                        <Badge variant="outline" className="text-[10px]">
                          {t("pages.agent.memory.read_only")}
                        </Badge>
                      )}
                      {item.exists ? (
                        <span className="text-muted-foreground/60 hidden text-[10px] tabular-nums lg:inline">
                          {formatBytes(item.size)}
                        </span>
                      ) : (
                        <Badge variant="outline" className="text-[10px]">
                          {t("pages.agent.memory.not_created")}
                        </Badge>
                      )}
                    </span>
                  </button>
                )
              })
            )}
          </div>
        ))
      )}
    </aside>
  )
}
