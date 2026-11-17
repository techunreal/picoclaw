import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { getFirewallStatus, setFirewallRule } from "@/api/system"
import { Button } from "@/components/ui/button"

export function FirewallCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const query = useQuery({
    queryKey: ["system", "firewall"],
    queryFn: getFirewallStatus,
    staleTime: 5000,
  })

  const mutation = useMutation({
    mutationFn: setFirewallRule,
    onSuccess: () => {
      toast.info(
        t(
          "pages.config.firewall_prompt",
          "Confirm the Windows permission prompt to apply the rule.",
        ),
      )
      const refresh = () =>
        queryClient.invalidateQueries({ queryKey: ["system", "firewall"] })
      window.setTimeout(refresh, 2500)
      window.setTimeout(refresh, 8000)
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.config.firewall_error",
              "Could not change the firewall rule.",
            ),
      ),
  })

  const status = query.data

  return (
    <div className="bg-card border-border/40 flex flex-col gap-3 rounded-2xl border p-5 shadow-sm sm:flex-row sm:items-center sm:justify-between">
      <div className="space-y-1">
        <p className="text-sm font-medium">
          {t("pages.config.firewall_title", "Windows Firewall")}
        </p>
        {!status ? (
          <p className="text-muted-foreground text-xs">
            {t("labels.loading", "Loading…")}
          </p>
        ) : !status.supported ? (
          <p className="text-muted-foreground text-xs">
            {t(
              "pages.config.firewall_unsupported",
              "Firewall rules can only be managed on Windows.",
            )}
          </p>
        ) : status.present ? (
          <p className="text-xs text-emerald-600 dark:text-emerald-400">
            {t(
              "pages.config.firewall_allowed",
              "Inbound access is allowed for TCP {{port}} (local network only).",
              { port: status.port },
            )}
          </p>
        ) : (
          <p className="text-xs text-amber-600 dark:text-amber-400">
            {t(
              "pages.config.firewall_missing",
              "Inbound access is not allowed yet. Phones on your network cannot connect until the rule exists.",
            )}
          </p>
        )}
      </div>

      {status?.supported && (
        <Button
          type="button"
          variant={status.present ? "outline" : "default"}
          className="h-10 shrink-0 rounded-xl px-4"
          onClick={() =>
            mutation.mutate(status.present ? "remove" : "allow")
          }
          disabled={mutation.isPending}
        >
          {status.present
            ? t("pages.config.firewall_remove", "Remove rule")
            : t("pages.config.firewall_allow", "Allow through firewall")}
        </Button>
      )}
    </div>
  )
}
