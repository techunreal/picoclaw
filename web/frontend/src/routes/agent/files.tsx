import { createFileRoute } from "@tanstack/react-router"

import { MemoryPage } from "@/components/agent/memory/memory-page"

export const Route = createFileRoute("/agent/files")({
  component: AgentFilesRoute,
})

function AgentFilesRoute() {
  return <MemoryPage />
}
