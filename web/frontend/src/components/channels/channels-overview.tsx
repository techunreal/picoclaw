import { IconChevronRight, IconLoader2 } from "@tabler/icons-react"
import { Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import {
  type AppConfig,
  type ChannelConfig,
  getAppConfig,
  getChannelsCatalog,
} from "@/api/channels"
import { getChannelDisplayName } from "@/components/channels/channel-display-name"
import { PageHeader } from "@/components/page-header"

function channelEnabled(config: AppConfig, configKey: string): boolean {
  const channels = config.channel_list as
    | Record<string, ChannelConfig>
    | undefined
  const entry = channels?.[configKey]
  if (!entry) return false
  return entry.enabled === true
}

export function ChannelsOverview() {
  const { t } = useTranslation()

  const catalogQuery = useQuery({
    queryKey: ["channels-catalog"],
    queryFn: getChannelsCatalog,
  })
  const configQuery = useQuery({
    queryKey: ["app-config"],
    queryFn: getAppConfig,
  })

  const loading = catalogQuery.isLoading || configQuery.isLoading
  const error = catalogQuery.error ?? configQuery.error
  const channels = catalogQuery.data?.channels ?? []

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.channels_group")} />
      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 sm:px-6">
        <p className="text-muted-foreground mt-1 mb-4 text-sm">
          {t("channels.overview_description")}
        </p>

        {loading && (
          <div className="flex items-center justify-center py-16">
            <IconLoader2 className="text-muted-foreground size-5 animate-spin" />
          </div>
        )}

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-lg px-4 py-3 text-sm">
            {error instanceof Error ? error.message : String(error)}
          </div>
        )}

        <div className="flex flex-col gap-1.5">
          {channels
            .slice()
            .sort((a, b) => {
              const aKey = a.config_key || a.name
              const bKey = b.config_key || b.name
              const aOn = configQuery.data
                ? channelEnabled(configQuery.data, aKey)
                : false
              const bOn = configQuery.data
                ? channelEnabled(configQuery.data, bKey)
                : false
              if (aOn !== bOn) return aOn ? -1 : 1
              return getChannelDisplayName(a, t).localeCompare(
                getChannelDisplayName(b, t),
              )
            })
            .map((channel) => {
              const configKey = channel.config_key || channel.name
              const enabled = configQuery.data
                ? channelEnabled(configQuery.data, configKey)
                : false
              return (
                <Link
                  key={`${channel.name}-${channel.variant ?? ""}`}
                  to="/channels/$name"
                  params={{ name: channel.name }}
                  className="hover:bg-muted/40 flex items-center gap-3 rounded-lg border border-border/60 px-3 py-2 transition-colors"
                >
                  <span
                    className={`h-2 w-2 shrink-0 rounded-full ${enabled ? "bg-green-500" : "bg-muted-foreground/25"}`}
                    title={
                      enabled
                        ? t("channels.overview_enabled")
                        : t("channels.overview_disabled")
                    }
                  />
                  <span className="text-foreground min-w-0 flex-1 truncate text-sm font-medium">
                    {getChannelDisplayName(channel, t)}
                  </span>
                  <span className="text-muted-foreground font-mono text-[11px]">
                    {configKey}
                  </span>
                  <span className="text-muted-foreground hidden text-xs sm:inline">
                    {enabled
                      ? t("channels.overview_enabled")
                      : t("channels.overview_disabled")}
                  </span>
                  <IconChevronRight className="text-muted-foreground size-4" />
                </Link>
              )
            })}
        </div>
      </div>
    </div>
  )
}
