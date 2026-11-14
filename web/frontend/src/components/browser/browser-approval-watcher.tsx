import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  type BrowserApproval,
  type BrowserApprovalDecision,
  listBrowserApprovals,
  resolveBrowserApproval,
} from "@/api/browser-approvals"
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"

const queryKey = ["browser-approvals"] as const

export function BrowserApprovalWatcher() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [active, setActive] = useState<BrowserApproval | null>(null)

  const approvalsQuery = useQuery({
    queryKey,
    queryFn: listBrowserApprovals,
    refetchInterval: 2000,
  })

  useEffect(() => {
    if (active) {
      return
    }
    const next = approvalsQuery.data?.[0]
    if (next) {
      setActive(next)
    }
  }, [active, approvalsQuery.data])

  const resolveMutation = useMutation({
    mutationFn: ({
      id,
      decision,
    }: {
      id: string
      decision: BrowserApprovalDecision
    }) => resolveBrowserApproval(id, decision),
    onSuccess: () => {
      setActive(null)
      void queryClient.invalidateQueries({ queryKey })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "browserApproval.error",
              "Could not answer the browser permission request.",
            ),
      )
      setActive(null)
    },
  })

  if (!active) {
    return null
  }

  const answer = (decision: BrowserApprovalDecision) => {
    resolveMutation.mutate({ id: active.id, decision })
  }

  return (
    <AlertDialog open>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t("browserApproval.title", "Allow this website?")}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("browserApproval.description", {
              domain: active.domain,
              defaultValue:
                "The agent wants to open {{domain}}. Only allow sites you trust.",
            })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter className="flex-wrap gap-2 sm:justify-end">
          <Button
            type="button"
            variant="ghost"
            onClick={() => answer("deny")}
            disabled={resolveMutation.isPending}
          >
            {t("browserApproval.deny", "Deny")}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => answer("once")}
            disabled={resolveMutation.isPending}
          >
            {t("browserApproval.once", "Allow once")}
          </Button>
          <Button
            type="button"
            onClick={() => answer("always")}
            disabled={resolveMutation.isPending}
          >
            {t("browserApproval.always", "Always allow")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
