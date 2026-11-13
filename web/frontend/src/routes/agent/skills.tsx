import { createFileRoute, useNavigate } from "@tanstack/react-router"

import { SkillsPage } from "@/components/agent/skills/skills-page"
import type { SkillPageTab } from "@/components/agent/skills/types"

interface SkillsSearch {
  tab: SkillPageTab
}

export const Route = createFileRoute("/agent/skills")({
  validateSearch: (search: Record<string, unknown>): SkillsSearch => ({
    tab: search.tab === "discover" ? "discover" : "installed",
  }),
  component: AgentSkillsRoute,
})

function AgentSkillsRoute() {
  const { tab } = Route.useSearch()
  const navigate = useNavigate()

  return (
    <SkillsPage
      tab={tab}
      onTabChange={(nextTab) => {
        void navigate({
          to: "/agent/skills",
          search: { tab: nextTab },
          replace: true,
        })
      }}
    />
  )
}
