import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { type ReactNode, useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  type BrowserToolConfig,
  getBrowserInstallStatus,
  getBrowserToolConfig,
  installBrowserEngine,
  testBrowserTool,
  updateBrowserToolConfig,
} from "@/api/tools"
import { ConfigChangeNotice } from "@/components/config-change-notice"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { showSaveSuccessOrRestartToast } from "@/lib/restart-required"
import { refreshGatewayState } from "@/store/gateway"

const queryKey = ["tools", "browser-config"] as const

function withoutStatus(config: BrowserToolConfig): Omit<BrowserToolConfig, "status"> {
  const rest: Partial<BrowserToolConfig> = { ...config }
  delete rest.status
  return rest as Omit<BrowserToolConfig, "status">
}

function sameConfig(left: BrowserToolConfig, right: BrowserToolConfig) {
  return JSON.stringify(withoutStatus(left)) === JSON.stringify(withoutStatus(right))
}

export function BrowserTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<BrowserToolConfig | null>(null)

  const query = useQuery({
    queryKey,
    queryFn: getBrowserToolConfig,
  })

  const installQuery = useQuery({
    queryKey: ["tools", "browser-install"],
    queryFn: getBrowserInstallStatus,
    refetchInterval: (queryState) =>
      queryState.state.data?.running ? 2000 : false,
  })

  const installMutation = useMutation({
    mutationFn: installBrowserEngine,
    onSuccess: (status) => {
      queryClient.setQueryData(["tools", "browser-install"], status)
      toast.success(
        t(
          "pages.agent.tools.browser.download_started",
          "Browser download started. This can take a while.",
        ),
      )
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.agent.tools.browser.download_failed",
              "Could not start the browser download",
            ),
      ),
  })

  useEffect(() => {
    if (installQuery.data?.done && !installQuery.data.error) {
      void queryClient.invalidateQueries({ queryKey })
    }
  }, [installQuery.data?.done, installQuery.data?.error, queryClient])

  useEffect(() => {
    if (query.data) {
      setDraft(query.data)
    }
  }, [query.data])

  const isDirty = useMemo(
    () => (draft && query.data ? !sameConfig(draft, query.data) : false),
    [draft, query.data],
  )

  const saveMutation = useMutation({
    mutationFn: (config: BrowserToolConfig) =>
      updateBrowserToolConfig(withoutStatus(config)),
    onSuccess: async (updated) => {
      queryClient.setQueryData(queryKey, updated)
      setDraft(updated)
      const gateway = await refreshGatewayState({ force: true })
      showSaveSuccessOrRestartToast(
        t,
        t("pages.agent.tools.browser.save_success", "Browser settings saved"),
        t("pages.agent.tools.browser.title", "Browser"),
        gateway?.restartRequired === true,
      )
      void queryClient.invalidateQueries({ queryKey: ["tools"] })
    },
    onError: (error) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.agent.tools.browser.save_error",
              "Failed to save browser settings",
            ),
      )
    },
  })

  const testMutation = useMutation({
    mutationFn: testBrowserTool,
    onSuccess: (result) => {
      if (result.ok) {
        toast.success(
          t("pages.agent.tools.browser.test_ok", {
            version: result.version ?? "agent-browser",
            defaultValue: "agent-browser is available ({{version}})",
          }),
        )
        return
      }
      toast.error(
        result.error ||
          t(
            "pages.agent.tools.browser.test_failed",
            "agent-browser could not be started",
          ),
      )
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t(
              "pages.agent.tools.browser.test_failed",
              "agent-browser could not be started",
            ),
      ),
  })

  const update = (patch: Partial<BrowserToolConfig>) => {
    setDraft((current) => (current ? { ...current, ...patch } : current))
  }

  return (
    <div className="animate-in fade-in slide-in-from-bottom-2 space-y-8 pt-2 duration-500">
      {query.isError ? (
        <div className="py-20 text-center">
          <p className="text-destructive font-medium">
            {t(
              "pages.agent.tools.browser.load_error",
              "Failed to load browser settings",
            )}
          </p>
        </div>
      ) : query.isLoading || !draft ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full rounded-2xl" />
          <Skeleton className="h-64 w-full rounded-2xl" />
        </div>
      ) : (
        <>
          <div className="flex flex-col gap-6 sm:flex-row sm:items-start sm:justify-between">
            <div className="max-w-xl space-y-3">
              <h1 className="text-foreground/90 text-2xl font-semibold tracking-tight">
                {t("pages.agent.tools.browser.title", "Browser")}
              </h1>
              <p className="text-muted-foreground/80 text-[14px] leading-relaxed">
                {t(
                  "pages.agent.tools.browser.description",
                  "Let the agent drive a real headless browser. Only allowlisted domains can be reached; JavaScript eval, uploads and downloads stay blocked.",
                )}
              </p>
            </div>

            <div className="flex shrink-0 gap-2">
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-xl px-4"
                onClick={() => testMutation.mutate()}
                disabled={testMutation.isPending}
              >
                {testMutation.isPending
                  ? t("pages.agent.tools.browser.testing", "Testing…")
                  : t("pages.agent.tools.browser.test", "Test")}
              </Button>
              <Button
                type="button"
                className="h-10 rounded-xl px-6 shadow-sm"
                onClick={() => draft && saveMutation.mutate(draft)}
                disabled={!isDirty || saveMutation.isPending}
              >
                {t("pages.agent.tools.browser.save", "Save Changes")}
              </Button>
            </div>
          </div>

          {isDirty && (
            <ConfigChangeNotice
              kind="save"
              title={t(
                "pages.agent.tools.browser.unsaved",
                "Unsaved changes",
              )}
              description={t(
                "pages.agent.tools.browser.unsaved_description",
                "Save to write these settings to config.json.",
              )}
            />
          )}

          <div className="bg-card border-border/40 divide-border/40 divide-y overflow-hidden rounded-2xl border shadow-sm">
            <SettingRow
              label={t("pages.agent.tools.browser.enabled", "Enable browser")}
              description={t(
                "pages.agent.tools.browser.enabled_description",
                "Register the browser tool for the agent. Requires a gateway restart.",
              )}
            >
              <Switch
                checked={draft.enabled}
                onCheckedChange={(checked) => update({ enabled: checked })}
                aria-label={t(
                  "pages.agent.tools.browser.enabled",
                  "Enable browser",
                )}
              />
            </SettingRow>

            <SettingRow
              label={t("pages.agent.tools.browser.headless", "Headless")}
              description={t(
                "pages.agent.tools.browser.headless_description",
                "Run the browser without a visible window.",
              )}
            >
              <Switch
                checked={draft.headless}
                onCheckedChange={(checked) => update({ headless: checked })}
                aria-label={t("pages.agent.tools.browser.headless", "Headless")}
              />
            </SettingRow>

            <SettingRow
              label={t(
                "pages.agent.tools.browser.allow_all",
                "Allow all domains",
              )}
              description={t(
                "pages.agent.tools.browser.allow_all_description",
                "Disables the domain allowlist. Only use this for trusted research tasks.",
              )}
            >
              <Switch
                checked={draft.allow_all_domains}
                onCheckedChange={(checked) =>
                  update({ allow_all_domains: checked })
                }
                aria-label={t(
                  "pages.agent.tools.browser.allow_all",
                  "Allow all domains",
                )}
              />
            </SettingRow>

            <SettingRow
              label={t(
                "pages.agent.tools.browser.allow_remote",
                "Allow remote channels",
              )}
              description={t(
                "pages.agent.tools.browser.allow_remote_description",
                "Let external chat channels (Telegram, Discord, ...) drive the browser. Off by default.",
              )}
            >
              <Switch
                checked={draft.allow_remote}
                onCheckedChange={(checked) =>
                  update({ allow_remote: checked })
                }
                aria-label={t(
                  "pages.agent.tools.browser.allow_remote",
                  "Allow remote channels",
                )}
              />
            </SettingRow>
          </div>

          <div className="space-y-3">
            <div className="space-y-1">
              <h3 className="text-foreground/80 text-[13px] font-bold tracking-widest uppercase">
                {t("pages.agent.tools.browser.domains", "Allowed domains")}
              </h3>
              <p className="text-muted-foreground text-sm">
                {t(
                  "pages.agent.tools.browser.domains_description",
                  "One host per line. Use *.example.com to allow subdomains. CDN hosts must be listed too or pages may render blank.",
                )}
              </p>
            </div>
            <Textarea
              value={draft.allowed_domains.join("\n")}
              onChange={(event) =>
                update({
                  allowed_domains: event.target.value
                    .split(/[\n,]+/)
                    .map((value) => value.trim())
                    .filter(Boolean),
                })
              }
              placeholder={"example.com\n*.wikipedia.org"}
              rows={6}
              className="font-mono text-sm"
              disabled={draft.allow_all_domains}
            />
          </div>

          <div className="space-y-4">
            <h3 className="text-foreground/80 text-[13px] font-bold tracking-widest uppercase">
              {t("pages.agent.tools.browser.advanced", "Advanced")}
            </h3>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="browser-binary-path">
                  {t(
                    "pages.agent.tools.browser.binary_path",
                    "agent-browser binary",
                  )}
                </Label>
                <Input
                  id="browser-binary-path"
                  value={draft.binary_path}
                  onChange={(event) =>
                    update({ binary_path: event.target.value })
                  }
                  placeholder={t(
                    "pages.agent.tools.browser.binary_placeholder",
                    "Auto-detect next to the gateway or on PATH",
                  )}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="browser-executable-path">
                  {t(
                    "pages.agent.tools.browser.browser_path",
                    "Browser executable",
                  )}
                </Label>
                <Input
                  id="browser-executable-path"
                  value={draft.browser_path}
                  onChange={(event) =>
                    update({ browser_path: event.target.value })
                  }
                  placeholder={t(
                    "pages.agent.tools.browser.browser_placeholder",
                    "Auto-detect Edge or Chrome",
                  )}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="browser-max-output">
                  {t(
                    "pages.agent.tools.browser.max_output",
                    "Max page output (characters)",
                  )}
                </Label>
                <Input
                  id="browser-max-output"
                  type="number"
                  min={1000}
                  max={1000000}
                  value={draft.max_output}
                  onChange={(event) =>
                    update({ max_output: Number(event.target.value) || 0 })
                  }
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="browser-timeout">
                  {t(
                    "pages.agent.tools.browser.timeout",
                    "Command timeout (seconds)",
                  )}
                </Label>
                <Input
                  id="browser-timeout"
                  type="number"
                  min={5}
                  max={600}
                  value={draft.timeout_seconds}
                  onChange={(event) =>
                    update({ timeout_seconds: Number(event.target.value) || 0 })
                  }
                />
              </div>
            </div>
          </div>

          <div className="border-border/40 bg-muted/20 rounded-2xl border p-4 text-sm">
            <div className="grid gap-2 sm:grid-cols-2">
              <div>
                <span className="text-muted-foreground">
                  {t("pages.agent.tools.browser.status_binary", "CLI")}:
                </span>{" "}
                {draft.status.binary_found
                  ? draft.status.binary_version ||
                    draft.status.binary_path ||
                    t("pages.agent.tools.browser.found", "found")
                  : t("pages.agent.tools.browser.not_found", "not found")}
              </div>
              <div className="truncate">
                <span className="text-muted-foreground">
                  {t("pages.agent.tools.browser.status_browser", "Browser")}:
                </span>{" "}
                {draft.status.browser_detected
                  ? draft.status.browser_path
                  : t("pages.agent.tools.browser.not_found", "not found")}
              </div>
            </div>
            {draft.status.error && (
              <p className="text-destructive mt-2 text-xs">
                {draft.status.error}
              </p>
            )}
            {!draft.status.binary_found && (
              <p className="text-muted-foreground mt-2 text-xs">
                {t(
                  "pages.agent.tools.browser.install_hint",
                  "Install agent-browser (npm install -g agent-browser) or set its path above. The CLI ships separately from PicoClaw.",
                )}
              </p>
            )}
          </div>

          {!draft.status.browser_detected && (
            <div className="border-amber-500/30 bg-amber-500/5 rounded-2xl border p-4 text-sm">
              <p className="font-medium">
                {t(
                  "pages.agent.tools.browser.no_browser_title",
                  "No system Chrome or Chromium detected",
                )}
              </p>
              <p className="text-muted-foreground mt-1 text-xs">
                {t(
                  "pages.agent.tools.browser.no_browser_description",
                  "The browser feature needs a Chromium-based browser. Download a private Chrome for Testing build (large download) or install Chrome/Chromium yourself.",
                )}
              </p>
              <div className="mt-3 flex flex-wrap items-center gap-3">
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  className="gap-1.5"
                  onClick={() => installMutation.mutate()}
                  disabled={
                    installMutation.isPending ||
                    installQuery.data?.running === true
                  }
                >
                  {t(
                    "pages.agent.tools.browser.download_engine",
                    "Download browser engine",
                  )}
                </Button>
                {(installQuery.data?.running ||
                  installMutation.isPending) && (
                  <span className="text-muted-foreground text-xs">
                    {installQuery.data?.last_line ||
                      t(
                        "pages.agent.tools.browser.downloading",
                        "Downloading…",
                      )}
                  </span>
                )}
                {installQuery.data?.error && (
                  <span className="text-destructive text-xs">
                    {installQuery.data.error}
                  </span>
                )}
                {installQuery.data?.done && !installQuery.data.error && (
                  <span className="text-emerald-600 text-xs">
                    {t(
                      "pages.agent.tools.browser.download_done",
                      "Browser engine installed.",
                    )}
                  </span>
                )}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function SettingRow({
  label,
  description,
  children,
}: {
  label: string
  description: string
  children: ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-6 p-5">
      <div className="space-y-1">
        <p className="text-sm font-medium">{label}</p>
        <p className="text-muted-foreground text-xs">{description}</p>
      </div>
      {children}
    </div>
  )
}
