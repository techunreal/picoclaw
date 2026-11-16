import {
  IconAlertTriangle,
  IconArrowDown,
  IconArrowUp,
  IconPlus,
  IconRefresh,
  IconRoute,
  IconTrash,
  IconUsers,
} from "@tabler/icons-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Link } from "@tanstack/react-router"

import type { AgentView, DispatchRule } from "@/api/agents"
import { AgentAvatar } from "@/components/agent-avatar"
import { AgentEditDialog } from "@/components/agent/agents/agent-edit-dialog"
import { RoutingRuleDialog } from "@/components/agent/agents/routing-rule-dialog"
import {
  agentErrorMessage,
  useAgentsPage,
} from "@/components/agent/agents/use-agents-page"
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
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

type PageTab = "overview" | "routing"

function basename(path: string): string {
  const parts = path.split(/[\\/]/).filter(Boolean)
  return parts[parts.length - 1] || path
}

function describeRule(rule: DispatchRule): string {
  const parts: string[] = []
  if (rule.when.channel) parts.push(`channel: ${rule.when.channel}`)
  if (rule.when.account) parts.push(`account: ${rule.when.account}`)
  if (rule.when.chat) parts.push(`chat: ${rule.when.chat}`)
  if (rule.when.sender) parts.push(`sender: ${rule.when.sender}`)
  if (rule.when.mentioned === true) parts.push("mentioned: yes")
  if (rule.when.mentioned === false) parts.push("mentioned: no")
  return parts.join(" · ")
}

