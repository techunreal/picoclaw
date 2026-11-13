import { IconLoader2, IconPlus } from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import { DeleteDialog } from "./delete-dialog"
import { DetailSheet } from "./detail-sheet"
import { DiscoverPanel } from "./discover/discover-panel"
import { FilterBar } from "./filter-bar"
import { ImportDialog } from "./import-dialog"
import { PageSkeleton } from "./page-skeleton"
import { SkillsList } from "./skills-list"
import { Stats } from "./stats"
import type { SkillPageTab } from "./types"
import { useSkillsPage } from "./use-skills-page"

interface SkillsPageProps {
  tab: SkillPageTab
  onTabChange: (tab: SkillPageTab) => void
}

const TABS: SkillPageTab[] = ["installed", "discover"]

export function SkillsPage({ tab, onTabChange }: SkillsPageProps) {
  const { t } = useTranslation()
  const {
    searchQuery,
    sourceFilter,
    sortOrder,
    layoutMode,
    detailView,
    isDragActive,
    isImportDialogOpen,
    selectedSkill,
    skillPendingDelete,
    availableOrigins,
    groupedSkills,
    stats,
    sortedSkills,
    hasActiveFilters,
    importInputRef,
    selectedSkillDetail,
    skillsError,
    skillDetailError,
    isLoading,
    isSkillDetailLoading,
    isImportPending,
    isDeletePending,
    setSearchQuery,
    setSourceFilter,
    setSortOrder,
    setLayoutMode,
    setDetailView,
    openImportDialog,
    handleViewSkill,
    handleViewSkillById,
    handleRequestDelete,
    handleConfirmDelete,
    handleImportClick,
    handleImportFileChange,
    handleDropZoneDragEnter,
    handleDropZoneDragLeave,
    handleDropZoneDrop,
    handleDetailSheetOpenChange,
    handleImportDialogOpenChange,
    handleDeleteDialogOpenChange,
  } = useSkillsPage()

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        title={t("navigation.skills")}
        children={
          tab === "installed" ? (
            <>
              <input
                ref={importInputRef}
                type="file"
                accept=".md,.zip,text/markdown,text/plain,application/zip,application/x-zip-compressed"
                className="hidden"
                onChange={handleImportFileChange}
              />
              <Button
                variant="outline"
                onClick={openImportDialog}
                disabled={isImportPending}
              >
                {isImportPending ? (
                  <IconLoader2 className="size-4 animate-spin" />
                ) : (
                  <IconPlus className="size-4" />
                )}
                {t("pages.agent.skills.import")}
              </Button>
            </>
          ) : null
        }
      />

      <div className="border-border/40 flex shrink-0 items-center gap-1 border-b px-6">
        {TABS.map((value) => (
          <button
            key={value}
            type="button"
            onClick={() => onTabChange(value)}
            className={cn(
              "relative px-3 py-2.5 text-sm font-medium transition-colors",
              tab === value
                ? "text-foreground after:bg-primary after:absolute after:inset-x-0 after:-bottom-px after:h-0.5"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {t(`pages.agent.skills.tabs.${value}`)}
          </button>
        ))}
      </div>

      {tab === "discover" ? (
        <DiscoverPanel onViewInstalled={handleViewSkillById} />
      ) : (
        <div className="flex-1 overflow-auto px-6 py-6">
          <div className="w-full max-w-6xl space-y-8">
            {isLoading ? (
              <PageSkeleton />
            ) : skillsError ? (
              <div className="text-destructive py-6 text-sm">
                {t("pages.agent.load_error")}
              </div>
            ) : (
              <section className="animate-in fade-in space-y-3 duration-300 md:duration-500">
                <Stats stats={stats} />

                <div className="flex flex-col gap-4 py-3">
                  <FilterBar
                    searchQuery={searchQuery}
                    sourceFilter={sourceFilter}
                    availableOrigins={availableOrigins}
                    sortOrder={sortOrder}
                    layoutMode={layoutMode}
                    onSearchQueryChange={setSearchQuery}
                    onSourceFilterChange={setSourceFilter}
                    onSortOrderChange={setSortOrder}
                    onLayoutModeChange={setLayoutMode}
                  />
                </div>

                <SkillsList
                  sortedSkills={sortedSkills}
                  groupedSkills={groupedSkills}
                  layoutMode={layoutMode}
                  sourceFilter={sourceFilter}
                  hasActiveFilters={hasActiveFilters}
                  onViewSkill={handleViewSkill}
                  onDeleteSkill={handleRequestDelete}
                />
              </section>
            )}
          </div>
        </div>
      )}

      <DetailSheet
        open={selectedSkill !== null}
        selectedSkill={selectedSkill}
        selectedSkillDetail={selectedSkillDetail}
        isLoading={isSkillDetailLoading}
        error={skillDetailError}
        detailView={detailView}
        onDetailViewChange={setDetailView}
        onOpenChange={handleDetailSheetOpenChange}
      />

      <ImportDialog
        open={isImportDialogOpen}
        isImportPending={isImportPending}
        isDragActive={isDragActive}
        onOpenChange={handleImportDialogOpenChange}
        onImportClick={handleImportClick}
        onDragEnter={handleDropZoneDragEnter}
        onDragLeave={handleDropZoneDragLeave}
        onDrop={handleDropZoneDrop}
      />

      <DeleteDialog
        open={skillPendingDelete !== null}
        skillPendingDelete={skillPendingDelete}
        isDeletePending={isDeletePending}
        onOpenChange={handleDeleteDialogOpenChange}
        onConfirm={handleConfirmDelete}
      />
    </div>
  )
}
