import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  deleteAgentAvatar,
  uploadAgentAvatar,
  type AgentAvatar as AgentAvatarData,
  type AgentConfig,
  type AgentDefaults,
  type AgentView,
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
import { Switch } from "@/components/ui/switch"

const agentIDPattern = "^[a-z0-9][a-z0-9_-]{0,63}$"

interface AgentEditDialogProps {
  open: boolean
  mode: "create" | "edit"
  initial: AgentConfig
  agents: AgentView[]
  defaults: AgentDefaults
  isSaving: boolean
  error: string | null
  onOpenChange: (open: boolean) => void
  onSave: (agent: AgentConfig) => Promise<boolean> | boolean
}

export function AgentEditDialog({
  open,
  mode,
  initial,
  agents,
  defaults,
  isSaving,
  error,
  onOpenChange,
  onSave,
}: AgentEditDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<AgentConfig>(initial)
  const [skillDefaultsApplied, setSkillDefaultsApplied] = useState(false)
  const [avatarBusy, setAvatarBusy] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [pendingAvatar, setPendingAvatar] = useState<{
    file: File
    previewUrl: string
  } | null>(null)

  useEffect(() => {
    if (open) {
      setForm(initial)
      setSkillDefaultsApplied(false)
      setPendingAvatar(null)
      setSubmitting(false)
    }
  }, [open, initial])

  useEffect(() => {
    return () => {
      if (pendingAvatar) URL.revokeObjectURL(pendingAvatar.previewUrl)
    }
  }, [pendingAvatar])

  const modelsQuery = useQuery({
    queryKey: ["models"],
    queryFn: getModels,
    enabled: open,
  })
  const skillsQuery = useQuery({
    queryKey: ["skills", "agent-picker", mode === "edit" ? initial.id : ""],
    queryFn: () => getSkills(mode === "edit" ? initial.id : undefined),
    enabled: open,
  })

  const models = modelsQuery.data?.models ?? []
  const skills = useMemo(() => skillsQuery.data?.skills ?? [], [skillsQuery.data])
  const selectedSkills = form.skills ?? []

  useEffect(() => {
    if (!open || skillDefaultsApplied || skillsQuery.isLoading) return
    if (form.skills !== undefined) {
      setSkillDefaultsApplied(true)
      return
    }
    if (skills.length === 0) return
    setForm((current) =>
      current.skills === undefined
        ? { ...current, skills: defaultSkillSelection(skills) }
        : current,
    )
    setSkillDefaultsApplied(true)
  }, [open, skillDefaultsApplied, skillsQuery.isLoading, skills, form.skills])

  const update = (patch: Partial<AgentConfig>) => {
    setForm((current) => ({ ...current, ...patch }))
  }

  const refreshAgentQueries = () => {
    void queryClient.invalidateQueries({ queryKey: ["agents"] })
    void queryClient.invalidateQueries({ queryKey: ["sidebar-agents"] })
  }

  const handleAvatarFile = async (file: File) => {
    if (mode === "create") {
      const previewUrl = URL.createObjectURL(file)
      setPendingAvatar({ file, previewUrl })
      return
    }
    if (!form.id.trim()) return
    setAvatarBusy(true)
    try {
      const result = await uploadAgentAvatar(form.id.trim(), file)
      update({ avatar: { ...form.avatar, ...result.avatar } })
      refreshAgentQueries()
      toast.success(t("agents.avatarUploaded"))
    } catch {
      toast.error(t("agents.avatarUploadFailed"))
    } finally {
      setAvatarBusy(false)
    }
  }

  const handleAvatarRemove = async () => {
    if (mode === "create") {
      setPendingAvatar(null)
      return
    }
    if (!form.id.trim()) return
    setAvatarBusy(true)
    try {
      await deleteAgentAvatar(form.id.trim())
      const nextAvatar: AgentAvatarData = { emoji: form.avatar?.emoji }
      update({ avatar: nextAvatar })
      refreshAgentQueries()
      toast.success(t("agents.avatarRemoved"))
    } catch {
      toast.error(t("agents.avatarRemoveFailed"))
    } finally {
      setAvatarBusy(false)
    }
  }

  const busy = isSaving || submitting

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const agent: AgentConfig = {
      ...form,
      id: form.id.trim(),
      name: form.name?.trim(),
      workspace: form.workspace?.trim(),
      skills: form.skills,
    }
    setSubmitting(true)
    try {
      const success = await onSave(agent)
      if (!success) return
      if (mode === "create" && pendingAvatar) {
        try {
          await uploadAgentAvatar(agent.id, pendingAvatar.file)
          refreshAgentQueries()
          toast.success(t("agents.avatarUploaded"))
        } catch {
          toast.error(t("agents.avatarUploadFailed"))
        }
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {mode === "create" ? t("agents.addTitle") : t("agents.editTitle")}
          </DialogTitle>
          <DialogDescription>{t("agents.editDescription")}</DialogDescription>
        </DialogHeader>

        <form className="flex flex-col gap-6" onSubmit={handleSubmit}>
          <fieldset className="flex flex-col gap-4">
            <legend className="mb-1 text-sm font-semibold">
              {t("agents.sectionIdentity")}
            </legend>

            <div className="grid gap-2">
              <Label htmlFor="agent-id">{t("agents.id")}</Label>
              <Input
                id="agent-id"
                name="agent_id"
                value={form.id}
                onChange={(event) => update({ id: event.target.value })}
                placeholder="alpha"
                pattern={mode === "create" ? agentIDPattern : undefined}
                maxLength={64}
                autoComplete="off"
                autoCapitalize="none"
                spellCheck={false}
                disabled={mode === "edit" || busy}
                required
                aria-describedby="agent-id-help"
              />
              <p id="agent-id-help" className="text-muted-foreground text-xs">
                {t("agents.idHelp")}
              </p>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="agent-name">{t("agents.name")}</Label>
              <Input
                id="agent-name"
                name="agent_name"
                value={form.name ?? ""}
                onChange={(event) => update({ name: event.target.value })}
                placeholder={t("agents.namePlaceholder")}
                maxLength={120}
                disabled={busy}
              />
            </div>

            <AgentAvatarPicker
              agentId={mode === "edit" ? form.id.trim() : undefined}
              avatar={form.avatar}
              name={form.name || form.id}
              previewUrl={pendingAvatar?.previewUrl}
              disabled={busy || avatarBusy}
              canRemove={
                mode === "edit" ? Boolean(form.avatar?.image) : Boolean(pendingAvatar)
              }
              onSelectFile={(file) => void handleAvatarFile(file)}
              onRemove={() => void handleAvatarRemove()}
            />

            <label className="flex items-center justify-between gap-3 rounded-xl border p-3">
              <span className="flex flex-col">
                <span className="text-sm font-medium">{t("agents.makeDefault")}</span>
                <span className="text-muted-foreground text-xs">
                  {t("agents.makeDefaultHint")}
                </span>
              </span>
              <Switch
                checked={Boolean(form.default)}
                onCheckedChange={(checked) => update({ default: checked })}
                disabled={busy}
              />
            </label>
          </fieldset>

          <fieldset className="flex flex-col gap-4">
            <legend className="mb-1 text-sm font-semibold">
              {t("agents.sectionRuntime")}
            </legend>

            <AgentWorkspaceField
              idPrefix="agent"
              defaultsWorkspace={defaults.workspace}
              agentId={form.id.trim()}
              value={form.workspace}
              onChange={(workspace) => update({ workspace })}
              disabled={busy}
            />

            <AgentModelField
              idPrefix="agent"
              models={models}
              value={form.model?.primary}
              defaultModelName={defaults.model_name}
              onChange={(primary) =>
                update({ model: primary ? { primary } : undefined })
              }
              disabled={busy}
            />

            <AgentSkillsField
              idPrefix="agent"
              skills={skills}
              selected={selectedSkills}
              onChange={(next) => update({ skills: next })}
              loading={skillsQuery.isLoading}
              disabled={busy}
            />
          </fieldset>

          <fieldset className="flex flex-col gap-4">
            <legend className="mb-1 text-sm font-semibold">
              {t("agents.sectionSubagents")}
            </legend>

            <AgentSubagentsField
              agents={agents}
              currentId={form.id}
              value={form.subagents}
              onChange={(subagents) => update({ subagents })}
              disabled={busy}
            />
          </fieldset>

          {error && (
            <div
              role="alert"
              className="border-destructive/30 bg-destructive/5 text-destructive rounded-xl border p-3 text-sm"
            >
              {error}
            </div>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={busy}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={busy}>
              {busy ? t("common.saving") : t("agents.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
