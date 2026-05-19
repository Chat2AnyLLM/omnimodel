import { createLogger } from "@/lib/logger"

const log = createLogger("api")

// ─── Shared types ─────────────────────────────────────────────────────────────

export interface Provider {
  id: string // Instance ID (e.g., "antigravity-1", "alibaba-2")
  type: string // Provider type (e.g., "antigravity", "alibaba")
  name: string
  subtitle?: string // Custom display subtitle (defaults to instance id display)
  isActive: boolean
  authStatus: "authenticated" | "unauthenticated"
  enabledModelCount?: number
  totalModelCount?: number
  priority?: number
  config?: {
    endpoint?: string
    apiVersion?: string
    deployments?: Array<string>
    models?: Array<string>
    apiFormat?: string
  }
}

export interface AuthFlow {
  providerId: string
  status: "pending" | "awaiting_user" | "complete" | "error"
  instructionURL?: string
  userCode?: string
  error?: string
}

export interface Status {
  activeProvider: { id: string; name: string } | null
  modelCount: number
  manualApprove: boolean
  rateLimitSeconds: number | null
  rateLimitWait: boolean
  authFlow: AuthFlow | null
}

export interface ServerInfo {
  version: string
  port: number
  authRequired?: boolean
}

export type LogLevel = "fatal" | "error" | "warn" | "info" | "debug" | "trace"

const LOG_LEVEL_BY_SEVERITY = [
  "fatal",
  "error",
  "warn",
  "info",
  "debug",
  "trace",
] as const

function normalizeLogLevel(value: unknown): LogLevel {
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase()
    if ((LOG_LEVEL_BY_SEVERITY as ReadonlyArray<string>).includes(normalized)) {
      return normalized as LogLevel
    }

    if (normalized === "warning") {
      return "warn"
    }
  }

  if (
    typeof value === "number"
    && Number.isInteger(value)
    && value >= 0
    && value < LOG_LEVEL_BY_SEVERITY.length
  ) {
    return LOG_LEVEL_BY_SEVERITY[value] as LogLevel
  }

  return "info"
}

export interface Model {
  id: string
  name: string
  vendor?: string
  enabled: boolean
}

export interface ModelInfo {
  id: string
  display_name?: string
  name?: string
  owned_by?: string
  api_shape?: string
}

export interface QuotaSnapshot {
  entitlement: number
  remaining: number
  quota_remaining?: number
  percent_remaining: number
  unlimited: boolean
  overage_count?: number
  overage_permitted?: boolean
  quota_id?: string
}

export interface ChatMessage {
  role: "user" | "assistant" | "system"
  content: string
}

export interface ChatCompletionRequest {
  model: string
  messages: Array<ChatMessage>
  stream?: boolean
  max_tokens?: number
  temperature?: number
}

export interface ChatCompletionResponse {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    message: ChatMessage
    finish_reason: string | null
  }>
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

export interface ResponsesInputItem {
  type: "message"
  role: "user" | "assistant" | "system"
  content: string
}

export interface ResponsesRequest {
  model: string
  input: Array<ResponsesInputItem>
  max_output_tokens?: number
  stream?: boolean
  temperature?: number
}

export interface ResponsesResponse {
  id: string
  object: "response"
  model: string
  status?: "in_progress" | "completed" | "incomplete" | "failed"
  output_text?: string
  output: Array<{
    type: "message" | "function_call"
    id: string
    role: "assistant"
    status?: "in_progress" | "completed" | "incomplete"
    name?: string
    arguments?: string
    content?: Array<{
      type: "output_text"
      text: string
      annotations?: Array<unknown>
    }>
  }>
  usage?: {
    input_tokens: number
    output_tokens: number
    total_tokens?: number
  }
  created_at?: number
}

export type ChatApiResponse = ChatCompletionResponse | ResponsesResponse

export interface UsageData {
  // GitHub Copilot fields
  access_type_sku?: string
  copilot_plan?: string
  quota_reset_date?: string
  chat_enabled?: boolean
  assigned_date?: string
  quota_snapshots?: Record<string, QuotaSnapshot>
  [key: string]: unknown
}

