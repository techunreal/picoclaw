import {
  IconAlertTriangle,
  IconArrowBackUp,
  IconDeviceFloppy,
  IconFileCode,
  IconLoader2,
  IconSparkles,
  IconWorld,
  IconX,
} from "@tabler/icons-react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { type ReactNode, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import ReactMarkdown from "react-markdown"
import rehypeHighlight from "rehype-highlight"
import rehypeRaw from "rehype-raw"
import rehypeSanitize from "rehype-sanitize"
import remarkGfm from "remark-gfm"
import { toast } from "sonner"

import {
  SkillApiError,
  type SkillDetailResponse,
  type SkillSupportItem,
  saveSkillContent,
} from "@/api/skills"
import {
  MarkdownCodeBlock,
  MessageCodeBlock,
} from "@/components/chat/message-code-block"
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
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

import { OriginBadge } from "./origin-badge"
import { getOriginLabel, getSkillOriginKind } from "./origin-utils"
import type { SkillDetailView } from "./types"

const DETAIL_VIEWS = [
  "preview",
  "raw",
  "meta",
] as const satisfies SkillDetailView[]

interface DetailSheetProps {
  open: boolean
  selectedSkill: SkillSupportItem | null
  selectedSkillDetail?: SkillDetailResponse
  isLoading: boolean
  error: unknown
  detailView: SkillDetailView
  onDetailViewChange: (view: SkillDetailView) => void
  onOpenChange: (open: boolean) => void
}

