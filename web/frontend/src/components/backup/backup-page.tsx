import {
  IconAlertTriangle,
  IconArchive,
  IconDownload,
  IconFileZip,
  IconRefresh,
  IconShieldLock,
  IconUpload,
} from "@tabler/icons-react"
import dayjs from "dayjs"
import { useMutation } from "@tanstack/react-query"
import { type ChangeEvent, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  BackupApiError,
  type BackupInspection,
  type BackupRestoreResult,
  backupDownloadUrl,
  inspectBackup,
  restoreBackup,
} from "@/api/backup"
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
import { Button } from "@/components/ui/button"

function formatBytes(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`
  }
  const units = ["KB", "MB", "GB"]
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[unit]}`
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof BackupApiError ? error.message : fallback
}

export function BackupPage() {
  const { t } = useTranslation()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [inspection, setInspection] = useState<BackupInspection | null>(null)
  const [restoreResult, setRestoreResult] =
    useState<BackupRestoreResult | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)

  const inspectMutation = useMutation({
    mutationFn: (file: File) => inspectBackup(file),
    onSuccess: (data) => {
      setInspection(data)
      setRestoreResult(null)
    },
    onError: (error) =>
      toast.error(errorMessage(error, t("backup.inspectFailed"))),
  })

  const restoreMutation = useMutation({
    mutationFn: (id: string) => restoreBackup(id),
    onSuccess: (data) => {
      setRestoreResult(data)
      setInspection(null)
      setConfirmOpen(false)
      toast.success(t("backup.restoredToast"))
    },
    onError: (error) => {
      setConfirmOpen(false)
      toast.error(errorMessage(error, t("backup.restoreFailed")))
    },
  })

  const handleFileSelection = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (!file) {
      return
    }
    setInspection(null)
    setRestoreResult(null)
    inspectMutation.mutate(file)
  }

  const handleDownload = () => {
    const link = document.createElement("a")
    link.href = backupDownloadUrl()
    link.download = ""
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  const sectionLabel = (name: string) => {
    switch (name) {
      case "config":
        return t("backup.section.config")
      case "workspace":
        return t("backup.section.workspace")
      case "global-skills":
        return t("backup.section.globalSkills")
      default:
        return name
    }
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.backup")} />

      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 md:px-8">
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-4">
          <section className="bg-card rounded-2xl border p-5 shadow-xs">
            <div className="flex items-start gap-3">
              <IconArchive className="text-muted-foreground mt-0.5 size-5" />
              <div className="min-w-0 flex-1">
                <h2 className="text-[15px] font-medium">
                  {t("backup.exportTitle")}
                </h2>
                <p className="text-muted-foreground mt-1 text-sm">
                  {t("backup.exportDescription")}
                </p>
                <div className="border-border/60 bg-muted/40 mt-3 flex items-start gap-2 rounded-xl border p-3 text-sm">
                  <IconShieldLock className="text-muted-foreground mt-0.5 size-4 shrink-0" />
                  <div>
                    <p className="font-medium">{t("backup.secretsTitle")}</p>
                    <p className="text-muted-foreground mt-0.5 text-xs">
                      {t("backup.secretsDescription")}
                    </p>
                  </div>
                </div>
                <Button
                  type="button"
                  className="mt-4 gap-1.5"
                  onClick={handleDownload}
                >
                  <IconDownload className="size-4" />
                  {t("backup.download")}
                </Button>
              </div>
            </div>
          </section>

          <section className="bg-card rounded-2xl border p-5 shadow-xs">
            <div className="flex items-start gap-3">
              <IconUpload className="text-muted-foreground mt-0.5 size-5" />
              <div className="min-w-0 flex-1">
                <h2 className="text-[15px] font-medium">
                  {t("backup.restoreTitle")}
                </h2>
                <p className="text-muted-foreground mt-1 text-sm">
                  {t("backup.restoreDescription")}
                </p>

                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".zip,application/zip"
                  className="hidden"
                  onChange={handleFileSelection}
                />
                <Button
                  type="button"
                  variant="secondary"
                  className="mt-4 gap-1.5"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={inspectMutation.isPending || restoreMutation.isPending}
                >
                  <IconFileZip className="size-4" />
                  {inspectMutation.isPending
                    ? t("backup.inspecting")
                    : t("backup.chooseFile")}
                </Button>

                {inspection && (
                  <div className="border-border/60 mt-4 rounded-xl border p-4">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="min-w-0">
                        <p className="truncate font-medium">
                          {inspection.filename}
                        </p>
                        <p className="text-muted-foreground mt-0.5 text-xs">
                          {t("backup.createdAt")}:{" "}
                          {dayjs(inspection.manifest.created_at).format(
                            "YYYY-MM-DD HH:mm",
                          )}{" "}
                          · {t("backup.version")}:{" "}
                          {inspection.manifest.app_version || "dev"} ·{" "}
                          {inspection.manifest.platform}
                        </p>
                      </div>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="text-muted-foreground hover:text-foreground h-8 w-8"
                        onClick={() => {
                          setInspection(null)
                          fileInputRef.current?.click()
                        }}
                        aria-label={t("backup.chooseAnother")}
                        title={t("backup.chooseAnother")}
                      >
                        <IconRefresh className="size-4" />
                      </Button>
                    </div>

                    <div className="mt-3 grid gap-2 text-sm sm:grid-cols-2">
                      <div>
                        {t("backup.filesCount", {
                          count: inspection.files,
                          size: formatBytes(inspection.bytes),
                        })}
                      </div>
                      <div>
                        {t("backup.conflicts", {
                          overwrite: inspection.overwrite,
                          created: inspection.new,
                        })}
                      </div>
                    </div>

                    <div className="text-muted-foreground mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs">
                      {inspection.manifest.sections.map((section) => (
                        <span key={section.name}>
                          {sectionLabel(section.name)}: {section.files} (
                          {formatBytes(section.bytes)})
                        </span>
                      ))}
                      {inspection.ignored > 0 && (
                        <span>
                          {t("backup.ignoredCount", {
                            count: inspection.ignored,
                          })}
                        </span>
                      )}
                    </div>

                    {inspection.overwrite > 0 && (
                      <div className="border-amber-500/30 bg-amber-500/5 mt-3 flex items-start gap-2 rounded-lg border p-3 text-xs">
                        <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600" />
                        <div>
                          <p>{t("backup.overwriteWarning")}</p>
                          {inspection.overwrite_sample &&
                            inspection.overwrite_sample.length > 0 && (
                              <ul className="text-muted-foreground mt-1 list-inside list-disc font-mono">
                                {inspection.overwrite_sample.map((path) => (
                                  <li key={path} className="truncate">
                                    {path}
                                  </li>
                                ))}
                              </ul>
                            )}
                        </div>
                      </div>
                    )}

                    {inspection.warnings && inspection.warnings.length > 0 && (
                      <ul className="text-muted-foreground mt-3 list-inside list-disc text-xs">
                        {inspection.warnings.map((warning) => (
                          <li key={warning}>{warning}</li>
                        ))}
                      </ul>
                    )}

                    <Button
                      type="button"
                      className="mt-4 gap-1.5"
                      onClick={() => setConfirmOpen(true)}
                      disabled={restoreMutation.isPending}
                    >
                      <IconUpload className="size-4" />
                      {restoreMutation.isPending
                        ? t("backup.restoring")
                        : t("backup.restore")}
                    </Button>
                  </div>
                )}

                {restoreResult && (
                  <div className="border-emerald-500/30 bg-emerald-500/5 mt-4 rounded-xl border p-4 text-sm">
                    <p className="font-medium">{t("backup.restoreDoneTitle")}</p>
                    <p className="text-muted-foreground mt-1 text-xs">
                      {t("backup.restoreDoneDescription", {
                        count: restoreResult.restored,
                        ignored: restoreResult.ignored,
                      })}
                    </p>
                    {restoreResult.restart_required && (
                      <p className="mt-2 text-xs font-medium text-amber-700 dark:text-amber-400">
                        {t("backup.restartRequired")}
                      </p>
                    )}
                  </div>
                )}
              </div>
            </div>
          </section>
        </div>
      </div>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("backup.confirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("backup.confirmDescription", {
                count: inspection?.overwrite ?? 0,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={restoreMutation.isPending}>
              {t("backup.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault()
                if (inspection) {
                  restoreMutation.mutate(inspection.id)
                }
              }}
              disabled={restoreMutation.isPending}
            >
              {restoreMutation.isPending
                ? t("backup.restoring")
                : t("backup.restore")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
