import {
  IconActivity,
  IconArchive,
  IconAtom,
  IconCalendarTime,
  IconChevronLeft,
  IconKey,
  IconListDetails,
  IconMessageCircle,
  IconNotebook,
  IconSettings,
  IconSparkles,
  IconTools,
  IconUsers,
  IconBroadcast,
} from "@tabler/icons-react"
import { Link, useRouterState } from "@tanstack/react-router"
import * as React from "react"
import { useTranslation } from "react-i18next"

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

const workspaceGroup: NavGroup = {
  label: "navigation.agent_group",
  items: [
    {
      title: "navigation.agents",
      url: "/agent/agents",
      icon: IconUsers,
      translateTitle: true,
      tour: "agents-nav",
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
      title: "navigation.tools",
      url: "/agent/tools",
      icon: IconTools,
      translateTitle: true,
    },
    {
      title: "navigation.subagents",
      url: "/agent/subagents",
      icon: IconActivity,
      translateTitle: true,
    },
  ],
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

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const routerState = useRouterState()
  const { t } = useTranslation()
  const { isMobile, setOpenMobile } = useSidebar()
  const currentPath = routerState.location.pathname
  const settingsMode = isSettingsPath(currentPath)

  const handleNavItemClick = React.useCallback(() => {
    if (isMobile) {
      setOpenMobile(false)
    }
  }, [isMobile, setOpenMobile])

  const groups: NavGroup[] = settingsMode ? [settingsGroup] : [workspaceGroup]

  return (
    <Sidebar
      {...props}
      className="bg-background border-r-border/20 border-r pt-2"
    >
      <SidebarContent className="bg-background">
        {settingsMode && (
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
        )}

        {!settingsMode && (
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
            </SidebarMenu>
          </SidebarGroup>
        )}

        {groups.map((group, groupIndex) => (
          <SidebarGroup
            key={group.label ?? `group-${groupIndex}`}
            className="px-2 py-0"
          >
            {group.label && (
              <SidebarGroupLabel
                className="text-muted-foreground/70 h-7 px-2.5 text-[10px] font-semibold tracking-wider uppercase"
                data-tour={
                  !settingsMode && group.label === "navigation.agent_group"
                    ? "agent-group"
                    : undefined
                }
              >
                {t(group.label)}
              </SidebarGroupLabel>
            )}
            {settingsMode && (
              <SidebarGroupLabel
                className="text-muted-foreground/70 h-8 px-2.5 text-[10px] font-semibold tracking-wider uppercase"
                data-tour="settings-nav"
              >
                {t("navigation.settings")}
              </SidebarGroupLabel>
            )}
            <SidebarGroupContent>
              <SidebarMenu>
                {group.items.map((item) => {
                  const isActive =
                    currentPath === item.url ||
                    (item.url !== "/" && currentPath.startsWith(`${item.url}/`))
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
        ))}
      </SidebarContent>

      <SidebarFooter className="bg-background border-border/20 border-t px-2 py-2">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              isActive={settingsMode}
              onClick={handleNavItemClick}
              data-tour="settings-gear"
              className={`h-8 px-2.5 text-[13px] ${settingsMode ? "bg-accent/80 text-foreground font-medium" : "text-muted-foreground hover:bg-muted/60"}`}
            >
              <Link to="/config">
                <IconSettings className="size-4" />
                <span>{t("navigation.settings")}</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