export function DetailSheet({
  open,
  selectedSkill,
  selectedSkillDetail,
  isLoading,
  error,
  detailView,
  onDetailViewChange,
  onOpenChange,
}: DetailSheetProps) {
  const { t } = useTranslation()

  const activeSkillDetail = selectedSkillDetail ?? selectedSkill
  const activeSkillOrigin = activeSkillDetail
    ? getSkillOriginKind(activeSkillDetail)
    : null
  const detailLineCount = selectedSkillDetail
    ? selectedSkillDetail.content.split("\n").length
    : 0
  const detailCharacterCount = selectedSkillDetail?.content.length ?? 0

  const queryClient = useQueryClient()
  const [draft, setDraft] = useState("")
  const [conflictOpen, setConflictOpen] = useState(false)

  const editable = Boolean(selectedSkillDetail?.editable)
  const detailViews: SkillDetailView[] = editable
    ? [...DETAIL_VIEWS, "edit"]
    : [...DETAIL_VIEWS]
  const dirty =
    selectedSkillDetail !== undefined &&
    draft !== selectedSkillDetail.raw_content

  useEffect(() => {
    if (detailView === "edit") {
      setDraft(selectedSkillDetail?.raw_content ?? "")
    }
  }, [
    detailView,
    selectedSkillDetail?.id,
    selectedSkillDetail?.revision,
    selectedSkillDetail?.raw_content,
  ])

  const saveMutation = useMutation({
    mutationFn: (input: {
      content: string
      revision: string
      force?: boolean
    }) => {
      if (!selectedSkillDetail) {
        return Promise.reject(new Error("no skill selected"))
      }
      return saveSkillContent(selectedSkillDetail.id, input)
    },
    onSuccess: () => {
      toast.success(t("pages.agent.skills.editor.save_success"))
      setConflictOpen(false)
      onDetailViewChange("preview")
      void queryClient.invalidateQueries({ queryKey: ["skills"] })
    },
    onError: (err) => {
      if (err instanceof SkillApiError && err.status === 409) {
        setConflictOpen(true)
        return
      }
      toast.error(
        err instanceof Error
          ? err.message
          : t("pages.agent.skills.editor.save_error"),
      )
    },
  })

  const handleSave = (force = false) => {
    if (!selectedSkillDetail) {
      return
    }
    saveMutation.mutate({
      content: draft,
      revision: selectedSkillDetail.revision,
      force: force || undefined,
    })
  }

  const handleConflictReload = () => {
    setConflictOpen(false)
    if (selectedSkillDetail) {
      void queryClient.invalidateQueries({
        queryKey: ["skills", "detail", selectedSkillDetail.id],
      })
    }
  }

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          side="right"
          className="flex w-full flex-col gap-0 p-0 shadow-2xl data-[side=right]:!w-full data-[side=right]:sm:!w-[720px] data-[side=right]:sm:!max-w-[720px]"
        >
          <SheetHeader className="bg-muted/10 border-b px-6 py-6">
            <div className="flex items-center gap-3">
              <div className="bg-primary/10 string-1 ring-primary/20 text-primary flex size-10 items-center justify-center rounded-xl">
                {activeSkillDetail?.origin_kind === "builtin" ? (
                  <IconSparkles className="size-5" />
                ) : activeSkillDetail?.registry_name ? (
                  <IconWorld className="size-5" />
                ) : (
                  <IconFileCode className="size-5" />
                )}
              </div>
              <div className="min-w-0 flex-1 space-y-1 text-left">
                <SheetTitle className="truncate text-xl font-bold tracking-tight">
                  {activeSkillDetail?.name ||
                    t("pages.agent.skills.viewer_title")}
                </SheetTitle>
                <SheetDescription className="line-clamp-2">
                  {activeSkillDetail?.description ||
                    t("pages.agent.skills.viewer_description")}
                </SheetDescription>
              </div>
            </div>
          </SheetHeader>

          <div className="flex-1 overflow-x-hidden overflow-y-scroll px-6 py-6">
            {isLoading ? (
              <div className="space-y-6">
                <Skeleton className="h-6 w-48" />
                <Skeleton className="h-24 w-full rounded-xl" />
                <Skeleton className="h-[400px] w-full rounded-xl" />
              </div>
            ) : error ? (
              <div className="text-destructive border-destructive/20 bg-destructive/5 flex h-40 flex-col items-center justify-center gap-3 rounded-xl border">
                <IconX className="size-6 opacity-80" />
                <span className="text-sm font-medium">
                  {t("pages.agent.skills.load_detail_error")}
                </span>
              </div>
            ) : selectedSkillDetail ? (
              <div className="space-y-6">
                {activeSkillOrigin === "third_party" ? (
                  <div className="border-border/40 bg-card/40 space-y-4 rounded-xl border p-4 shadow-sm">
                    <div className="flex flex-wrap items-center gap-2 px-1">
                      <OriginBadge
                        origin={activeSkillOrigin}
                        label={getOriginLabel(activeSkillOrigin, t)}
                      />
                    </div>

                    <div className="grid gap-3 sm:grid-cols-2">
                      {selectedSkillDetail.registry_name ? (
                        <MetadataItem
                          label={t("pages.agent.skills.metadata.registry")}
                          value={selectedSkillDetail.registry_name}
                        />
                      ) : null}
                      {selectedSkillDetail.installed_version ? (
                        <MetadataItem
                          label={t("pages.agent.skills.metadata.version")}
                          value={selectedSkillDetail.installed_version}
                        />
                      ) : null}
                      {selectedSkillDetail.registry_url ? (
                        <MetadataItem
                          label={t("pages.agent.skills.metadata.url")}
                          value={
                            <a
                              href={selectedSkillDetail.registry_url}
                              target="_blank"
                              rel="noreferrer"
                              className="text-primary hover:text-primary/80 inline break-all underline-offset-4 hover:underline"
                            >
                              {selectedSkillDetail.registry_url}
                            </a>
                          }
                          mono
                        />
                      ) : null}
                    </div>
                  </div>
                ) : null}

                {selectedSkillDetail.name_mismatch ? (
                  <div className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                    <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
                    <span>
                      {t("pages.agent.skills.editor.name_mismatch", {
                        name: selectedSkillDetail.name,
                        directory: selectedSkillDetail.directory,
                      })}
                    </span>
                  </div>
                ) : null}

                <div className="border-border/70 bg-muted/20 inline-flex rounded-lg border p-1 shadow-sm">
                  {detailViews.map((view) => (
                    <button
                      key={view}
                      type="button"
                      className={cn(
                        "rounded-md px-4 py-1.5 text-xs font-medium transition-all duration-200",
                        detailView === view
                          ? "bg-background text-foreground ring-border/30 shadow-[0_1px_3px_rgba(0,0,0,0.1)] ring-1"
                          : "text-muted-foreground hover:text-foreground hover:bg-muted/50",
                      )}
                      onClick={() => onDetailViewChange(view)}
                    >
                      {t(`pages.agent.skills.detail_tabs.${view}`)}
                    </button>
                  ))}
                </div>

                {detailView === "preview" ? (
                  <div className="prose prose-zinc dark:prose-invert prose-sm sm:prose-base prose-pre:rounded-xl prose-pre:border prose-pre:border-border/40 prose-pre:bg-zinc-100 prose-pre:p-0 prose-pre:shadow-sm dark:prose-pre:bg-zinc-950/90 prose-headings:tracking-tight prose-a:text-primary prose-a:no-underline hover:prose-a:underline max-w-none">
                    <ReactMarkdown
                      remarkPlugins={[remarkGfm]}
                      rehypePlugins={[
                        rehypeRaw,
                        rehypeSanitize,
                        rehypeHighlight,
                      ]}
                      components={{
                        pre: MarkdownCodeBlock,
                      }}
                    >
                      {selectedSkillDetail.content}
                    </ReactMarkdown>
                  </div>
                ) : null}

                {detailView === "raw" ? (
                  <MessageCodeBlock
                    code={selectedSkillDetail.content}
                    label={t("pages.agent.skills.detail_tabs.raw")}
                    className="my-0"
                    bodyClassName="text-[13px] leading-relaxed"
                  />
                ) : null}

                {detailView === "meta" ? (
                  <div className="grid gap-4 sm:grid-cols-2">
                    <MetadataItem
                      label={t("pages.agent.skills.metadata.name")}
                      value={selectedSkillDetail.name}
                    />
                    <MetadataItem
                      label={t("pages.agent.skills.metadata.directory")}
                      value={selectedSkillDetail.directory}
                      mono
                    />
                    <MetadataItem
                      label={t("pages.agent.skills.metadata.description")}
                      value={
                        selectedSkillDetail.description ||
                        t("pages.agent.skills.no_description")
                      }
                    />
                    <MetadataItem
                      label={t("pages.agent.skills.metadata.lines")}
                      value={String(detailLineCount)}
                    />
                    <MetadataItem
                      label={t("pages.agent.skills.metadata.characters")}
                      value={String(detailCharacterCount)}
                    />
                  </div>
                ) : null}

                {detailView === "edit" && selectedSkillDetail ? (
                  <div className="space-y-3">
                    {selectedSkillDetail.name_mismatch ? (
                      <div className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
                        <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
                        <span>
                          {t("pages.agent.skills.editor.name_mismatch", {
                            name: selectedSkillDetail.name,
                            directory: selectedSkillDetail.directory,
                          })}
                        </span>
                      </div>
                    ) : null}

                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="text-muted-foreground text-xs tabular-nums">
                        {selectedSkillDetail.size} B{" · "}
                        {dirty
                          ? t("pages.agent.skills.editor.unsaved")
                          : t("pages.agent.skills.editor.saved")}
                      </span>
                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            setDraft(selectedSkillDetail.raw_content)
                          }
                          disabled={!dirty || saveMutation.isPending}
                        >
                          <IconArrowBackUp className="size-4" />
                          {t("pages.agent.skills.editor.revert")}
                        </Button>
                        <Button
                          size="sm"
                          onClick={() => handleSave()}
                          disabled={!dirty || saveMutation.isPending}
                        >
                          {saveMutation.isPending ? (
                            <IconLoader2 className="size-4 animate-spin" />
                          ) : (
                            <IconDeviceFloppy className="size-4" />
                          )}
                          {t("pages.agent.skills.editor.save")}
                        </Button>
                      </div>
                    </div>

                    <Textarea
                      value={draft}
                      onChange={(event) => setDraft(event.target.value)}
                      spellCheck={false}
                      className="min-h-[420px] w-full resize-y font-mono text-[13px] leading-relaxed"
                    />
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
        </SheetContent>
      </Sheet>

      <AlertDialog open={conflictOpen} onOpenChange={setConflictOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("pages.agent.skills.editor.conflict_title")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("pages.agent.skills.editor.conflict_description")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={handleConflictReload}>
              {t("pages.agent.skills.editor.conflict_reload")}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => handleSave(true)}
              disabled={saveMutation.isPending}
            >
              {t("pages.agent.skills.editor.conflict_overwrite")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function MetadataItem({
  label,
  value,
  mono = false,
}: {
  label: string
  value: ReactNode
  mono?: boolean
}) {
  return (
    <div className="border-border/70 bg-muted/20 rounded-xl border px-4 py-3">
      <div className="text-muted-foreground text-[11px] font-semibold tracking-[0.18em] uppercase">
        {label}
      </div>
      <div
        className={cn(
          "text-foreground mt-2 text-sm leading-6 break-all",
          mono && "font-mono text-xs",
        )}
      >
        {value}
      </div>
    </div>
  )
}
