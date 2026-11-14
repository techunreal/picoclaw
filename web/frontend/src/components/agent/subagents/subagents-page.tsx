import {
  IconActivity,
  IconAlertTriangle,
  IconPlayerStop,
  IconRefresh,
} from "@tabler/icons-react"
import dayjs from "dayjs"
import { useTranslation } from "react-i18next"

import type { SubagentStatus } from "@/api/subagents"
import {
  subagentErrorMessage,
  useSubagentsPage,
} from "@/components/agent/subagents/use-subagents-page"
import { PageHeader } from "@/components/page-header"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return "0s"
  if (ms < 1000) return `${Math.round(ms)}ms`
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  return `${minutes}m ${rest}s`
}

function StatusBadge({
  status,
  t,
}: {
  status: SubagentStatus
  t: (key: string) => string
}) {
  switch (status) {
    case "running":
      return (
        <Badge variant="secondary" className="gap-1.5">
          <span className="size-1.5 animate-pulse rounded-full bg-emerald-500" />
          {t("subagents.status.running")}
        </Badge>
      )
    case "completed":
      return <Badge variant="outline">{t("subagents.status.completed")}</Badge>
    case "failed":
      return <Badge variant="destructive">{t("subagents.status.failed")}</Badge>
    case "canceled":
      return <Badge variant="outline">{t("subagents.status.canceled")}</Badge>
    default:
      return <Badge variant="outline">{status}</Badge>
  }
}

export function SubagentsPage() {
  const { t } = useTranslation()
  const {
    listQuery,
    runs,
    counts,
    cancelTarget,
    setCancelTarget,
    cancelMutation,
    confirmCancel,
  } = useSubagentsPage()

  const listError = listQuery.error
    ? subagentErrorMessage(listQuery.error, t("subagents.loadFailed"))
    : null
  // dataUpdatedAt advances on every poll, so running durations stay stable
  // within a render and do not call impure time functions.
  const now = listQuery.dataUpdatedAt

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("subagents.title")}>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-muted-foreground hover:text-foreground h-8 w-8 rounded-full"
          onClick={() => void listQuery.refetch()}
          disabled={listQuery.isFetching}
          aria-label={t("subagents.refresh")}
          title={t("subagents.refresh")}
        >
          <IconRefresh className="size-4" />
        </Button>
      </PageHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 md:px-8">
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-4">
          <p className="text-muted-foreground text-sm">
            {t("subagents.description")}
          </p>

          {runs.length > 0 && (
            <p className="text-muted-foreground text-xs">
              {t("subagents.counts", {
                running: counts.running,
                completed: counts.completed,
                failed: counts.failed + counts.canceled,
              })}
            </p>
          )}

          {listError && (
            <div className="border-destructive/30 bg-destructive/5 text-destructive flex items-start gap-2 rounded-xl border p-4 text-sm">
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{listError}</span>
            </div>
          )}

          {listQuery.isLoading && (
            <div className="text-muted-foreground py-16 text-center text-sm">
              {t("subagents.loading")}
            </div>
          )}

          {!listQuery.isLoading && !listError && runs.length === 0 && (
            <div className="border-border/60 flex flex-col items-center gap-3 rounded-2xl border border-dashed p-12 text-center">
              <IconActivity className="text-muted-foreground size-10 opacity-60" />
              <div>
                <p className="font-medium">{t("subagents.emptyTitle")}</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  {t("subagents.emptyDescription")}
                </p>
              </div>
            </div>
          )}

          {runs.map((run) => {
            const duration =
              run.status === "running" && now > 0
                ? Math.max(0, now - run.started_at_ms)
                : run.duration_ms
            return (
              <div
                key={run.id}
                className="bg-card flex flex-col gap-3 rounded-2xl border p-4 shadow-xs"
              >
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="font-mono text-[13px] font-medium">
                        {run.id}
                      </h3>
                      <StatusBadge status={run.status} t={t} />
                      <Badge variant="outline">
                        {t("subagents.agent")}: {run.agent_id}
                      </Badge>
                      {run.depth > 1 && (
                        <Badge variant="outline">
                          {t("subagents.depth", { depth: run.depth })}
                        </Badge>
                      )}
                    </div>
                    <p className="text-muted-foreground mt-1 text-xs">
                      {t("subagents.started")}{" "}
                      {dayjs(run.started_at_ms).format("HH:mm:ss")} ·{" "}
                      {t("subagents.duration")} {formatDuration(duration)}
                      {run.parent_turn_id
                        ? ` · ${t("subagents.parent")} ${run.parent_turn_id}`
                        : ""}
                    </p>
                  </div>

                  {run.status === "running" && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive gap-1.5"
                      onClick={() => setCancelTarget(run)}
                    >
                      <IconPlayerStop className="size-4" />
                      {t("subagents.cancel")}
                    </Button>
                  )}
                </div>

                <p className="text-[14px] leading-relaxed break-words whitespace-pre-wrap">
                  {run.task}
                </p>

                {run.result_preview ? (
                  <div className="border-border/50 bg-muted/30 rounded-xl border p-3">
                    <p className="text-muted-foreground mb-1 text-[11px] font-medium tracking-wide uppercase">
                      {t("subagents.result")}
                    </p>
                    <p className="text-[13px] leading-relaxed break-words whitespace-pre-wrap">
                      {run.result_preview}
                    </p>
                  </div>
                ) : run.status !== "running" ? (
                  <p className="text-muted-foreground text-xs">
                    {t("subagents.noResult")}
                  </p>
                ) : null}
              </div>
            )
          })}
        </div>
      </div>

      <AlertDialog
        open={cancelTarget !== null}
        onOpenChange={(open) => {
          if (!open) setCancelTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("subagents.cancelTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("subagents.cancelDescription")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={cancelMutation.isPending}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={cancelMutation.isPending}
              onClick={(event) => {
                event.preventDefault()
                confirmCancel()
              }}
            >
              {t("subagents.cancelConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
