import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  AgentApiError,
  type AgentConfig,
  type AgentView,
  type DispatchConfig,
  type DispatchRule,
  getAgents,
  toAgentConfig,
  updateAgents,
} from "@/api/agents"
import { showSaveSuccessOrRestartToast } from "@/lib/restart-required"

const listKey = ["agents"] as const

export function agentErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof AgentApiError) {
    return error.errors.length > 0 ? error.errors.join(" ") : error.message
  }
  return fallback
}

export function suggestAgentWorkspace(
  defaultsWorkspace: string,
  id: string,
): string {
  const base = defaultsWorkspace.replace(/[\\/]+$/, "")
  const agentID = id.trim()
  if (!agentID || agentID.toLowerCase() === "main") {
    return base
  }
  const separator = base.includes("\\") ? "\\" : "/"
  const parentIndex = Math.max(base.lastIndexOf("\\"), base.lastIndexOf("/"))
  const parent = parentIndex > 0 ? base.slice(0, parentIndex) : base
  return `${parent}${separator}agents${separator}${agentID}`
}

export function useAgentsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const listQuery = useQuery({ queryKey: listKey, queryFn: getAgents })
  const agents = listQuery.data?.agents ?? []
  const dispatch = listQuery.data?.dispatch ?? { rules: [] }

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [editor, setEditor] = useState<{
    mode: "create" | "edit"
    agent: AgentConfig
  } | null>(null)
  const [ruleEditor, setRuleEditor] = useState<{
    index: number | null
    rule: DispatchRule
  } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<AgentView | null>(null)

  const selectedAgent =
    agents.find((agent) => agent.id === selectedId) ?? agents[0] ?? null

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: listKey })
  }

  const saveMutation = useMutation({
    mutationFn: ({
      agents: nextAgents,
      dispatch: nextDispatch,
    }: {
      agents: AgentConfig[]
      dispatch: DispatchConfig | null
    }) => updateAgents(nextAgents, nextDispatch),
    onSuccess: (result) => {
      showSaveSuccessOrRestartToast(
        t,
        t("agents.saved"),
        t("agents.title"),
        result.restart_required,
      )
      setEditor(null)
      setRuleEditor(null)
      invalidate()
    },
    onError: (error) => {
      toast.error(agentErrorMessage(error, t("agents.saveFailed")))
    },
  })

  const persist = (
    nextAgents: AgentConfig[],
    nextDispatch: DispatchConfig | null,
    options?: { selectId?: string },
  ) => {
    if (options?.selectId) setSelectedId(options.selectId)
    saveMutation.mutate({ agents: nextAgents, dispatch: nextDispatch })
  }

  const saveAgent = async (agent: AgentConfig): Promise<boolean> => {
    const normalized: AgentConfig = {
      ...agent,
      default: Boolean(agent.default),
    }
    let next: AgentConfig[]
    if (editor?.mode === "create") {
      next = [...agents.map(toAgentConfig), normalized]
    } else {
      next = agents.map((item) =>
        item.id === normalized.id ? normalized : toAgentConfig(item),
      )
    }
    if (normalized.default) {
      next = next.map((item) => ({
        ...item,
        default: item.id === normalized.id,
      }))
    }
    setSelectedId(normalized.id)
    try {
      await saveMutation.mutateAsync({
        agents: next,
        dispatch: dispatch.rules.length > 0 ? dispatch : null,
      })
      return true
    } catch {
      return false
    }
  }

  const deleteAgent = () => {
    if (!deleteTarget) return
    const removedID = deleteTarget.id
    const remaining = agents
      .filter((agent) => agent.id !== removedID)
      .map((agent) => {
        const config = toAgentConfig(agent)
        const allow = config.subagents?.allow_agents
        if (allow && allow.includes(removedID)) {
          config.subagents = {
            ...config.subagents,
            allow_agents: allow.filter((id) => id !== removedID),
          }
        }
        return config
      })
    const remainingRules = dispatch.rules.filter(
      (rule) => rule.agent !== deleteTarget.id,
    )
    if (remainingRules.length !== dispatch.rules.length) {
      toast.info(t("agents.routingRemoved", { count: dispatch.rules.length - remainingRules.length }))
    }
    persist(
      remaining,
      remainingRules.length > 0 ? { rules: remainingRules } : null,
      { selectId: remaining[0]?.id },
    )
    setDeleteTarget(null)
  }

  const saveRule = (rule: DispatchRule) => {
    const rules = [...dispatch.rules]
    if (ruleEditor?.index === null || ruleEditor?.index === undefined) {
      rules.push(rule)
    } else {
      rules[ruleEditor.index] = rule
    }
    persist(agents.map(toAgentConfig), { rules })
  }

  const deleteRule = (index: number) => {
    const rules = dispatch.rules.filter((_, i) => i !== index)
    persist(agents.map(toAgentConfig), rules.length > 0 ? { rules } : null)
  }

  const moveRule = (index: number, delta: number) => {
    const target = index + delta
    if (target < 0 || target >= dispatch.rules.length) return
    const rules = [...dispatch.rules]
    const [rule] = rules.splice(index, 1)
    rules.splice(target, 0, rule)
    persist(agents.map(toAgentConfig), { rules })
  }

  return {
    listQuery,
    agents,
    dispatch,
    selectedAgent,
    selectedId,
    setSelectedId,
    editor,
    setEditor,
    openCreate: () => {
      saveMutation.reset()
      setEditor({
        mode: "create",
        agent: { id: "", name: "", default: false },
      })
    },
    openEdit: (agent: AgentView) => {
      saveMutation.reset()
      setEditor({ mode: "edit", agent: toAgentConfig(agent) })
    },
    saveAgent,
    ruleEditor,
    setRuleEditor,
    openCreateRule: () => {
      saveMutation.reset()
      setRuleEditor({
        index: null,
        rule: { agent: agents[0]?.id ?? "", when: {} },
      })
    },
    openEditRule: (index: number) => {
      saveMutation.reset()
      setRuleEditor({ index, rule: { ...dispatch.rules[index] } })
    },
    saveRule,
    deleteRule,
    moveRule,
    deleteTarget,
    setDeleteTarget,
    deleteAgent,
    isSaving: saveMutation.isPending,
    saveError: saveMutation.error,
  }
}
