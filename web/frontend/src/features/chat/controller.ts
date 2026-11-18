import { getDefaultStore } from "jotai"
import { toast } from "sonner"

import {
  loadSessionMessages,
  mergeHistoryMessages,
} from "@/features/chat/history"
import {
  isSessionMemoryEnabled,
  setSessionMemoryPreference,
} from "@/features/chat/memory-preference"
import {
  type ApplyChatSessionPatch,
  type ChatSessionPatch,
  type ChatSessionSnapshot,
  type PicoMessage,
  handlePicoMessage,
} from "@/features/chat/protocol"
import {
  clearStoredSessionId,
  generateSessionId,
  isIncognitoSession,
  markIncognitoSession,
  readStoredSessionId,
} from "@/features/chat/state"
import { invalidateSocket } from "@/features/chat/websocket"
import i18n from "@/i18n"
import {
  type ChatAttachment,
  getChatState,
  updateChatStore,
} from "@/store/chat"
import { type GatewayState, gatewayAtom } from "@/store/gateway"

const store = getDefaultStore()

interface SessionConnection {
  sessionId: string
  agentId: string
  socket: WebSocket | null
  connecting: boolean
  reconnectTimer: number | null
  reconnectAttempts: number
  lastActivity: number
  closedByUs: boolean
}

const connections = new Map<string, SessionConnection>()
const backgroundSnapshots = new Map<string, ChatSessionSnapshot>()
const MAX_BACKGROUND_CONNECTIONS = 4
const MAX_BACKGROUND_RECONNECTS = 2

let msgIdCounter = 0
let activeSessionIdRef = getChatState().activeSessionId
let chatAgentIdRef = ""
let initialized = false
let unsubscribeGateway: (() => void) | null = null
let hydratePromise: Promise<void> | null = null
let shouldMaintainConnection = false

function needsActiveSessionHydration(): boolean {
  const state = getChatState()
  const storedSessionId = readStoredSessionId()

  return Boolean(
    storedSessionId &&
    storedSessionId === state.activeSessionId &&
    !state.hasHydratedActiveSession,
  )
}

function setActiveSessionId(sessionId: string) {
  activeSessionIdRef = sessionId
  updateChatStore({ activeSessionId: sessionId })
}

function snapshotActiveSession(): ChatSessionSnapshot {
  const state = getChatState()
  return {
    messages: state.messages,
    isTyping: state.isTyping,
    contextUsage: state.contextUsage,
  }
}

function captureActiveSnapshot() {
  backgroundSnapshots.set(activeSessionIdRef, snapshotActiveSession())
}

function applyActivePatch(patch: ChatSessionPatch | ((prev: ChatSessionSnapshot) => ChatSessionPatch)) {
  updateChatStore((prev) => (typeof patch === "function" ? patch(prev) : patch))
}

function applySessionPatch(sessionId: string): ApplyChatSessionPatch {
  return (patch) => {
    if (sessionId === activeSessionIdRef) {
      applyActivePatch(patch)
      return
    }

    const prev = backgroundSnapshots.get(sessionId) ?? {
      messages: [],
      isTyping: false,
    }
    const next = typeof patch === "function" ? patch(prev) : patch
    backgroundSnapshots.set(sessionId, {
      messages: next.messages ?? prev.messages,
      isTyping: next.isTyping ?? prev.isTyping,
      contextUsage:
        next.contextUsage !== undefined
          ? next.contextUsage
          : prev.contextUsage,
    })
  }
}

function closeConnection(connection: SessionConnection) {
  connection.closedByUs = true
  if (connection.reconnectTimer !== null) {
    window.clearTimeout(connection.reconnectTimer)
    connection.reconnectTimer = null
  }
  const socket = connection.socket
  connection.socket = null
  connection.connecting = false
  invalidateSocket(socket)
}

