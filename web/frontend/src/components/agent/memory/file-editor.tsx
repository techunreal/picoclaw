import {
  IconAlertTriangle,
  IconArrowBackUp,
  IconDeviceFloppy,
  IconEye,
  IconFileText,
  IconHistory,
  IconLoader2,
  IconPencil,
  IconPlus,
  IconRefresh,
} from "@tabler/icons-react"
import { useTranslation } from "react-i18next"
import ReactMarkdown from "react-markdown"
import rehypeSanitize from "rehype-sanitize"
import remarkGfm from "remark-gfm"

import type { AgentFileItem } from "@/api/agent-files"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

import {
  LARGE_FILE_BYTES,
  agentFileLabel,
  estimateTokens,
  formatBytes,
  frontmatterChanged,
  utf8Size,
} from "./format"
import type { MemoryEditorState, MemoryView } from "./use-memory-page"

interface FileEditorProps {
  item: AgentFileItem | null
  editor: MemoryEditorState | null
  view: MemoryView
  isLoading: boolean
  hasError: boolean
  isSaving: boolean
  isCreating: boolean
  onViewChange: (view: MemoryView) => void
  onDraftChange: (draft: string) => void
  onSave: () => void
  onRevert: () => void
  onReload: () => void
  onHistory: () => void
  onCreate: () => void
}

export function FileEditor({
  item,
  editor,
  view,
  isLoading,
  hasError,
  isSaving,
  isCreating,
  onViewChange,
  onDraftChange,
  onSave,
  onRevert,
  onReload,
  onHistory,
  onCreate,
}: FileEditorProps) {
  const { t } = useTranslation()

  if (!item) {
    return (
      <section className="text-muted-foreground flex min-h-0 flex-1 items-center justify-center p-8 text-sm">
        {t("pages.agent.memory.select_file")}
      </section>
    )
  }

  const label = agentFileLabel(item, t)
  const readOnly = editor ? !editor.editable : !item.editable
  const dirty = editor !== null && editor.draft !== editor.content
  const content = editor?.draft ?? ""
  const bytes = editor ? utf8Size(content) : item.size
  const tokens = estimateTokens(content)
  const isLarge = bytes > LARGE_FILE_BYTES
  const restartWarning =
    editor !== null &&
    editor.id === "agent" &&
    dirty &&
    frontmatterChanged(editor.content, editor.draft)

  return (
    <section className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-wrap items-start justify-between gap-3 pb-3">
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <IconFileText className="text-muted-foreground size-4 shrink-0" />
            <h3 className="truncate text-base font-medium">{label}</h3>
            {item.legacy && (
              <Badge variant="secondary">
                {t("pages.agent.memory.legacy")}
              </Badge>
            )}
            {readOnly && (
              <Badge variant="outline">
                {t("pages.agent.memory.read_only")}
              </Badge>
            )}
          </div>
          <div className="text-muted-foreground mt-1 truncate font-mono text-xs">
            {item.path}
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={onHistory}
            disabled={!editor || isLoading}
          >
            <IconHistory className="size-4" />
            {t("pages.agent.memory.history")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={onReload}
            disabled={!editor || isLoading}
          >
            <IconRefresh className="size-4" />
            {t("pages.agent.memory.reload")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={onRevert}
            disabled={!dirty}
          >
            <IconArrowBackUp className="size-4" />
            {t("pages.agent.memory.revert")}
          </Button>
          <Button
            size="sm"
            onClick={onSave}
            disabled={!editor || !dirty || readOnly || isSaving || isCreating}
          >
            {isSaving ? (
              <IconLoader2 className="size-4 animate-spin" />
            ) : (
              <IconDeviceFloppy className="size-4" />
            )}
            {t("pages.agent.memory.save")}
          </Button>
        </div>
      </div>

      {readOnly && (
        <div className="border-border/50 bg-muted/30 text-muted-foreground mb-3 rounded-md border px-3 py-2 text-xs">
          {t("pages.agent.memory.read_only_hint")}
        </div>
      )}

      {restartWarning && (
        <div className="mb-3 flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
          <span>{t("pages.agent.memory.restart_warning_inline")}</span>
        </div>
      )}

      {isLarge && (
        <div className="text-muted-foreground mb-3 flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
          <span>{t("pages.agent.memory.large_file_warning")}</span>
        </div>
      )}

      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div className="border-border/50 bg-muted/30 flex items-center rounded-md border p-0.5">
          <button
            type="button"
            onClick={() => onViewChange("edit")}
            className={cn(
              "flex items-center gap-1.5 rounded px-2.5 py-1 text-xs transition-colors",
              view === "edit"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            <IconPencil className="size-3.5" />
            {t("pages.agent.memory.edit")}
          </button>
          <button
            type="button"
            onClick={() => onViewChange("preview")}
            className={cn(
              "flex items-center gap-1.5 rounded px-2.5 py-1 text-xs transition-colors",
              view === "preview"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            <IconEye className="size-3.5" />
            {t("pages.agent.memory.preview")}
          </button>
        </div>

        <div className="text-muted-foreground flex items-center gap-3 text-xs tabular-nums">
          <span>
            {t("pages.agent.memory.size_label", {
              size: formatBytes(bytes),
              tokens: tokens.toLocaleString(),
            })}
          </span>
          <span className={dirty ? "text-amber-600" : undefined}>
            {dirty
              ? t("pages.agent.memory.unsaved")
              : t("pages.agent.memory.saved")}
          </span>
        </div>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-4 w-2/3" />
          <Skeleton className="h-4 w-1/2" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : hasError ? (
        <div className="text-destructive py-6 text-sm">
          {t("pages.agent.memory.detail_load_error")}
        </div>
      ) : editor && !editor.exists ? (
        <div className="border-border/50 flex min-h-[360px] flex-1 flex-col items-center justify-center gap-4 rounded-lg border border-dashed p-10 text-center">
          <IconFileText className="text-muted-foreground/40 size-10" />
          <div>
            <div className="font-medium">
              {t("pages.agent.memory.missing_title")}
            </div>
            <p className="text-muted-foreground mt-1 text-sm">
              {t("pages.agent.memory.missing_description")}
            </p>
          </div>
          <Button onClick={onCreate} disabled={isCreating || !item.creatable}>
            {isCreating ? (
              <IconLoader2 className="size-4 animate-spin" />
            ) : (
              <IconPlus className="size-4" />
            )}
            {t("pages.agent.memory.create")}
          </Button>
        </div>
      ) : (
        <div className="border-border/50 bg-background flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border">
          {view === "edit" ? (
            <Textarea
              value={content}
              onChange={(event) => onDraftChange(event.target.value)}
              readOnly={readOnly}
              spellCheck={false}
              className="h-full min-h-[360px] w-full flex-1 resize-none rounded-none border-0 bg-transparent p-4 font-mono text-[13px] leading-relaxed shadow-none focus-visible:ring-0"
            />
          ) : (
            <div className="min-h-[360px] flex-1 overflow-y-auto">
              <div className="prose dark:prose-invert max-w-none p-5 text-sm">
                {content.trim() === "" ? (
                  <p className="text-muted-foreground">
                    {t("pages.agent.memory.preview_empty")}
                  </p>
                ) : (
                  <ReactMarkdown
                    remarkPlugins={[remarkGfm]}
                    rehypePlugins={[rehypeSanitize]}
                  >
                    {content}
                  </ReactMarkdown>
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </section>
  )
}
