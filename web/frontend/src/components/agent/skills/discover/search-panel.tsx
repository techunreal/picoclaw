import { IconLoader2 } from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import type { SkillSearchSort } from "@/api/skills"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import type { UnavailableToolMessage } from "./tool-support"

export function SearchPanel({
  marketQuery,
  marketSort,
  canSearchMarketplace,
  isMarketSearchInitialLoading,
  unavailableToolMessages,
  onMarketQueryChange,
  onSearchSubmit,
  onMarketSortChange,
}: {
  marketQuery: string
  marketSort: SkillSearchSort
  canSearchMarketplace: boolean
  isMarketSearchInitialLoading: boolean
  unavailableToolMessages: UnavailableToolMessage[]
  onMarketQueryChange: (value: string) => void
  onSearchSubmit: () => void
  onMarketSortChange: (value: SkillSearchSort) => void
}) {
  const { t } = useTranslation()

  return (
    <div className="flex flex-col items-center justify-center space-y-6 py-8 text-center sm:py-12">
      <div className="space-y-2">
        <h2 className="text-2xl font-bold tracking-tight md:text-3xl">
          {t("pages.agent.skills.marketplace_title", {
            defaultValue: "Discover Skills",
          })}
        </h2>
        <p className="text-muted-foreground max-w-[600px] text-base md:text-lg">
          {t("pages.agent.skills.marketplace_description")}
        </p>
      </div>

      <form
        className="w-full max-w-2xl px-4 md:px-0"
        onSubmit={(event) => {
          event.preventDefault()
          onSearchSubmit()
        }}
      >
        <div className="group relative flex items-center justify-center">
          <Input
            value={marketQuery}
            onChange={(event) => onMarketQueryChange(event.target.value)}
            placeholder={t("pages.agent.skills.marketplace_search_placeholder")}
            className="border-border/60 bg-background/50 hover:bg-background focus-visible:ring-primary/20 h-12 w-full rounded-full pr-20 pl-5 text-sm shadow-sm backdrop-blur-sm transition-all focus-visible:ring-2 md:min-w-[520px]"
            disabled={!canSearchMarketplace}
          />
          <Button
            type="submit"
            className="absolute top-1/2 right-1.5 h-9 -translate-y-1/2 rounded-full px-4 font-medium shadow-sm transition-all"
            disabled={
              !canSearchMarketplace ||
              isMarketSearchInitialLoading ||
              marketQuery.trim() === ""
            }
          >
            {isMarketSearchInitialLoading ? (
              <IconLoader2 className="size-4 animate-spin" />
            ) : (
              <span>
                {t("pages.agent.skills.marketplace_search_action", {
                  defaultValue: "Search",
                })}
              </span>
            )}
          </Button>
        </div>
      </form>

      <div className="flex items-center gap-2">
        <span className="text-muted-foreground text-xs font-medium">
          {t("pages.agent.skills.market_sort_label")}
        </span>
        <Select
          value={marketSort}
          onValueChange={(value) =>
            onMarketSortChange(value as SkillSearchSort)
          }
        >
          <SelectTrigger className="h-8 w-[180px] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent align="center">
            <SelectItem value="relevance">
              {t("pages.agent.skills.market_sort.relevance")}
            </SelectItem>
            <SelectItem value="downloads">
              {t("pages.agent.skills.market_sort.downloads")}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      {unavailableToolMessages.length ? (
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-3 pt-2">
          {unavailableToolMessages.map((item) => (
            <div
              key={item.key}
              className="rounded-xl border border-amber-200/80 bg-amber-50/70 px-4 py-3 text-left text-sm text-amber-900"
            >
              <div className="font-semibold">{item.label}</div>
              <div className="mt-1 leading-6">{item.message}</div>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}