function clearConnection(sessionId: string) {
  const connection = connections.get(sessionId)
  if (!connection) return
  closeConnection(connection)
  connections.delete(sessionId)
}

function reconnectActiveSession() {
  clearConnection(activeSessionIdRef)
  if (store.get(gatewayAtom).status === "running") {
    void connectChat()
  }
}

function scheduleConnectionReconnect(
  connection: SessionConnection,
  background: boolean,
) {
  if (
    !shouldMaintainConnection ||
    connection.closedByUs ||
    store.get(gatewayAtom).status !== "running"
  ) {
    return
  }
  if (background && connection.reconnectAttempts >= MAX_BACKGROUND_RECONNECTS) {
    return
  }
  if (connection.reconnectTimer !== null) {
    return
  }

  const delay = Math.min(1000 * 2 ** connection.reconnectAttempts, 5000)
  connection.reconnectAttempts += 1
  connection.reconnectTimer = window.setTimeout(() => {
    connection.reconnectTimer = null
    if (
      connection.closedByUs ||
      connection.socket ||
      !shouldMaintainConnection
    ) {
      return
    }
    void connectSession(connection.sessionId, connection.agentId)
  }, delay)
}

function sessionWebSocketUrl(sessionId: string, agentId: string): string {
  const wsScheme = window.location.protocol === "https:" ? "wss:" : "ws:"
  const incognito = isIncognitoSession(sessionId)
  const memory =
    incognito || !isSessionMemoryEnabled(sessionId) ? "off" : "on"
  const incognitoParam = incognito ? "&incognito=1" : ""
  const agentParam = agentId
    ? `&agent=${encodeURIComponent(agentId)}`
    : ""
  return `${wsScheme}//${window.location.host}/pico/ws?session_id=${encodeURIComponent(sessionId)}&memory=${memory}${incognitoParam}${agentParam}`
}

function connectSession(
  sessionId: string,
  agentId: string,
) {
  if (store.get(gatewayAtom).status !== "running") {
    return
  }
  if (sessionId === activeSessionIdRef && needsActiveSessionHydration()) {
    return
  }

  let connection = connections.get(sessionId)
  if (connection) {
    if (
      connection.socket &&
      (connection.socket.readyState === WebSocket.OPEN ||
        connection.socket.readyState === WebSocket.CONNECTING)
    ) {
      return
    }
    if (connection.reconnectTimer !== null) {
      window.clearTimeout(connection.reconnectTimer)
      connection.reconnectTimer = null
    }
  } else {
    connection = {
      sessionId,
      agentId,
      socket: null,
      connecting: false,
      reconnectTimer: null,
      reconnectAttempts: 0,
      lastActivity: Date.now(),
      closedByUs: false,
    }
    connections.set(sessionId, connection)
  }

  connection.agentId = agentId
  connection.closedByUs = false
  connection.connecting = true

  const socket = new WebSocket(sessionWebSocketUrl(sessionId, agentId))
  connection.socket = socket

  socket.onopen = () => {
    if (connection.socket !== socket || connection.closedByUs) return
    connection.connecting = false
    connection.reconnectAttempts = 0
    connection.lastActivity = Date.now()
    if (sessionId === activeSessionIdRef) {
      updateChatStore({ connectionState: "connected" })
    }
  }

  socket.onmessage = (event) => {
    if (connection.socket !== socket || connection.closedByUs) return
    connection.lastActivity = Date.now()
    try {
      const message = JSON.parse(event.data) as PicoMessage
      handlePicoMessage(message, sessionId, applySessionPatch(sessionId))
    } catch {
      console.warn("Non-JSON message from pico:", event.data)
    }
  }

  socket.onclose = () => {
    if (connection.socket !== socket) return
    connection.socket = null
    connection.connecting = false
    if (connection.closedByUs) return
    if (sessionId === activeSessionIdRef) {
      updateChatStore({ connectionState: "disconnected" })
    }
    scheduleConnectionReconnect(connection, connection.sessionId !== activeSessionIdRef)
  }

  socket.onerror = () => {
    if (connection.socket !== socket || connection.closedByUs) return
    connection.connecting = false
    if (sessionId === activeSessionIdRef) {
      updateChatStore({ connectionState: "error" })
    }
    scheduleConnectionReconnect(connection, connection.sessionId !== activeSessionIdRef)
  }
}

