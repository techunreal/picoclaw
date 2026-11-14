import { createFileRoute } from "@tanstack/react-router"

import { AgentsPage } from "@/components/agent/agents/agents-page"

export const Route = createFileRoute("/agent/agents")({
  component: AgentAgentsRoute,
})

function AgentAgentsRoute() {
  return <AgentsPage />
}