// ─── Base fetch ───────────────────────────────────────────────────────────────

// Auto-detect backend port at runtime
let detectedBackendPort: number | null = null
let detectionPromise: Promise<number> | null = null
let cachedApiKey: string | null = null

declare const __API_KEY__: string | undefined

function getApiKey(): string {
  if (cachedApiKey !== null) {
    return cachedApiKey
  }

  const meta = globalThis.document.querySelector('meta[name="omnillm-api-key"]')
  const content = meta?.getAttribute("content")?.trim() ?? ""
  cachedApiKey =
    content || (typeof __API_KEY__ !== "undefined" ? __API_KEY__ : "")
  return cachedApiKey
}

async function detectBackendPort(): Promise<number> {
  if (detectedBackendPort) {
    return detectedBackendPort
  }

  if (detectionPromise) {
    return detectionPromise
  }

  detectionPromise = (async () => {
    const possiblePorts = [
      // Try compile-time port first if available
      typeof __SERVER_PORT__ !== "undefined" ?
        Number(__SERVER_PORT__)
      : undefined,
      Number(import.meta.env.VITE_SERVER_PORT), // Vite env variable
      4141, // default from start.ts
      5000, // common go backend port override
      5002, // common alternative
      3000, // common dev port
      8000, // another common port
    ].filter((port): port is number => Boolean(port && !Number.isNaN(port)))

    for (const port of possiblePorts) {
      try {
        log.trace(`trying backend port ${port}`)
        const response = await fetch(
          `${globalThis.location.protocol}//${globalThis.location.hostname}:${port}/api/admin/info`,
          {
            method: "GET",
            signal: AbortSignal.timeout(2000),
          },
        )
        if (response.ok) {
          detectedBackendPort = port
          log.info(`Auto-detected backend server on port ${port}`)
          return port
        }
      } catch {
        // Port not available, continue checking
      }
    }

    // If we're running on a different port than 5080, assume backend is on a different port too
    const currentPort = Number(globalThis.location.port)
    if (currentPort && currentPort !== 5080) {
      // Try the port that's 1000 less (common pattern: frontend 5173, backend 4173)
      const guessedPort = currentPort - 1000
      try {
        const response = await fetch(
          `${globalThis.location.protocol}//${globalThis.location.hostname}:${guessedPort}/api/admin/info`,
          {
            method: "GET",
            signal: AbortSignal.timeout(2000),
          },
        )
        if (response.ok) {
          detectedBackendPort = guessedPort
          log.info(
            `Auto-detected backend server on port ${guessedPort} (frontend port - 1000)`,
          )
          return guessedPort
        }
      } catch {
        // Guess failed
      }
    }

    // Last resort: use compile-time port or default
    const fallback =
      typeof __SERVER_PORT__ !== "undefined" ? Number(__SERVER_PORT__) : 4141
    console.warn(
      `⚠️ Could not auto-detect backend port, falling back to ${fallback}`,
    )
    detectedBackendPort = fallback
    return fallback
  })()

  return detectionPromise
}

async function getBackendBase(): Promise<string> {
  // If we're in development and running on localhost with a port, auto-detect
  if (
    globalThis.location.hostname === "localhost"
    && globalThis.location.port
  ) {
    const port = await detectBackendPort()
    return `${globalThis.location.protocol}//${globalThis.location.hostname}:${port}`
  }

  // In production or when served from the same host, assume relative URLs work
  return ""
}

async function apiFetch<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const backendBase = await getBackendBase()
  const fullUrl = `${backendBase}${path}`
  const method = opts.method || "GET"
  const apiKey = getApiKey()

  const t0 = performance.now()
  log.trace(`${method} ${path}`, { url: fullUrl })

  const headers = new Headers(opts.headers ?? {})
  if (!headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json")
  }
  if (apiKey) {
    headers.set("Authorization", `Bearer ${apiKey}`)
  }

  const res = await fetch(fullUrl, {
    ...opts,
    headers,
  })

  const ms = Math.round(performance.now() - t0)
  if (!res.ok) {
    const body = (await res
      .json()
      .catch(() => ({}) as Record<string, unknown>)) as Record<string, unknown>
    log.error(`${method} ${path} HTTP ${res.status}`, {
      durationMs: ms,
      error: body.error,
    })
    // eslint-disable-next-line @typescript-eslint/no-base-to-string
    throw new Error(String(body.error || `HTTP ${res.status}`))
  }

  log.debug(`${method} ${path} ${res.status} ${ms}ms`)
  return res.json() as Promise<T>
}