export async function connectChat() {
  if (
    store.get(gatewayAtom).status !== "running" ||
    needsActiveSessionHydration()
  ) {
    return
  }

  const existing = connections.get(activeSessionIdRef)
  if (existing?.socket) {
    if (existing.socket.readyState === WebSocket.OPEN) {
      updateChatStore({ connectionState: "connected" })
      return
    }
    if (existing.socket.readyState === WebSocket.CONNECTING) {
      updateChatStore({ connectionState: "connecting" })
      return
    }
  }

  updateChatStore({ connectionState: "connecting" })
  connectSession(activeSessionIdRef, chatAgentIdRef)
}

export function disconnectChat() {
  shouldMaintainConnection = false
  for (const connection of [...connections.values()]) {
    closeConnection(connection)
  }
  connections.clear()
  backgroundSnapshots.clear()
  updateChatStore({
    connectionState: "disconnected",
    isTyping: false,
  })
}

// setChatAgent targets the next Pico WebSocket connections at a specific agent.
export function setChatAgent(agentId: string) {
  const next = agentId.trim()
  if (next === chatAgentIdRef) return
  chatAgentIdRef = next
  if (shouldMaintainConnection) {
    reconnectActiveSession()
  }
}

function enforceBackgroundLimit() {
  const background = [...connections.values()]
    .filter((connection) => connection.sessionId !== activeSessionIdRef)
    .sort((a, b) => a.lastActivity - b.lastActivity)
  const excess = background.length - MAX_BACKGROUND_CONNECTIONS
  if (excess <= 0) return
  for (const connection of background.slice(0, excess)) {
    clearConnection(connection.sessionId)
  }
}

export async function hydrateActiveSession() {
  if (hydratePromise) {
    return hydratePromise
  }

  const state = getChatState()
  const storedSessionId = readStoredSessionId()

  if (
    !storedSessionId ||
    state.hasHydratedActiveSession ||
    storedSessionId !== state.activeSessionId
  ) {
    if (!state.hasHydratedActiveSession) {
      updateChatStore({ hasHydratedActiveSession: true })
    }
    return
  }

  hydratePromise = loadSessionMessages(storedSessionId)
    .then((historyMessages) => {
      const currentState = getChatState()
      if (currentState.activeSessionId !== storedSessionId) {
        return
      }

      if (currentState.messages.length > 0) {
        updateChatStore({
          messages: mergeHistoryMessages(
            historyMessages,
            currentState.messages,
          ),
          hasHydratedActiveSession: true,
        })
        return
      }

      updateChatStore({
        messages: historyMessages,
        isTyping: false,
        hasHydratedActiveSession: true,
      })
    })
    .catch((error) => {
      console.error("Failed to restore last session history:", error)

      const currentState = getChatState()
      if (currentState.activeSessionId !== storedSessionId) {
        return
      }

      if (currentState.messages.length > 0) {
        updateChatStore({ hasHydratedActiveSession: true })
        return
      }

      clearStoredSessionId()
      updateChatStore({
        messages: [],
        isTyping: false,
        hasHydratedActiveSession: true,
      })
    })
    .finally(() => {
      hydratePromise = null
    })

  return hydratePromise
}

async function refreshActiveHistory(sessionId: string) {
  try {
    const history = await loadSessionMessages(sessionId)
    if (activeSessionIdRef !== sessionId) return
    updateChatStore((prev) => ({
      messages: mergeHistoryMessages(history, prev.messages),
      hasHydratedActiveSession: true,
    }))
  } catch (error) {
    console.error("Failed to refresh active session history:", error)
  }
}

