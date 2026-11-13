import { useTranslation } from "react-i18next"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"

import { GithubSkillInstall } from "./github-skill-install"
import { ResultsPanel } from "./results-panel"
import { SearchPanel } from "./search-panel"
import { useSkillsDiscover } from "./use-skills-discover"

interface DiscoverPanelProps {
  onViewInstalled: (installedName: string) => void
}

export function DiscoverPanel({ onViewInstalled }: DiscoverPanelProps) {
  const { t } = useTranslation()
  const discover = useSkillsDiscover()

  return (
    <div
      className="flex-1 overflow-auto px-6 py-6"
      onScroll={discover.handleScroll}
    >
      <div className="mx-auto w-full max-w-[1000px] space-y-8">
        <section className="animate-in fade-in mx-auto flex w-full flex-col items-center space-y-8 duration-300 md:duration-500">
          <SearchPanel
            marketQuery={discover.marketQuery}
            marketSort={discover.marketSort}
            canSearchMarketplace={discover.canSearchMarketplace}
            isMarketSearchInitialLoading={discover.isMarketSearchInitialLoading}
            unavailableToolMessages={discover.unavailableToolMessages}
            onMarketQueryChange={discover.setMarketQuery}
            onSearchSubmit={discover.handleSearchSubmit}
            onMarketSortChange={discover.handleSortChange}
          />

          <GithubSkillInstall canInstall={discover.canInstallFromMarketplace} />

          <ResultsPanel
            canSearchMarketplace={discover.canSearchMarketplace}
            hasSubmittedQuery={discover.hasSubmittedQuery}
            submittedQuery={discover.submittedMarketQuery}
            marketResults={discover.marketResults}
            marketSearchError={discover.marketSearchError}
            isMarketSearchInitialLoading={discover.isMarketSearchInitialLoading}
            isMarketSearchLoadingMore={discover.isMarketSearchLoadingMore}
            popularResults={discover.popularResults}
            popularError={discover.popularError}
            isPopularInitialLoading={discover.isPopularInitialLoading}
            isPopularLoadingMore={discover.isPopularLoadingMore}
            canInstallFromMarketplace={discover.canInstallFromMarketplace}
            getInstalledSkill={discover.getInstalledSkill}
            isInstallPending={discover.isInstallPending}
            onInstall={discover.handleInstall}
            onViewInstalled={onViewInstalled}
          />
        </section>
      </div>

      <AlertDialog
        open={discover.ambiguousInstall !== null}
        onOpenChange={(open) => {
          if (!open) {
            discover.clearAmbiguousInstall()
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("pages.agent.skills.publisher_required_title")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("pages.agent.skills.publisher_required_description", {
                slug: discover.ambiguousInstall?.slug ?? "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="grid gap-2">
            {discover.ambiguousInstall?.matches.map((match) => (
              <Button
                key={match.owner_handle}
                variant="outline"
                className="justify-start"
                onClick={() =>
                  discover.handleInstallWithOwner(match.owner_handle)
                }
              >
                @{match.owner_handle}
                {match.ref ? (
                  <span className="text-muted-foreground ml-auto font-mono text-xs">
                    {match.ref}
                  </span>
                ) : null}
              </Button>
            ))}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>
              {t("pages.agent.skills.publisher_required_cancel")}
            </AlertDialogCancel>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog
        open={discover.suspiciousInstall !== null}
        onOpenChange={(open) => {
          if (!open) {
            discover.clearSuspiciousInstall()
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("pages.agent.skills.suspicious.title")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("pages.agent.skills.suspicious.description")}
              {discover.suspiciousInstall?.summary ? (
                <span className="mt-2 block font-mono text-xs">
                  {discover.suspiciousInstall.summary}
                </span>
              ) : null}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>
              {t("pages.agent.skills.suspicious.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={discover.handleConfirmSuspicious}
            >
              {t("pages.agent.skills.suspicious.confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