// ─── Providers ────────────────────────────────────────────────────────────────

export const listProviders = () =>
  apiFetch<Array<Provider>>("/api/admin/providers")

export const switchProvider = (providerId: string) =>
  apiFetch<{
    success?: boolean
    requiresAuth?: boolean
    provider?: { id: string; name: string }
  }>("/api/admin/providers/switch", {
    method: "POST",
    body: JSON.stringify({ providerId }),
  })

export const getProviderModels = (id: string) =>
  apiFetch<{ models: Array<Model> }>(`/api/admin/providers/${id}/models`)

export const refreshProviderModels = (id: string) =>
  apiFetch<{ models: Array<Model>; total: number }>(
    `/api/admin/providers/${id}/models/refresh`,
    { method: "POST" },
  )

export const activateProvider = (id: string) =>
  apiFetch<{ success?: boolean; provider?: { id: string; name: string } }>(
    `/api/admin/providers/${id}/activate`,
    { method: "POST" },
  )

export const deactivateProvider = (id: string) =>
  apiFetch<{ success?: boolean }>(`/api/admin/providers/${id}/deactivate`, {
    method: "POST",
  })

export const deleteProvider = (id: string) =>
  apiFetch<{ success?: boolean; message?: string }>(
    `/api/admin/providers/${id}`,
    {
      method: "DELETE",
    },
  )

export const toggleProviderModel = (
  id: string,
  modelId: string,
  enabled: boolean,
) =>
  apiFetch<{ success?: boolean; modelId: string; enabled: boolean }>(
    `/api/admin/providers/${id}/models/toggle`,
    { method: "POST", body: JSON.stringify({ modelId, enabled }) },
  )

export const getProviderPriorities = () =>
  apiFetch<{ priorities: Record<string, number> }>(
    "/api/admin/providers/priorities",
  )

export const setProviderPriorities = (priorities: Record<string, number>) =>
  apiFetch<{ success?: boolean }>("/api/admin/providers/priorities", {
    method: "POST",
    body: JSON.stringify({ priorities }),
  })

export const authProvider = (id: string, body: Record<string, string>) =>
  apiFetch<{ success?: boolean; requiresAuth?: boolean }>(
    `/api/admin/providers/${id}/auth`,
    { method: "POST", body: JSON.stringify(body) },
  )

export const authAndCreateProvider = (
  providerType: string,
  body: Record<string, string>,
) =>
  apiFetch<{
    success?: boolean
    requiresAuth?: boolean
    provider?: Provider
    pending_id?: string
    user_code?: string
    verification_uri?: string
    message?: string
  }>(`/api/admin/providers/auth-and-create/${providerType}`, {
    method: "POST",
    body: JSON.stringify(body),
  })

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const updateProviderConfig = (id: string, config: Record<string, any>) =>
  apiFetch<{ success?: boolean; config?: Provider["config"] }>(
    `/api/admin/providers/${id}/config`,
    { method: "PUT", body: JSON.stringify(config) },
  )

export const renameProvider = (
  id: string,
  fields: { name?: string; subtitle?: string },
) =>
  apiFetch<{ success?: boolean; name?: string; subtitle?: string }>(
    `/api/admin/providers/${id}/name`,
    { method: "PATCH", body: JSON.stringify(fields) },
  )

// ─── Antigravity Google OAuth ─────────────────────────────────────────────────

