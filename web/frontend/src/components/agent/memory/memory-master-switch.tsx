import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  type MemorySettings,
  getMemorySettings,
  setMemoryEnabled,
} from "@/api/agent-memory"
import { Switch } from "@/components/ui/switch"

export function MemoryMasterSwitch() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const settingsQuery = useQuery({
    queryKey: ["agent-memory-settings"],
    queryFn: getMemorySettings,
  })

  const mutation = useMutation({
    mutationFn: (enabled: boolean) => setMemoryEnabled(enabled),
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
        settings.enabled
          ? t("pages.agent.memory.master_switch_success_on")
          : t("pages.agent.memory.master_switch_success_off"),
      )
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.memory.master_switch_error"),
      )
    },
  })

  const enabled = settingsQuery.data?.enabled ?? true

  return (
    <label className="border-border/50 bg-background flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-1.5">
      <Switch
        size="sm"
        checked={enabled}
        disabled={settingsQuery.isLoading || mutation.isPending}
        onCheckedChange={(checked) => mutation.mutate(checked)}
      />
      <span className="flex flex-col leading-tight">
        <span className="text-xs font-medium">
          {t("pages.agent.memory.master_switch_label")}
        </span>
        <span className="text-muted-foreground text-[10px]">
          {enabled
            ? t("pages.agent.memory.master_switch_on")
            : t("pages.agent.memory.master_switch_off")}
        </span>
      </span>
    </label>
  )
}
