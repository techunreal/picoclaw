import { createFileRoute, redirect } from "@tanstack/react-router"

export const Route = createFileRoute("/agent/hub")({
  beforeLoad: () => {
    throw redirect({
      to: "/agent/skills",
      search: { tab: "discover" },
      replace: true,
    })
  },
})
