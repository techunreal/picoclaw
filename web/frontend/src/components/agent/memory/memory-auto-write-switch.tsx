import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  type MemorySettings,
  getMemorySettings,
  setMemoryAutoWrite,
} from "@/api/agent-memory"
import { Switch } from "@/components/ui/switch"

export function MemoryAutoWriteSwitch() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const settingsQuery = useQuery({
    queryKey: ["agent-memory-settings"],
    queryFn: getMemorySettings,
  })

  const mutation = useMutation({
    mutationFn: (autoWrite: boolean) => setMemoryAutoWrite(autoWrite),
    onSuccess: (settings) => {
      queryClient.setQueryData<MemorySettings>(
        ["agent-memory-settings"],
        settings,
      )
      if (settings.restart_required) {
        toast.warning(t("pages.agent.memory.restart_required"))
        return
      }
      toast.success(
        settings.auto_write
          ? t("pages.agent.memory.auto_write_success_on")
          : t("pages.agent.memory.auto_write_success_off"),
      )
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.memory.auto_write_error"),
      )
    },
  })

  const memoryEnabled = settingsQuery.data?.enabled ?? true
  const autoWrite = settingsQuery.data?.auto_write ?? false
  const lastWriteAt = settingsQuery.data?.last_write_at

  const status = !memoryEnabled
    ? t("pages.agent.memory.auto_write_blocked")
    : autoWrite
      ? t("pages.agent.memory.auto_write_on")
      : t("pages.agent.memory.auto_write_off")

  return (
    <label className="border-border/50 bg-background flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-1.5">
      <Switch
        size="sm"
        checked={autoWrite}
        disabled={
          settingsQuery.isLoading || mutation.isPending || !memoryEnabled
        }
        onCheckedChange={(checked) => mutation.mutate(checked)}
      />
      <span className="flex flex-col leading-tight">
        <span className="text-xs font-medium">
          {t("pages.agent.memory.auto_write_label")}
        </span>
        <span className="text-muted-foreground text-[10px]">{status}</span>
        <span className="text-muted-foreground text-[10px]">
          {lastWriteAt
            ? t("pages.agent.memory.auto_write_last", {
                date: new Date(lastWriteAt * 1000).toLocaleString(),
              })
            : t("pages.agent.memory.auto_write_never")}
        </span>
      </span>
    </label>
  )
}