export const startAntigravityOAuth = (
  clientId: string,
  clientSecret: string,
  providerId?: string,
) =>
  apiFetch<{ auth_url: string; state: string; provider_id: string }>(
    "/api/admin/providers/antigravity/start-oauth",
    {
      method: "POST",
      body: JSON.stringify({
        client_id: clientId,
        client_secret: clientSecret,
        provider_id: providerId ?? "",
      }),
    },
  )

export const pollAntigravityOAuthStatus = (providerId: string) =>
  apiFetch<{
    done: boolean
    error?: string
    provider_id?: string
    is_new?: boolean
  }>(
    `/api/admin/providers/antigravity/oauth-status?provider_id=${encodeURIComponent(providerId)}`,
  )

// ─── Status / Auth flow ───────────────────────────────────────────────────────

export const getStatus = () => apiFetch<Status>("/api/admin/status")

export const getAuthStatus = () =>
  apiFetch<AuthFlow | null>("/api/admin/auth-status")

export const cancelAuth = () =>
  apiFetch<{ success: boolean }>("/api/admin/auth/cancel", { method: "POST" })

// ─── Info ─────────────────────────────────────────────────────────────────────

export const getInfo = () => apiFetch<ServerInfo>("/api/admin/info")

// ─── Usage ────────────────────────────────────────────────────────────────────

export const getUsage = () => apiFetch<UsageData>("/usage")

export const getProviderUsage = (id: string) =>
  apiFetch<UsageData>(`/api/admin/providers/${id}/usage`)

// ─── Metering ─────────────────────────────────────────────────────────────────

export interface MeteringRecord {
  id: number
  request_id: string
  model_id: string
  model_used: string
  provider_id: string
  client: string
  api_shape: string
  input_tokens: number
  output_tokens: number
  total_tokens: number
  latency_ms: number
  is_stream: boolean
  status_code: number
  error_message: string
  created_at: string
}

export interface MeteringStats {
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_tokens: number
  avg_latency_ms: number
  error_count: number
}

export interface MeteringBreakdownItem {
  model_id?: string
  provider_id?: string
  client?: string
  requests: number
  input_tokens: number
  output_tokens: number
  total_tokens: number
  avg_latency_ms: number
}

export interface MeteringLogsResponse {
  items: Array<MeteringRecord> | null
  total: number
  page: number
  page_size: number
}

export interface MeteringQuery {
  model_id?: string
  provider_id?: string
  client?: string
  api_shape?: string
  since?: string
  until?: string
  page?: number
  page_size?: number
}

function buildQueryString(params: MeteringQuery): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === "") continue
    search.set(key, String(value))
  }
  const query = search.toString()
  return query ? `?${query}` : ""
}

export const getMeteringLogs = (params: MeteringQuery = {}) =>
  apiFetch<MeteringLogsResponse>(
    `/api/admin/metering/logs${buildQueryString(params)}`,
  )

export const getMeteringStats = (params: MeteringQuery = {}) =>
  apiFetch<MeteringStats>(
    `/api/admin/metering/stats${buildQueryString(params)}`,
  )

export const getMeteringByModel = (params: MeteringQuery = {}) =>
  apiFetch<{ items: Array<MeteringBreakdownItem> | null }>(
    `/api/admin/metering/by-model${buildQueryString(params)}`,
  )

export const getMeteringByProvider = (params: MeteringQuery = {}) =>
  apiFetch<{ items: Array<MeteringBreakdownItem> | null }>(
    `/api/admin/metering/by-provider${buildQueryString(params)}`,
  )

export const getMeteringByClient = (params: MeteringQuery = {}) =>
  apiFetch<{ items: Array<MeteringBreakdownItem> | null }>(
    `/api/admin/metering/by-client${buildQueryString(params)}`,
  )

export const getMeteringModels = (params: MeteringQuery = {}) =>
  apiFetch<{ items: Array<string> | null }>(
    `/api/admin/metering/models${buildQueryString(params)}`,
  )

export const getMeteringProviders = (params: MeteringQuery = {}) =>
  apiFetch<{ items: Array<string> | null }>(
    `/api/admin/metering/providers${buildQueryString(params)}`,
  )

