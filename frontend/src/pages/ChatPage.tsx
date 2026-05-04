/* eslint-disable @typescript-eslint/no-unsafe-member-access,@typescript-eslint/no-unnecessary-condition,@typescript-eslint/no-floating-promises,no-nested-ternary,@typescript-eslint/restrict-template-expressions */
import {
  Send as SendIcon,
  Bot,
  User,
  X,
  Trash2,
  MessageSquare,
  Loader2,
} from "lucide-react"
import { useEffect, useState, useRef, useCallback, useMemo } from "react"
import ReactMarkdown from "react-markdown"

import {
  getModels,
  createChatCompletion,
  listChatSessions,
  getChatSession,
  createChatSession,
  addChatMessage,
  type ModelInfo,
  deleteChatSession,
  deleteAllChatSessions,
  type ChatMessage,
  type ChatApiResponse,
  type ChatSessionSummary,
} from "@/api"
import { EmptyState } from "@/components/EmptyState"
import { ModelCombobox } from "@/components/ModelCombobox"
import { createLogger } from "@/lib/logger"

const _log = createLogger("chat-page")

interface ChatPageProps {
  showToast: (msg: string, type?: "success" | "error") => void
}

type ApiShape = "openai" | "responses"

interface MessageWithId extends ChatMessage {
  id: string
}

function extractMessageContent(
  response: ChatApiResponse,
  _apiShape: ApiShape,
): string {
  if (
    "choices" in response
    && response.choices
    && response.choices.length > 0
  ) {
    return response.choices[0].message.content || ""
  }
  if ("content" in response && Array.isArray(response.content)) {
    const textBlocks = response.content.filter(
      (block): block is { type: string; text?: string } =>
        Boolean(block)
        && typeof block === "object"
        && "type" in block
        && block.type === "text",
    )
    return textBlocks.map((block) => block.text || "").join("")
  }
  if ("output" in response && Array.isArray(response.output)) {
    const messageItems = response.output.filter(
      (item) => Boolean(item) && item.type === "message",
    )
    if (messageItems.length > 0 && Array.isArray(messageItems[0].content)) {
      const textBlocks = messageItems[0].content.filter(
        (block) =>
          Boolean(block)
          && typeof block === "object"
          && "type" in block
          && block.type === "output_text",
      ) as Array<{ type: "output_text"; text?: string }>
      return textBlocks.map((block) => block.text || "").join("")
    }
  }
  return ""
}

