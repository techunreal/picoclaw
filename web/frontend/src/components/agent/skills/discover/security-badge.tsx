import {
  IconAlertTriangle,
  IconBan,
  IconShield,
  IconShieldCheck,
} from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import type { SkillSecuritySummary } from "@/api/skills"
import { Badge } from "@/components/ui/badge"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

interface SecurityBadgeProps {
  security?: SkillSecuritySummary | null
}

export function SecurityBadge({ security }: SecurityBadgeProps) {
  const { t } = useTranslation()

  const decision = security?.decision ?? "unknown"
  const label =
    decision === "pass"
      ? t("pages.agent.skills.security.clean")
      : decision === "warn"
        ? t("pages.agent.skills.security.review")
        : decision === "fail"
          ? t("pages.agent.skills.security.blocked")
          : t("pages.agent.skills.security.unscanned")

  const Icon =
    decision === "pass"
      ? IconShieldCheck
      : decision === "warn"
        ? IconAlertTriangle
        : decision === "fail"
          ? IconBan
          : IconShield

  return (
    <Tooltip delayDuration={200}>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          className={cn(
            "gap-1 text-[10px]",
            decision === "pass" &&
              "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
            decision === "warn" &&
              "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400",
            decision === "fail" &&
              "border-destructive/30 bg-destructive/10 text-destructive",
          )}
        >
          <Icon className="size-3" />
          {label}
          {security?.risk_level ? ` · ${security.risk_level}` : ""}
        </Badge>
      </TooltipTrigger>
      <TooltipContent className="max-w-sm">
        <div className="space-y-1">
          <div className="font-medium">{label}</div>
          {security?.summary ? <p>{security.summary}</p> : null}
          {security?.audit_url ? (
            <a
              href={security.audit_url}
              target="_blank"
              rel="noreferrer"
              className="underline underline-offset-2"
            >
              {t("pages.agent.skills.security.audit_link")}
            </a>
          ) : null}
        </div>
      </TooltipContent>
    </Tooltip>
  )
}
