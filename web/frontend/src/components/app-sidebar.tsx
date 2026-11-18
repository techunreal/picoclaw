import {
  IconActivity,
  IconArchive,
  IconArrowDown,
  IconArrowUp,
  IconAtom,
  IconBroadcast,
  IconCalendarTime,
  IconChevronDown,
  IconChevronLeft,
  IconChevronRight,
  IconDots,
  IconEye,
  IconKey,
  IconListDetails,
  IconMessageCircle,
  IconNotebook,
  IconPencil,
  IconPin,
  IconLoader2,
  IconSettings,
  IconSparkles,
  IconTools,
  IconTrash,
  IconUsers,
} from "@tabler/icons-react"
import { Link, useNavigate, useRouterState } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useAtomValue, useSetAtom } from "jotai"
import * as React from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  getAgents,
  updateAgentOrder,
  updateHiddenAgents,
  type AgentAvatar as AgentAvatarData,
} from "@/api/agents"
import {
  deleteSession,
  getSessions,
  updateSession,
  type SessionSummary,
} from "@/api/sessions"
import { AgentAvatar } from "@/components/agent-avatar"
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from "@/components/ui/sidebar"
import { chatAtom, pendingAgentChatAtom, pendingSessionAtom } from "@/store/chat"
import { newChatSession } from "@/features/chat/controller"

interface NavItem {
  title: string
  url: string
  icon: React.ComponentType<{ className?: string }>
  translateTitle?: boolean
  tour?: string
}

interface NavGroup {
  label?: string
  items: NavItem[]
}

const SETTINGS_PATHS = [
  "/config",
  "/models",
  "/credentials",
  "/channels",
  "/agent/agents",
  "/agent/files",
  "/agent/skills",
  "/agent/tools",
  "/cron",
  "/backup",
  "/logs",
]

function isSettingsPath(pathname: string) {
  return SETTINGS_PATHS.some(
    (path) => pathname === path || pathname.startsWith(`${path}/`),
  )
}

const settingsGroup: NavGroup = {
  items: [
    {
      title: "navigation.general",
      url: "/config",
      icon: IconSettings,
      translateTitle: true,
      tour: "settings-general",
    },
    {
      title: "navigation.models",
      url: "/models",
      icon: IconAtom,
      translateTitle: true,
      tour: "models-nav",
    },
    {
      title: "navigation.credentials",
      url: "/credentials",
      icon: IconKey,
      translateTitle: true,
    },
    {
      title: "navigation.agents",
      url: "/agent/agents",
      icon: IconUsers,
      translateTitle: true,
    },
    {
      title: "navigation.memory",
      url: "/agent/files",
      icon: IconNotebook,
      translateTitle: true,
    },
    {
      title: "navigation.skills",
      url: "/agent/skills",
      icon: IconSparkles,
      translateTitle: true,
    },
    {
      title: "navigation.channels_group",
      url: "/channels",
      icon: IconBroadcast,
      translateTitle: true,
    },
    {
      title: "navigation.tools",
      url: "/agent/tools",
      icon: IconTools,
      translateTitle: true,
    },
    {
      title: "navigation.cron",
      url: "/cron",
      icon: IconCalendarTime,
      translateTitle: true,
    },
    {
      title: "navigation.backup",
      url: "/backup",
      icon: IconArchive,
      translateTitle: true,
    },
    {
      title: "navigation.logs",
      url: "/logs",
      icon: IconListDetails,
      translateTitle: true,
    },
  ],
}

