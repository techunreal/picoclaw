import { useQuery } from "@tanstack/react-query"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import type { AgentConfig, AgentDefaults, AgentView } from "@/api/agents"
import { getModels } from "@/api/models"
import { getSkills } from "@/api/skills"
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
import { Switch } from "@/components/ui/switch"
import { suggestAgentWorkspace } from "@/components/agent/agents/use-agents-page"

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
  onSave: (agent: AgentConfig) => void
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
  const [form, setForm] = useState<AgentConfig>(initial)
  const [skillFilter, setSkillFilter] = useState("")

  useEffect(() => {
    if (open) {
      setForm(initial)
      setSkillFilter("")
    }
  }, [open, initial])

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
  const skills = skillsQuery.data?.skills ?? []
  const selectedSkills = form.skills ?? []
  const subagentsEnabled = form.subagents?.enabled !== false
  const allowAgents = form.subagents?.allow_agents ?? []
  const allowAll = allowAgents.includes("*")

  const update = (patch: Partial<AgentConfig>) => {
    setForm((current) => ({ ...current, ...patch }))
  }
  const toggleSkill = (name: string, checked: boolean) => {
    const next = checked
      ? [...new Set([...selectedSkills, name])]
      : selectedSkills.filter((skill) => skill !== name)
    update({ skills: next })
  }
  const toggleAllowAgent = (id: string, checked: boolean) => {
    const next = checked
      ? [...new Set([...allowAgents, id])]
      : allowAgents.filter((item) => item !== id)
    update({
      subagents: { ...form.subagents, enabled: subagentsEnabled, allow_agents: next },
    })
  }

  const visibleSkills = skills.filter((skill) =>
    skill.name.toLowerCase().includes(skillFilter.trim().toLowerCase()),
  )
  const selectedNotListed = selectedSkills.filter(
    (name) => !skills.some((skill) => skill.name === name),
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {mode === "create" ? t("agents.addTitle") : t("agents.editTitle")}
          </DialogTitle>
          <DialogDescription>{t("agents.editDescription")}</DialogDescription>
        </DialogHeader>

        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault()
            onSave({
              ...form,
              id: form.id.trim(),
              name: form.name?.trim(),
              workspace: form.workspace?.trim(),
              skills: selectedSkills.length > 0 ? selectedSkills : undefined,
            })
          }}
        >
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
                disabled={mode === "edit" || isSaving}
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
                disabled={isSaving}
              />
            </div>

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
                disabled={isSaving}
              />
            </label>
          </fieldset>

          <fieldset className="flex flex-col gap-4">
            <legend className="mb-1 text-sm font-semibold">
              {t("agents.sectionRuntime")}
            </legend>

            <div className="grid gap-2">
              <Label htmlFor="agent-workspace">{t("agents.workspace")}</Label>
              <div className="flex flex-col gap-2 sm:flex-row">
                <Input
                  id="agent-workspace"
                  name="agent_workspace"
                  value={form.workspace ?? ""}
                  onChange={(event) => update({ workspace: event.target.value })}
                  placeholder={defaults.workspace}
                  autoComplete="off"
                  spellCheck={false}
                  disabled={isSaving}
                  aria-describedby="agent-workspace-help"
                />
                <Button
                  type="button"
                  variant="outline"
                  className="shrink-0"
                  onClick={() =>
                    update({
                      workspace: suggestAgentWorkspace(
                        defaults.workspace,
                        form.id.trim(),
                        Boolean(form.default),
                      ),
                    })
                  }
                  disabled={isSaving || !defaults.workspace}
                >
                  {t("agents.suggestWorkspace")}
                </Button>
              </div>
              <p id="agent-workspace-help" className="text-muted-foreground text-xs">
                {t("agents.workspaceHelp")}
              </p>
              {form.workspace?.trim() ? null : (
                <p className="text-muted-foreground text-xs">
                  {t("agents.workspaceEffective", {
                    path:
                      suggestAgentWorkspace(
                        defaults.workspace,
                        form.id.trim(),
                        Boolean(form.default),
                      ) || defaults.workspace,
                  })}
                </p>
              )}
            </div>

            <div className="grid gap-2">
              <Label htmlFor="agent-model">{t("agents.model")}</Label>
              <select
                id="agent-model"
                name="agent_model"
                value={form.model?.primary ?? ""}
                onChange={(event) =>
                  update({
                    model: event.target.value
                      ? { primary: event.target.value }
                      : undefined,
                  })
                }
                disabled={isSaving}
                className="border-input bg-background h-9 rounded-md border px-3 text-sm shadow-xs"
              >
                <option value="">
                  {t("agents.useDefaultModel", { model: defaults.model_name || "—" })}
                </option>
                {models.map((model) => (
                  <option key={model.model_name} value={model.model_name}>
                    {model.model_name}
                    {model.enabled ? "" : ` (${t("agents.disabledModel")})`}
                  </option>
                ))}
              </select>
              <p className="text-muted-foreground text-xs">
                {t("agents.modelOverrideHint")}
              </p>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="agent-skill-filter">{t("agents.skills")}</Label>
              <Input
                id="agent-skill-filter"
                value={skillFilter}
                onChange={(event) => setSkillFilter(event.target.value)}
                placeholder={t("agents.skillsFilterPlaceholder")}
                autoComplete="off"
                disabled={isSaving}
              />
              {selectedSkills.length > 0 && (
                <div className="flex flex-wrap gap-1.5">
                  {selectedSkills.map((name) => (
                    <Badge key={name} variant="secondary" className="gap-1">
                      {name}
                      <button
                        type="button"
                        className="text-muted-foreground hover:text-foreground"
                        aria-label={t("agents.removeSkill", { name })}
                        onClick={() =>
                          update({
                            skills: selectedSkills.filter((skill) => skill !== name),
                          })
                        }
                      >
                        ×
                      </button>
                    </Badge>
                  ))}
                </div>
              )}
              <div className="border-border/60 max-h-44 overflow-y-auto rounded-xl border p-3">
                {skillsQuery.isLoading ? (
                  <p className="text-muted-foreground text-xs">
                    {t("agents.skillsLoading")}
                  </p>
                ) : visibleSkills.length === 0 && selectedNotListed.length === 0 ? (
                  <p className="text-muted-foreground text-xs">
                    {t("agents.skillsEmpty")}
                  </p>
                ) : (
                  <ul className="flex flex-col gap-2">
                    {visibleSkills.map((skill) => (
                      <li key={skill.id}>
                        <label className="flex items-start gap-2 text-sm">
                          <input
                            type="checkbox"
                            className="accent-primary mt-0.5 size-4"
                            checked={selectedSkills.includes(skill.name)}
                            onChange={(event) =>
                              toggleSkill(skill.name, event.target.checked)
                            }
                            disabled={isSaving}
                          />
                          <span className="min-w-0">
                            <span className="font-medium">{skill.name}</span>
                            <span className="text-muted-foreground ml-2 text-xs">
                              {skill.source}
                            </span>
                            <span className="text-muted-foreground block text-xs">
                              {skill.description}
                            </span>
                          </span>
                        </label>
                      </li>
                    ))}
                    {selectedNotListed.map((name) => (
                      <li key={name}>
                        <label className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            className="accent-primary size-4"
                            checked
                            onChange={() =>
                              update({
                                skills: selectedSkills.filter((skill) => skill !== name),
                              })
                            }
                            disabled={isSaving}
                          />
                          <span className="font-medium">{name}</span>
                          <span className="text-muted-foreground text-xs">
                            {t("agents.skillNotInWorkspace")}
                          </span>
                        </label>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              <p className="text-muted-foreground text-xs">
                {t("agents.skillsHelp")}
              </p>
            </div>
          </fieldset>

          <fieldset className="flex flex-col gap-4">
            <legend className="mb-1 text-sm font-semibold">
              {t("agents.sectionSubagents")}
            </legend>

            <label className="flex items-center justify-between gap-3 rounded-xl border p-3">
              <span className="flex flex-col">
                <span className="text-sm font-medium">{t("agents.subagentsEnabled")}</span>
                <span className="text-muted-foreground text-xs">
                  {t("agents.subagentsEnabledHint")}
                </span>
              </span>
              <Switch
                checked={subagentsEnabled}
                onCheckedChange={(checked) =>
                  update({
                    subagents: {
                      ...form.subagents,
                      enabled: checked,
                      allow_agents: allowAgents,
                    },
                  })
                }
                disabled={isSaving}
              />
            </label>

            {subagentsEnabled && (
              <div className="grid gap-2">
                <span className="text-sm font-medium">{t("agents.allowAgents")}</span>
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="accent-primary size-4"
                    checked={allowAll}
                    onChange={(event) =>
                      update({
                        subagents: {
                          ...form.subagents,
                          enabled: true,
                          allow_agents: event.target.checked ? ["*"] : [],
                        },
                      })
                    }
                    disabled={isSaving}
                  />
                  {t("agents.allowAllAgents")}
                </label>
                {!allowAll && (
                  <div className="flex flex-wrap gap-3">
                    {agents
                      .filter((agent) => agent.id !== form.id)
                      .map((agent) => (
                        <label key={agent.id} className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            className="accent-primary size-4"
                            checked={allowAgents.includes(agent.id)}
                            onChange={(event) =>
                              toggleAllowAgent(agent.id, event.target.checked)
                            }
                            disabled={isSaving}
                          />
                          {agent.id}
                        </label>
                      ))}
                    {agents.filter((agent) => agent.id !== form.id).length === 0 && (
                      <span className="text-muted-foreground text-xs">
                        {t("agents.noOtherAgents")}
                      </span>
                    )}
                  </div>
                )}
              </div>
            )}
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
              disabled={isSaving}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={isSaving}>
              {isSaving ? t("common.saving") : t("agents.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
