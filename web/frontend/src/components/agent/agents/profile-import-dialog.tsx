import { IconAlertTriangle, IconFileZip, IconFolderOpen, IconUpload } from "@tabler/icons-react"
import { useQueryClient } from "@tanstack/react-query"
import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import {
  AgentApiError,
  importAgentProfile,
  inspectAgentProfile,
  type ProfilePreview,
  type ProfileUpload,
} from "@/api/agents"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { showSaveSuccessOrRestartToast } from "@/lib/restart-required"

interface ProfileImportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function ProfileImportDialog({
  open,
  onOpenChange,
}: ProfileImportDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const folderInputRef = useRef<HTMLInputElement>(null)

  const [upload, setUpload] = useState<ProfileUpload | null>(null)
  const [sourceLabel, setSourceLabel] = useState("")
  const [preview, setPreview] = useState<ProfilePreview | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [agentId, setAgentId] = useState("")
  const [name, setName] = useState("")
  const [emoji, setEmoji] = useState("")
  const [overwrite, setOverwrite] = useState(false)

  useEffect(() => {
    if (!open) return
    folderInputRef.current?.setAttribute("webkitdirectory", "")
    folderInputRef.current?.setAttribute("directory", "")
  }, [open])

  const reset = () => {
    setUpload(null)
    setSourceLabel("")
    setPreview(null)
    setPreviewing(false)
    setImporting(false)
    setError(null)
    setAgentId("")
    setName("")
    setEmoji("")
    setOverwrite(false)
  }

  const handleOpenChange = (next: boolean) => {
    if (!next) reset()
    onOpenChange(next)
  }

  const runPreview = async (next: ProfileUpload, label: string) => {
    setUpload(next)
    setSourceLabel(label)
    setPreview(null)
    setError(null)
    setPreviewing(true)
    try {
      const result = await inspectAgentProfile(next)
      setPreview(result)
      setAgentId(result.suggested_id)
      setName(result.identity.name ?? "")
      setEmoji(result.identity.emoji ?? "")
      setOverwrite(false)
    } catch (err) {
      setError(
        err instanceof AgentApiError
          ? err.message
          : t("agents.importPreviewFailed"),
      )
    } finally {
      setPreviewing(false)
    }
  }

  const handleZip = (file: File | undefined) => {
    if (!file) return
    void runPreview({ zip: file }, file.name)
  }

  const handleFolder = (files: FileList | null) => {
    if (!files || files.length === 0) return
    const list = Array.from(files)
    const root =
      (list[0] as File & { webkitRelativePath?: string }).webkitRelativePath
        ?.split("/")[0] || t("agents.importFolder")
    void runPreview({ files: list }, `${root} (${list.length})`)
  }

  const handleImport = async () => {
    if (!upload || !preview) return
    if (!agentId.trim()) {
      setError(t("agents.importIdRequired"))
      return
    }
    setImporting(true)
    setError(null)
    try {
      const result = await importAgentProfile(upload, {
        id: agentId.trim(),
        name: name.trim(),
        emoji: emoji.trim(),
        overwrite,
      })
      showSaveSuccessOrRestartToast(
        t,
        t("agents.importSuccess", { id: result.agent }),
        t("agents.title"),
        result.restart_required,
      )
      void queryClient.invalidateQueries({ queryKey: ["agents"] })
      void queryClient.invalidateQueries({ queryKey: ["sidebar-agents"] })
      handleOpenChange(false)
    } catch (err) {
      if (err instanceof AgentApiError && err.code === "profile_conflict") {
        setError(t("agents.importConflict"))
        setOverwrite(true)
      } else {
        setError(
          err instanceof AgentApiError ? err.message : t("agents.importFailed"),
        )
      }
    } finally {
      setImporting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("agents.importTitle")}</DialogTitle>
          <DialogDescription>{t("agents.importDescription")}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="border-border/60 hover:bg-muted/40 flex cursor-pointer flex-col items-center gap-2 rounded-xl border border-dashed p-4 text-center">
              <IconFileZip className="text-muted-foreground size-7" />
              <span className="text-sm font-medium">{t("agents.importChooseZip")}</span>
              <span className="text-muted-foreground text-xs">
                {t("agents.importZipHint")}
              </span>
              <input
                type="file"
                accept=".zip,application/zip,application/x-zip-compressed"
                className="hidden"
                onChange={(event) => {
                  handleZip(event.target.files?.[0])
                  event.target.value = ""
                }}
                disabled={previewing || importing}
              />
            </label>

            <label className="border-border/60 hover:bg-muted/40 flex cursor-pointer flex-col items-center gap-2 rounded-xl border border-dashed p-4 text-center">
              <IconFolderOpen className="text-muted-foreground size-7" />
              <span className="text-sm font-medium">
                {t("agents.importChooseFolder")}
              </span>
              <span className="text-muted-foreground text-xs">
                {t("agents.importFolderHint")}
              </span>
              <input
                ref={folderInputRef}
                type="file"
                className="hidden"
                onChange={(event) => {
                  handleFolder(event.target.files)
                  event.target.value = ""
                }}
                disabled={previewing || importing}
              />
            </label>
          </div>

          {sourceLabel && (
            <p className="text-muted-foreground text-xs">
              {t("agents.importSource", { source: sourceLabel })}
            </p>
          )}

          {previewing && (
            <p className="text-muted-foreground text-sm">
              {t("agents.importPreviewing")}
            </p>
          )}

          {error && (
            <div className="border-destructive/30 bg-destructive/5 text-destructive flex items-start gap-2 rounded-xl border p-3 text-sm">
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {preview && (
            <>
              <div className="bg-card rounded-xl border p-4">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <p className="text-sm font-medium">
                      {t("agents.importProfileRoot")}{" "}
                      <span className="font-mono">{preview.root || "."}</span>
                    </p>
                    <p className="text-muted-foreground text-xs">
                      {t("agents.importFilesFound", { count: preview.files.length })}
                    </p>
                  </div>
                  {preview.combined && (
                    <Badge variant="outline">{t("agents.importCombinedBadge")}</Badge>
                  )}
                </div>

                <ul className="mt-3 flex flex-col gap-2">
                  {preview.files.map((file) => (
                    <li key={file.target} className="rounded-lg border p-2">
                      <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
                        <span className="font-mono">
                          {file.source} → <strong>{file.target}</strong>
                        </span>
                        <span className="text-muted-foreground">
                          {formatSize(file.size)}
                        </span>
                      </div>
                      <details className="mt-1">
                        <summary className="text-muted-foreground cursor-pointer text-xs">
                          {t("agents.importShowPreview")}
                        </summary>
                        <pre className="bg-muted/40 mt-1 max-h-40 overflow-auto rounded p-2 text-[11px] whitespace-pre-wrap">
                          {file.preview}
                        </pre>
                      </details>
                    </li>
                  ))}
                </ul>

                {preview.ignored_count > 0 && (
                  <details className="mt-3">
                    <summary className="text-muted-foreground cursor-pointer text-xs">
                      {t("agents.importIgnored", { count: preview.ignored_count })}
                    </summary>
                    <ul className="text-muted-foreground mt-1 flex flex-col gap-0.5 font-mono text-[11px]">
                      {preview.ignored.map((item) => (
                        <li key={item}>{item}</li>
                      ))}
                      {preview.ignored_count > preview.ignored.length && (
                        <li>…</li>
                      )}
                    </ul>
                  </details>
                )}

                {preview.notes?.map((note) => (
                  <p key={note} className="text-muted-foreground mt-2 text-xs">
                    {note}
                  </p>
                ))}
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <div className="grid gap-2">
                  <Label htmlFor="import-agent-id">{t("agents.importId")}</Label>
                  <Input
                    id="import-agent-id"
                    value={agentId}
                    onChange={(event) => setAgentId(event.target.value)}
                    autoComplete="off"
                    spellCheck={false}
                    disabled={importing}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="import-agent-name">{t("agents.name")}</Label>
                  <Input
                    id="import-agent-name"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    maxLength={120}
                    disabled={importing}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="import-agent-emoji">
                    {t("agents.avatarEmoji")}
                  </Label>
                  <Input
                    id="import-agent-emoji"
                    value={emoji}
                    onChange={(event) => setEmoji(event.target.value)}
                    maxLength={16}
                    disabled={importing}
                  />
                </div>
              </div>

              <label className="flex items-start gap-2 rounded-xl border p-3 text-sm">
                <input
                  type="checkbox"
                  className="accent-primary mt-0.5 size-4"
                  checked={overwrite}
                  onChange={(event) => setOverwrite(event.target.checked)}
                  disabled={importing}
                />
                <span>
                  <span className="font-medium">{t("agents.importOverwrite")}</span>
                  <span className="text-muted-foreground block text-xs">
                    {t("agents.importOverwriteHint")}
                  </span>
                </span>
              </label>
            </>
          )}
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="ghost"
            onClick={() => handleOpenChange(false)}
            disabled={importing}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            onClick={() => void handleImport()}
            disabled={!preview || importing || previewing || !agentId.trim()}
            className="gap-1.5"
          >
            <IconUpload className="size-4" />
            {importing ? t("agents.importImporting") : t("agents.importConfirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