interface AgentChatGroup {
  id: string
  label: string
  isDefault: boolean
  avatar?: AgentAvatarData | null
}

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const routerState = useRouterState()
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { isMobile, setOpenMobile } = useSidebar()
  const currentPath = routerState.location.pathname
  const settingsMode = isSettingsPath(currentPath)
  const queryClient = useQueryClient()
  const setPendingSession = useSetAtom(pendingSessionAtom)
  const setPendingAgentChat = useSetAtom(pendingAgentChatAtom)
  const chatState = useAtomValue(chatAtom)

  const [expandedAgents, setExpandedAgents] = React.useState<
    Record<string, boolean>
  >({})
  const [renameTarget, setRenameTarget] = React.useState<SessionSummary | null>(
    null,
  )
  const [renameValue, setRenameValue] = React.useState("")
  const [deleteTarget, setDeleteTarget] = React.useState<SessionSummary | null>(
    null,
  )

  const agentsQuery = useQuery({
    queryKey: ["sidebar-agents"],
    queryFn: getAgents,
  })
  const sessionsQuery = useQuery({
    queryKey: ["sidebar-sessions"],
    queryFn: () => getSessions(0, 20),
    refetchOnWindowFocus: true,
    refetchInterval: 3000,
  })

  const allAgents = React.useMemo(
    () => agentsQuery.data?.agents ?? [],
    [agentsQuery.data],
  )
  const hiddenAgents = React.useMemo(
    () => agentsQuery.data?.hidden ?? [],
    [agentsQuery.data],
  )
  const sessions = sessionsQuery.data ?? []

  const refreshAgents = React.useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["sidebar-agents"] })
    void queryClient.invalidateQueries({ queryKey: ["agents"] })
  }, [queryClient])
  const refreshSessions = React.useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ["sidebar-sessions"] })
  }, [queryClient])

  const startAgentChat = React.useCallback(
    (agentId: string) => {
      setPendingAgentChat(agentId)
      void navigate({ to: "/" })
      if (isMobile) setOpenMobile(false)
    },
    [navigate, setPendingAgentChat, isMobile, setOpenMobile],
  )

  const moveAgent = React.useCallback(
    async (agentId: string, direction: -1 | 1) => {
      const ids = allAgents.map((agent) => agent.id)
      const index = ids.indexOf(agentId)
      const target = index + direction
      if (index < 0 || target < 0 || target >= ids.length) return
      ;[ids[index], ids[target]] = [ids[target], ids[index]]
      try {
        await updateAgentOrder(ids)
        refreshAgents()
      } catch {
        toast.error(t("agents.reorderFailed"))
      }
    },
    [allAgents, refreshAgents, t],
  )

  const hideAgent = React.useCallback(
    async (agentId: string) => {
      try {
        await updateHiddenAgents([...hiddenAgents, agentId])
        refreshAgents()
      } catch {
        toast.error(t("agents.hideFailed"))
      }
    },
    [hiddenAgents, refreshAgents, t],
  )

  const unhideAgent = React.useCallback(
    async (agentId: string) => {
      try {
        await updateHiddenAgents(hiddenAgents.filter((id) => id !== agentId))
        refreshAgents()
      } catch {
        toast.error(t("agents.hideFailed"))
      }
    },
    [hiddenAgents, refreshAgents, t],
  )

  const toggleSessionPin = React.useCallback(
    async (session: SessionSummary) => {
      try {
        await updateSession(session.id, { pinned: !session.pinned })
        refreshSessions()
      } catch {
        toast.error(t("chat.pinFailed"))
      }
    },
    [refreshSessions, t],
  )

  const openRename = (session: SessionSummary) => {
    setRenameTarget(session)
    setRenameValue(session.title)
  }

  const saveRename = async () => {
    if (!renameTarget) return
    try {
      await updateSession(renameTarget.id, { title: renameValue.trim() })
      setRenameTarget(null)
      refreshSessions()
    } catch {
      toast.error(t("chat.renameFailed"))
    }
  }

  const confirmDelete = async () => {
    if (!deleteTarget) return
    const target = deleteTarget
    try {
      await deleteSession(target.id)
      setDeleteTarget(null)
      if (target.id === chatState.activeSessionId) {
        // The open conversation was deleted: clear it and start a fresh chat
        // for the same agent instead of leaving stale messages on screen.
        await newChatSession(target.agent_id)
      }
      refreshSessions()
      toast.success(t("chat.deleted"))
    } catch {
      toast.error(t("chat.deleteFailed"))
    }
  }

  const handleNavItemClick = React.useCallback(() => {
    if (isMobile) {
      setOpenMobile(false)
    }
  }, [isMobile, setOpenMobile])

  const agentGroups: AgentChatGroup[] = React.useMemo(() => {
    const groups: AgentChatGroup[] = allAgents
      .filter((agent) => !hiddenAgents.includes(agent.id))
      .map((agent) => ({
        id: agent.id,
        label:
          agent.name?.trim() ||
          (agent.id === "main" ? "Pico" : agent.id),
        isDefault: agent.default === true,
        avatar: agent.avatar,
      }))
    if (
      !groups.some((group) => group.id === "main") &&
      !hiddenAgents.includes("main")
    ) {
      groups.unshift({
        id: "main",
        label: "Pico",
        isDefault: !groups.some((group) => group.isDefault),
      })
    }
    return groups
  }, [allAgents, hiddenAgents])

  const hiddenAgentGroups = React.useMemo(
    () =>
      hiddenAgents.map((id) => {
        const agent = allAgents.find((item) => item.id === id)
        return {
          id,
          label:
            agent?.name?.trim() || (id === "main" ? "Pico" : id),
          avatar: agent?.avatar,
        }
      }),
    [allAgents, hiddenAgents],
  )

  const openSession = React.useCallback(
    (sessionId: string) => {
      setPendingSession(sessionId)
      void navigate({ to: "/" })
      if (isMobile) setOpenMobile(false)
    },
    [navigate, setPendingSession, isMobile, setOpenMobile],
  )

  const isExpanded = (agent: AgentChatGroup) =>
    expandedAgents[agent.id] ?? (agent.id === "main" || agent.isDefault)

  return (
    <Sidebar
      {...props}
      className="bg-background border-r-border/20 border-r pt-2"
    >
      <SidebarContent className="bg-background">
        {settingsMode ? (
          <>
            <SidebarGroup className="px-2 pt-1 pb-0">
              <Link
                to="/"
                onClick={handleNavItemClick}
                className="text-muted-foreground hover:bg-muted/60 hover:text-foreground flex items-center gap-1.5 rounded-md px-2 py-1.5 text-xs font-medium transition-colors"
              >
                <IconChevronLeft className="size-3.5" />
                {t("navigation.back_to_app")}
              </Link>
            </SidebarGroup>
            <SidebarGroup className="px-2 py-0">
              <SidebarGroupLabel
                className="text-muted-foreground/70 h-8 px-2.5 text-[10px] font-semibold tracking-wider uppercase"
                data-tour="settings-nav"
              >
                {t("navigation.settings")}
              </SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {settingsGroup.items.map((item) => {
                    const isActive =
                      currentPath === item.url ||
                      currentPath.startsWith(`${item.url}/`)
                    return (
                      <SidebarMenuItem key={`${item.url}-${item.title}`}>
                        <SidebarMenuButton
                          asChild
                          isActive={isActive}
                          onClick={handleNavItemClick}
                          data-tour={item.tour}
                          className={`h-8 px-2.5 text-[13px] ${isActive ? "bg-accent/80 text-foreground font-medium" : "text-muted-foreground hover:bg-muted/60"}`}
                        >
                          <Link to={item.url}>
                            <item.icon
                              className={`size-4 ${isActive ? "opacity-100" : "opacity-60"}`}
                            />
                            <span
                              className={
                                isActive ? "opacity-100" : "opacity-80"
                              }
                            >
                              {item.translateTitle === false
                                ? item.title
                                : t(item.title)}
                            </span>
                          </Link>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
                    )
                  })}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          </>
        ) : (
          <>
            <SidebarGroup className="px-2 py-0">
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={currentPath === "/"}
                    onClick={handleNavItemClick}
                    data-tour="chat-nav"
                    className={`h-8 px-2.5 text-[13px] ${currentPath === "/" ? "bg-accent/80 text-foreground font-medium" : "text-muted-foreground hover:bg-muted/60"}`}
                  >
                    <Link to="/">
                      <IconMessageCircle
                        className={`size-4 ${currentPath === "/" ? "opacity-100" : "opacity-60"}`}
                      />
                      <span>{t("navigation.chat")}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={currentPath.startsWith("/agent/subagents")}
                    onClick={handleNavItemClick}
                    className={`h-8 px-2.5 text-[13px] ${currentPath.startsWith("/agent/subagents") ? "bg-accent/80 text-foreground font-medium" : "text-muted-foreground hover:bg-muted/60"}`}
                  >
                    <Link to="/agent/subagents">
                      <IconActivity className="size-4 opacity-60" />
                      <span>{t("navigation.subagents")}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroup>

            <div className="border-border/40 mx-3 my-2 border-t" data-tour="agent-group" />

            <div className="flex min-h-0 flex-1 flex-col">
              <SidebarGroup className="px-2 py-0">
                <div className="group/chats flex items-center justify-between pr-2">
                  <SidebarGroupLabel
                    className="text-muted-foreground/70 h-7 px-2.5 text-[10px] font-semibold tracking-wider uppercase"
                    data-tour="chat-profiles"
                  >
                    {t("navigation.chats")}
                  </SidebarGroupLabel>
                  {hiddenAgentGroups.length > 0 && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <button
                          type="button"
                          aria-label={t("agents.showHidden")}
                          title={t("agents.showHidden")}
                          className="text-muted-foreground hover:text-foreground rounded-md p-1 opacity-0 transition-opacity group-hover/chats:opacity-100 focus-visible:opacity-100"
                        >
                          <IconDots className="size-4" />
                        </button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="min-w-48">
                        <div className="text-muted-foreground px-2 py-1.5 text-xs font-medium">
                          {t("agents.showHidden")}
                        </div>
                        {hiddenAgentGroups.map((hidden) => (
                          <DropdownMenuItem
                            key={hidden.id}
                            onSelect={() => void unhideAgent(hidden.id)}
                            className="gap-2"
                          >
                            <AgentAvatar
                              agentId={hidden.id}
                              avatar={hidden.avatar}
                              name={hidden.label}
                              fallback={hidden.id === "main" ? "🦞" : undefined}
                              size="sm"
                            />
                            <span>{hidden.label}</span>
                            <IconEye className="ml-auto size-4" />
                          </DropdownMenuItem>
                        ))}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </div>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {agentGroups.map((agent) => {
                      const expanded = isExpanded(agent)
                      const agentSessions = sessions.filter(
                        (session) => (session.agent_id || "main") === agent.id,
                      )
                      const agentIndex = allAgents.findIndex(
                        (item) => item.id === agent.id,
                      )
                      return (
                        <React.Fragment key={agent.id}>
                          <SidebarMenuItem className="group/agent relative">
                            <SidebarMenuButton
                              onClick={() =>
                                setExpandedAgents((prev) => ({
                                  ...prev,
                                  [agent.id]: !expanded,
                                }))
                              }
                              className="text-muted-foreground hover:bg-muted/60 h-8 px-2.5 pr-7 text-[13px]"
                            >
                              {expanded ? (
                                <IconChevronDown className="size-3.5 shrink-0 opacity-60" />
                              ) : (
                                <IconChevronRight className="size-3.5 shrink-0 opacity-60" />
                              )}
                              <AgentAvatar
                                agentId={agent.id}
                                avatar={agent.avatar}
                                name={agent.label}
                                fallback={agent.id === "main" ? "🦞" : undefined}
                                size="sm"
                              />
                              <span className="truncate">{agent.label}</span>
                            </SidebarMenuButton>
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <button
                                  type="button"
                                  aria-label={t("agents.agentMenu")}
                                  title={t("agents.agentMenu")}
                                  onClick={(event) => event.stopPropagation()}
                                  className="text-muted-foreground hover:bg-muted/70 hover:text-foreground absolute top-1.5 right-1 rounded-md p-0.5 opacity-0 transition-opacity group-hover/agent:opacity-100 focus-visible:opacity-100"
                                >
                                  <IconDots className="size-4" />
                                </button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="min-w-44">
                                <DropdownMenuItem
                                  onSelect={() => startAgentChat(agent.id)}
                                  className="gap-2"
                                >
                                  <IconMessageCircle className="size-4" />
                                  {t("agents.newChat")}
                                </DropdownMenuItem>
                                {agent.id !== "main" && (
                                  <>
                                    <DropdownMenuItem
                                      disabled={agentIndex <= 0}
                                      onSelect={() => void moveAgent(agent.id, -1)}
                                      className="gap-2"
                                    >
                                      <IconArrowUp className="size-4" />
                                      {t("agents.moveUp")}
                                    </DropdownMenuItem>
                                    <DropdownMenuItem
                                      disabled={
                                        agentIndex < 0 ||
                                        agentIndex >= allAgents.length - 1
                                      }
                                      onSelect={() => void moveAgent(agent.id, 1)}
                                      className="gap-2"
                                    >
                                      <IconArrowDown className="size-4" />
                                      {t("agents.moveDown")}
                                    </DropdownMenuItem>
                                  </>
                                )}
                                <DropdownMenuSeparator />
                                <DropdownMenuItem
                                  onSelect={() => void hideAgent(agent.id)}
                                  className="gap-2"
                                >
                                  <IconEye className="size-4" />
                                  {t("agents.hide")}
                                </DropdownMenuItem>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </SidebarMenuItem>
                          {expanded && (
                            <div className="border-border/30 ml-4 border-l pl-1">
                              {agentSessions.length === 0 && (
                                <p className="text-muted-foreground/70 px-2.5 py-1.5 text-[11px]">
                                  {t("navigation.no_chats")}
                                </p>
                              )}
                              {agentSessions.map((session) => {
                                const active =
                                  session.id === chatState.activeSessionId
                                const title =
                                  session.title.trim() ||
                                  t("chat.untitledChat")
                                return (
                                  <div
                                    key={session.id}
                                    className="group/session relative"
                                  >
                                    <button
                                      type="button"
                                      onClick={() => openSession(session.id)}
                                      title={title}
                                      className={`block w-full truncate rounded-md px-2.5 py-1.5 pr-7 text-left text-[12px] transition-colors ${
                                        active
                                          ? "bg-accent/70 text-foreground font-medium"
                                          : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
                                      }`}
                                    >
                                      {session.running && (
                                        <IconLoader2 className="text-primary mr-1 inline size-3 -translate-y-px animate-spin" />
                                      )}
                                      {session.pinned && (
                                        <IconPin className="mr-1 inline size-3 -translate-y-px" />
                                      )}
                                      {title}
                                    </button>
                                    <DropdownMenu>
                                      <DropdownMenuTrigger asChild>
                                        <button
                                          type="button"
                                          aria-label={t("chat.sessionMenu")}
                                          title={t("chat.sessionMenu")}
                                          className="text-muted-foreground hover:bg-muted/70 hover:text-foreground absolute top-1 right-1 rounded-md p-0.5 opacity-0 transition-opacity group-hover/session:opacity-100 focus-visible:opacity-100"
                                        >
                                          <IconDots className="size-3.5" />
                                        </button>
                                      </DropdownMenuTrigger>
                                      <DropdownMenuContent align="end" className="min-w-40">
                                        <DropdownMenuItem
                                          onSelect={() => void toggleSessionPin(session)}
                                          className="gap-2"
                                        >
                                          <IconPin className="size-4" />
                                          {session.pinned
                                            ? t("chat.unpin")
                                            : t("chat.pin")}
                                        </DropdownMenuItem>
                                        <DropdownMenuItem
                                          onSelect={() => openRename(session)}
                                          className="gap-2"
                                        >
                                          <IconPencil className="size-4" />
                                          {t("chat.rename")}
                                        </DropdownMenuItem>
                                        <DropdownMenuSeparator />
                                        <DropdownMenuItem
                                          onSelect={() => setDeleteTarget(session)}
                                          className="text-destructive gap-2"
                                        >
                                          <IconTrash className="size-4" />
                                          {t("chat.delete")}
                                        </DropdownMenuItem>
                                      </DropdownMenuContent>
                                    </DropdownMenu>
                                  </div>
                                )
                              })}
                            </div>
                          )}
                        </React.Fragment>
                      )
                    })}
                  </SidebarMenu>
                </SidebarGroupContent>
              </SidebarGroup>
            </div>
          </>
        )}
      </SidebarContent>

      <SidebarFooter className="bg-background border-border/20 border-t px-2 py-2">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              isActive={settingsMode}
              onClick={handleNavItemClick}
              data-tour="settings-gear"
              title={t("navigation.settings")}
              aria-label={t("navigation.settings")}
              className={`h-8 w-8 justify-center px-0 text-[13px] ${settingsMode ? "bg-accent/80 text-foreground" : "text-muted-foreground hover:bg-muted/60"}`}
            >
              <Link to="/config" aria-label={t("navigation.settings")}>
                <IconSettings className="size-4" />
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />

      <Dialog
        open={renameTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRenameTarget(null)
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("chat.renameTitle")}</DialogTitle>
            <DialogDescription>{t("chat.renameDescription")}</DialogDescription>
          </DialogHeader>
          <Input
            value={renameValue}
            onChange={(event) => setRenameValue(event.target.value)}
            maxLength={60}
            autoFocus
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault()
                void saveRename()
              }
            }}
          />
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setRenameTarget(null)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="button" onClick={() => void saveRename()}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("chat.deleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("chat.deleteDescription", {
                title: deleteTarget?.title ?? "",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault()
                void confirmDelete()
              }}
            >
              {t("chat.delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Sidebar>
  )
}