export function AgentsPage() {
  const { t } = useTranslation()
  const {
    listQuery,
    agents,
    dispatch,
    selectedAgent,
    setSelectedId,
    editor,
    setEditor,
    openCreate,
    openEdit,
    saveAgent,
    ruleEditor,
    setRuleEditor,
    openCreateRule,
    openEditRule,
    saveRule,
    deleteRule,
    moveRule,
    deleteTarget,
    setDeleteTarget,
    deleteAgent,
    isSaving,
    saveError,
  } = useAgentsPage()

  const [tab, setTab] = useState<PageTab>("overview")
  const defaults = listQuery.data?.defaults
  const implicit = listQuery.data?.implicit ?? false
  const listError = listQuery.error
    ? agentErrorMessage(listQuery.error, t("agents.loadFailed"))
    : null
  const formError = saveError
    ? agentErrorMessage(saveError, t("agents.saveFailed"))
    : null

  const renderAgentCard = (agent: AgentView) => {
    const selected = selectedAgent?.id === agent.id
    return (
      <button
        key={agent.id}
        type="button"
        onClick={() => setSelectedId(agent.id)}
        className={`flex w-full flex-col gap-1 rounded-xl border p-3 text-left transition-colors ${
          selected
            ? "border-primary/40 bg-primary/5"
            : "border-border/60 hover:bg-muted/40"
        }`}
      >
        <span className="flex items-center gap-2">
          <AgentAvatar
            agentId={agent.id}
            avatar={agent.avatar}
            name={agent.name || agent.id}
            size="md"
          />
          <span className="font-mono text-sm font-medium">{agent.id}</span>
          {agent.default && <Badge variant="secondary">{t("agents.defaultBadge")}</Badge>}
          {agent.subagents?.enabled === false && (
            <Badge variant="outline">{t("agents.subagentsOffBadge")}</Badge>
          )}
        </span>
        <span className="text-muted-foreground truncate text-xs">
          {agent.name || t("agents.noName")}
        </span>
        <span className="text-muted-foreground truncate text-xs">
          {agent.effective_model || t("agents.useDefaultModelShort")}
        </span>
      </button>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("agents.title")}>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="text-muted-foreground hover:text-foreground h-8 w-8 rounded-full"
            onClick={() => void listQuery.refetch()}
            disabled={listQuery.isFetching}
            aria-label={t("agents.refresh")}
            title={t("agents.refresh")}
          >
            <IconRefresh className="size-4" />
          </Button>
          <Button type="button" size="sm" className="gap-1.5" onClick={openCreate}>
            <IconPlus className="size-4" />
            {t("agents.add")}
          </Button>
        </div>
      </PageHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-8 md:px-8">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-4">
          <p className="text-muted-foreground text-sm">{t("agents.description")}</p>

          {listError && (
            <div className="border-destructive/30 bg-destructive/5 text-destructive flex items-start gap-2 rounded-xl border p-4 text-sm">
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{listError}</span>
            </div>
          )}

          {listQuery.data?.warnings?.map((warning) => (
            <div
              key={warning}
              className="flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/20 dark:text-amber-300"
            >
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
              <span>{warning}</span>
            </div>
          ))}

          {listQuery.isLoading && (
            <div className="text-muted-foreground py-16 text-center text-sm">
              {t("agents.loading")}
            </div>
          )}

          {!listQuery.isLoading && !listError && implicit && (
            <div className="border-border/60 flex flex-col items-start gap-3 rounded-2xl border border-dashed p-8">
              <IconUsers className="text-muted-foreground size-9 opacity-60" />
              <div>
                <p className="font-medium">{t("agents.implicitTitle")}</p>
                <p className="text-muted-foreground mt-1 text-sm">
                  {t("agents.implicitDescription")}
                </p>
                <p className="text-muted-foreground mt-2 font-mono text-xs">
                  {t("agents.implicitWorkspace", {
                    path: defaults?.workspace ?? "—",
                  })}
                </p>
              </div>
              <Button type="button" size="sm" className="gap-1.5" onClick={openCreate}>
                <IconPlus className="size-4" />
                {t("agents.add")}
              </Button>
            </div>
          )}

          {agents.length > 0 && (
            <div className="grid gap-4 lg:grid-cols-[280px_1fr]">
              <div className="flex flex-col gap-2">
                {agents.map(renderAgentCard)}
              </div>

              {selectedAgent && (
                <div className="flex min-w-0 flex-col gap-4">
                  <div className="bg-card rounded-2xl border p-4 shadow-xs">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <AgentAvatar
                            agentId={selectedAgent.id}
                            avatar={selectedAgent.avatar}
                            name={selectedAgent.name || selectedAgent.id}
                            size="lg"
                          />
                          <h2 className="font-mono text-base font-semibold">
                            {selectedAgent.id}
                          </h2>
                          {selectedAgent.default && (
                            <Badge variant="secondary">{t("agents.defaultBadge")}</Badge>
                          )}
                        </div>
                        <p className="text-muted-foreground mt-1 text-sm">
                          {selectedAgent.name || t("agents.noName")}
                        </p>
                      </div>
                      <div className="flex items-center gap-1">
                        <Button asChild variant="ghost" size="sm">
                          <Link
                            to="/agent/files"
                            search={{ agent: selectedAgent.id }}
                          >
                            {t("agents.openFiles")}
                          </Link>
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={() => openEdit(selectedAgent)}
                        >
                          {t("agents.edit")}
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="text-muted-foreground hover:text-destructive h-8 w-8"
                          onClick={() => setDeleteTarget(selectedAgent)}
                          aria-label={t("agents.delete")}
                          title={t("agents.delete")}
                        >
                          <IconTrash className="size-4" />
                        </Button>
                      </div>
                    </div>

                    <div className="mt-4 flex gap-1">
                      <Button
                        type="button"
                        variant={tab === "overview" ? "secondary" : "ghost"}
                        size="sm"
                        onClick={() => setTab("overview")}
                      >
                        {t("agents.tabOverview")}
                      </Button>
                      <Button
                        type="button"
                        variant={tab === "routing" ? "secondary" : "ghost"}
                        size="sm"
                        className="gap-1.5"
                        onClick={() => setTab("routing")}
                      >
                        <IconRoute className="size-4" />
                        {t("agents.tabRouting")}
                      </Button>
                    </div>
                  </div>

                  {tab === "overview" ? (
                    <div className="bg-card flex flex-col gap-4 rounded-2xl border p-4 shadow-xs">
                      <dl className="grid gap-4 sm:grid-cols-2">
                        <div>
                          <dt className="text-muted-foreground text-xs uppercase">
                            {t("agents.model")}
                          </dt>
                          <dd className="mt-1 text-sm">
                            {selectedAgent.model?.primary ||
                              t("agents.useDefaultModel", {
                                model: selectedAgent.effective_model || "—",
                              })}
                          </dd>
                        </div>
                        <div>
                          <dt className="text-muted-foreground text-xs uppercase">
                            {t("agents.workspace")}
                          </dt>
                          <dd
                            className="mt-1 truncate font-mono text-xs"
                            title={selectedAgent.effective_workspace}
                          >
                            {basename(selectedAgent.effective_workspace)}
                            <span className="text-muted-foreground ml-2">
                              ({selectedAgent.workspace_source})
                            </span>
                          </dd>
                        </div>
                        <div>
                          <dt className="text-muted-foreground text-xs uppercase">
                            {t("agents.skills")}
                          </dt>
                          <dd className="mt-1 text-sm">
                            {selectedAgent.skills && selectedAgent.skills.length > 0
                              ? t("agents.skillCount", {
                                  count: selectedAgent.skills.length,
                                })
                              : t("agents.allSkills")}
                          </dd>
                        </div>
                        <div>
                          <dt className="text-muted-foreground text-xs uppercase">
                            {t("agents.sectionSubagents")}
                          </dt>
                          <dd className="mt-1 text-sm">
                            {selectedAgent.subagents?.enabled === false
                              ? t("agents.subagentsOff")
                              : selectedAgent.subagents?.allow_agents?.length
                                ? t("agents.allowedAgents", {
                                    agents: selectedAgent.subagents.allow_agents.join(", "),
                                  })
                                : t("agents.noDelegation")}
                          </dd>
                        </div>
                      </dl>
                      <p className="text-muted-foreground border-border/50 border-t pt-3 text-xs">
                        {t("agents.personalityHint")}
                      </p>
                    </div>
                  ) : (
                    <div className="bg-card flex flex-col gap-4 rounded-2xl border p-4 shadow-xs">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div>
                          <h3 className="text-sm font-semibold">
                            {t("agents.routingTitle")}
                          </h3>
                          <p className="text-muted-foreground text-xs">
                            {t("agents.routingDescription")}
                          </p>
                        </div>
                        <Button
                          type="button"
                          size="sm"
                          className="gap-1.5"
                          onClick={openCreateRule}
                          disabled={agents.length === 0}
                        >
                          <IconPlus className="size-4" />
                          {t("agents.addRule")}
                        </Button>
                      </div>

                      {dispatch.rules.length === 0 ? (
                        <p className="text-muted-foreground rounded-xl border border-dashed p-6 text-center text-sm">
                          {t("agents.noRules")}
                        </p>
                      ) : (
                        <ul className="flex flex-col gap-2">
                          {dispatch.rules.map((rule, index) => (
                            <li
                              key={`${rule.agent}-${index}`}
                              className="flex flex-wrap items-center justify-between gap-3 rounded-xl border p-3"
                            >
                              <div className="min-w-0">
                                <p className="text-sm">
                                  {describeRule(rule) || t("agents.ruleNoConditions")}
                                </p>
                                <p className="text-muted-foreground mt-0.5 text-xs">
                                  → <span className="font-mono">{rule.agent}</span>
                                  {rule.name ? ` · ${rule.name}` : ""}
                                </p>
                              </div>
                              <div className="flex items-center gap-1">
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="icon"
                                  className="h-8 w-8"
                                  onClick={() => moveRule(index, -1)}
                                  disabled={index === 0 || isSaving}
                                  aria-label={t("agents.moveUp")}
                                  title={t("agents.moveUp")}
                                >
                                  <IconArrowUp className="size-4" />
                                </Button>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="icon"
                                  className="h-8 w-8"
                                  onClick={() => moveRule(index, 1)}
                                  disabled={
                                    index === dispatch.rules.length - 1 || isSaving
                                  }
                                  aria-label={t("agents.moveDown")}
                                  title={t("agents.moveDown")}
                                >
                                  <IconArrowDown className="size-4" />
                                </Button>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => openEditRule(index)}
                                >
                                  {t("agents.edit")}
                                </Button>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="icon"
                                  className="text-muted-foreground hover:text-destructive h-8 w-8"
                                  onClick={() => deleteRule(index)}
                                  aria-label={t("agents.delete")}
                                  title={t("agents.delete")}
                                >
                                  <IconTrash className="size-4" />
                                </Button>
                              </div>
                            </li>
                          ))}
                        </ul>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {editor && (
        <AgentEditDialog
          open
          mode={editor.mode}
          initial={editor.agent}
          agents={agents}
          defaults={defaults ?? { workspace: "", provider: "", model_name: "" }}
          isSaving={isSaving}
          error={formError}
          onOpenChange={(open) => {
            if (!open) setEditor(null)
          }}
          onSave={saveAgent}
        />
      )}

      {ruleEditor && (
        <RoutingRuleDialog
          open
          rule={ruleEditor.rule}
          agents={agents}
          isSaving={isSaving}
          error={formError}
          onOpenChange={(open) => {
            if (!open) setRuleEditor(null)
          }}
          onSave={saveRule}
        />
      )}

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("agents.deleteTitle", { id: deleteTarget?.id ?? "" })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("agents.deleteDescription", {
                count: dispatch.rules.filter(
                  (rule) => rule.agent === deleteTarget?.id,
                ).length,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isSaving}>
              {t("common.cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={isSaving}
              onClick={(event) => {
                event.preventDefault()
                deleteAgent()
              }}
            >
              {t("agents.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