export const getMeteringClients = (params: MeteringQuery = {}) =>
  apiFetch<{ items: Array<string> | null }>(
    `/api/admin/metering/clients${buildQueryString(params)}`,
  )

// ─── Log level ────────────────────────────────────────────────────────────────

export const getLogLevel = async () => {
  const result = await apiFetch<{ level: unknown }>(
    "/api/admin/settings/log-level",
  )

  return { level: normalizeLogLevel(result.level) }
}

export const updateLogLevel = async (level: LogLevel) => {
  const result = await apiFetch<{ success: boolean; level: unknown }>(
    "/api/admin/settings/log-level",
    {
      method: "PUT",
      body: JSON.stringify({ level }),
    },
  )

  return {
    ...result,
    level: normalizeLogLevel(result.level),
  }
}

export async function subscribeToLogs(
  onLine: (line: string) => void,
): Promise<EventSource> {
  const backendBase = await getBackendBase()
  const url = `${backendBase}/api/admin/logs/stream`
  const apiKey = getApiKey()

  const es = new EventSource(
    apiKey ? `${url}?api_key=${encodeURIComponent(apiKey)}` : url,
  )
  es.addEventListener("message", (e: Event) => {
    const event = e as MessageEvent
    onLine(event.data as string)
  })
  es.addEventListener("error", () => {
    console.error(`❌ EventSource connection failed to ${url}`)
  })
  return es
}

export async function subscribeToLogsWebSocket(
  onLine: (line: string) => void,
): Promise<WebSocket> {
  const backendBase = await getBackendBase()
  const protocol = globalThis.location.protocol === "https:" ? "wss:" : "ws:"

  // Extract host and port from backendBase
  let host = globalThis.location.host
  if (backendBase && backendBase !== "") {
    const url = new URL(backendBase)
    host = url.host
  }

  const wsUrl = `${protocol}//${host}/api/admin/logs/websocket`
  const ws = new WebSocket(wsUrl)

  ws.addEventListener("message", (event: Event) => {
    const messageEvent = event as MessageEvent
    onLine(messageEvent.data as string)
  })
  ws.addEventListener("error", () => {
    console.error(`❌ WebSocket connection failed to ${wsUrl}`)
  })

  return ws
}

// ─── Models ────────────────────────────────────────────────────────────────

export const getModels = () =>
  apiFetch<{ object: string; data: Array<ModelInfo>; has_more: boolean }>(
    "/models",
  )

// ─── Chat Completions ──────────────────────────────────────────────────────

export const createChatCompletion = async (
  request: ChatCompletionRequest,
  apiShape: "openai" | "responses" = "openai",
) => {
  let endpoint: string
  let requestBody: Record<string, unknown>

  switch (apiShape) {
    case "responses": {
      endpoint = "/v1/responses"
      // Convert OpenAI-style messages to Responses format
      requestBody = {
        model: request.model,
        input: request.messages.map((msg) => ({
          type: "message",
          role: msg.role,
          content: msg.content,
        })),
        max_output_tokens: request.max_tokens || 1024,
        stream: request.stream || false,
        temperature: request.temperature,
      } as Record<string, unknown>
      break
    }
    default: {
      endpoint = "/v1/chat/completions"
      requestBody = request as unknown as Record<string, unknown>
      break
    }
  }

  if (request.stream) {
    // Return a ReadableStream for streaming responses
    const backendBase = await getBackendBase()
    const fullUrl = `${backendBase}${endpoint}`

    const response = await fetch(fullUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "text/event-stream",
        ...(getApiKey() ? { Authorization: `Bearer ${getApiKey()}` } : {}),
      },
      body: JSON.stringify(requestBody),
    })

    if (!response.ok) {
      const body = (await response.json().catch(() => ({}))) as Record<
        string,
        unknown
      >
      const errorMsg =
        typeof body.error === "string" ? body.error : `HTTP ${response.status}`
      throw new Error(errorMsg)
    }

    return response.body
  } else {
    // Regular API call for non-streaming
    return apiFetch<ChatApiResponse>(endpoint, {
      method: "POST",
      body: JSON.stringify(requestBody),
    })
  }
}

