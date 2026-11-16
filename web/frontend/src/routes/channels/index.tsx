import { createFileRoute } from "@tanstack/react-router"

import { ChannelsOverview } from "@/components/channels/channels-overview"

export const Route = createFileRoute("/channels/")({
  component: ChannelsOverview,
})
