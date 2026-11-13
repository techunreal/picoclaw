import { ResultsPanel } from "./results-panel"
import { SearchPanel } from "./search-panel"
import { useSkillsDiscover } from "./use-skills-discover"

interface DiscoverPanelProps {
  onViewInstalled: (installedName: string) => void
}

export function DiscoverPanel({ onViewInstalled }: DiscoverPanelProps) {
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
            canSearchMarketplace={discover.canSearchMarketplace}
            isMarketSearchInitialLoading={discover.isMarketSearchInitialLoading}
            unavailableToolMessages={discover.unavailableToolMessages}
            onMarketQueryChange={discover.setMarketQuery}
            onSearchSubmit={discover.handleSearchSubmit}
          />

          <ResultsPanel
            canSearchMarketplace={discover.canSearchMarketplace}
            hasSubmittedQuery={discover.hasSubmittedQuery}
            submittedQuery={discover.submittedMarketQuery}
            marketResults={discover.marketResults}
            marketSearchError={discover.marketSearchError}
            isMarketSearchInitialLoading={discover.isMarketSearchInitialLoading}
            isMarketSearchLoadingMore={discover.isMarketSearchLoadingMore}
            canInstallFromMarketplace={discover.canInstallFromMarketplace}
            getInstalledSkill={discover.getInstalledSkill}
            isInstallPending={discover.isInstallPending}
            onInstall={discover.handleInstall}
            onViewInstalled={onViewInstalled}
          />
        </section>
      </div>
    </div>
  )
}
