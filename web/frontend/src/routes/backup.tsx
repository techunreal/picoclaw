import { createFileRoute } from "@tanstack/react-router"

import { BackupPage } from "@/components/backup/backup-page"

export const Route = createFileRoute("/backup")({
  component: BackupRoute,
})

function BackupRoute() {
  return <BackupPage />
}
