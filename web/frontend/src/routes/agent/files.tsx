import { createFileRoute } from "@tanstack/react-router"

import { MemoryPage } from "@/components/agent/memory/memory-page"

export const Route = createFileRoute("/agent/files")({
  validateSearch: (search: Record<string, unknown>) => ({
    agent:
      typeof search.agent === "string" && search.agent.trim() !== ""
        ? search.agent.trim()
        : undefined,
  }),
  component: AgentFilesRoute,
})

function AgentFilesRoute() {
  const { agent } = Route.useSearch()
  return <MemoryPage agentId={agent} />
}
