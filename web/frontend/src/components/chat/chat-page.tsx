import { IconCheck, IconChevronDown, IconGhost, IconPlus } from "@tabler/icons-react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useAtom, useAtomValue, useSetAtom } from "jotai"
import {
  type ChangeEvent,
  type ClipboardEvent,
  type DragEvent,
  useEffect,
  useRef,
  useState,
} from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import { agentModelPrimary, getAgents } from "@/api/agents"
import { AgentAvatar } from "@/components/agent-avatar"
import { AssistantMessage } from "@/components/chat/assistant-message"
import {
  ChatComposer,
  type ChatInputDisabledReason,
} from "@/components/chat/chat-composer"
import { ChatEmptyState } from "@/components/chat/chat-empty-state"
import { ChatMemoryToggle } from "@/components/chat/chat-memory-toggle"
import { ModelSelector } from "@/components/chat/model-selector"
import { TypingIndicator } from "@/components/chat/typing-indicator"
import { UserMessage } from "@/components/chat/user-message"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  CHAT_ATTACHMENT_ACCEPT,
  MAX_CHAT_ATTACHMENTS,
  buildChatAttachments,
  getTransferredFiles,
  hasFileTransfer,
} from "@/features/chat/attachment-input"
import { isIncognitoSession } from "@/features/chat/state"
import { getSessions } from "@/api/sessions"
import { useChatModels } from "@/hooks/use-chat-models"
import { useGateway } from "@/hooks/use-gateway"
import { usePicoChat } from "@/hooks/use-pico-chat"
import type { AssistantDetailVisibility } from "@/store/chat"
import type { ConnectionState } from "@/store/chat"
import type { ChatAttachment } from "@/store/chat"
import {
  assistantDetailVisibilityAtom,
  chatAtom,
  pendingAgentChatAtom,
  pendingSessionAtom,
  shouldShowAssistantMessage,
} from "@/store/chat"
import type { GatewayState } from "@/store/gateway"

function resolveChatInputDisabledReason({
  hasDefaultModel,
  connectionState,
  gatewayState,
}: {
  hasDefaultModel: boolean
  connectionState: ConnectionState
  gatewayState: GatewayState
}): ChatInputDisabledReason | null {
  if (gatewayState === "unknown") {
    return "gatewayUnknown"
  }

  if (gatewayState === "starting") {
    return "gatewayStarting"
  }

  if (gatewayState === "restarting") {
    return "gatewayRestarting"
  }

  if (gatewayState === "stopping") {
    return "gatewayStopping"
  }

  if (gatewayState === "stopped") {
    return "gatewayStopped"
  }

  if (gatewayState === "error") {
    return "gatewayError"
  }

  if (connectionState === "connecting") {
    return "websocketConnecting"
  }

  if (connectionState === "error") {
    return "websocketError"
  }

  if (connectionState === "disconnected") {
    return "websocketDisconnected"
  }

  if (!hasDefaultModel) {
    return "noDefaultModel"
  }

  return null
}

