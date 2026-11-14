import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  AgentFileApiError,
  type AgentFileDetail,
  type AgentFileItem,
  getAgentFile,
  getAgentFileHistory,
  listAgentFiles,
  restoreAgentFile,
  saveAgentFile,
} from "@/api/agent-files"

import { agentFileLabel, frontmatterChanged } from "./format"

export type MemoryView = "edit" | "preview"

export interface MemoryEditorState {
  id: string
  revision: string
  exists: boolean
  editable: boolean
  content: string
  draft: string
}

const listKey = ["agent-files", "list"] as const
const detailKey = (id: string, agent?: string) =>
  ["agent-files", "detail", agent ?? "", id] as const
const historyKey = (id: string, agent?: string) =>
  ["agent-files", "history", agent ?? "", id] as const

interface WriteVariables {
  id: string
  name: string
  content: string
  revision: string
  frontmatterChanged: boolean
}

interface RestoreVariables {
  id: string
  revision: string
  createdAt: number
}

function createTemplate(item: AgentFileItem): string {
  if (item.id.startsWith("daily:")) {
    return `# ${item.label}\n\n`
  }
  return ""
}

function toEditorState(detail: AgentFileDetail): MemoryEditorState {
  return {
    id: detail.id,
    revision: detail.revision,
    exists: detail.exists,
    editable: detail.editable,
    content: detail.content,
    draft: detail.content,
  }
}

