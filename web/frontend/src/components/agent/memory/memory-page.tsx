import { useTranslation } from "react-i18next"

import { PageHeader } from "@/components/page-header"
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

import { FileEditor } from "./file-editor"
import { FileList } from "./file-list"
import { agentFileLabel } from "./format"
import { HistorySheet } from "./history-sheet"
import { MemoryMasterSwitch } from "./memory-master-switch"
import { useMemoryPage } from "./use-memory-page"

export function MemoryPage() {
  const { t } = useTranslation()
  const {
    groups,
    selectedItem,
    editor,
    view,
    isLoadingList,
    listError,
    isLoadingDetail,
    detailError,
    isSaving,
    isCreating,
    isRestoring,
    historyOpen,
    history,
    isLoadingHistory,
    historyError,
    conflictOpen,
    restartWarningOpen,
    setView,
    setConflictOpen,
    setRestartWarningOpen,
    setHistoryOpen,
    handleSelectFile,
    handleDraftChange,
    handleSave,
    handleConfirmRestartWarning,
    handleRevert,
    handleReload,
    handleConflictOverwrite,
    handleOpenHistory,
    handleCreate,
    handleRestore,
  } = useMemoryPage()

  const selectedId = editor?.id ?? selectedItem?.id ?? null
  const selectedName = selectedItem ? agentFileLabel(selectedItem, t) : ""

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.memory")}>
        <MemoryMasterSwitch />
      </PageHeader>

      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-6 pb-6 md:flex-row md:overflow-hidden">
        <FileList
          groups={groups}
          selectedId={selectedId}
          isLoading={isLoadingList}
          hasError={Boolean(listError)}
          onSelect={handleSelectFile}
        />

        <FileEditor
          item={selectedItem}
          editor={editor}
          view={view}
          isLoading={isLoadingDetail}
          hasError={Boolean(detailError)}
          isSaving={isSaving}
          isCreating={isCreating}
          onViewChange={setView}
          onDraftChange={handleDraftChange}
          onSave={handleSave}
          onRevert={handleRevert}
          onReload={handleReload}
          onHistory={handleOpenHistory}
          onCreate={handleCreate}
        />
      </div>

      <HistorySheet
        open={historyOpen}
        fileName={selectedName}
        history={history}
        isLoading={isLoadingHistory}
        hasError={Boolean(historyError)}
        isRestoring={isRestoring}
        onOpenChange={setHistoryOpen}
        onRestore={handleRestore}
      />

      <AlertDialog open={conflictOpen} onOpenChange={setConflictOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("pages.agent.memory.conflict_title")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("pages.agent.memory.conflict_description")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={handleReload}>
              {t("pages.agent.memory.conflict_reload")}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConflictOverwrite}
              disabled={isSaving}
            >
              {t("pages.agent.memory.conflict_overwrite")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={restartWarningOpen}
        onOpenChange={setRestartWarningOpen}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("pages.agent.memory.restart_warning_title")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("pages.agent.memory.restart_warning_description")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>
              {t("pages.agent.memory.history_cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmRestartWarning}
              disabled={isSaving}
            >
              {t("pages.agent.memory.restart_warning_confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
