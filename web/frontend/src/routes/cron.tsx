import { createFileRoute } from "@tanstack/react-router"

import { CronPage } from "@/components/automation/cron-page"

export const Route = createFileRoute("/cron")({
  component: CronRoute,
})

function CronRoute() {
  return <CronPage />
}