export function ChatPage() {
  const { t } = useTranslation()
  const scrollRef = useRef<HTMLDivElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const dragDepthRef = useRef(0)
  const [isAtBottom, setIsAtBottom] = useState(true)
  const [hasScrolled, setHasScrolled] = useState(false)
  const [input, setInput] = useState("")
  const [attachments, setAttachments] = useState<ChatAttachment[]>([])
  const [isDragActive, setIsDragActive] = useState(false)
  const [assistantDetailVisibility, setAssistantDetailVisibility] = useAtom(
    assistantDetailVisibilityAtom,
  )

  const assistantDetailVisibilityOptions: Array<{
    value: AssistantDetailVisibility
    label: string
  }> = [
    { value: "none", label: t("chat.assistantDetailVisibility.none") },
    { value: "thought", label: t("chat.assistantDetailVisibility.thought") },
    {
      value: "tool_calls",
      label: t("chat.assistantDetailVisibility.toolCalls"),
    },
    { value: "all", label: t("chat.assistantDetailVisibility.all") },
  ]

  const activeSessionIdForAgent = useAtomValue(chatAtom).activeSessionId
  const sessionsQuery = useQuery({
    queryKey: ["sidebar-sessions"],
    queryFn: () => getSessions(0, 50),
    refetchInterval: 3000,
  })
  const activeSessionAgent = sessionsQuery.data?.find(
    (session) => session.id === activeSessionIdForAgent,
  )?.agent_id
  const activeSessionRunning =
    sessionsQuery.data?.find((session) => session.id === activeSessionIdForAgent)
      ?.running === true
  const [selectedAgentId, setSelectedAgentId] = useState("main")
  useEffect(() => {
    if (activeSessionAgent) setSelectedAgentId(activeSessionAgent)
  }, [activeSessionAgent])
  const chatAgentId = activeSessionAgent || selectedAgentId

  const {
    messages,
    connectionState,
    isTyping,
    activeSessionId,
    contextUsage,
    sendMessage,
    stopActiveTurn,
    switchSession,
    newChat,
    newIncognitoChat,
  } = usePicoChat(chatAgentId)
  const isIncognito = isIncognitoSession(activeSessionId)

  const queryClient = useQueryClient()
  const agentsQuery = useQuery({
    queryKey: ["sidebar-agents"],
    queryFn: getAgents,
  })
  const chatAgent = agentsQuery.data?.agents.find(
    (agent) => agent.id === chatAgentId,
  )
  const chatAgentName =
    chatAgent?.name?.trim() || (chatAgentId === "main" ? "Pico" : chatAgentId)

  const agentOptions = (() => {
    const list = agentsQuery.data?.agents ?? []
    const options = list.map((agent) => ({
      id: agent.id,
      name:
        agent.name?.trim() || (agent.id === "main" ? "Pico" : agent.id),
      avatar: agent.avatar,
    }))
    if (!options.some((agent) => agent.id === "main")) {
      options.unshift({ id: "main", name: "Pico", avatar: undefined })
    }
    return options
  })()

  const handleAgentChange = (nextAgentId: string) => {
    setSelectedAgentId(nextAgentId)
    const latest = sessionsQuery.data?.find(
      (session) => (session.agent_id || "main") === nextAgentId,
    )
    if (latest) {
      void switchSession(latest.id, nextAgentId)
      return
    }
    void newChat(nextAgentId)
  }

  const pendingSessionId = useAtomValue(pendingSessionAtom)
  const setPendingSessionId = useSetAtom(pendingSessionAtom)
  useEffect(() => {
    if (!pendingSessionId) return
    const agent = sessionsQuery.data?.find(
      (session) => session.id === pendingSessionId,
    )?.agent_id
    void switchSession(pendingSessionId, agent)
    setPendingSessionId(null)
  }, [pendingSessionId, switchSession, setPendingSessionId, sessionsQuery.data])

  const pendingAgentChat = useAtomValue(pendingAgentChatAtom)
  const setPendingAgentChat = useSetAtom(pendingAgentChatAtom)
  useEffect(() => {
    if (!pendingAgentChat) return
    setSelectedAgentId(pendingAgentChat)
    void newChat(pendingAgentChat)
    setPendingAgentChat(null)
  }, [pendingAgentChat, newChat, setPendingAgentChat])

  const { state: gwState } = useGateway()
  const isGatewayRunning = gwState === "running"

  const {
    defaultModelName,
    hasAvailableModels,
    apiKeyModels,
    oauthModels,
    localModels,
    settingDefault,
    settingAgentModel,
    handleSetDefault,
    handleSetAgentModel,
  } = useChatModels({ isConnected: isGatewayRunning })

  const isMainAgent = chatAgentId === "main" || !chatAgent
  const globalDefaultModel = defaultModelName
  const chatModelName = isMainAgent
    ? globalDefaultModel
    : chatAgent?.effective_model ||
      agentModelPrimary(chatAgent?.model) ||
      globalDefaultModel
  const chatModelSource = isMainAgent
    ? "global"
    : chatAgent?.model_source ||
      (agentModelPrimary(chatAgent?.model) ? "override" : "global")
  const chatModelInherited = chatModelSource === "global"
  const chatModelFromFrontmatter = chatModelSource === "frontmatter"

  const handleSetChatModel = async (modelName: string) => {
    if (modelName === "__pin__") {
      if (!chatModelName) return
      const result = await handleSetAgentModel(chatAgentId, chatModelName)
      if (result) {
        await queryClient.invalidateQueries({ queryKey: ["sidebar-agents"] })
      }
      return
    }
    if (modelName === "__global__") {
      const result = await handleSetAgentModel(chatAgentId, null)
      if (result) {
        await queryClient.invalidateQueries({ queryKey: ["sidebar-agents"] })
      }
      return
    }
    if (modelName === chatModelName) {
      // An inheriting agent can pin the current global model as its own
      // override. Pico itself is always global, so selecting the same model is
      // a no-op there.
      if (!(chatModelInherited && !isMainAgent)) return
    }
    if (isMainAgent) {
      await handleSetDefault(modelName)
      await queryClient.invalidateQueries({ queryKey: ["sidebar-agents"] })
      return
    }
    const result = await handleSetAgentModel(chatAgentId, modelName)
    if (result) {
      await queryClient.invalidateQueries({ queryKey: ["sidebar-agents"] })
    }
  }

  const hasDefaultModel = Boolean(defaultModelName)
  const inputDisabledReason = resolveChatInputDisabledReason({
    hasDefaultModel,
    connectionState,
    gatewayState: gwState,
  })
  const canInput = inputDisabledReason === null

  const syncScrollState = (element: HTMLDivElement) => {
    const { clientHeight, scrollHeight, scrollTop } = element
    setHasScrolled(scrollTop > 0)
    setIsAtBottom(scrollHeight - scrollTop <= clientHeight + 10)
  }

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    syncScrollState(e.currentTarget)
  }

  useEffect(() => {
    if (scrollRef.current) {
      if (isAtBottom) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight
      }
      syncScrollState(scrollRef.current)
    }
  }, [messages, isTyping, isAtBottom])

  const handleSend = () => {
    if ((!input.trim() && attachments.length === 0) || !canInput) return
    if (
      sendMessage({
        content: input,
        attachments,
      })
    ) {
      setInput("")
      setAttachments([])
    }
  }

  const handleAddFiles = () => {
    if (!canInput) return
    fileInputRef.current?.click()
  }

  const handleRemoveAttachment = (index: number) => {
    setAttachments((prev) => prev.filter((_, itemIndex) => itemIndex !== index))
  }

  const appendFiles = async (files: readonly File[]) => {
    if (!canInput || files.length === 0) {
      return
    }

    const remainingSlots = Math.max(0, MAX_CHAT_ATTACHMENTS - attachments.length)
    if (remainingSlots === 0) {
      toast.error(t("chat.attachmentLimit", { count: MAX_CHAT_ATTACHMENTS }))
      return
    }
    if (files.length > remainingSlots) {
      toast.error(t("chat.attachmentLimit", { count: MAX_CHAT_ATTACHMENTS }))
    }

    const nextAttachments = await buildChatAttachments(
      files.slice(0, remainingSlots),
      t,
    )
    if (nextAttachments.length === 0) {
      return
    }

    setAttachments((prev) =>
      [...prev, ...nextAttachments].slice(0, MAX_CHAT_ATTACHMENTS),
    )
  }

  const handleFileSelection = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? [])
    event.target.value = ""

    if (files.length === 0) {
      return
    }

    await appendFiles(files)
  }

  const resetDragState = () => {
    dragDepthRef.current = 0
    setIsDragActive(false)
  }

  const handleComposerPaste = async (
    event: ClipboardEvent<HTMLTextAreaElement>,
  ) => {
    const files = getTransferredFiles(event.clipboardData)
    if (files.length === 0) {
      return
    }

    await appendFiles(files)
  }

  const handleComposerDragEnter = (event: DragEvent<HTMLDivElement>) => {
    if (!hasFileTransfer(event.dataTransfer)) {
      return
    }

    event.preventDefault()
    if (!canInput) {
      return
    }
    dragDepthRef.current += 1
    setIsDragActive(true)
  }

  const handleComposerDragLeave = (event: DragEvent<HTMLDivElement>) => {
    if (!hasFileTransfer(event.dataTransfer)) {
      return
    }

    event.preventDefault()
    if (!canInput) {
      resetDragState()
      return
    }
    dragDepthRef.current = Math.max(0, dragDepthRef.current - 1)
    if (dragDepthRef.current === 0) {
      setIsDragActive(false)
    }
  }

  const handleComposerDragOver = (event: DragEvent<HTMLDivElement>) => {
    if (!hasFileTransfer(event.dataTransfer)) {
      return
    }

    event.preventDefault()
    event.dataTransfer.dropEffect = canInput ? "copy" : "none"
  }

  const handleComposerDrop = async (event: DragEvent<HTMLDivElement>) => {
    if (!hasFileTransfer(event.dataTransfer)) {
      return
    }

    event.preventDefault()
    const files = getTransferredFiles(event.dataTransfer)
    resetDragState()

    if (!canInput || files.length === 0) {
      return
    }

    await appendFiles(files)
  }

  const canSubmit =
    canInput && (Boolean(input.trim()) || attachments.length > 0)

  return (
    <div className="bg-background/95 flex h-full flex-col">
      <PageHeader
        title={
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className="hover:bg-accent flex items-center gap-2 rounded-lg px-1.5 py-1"
              >
                <AgentAvatar
                  agentId={chatAgentId}
                  avatar={chatAgent?.avatar}
                  name={chatAgentName}
                  fallback={chatAgent ? undefined : "🦞"}
                  size="md"
                  className="border-border/50 border"
                />
                <span>{chatAgentName}</span>
                <IconChevronDown className="size-4 opacity-60" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="min-w-56">
              {agentOptions.map((option) => (
                <DropdownMenuItem
                  key={option.id}
                  onSelect={() => handleAgentChange(option.id)}
                  className="gap-2"
                >
                  <AgentAvatar
                    agentId={option.id}
                    avatar={option.avatar}
                    name={option.name}
                    fallback={option.id === "main" ? "🦞" : undefined}
                    size="sm"
                  />
                  <span>{option.name}</span>
                  {option.id === chatAgentId && (
                    <IconCheck className="ml-auto size-4" />
                  )}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        }
        className={`transition-shadow ${
          hasScrolled ? "shadow-xs" : "shadow-none"
        }`}
        titleExtra={
          hasAvailableModels && (
            <ModelSelector
              defaultModelName={chatModelName || defaultModelName}
              apiKeyModels={apiKeyModels}
              oauthModels={oauthModels}
              localModels={localModels}
              badgeLabel={
                chatModelInherited
                  ? t("models.globalBadge")
                  : chatModelFromFrontmatter
                    ? t("models.frontmatterBadge")
                    : undefined
              }
              badgeTitle={
                chatModelInherited
                  ? t("models.globalDefaultHint", {
                      model: globalDefaultModel,
                    })
                  : chatModelFromFrontmatter
                    ? t("models.frontmatterHint")
                    : undefined
              }
              globalDefaultModelName={globalDefaultModel}
              canUseGlobalDefault={
                !isMainAgent && chatModelSource === "override"
              }
              canPinCurrentModel={!isMainAgent && chatModelInherited}
              disabled={
                settingDefault || settingAgentModel || chatModelFromFrontmatter
              }
              onValueChange={handleSetChatModel}
            />
          )
        }
      >
        <div className="border-border/60 hidden items-center gap-2 rounded-lg border px-3 py-1.5 sm:flex">
          <span className="text-muted-foreground text-sm">
            {t("chat.showAssistantDetails")}
          </span>
          <Select
            value={assistantDetailVisibility}
            onValueChange={(value) =>
              setAssistantDetailVisibility(value as AssistantDetailVisibility)
            }
          >
            <SelectTrigger
              size="sm"
              aria-label={t("chat.showAssistantDetails")}
              className="text-muted-foreground hover:text-foreground focus-visible:border-input h-8 min-w-[104px] bg-transparent shadow-none focus-visible:ring-0"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              {assistantDetailVisibilityOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <ChatMemoryToggle sessionId={activeSessionId} />

        {isIncognito && (
          <span
            className="border-border/60 text-muted-foreground hidden items-center gap-1 rounded-full border px-2 py-0.5 text-xs sm:inline-flex"
            title={t("chat.incognitoHint")}
          >
            <IconGhost className="size-3.5" />
            {t("chat.incognitoBadge")}
          </span>
        )}

        <div className="hidden items-center gap-2 sm:flex">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void newChat(selectedAgentId)}
            className="h-9 gap-2"
          >
            <IconPlus className="size-4" />
            <span className="hidden sm:inline">{t("chat.newChat")}</span>
          </Button>

          <Button
            variant="outline"
            size="sm"
            onClick={() => void newIncognitoChat(selectedAgentId)}
            title={t("chat.newIncognitoChat")}
            aria-label={t("chat.newIncognitoChat")}
            className="h-9 gap-2"
          >
            <IconGhost className="size-4" />
            <span className="hidden sm:inline">{t("chat.incognitoBadge")}</span>
          </Button>
        </div>

        <div className="sm:hidden">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="secondary"
                size="sm"
                className="h-9 w-9 p-0"
                aria-label={t("chat.newChat")}
              >
                <IconPlus className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-48">
              <DropdownMenuItem
                onSelect={() => void newChat(selectedAgentId)}
                className="gap-2"
              >
                <IconPlus className="size-4" />
                {t("chat.newChat")}
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() => void newIncognitoChat(selectedAgentId)}
                className="gap-2"
              >
                <IconGhost className="size-4" />
                {t("chat.newIncognitoChat")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </PageHeader>

      <div
        ref={scrollRef}
        onScroll={handleScroll}
        className="min-h-0 flex-1 [scrollbar-gutter:stable] overflow-y-auto px-4 py-6 md:px-8 lg:px-24 xl:px-48"
      >
        <div className="mx-auto flex w-full max-w-250 flex-col gap-8 pb-8">
          {messages.length === 0 && !isTyping && (
            <ChatEmptyState
              hasAvailableModels={hasAvailableModels}
              defaultModelName={defaultModelName}
              isConnected={isGatewayRunning}
            />
          )}

          {messages.map((msg) => {
            if (
              !shouldShowAssistantMessage(assistantDetailVisibility, msg.kind)
            ) {
              return null
            }

            return (
              <div key={msg.id} className="flex w-full">
                {msg.role === "assistant" ? (
                  <AssistantMessage
                    content={msg.content}
                    attachments={msg.attachments}
                    kind={msg.kind}
                    modelName={msg.modelName}
                    toolCalls={msg.toolCalls}
                    timestamp={msg.timestamp}
                  />
                ) : (
                  <UserMessage
                    content={msg.content}
                    attachments={msg.attachments}
                    timestamp={msg.timestamp}
                  />
                )}
              </div>
            )
          })}

          {isTyping && <TypingIndicator />}
        </div>
      </div>

      <input
        ref={fileInputRef}
        type="file"
        accept={CHAT_ATTACHMENT_ACCEPT}
        multiple
        className="hidden"
        onChange={handleFileSelection}
      />

      <ChatComposer
        input={input}
        attachments={attachments}
        onInputChange={setInput}
        onAddFiles={handleAddFiles}
        onPaste={handleComposerPaste}
        onDragEnter={handleComposerDragEnter}
        onDragLeave={handleComposerDragLeave}
        onDragOver={handleComposerDragOver}
        onDrop={handleComposerDrop}
        onRemoveAttachment={handleRemoveAttachment}
        onSend={handleSend}
        onStop={() => {
          stopActiveTurn()
        }}
        isTyping={isTyping}
        turnRunning={activeSessionRunning}
        onContextDetail={() => {
          if (sendMessage({ content: "/context", attachments: [] })) {
            setInput("")
          }
        }}
        inputDisabledReason={inputDisabledReason}
        canSend={canSubmit}
        isDragActive={isDragActive}
        contextUsage={contextUsage}
      />
    </div>
  )
}