interface SendChatMessageInput {
  content: string
  attachments?: ChatAttachment[]
}

export function sendChatMessage({
  content,
  attachments = [],
}: SendChatMessageInput) {
  const connection = connections.get(activeSessionIdRef)
  const socket = connection?.socket
  if (!socket || socket.readyState !== WebSocket.OPEN) {
    console.warn("WebSocket not connected")
    return false
  }

  const normalizedContent = content.trim()
  const imageAttachments = attachments
    .filter((attachment) => attachment.type === "image" && attachment.url)
    .map((attachment) => ({ ...attachment }))
  const fileAttachments = attachments
    .filter(
      (attachment) =>
        attachment.type !== "image" &&
        Boolean(attachment.text?.trim()) &&
        Boolean(attachment.filename?.trim()),
    )
    .map((attachment) => ({ ...attachment }))
  const normalizedAttachments = [...imageAttachments, ...fileAttachments]

  if (!normalizedContent && normalizedAttachments.length === 0) {
    return false
  }

  const id = `msg-${++msgIdCounter}-${Date.now()}`

  updateChatStore((prev) => ({
    messages: [
      ...prev.messages,
      {
        id,
        role: "user",
        content: normalizedContent,
        attachments:
          normalizedAttachments.length > 0 ? normalizedAttachments : undefined,
        timestamp: Date.now(),
      },
    ],
    isTyping: true,
  }))

  try {
    const payload: Record<string, unknown> = {
      content: normalizedContent,
      media: imageAttachments.map((attachment) => attachment.url),
    }
    if (fileAttachments.length > 0) {
      payload.attachments = fileAttachments.map((attachment) => ({
        type: "file",
        filename: attachment.filename,
        content_type: attachment.contentType,
        text: attachment.text,
      }))
    }

    socket.send(
      JSON.stringify({
        type: "message.send",
        id,
        session_id: activeSessionIdRef,
        payload,
      }),
    )
    return true
  } catch (error) {
    console.error("Failed to send pico message:", error)
    updateChatStore((prev) => ({
      messages: prev.messages.filter((message) => message.id !== id),
      isTyping: false,
    }))
    return false
  }
}

export function stopActiveTurn() {
  const connection = connections.get(activeSessionIdRef)
  const socket = connection?.socket
  if (!socket || socket.readyState !== WebSocket.OPEN) {
    return false
  }
  const id = `msg-${++msgIdCounter}-${Date.now()}`
  try {
    socket.send(
      JSON.stringify({
        type: "message.send",
        id,
        session_id: activeSessionIdRef,
        payload: { content: "/stop", media: [] },
      }),
    )
    return true
  } catch (error) {
    console.error("Failed to send stop command:", error)
    return false
  }
}

export async function switchChatSession(sessionId: string, agentId?: string) {
  if (sessionId === activeSessionIdRef) {
    if (agentId !== undefined && agentId.trim() !== chatAgentIdRef) {
      chatAgentIdRef = agentId.trim()
      reconnectActiveSession()
    }
    return
  }

  captureActiveSnapshot()
  if (agentId !== undefined) {
    chatAgentIdRef = agentId.trim()
  }
  setActiveSessionId(sessionId)

  const cached = backgroundSnapshots.get(sessionId)
  const live =
    connections.get(sessionId)?.socket?.readyState === WebSocket.OPEN
  if (cached) {
    backgroundSnapshots.delete(sessionId)
    updateChatStore({
      messages: cached.messages,
      isTyping: cached.isTyping,
      contextUsage: cached.contextUsage,
      hasHydratedActiveSession: true,
    })
  } else {
    try {
      const historyMessages = await loadSessionMessages(sessionId)
      if (activeSessionIdRef !== sessionId) return
      updateChatStore({
        messages: historyMessages,
        isTyping: false,
        hasHydratedActiveSession: true,
        contextUsage: undefined,
      })
    } catch (error) {
      console.error("Failed to load session history:", error)
      toast.error(i18n.t("chat.historyOpenFailed"))
    }
  }

  // When the session has a live connection, its cache/stream is authoritative.
  // Otherwise merge persisted messages (e.g. a response that completed while
  // this session was in the background) with the snapshot.
  if (!cached || !live) {
    void refreshActiveHistory(sessionId)
  }

  if (store.get(gatewayAtom).status === "running") {
    shouldMaintainConnection = true
    await connectChat()
  }
  enforceBackgroundLimit()
}

