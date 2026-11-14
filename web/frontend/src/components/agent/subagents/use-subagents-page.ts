import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  SubagentApiError,
  type SubagentRun,
  cancelSubagentRun,
  listSubagentRuns,
} from "@/api/subagents"

const listKey = ["subagents", "runs"] as const

export function subagentErrorMessage(
  error: unknown,
  fallback: string,
): string {
  if (error instanceof SubagentApiError) {
    return error.message
  }
  return fallback
}

export function useSubagentsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [cancelTarget, setCancelTarget] = useState<SubagentRun | null>(null)

  const listQuery = useQuery({
    queryKey: listKey,
    queryFn: listSubagentRuns,
    refetchInterval: 2500,
  })

  const cancelMutation = useMutation({
    mutationFn: (id: string) => cancelSubagentRun(id),
    onSuccess: () => {
      toast.success(t("subagents.canceled"))
      setCancelTarget(null)
      void queryClient.invalidateQueries({ queryKey: listKey })
    },
    onError: (error) => {
      toast.error(subagentErrorMessage(error, t("subagents.cancelFailed")))
      void queryClient.invalidateQueries({ queryKey: listKey })
    },
  })

  const runs = listQuery.data ?? []
  const counts = { running: 0, completed: 0, failed: 0, canceled: 0 }
  for (const run of runs) {
    if (run.status === "running") counts.running += 1
    else if (run.status === "completed") counts.completed += 1
    else if (run.status === "failed") counts.failed += 1
    else if (run.status === "canceled") counts.canceled += 1
  }

  return {
    listQuery,
    runs,
    counts,
    cancelTarget,
    setCancelTarget,
    cancelMutation,
    confirmCancel: () => {
      if (cancelTarget) {
        cancelMutation.mutate(cancelTarget.id)
      }
    },
  }
}
