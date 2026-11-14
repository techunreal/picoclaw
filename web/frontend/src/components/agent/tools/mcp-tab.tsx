import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  IconAlertTriangle,
  IconCheck,
  IconPencil,
  IconPlus,
  IconTrash,
  IconX,
} from "@tabler/icons-react"
import { useEffect, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  type MCPDiscoverySettings,
  type MCPServer,
  type MCPServerPayload,
  type MCPTestResult,
  type MCPTransport,
  deleteMCPServer,
  getMCPServers,
  saveMCPServer,
  testMCPServer,
  updateMCPSettings,
} from "@/api/mcp"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { showSaveSuccessOrRestartToast } from "@/lib/restart-required"
import { refreshGatewayState } from "@/store/gateway"

const queryKey = ["tools", "mcp-servers"] as const

interface EditorState {
  name: string
  payload: MCPServerPayload
}

function linesToMap(value: string, separator: "=" | ":"): Record<string, string> {
  const result: Record<string, string> = {}
  for (const line of value.split("\n")) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const index = trimmed.indexOf(separator)
    if (index <= 0) continue
    const key = trimmed.slice(0, index).trim()
    const entryValue = trimmed.slice(index + 1).trim()
    if (key) {
      result[key] = entryValue
    }
  }
  return result
}

function mapToLines(values: Record<string, string>, separator: "=" | ":"): string {
  return Object.entries(values)
    .map(([key, value]) => `${key}${separator === "=" ? "=" : ": "}${value}`)
    .join("\n")
}

function toPayload(server: MCPServer): MCPServerPayload {
  return {
    enabled: server.enabled,
    deferred: server.deferred ?? null,
    type: server.type,
    command: server.command,
    args: [...(server.args ?? [])],
    env: { ...server.env },
    env_file: server.env_file,
    url: server.url,
    headers: { ...server.headers },
  }
}

function emptyEditor(): EditorState {
  return {
    name: "",
    payload: {
      enabled: true,
      deferred: null,
      type: "stdio",
      command: "",
      args: [],
      env: {},
      env_file: "",
      url: "",
      headers: {},
    },
  }
}

