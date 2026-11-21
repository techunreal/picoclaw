import {
  IconAlertTriangle,
  IconFileZip,
  IconFolderOpen,
  IconUpload,
} from "@tabler/icons-react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  AgentApiError,
  agentModelPrimary,
  importAgentProfile,
  inspectAgentProfile,
  toAgentConfig,
  uploadAgentAvatar,
  type AgentConfig,
  type AgentDefaults,
  type AgentView,
  type ProfilePreview,
  type ProfileUpload,
} from "@/api/agents"
import { getModels } from "@/api/models"
import { getSkills } from "@/api/skills"
import {
  AgentAvatarPicker,
  AgentModelField,
  AgentSkillsField,
  AgentSubagentsField,
  AgentWorkspaceField,
} from "@/components/agent/agents/agent-form-fields"
import { defaultSkillSelection } from "@/components/agent/agents/agent-form-utils"
import { suggestAgentWorkspace } from "@/components/agent/agents/use-agents-page"
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
  agents: AgentView[]
  defaults: AgentDefaults
  onOpenChange: (open: boolean) => void
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function emptyAgent(): AgentConfig {
  return { id: "", name: "" }
}

export function ProfileImportDialog({
  open,
  agents,
  defaults,
  onOpenChange,
}: ProfileImportDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [upload, setUpload] = useState<ProfileUpload | null>(null)
  const [sourceLabel, setSourceLabel] = useState("")
  const [preview, setPreview] = useState<ProfilePreview | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [form, setForm] = useState<AgentConfig>(emptyAgent())
  const [workspaceTouched, setWorkspaceTouched] = useState(false)
  const [overwrite, setOverwrite] = useState(false)
  const [skillsDefaultsApplied, setSkillsDefaultsApplied] = useState(false)
  const [pendingAvatar, setPendingAvatar] = useState<{
    file: File
    previewUrl: string
  } | null>(null)
  const [loadedExistingID, setLoadedExistingID] = useState<string | null>(null)

  const modelsQuery = useQuery({
    queryKey: ["models"],
    queryFn: getModels,
    enabled: open,
  })
  const skillsQuery = useQuery({
    queryKey: ["skills", "import-picker"],
    queryFn: () => getSkills(),
    enabled: open,
  })
  const models = modelsQuery.data?.models ?? []
  const skills = useMemo(() => skillsQuery.data?.skills ?? [], [skillsQuery.data])
  const selectedSkills = form.skills ?? []
  const selectedSkillBytes = selectedSkills.reduce((sum, name) => {
    const skill = skills.find((item) => item.name === name)
    return sum + (skill?.size ?? 0)
  }, 0)
  const selectedSkillTokens = Math.round(selectedSkillBytes / 4)

  const setFolderInput = (node: HTMLInputElement | null) => {
    if (!node) return
    node.setAttribute("webkitdirectory", "")
    node.setAttribute("directory", "")
  }

  useEffect(() => {
    return () => {
      if (pendingAvatar) URL.revokeObjectURL(pendingAvatar.previewUrl)
    }
  }, [pendingAvatar])

  useEffect(() => {
    if (!open || skillsDefaultsApplied || skillsQuery.isLoading) return
    if (form.skills !== undefined) {
      setSkillsDefaultsApplied(true)
      return
    }
    if (skills.length === 0) return
    setForm((current) =>
      current.skills === undefined
        ? { ...current, skills: defaultSkillSelection(skills) }
        : current,
    )
    setSkillsDefaultsApplied(true)
  }, [open, skillsDefaultsApplied, skillsQuery.isLoading, skills, form.skills])

  const reset = () => {
    setUpload(null)
    setSourceLabel("")
    setPreview(null)
    setPreviewing(false)
    setImporting(false)
    setError(null)
    setForm(emptyAgent())
    setWorkspaceTouched(false)
    setOverwrite(false)
    setSkillsDefaultsApplied(false)
    setPendingAvatar(null)
    setLoadedExistingID(null)
  }

  const handleOpenChange = (next: boolean) => {
    if (!next) reset()
    onOpenChange(next)
  }

  const existingAgent = agents.find((agent) => agent.id === form.id.trim())

  const applyID = (nextID: string) => {
    const trimmed = nextID.trim()
    const existing = agents.find((agent) => agent.id === trimmed)
    if (existing) {
      if (loadedExistingID !== trimmed) {
        setForm({ ...toAgentConfig(existing), id: trimmed })
        setLoadedExistingID(trimmed)
        setWorkspaceTouched(false)
        setSkillsDefaultsApplied(existing.skills !== undefined)
        setPendingAvatar(null)
      }
      return
    }
    if (loadedExistingID !== null) {
      setLoadedExistingID(null)
      setWorkspaceTouched(false)
      setSkillsDefaultsApplied(false)
      setPendingAvatar(null)
      setForm({
        id: nextID,
        name: preview?.identity.name ?? "",
        workspace: suggestAgentWorkspace(defaults.workspace, nextID),
      })
      return
    }
    setForm((current) => ({
      ...current,
      id: nextID,
      workspace: workspaceTouched
        ? current.workspace
        : suggestAgentWorkspace(defaults.workspace, nextID),
    }))
  }

  const runPreview = async (next: ProfileUpload, label: string) => {
    setUpload(next)
    setSourceLabel(label)
    setPreview(null)
    setError(null)
    setPendingAvatar(null)
    setPreviewing(true)
    try {
      const result = await inspectAgentProfile(next)
      setPreview(result)
      const existing = agents.find((agent) => agent.id === result.suggested_id)
      const base = existing
        ? { ...toAgentConfig(existing), id: result.suggested_id }
        : {
            id: result.suggested_id,
            name: result.identity.name ?? "",
            workspace: suggestAgentWorkspace(defaults.workspace, result.suggested_id),
            skills: skills.length > 0 ? defaultSkillSelection(skills) : undefined,
          }
      setForm(base)
      setWorkspaceTouched(false)
      setOverwrite(false)
      setSkillsDefaultsApplied(
        existing ? existing.skills !== undefined : skills.length > 0,
      )
      setLoadedExistingID(existing ? existing.id : null)
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

  const handleAvatarSelect = (file: File) => {
    setPendingAvatar({ file, previewUrl: URL.createObjectURL(file) })
  }

  const handleImport = async () => {
    if (!upload || !preview) return
    if (!form.id.trim()) {
      setError(t("agents.importIdRequired"))
      return
    }
    setImporting(true)
    setError(null)
    try {
      const result = await importAgentProfile(upload, {
        agent: {
          ...form,
          id: form.id.trim(),
          name: form.name?.trim(),
          workspace: form.workspace?.trim(),
          skills: form.skills,
        },
        overwrite,
      })
      if (pendingAvatar) {
        try {
          await uploadAgentAvatar(result.agent, pendingAvatar.file)
        } catch {
          toast.error(t("agents.avatarUploadFailed"))
        }
      }
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
                ref={setFolderInput}
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
                      {preview.ignored_count > preview.ignored.length && <li>…</li>}
                    </ul>
                  </details>
                )}

                {preview.notes?.map((note) => (
                  <p key={note} className="text-muted-foreground mt-2 text-xs">
                    {note}
                  </p>
                ))}
              </div>

              <fieldset className="flex flex-col gap-4">
                <legend className="mb-1 text-sm font-semibold">
                  {t("agents.sectionIdentity")}
                </legend>

                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="import-agent-id">{t("agents.importId")}</Label>
                    <Input
                      id="import-agent-id"
                      value={form.id}
                      onChange={(event) => applyID(event.target.value)}
                      autoComplete="off"
                      spellCheck={false}
                      disabled={importing}
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="import-agent-name">{t("agents.name")}</Label>
                    <Input
                      id="import-agent-name"
                      value={form.name ?? ""}
                      onChange={(event) =>
                        setForm((current) => ({
                          ...current,
                          name: event.target.value,
                        }))
                      }
                      maxLength={120}
                      disabled={importing}
                    />
                  </div>
                </div>

                {existingAgent && (
                  <p className="text-muted-foreground text-xs">
                    {t("agents.importExistingHint")}
                  </p>
                )}

                <AgentAvatarPicker
                  agentId={existingAgent ? form.id : undefined}
                  avatar={form.avatar}
                  name={form.name || form.id}
                  previewUrl={pendingAvatar?.previewUrl}
                  disabled={importing}
                  canRemove={Boolean(pendingAvatar)}
                  onSelectFile={handleAvatarSelect}
                  onRemove={() => setPendingAvatar(null)}
                />
              </fieldset>

              <fieldset className="flex flex-col gap-4">
                <legend className="mb-1 text-sm font-semibold">
                  {t("agents.sectionRuntime")}
                </legend>

                <AgentWorkspaceField
                  idPrefix="import"
                  defaultsWorkspace={defaults.workspace}
                  agentId={form.id}
                  value={form.workspace}
                  onChange={(workspace) => {
                    setWorkspaceTouched(true)
                    setForm((current) => ({ ...current, workspace }))
                  }}
                  disabled={importing}
                />

                <AgentModelField
                  idPrefix="import"
                  models={models}
                  value={agentModelPrimary(form.model)}
                  defaultModelName={defaults.model_name}
                  onChange={(primary) =>
                    setForm((current) => ({
                      ...current,
                      model: primary ? { primary } : undefined,
                    }))
                  }
                  disabled={importing}
                />

                <AgentSkillsField
                  idPrefix="import"
                  skills={skills}
                  selected={selectedSkills}
                  onChange={(next) =>
                    setForm((current) => ({ ...current, skills: next }))
                  }
                  loading={skillsQuery.isLoading}
                  disabled={importing}
                />
                <p className="text-muted-foreground text-xs">
                  {t("agents.skillTokenHint", { tokens: selectedSkillTokens })}
                </p>
              </fieldset>

              <fieldset className="flex flex-col gap-4">
                <legend className="mb-1 text-sm font-semibold">
                  {t("agents.sectionSubagents")}
                </legend>

                <AgentSubagentsField
                  agents={agents}
                  currentId={form.id}
                  value={form.subagents}
                  onChange={(subagents) =>
                    setForm((current) => ({ ...current, subagents }))
                  }
                  disabled={importing}
                />
              </fieldset>

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
            disabled={!preview || importing || previewing || !form.id.trim()}
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
