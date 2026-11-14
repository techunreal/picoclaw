import {
  IconAlertTriangle,
  IconChartBar,
  IconCoin,
  IconRefresh,
} from "@tabler/icons-react"
import { useQuery } from "@tanstack/react-query"
import dayjs from "dayjs"
import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"

import type { UsageModelSummary, UsageSessionSummary } from "@/api/usage"
import { getUsageSummary } from "@/api/usage"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value)
}

function formatCost(value: number | null | undefined, currency?: string): string {
  if (value == null || !Number.isFinite(value)) return "—"
  const code = (currency || "USD").toUpperCase()
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: code,
      maximumFractionDigits: value < 1 ? 6 : 2,
    }).format(value)
  } catch {
    return `${value.toFixed(6)} ${code}`
  }
}

function shortKey(key: string): string {
  if (key.length <= 20) return key
  return `${key.slice(0, 10)}…${key.slice(-6)}`
}

function StatCard({
  label,
  value,
  hint,
  icon,
}: {
  label: string
  value: string
  hint?: string
  icon?: ReactNode
}) {
  return (
    <div className="bg-card flex flex-col gap-2 rounded-2xl border p-4 shadow-xs">
      <div className="text-muted-foreground flex items-center gap-2 text-xs font-medium uppercase">
        {icon}
        {label}
      </div>
      <p className="text-2xl font-semibold tracking-tight">{value}</p>
      {hint && <p className="text-muted-foreground text-xs">{hint}</p>}
    </div>
  )
}

function ModelRow({ model }: { model: UsageModelSummary }) {
  const { t } = useTranslation()
  return (
    <tr className="border-border/50 border-b last:border-0">
      <td className="py-2 pr-3 font-mono text-xs">{model.model}</td>
      <td className="px-3 py-2 text-right text-sm">{formatNumber(model.llm_calls)}</td>
      <td className="px-3 py-2 text-right text-sm">
        {formatNumber(model.input_tokens)}
      </td>
      <td className="px-3 py-2 text-right text-sm">
        {formatNumber(model.output_tokens)}
      </td>
      <td className="px-3 py-2 text-right text-sm font-medium">
        {formatNumber(model.total_tokens)}
      </td>
      <td className="py-2 pl-3 text-right text-sm">
        {model.price_configured ? (
          formatCost(model.estimated_cost, model.currency)
        ) : (
          <span className="text-muted-foreground text-xs">
            {t("usage.noPrice")}
          </span>
        )}
      </td>
    </tr>
  )
}

function SessionRow({ session }: { session: UsageSessionSummary }) {
  const { t } = useTranslation()
  return (
    <tr className="border-border/50 border-b last:border-0">
      <td className="py-2 pr-3">
        <p className="font-mono text-xs" title={session.session_key}>
          {shortKey(session.session_key)}
        </p>
        <p className="text-muted-foreground text-xs">
          {[session.channel, session.chat_id, session.agent_id]
            .filter(Boolean)
            .join(" · ") || "—"}
        </p>
      </td>
      <td className="px-3 py-2 text-right text-sm">
        {formatNumber(session.llm_calls)}
      </td>
      <td className="px-3 py-2 text-right text-sm">
        {formatNumber(session.input_tokens)}
      </td>
      <td className="px-3 py-2 text-right text-sm">
        {formatNumber(session.output_tokens)}
      </td>
      <td className="px-3 py-2 text-right text-sm font-medium">
        {formatNumber(session.total_tokens)}
      </td>
      <td className="py-2 pl-3 text-right text-sm">
        {session.estimated_cost != null
          ? formatCost(session.estimated_cost, session.currency)
          : t("usage.noPrice")}
      </td>
    </tr>
  )
}