export async function newChatSession(agentId?: string) {
  const targetAgent = agentId?.trim()
  const agentChanged = Boolean(targetAgent && targetAgent !== chatAgentIdRef)
  if (
    !agentChanged &&
    getChatState().messages.length === 0 &&
    !isIncognitoSession(activeSessionIdRef)
  ) {
    return
  }

  captureActiveSnapshot()
  if (targetAgent) {
    chatAgentIdRef = targetAgent
  }

  setActiveSessionId(generateSessionId())
  updateChatStore({
    messages: [],
    isTyping: false,
    hasHydratedActiveSession: true,
    contextUsage: undefined,
  })

  if (store.get(gatewayAtom).status === "running") {
    shouldMaintainConnection = true
    await connectChat()
  }
  enforceBackgroundLimit()
}

export async function newIncognitoChatSession(agentId?: string) {
  captureActiveSnapshot()
  if (agentId !== undefined) {
    chatAgentIdRef = agentId.trim()
  }
  const sessionId = generateSessionId()
  markIncognitoSession(sessionId)
  setActiveSessionId(sessionId)
  updateChatStore({
    messages: [],
    isTyping: false,
    hasHydratedActiveSession: true,
    contextUsage: undefined,
  })

  if (store.get(gatewayAtom).status === "running") {
    shouldMaintainConnection = true
    await connectChat()
  }
  enforceBackgroundLimit()
}

export function isActiveSessionMemoryEnabled(): boolean {
  if (isIncognitoSession(activeSessionIdRef)) {
    return false
  }
  return isSessionMemoryEnabled(activeSessionIdRef)
}

export function setActiveSessionMemoryEnabled(enabled: boolean) {
  if (isIncognitoSession(activeSessionIdRef)) {
    return
  }
  setSessionMemoryPreference(activeSessionIdRef, enabled)
  if (store.get(gatewayAtom).status === "running") {
    reconnectActiveSession()
  }
}

export function initializeChatStore() {
  if (initialized) {
    return
  }

  initialized = true
  activeSessionIdRef = getChatState().activeSessionId
  let lastGatewayStatus: GatewayState | null = null

  const syncConnectionWithGateway = (force: boolean = false) => {
    const gatewayStatus = store.get(gatewayAtom).status
    if (!force && gatewayStatus === lastGatewayStatus) {
      return
    }
    lastGatewayStatus = gatewayStatus

    if (gatewayStatus === "running") {
      shouldMaintainConnection = true
      if (needsActiveSessionHydration()) {
        return
      }
      void connectChat()
      return
    }

    if (gatewayStatus === "stopped" || gatewayStatus === "error") {
      disconnectChat()
    }
  }

  unsubscribeGateway = store.sub(gatewayAtom, syncConnectionWithGateway)

  if (!readStoredSessionId()) {
    updateChatStore({ hasHydratedActiveSession: true })
    syncConnectionWithGateway(true)
    return
  }

  void hydrateActiveSession().finally(() => {
    if (!initialized) {
      return
    }
    syncConnectionWithGateway(true)
  })
}

export function teardownChatStore() {
  unsubscribeGateway?.()
  unsubscribeGateway = null
  initialized = false
  disconnectChat()
}