export function useMemoryPage(agentId?: string) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [view, setView] = useState<MemoryView>("edit")
  const [editor, setEditor] = useState<MemoryEditorState | null>(null)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [historyTargetId, setHistoryTargetId] = useState<string | null>(null)
  const [conflictOpen, setConflictOpen] = useState(false)
  const [restartWarningOpen, setRestartWarningOpen] = useState(false)

  useEffect(() => {
    setSelectedId(null)
    setEditor(null)
    setHistoryOpen(false)
    setHistoryTargetId(null)
    setConflictOpen(false)
  }, [agentId])

  const listQuery = useQuery({
    queryKey: [...listKey, agentId ?? ""],
    queryFn: () => listAgentFiles(agentId),
  })

  const files = useMemo(
    () => listQuery.data?.groups.flatMap((group) => group.files) ?? [],
    [listQuery.data?.groups],
  )

  const selectedItem = useMemo(
    () => files.find((file) => file.id === selectedId) ?? null,
    [files, selectedId],
  )

  // Preselect a sensible file once the list arrives.
  useEffect(() => {
    if (selectedId !== null || files.length === 0) {
      return
    }
    const preferred =
      files.find((file) => file.exists && file.editable) ??
      files.find((file) => file.exists) ??
      files.find((file) => file.creatable) ??
      files[0]
    if (preferred) {
      setSelectedId(preferred.id)
    }
  }, [files, selectedId])

  const detailQuery = useQuery({
    queryKey: detailKey(selectedId ?? "", agentId),
    queryFn: () => getAgentFile(selectedId as string, agentId),
    enabled: selectedId !== null,
  })

  // Adopt server content whenever the revision changes (initial load, reload,
  // restore). Local edits keep the same revision, so they are preserved.
  useEffect(() => {
    const detail = detailQuery.data
    if (!detail) {
      return
    }
    setEditor((previous) => {
      if (
        previous &&
        previous.id === detail.id &&
        previous.revision === detail.revision
      ) {
        return previous
      }
      return toEditorState(detail)
    })
  }, [detailQuery.data])

  const writeMutation = useMutation({
    mutationFn: (variables: WriteVariables) =>
      saveAgentFile(
        variables.id,
        {
          content: variables.content,
          revision: variables.revision,
        },
        agentId,
      ),
    onSuccess: (response, variables) => {
      const cached = queryClient.getQueryData<AgentFileDetail>(
        detailKey(variables.id, agentId),
      )
      if (cached) {
        queryClient.setQueryData<AgentFileDetail>(
          detailKey(variables.id, agentId),
          {
          ...cached,
          content: variables.content,
          revision: response.revision,
          exists: true,
          size: response.size,
          modified_at: response.modified_at,
        })
      }
      setEditor((previous) =>
        previous && previous.id === variables.id
          ? {
              ...previous,
              exists: true,
              revision: response.revision,
              content: variables.content,
              draft: variables.content,
            }
          : previous,
      )
      void queryClient.invalidateQueries({ queryKey: ["agent-files", "list"] })
      setConflictOpen(false)
      toast.success(
        t("pages.agent.memory.save_success", { name: variables.name }),
      )
      if (variables.frontmatterChanged) {
        toast.warning(t("pages.agent.memory.restart_warning_toast"))
      }
    },
    onError: (error) => {
      if (error instanceof AgentFileApiError && error.status === 409) {
        setConflictOpen(true)
        return
      }
      if (error instanceof AgentFileApiError && error.status === 403) {
        toast.error(t("pages.agent.memory.save_read_only"))
        return
      }
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.memory.save_error"),
      )
    },
  })

  const createMutation = useMutation({
    mutationFn: (variables: { item: AgentFileItem; content: string }) =>
      saveAgentFile(
        variables.item.id,
        {
          content: variables.content,
          revision: "",
        },
        agentId,
      ),
    onSuccess: (response, variables) => {
      const detail: AgentFileDetail = {
        ...variables.item,
        exists: true,
        content: variables.content,
        revision: response.revision,
        size: response.size,
        modified_at: response.modified_at,
      }
      queryClient.setQueryData<AgentFileDetail>(
        detailKey(variables.item.id, agentId),
        detail,
      )
      setEditor(toEditorState(detail))
      setView("edit")
      void queryClient.invalidateQueries({ queryKey: ["agent-files", "list"] })
      toast.success(t("pages.agent.memory.create_success"))
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.memory.create_error"),
      )
    },
  })

  const restoreMutation = useMutation({
    mutationFn: (variables: RestoreVariables) =>
      restoreAgentFile(variables.id, variables.revision, agentId),
    onSuccess: (_response, variables) => {
      setHistoryOpen(false)
      toast.success(
        t("pages.agent.memory.history_restore_success", {
          date: new Date(variables.createdAt * 1000).toLocaleString(),
        }),
      )
      void queryClient.invalidateQueries({ queryKey: ["agent-files", "list"] })
      void queryClient.invalidateQueries({
        queryKey: historyKey(variables.id, agentId),
      })
      void getAgentFile(variables.id, agentId)
        .then((detail) => {
          queryClient.setQueryData<AgentFileDetail>(
            detailKey(variables.id, agentId),
            detail,
          )
          setEditor((previous) =>
            previous && previous.id === detail.id
              ? toEditorState(detail)
              : previous,
          )
        })
        .catch(() => {
          // Keep the current editor state; the next manual reload can retry.
        })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.memory.history_restore_error"),
      )
    },
  })

  const historyQuery = useQuery({
    queryKey: historyKey(historyTargetId ?? "", agentId),
    queryFn: () => getAgentFileHistory(historyTargetId as string, agentId),
    enabled: historyOpen && historyTargetId !== null,
  })

  const performSave = (revisionOverride?: string) => {
    if (!editor) {
      return
    }
    const name = selectedItem ? agentFileLabel(selectedItem, t) : editor.id
    writeMutation.mutate({
      id: editor.id,
      name,
      content: editor.draft,
      revision: revisionOverride ?? (editor.exists ? editor.revision : ""),
      frontmatterChanged:
        editor.id === "agent" &&
        frontmatterChanged(editor.content, editor.draft),
    })
  }

  const handleSave = () => {
    if (!editor) {
      return
    }
    if (
      editor.id === "agent" &&
      frontmatterChanged(editor.content, editor.draft)
    ) {
      setRestartWarningOpen(true)
      return
    }
    performSave()
  }

  const handleConfirmRestartWarning = () => {
    setRestartWarningOpen(false)
    performSave()
  }

  const handleSelectFile = (id: string) => {
    if (id === selectedId) {
      return
    }
    setSelectedId(id)
    setEditor(null)
    setView("edit")
  }

  const handleDraftChange = (draft: string) => {
    setEditor((previous) => (previous ? { ...previous, draft } : previous))
  }

  const handleRevert = () => {
    setEditor((previous) =>
      previous ? { ...previous, draft: previous.content } : previous,
    )
  }

  const handleReload = () => {
    if (!editor) {
      return
    }
    const id = editor.id
    void getAgentFile(id, agentId)
      .then((detail) => {
        queryClient.setQueryData<AgentFileDetail>(detailKey(id, agentId), detail)
        setEditor(toEditorState(detail))
        setConflictOpen(false)
        toast.success(t("pages.agent.memory.reload_success"))
      })
      .catch((error: unknown) => {
        toast.error(
          error instanceof Error
            ? error.message
            : t("pages.agent.memory.detail_load_error"),
        )
      })
  }

  const handleConflictOverwrite = () => {
    if (!editor) {
      return
    }
    const id = editor.id
    void getAgentFile(id, agentId)
      .then((latest) => {
        setConflictOpen(false)
        performSave(latest.exists ? latest.revision : "")
      })
      .catch((error: unknown) => {
        toast.error(
          error instanceof Error
            ? error.message
            : t("pages.agent.memory.conflict_error"),
        )
      })
  }

  const handleOpenHistory = () => {
    if (!editor) {
      return
    }
    setHistoryTargetId(editor.id)
    setHistoryOpen(true)
  }

  const handleCreate = () => {
    const item = selectedItem
    if (!item) {
      return
    }
    createMutation.mutate({ item, content: createTemplate(item) })
  }

  const handleRestore = (revision: string, createdAt: number) => {
    if (!historyTargetId) {
      return
    }
    restoreMutation.mutate({ id: historyTargetId, revision, createdAt })
  }

  const activeItem =
    editor && selectedItem && editor.id === selectedItem.id
      ? { ...selectedItem, exists: editor.exists, editable: editor.editable }
      : selectedItem

  return {
    groups: listQuery.data?.groups ?? [],
    files,
    selectedItem: activeItem,
    editor,
    view,
    isLoadingList: listQuery.isLoading,
    listError: listQuery.error,
    isLoadingDetail: detailQuery.isLoading,
    detailError: detailQuery.error,
    isSaving: writeMutation.isPending,
    isCreating: createMutation.isPending,
    isRestoring: restoreMutation.isPending,
    historyOpen,
    history: historyQuery.data?.history ?? [],
    isLoadingHistory: historyQuery.isLoading,
    historyError: historyQuery.error,
    conflictOpen,
    restartWarningOpen,
    setView,
    setConflictOpen,
    setRestartWarningOpen,
    setHistoryOpen,
    handleSelectFile,
    handleDraftChange,
    handleSave,
    handleConfirmRestartWarning,
    handleRevert,
    handleReload,
    handleConflictOverwrite,
    handleOpenHistory,
    handleCreate,
    handleRestore,
  }
}
