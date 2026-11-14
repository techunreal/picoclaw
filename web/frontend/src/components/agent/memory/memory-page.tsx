import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { getAgents } from "@/api/agents"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
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
import { MemoryAutoWriteSwitch } from "./memory-auto-write-switch"
import { MemoryMasterSwitch } from "./memory-master-switch"
import { useMemoryPage } from "./use-memory-page"

export function MemoryPage({ agentId }: { agentId?: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const agentsQuery = useQuery({ queryKey: ["agents"], queryFn: getAgents })
  const agentOptions = agentsQuery.data?.agents ?? []
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
  } = useMemoryPage(agentId)

  const selectedId = editor?.id ?? selectedItem?.id ?? null
  const selectedName = selectedItem ? agentFileLabel(selectedItem, t) : ""

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.memory")}>
        <div className="flex items-center gap-2">
          {agentId && (
            <Badge variant="secondary" className="font-mono">
              {t("pages.agent.memory.agent_badge", { agent: agentId })}
            </Badge>
          )}
          <label htmlFor="memory-agent" className="sr-only">
            {t("pages.agent.memory.select_agent")}
          </label>
          <select
            id="memory-agent"
            value={agentId ?? ""}
            onChange={(event) =>
              void navigate({
                to: "/agent/files",
                search: { agent: event.target.value || undefined },
              })
            }
            className="border-input bg-background h-9 rounded-md border px-3 text-sm shadow-xs"
            aria-label={t("pages.agent.memory.select_agent")}
          >
            <option value="">{t("pages.agent.memory.default_agent")}</option>
            {agentOptions.map((agent) => (
              <option key={agent.id} value={agent.id}>
                {agent.name ? `${agent.id} — ${agent.name}` : agent.id}
              </option>
            ))}
          </select>
        </div>
        <MemoryAutoWriteSwitch />
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
