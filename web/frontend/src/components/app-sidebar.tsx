import {
  IconActivity,
  IconArchive,
  IconAtom,
  IconBroadcast,
  IconCalendarTime,
  IconChevronDown,
  IconChevronLeft,
  IconChevronRight,
  IconKey,
  IconListDetails,
  IconMessageCircle,
  IconNotebook,
  IconSettings,
  IconSparkles,
  IconTools,
  IconUsers,
} from "@tabler/icons-react"
import { Link, useNavigate, useRouterState } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useAtomValue, useSetAtom } from "jotai"
import * as React from "react"
import { useTranslation } from "react-i18next"

import { getAgents, type AgentAvatar as AgentAvatarData } from "@/api/agents"
import { getSessions } from "@/api/sessions"
import { AgentAvatar } from "@/components/agent-avatar"
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
import { chatAtom, pendingSessionAtom } from "@/store/chat"

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
  const setPendingSession = useSetAtom(pendingSessionAtom)
  const chatState = useAtomValue(chatAtom)

  const [expandedAgents, setExpandedAgents] = React.useState<
    Record<string, boolean>
  >({})

  const agentsQuery = useQuery({
    queryKey: ["sidebar-agents"],
    queryFn: getAgents,
  })
  const sessionsQuery = useQuery({
    queryKey: ["sidebar-sessions"],
    queryFn: () => getSessions(0, 20),
    refetchOnWindowFocus: true,
  })

  const handleNavItemClick = React.useCallback(() => {
    if (isMobile) {
      setOpenMobile(false)
    }
  }, [isMobile, setOpenMobile])

  const agentGroups: AgentChatGroup[] = React.useMemo(() => {
    const agents = agentsQuery.data?.agents ?? []
    const groups: AgentChatGroup[] = agents.map((agent) => ({
      id: agent.id,
      label:
        agent.name?.trim() ||
        (agent.id === "main" ? "Pico" : agent.id),
      isDefault: agent.default === true,
      avatar: agent.avatar,
    }))
    if (!groups.some((group) => group.id === "main")) {
      groups.unshift({
        id: "main",
        label: "Pico",
        isDefault: !groups.some((group) => group.isDefault),
      })
    }
    return groups
  }, [agentsQuery.data])

  const sessions = sessionsQuery.data ?? []

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
                <SidebarGroupLabel
                  className="text-muted-foreground/70 h-7 px-2.5 text-[10px] font-semibold tracking-wider uppercase"
                  data-tour="chat-profiles"
                >
                  {t("navigation.chats")}
                </SidebarGroupLabel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {agentGroups.map((agent) => {
                      const expanded = isExpanded(agent)
                      const agentSessions = sessions.filter(
                        (session) => (session.agent_id || "main") === agent.id,
                      )
                      return (
                        <React.Fragment key={agent.id}>
                          <SidebarMenuItem>
                            <SidebarMenuButton
                              onClick={() =>
                                setExpandedAgents((prev) => ({
                                  ...prev,
                                  [agent.id]: !expanded,
                                }))
                              }
                              className="text-muted-foreground hover:bg-muted/60 h-8 px-2.5 text-[13px]"
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
                                  <button
                                    key={session.id}
                                    type="button"
                                    onClick={() => openSession(session.id)}
                                    title={title}
                                    className={`block w-full truncate rounded-md px-2.5 py-1.5 text-left text-[12px] transition-colors ${
                                      active
                                        ? "bg-accent/70 text-foreground font-medium"
                                        : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
                                    }`}
                                  >
                                    {title}
                                  </button>
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
    </Sidebar>
  )
}