export function UsagePage() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ["usage", "summary"],
    queryFn: getUsageSummary,
    refetchInterval: 5000,
  })
  const data = query.data
  const error = query.error
    ? query.error instanceof Error
      ? query.error.message
      : t("usage.loadFailed")
    : null

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("usage.title")}>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-muted-foreground hover:text-foreground h-8 w-8 rounded-full"
          onClick={() => void query.refetch()}
          disabled={query.isFetching}
          aria-label={t("usage.refresh")}
          title={t("usage.refresh")}
        >
          <IconRefresh className="size-4" />
        </Button>
      </PageHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 md:px-8">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
          <p className="text-muted-foreground text-sm">{t("usage.description")}</p>

          {error && (
            <div className="border-destructive/30 bg-destructive/5 text-destructive flex items-start gap-2 rounded-xl border p-4 text-sm">
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {query.isLoading && (
            <div className="text-muted-foreground py-16 text-center text-sm">
              {t("usage.loading")}
            </div>
          )}

          {data && data.totals.llm_calls === 0 && (
            <div className="border-border/60 flex flex-col items-center gap-3 rounded-2xl border border-dashed p-12 text-center">
              <IconChartBar className="text-muted-foreground size-10 opacity-60" />
              <div>
                <p className="font-medium">{t("usage.emptyTitle")}</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  {t("usage.emptyDescription")}
                </p>
              </div>
            </div>
          )}

          {data && data.totals.llm_calls > 0 && (
            <>
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <StatCard
                  label={t("usage.totalTokens")}
                  value={formatNumber(data.totals.total_tokens)}
                  hint={t("usage.calls", { count: data.totals.llm_calls })}
                  icon={<IconChartBar className="size-4" />}
                />
                <StatCard
                  label={t("usage.inputTokens")}
                  value={formatNumber(data.totals.input_tokens)}
                />
                <StatCard
                  label={t("usage.outputTokens")}
                  value={formatNumber(data.totals.output_tokens)}
                />
                <StatCard
                  label={t("usage.estimatedCost")}
                  value={formatCost(data.estimated_cost, data.currency)}
                  hint={
                    data.cost_complete
                      ? t("usage.pricesConfigured", {
                          count: data.priced_models,
                        })
                      : t("usage.partialCost", {
                          priced: data.priced_models,
                          unpriced: data.unpriced_models,
                        })
                  }
                  icon={<IconCoin className="size-4" />}
                />
              </div>

              <p className="text-muted-foreground text-xs">
                {t("usage.since", {
                  time: dayjs(data.started_at_ms).format("YYYY-MM-DD HH:mm:ss"),
                })}{" "}
                · {t("usage.resetHint")}
              </p>

              <section className="bg-card flex flex-col gap-3 rounded-2xl border p-4 shadow-xs">
                <h2 className="text-sm font-semibold">{t("usage.modelsTitle")}</h2>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="text-muted-foreground border-border/60 border-b text-left text-xs uppercase">
                        <th className="py-2 pr-3 font-medium">{t("usage.model")}</th>
                        <th className="px-3 py-2 text-right font-medium">
                          {t("usage.callsLabel")}
                        </th>
                        <th className="px-3 py-2 text-right font-medium">
                          {t("usage.input")}
                        </th>
                        <th className="px-3 py-2 text-right font-medium">
                          {t("usage.output")}
                        </th>
                        <th className="px-3 py-2 text-right font-medium">
                          {t("usage.total")}
                        </th>
                        <th className="py-2 pl-3 text-right font-medium">
                          {t("usage.cost")}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.models.map((model) => (
                        <ModelRow key={model.model} model={model} />
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>

              <section className="bg-card flex flex-col gap-3 rounded-2xl border p-4 shadow-xs">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h2 className="text-sm font-semibold">{t("usage.sessionsTitle")}</h2>
                  <Badge variant="outline">
                    {t("usage.sessionCount", { count: data.sessions.length })}
                  </Badge>
                </div>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="text-muted-foreground border-border/60 border-b text-left text-xs uppercase">
                        <th className="py-2 pr-3 font-medium">{t("usage.session")}</th>
                        <th className="px-3 py-2 text-right font-medium">
                          {t("usage.callsLabel")}
                        </th>
                        <th className="px-3 py-2 text-right font-medium">
                          {t("usage.input")}
                        </th>
                        <th className="px-3 py-2 text-right font-medium">
                          {t("usage.output")}
                        </th>
                        <th className="px-3 py-2 text-right font-medium">
                          {t("usage.total")}
                        </th>
                        <th className="py-2 pl-3 text-right font-medium">
                          {t("usage.cost")}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {data.sessions.map((session) => (
                        <SessionRow key={session.session_key} session={session} />
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