// ─── Chat History ──────────────────────────────────────────────────────────

export interface ChatSessionSummary {
  session_id: string
  title: string
  model_id: string
  api_shape: string
  created_at: string
  updated_at: string
}

export interface ChatSessionDetail {
  session: ChatSessionSummary
  messages: Array<{
    message_id: string
    session_id: string
    role: string
    content: string
    created_at: string
  }>
}

function normalizeSessionSummary(
  raw: Record<string, unknown>,
): ChatSessionSummary {
  const sessionId = raw.session_id ?? raw.id
  const title = raw.title
  const modelId = raw.model_id
  const apiShape = raw.api_shape ?? "openai"

  return {
    session_id: typeof sessionId === "string" ? sessionId : "",
    title: typeof title === "string" ? title : "",
    model_id: typeof modelId === "string" ? modelId : "",
    api_shape: typeof apiShape === "string" ? apiShape : "openai",
    created_at: typeof raw.created_at === "string" ? raw.created_at : "",
    updated_at: typeof raw.updated_at === "string" ? raw.updated_at : "",
  }
}

export const listChatSessions = async () => {
  const response = await apiFetch<
    | Array<Record<string, unknown>>
    | { sessions?: Array<Record<string, unknown>> }
  >("/api/admin/chat/sessions")
  const sessions =
    Array.isArray(response) ? response : (response.sessions ?? [])
  return sessions.map((s) => normalizeSessionSummary(s))
}

export const getChatSession = async (sessionId: string) => {
  const response = await apiFetch<Record<string, unknown> | ChatSessionDetail>(
    `/api/admin/chat/sessions/${sessionId}`,
  )

  // Node backend shape: { session: {...}, messages: [...] }
  if ("session" in response && response.session) {
    const typed = response as ChatSessionDetail
    return {
      session: normalizeSessionSummary(
        typed.session as unknown as Record<string, unknown>,
      ),
      messages: typed.messages.map((msg) => ({
        message_id: msg.message_id,
        session_id: msg.session_id,
        role: msg.role,
        content: msg.content,
        created_at: msg.created_at,
      })),
    }
  }

  // Go backend shape: { id, title, ..., messages: [{ id, role, content, created_at }] }
  const raw = response as Record<string, unknown>
  const rawMessages = Array.isArray(raw.messages) ? raw.messages : []
  return {
    session: normalizeSessionSummary(raw),
    messages: rawMessages.map((msg) => {
      const rawMsg = msg as Record<string, unknown>
      let messageId = ""
      if (typeof rawMsg.message_id === "string") {
        messageId = rawMsg.message_id
      } else if (typeof rawMsg.id === "string") {
        messageId = rawMsg.id
      }
      return {
        message_id: messageId,
        session_id: sessionId,
        role: typeof rawMsg.role === "string" ? rawMsg.role : "",
        content: typeof rawMsg.content === "string" ? rawMsg.content : "",
        created_at:
          typeof rawMsg.created_at === "string" ? rawMsg.created_at : "",
      }
    }),
  }
}

export const createChatSession = (body: {
  session_id: string
  title: string
  model_id: string
  api_shape: string
}) =>
  apiFetch<{ ok?: boolean; success?: boolean; session_id?: string }>(
    "/api/admin/chat/sessions",
    { method: "POST", body: JSON.stringify(body) },
  )

export const addChatMessage = (
  sessionId: string,
  body: { message_id: string; role: string; content: string },
) =>
  apiFetch<{ ok: boolean }>(`/api/admin/chat/sessions/${sessionId}/messages`, {
    method: "POST",
    body: JSON.stringify(body),
  })

export const deleteChatSession = (sessionId: string) =>
  apiFetch<{ ok: boolean }>(`/api/admin/chat/sessions/${sessionId}`, {
    method: "DELETE",
  })

export const deleteAllChatSessions = () =>
  apiFetch<{ ok: boolean }>("/api/admin/chat/sessions", { method: "DELETE" })

