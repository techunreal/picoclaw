import { IconBrandGithub, IconLoader2, IconPlus } from "@tabler/icons-react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { type FormEvent, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { installSkill } from "@/api/skills"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

interface GithubSkillInstallProps {
  canInstall: boolean
}

function looksLikeGithubTarget(target: string): boolean {
  if (target.includes("://")) {
    try {
      const parsed = new URL(target)
      if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
        return false
      }
      return (
        parsed.hostname.length > 0 &&
        parsed.pathname.split("/").filter(Boolean).length >= 2
      )
    } catch {
      return false
    }
  }
  return /^[^\s/]+\/[^\s/]+/.test(target)
}

export function GithubSkillInstall({ canInstall }: GithubSkillInstallProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [value, setValue] = useState("")
  const [error, setError] = useState<string | null>(null)

  const installMutation = useMutation({
    mutationFn: (target: string) =>
      installSkill({ slug: target, registry: "github" }),
    onSuccess: (response) => {
      toast.success(
        t("pages.agent.skills.github_install_success", {
          name: response.skill?.name ?? response.slug,
        }),
      )
      setValue("")
      setError(null)
      void queryClient.invalidateQueries({ queryKey: ["skills"] })
      void queryClient.invalidateQueries({ queryKey: ["skills-marketplace"] })
      void queryClient.invalidateQueries({ queryKey: ["skills-popular"] })
    },
    onError: (err) => {
      setError(
        err instanceof Error
          ? err.message
          : t("pages.agent.skills.github_install_error"),
      )
    },
  })

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const target = value.trim()
    if (target === "") {
      setError(t("pages.agent.skills.github_install_required"))
      return
    }
    if (!looksLikeGithubTarget(target)) {
      setError(t("pages.agent.skills.github_install_invalid"))
      return
    }
    setError(null)
    installMutation.mutate(target)
  }

  return (
    <section className="border-border/50 bg-muted/10 w-full rounded-xl border px-4 py-4 text-left">
      <div className="flex items-start gap-3">
        <div className="bg-background border-border/50 flex size-9 shrink-0 items-center justify-center rounded-lg border">
          <IconBrandGithub className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">
            {t("pages.agent.skills.github_title")}
          </h3>
          <p className="text-muted-foreground mt-1 text-xs leading-relaxed">
            {t("pages.agent.skills.github_description")}
          </p>

          {canInstall ? (
            <form
              className="mt-3 flex flex-col gap-2 sm:flex-row"
              onSubmit={handleSubmit}
            >
              <Input
                value={value}
                onChange={(event) => setValue(event.target.value)}
                placeholder={t("pages.agent.skills.github_placeholder")}
                className="h-9 flex-1 font-mono text-xs"
                disabled={installMutation.isPending}
                spellCheck={false}
              />
              <Button
                type="submit"
                size="sm"
                disabled={installMutation.isPending || value.trim() === ""}
              >
                {installMutation.isPending ? (
                  <IconLoader2 className="size-4 animate-spin" />
                ) : (
                  <IconPlus className="size-4" />
                )}
                {t("pages.agent.skills.github_add")}
              </Button>
            </form>
          ) : (
            <p className="text-muted-foreground mt-3 text-xs">
              {t("pages.agent.skills.github_disabled")}
            </p>
          )}

          {error ? (
            <p className="text-destructive mt-2 text-xs">{error}</p>
          ) : null}

          <p className="text-muted-foreground/70 mt-2 text-[11px] leading-relaxed">
            {t("pages.agent.skills.github_warning")}
          </p>
        </div>
      </div>
    </section>
  )
}
