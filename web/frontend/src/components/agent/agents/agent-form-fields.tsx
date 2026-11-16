import { useId, useState } from "react"
import { useTranslation } from "react-i18next"

import type {
  AgentAvatar as AgentAvatarData,
  AgentSubagentsConfig,
  AgentView,
} from "@/api/agents"
import type { ModelInfo } from "@/api/models"
import type { SkillSupportItem } from "@/api/skills"
import { AgentAvatar } from "@/components/agent-avatar"
import { LOCKED_AGENT_SKILL } from "@/components/agent/agents/agent-form-utils"
import { suggestAgentWorkspace } from "@/components/agent/agents/use-agents-page"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"

interface AgentWorkspaceFieldProps {
  idPrefix: string
  defaultsWorkspace?: string
  agentId: string
  value?: string
  onChange: (workspace: string) => void
  disabled?: boolean
}

export function AgentWorkspaceField({
  idPrefix,
  defaultsWorkspace,
  agentId,
  value,
  onChange,
  disabled,
}: AgentWorkspaceFieldProps) {
  const { t } = useTranslation()
  const inputId = `${idPrefix}-workspace`
  const helpId = `${idPrefix}-workspace-help`
  const effective = suggestAgentWorkspace(defaultsWorkspace ?? "", agentId)

  return (
    <div className="grid gap-2">
      <Label htmlFor={inputId}>{t("agents.workspace")}</Label>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          id={inputId}
          value={value ?? ""}
          onChange={(event) => onChange(event.target.value)}
          placeholder={defaultsWorkspace}
          autoComplete="off"
          spellCheck={false}
          disabled={disabled}
          aria-describedby={helpId}
        />
      </div>
      <p id={helpId} className="text-muted-foreground text-xs">
        {t("agents.workspaceHelp")}
      </p>
      {!value?.trim() && (
        <p className="text-muted-foreground text-xs">
          {t("agents.workspaceEffective", { path: effective || defaultsWorkspace })}
        </p>
      )}
    </div>
  )
}

interface AgentModelFieldProps {
  idPrefix: string
  models: ModelInfo[]
  value?: string
  defaultModelName?: string
  onChange: (primary: string | undefined) => void
  disabled?: boolean
}

export function AgentModelField({
  idPrefix,
  models,
  value,
  defaultModelName,
  onChange,
  disabled,
}: AgentModelFieldProps) {
  const { t } = useTranslation()
  const selectId = `${idPrefix}-model`

  return (
    <div className="grid gap-2">
      <Label htmlFor={selectId}>{t("agents.model")}</Label>
      <select
        id={selectId}
        value={value ?? ""}
        onChange={(event) =>
          onChange(event.target.value ? event.target.value : undefined)
        }
        disabled={disabled}
        className="border-input bg-background h-9 rounded-md border px-3 text-sm shadow-xs"
      >
        <option value="">
          {t("agents.useDefaultModel", { model: defaultModelName || "—" })}
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
  )
}

interface AgentSkillsFieldProps {
  idPrefix: string
  skills: SkillSupportItem[]
  selected: string[]
  onChange: (skills: string[]) => void
  loading?: boolean
  disabled?: boolean
}

