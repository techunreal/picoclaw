import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  CronApiError,
  type CronJob,
  type CronJobInput,
  createCronJob,
  deleteCronJob,
  listCronJobs,
  runCronJobNow,
  setCronJobEnabled,
  updateCronJob,
} from "@/api/cron"

const listKey = ["cron", "jobs"] as const

export function cronErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof CronApiError) {
    return error.message
  }
  return fallback
}

export function useCronPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingJob, setEditingJob] = useState<CronJob | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<CronJob | null>(null)

  const listQuery = useQuery({
    queryKey: listKey,
    queryFn: listCronJobs,
    refetchInterval: 15_000,
  })

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: listKey })
  }

  const createMutation = useMutation({
    mutationFn: (input: CronJobInput) => createCronJob(input),
    onSuccess: () => {
      toast.success(t("cron.saved"))
      setDialogOpen(false)
      invalidate()
    },
    onError: (error) =>
      toast.error(cronErrorMessage(error, t("cron.saveFailed"))),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, input }: { id: string; input: CronJobInput }) =>
      updateCronJob(id, input),
    onSuccess: () => {
      toast.success(t("cron.saved"))
      setDialogOpen(false)
      setEditingJob(null)
      invalidate()
    },
    onError: (error) =>
      toast.error(cronErrorMessage(error, t("cron.saveFailed"))),
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      setCronJobEnabled(id, enabled),
    onSuccess: (job) => {
      toast.success(job.enabled ? t("cron.enabledToast") : t("cron.disabledToast"))
      invalidate()
    },
    onError: (error) =>
      toast.error(cronErrorMessage(error, t("cron.updateFailed"))),
  })

  const runMutation = useMutation({
    mutationFn: (id: string) => runCronJobNow(id),
    onSuccess: () => {
      toast.success(t("cron.runQueued"))
      window.setTimeout(invalidate, 1500)
    },
    onError: (error) =>
      toast.error(cronErrorMessage(error, t("cron.runFailed"))),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteCronJob(id),
    onSuccess: () => {
      toast.success(t("cron.deleted"))
      setDeleteTarget(null)
      invalidate()
    },
    onError: (error) =>
      toast.error(cronErrorMessage(error, t("cron.deleteFailed"))),
  })

  const openCreate = () => {
    setEditingJob(null)
    setDialogOpen(true)
  }

  const openEdit = (job: CronJob) => {
    setEditingJob(job)
    setDialogOpen(true)
  }

  const handleDialogOpenChange = (open: boolean) => {
    setDialogOpen(open)
    if (!open) {
      setEditingJob(null)
    }
  }

  const handleSave = (input: CronJobInput) => {
    if (editingJob) {
      updateMutation.mutate({ id: editingJob.id, input })
      return
    }
    createMutation.mutate(input)
  }

  return {
    listQuery,
    jobs: listQuery.data?.jobs ?? [],
    status: listQuery.data?.status,
    dialogOpen,
    editingJob,
    deleteTarget,
    isSaving: createMutation.isPending || updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
    pendingRunId: runMutation.isPending ? runMutation.variables : undefined,
    openCreate,
    openEdit,
    handleDialogOpenChange,
    handleSave,
    setDeleteTarget,
    handleDelete: () => {
      if (deleteTarget) {
        deleteMutation.mutate(deleteTarget.id)
      }
    },
    handleRunNow: (job: CronJob) => runMutation.mutate(job.id),
    handleToggle: (job: CronJob, enabled: boolean) =>
      toggleMutation.mutate({ id: job.id, enabled }),
  }
}
