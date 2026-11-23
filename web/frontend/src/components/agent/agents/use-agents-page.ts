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
  deleteAgent as deleteAgentRequest,
  exportAgentBundle,
  getAgents,
  importAgentBundle,
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

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteAgentRequest(id),
    onSuccess: (result, id) => {
      showSaveSuccessOrRestartToast(
        t,
        t("agents.deleted"),
        t("agents.title"),
        Boolean(result.restart_required),
      )
      if (result.warning) toast.warning(result.warning)
      setDeleteTarget(null)
      setSelectedId((current) => {
        if (current !== id) return current
        const next = agents.find((agent) => agent.id !== id)
        return next?.id ?? null
      })
      invalidate()
    },
    onError: (error) => {
      toast.error(agentErrorMessage(error, t("agents.deleteFailed")))
    },
  })

  const importMutation = useMutation({
    mutationFn: (file: File) => importAgentBundle(file),
    onSuccess: (result) => {
      showSaveSuccessOrRestartToast(
        t,
        t("agents.importAgentSuccess", { id: result.agent }),
        t("agents.title"),
        Boolean(result.restart_required),
      )
      if (result.renamed) {
        toast.info(t("agents.importRenamed", { id: result.agent }))
      }
      invalidate()
    },
    onError: (error) => {
      toast.error(agentErrorMessage(error, t("agents.importAgentFailed")))
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

  const deleteAgent = () => {
    if (!deleteTarget || deleteMutation.isPending) return
    deleteMutation.mutate(deleteTarget.id)
  }

  const [exportingId, setExportingId] = useState<string | null>(null)

  const exportAgent = async (agent: AgentView) => {
    if (exportingId) return
    setExportingId(agent.id)
    try {
      const blob = await exportAgentBundle(agent.id)
      const url = URL.createObjectURL(blob)
      const link = document.createElement("a")
      link.href = url
      link.download = `picoclaw-agent-${agent.id}.zip`
      document.body.appendChild(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
      toast.success(t("agents.exportSuccess"))
    } catch (error) {
      toast.error(agentErrorMessage(error, t("agents.exportFailed")))
    } finally {
      setExportingId(null)
    }
  }

  const importAgent = (file: File) => {
    if (importMutation.isPending) return
    importMutation.mutate(file)
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
    exportAgent,
    exportingId,
    importAgent,
    isImporting: importMutation.isPending,
    isSaving: saveMutation.isPending || deleteMutation.isPending || importMutation.isPending,
    saveError: saveMutation.error,
  }
}