export function AgentSkillsField({
  idPrefix,
  skills,
  selected,
  onChange,
  loading,
  disabled,
}: AgentSkillsFieldProps) {
  const { t } = useTranslation()
  const filterId = `${idPrefix}-skill-filter`
  const [filter, setFilter] = useState("")

  const selectedNotListed = selected.filter(
    (name) => !skills.some((skill) => skill.name === name),
  )
  const visibleSkills = skills.filter((skill) =>
    skill.name.toLowerCase().includes(filter.trim().toLowerCase()),
  )

  const toggleSkill = (name: string, checked: boolean) => {
    if (name === LOCKED_AGENT_SKILL && !checked) return
    const next = checked
      ? [...new Set([...selected, name])]
      : selected.filter((skill) => skill !== name)
    onChange(next)
  }

  const selectAll = () => onChange(skills.map((skill) => skill.name))
  const clearAll = () =>
    onChange(
      skills
        .filter((skill) => skill.name === LOCKED_AGENT_SKILL)
        .map((skill) => skill.name),
    )

  return (
    <div className="grid gap-2">
      <Label htmlFor={filterId}>{t("agents.skills")}</Label>
      <Input
        id={filterId}
        value={filter}
        onChange={(event) => setFilter(event.target.value)}
        placeholder={t("agents.skillsFilterPlaceholder")}
        autoComplete="off"
        disabled={disabled}
      />
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-muted-foreground text-xs">
          {t("agents.skillCount", { count: selected.length })}
        </span>
        <div className="flex gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={selectAll}
            disabled={disabled || skills.length === 0}
          >
            {t("agents.selectAllSkills")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={clearAll}
            disabled={disabled || selected.length === 0}
          >
            {t("agents.clearAllSkills")}
          </Button>
        </div>
      </div>
      {selected.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {selected.map((name) => (
            <Badge key={name} variant="secondary" className="gap-1">
              {name}
              {name === LOCKED_AGENT_SKILL ? (
                <span
                  className="text-muted-foreground"
                  title={t("agents.skillLockedHint")}
                >
                  🔒
                </span>
              ) : (
                <button
                  type="button"
                  className="text-muted-foreground hover:text-foreground"
                  aria-label={t("agents.removeSkill", { name })}
                  onClick={() => toggleSkill(name, false)}
                >
                  ×
                </button>
              )}
            </Badge>
          ))}
        </div>
      )}
      <div className="border-border/60 max-h-44 overflow-y-auto rounded-xl border p-3">
        {loading ? (
          <p className="text-muted-foreground text-xs">{t("agents.skillsLoading")}</p>
        ) : visibleSkills.length === 0 && selectedNotListed.length === 0 ? (
          <p className="text-muted-foreground text-xs">{t("agents.skillsEmpty")}</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {visibleSkills.map((skill) => {
              const locked = skill.name === LOCKED_AGENT_SKILL
              return (
                <li key={skill.id}>
                  <label className="flex items-start gap-2 text-sm">
                    <input
                      type="checkbox"
                      className="accent-primary mt-0.5 size-4"
                      checked={selected.includes(skill.name)}
                      onChange={(event) =>
                        toggleSkill(skill.name, event.target.checked)
                      }
                      disabled={disabled || locked}
                    />
                    <span className="min-w-0">
                      <span className="font-medium">{skill.name}</span>
                      <span className="text-muted-foreground ml-2 text-xs">
                        {skill.source}
                      </span>
                      {locked && (
                        <span className="text-muted-foreground ml-2 text-xs">
                          {t("agents.skillLocked")}
                        </span>
                      )}
                      <span className="text-muted-foreground block text-xs">
                        {skill.description}
                      </span>
                    </span>
                  </label>
                </li>
              )
            })}
            {selectedNotListed.map((name) => {
              const locked = name === LOCKED_AGENT_SKILL
              return (
                <li key={name}>
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      className="accent-primary size-4"
                      checked
                      onChange={() => {
                        if (locked) return
                        toggleSkill(name, false)
                      }}
                      disabled={disabled || locked}
                    />
                    <span className="font-medium">{name}</span>
                    <span className="text-muted-foreground text-xs">
                      {locked
                        ? t("agents.skillLocked")
                        : t("agents.skillNotInWorkspace")}
                    </span>
                  </label>
                </li>
              )
            })}
          </ul>
        )}
      </div>
      <p className="text-muted-foreground text-xs">{t("agents.skillsHelp")}</p>
    </div>
  )
}

interface AgentSubagentsFieldProps {
  agents: AgentView[]
  currentId: string
  value?: AgentSubagentsConfig
  onChange: (value: AgentSubagentsConfig) => void
  disabled?: boolean
}