function generateUUID(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 11)}`
}

function formatChatError(error: unknown): string {
  if (error instanceof Error) {
    if (error.message.includes("fetch") || error.message.includes("network")) {
      return "Network error. Please check your connection and try again."
    }
    if (
      error.message.includes("401")
      || error.message.includes("unauthorized")
    ) {
      return "Authentication failed. Please check your API key."
    }
    if (error.message.includes("429") || error.message.includes("rate limit")) {
      return "Rate limited. Please wait a moment and try again."
    }
    if (error.message.includes("5") || error.message.includes("server")) {
      return "Server error. Please try again shortly."
    }
    return "Something went wrong. Please try again."
  }
  return "An unexpected error occurred. Please try again."
}

// ─── Message avatar ───────────────────────────────────────────────────────────

function MessageAvatar({ role }: { role: "user" | "assistant" | "system" }) {
  return (
    <div
      aria-hidden="true"
      style={{
        width: 32,
        height: 32,
        borderRadius: "var(--radius-md)",
        background:
          role === "user" ? "var(--color-blue)" : "var(--color-green)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        flexShrink: 0,
        boxShadow: "var(--shadow-btn)",
      }}
    >
      {role === "user" ?
        <User size={16} style={{ color: "white" }} />
      : <Bot size={16} style={{ color: "white" }} />}
    </div>
  )
}

// ─── ChatPage ─────────────────────────────────────────────────────────────────

export function ChatPage({ showToast }: ChatPageProps) {
  const [models, setModels] = useState<Array<ModelInfo>>([])
  const [selectedModel, setSelectedModel] = useState<string>("")
  const [apiShape, setApiShape] = useState<ApiShape>("openai")
  const [messages, setMessages] = useState<Array<MessageWithId>>([])
  const [inputValue, setInputValue] = useState("")
  const [isLoading, setIsLoading] = useState(false)
  const [modelsLoading, setModelsLoading] = useState(true)
  const [sessions, setSessions] = useState<Array<ChatSessionSummary>>([])
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null)
  const [sessionsLoading, setSessionsLoading] = useState(true)
  const [showDeleteAllConfirm, setShowDeleteAllConfirm] = useState(false)

  useEffect(() => {
    if (!showDeleteAllConfirm) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setShowDeleteAllConfirm(false)
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [showDeleteAllConfirm])

  const unavailableModelToastRef = useRef<string | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const messagesContainerRef = useRef<HTMLDivElement>(null)

  const availableModels = useMemo(
    () =>
      models.filter(
        (model) => !model.api_shape || model.api_shape === apiShape,
      ),
    [models, apiShape],
  )

  // Load sessions and models on mount
  useEffect(() => {
    const loadData = async () => {
      try {
        const [modelsRes, sessionsRes] = await Promise.all([
          getModels(),
          listChatSessions(),
        ])
        const loadedModels = modelsRes.data || []
        setModels(loadedModels)
        const initialModels = loadedModels.filter(
          (model) => !model.api_shape || model.api_shape === apiShape,
        )
        if (initialModels.length > 0) setSelectedModel(initialModels[0].id)
        setSessions(sessionsRes)
      } catch (error) {
        showToast(`Failed to load data: ${error}`, "error")
      } finally {
        setModelsLoading(false)
        setSessionsLoading(false)
      }
    }
    loadData()
  }, [showToast])

  // Keep selected model valid when apiShape or models change
  useEffect(() => {
    if (modelsLoading) return
    if (availableModels.length === 0) {
      if (selectedModel) setSelectedModel("")
      return
    }
    if (!selectedModel) {
      setSelectedModel(availableModels[0].id)
      return
    }
    if (!availableModels.some((m) => m.id === selectedModel)) {
      const fallback = availableModels[0]
      setSelectedModel(fallback.id)
      if (unavailableModelToastRef.current !== selectedModel) {
        unavailableModelToastRef.current = selectedModel
        showToast(
          `Model "${selectedModel}" is unavailable; switched to ${fallback.display_name || fallback.id}`,
          "error",
        )
      }
      return
    }
    unavailableModelToastRef.current = null
  }, [availableModels, modelsLoading, selectedModel, showToast])

  // Load session messages when switching sessions
  useEffect(() => {
    if (!currentSessionId) return
    const loadSession = async () => {
      try {
        const session = await getChatSession(currentSessionId)
        setMessages(
          session.messages.map((msg) => ({
            id: generateUUID(),
            role: msg.role as "user" | "assistant" | "system",
            content: msg.content,
          })),
        )
        setSelectedModel(session.session.model_id)
        setApiShape(session.session.api_shape as ApiShape)
      } catch (error) {
        showToast(`Failed to load session: ${error}`, "error")
      }
    }
    loadSession()
  }, [currentSessionId, showToast])

  // Auto-scroll only when already near bottom
  useEffect(() => {
    const container = messagesContainerRef.current
    if (!container) return
    const isAtBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight < 50
    if (isAtBottom)
      messagesEndRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [messages])

  const handleNewChat = useCallback(() => {
    setCurrentSessionId(null)
    setMessages([])
    setInputValue("")
  }, [])

  const handleSendMessage = useCallback(async () => {
    if (!inputValue.trim() || !selectedModel || isLoading) return

    // Handle slash commands
    const trimmed = inputValue.trim()
    if (trimmed === "/clear" || trimmed === "/cls") {
      handleNewChat()
      return
    }

    const userMessage: MessageWithId = {
      id: generateUUID(),
      role: "user",
      content: inputValue.trim(),
    }

    const newMessages = [...messages, userMessage]
    setMessages(newMessages)
    setInputValue("")
    setIsLoading(true)

    try {
      let sessionId = currentSessionId
      if (!sessionId) {
        sessionId = generateUUID()
        const title =
          inputValue.slice(0, 50) + (inputValue.length > 50 ? "..." : "")
        try {
          const created = await createChatSession({
            session_id: sessionId,
            title,
            model_id: selectedModel,
            api_shape: apiShape,
          })
          sessionId = created.session_id || sessionId
          setCurrentSessionId(sessionId)
        } catch (err) {
          showToast(
            `Failed to create chat session: ${err instanceof Error ? err.message : String(err)}`,
            "error",
          )
          setMessages(messages)
          setInputValue(userMessage.content)
          setIsLoading(false)
          return
        }
      }

      await addChatMessage(sessionId, {
        message_id: generateUUID(),
        role: "user",
        content: userMessage.content,
      })

      const response = await createChatCompletion(
        {
          model: selectedModel,
          messages: newMessages.map(({ id: _, ...msg }) => msg),
          stream: false,
        },
        apiShape,
      )

      if (response && !(response instanceof ReadableStream)) {
        const content = extractMessageContent(response, apiShape)
        if (content) {
          const assistantMsg: MessageWithId = {
            id: generateUUID(),
            role: "assistant",
            content,
          }
          setMessages([...newMessages, assistantMsg])
          await addChatMessage(sessionId, {
            message_id: generateUUID(),
            role: "assistant",
            content,
          })
          setSessions(await listChatSessions())
        } else {
          throw new Error("No content in response")
        }
      } else {
        throw new Error("No response received")
      }
    } catch (error) {
      showToast(`Chat error: ${formatChatError(error)}`, "error")
    } finally {
      setIsLoading(false)
    }
  }, [
    inputValue,
    selectedModel,
    isLoading,
    messages,
    currentSessionId,
    apiShape,
    showToast,
    handleNewChat,
  ])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault()
      void handleSendMessage()
    }
  }

  const handleDeleteSession = async (
    sessionId: string,
    e: React.MouseEvent,
  ) => {
    e.stopPropagation()
    try {
      await deleteChatSession(sessionId)
      setSessions((prev) => prev.filter((s) => s.session_id !== sessionId))
      if (currentSessionId === sessionId) handleNewChat()
      showToast("Chat deleted", "success")
    } catch (error) {
      showToast(`Failed to delete chat: ${error}`, "error")
    }
  }

  const handleDeleteAllSessions = async () => {
    try {
      await deleteAllChatSessions()
      setSessions([])
      handleNewChat()
      showToast("All chats deleted", "success")
    } catch (error) {
      showToast(`Failed to delete all chats: ${error}`, "error")
    } finally {
      setShowDeleteAllConfirm(false)
    }
  }

  return (
    <div className="chat-layout">
      {/* ── Left Sidebar ─────────────────────────────────── */}
      <div className="panel chat-sidebar">
        {/* New Chat */}
        <button
          onClick={handleNewChat}
          className="btn btn-primary"
          style={{ width: "100%" }}
        >
          + New Chat
        </button>

        {/* API Endpoint */}
        <div>
          <label
            htmlFor="api-shape-select"
            className="sys-label"
            style={{
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              fontSize: 11,
            }}
          >
            API Endpoint
          </label>
          <select
            id="api-shape-select"
            value={apiShape}
            onChange={(e) => setApiShape(e.target.value as ApiShape)}
            className="sys-select"
            style={{ background: "var(--color-surface-2)", fontSize: 13 }}
          >
            <option value="openai">OpenAI</option>
            <option value="responses">Responses</option>
          </select>
        </div>

        {/* Model */}
        <div>
          <label
            className="sys-label"
            style={{
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              fontSize: 11,
            }}
          >
            Model
          </label>
          {modelsLoading ?
            <div
              aria-live="polite"
              style={{
                padding: "10px 12px",
                border: "1px solid var(--color-separator)",
                borderRadius: "var(--radius-md)",
                background: "var(--color-surface)",
                color: "var(--color-text-secondary)",
                fontSize: 12,
                display: "flex",
                alignItems: "center",
                gap: 8,
              }}
            >
              <span className="spinner" aria-hidden="true" />
              Loading models…
            </div>
          : <ModelCombobox
              models={availableModels}
              selectedModel={selectedModel}
              onSelect={setSelectedModel}
            />
          }
        </div>

        {/* Chat History */}
        <div
          className="chat-history-section"
          style={{
            flex: 1,
            overflow: "hidden",
            display: "flex",
            flexDirection: "column",
          }}
        >
          <div className="chat-history-header">
            <label
              className="sys-label"
              style={{
                textTransform: "uppercase",
                letterSpacing: "0.05em",
                fontSize: 11,
                margin: 0,
              }}
            >
              History
            </label>
            {sessions.length > 0 && (
              <button
                onClick={() => setShowDeleteAllConfirm(true)}
                className="btn btn-icon btn-icon-ghost btn-icon-danger"
                aria-label="Delete all chat history"
              >
                <Trash2 size={14} />
              </button>
            )}
          </div>

          {sessionsLoading ?
            <div className="chat-history-state">
              <span className="spinner" aria-hidden="true" />
              Loading…
            </div>
          : sessions.length === 0 ?
            <div className="chat-history-state chat-history-empty">
              <EmptyState
                icon={<MessageSquare size={20} />}
                title="No history yet"
                description="Start a conversation to save it here."
              />
            </div>
          : <div className="scrollable-list">
              {sessions.map((session) => {
                const isActive = currentSessionId === session.session_id
                return (
                  <div
                    key={session.session_id}
                    className="list-item-interactive"
                    style={{
                      background:
                        isActive ?
                          "var(--color-blue-fill)"
                        : "var(--color-surface-2)",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      gap: 8,
                      padding: 0,
                    }}
                  >
                    <button
                      type="button"
                      aria-label={`Open chat: ${session.title}`}
                      aria-pressed={isActive}
                      onClick={() => setCurrentSessionId(session.session_id)}
                      style={{
                        flex: 1,
                        overflow: "hidden",
                        background: "transparent",
                        border: "none",
                        font: "inherit",
                        color: "inherit",
                        cursor: "pointer",
                        textAlign: "left",
                        padding: "8px 0 8px 12px",
                      }}
                    >
                      <div
                        style={{
                          fontSize: 12,
                          color: isActive ? "white" : "var(--color-text)",
                          whiteSpace: "nowrap",
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                        }}
                      >
                        {session.title}
                      </div>
                      <div
                        style={{
                          fontSize: 11,
                          color:
                            isActive ?
                              "rgba(255,255,255,0.7)"
                            : "var(--color-text-secondary)",
                          marginTop: 2,
                        }}
                      >
                        {new Date(session.updated_at).toLocaleDateString()}
                      </div>
                    </button>
                    <button
                      onClick={(e) =>
                        handleDeleteSession(session.session_id, e)
                      }
                      className="btn btn-icon btn-icon-ghost btn-icon-danger"
                      aria-label={`Delete chat: ${session.title}`}
                      style={{ marginRight: 6 }}
                    >
                      <X size={16} />
                    </button>
                  </div>
                )
              })}
            </div>
          }
        </div>

        {/* Clear current */}
        <div
          style={{
            borderTop: "1px solid var(--color-separator)",
            paddingTop: 12,
          }}
        >
          <button
            onClick={handleNewChat}
            className="btn btn-ghost"
            style={{ width: "100%", fontSize: 13 }}
          >
            Clear Current
          </button>
        </div>
      </div>

      {/* ── Chat Main ─────────────────────────────────────── */}
      <div className="panel chat-main">
        {/* Messages */}
        <div
          ref={messagesContainerRef}
          role="log"
          aria-label="Chat messages"
          aria-live="polite"
          style={{
            flex: 1,
            overflowY: "auto",
            padding: "24px",
            display: "flex",
            flexDirection: "column",
            gap: 20,
          }}
        >
          {messages.length === 0 ?
            <EmptyState
              icon={<MessageSquare size={24} />}
              title="Start a conversation"
              description="Select a model and send your first message to begin chatting."
            />
          : messages.map((message, index) => (
              <div
                key={message.id}
                className="animate-slide-in"
                role={message.role === "user" ? "note" : "article"}
                aria-label={`${message.role === "user" ? "You" : "Assistant"} said`}
                style={{
                  display: "flex",
                  justifyContent:
                    message.role === "user" ? "flex-end" : "flex-start",
                  animationDelay: `${Math.min(index * 0.05, 0.3)}s`,
                }}
              >
                <div
                  className="chat-bubble-shell"
                  style={{
                    display: "flex",
                    alignItems: "flex-start",
                    gap: 12,
                    flexDirection:
                      message.role === "user" ? "row-reverse" : "row",
                  }}
                >
                  <MessageAvatar role={message.role} />
                  <div
                    style={{
                      padding: "12px 16px",
                      borderRadius: "var(--radius-lg)",
                      background:
                        message.role === "user" ?
                          "var(--color-blue)"
                        : "var(--color-surface)",
                      color:
                        message.role === "user" ? "white" : "var(--color-text)",
                      border:
                        message.role === "user" ?
                          "none"
                        : "1px solid var(--color-separator)",
                      boxShadow: "var(--shadow-btn)",
                    }}
                  >
                    {message.role === "user" ?
                      <p
                        className="chat-message-markdown"
                        style={{
                          margin: 0,
                          whiteSpace: "pre-wrap",
                          fontSize: 14,
                          lineHeight: 1.6,
                        }}
                      >
                        {message.content}
                      </p>
                    : <div className="chat-message-markdown">
                        <ReactMarkdown>{message.content}</ReactMarkdown>
                      </div>
                    }
                  </div>
                </div>
              </div>
            ))
          }

          {isLoading && (
            <div
              className="animate-slide-in"
              style={{ display: "flex", justifyContent: "flex-start" }}
            >
              <div
                className="chat-bubble-shell"
                style={{
                  display: "flex",
                  alignItems: "flex-start",
                  gap: 12,
                }}
              >
                <MessageAvatar role="assistant" />
                <div
                  style={{
                    padding: "14px 18px",
                    borderRadius: "var(--radius-lg)",
                    background: "var(--color-surface)",
                    border: "1px solid var(--color-separator)",
                    color: "var(--color-text-tertiary)",
                    boxShadow: "var(--shadow-btn)",
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                  }}
                >
                  <span style={{ fontSize: 12, fontStyle: "italic" }}>
                    Thinking
                  </span>
                  <span className="thinking-dots" aria-hidden="true">
                    <span />
                    <span />
                    <span />
                  </span>
                </div>
              </div>
            </div>
          )}

          <div ref={messagesEndRef} />
        </div>

        {/* Input */}
        <div
          style={{
            padding: "12px 16px 16px",
            borderTop: "1px solid var(--color-separator)",
            background: "var(--color-surface)",
            borderBottomLeftRadius: "var(--radius-lg)",
            borderBottomRightRadius: "var(--radius-lg)",
          }}
        >
          <div
            className="chat-input-wrapper"
            style={{ display: "flex", alignItems: "flex-end", gap: 8 }}
          >
            <div style={{ flex: 1 }}>
              <label
                htmlFor="chat-input"
                style={{
                  position: "absolute",
                  width: 1,
                  height: 1,
                  padding: 0,
                  margin: -1,
                  overflow: "hidden",
                  clip: "rect(0,0,0,0)",
                  whiteSpace: "nowrap",
                  border: 0,
                }}
              >
                Chat message
              </label>
              <textarea
                id="chat-input"
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Chat with the model… (type /clear to reset)"
                disabled={!selectedModel || isLoading}
                aria-describedby={
                  !selectedModel ? "model-required-hint" : undefined
                }
                className="chat-textarea"
                style={{ minHeight: 52 }}
              />
            </div>
            <button
              onClick={() => void handleSendMessage()}
              disabled={!inputValue.trim() || !selectedModel || isLoading}
              className="btn btn-primary"
              style={{
                width: 40,
                height: 40,
                padding: 0,
                flexShrink: 0,
                borderRadius: "var(--radius-md)",
                marginBottom: 6,
                marginRight: 6,
              }}
              aria-label={isLoading ? "Sending message…" : "Send message"}
            >
              {isLoading ?
                <Loader2 size={16} className="animate-spin" />
              : <SendIcon size={16} />}
            </button>
          </div>
          {!selectedModel && (
            <span
              id="model-required-hint"
              style={{
                fontSize: 12,
                color: "var(--color-text-tertiary)",
                marginTop: 8,
                display: "block",
              }}
            >
              No model available for this API shape.
            </span>
          )}
        </div>
      </div>

      {/* ── Delete-All Confirmation Dialog ────────────────── */}
      {showDeleteAllConfirm && (
        <div
          className="dialog-overlay"
          role="dialog"
          aria-modal="true"
          aria-labelledby="delete-all-title"
          onClick={(e) => {
            if (e.target === e.currentTarget) setShowDeleteAllConfirm(false)
          }}
        >
          <div className="dialog-box" style={{ maxWidth: 380 }}>
            <div className="dialog-header">
              <span
                id="delete-all-title"
                style={{ fontWeight: 600, fontSize: 15 }}
              >
                Delete all chat history?
              </span>
            </div>
            <div className="dialog-body">
              <p
                style={{
                  fontSize: 13,
                  color: "var(--color-text-secondary)",
                  margin: 0,
                }}
              >
                This will permanently remove all sessions and cannot be undone.
              </p>
              <div style={{ display: "flex", gap: 8, marginTop: 20 }}>
                <button
                  onClick={() => setShowDeleteAllConfirm(false)}
                  className="btn btn-ghost"
                  style={{ flex: 1 }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleDeleteAllSessions}
                  className="btn btn-danger"
                  style={{ flex: 1 }}
                >
                  Delete All
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
