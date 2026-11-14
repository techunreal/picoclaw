import {
  IconAlertTriangle,
  IconCalendarTime,
  IconPencil,
  IconPlayerPlay,
  IconPlus,
  IconRefresh,
  IconTrash,
} from "@tabler/icons-react"
import dayjs from "dayjs"
import { useTranslation } from "react-i18next"

import type { CronJob } from "@/api/cron"
import { CronJobDialog } from "@/components/automation/cron-job-dialog"
import { describeSchedule, formatRunTime } from "@/components/automation/format"
import { cronErrorMessage, useCronPage } from "@/components/automation/use-cron-page"
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
import { Switch } from "@/components/ui/switch"

function isCommandJob(job: CronJob): boolean {
  return Boolean(job.payload.command?.trim())
}

function jobStatusBadge(job: CronJob, t: (key: string) => string) {
  if (!job.enabled) {
    return <Badge variant="outline">{t("cron.status.disabled")}</Badge>
  }
  if (job.state.lastStatus === "error") {
    return <Badge variant="destructive">{t("cron.status.lastRunFailed")}</Badge>
  }
  if (job.state.lastStatus === "ok") {
    return <Badge variant="secondary">{t("cron.status.lastRunOk")}</Badge>
  }
  return <Badge variant="secondary">{t("cron.status.enabled")}</Badge>
}

export function CronPage() {
  const { t } = useTranslation()
  const {
    listQuery,
    jobs,
    status,
    dialogOpen,
    editingJob,
    deleteTarget,
    isSaving,
    isDeleting,
    pendingRunId,
    openCreate,
    openEdit,
    handleDialogOpenChange,
    handleSave,
    setDeleteTarget,
    handleDelete,
    handleRunNow,
    handleToggle,
  } = useCronPage()

  const listError = listQuery.error
    ? cronErrorMessage(listQuery.error, t("cron.loadFailed"))
    : null
  const schedulerWarning =
    status && !status.tool_enabled
      ? t("cron.toolDisabledHint")
      : status && !status.running
        ? t("cron.schedulerStoppedHint")
        : null

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.cron")}>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-muted-foreground hover:text-foreground h-8 w-8 rounded-full"
          onClick={() => void listQuery.refetch()}
          disabled={listQuery.isFetching}
          aria-label={t("cron.refresh")}
          title={t("cron.refresh")}
        >
          <IconRefresh
            className={`size-4 ${listQuery.isFetching ? "animate-spin" : ""}`}
          />
        </Button>
        <Button type="button" size="sm" className="gap-1.5" onClick={openCreate}>
          <IconPlus className="size-4" />
          {t("cron.newTask")}
        </Button>
      </PageHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 md:px-8">
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-4">
          {listError && (
            <div className="border-destructive/30 bg-destructive/5 text-destructive flex items-start gap-2 rounded-xl border p-4 text-sm">
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{listError}</span>
            </div>
          )}

          {schedulerWarning && (
            <div className="border-border/60 bg-muted/40 text-muted-foreground flex items-start gap-2 rounded-xl border p-4 text-sm">
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{schedulerWarning}</span>
            </div>
          )}

          {listQuery.isLoading && (
            <div className="text-muted-foreground py-16 text-center text-sm">
              {t("cron.loading")}
            </div>
          )}

          {!listQuery.isLoading && !listError && jobs.length === 0 && (
            <div className="border-border/60 flex flex-col items-center gap-3 rounded-2xl border border-dashed p-12 text-center">
              <IconCalendarTime className="text-muted-foreground size-10 opacity-60" />
              <div>
                <p className="font-medium">{t("cron.emptyTitle")}</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  {t("cron.emptyDescription")}
                </p>
              </div>
              <Button type="button" size="sm" className="gap-1.5" onClick={openCreate}>
                <IconPlus className="size-4" />
                {t("cron.newTask")}
              </Button>
            </div>
          )}

          {jobs.map((job) => (
            <div
              key={job.id}
              className="bg-card flex flex-col gap-3 rounded-2xl border p-4 shadow-xs"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="truncate text-[15px] font-medium">
                      {job.name}
                    </h3>
                    {jobStatusBadge(job, t)}
                    {isCommandJob(job) && (
                      <Badge variant="outline">{t("cron.status.command")}</Badge>
                    )}
                  </div>
                  <p className="text-muted-foreground mt-1 text-sm">
                    {describeSchedule(job.schedule, t)}
                  </p>
                </div>

                <div className="flex items-center gap-1">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="gap-1.5"
                    onClick={() => handleRunNow(job)}
                    disabled={pendingRunId === job.id}
                  >
                    <IconPlayerPlay className="size-4" />
                    {t("cron.runNow")}
                  </Button>
                  {!isCommandJob(job) && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="text-muted-foreground hover:text-foreground h-8 w-8"
                      onClick={() => openEdit(job)}
                      aria-label={t("cron.edit")}
                      title={t("cron.edit")}
                    >
                      <IconPencil className="size-4" />
                    </Button>
                  )}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="text-muted-foreground hover:text-destructive h-8 w-8"
                    onClick={() => setDeleteTarget(job)}
                    aria-label={t("cron.delete")}
                    title={t("cron.delete")}
                  >
                    <IconTrash className="size-4" />
                  </Button>
                  <Switch
                    checked={job.enabled}
                    onCheckedChange={(checked) => handleToggle(job, checked)}
                    aria-label={t("cron.toggle")}
                  />
                </div>
              </div>

              <p className="text-muted-foreground line-clamp-2 text-sm">
                {job.payload.message}
              </p>

              <div className="text-muted-foreground flex flex-wrap gap-x-6 gap-y-1 text-xs">
                <span>
                  {t("cron.nextRun")}:{" "}
                  {job.enabled
                    ? formatRunTime(job.state.nextRunAtMs, t)
                    : t("cron.status.disabled")}
                </span>
                <span>
                  {t("cron.lastRun")}:{" "}
                  {formatRunTime(job.state.lastRunAtMs, t)}
                </span>
                <span>
                  {t("cron.updated")}:{" "}
                  {dayjs(job.updatedAtMs).format("YYYY-MM-DD HH:mm")}
                </span>
              </div>

              {job.state.lastStatus === "error" && job.state.lastError && (
                <p className="text-destructive wrap-break-word text-xs">
                  {job.state.lastError}
                </p>
              )}
            </div>
          ))}
        </div>
      </div>

      <CronJobDialog
        open={dialogOpen}
        job={editingJob}
        isSaving={isSaving}
        onOpenChange={handleDialogOpenChange}
        onSave={handleSave}
      />

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("cron.deleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("cron.deleteDescription", { name: deleteTarget?.name ?? "" })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>
              {t("cron.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault()
                handleDelete()
              }}
              disabled={isDeleting}
            >
              {isDeleting ? t("cron.deleting") : t("cron.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