export function AgentSubagentsField({
  agents,
  currentId,
  value,
  onChange,
  disabled,
}: AgentSubagentsFieldProps) {
  const { t } = useTranslation()
  const enabled = value?.enabled !== false
  const allowAgents = value?.allow_agents ?? []
  const allowAll = allowAgents.includes("*")
  const otherAgents = agents.filter((agent) => agent.id !== currentId)

  const update = (patch: Partial<AgentSubagentsConfig>) => {
    onChange({ ...value, enabled, allow_agents: allowAgents, ...patch })
  }

  const toggleAllowAgent = (id: string, checked: boolean) => {
    const next = checked
      ? [...new Set([...allowAgents, id])]
      : allowAgents.filter((item) => item !== id)
    update({ allow_agents: next })
  }

  return (
    <div className="flex flex-col gap-4">
      <label className="flex items-center justify-between gap-3 rounded-xl border p-3">
        <span className="flex flex-col">
          <span className="text-sm font-medium">{t("agents.subagentsEnabled")}</span>
          <span className="text-muted-foreground text-xs">
            {t("agents.subagentsEnabledHint")}
          </span>
        </span>
        <Switch
          checked={enabled}
          onCheckedChange={(checked) => update({ enabled: checked })}
          disabled={disabled}
        />
      </label>

      {enabled && (
        <div className="grid gap-2">
          <span className="text-sm font-medium">{t("agents.allowAgents")}</span>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="accent-primary size-4"
              checked={allowAll}
              onChange={(event) =>
                update({ allow_agents: event.target.checked ? ["*"] : [] })
              }
              disabled={disabled}
            />
            {t("agents.allowAllAgents")}
          </label>
          {!allowAll && (
            <div className="flex flex-wrap gap-3">
              {otherAgents.map((agent) => (
                <label key={agent.id} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="accent-primary size-4"
                    checked={allowAgents.includes(agent.id)}
                    onChange={(event) =>
                      toggleAllowAgent(agent.id, event.target.checked)
                    }
                    disabled={disabled}
                  />
                  {agent.id}
                </label>
              ))}
              {otherAgents.length === 0 && (
                <span className="text-muted-foreground text-xs">
                  {t("agents.noOtherAgents")}
                </span>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

interface AgentAvatarPickerProps {
  agentId?: string
  avatar?: AgentAvatarData | null
  name?: string
  previewUrl?: string | null
  disabled?: boolean
  canRemove?: boolean
  onSelectFile: (file: File) => void
  onRemove: () => void
}

export function AgentAvatarPicker({
  agentId,
  avatar,
  name,
  previewUrl,
  disabled,
  canRemove,
  onSelectFile,
  onRemove,
}: AgentAvatarPickerProps) {
  const { t } = useTranslation()
  const inputId = useId()

  return (
    <div className="grid gap-2">
      <Label htmlFor={inputId}>{t("agents.avatarImage")}</Label>
      <div className="flex flex-wrap items-center gap-3">
        {previewUrl ? (
          <img
            src={previewUrl}
            alt={name || agentId || "avatar"}
            className="border-border/60 size-10 rounded-full border object-cover"
          />
        ) : (
          <AgentAvatar
            agentId={agentId}
            avatar={avatar}
            name={name}
            size="lg"
            className="border-border/50 border"
          />
        )}
        <label
          htmlFor={inputId}
          className="border-border/60 bg-background hover:bg-accent inline-flex h-9 cursor-pointer items-center rounded-md border px-3 text-sm"
        >
          {t("agents.chooseImage")}
        </label>
        <input
          id={inputId}
          type="file"
          accept="image/png,image/jpeg,image/webp"
          className="hidden"
          disabled={disabled}
          onChange={(event) => {
            const file = event.target.files?.[0]
            event.target.value = ""
            if (file) onSelectFile(file)
          }}
        />
        {canRemove && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={onRemove}
            disabled={disabled}
          >
            {t("agents.avatarRemove")}
          </Button>
        )}
      </div>
      <p className="text-muted-foreground text-xs">{t("agents.avatarImageHelp")}</p>
    </div>
  )
}