export function MCPTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editor, setEditor] = useState<EditorState | null>(null)
  const [argsText, setArgsText] = useState("")
  const [envText, setEnvText] = useState("")
  const [headersText, setHeadersText] = useState("")
  const [deleteTarget, setDeleteTarget] = useState<MCPServer | null>(null)
  const [testResults, setTestResults] = useState<Record<string, MCPTestResult>>(
    {},
  )

  const query = useQuery({ queryKey, queryFn: getMCPServers })

  const servers = useMemo(() => query.data?.servers ?? [], [query.data?.servers])
  const discovery: MCPDiscoverySettings | undefined = query.data?.discovery

  useEffect(() => {
    if (!editor) return
    setArgsText(editor.payload.args.join("\n"))
    setEnvText(mapToLines(editor.payload.env, "="))
    setHeadersText(mapToLines(editor.payload.headers, ":"))
  }, [editor])

  const persistSettings = async (next: {
    enabled: boolean
    discovery?: MCPDiscoverySettings
  }) => {
    const updated = await updateMCPSettings(next)
    queryClient.setQueryData(queryKey, updated)
    const gateway = await refreshGatewayState({ force: true })
    showSaveSuccessOrRestartToast(
      t,
      t("pages.agent.tools.mcp.saved", "MCP settings saved."),
      t("pages.agent.tools.mcp.title", "MCP"),
      gateway?.restartRequired === true,
    )
  }

  const settingsMutation = useMutation({
    mutationFn: persistSettings,
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.tools.mcp.save_error", "Failed to save MCP settings."),
      ),
  })

  const saveMutation = useMutation({
    mutationFn: ({ name, payload }: { name: string; payload: MCPServerPayload }) =>
      saveMCPServer(name, payload),
    onSuccess: async (updated) => {
      queryClient.setQueryData(queryKey, updated)
      setEditor(null)
      const gateway = await refreshGatewayState({ force: true })
      showSaveSuccessOrRestartToast(
        t,
        t("pages.agent.tools.mcp.saved", "MCP settings saved."),
        t("pages.agent.tools.mcp.title", "MCP"),
        gateway?.restartRequired === true,
      )
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.tools.mcp.save_error", "Failed to save MCP settings."),
      ),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteMCPServer,
    onSuccess: async (updated) => {
      queryClient.setQueryData(queryKey, updated)
      setDeleteTarget(null)
      const gateway = await refreshGatewayState({ force: true })
      showSaveSuccessOrRestartToast(
        t,
        t("pages.agent.tools.mcp.deleted", "MCP server deleted."),
        t("pages.agent.tools.mcp.title", "MCP"),
        gateway?.restartRequired === true,
      )
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t("pages.agent.tools.mcp.delete_error", "Failed to delete the server."),
      ),
  })

  const testMutation = useMutation({
    mutationFn: testMCPServer,
    onSuccess: (result, name) => {
      setTestResults((current) => ({ ...current, [name]: result }))
      if (result.ok) {
        toast.success(
          t("pages.agent.tools.mcp.test_ok", {
            count: result.tool_count,
            defaultValue: "Connected: {{count}} tool(s) discovered.",
          }),
        )
      } else {
        toast.error(result.error || t("pages.agent.tools.mcp.test_failed", "Connection failed."))
      }
    },
    onError: (error, name) => {
      setTestResults((current) => ({
        ...current,
        [name]: { ok: false, tool_count: 0, error: error.message },
      }))
      toast.error(error.message)
    },
  })

  const openCreate = () => setEditor(emptyEditor())
  const openEdit = (server: MCPServer) =>
    setEditor({ name: server.name, payload: toPayload(server) })

  const handleSave = () => {
    if (!editor) return
    const name = editor.name.trim()
    if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$/.test(name)) {
      toast.error(
        t(
          "pages.agent.tools.mcp.invalid_name",
          "Name must start with a letter or number and use only letters, numbers, dot, underscore or hyphen.",
        ),
      )
      return
    }
    const args = argsText
      .split("\n")
      .map((value) => value.trim())
      .filter(Boolean)
    const payload: MCPServerPayload = {
      ...editor.payload,
      args,
      env: linesToMap(envText, "="),
      headers: linesToMap(headersText, ":"),
      command: editor.payload.type === "stdio" ? editor.payload.command : "",
      url: editor.payload.type === "stdio" ? "" : editor.payload.url,
    }
    saveMutation.mutate({ name, payload })
  }

  const patchEditor = (patch: Partial<MCPServerPayload>) => {
    setEditor((current) =>
      current ? { ...current, payload: { ...current.payload, ...patch } } : current,
    )
  }

  return (
    <div className="animate-in fade-in slide-in-from-bottom-2 space-y-8 pt-2 duration-500">
      {query.isError ? (
        <div className="py-20 text-center">
          <p className="text-destructive font-medium">
            {t("pages.agent.tools.mcp.load_error", "Failed to load MCP servers.")}
          </p>
        </div>
      ) : query.isLoading || !query.data ? (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full rounded-2xl" />
          <Skeleton className="h-48 w-full rounded-2xl" />
        </div>
      ) : (
        <>
          <div className="flex flex-col gap-6 sm:flex-row sm:items-start sm:justify-between">
            <div className="max-w-xl space-y-3">
              <h1 className="text-foreground/90 text-2xl font-semibold tracking-tight">
                {t("pages.agent.tools.mcp.title", "MCP Servers")}
              </h1>
              <p className="text-muted-foreground/80 text-[14px] leading-relaxed">
                {t(
                  "pages.agent.tools.mcp.description",
                  "Connect Model Context Protocol servers over stdio, HTTP or SSE. Tools become available to the agent after a gateway restart.",
                )}
              </p>
            </div>
            <Button type="button" className="shrink-0 gap-1.5" onClick={openCreate}>
              <IconPlus className="size-4" />
              {t("pages.agent.tools.mcp.add_server", "Add server")}
            </Button>
          </div>

          <div className="bg-card border-border/40 divide-border/40 divide-y overflow-hidden rounded-2xl border shadow-sm">
            <div className="flex items-center justify-between gap-6 p-5">
              <div className="space-y-1">
                <p className="text-sm font-medium">
                  {t("pages.agent.tools.mcp.enable", "Enable MCP")}
                </p>
                <p className="text-muted-foreground text-xs">
                  {t(
                    "pages.agent.tools.mcp.enable_description",
                    "Load configured MCP servers when the gateway starts.",
                  )}
                </p>
              </div>
              <Switch
                checked={query.data.enabled}
                onCheckedChange={(checked) =>
                  settingsMutation.mutate({
                    enabled: checked,
                    discovery,
                  })
                }
              />
            </div>
            <div className="flex items-center justify-between gap-6 p-5">
              <div className="space-y-1">
                <p className="text-sm font-medium">
                  {t("pages.agent.tools.mcp.discovery", "Tool discovery")}
                </p>
                <p className="text-muted-foreground text-xs">
                  {t(
                    "pages.agent.tools.mcp.discovery_description",
                    "Keep large MCP tool sets hidden until the agent searches for them.",
                  )}
                </p>
              </div>
              <Switch
                checked={query.data.discovery.enabled}
                onCheckedChange={(checked) =>
                  settingsMutation.mutate({
                    enabled: query.data.enabled,
                    discovery: { ...query.data.discovery, enabled: checked },
                  })
                }
              />
            </div>
          </div>

          {servers.length === 0 ? (
            <div className="border-border/60 rounded-2xl border border-dashed p-10 text-center">
              <p className="font-medium">
                {t("pages.agent.tools.mcp.empty_title", "No MCP servers yet")}
              </p>
              <p className="text-muted-foreground mt-1 text-sm">
                {t(
                  "pages.agent.tools.mcp.empty_description",
                  "Add a stdio command or an HTTP/SSE endpoint to give the agent more tools.",
                )}
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {servers.map((server) => {
                const result = testResults[server.name]
                return (
                  <div
                    key={server.name}
                    className="bg-card flex flex-col gap-3 rounded-2xl border p-4 shadow-xs"
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="truncate text-[15px] font-medium">
                            {server.name}
                          </h3>
                          <Badge variant="secondary">{server.type}</Badge>
                          {server.deferred === true && (
                            <Badge variant="outline">
                              {t("pages.agent.tools.mcp.deferred", "Deferred")}
                            </Badge>
                          )}
                        </div>
                        <p className="text-muted-foreground mt-1 truncate font-mono text-xs">
                          {server.type === "stdio" ? server.command : server.url}
                        </p>
                      </div>
                      <div className="flex items-center gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={() => testMutation.mutate(server.name)}
                          disabled={testMutation.isPending}
                        >
                          {testMutation.isPending &&
                          testMutation.variables === server.name
                            ? t("pages.agent.tools.mcp.testing", "Testing…")
                            : t("pages.agent.tools.mcp.test", "Test")}
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="text-muted-foreground hover:text-foreground h-8 w-8"
                          onClick={() => openEdit(server)}
                          aria-label={t("pages.agent.tools.mcp.edit", "Edit")}
                        >
                          <IconPencil className="size-4" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="text-muted-foreground hover:text-destructive h-8 w-8"
                          onClick={() => setDeleteTarget(server)}
                          aria-label={t("pages.agent.tools.mcp.delete", "Delete")}
                        >
                          <IconTrash className="size-4" />
                        </Button>
                        <Switch
                          checked={server.enabled}
                          onCheckedChange={(checked) =>
                            saveMutation.mutate({
                              name: server.name,
                              payload: { ...toPayload(server), enabled: checked },
                            })
                          }
                        />
                      </div>
                    </div>

                    {result && (
                      <div
                        className={`rounded-lg border p-3 text-xs ${
                          result.ok
                            ? "border-emerald-500/30 bg-emerald-500/5"
                            : "border-destructive/30 bg-destructive/5 text-destructive"
                        }`}
                      >
                        {result.ok ? (
                          <div className="flex items-start gap-2">
                            <IconCheck className="mt-0.5 size-4 shrink-0" />
                            <div>
                              <p>
                                {t("pages.agent.tools.mcp.test_ok", {
                                  count: result.tool_count,
                                  defaultValue:
                                    "Connected: {{count}} tool(s) discovered.",
                                })}
                              </p>
                              {result.tools && result.tools.length > 0 && (
                                <p className="text-muted-foreground mt-1 font-mono">
                                  {result.tools.join(", ")}
                                </p>
                              )}
                            </div>
                          </div>
                        ) : (
                          <div className="flex items-start gap-2">
                            <IconX className="mt-0.5 size-4 shrink-0" />
                            <span>{result.error}</span>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}

          <div className="border-border/60 bg-muted/30 text-muted-foreground flex items-start gap-2 rounded-xl border p-3 text-xs">
            <IconAlertTriangle className="mt-0.5 size-4 shrink-0" />
            <span>
              {t(
                "pages.agent.tools.mcp.secrets_warning",
                "Environment values and headers are stored in config.json in plain text. MCP servers can run local commands; only add servers you trust.",
              )}
            </span>
          </div>
        </>
      )}

      <Dialog open={editor !== null} onOpenChange={(open) => !open && setEditor(null)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>
              {editor?.name
                ? t("pages.agent.tools.mcp.edit_title", "Edit MCP server")
                : t("pages.agent.tools.mcp.create_title", "Add MCP server")}
            </DialogTitle>
            <DialogDescription>
              {t(
                "pages.agent.tools.mcp.dialog_description",
                "Configure how the gateway connects to this MCP server.",
              )}
            </DialogDescription>
          </DialogHeader>

          {editor && (
            <div className="grid gap-4 py-2">
              <div className="grid gap-2">
                <Label htmlFor="mcp-name">
                  {t("pages.agent.tools.mcp.name", "Name")}
                </Label>
                <Input
                  id="mcp-name"
                  value={editor.name}
                  onChange={(event) =>
                    setEditor({ ...editor, name: event.target.value })
                  }
                  placeholder="filesystem"
                  disabled={Boolean(servers.find((item) => item.name === editor.name))}
                />
              </div>

              <div className="grid gap-2">
                <Label>{t("pages.agent.tools.mcp.transport", "Transport")}</Label>
                <Select
                  value={editor.payload.type}
                  onValueChange={(value) =>
                    patchEditor({ type: value as MCPTransport })
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="stdio">stdio</SelectItem>
                    <SelectItem value="http">HTTP</SelectItem>
                    <SelectItem value="sse">SSE</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {editor.payload.type === "stdio" ? (
                <>
                  <div className="grid gap-2">
                    <Label htmlFor="mcp-command">
                      {t("pages.agent.tools.mcp.command", "Command")}
                    </Label>
                    <Input
                      id="mcp-command"
                      value={editor.payload.command}
                      onChange={(event) =>
                        patchEditor({ command: event.target.value })
                      }
                      placeholder="npx"
                      className="font-mono"
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="mcp-args">
                      {t("pages.agent.tools.mcp.args", "Arguments (one per line)")}
                    </Label>
                    <Textarea
                      id="mcp-args"
                      value={argsText}
                      onChange={(event) => setArgsText(event.target.value)}
                      placeholder={"-y\n@modelcontextprotocol/server-filesystem\n."}
                      rows={4}
                      className="font-mono text-sm"
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="mcp-env-file">
                      {t("pages.agent.tools.mcp.env_file", "Environment file")}
                    </Label>
                    <Input
                      id="mcp-env-file"
                      value={editor.payload.env_file}
                      onChange={(event) =>
                        patchEditor({ env_file: event.target.value })
                      }
                      placeholder=".env.mcp"
                      className="font-mono"
                    />
                  </div>
                </>
              ) : (
                <>
                  <div className="grid gap-2">
                    <Label htmlFor="mcp-url">
                      {t("pages.agent.tools.mcp.url", "URL")}
                    </Label>
                    <Input
                      id="mcp-url"
                      value={editor.payload.url}
                      onChange={(event) => patchEditor({ url: event.target.value })}
                      placeholder="http://127.0.0.1:8080/mcp"
                      className="font-mono"
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label htmlFor="mcp-headers">
                      {t("pages.agent.tools.mcp.headers", "Headers (Key: Value)")}
                    </Label>
                    <Textarea
                      id="mcp-headers"
                      value={headersText}
                      onChange={(event) => setHeadersText(event.target.value)}
                      placeholder={"Authorization: Bearer ..."}
                      rows={3}
                      className="font-mono text-sm"
                    />
                  </div>
                </>
              )}

              <div className="grid gap-2">
                <Label htmlFor="mcp-env">
                  {t("pages.agent.tools.mcp.env", "Environment (KEY=value)")}
                </Label>
                <Textarea
                  id="mcp-env"
                  value={envText}
                  onChange={(event) => setEnvText(event.target.value)}
                  placeholder="API_TOKEN=..."
                  rows={3}
                  className="font-mono text-sm"
                />
              </div>

              <div className="grid gap-2">
                <Label>{t("pages.agent.tools.mcp.deferred_title", "Discovery")}</Label>
                <Select
                  value={
                    editor.payload.deferred === true
                      ? "on"
                      : editor.payload.deferred === false
                        ? "off"
                        : "inherit"
                  }
                  onValueChange={(value) =>
                    patchEditor({
                      deferred:
                        value === "on" ? true : value === "off" ? false : null,
                    })
                  }
                >
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="inherit">
                      {t("pages.agent.tools.mcp.deferred_inherit", "Use global setting")}
                    </SelectItem>
                    <SelectItem value="on">
                      {t("pages.agent.tools.mcp.deferred_on", "Deferred (search first)")}
                    </SelectItem>
                    <SelectItem value="off">
                      {t("pages.agent.tools.mcp.deferred_off", "Load tools immediately")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center justify-between gap-4 rounded-xl border p-3">
                <span className="text-sm font-medium">
                  {t("pages.agent.tools.mcp.enabled", "Enabled")}
                </span>
                <Switch
                  checked={editor.payload.enabled}
                  onCheckedChange={(checked) => patchEditor({ enabled: checked })}
                />
              </div>
            </div>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setEditor(null)}
              disabled={saveMutation.isPending}
            >
              {t("pages.agent.tools.mcp.cancel", "Cancel")}
            </Button>
            <Button
              type="button"
              onClick={handleSave}
              disabled={saveMutation.isPending}
            >
              {saveMutation.isPending
                ? t("pages.agent.tools.mcp.saving", "Saving…")
                : t("pages.agent.tools.mcp.save", "Save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("pages.agent.tools.mcp.delete_title", "Delete MCP server?")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("pages.agent.tools.mcp.delete_description", {
                name: deleteTarget?.name ?? "",
                defaultValue: "\"{{name}}\" will be removed from the configuration.",
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>
              {t("pages.agent.tools.mcp.cancel", "Cancel")}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={(event) => {
                event.preventDefault()
                if (deleteTarget) deleteMutation.mutate(deleteTarget.name)
              }}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending
                ? t("pages.agent.tools.mcp.deleting", "Deleting…")
                : t("pages.agent.tools.mcp.delete", "Delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