// ─── Virtual models ────────────────────────────────────────────────────────────

export type LbStrategy = "round-robin" | "random" | "priority" | "weighted"

export interface VirtualModelUpstream {
  id?: number
  virtual_model_id?: string
  provider_id?: string
  model_id: string
  weight: number
  priority: number
}

export interface VirtualModel {
  virtual_model_id: string
  name: string
  description: string
  api_shape: string
  lb_strategy: LbStrategy
  enabled: boolean
  created_at?: string
  updated_at?: string
  upstreams: Array<VirtualModelUpstream>
}

export interface VirtualModelPayload {
  virtual_model_id: string
  name: string
  description?: string
  api_shape?: string
  lb_strategy: LbStrategy
  enabled?: boolean
  upstreams: Array<{
    provider_id?: string
    model_id: string
    weight?: number
    priority?: number
  }>
}

export const listVirtualModels = () =>
  apiFetch<{ data: Array<VirtualModel> }>("/api/admin/virtualmodels").then(
    (r) => r.data,
  )

export const getVirtualModel = (id: string) =>
  apiFetch<VirtualModel>(`/api/admin/virtualmodels/${encodeURIComponent(id)}`)

export const createVirtualModel = (payload: VirtualModelPayload) =>
  apiFetch<VirtualModel>("/api/admin/virtualmodels", {
    method: "POST",
    body: JSON.stringify(payload),
  })

export const updateVirtualModel = (id: string, payload: VirtualModelPayload) =>
  apiFetch<VirtualModel>(`/api/admin/virtualmodels/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(payload),
  })

export const deleteVirtualModel = (id: string) =>
  apiFetch<{ deleted: string }>(
    `/api/admin/virtualmodels/${encodeURIComponent(id)}`,
    {
      method: "DELETE",
    },
  )

export const renameVirtualModel = (oldId: string, newId: string) =>
  apiFetch<VirtualModel>(
    `/api/admin/virtualmodels/${encodeURIComponent(oldId)}/rename`,
    {
      method: "POST",
      body: JSON.stringify({ new_id: newId }),
    },
  )

// ─── Config files ────────────────────────────────────────────────────────────

export interface ConfigFileEntry {
  name: string
  label: string
  description: string
  language: string
  exists: boolean
}

export interface ConfigFileContent {
  name: string
  label: string
  content: string
  exists: boolean
  message?: string
}

export const listConfigFiles = () =>
  apiFetch<{ configs: Array<ConfigFileEntry> }>("/api/admin/config")

export const getConfigFile = (name: string) =>
  apiFetch<ConfigFileContent>(`/api/admin/config/${encodeURIComponent(name)}`)

export const saveConfigFile = (name: string, content: string) =>
  apiFetch<{ success: boolean; message: string }>(
    `/api/admin/config/${encodeURIComponent(name)}`,
    {
      method: "PUT",
      body: JSON.stringify({ content }),
    },
  )

export const backupConfigFile = (name: string) =>
  apiFetch<{ success: boolean; backup: string; message: string }>(
    `/api/admin/config/${encodeURIComponent(name)}/backup`,
    { method: "POST" },
  )

// ─── Access tokens ──────────────────────────────────────────────────────────

export interface AccessToken {
  id: string
  name: string
  prefix: string
  token_plaintext?: string
  created_at: string
  expires_at: string | null
  last_used_at: string | null
  enabled: boolean
}

export interface CreateAccessTokenResponse {
  id: string
  name: string
  token: string
  prefix: string
  expires_at: string | null
  created_at: string
}

export const listAccessTokens = () =>
  apiFetch<Array<AccessToken>>("/api/admin/access-tokens")

export const createAccessToken = (name: string, expiresAt?: string) =>
  apiFetch<CreateAccessTokenResponse>("/api/admin/access-tokens", {
    method: "POST",
    body: JSON.stringify({ name, expires_at: expiresAt }),
  })

export const deleteAccessToken = (id: string) =>
  apiFetch<{ deleted: boolean }>(`/api/admin/access-tokens/${id}`, {
    method: "DELETE",
  })
