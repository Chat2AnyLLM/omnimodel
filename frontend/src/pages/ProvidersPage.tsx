/* eslint-disable @typescript-eslint/no-unnecessary-condition,@typescript-eslint/no-floating-promises,no-nested-ternary,unicorn/consistent-function-scoping */
import { Eye, EyeOff } from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"

import {
  activateProvider,
  authAndCreateProvider,
  authProvider,
  cancelAuth,
  deactivateProvider,
  deleteProvider,
  getAuthStatus,
  getProviderModels,
  getProviderUsage,
  listProviders,
  getStatus,
  toggleProviderModel,
  getProviderPriorities,
  setProviderPriorities,
  startAntigravityOAuth,
  pollAntigravityOAuthStatus,
  updateProviderConfig,
  refreshProviderModels,
  renameProvider,
  type AuthFlow,
  type Model,
  type Provider,
  type Status,
  type UsageData,
} from "@/api"
import { EmptyState } from "@/components/EmptyState"
import { Spinner } from "@/components/Spinner"
import { createLogger } from "@/lib/logger"
import { useConfirm } from "@/lib/useConfirm"
import { AuthFlowBanner } from "@/pages/providers/components/AuthFlowBanner"
import { GroupHeader } from "@/pages/providers/components/GroupHeader"
import { PriorityModal } from "@/pages/providers/components/PriorityModal"
import { StatsBar } from "@/pages/providers/components/StatsBar"
import { UsageDialog } from "@/pages/providers/components/UsageDialog"
import {
  PROVIDER_ACCENT,
  PROVIDER_ICONS,
  PROVIDER_TYPES as PROVIDER_TYPE_IDS,
  TYPE_NAMES,
} from "@/pages/providers/constants/providerRegistry"

const _log = createLogger("providers-page")

interface ProvidersPageProps {
  showToast: (msg: string, type?: "success" | "error") => void
}

function Spin({ size = 14 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 16 16"
      className="animate-spin"
      style={{ flexShrink: 0 }}
    >
      <circle
        cx="8"
        cy="8"
        r="6"
        stroke="currentColor"
        strokeWidth="2"
        strokeDasharray="28"
        strokeDashoffset="10"
        fill="none"
        opacity="0.6"
      />
    </svg>
  )
}

interface AzureDeploymentMapping {
  model: string
  deployment: string
}

function normalizeAzureDeploymentMappings(
  value: unknown,
): Array<AzureDeploymentMapping> {
  if (!Array.isArray(value)) return []
  return value.flatMap((item) => {
    if (typeof item === "string") {
      const trimmed = item.trim()
      return trimmed ? [{ model: trimmed, deployment: trimmed }] : []
    }
    if (item && typeof item === "object") {
      const model =
        typeof (item as { model?: unknown }).model === "string" ?
          (item as { model: string }).model.trim()
        : ""
      const deployment =
        typeof (item as { deployment?: unknown }).deployment === "string" ?
          (item as { deployment: string }).deployment.trim()
        : ""
      return model && deployment ? [{ model, deployment }] : []
    }
    return []
  })
}

function serializeAzureDeploymentMappings(
  mappings: Array<AzureDeploymentMapping>,
): string {
  return JSON.stringify(
    mappings
      .map((mapping) => ({
        model: mapping.model.trim(),
        deployment: mapping.deployment.trim(),
      }))
      .filter((mapping) => mapping.model && mapping.deployment),
  )
}

// ─── Inline Auth Forms ────────────────────────────────────────────────────────

function FormRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      <label
        style={{
          fontSize: 12,
          fontWeight: 600,
          color: "var(--color-text-secondary)",
          letterSpacing: "0.01em",
          textTransform: "uppercase",
        }}
      >
        {label}
      </label>
      {children}
    </div>
  )
}

function SecretInput({
  value,
  onChange,
  placeholder,
  className,
  style,
  autoComplete = "off",
}: {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  className?: string
  style?: React.CSSProperties
  autoComplete?: string
}) {
  const [visible, setVisible] = useState(false)

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 8,
        width: "100%",
      }}
    >
      <input
        className={className}
        type={visible ? "text" : "password"}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        style={{ ...style, flex: 1, minWidth: 0 }}
        autoComplete={autoComplete}
        spellCheck={false}
      />
      <button
        type="button"
        onClick={() => setVisible((current) => !current)}
        aria-label={visible ? "Hide secret value" : "Show secret value"}
        title={visible ? "Hide" : "Show"}
        style={{
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          width: 40,
          minWidth: 40,
          height: 40,
          borderRadius: "var(--radius-md)",
          border: "1px solid var(--color-separator)",
          background: "rgba(255,255,255,0.045)",
          color: "var(--color-text-secondary)",
          cursor: "pointer",
        }}
      >
        {visible ?
          <EyeOff size={16} />
        : <Eye size={16} />}
      </button>
    </div>
  )
}

const addFlowPanelStyle = {
  background:
    "linear-gradient(180deg, rgba(255,255,255,0.045) 0%, rgba(255,255,255,0.025) 100%)",
  border: "1px solid var(--color-separator)",
  borderRadius: "var(--radius-lg)",
  padding: 18,
  display: "flex",
  flexDirection: "column",
  gap: 16,
  boxShadow: "0 18px 44px rgba(0,0,0,0.16)",
} as const

const addFlowControlStyle = {
  width: "100%",
  minHeight: 44,
  borderRadius: "var(--radius-md)",
  border: "1px solid var(--color-separator)",
  background: "rgba(255,255,255,0.045)",
  color: "var(--color-text)",
  padding: "0 14px",
  outline: "none",
  fontSize: 13,
  lineHeight: 1.4,
} as const

const addFlowTextInputStyle = {
  ...addFlowControlStyle,
  padding: "11px 14px",
} as const

type OpenAICompatibleAPIFormat = "" | "chat.completions" | "responses"

const OPENAI_COMPATIBLE_API_FORMAT_OPTIONS: Array<{
  value: OpenAICompatibleAPIFormat
  label: string
}> = [
  { value: "", label: "Auto (recommended)" },
  { value: "chat.completions", label: "Chat Completions" },
  { value: "responses", label: "Responses" },
]

function normalizeOpenAICompatibleAPIFormat(
  value: string | undefined,
): OpenAICompatibleAPIFormat {
  if (value === "chat.completions" || value === "responses") {
    return value
  }
  return ""
}

function OpenAICompatibleAPIFormatControl({
  value,
  onChange,
  disabled = false,
}: {
  value: OpenAICompatibleAPIFormat
  onChange: (next: OpenAICompatibleAPIFormat) => void
  disabled?: boolean
}) {
  return (
    <select
      value={value}
      onChange={(e) =>
        onChange(normalizeOpenAICompatibleAPIFormat(e.target.value))
      }
      disabled={disabled}
      style={addFlowControlStyle}
    >
      {OPENAI_COMPATIBLE_API_FORMAT_OPTIONS.map((option) => (
        <option key={option.value || "auto"} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  )
}

function AddFlowChoiceGroup({
  value,
  onChange,
  options,
  accent = "#0a84ff",
}: {
  value: string
  onChange: (next: string) => void
  options: Array<{ value: string; label: string }>
  accent?: string
}) {
  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: `repeat(${options.length}, minmax(0, 1fr))`,
        gap: 8,
      }}
    >
      {options.map((option) => {
        const active = value === option.value
        return (
          <button
            key={option.value}
            type="button"
            onClick={() => onChange(option.value)}
            style={{
              minHeight: 44,
              borderRadius: "var(--radius-md)",
              border:
                active ?
                  `1px solid ${accent}66`
                : "1px solid var(--color-separator)",
              background: active ? `${accent}18` : "rgba(255,255,255,0.03)",
              color:
                active ? "var(--color-text)" : "var(--color-text-secondary)",
              fontSize: 13,
              fontWeight: active ? 600 : 500,
              cursor: "pointer",
              transition: "all 0.15s var(--ease)",
              padding: "0 12px",
              textAlign: "center",
            }}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}

function AddFlowHint({
  children,
  tone = "neutral",
}: {
  children: React.ReactNode
  tone?: "neutral" | "warning"
}) {
  const palette =
    tone === "warning" ?
      {
        background: "rgba(255,159,10,0.08)",
        border: "1px solid rgba(255,159,10,0.18)",
        color: "var(--color-text-secondary)",
      }
    : {
        background: "rgba(255,255,255,0.03)",
        border: "1px solid var(--color-separator)",
        color: "var(--color-text-secondary)",
      }

  return (
    <div
      style={{
        ...palette,
        borderRadius: "var(--radius-md)",
        padding: "12px 14px",
        fontSize: 13,
        lineHeight: 1.5,
      }}
    >
      {children}
    </div>
  )
}

function AuthFormWrapper({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <div
      style={{
        marginTop: 16,
        background: "rgba(10,132,255,0.06)",
        border: "1px solid rgba(10,132,255,0.18)",
        borderRadius: "var(--radius-lg)",
        padding: 16,
        display: "flex",
        flexDirection: "column",
        gap: 12,
        animation: "slide-up 0.18s var(--ease) both",
      }}
    >
      <div
        style={{ fontSize: 13, fontWeight: 600, color: "var(--color-blue)" }}
      >
        {title}
      </div>
      {children}
    </div>
  )
}

function AlibabaAuthForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [region, setRegion] = useState("global")
  const [apiKey, setApiKey] = useState("")
  const submit = async () => {
    if (!apiKey.trim()) return
    await onSubmit({
      method: "api-key",
      apiKey: apiKey.trim(),
      region,
    })
  }
  return (
    <AuthFormWrapper title="Authenticate Alibaba DashScope">
      <div
        style={{
          fontSize: 12,
          color: "var(--color-text-tertiary)",
          marginBottom: 12,
        }}
      >
        Only API key authentication is supported for Alibaba DashScope.
      </div>
      <FormRow label="Region">
        <select
          className="sys-select"
          value={region}
          onChange={(e) => setRegion(e.target.value)}
        >
          <option value="global">Global (dashscope-intl.aliyuncs.com)</option>
          <option value="china">China (dashscope.aliyuncs.com)</option>
        </select>
      </FormRow>
      <FormRow label="DashScope API Key">
        <SecretInput
          className="sys-input"
          placeholder="sk-…"
          value={apiKey}
          onChange={setApiKey}
        />
      </FormRow>
      <div style={{ display: "flex", gap: 8 }}>
        <button className="btn btn-primary btn-sm" onClick={submit}>
          Submit
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

function ModelScopeAuthForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [apiKey, setApiKey] = useState("")
  const submit = async () => {
    if (!apiKey.trim()) return
    await onSubmit({
      method: "api-key",
      apiKey: apiKey.trim(),
    })
  }
  return (
    <AuthFormWrapper title="Authenticate ModelScope">
      <div
        style={{
          fontSize: 12,
          color: "var(--color-text-tertiary)",
          marginBottom: 12,
        }}
      >
        Only API key authentication is supported for ModelScope.
      </div>
      <FormRow label="ModelScope API Key">
        <SecretInput
          className="sys-input"
          placeholder="sk-…"
          value={apiKey}
          onChange={setApiKey}
        />
      </FormRow>
      <div style={{ display: "flex", gap: 8 }}>
        <button className="btn btn-primary btn-sm" onClick={submit}>
          Submit
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

function CopilotAuthForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [token, setToken] = useState("")
  const submit = async () => {
    if (!token.trim()) return
    await onSubmit({ method: "token", token: token.trim() })
  }
  return (
    <AuthFormWrapper title="Authenticate GitHub Copilot">
      <FormRow label="GitHub Token">
        <SecretInput
          className="sys-input"
          placeholder="ghu_…"
          value={token}
          onChange={setToken}
        />
      </FormRow>
      <div style={{ display: "flex", gap: 8 }}>
        <button className="btn btn-primary btn-sm" onClick={submit}>
          Submit
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

function CodexAuthForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [apiKey, setApiKey] = useState("")

  const submit = async () => {
    if (!apiKey.trim()) return
    await onSubmit({ method: "api-key", apiKey: apiKey.trim() })
  }

  return (
    <AuthFormWrapper title="Authenticate Codex">
      <div
        style={{
          fontSize: 12,
          color: "var(--color-text-tertiary)",
          marginBottom: 12,
        }}
      >
        Official Codex supports ChatGPT sign-in and API key authentication. Use
        an OpenAI API key here for programmatic access.
      </div>
      <FormRow label="OpenAI API Key">
        <SecretInput
          className="sys-input"
          placeholder="sk-..."
          value={apiKey}
          onChange={setApiKey}
        />
      </FormRow>
      <div style={{ display: "flex", gap: 8 }}>
        <button className="btn btn-primary btn-sm" onClick={submit}>
          Save API Key
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

function useAntigravityOAuth(
  providerId: string | undefined,
  onSuccess: () => void,
  onError: (msg: string) => void,
) {
  const startOAuth = async (clientId: string, clientSecret: string) => {
    const resp = await startAntigravityOAuth(clientId, clientSecret, providerId)
    const tab = window.open(resp.auth_url, "_blank")
    if (!tab) {
      onError(
        "Could not open a new tab — please allow popups for this page and try again",
      )
      return
    }

    // Poll the backend for completion instead of relying on window.opener/postMessage,
    // which is severed by Google's Cross-Origin-Opener-Policy header.
    const timer = setInterval(async () => {
      try {
        const status = await pollAntigravityOAuthStatus(resp.provider_id)
        if (!status.done) return
        clearInterval(timer)
        if (status.error) {
          onError(status.error)
        } else {
          onSuccess()
        }
      } catch {
        // ignore transient fetch errors while polling
      }
      // Also stop polling if the user closed the tab without completing
      if (tab.closed) {
        clearInterval(timer)
      }
    }, 1500)

    // Safety: stop polling after 10 minutes regardless
    setTimeout(() => clearInterval(timer), 10 * 60 * 1000)
  }
  return { startOAuth }
}

function AntigravityAuthForm({
  provider,
  onSuccess,
  onCancel,
}: {
  provider: Provider
  onSuccess: () => void
  onCancel: () => void
}) {
  const [clientId, setClientId] = useState("")
  const [clientSecret, setClientSecret] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const { startOAuth } = useAntigravityOAuth(
    provider.id,
    () => {
      setLoading(false)
      onSuccess()
    },
    (msg) => {
      setLoading(false)
      setError(msg)
    },
  )

  const submit = async () => {
    if (!clientId.trim() || !clientSecret.trim()) return
    setLoading(true)
    setError(null)
    try {
      await startOAuth(clientId.trim(), clientSecret.trim())
    } catch (e) {
      setLoading(false)
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <AuthFormWrapper title="Authenticate Antigravity (Google)">
      {error && (
        <div
          style={{ fontSize: 12, color: "var(--color-error)", marginBottom: 8 }}
        >
          {error}
        </div>
      )}
      <FormRow label="OAuth Client ID">
        <input
          className="sys-input"
          type="text"
          placeholder="…apps.googleusercontent.com"
          value={clientId}
          onChange={(e) => setClientId(e.target.value)}
        />
      </FormRow>
      <FormRow label="OAuth Client Secret">
        <SecretInput
          className="sys-input"
          placeholder="GOCSPX-…"
          value={clientSecret}
          onChange={setClientSecret}
        />
      </FormRow>
      <div style={{ fontSize: 12, color: "var(--color-text-tertiary)" }}>
        Opens a Google OAuth browser popup to complete authentication.
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        <button
          className="btn btn-primary btn-sm"
          onClick={submit}
          disabled={loading}
        >
          {loading ?
            <>
              <Spin size={13} /> Waiting…
            </>
          : "Start OAuth →"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={loading}
        >
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

function AzureOpenAIAuthForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [apiKey, setApiKey] = useState("")
  const [endpoint, setEndpoint] = useState("")
  const [apiVersion, setApiVersion] = useState("")
  const [mappings, setMappings] = useState<Array<AzureDeploymentMapping>>([
    { model: "", deployment: "" },
  ])

  const updateMapping = (
    index: number,
    field: keyof AzureDeploymentMapping,
    value: string,
  ) => {
    setMappings((prev) =>
      prev.map((mapping, i) =>
        i === index ? { ...mapping, [field]: value } : mapping,
      ),
    )
  }

  const addMapping = () => {
    setMappings((prev) => [...prev, { model: "", deployment: "" }])
  }

  const removeMapping = (index: number) => {
    setMappings((prev) =>
      prev.length === 1 ?
        [{ model: "", deployment: "" }]
      : prev.filter((_, i) => i !== index),
    )
  }

  const submit = async () => {
    if (!apiKey.trim()) return

    await onSubmit({
      apiKey: apiKey.trim(),
      endpoint: endpoint.trim(),
      apiVersion: apiVersion.trim() || "2024-02-01",
      deployments: serializeAzureDeploymentMappings(mappings),
    })
  }

  return (
    <AuthFormWrapper title="Authenticate Azure OpenAI">
      <FormRow label="API Key *">
        <SecretInput
          className="sys-input"
          placeholder="Enter your Azure OpenAI API key"
          value={apiKey}
          onChange={setApiKey}
        />
      </FormRow>

      <FormRow label="Endpoint (optional)">
        <input
          className="sys-input"
          type="text"
          placeholder="https://your-resource.openai.azure.com"
          value={endpoint}
          onChange={(e) => setEndpoint(e.target.value)}
        />
      </FormRow>

      <FormRow label="API Version (optional)">
        <input
          className="sys-input"
          type="text"
          placeholder="2024-02-01"
          value={apiVersion}
          onChange={(e) => setApiVersion(e.target.value)}
        />
      </FormRow>

      <FormRow label="Models + Deployments (optional)">
        <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
          {mappings.map((mapping, index) => (
            <div
              key={index}
              style={{
                display: "grid",
                gridTemplateColumns: "1fr 1fr auto",
                gap: 8,
                alignItems: "center",
              }}
            >
              <input
                className="sys-input"
                type="text"
                placeholder="Model name (e.g. gpt-5.4)"
                value={mapping.model}
                onChange={(e) => updateMapping(index, "model", e.target.value)}
              />
              <input
                className="sys-input"
                type="text"
                placeholder="Deployment name"
                value={mapping.deployment}
                onChange={(e) =>
                  updateMapping(index, "deployment", e.target.value)
                }
              />
              <button
                className="btn btn-ghost btn-sm"
                onClick={() => removeMapping(index)}
                type="button"
              >
                Remove
              </button>
            </div>
          ))}
          <button
            className="btn btn-ghost btn-sm"
            onClick={addMapping}
            type="button"
            style={{ alignSelf: "flex-start" }}
          >
            + Add model mapping
          </button>
        </div>
      </FormRow>

      <div style={{ fontSize: 12, color: "var(--color-text-tertiary)" }}>
        Configure app-facing model names separately from Azure deployment names.
        You can also update mappings later via the provider’s Models menu.
      </div>

      <div style={{ display: "flex", gap: 8 }}>
        <button className="btn btn-primary btn-sm" onClick={submit}>
          Submit
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

// ─── Models Dialog ────────────────────────────────────────────────────────────

function _ModelsDialog({
  provider,
  onModelsChanged,
}: {
  provider: Provider
  onModelsChanged?: () => void
}) {
  const [models, setModels] = useState<Array<Model> | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const [configLoading, setConfigLoading] = useState(false)

  // Azure OpenAI deployment management
  const [azureMappings, setAzureMappings] = useState<
    Array<AzureDeploymentMapping>
  >(normalizeAzureDeploymentMappings(provider.config?.deployments))

  // OpenAI-compatible user-defined model management
  const [newModel, setNewModel] = useState("")
  const [userModels, setUserModels] = useState<Array<string>>(
    (provider.config?.models as Array<string>) || [],
  )
  const [apiFormat, setApiFormat] = useState<OpenAICompatibleAPIFormat>(
    normalizeOpenAICompatibleAPIFormat(provider.config?.apiFormat),
  )

  useEffect(() => {
    if (provider.type === "azure-openai") {
      setAzureMappings(
        normalizeAzureDeploymentMappings(provider.config?.deployments),
      )
    }
    if (provider.type === "openai-compatible") {
      setUserModels((provider.config?.models as Array<string>) || [])
      setApiFormat(
        normalizeOpenAICompatibleAPIFormat(provider.config?.apiFormat),
      )
    }
  }, [provider])

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const resp = await getProviderModels(provider.id)
      setModels(resp?.models ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const handleOpen = () => {
    setOpen(true)
    if (models === null && !loading) load()
    if (provider.type === "azure-openai") {
      setAzureMappings(
        normalizeAzureDeploymentMappings(provider.config?.deployments),
      )
    }
    if (provider.type === "openai-compatible") {
      setUserModels((provider.config?.models as Array<string>) || [])
      setApiFormat(
        normalizeOpenAICompatibleAPIFormat(provider.config?.apiFormat),
      )
    }
  }

  const handleToggle = async (model: Model) => {
    const newEnabled = !model.enabled
    setModels((prev) =>
      prev ?
        prev.map((m) => (m.id === model.id ? { ...m, enabled: newEnabled } : m))
      : prev,
    )
    try {
      await toggleProviderModel(provider.id, model.id, newEnabled)
      onModelsChanged?.()
    } catch {
      setModels((prev) =>
        prev ?
          prev.map((m) =>
            m.id === model.id ? { ...m, enabled: model.enabled } : m,
          )
        : prev,
      )
    }
  }

  const saveAzureMappings = async (
    nextMappings: Array<AzureDeploymentMapping>,
  ) => {
    if (provider.type !== "azure-openai") return

    setConfigLoading(true)
    setError(null)
    try {
      await updateProviderConfig(provider.id, {
        endpoint: provider.config?.endpoint,
        apiVersion: provider.config?.apiVersion || "2024-02-01",
        deployments: nextMappings
          .map((mapping) => ({
            model: mapping.model.trim(),
            deployment: mapping.deployment.trim(),
          }))
          .filter((mapping) => mapping.model && mapping.deployment),
      })
      setAzureMappings(nextMappings)
      onModelsChanged?.()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const updateAzureMapping = (
    index: number,
    field: keyof AzureDeploymentMapping,
    value: string,
  ) => {
    setAzureMappings((prev) =>
      prev.map((mapping, i) =>
        i === index ? { ...mapping, [field]: value } : mapping,
      ),
    )
  }

  const addAzureMappingRow = () => {
    setAzureMappings((prev) => [...prev, { model: "", deployment: "" }])
  }

  const removeAzureMappingRow = async (index: number) => {
    if (provider.type !== "azure-openai") return
    const nextMappings = azureMappings.filter((_, i) => i !== index)
    await saveAzureMappings(nextMappings)
  }

  const persistAzureMappings = async () => {
    await saveAzureMappings(azureMappings)
  }

  const handleAddUserModel = async () => {
    if (!newModel.trim() || provider.type !== "openai-compatible") return
    const ids = newModel
      .trim()
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
    if (ids.length === 0) return
    const alreadyExists = ids.some((id) => userModels.includes(id))
    if (alreadyExists) {
      setError("One or more models already in list")
      return
    }
    setConfigLoading(true)
    setError(null)
    try {
      const updated = [...userModels, ...ids]
      await updateProviderConfig(provider.id, { models: updated })
      setUserModels(updated)
      setNewModel("")
      onModelsChanged?.()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleRemoveUserModel = async (modelID: string) => {
    if (provider.type !== "openai-compatible") return
    setConfigLoading(true)
    setError(null)
    try {
      const updated = userModels.filter((m) => m !== modelID)
      await updateProviderConfig(provider.id, { models: updated })
      setUserModels(updated)
      onModelsChanged?.()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleUpdateAPIFormat = async (
    nextFormat: OpenAICompatibleAPIFormat,
  ) => {
    if (provider.type !== "openai-compatible" || nextFormat === apiFormat) {
      return
    }
    const previousFormat = apiFormat
    setConfigLoading(true)
    setError(null)
    setApiFormat(nextFormat)
    try {
      await updateProviderConfig(provider.id, { apiFormat: nextFormat })
      onModelsChanged?.()
      await load()
    } catch (e) {
      setApiFormat(previousFormat)
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleSelectAll = async () => {
    if (!models) return
    const toToggle = models.filter((m) => !m.enabled)
    setModels((prev) =>
      prev ? prev.map((m) => ({ ...m, enabled: true })) : prev,
    )
    try {
      await Promise.all(
        toToggle.map((m) => toggleProviderModel(provider.id, m.id, true)),
      )
      onModelsChanged?.()
    } catch {
      await load()
    }
  }

  const handleDeselectAll = async () => {
    if (!models) return
    const toToggle = models.filter((m) => m.enabled)
    setModels((prev) =>
      prev ? prev.map((m) => ({ ...m, enabled: false })) : prev,
    )
    try {
      await Promise.all(
        toToggle.map((m) => toggleProviderModel(provider.id, m.id, false)),
      )
      onModelsChanged?.()
    } catch {
      await load()
    }
  }

  const filtered =
    models ?
      models.filter((m) => m.id.toLowerCase().includes(search.toLowerCase()))
    : []
  const enabledCount = models ? models.filter((m) => m.enabled).length : null

  return (
    <>
      <button className="btn btn-ghost btn-sm" onClick={handleOpen}>
        Models
        {provider.totalModelCount !== undefined
          && provider.totalModelCount > 0 && (
            <span
              style={{
                color: "var(--color-blue)",
                fontSize: 11,
                fontWeight: 600,
              }}
            >
              {provider.enabledModelCount}/{provider.totalModelCount}
            </span>
          )}
      </button>

      {open && (
        <div
          className="dialog-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div className="dialog-box">
            <div className="dialog-header">
              <div>
                <div
                  style={{
                    fontFamily: "var(--font-display)",
                    fontWeight: 600,
                    fontSize: 15,
                    color: "var(--color-text)",
                  }}
                >
                  {provider.name} — Models
                </div>
                {enabledCount !== null && models && (
                  <div
                    style={{
                      fontSize: 12,
                      color: "var(--color-text-secondary)",
                      marginTop: 2,
                    }}
                  >
                    <span style={{ color: "var(--color-green)" }}>
                      {enabledCount}
                    </span>{" "}
                    of {models.length} enabled
                  </div>
                )}
              </div>
              <div style={{ display: "flex", gap: 8 }}>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={async () => {
                    setLoading(true)
                    setError(null)
                    try {
                      const resp = await refreshProviderModels(provider.id)
                      setModels(resp?.models ?? [])
                      onModelsChanged?.()
                    } catch (e) {
                      setError(e instanceof Error ? e.message : String(e))
                    } finally {
                      setLoading(false)
                    }
                  }}
                  disabled={loading}
                >
                  Refresh
                </button>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => setOpen(false)}
                >
                  Done
                </button>
              </div>
            </div>
            <div className="dialog-body">
              {loading && (
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    padding: "32px 0",
                    justifyContent: "center",
                    color: "var(--color-text-secondary)",
                    fontSize: 14,
                  }}
                >
                  <Spin /> Loading models…
                </div>
              )}
              {error && (
                <div
                  style={{
                    color: "var(--color-red)",
                    fontSize: 13,
                    padding: "12px 0",
                  }}
                >
                  {error}
                </div>
              )}
              {models && !loading && (
                <div
                  style={{ display: "flex", flexDirection: "column", gap: 14 }}
                >
                  {/* Azure OpenAI deployment management */}
                  {provider.type === "azure-openai" && (
                    <div
                      style={{
                        padding: "14px 16px",
                        background: "rgba(10,132,255,0.06)",
                        border: "1px solid rgba(10,132,255,0.18)",
                        borderRadius: "var(--radius-lg)",
                      }}
                    >
                      <div
                        style={{
                          fontSize: 13,
                          fontWeight: 600,
                          color: "var(--color-blue)",
                          marginBottom: 12,
                        }}
                      >
                        Model ↔ Deployment Mapping
                      </div>
                      <div
                        style={{
                          display: "flex",
                          flexDirection: "column",
                          gap: 8,
                          marginBottom: 10,
                        }}
                      >
                        {azureMappings.map((mapping, index) => (
                          <div
                            key={index}
                            style={{
                              display: "grid",
                              gridTemplateColumns: "1fr 1fr auto",
                              gap: 8,
                              alignItems: "center",
                            }}
                          >
                            <input
                              className="sys-input"
                              placeholder="Model name"
                              value={mapping.model}
                              onChange={(e) =>
                                updateAzureMapping(
                                  index,
                                  "model",
                                  e.target.value,
                                )
                              }
                              disabled={configLoading}
                              style={{ fontSize: 13 }}
                            />
                            <input
                              className="sys-input"
                              placeholder="Deployment name"
                              value={mapping.deployment}
                              onChange={(e) =>
                                updateAzureMapping(
                                  index,
                                  "deployment",
                                  e.target.value,
                                )
                              }
                              disabled={configLoading}
                              style={{ fontSize: 13 }}
                            />
                            <button
                              className="btn btn-ghost btn-sm"
                              onClick={() => removeAzureMappingRow(index)}
                              disabled={configLoading}
                              type="button"
                            >
                              Remove
                            </button>
                          </div>
                        ))}
                      </div>
                      <div style={{ display: "flex", gap: 8, marginBottom: 8 }}>
                        <button
                          className="btn btn-ghost btn-sm"
                          onClick={addAzureMappingRow}
                          disabled={configLoading}
                          type="button"
                        >
                          + Add mapping
                        </button>
                        <button
                          className="btn btn-primary btn-sm"
                          onClick={persistAzureMappings}
                          disabled={configLoading}
                          type="button"
                        >
                          {configLoading ?
                            <Spin size={12} />
                          : "Save mappings"}
                        </button>
                      </div>
                      <div
                        style={{
                          fontSize: 11,
                          color: "var(--color-text-tertiary)",
                        }}
                      >
                        Model names are shown in the app. Deployment names are
                        sent to Azure at request time.
                      </div>
                    </div>
                  )}

                  {/* OpenAI-compatible user-defined model management */}
                  {provider.type === "openai-compatible" && (
                    <div
                      style={{
                        padding: "14px 16px",
                        background: "rgba(16,185,129,0.06)",
                        border: "1px solid rgba(16,185,129,0.18)",
                        borderRadius: "var(--radius-lg)",
                      }}
                    >
                      <div
                        style={{
                          fontSize: 13,
                          fontWeight: 600,
                          color: "#10b981",
                          marginBottom: 12,
                        }}
                      >
                        Model IDs
                      </div>
                      <div
                        style={{
                          display: "flex",
                          flexDirection: "column",
                          gap: 8,
                          marginBottom: 12,
                        }}
                      >
                        <div
                          style={{
                            fontSize: 12,
                            fontWeight: 600,
                            color: "#10b981",
                          }}
                        >
                          Upstream API
                        </div>
                        <OpenAICompatibleAPIFormatControl
                          value={apiFormat}
                          onChange={handleUpdateAPIFormat}
                          disabled={configLoading}
                        />
                        <div
                          style={{
                            fontSize: 11,
                            color: "var(--color-text-tertiary)",
                          }}
                        >
                          Auto uses <code>/v1/responses</code> for official
                          OpenAI when requests arrive via{" "}
                          <code>/v1/messages</code> or{" "}
                          <code>/v1/responses</code>, and{" "}
                          <code>/v1/chat/completions</code> otherwise.
                        </div>
                      </div>
                      <div
                        style={{
                          display: "flex",
                          gap: 8,
                          alignItems: "center",
                          marginBottom: 8,
                        }}
                      >
                        <input
                          className="sys-input"
                          placeholder="Add model ID (e.g. llama3, mistral)..."
                          value={newModel}
                          onChange={(e) => setNewModel(e.target.value)}
                          disabled={configLoading}
                          style={{ flex: 1, fontSize: 13 }}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") {
                              e.preventDefault()
                              handleAddUserModel()
                            }
                          }}
                        />
                        <button
                          className="btn btn-primary btn-sm"
                          onClick={handleAddUserModel}
                          disabled={configLoading || !newModel.trim()}
                          style={{ minWidth: 32, padding: "6px 8px" }}
                        >
                          {configLoading ?
                            <Spin size={12} />
                          : "+"}
                        </button>
                      </div>
                      <div
                        style={{
                          fontSize: 11,
                          color: "var(--color-text-tertiary)",
                        }}
                      >
                        Enter model IDs to use from this endpoint. These are
                        merged with any models returned by <code>/models</code>.
                      </div>
                    </div>
                  )}

                  <div
                    style={{ display: "flex", gap: 8, alignItems: "center" }}
                  >
                    <input
                      className="sys-input"
                      placeholder="Filter models…"
                      value={search}
                      onChange={(e) => setSearch(e.target.value)}
                      style={{ flex: 1 }}
                    />
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={handleSelectAll}
                    >
                      All On
                    </button>
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={handleDeselectAll}
                    >
                      All Off
                    </button>
                  </div>
                  <div
                    style={{
                      borderRadius: "var(--radius-lg)",
                      overflow: "hidden",
                      border: "1px solid var(--color-separator)",
                    }}
                  >
                    {filtered.map((m, i) => (
                      <div
                        key={m.id}
                        style={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          padding: "10px 14px",
                          borderBottom:
                            i < filtered.length - 1 ?
                              "1px solid var(--color-separator)"
                            : "none",
                          background:
                            m.enabled ? "rgba(48,209,88,0.04)" : "transparent",
                          transition: "background 0.12s",
                        }}
                      >
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 12,
                            flex: 1,
                          }}
                        >
                          <div>
                            <div
                              style={{
                                fontFamily: "var(--font-mono)",
                                fontSize: 13,
                                color:
                                  m.enabled ? "var(--color-text)" : (
                                    "var(--color-text-secondary)"
                                  ),
                              }}
                            >
                              {m.id}
                            </div>
                            {m.name && m.name !== m.id && (
                              <div
                                style={{
                                  fontSize: 11,
                                  color: "var(--color-text-tertiary)",
                                  marginTop: 1,
                                }}
                              >
                                {m.name}
                              </div>
                            )}
                          </div>
                        </div>
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 8,
                          }}
                        >
                          {/* Remove deployment button for Azure OpenAI */}
                          {provider.type === "azure-openai"
                            && azureMappings.some(
                              (mapping) =>
                                mapping.deployment === m.id
                                || mapping.model === m.id,
                            ) && (
                              <button
                                className="btn btn-ghost btn-sm"
                                onClick={() => {
                                  const mappingIndex = azureMappings.findIndex(
                                    (mapping) =>
                                      mapping.deployment === m.id
                                      || mapping.model === m.id,
                                  )
                                  if (mappingIndex !== -1) {
                                    void removeAzureMappingRow(mappingIndex)
                                  }
                                }}
                                disabled={configLoading}
                                style={{
                                  minWidth: 32,
                                  padding: "4px 6px",
                                  color: "var(--color-red)",
                                }}
                                title={`Remove deployment ${m.id}`}
                              >
                                {configLoading ?
                                  <Spin size={10} />
                                : "−"}
                              </button>
                            )}
                          {/* Remove model button for openai-compatible */}
                          {provider.type === "openai-compatible"
                            && userModels.includes(m.id) && (
                              <button
                                className="btn btn-ghost btn-sm"
                                onClick={() => handleRemoveUserModel(m.id)}
                                disabled={configLoading}
                                style={{
                                  minWidth: 32,
                                  padding: "4px 6px",
                                  color: "var(--color-red)",
                                }}
                                title={`Remove model ${m.id}`}
                              >
                                {configLoading ?
                                  <Spin size={10} />
                                : "−"}
                              </button>
                            )}
                          <div
                            className={`toggle-track ${m.enabled ? "on" : ""}`}
                            onClick={() => handleToggle(m)}
                          >
                            <div className="toggle-thumb" />
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  )
}

// ─── Priority Modal ───────────────────────────────────────────────────────────

// ─── Menu Item Wrappers for Models Dialog ─────────────────────────────────────

function ModelsMenuItem({
  provider,
  onModelsChanged,
}: {
  provider: Provider
  onModelsChanged?: () => void
}) {
  const [models, setModels] = useState<Array<Model> | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const [configLoading, setConfigLoading] = useState(false)
  const [newDeployment, setNewDeployment] = useState("")
  const [azureMappings, setAzureMappings] = useState<
    Array<AzureDeploymentMapping>
  >(normalizeAzureDeploymentMappings(provider.config?.deployments))
  const [azureEndpoint, setAzureEndpoint] = useState<string | undefined>(
    provider.config?.endpoint,
  )
  const [azureAPIVersion, setAzureAPIVersion] = useState<string>(
    provider.config?.apiVersion || "2024-02-01",
  )

  // OpenAI-compatible user-defined model management
  const [newModel, setNewModel] = useState("")
  const [userModels, setUserModels] = useState<Array<string>>(
    (provider.config?.models as Array<string>) || [],
  )
  const [apiFormat, setApiFormat] = useState<OpenAICompatibleAPIFormat>(
    normalizeOpenAICompatibleAPIFormat(provider.config?.apiFormat),
  )
  const deployments = new Set(
    azureMappings.map((mapping) => mapping.deployment),
  )

  useEffect(() => {
    if (provider.type === "azure-openai") {
      setAzureMappings(
        normalizeAzureDeploymentMappings(provider.config?.deployments),
      )
      setAzureEndpoint(provider.config?.endpoint)
      setAzureAPIVersion(provider.config?.apiVersion || "2024-02-01")
    }
    if (provider.type === "openai-compatible") {
      setUserModels((provider.config?.models as Array<string>) || [])
      setApiFormat(
        normalizeOpenAICompatibleAPIFormat(provider.config?.apiFormat),
      )
    }
  }, [provider])

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const resp = await getProviderModels(provider.id)
      setModels(resp?.models ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const handleOpen = () => {
    setOpen(true)
    if (models === null && !loading) load()
    if (provider.type === "azure-openai") {
      setAzureMappings(
        normalizeAzureDeploymentMappings(provider.config?.deployments),
      )
    }
    if (provider.type === "openai-compatible") {
      setUserModels((provider.config?.models as Array<string>) || [])
      setApiFormat(
        normalizeOpenAICompatibleAPIFormat(provider.config?.apiFormat),
      )
    }
  }

  const handleToggle = async (model: Model) => {
    const newEnabled = !model.enabled
    setModels((prev) =>
      prev ?
        prev.map((m) => (m.id === model.id ? { ...m, enabled: newEnabled } : m))
      : prev,
    )
    try {
      await toggleProviderModel(provider.id, model.id, newEnabled)
      onModelsChanged?.()
    } catch {
      setModels((prev) =>
        prev ?
          prev.map((m) =>
            m.id === model.id ? { ...m, enabled: model.enabled } : m,
          )
        : prev,
      )
    }
  }

  const handleAddDeployment = async () => {
    if (!newDeployment.trim() || provider.type !== "azure-openai") return
    const deploymentName = newDeployment.trim()
    if (
      azureMappings.some((mapping) => mapping.deployment === deploymentName)
    ) {
      setError("Deployment already exists")
      return
    }
    setConfigLoading(true)
    setError(null)
    try {
      const nextMappings = [
        ...azureMappings,
        { model: deploymentName, deployment: deploymentName },
      ]
      const result = await updateProviderConfig(provider.id, {
        endpoint: azureEndpoint,
        apiVersion: azureAPIVersion,
        deployments: nextMappings,
      })
      setAzureMappings(
        normalizeAzureDeploymentMappings(
          result.config?.deployments ?? nextMappings,
        ),
      )
      setAzureEndpoint(result.config?.endpoint ?? azureEndpoint)
      setAzureAPIVersion(result.config?.apiVersion ?? azureAPIVersion)
      setNewDeployment("")
      onModelsChanged?.()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleRemoveDeployment = async (deploymentName: string) => {
    if (provider.type !== "azure-openai") return
    setConfigLoading(true)
    setError(null)
    try {
      const nextMappings = azureMappings.filter(
        (mapping) =>
          mapping.deployment !== deploymentName
          && mapping.model !== deploymentName,
      )
      const result = await updateProviderConfig(provider.id, {
        endpoint: azureEndpoint,
        apiVersion: azureAPIVersion,
        deployments: nextMappings,
      })
      setAzureMappings(
        normalizeAzureDeploymentMappings(
          result.config?.deployments ?? nextMappings,
        ),
      )
      setAzureEndpoint(result.config?.endpoint ?? azureEndpoint)
      setAzureAPIVersion(result.config?.apiVersion ?? azureAPIVersion)
      onModelsChanged?.()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleAddUserModel = async () => {
    if (!newModel.trim() || provider.type !== "openai-compatible") return
    const ids = newModel
      .trim()
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
    if (ids.length === 0) return
    const alreadyExists = ids.some((id) => userModels.includes(id))
    if (alreadyExists) {
      setError("One or more models already in list")
      return
    }
    setConfigLoading(true)
    setError(null)
    try {
      const updated = [...userModels, ...ids]
      await updateProviderConfig(provider.id, { models: updated })
      setUserModels(updated)
      setNewModel("")
      onModelsChanged?.()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleRemoveUserModel = async (modelID: string) => {
    if (provider.type !== "openai-compatible") return
    setConfigLoading(true)
    setError(null)
    try {
      const updated = userModels.filter((m) => m !== modelID)
      await updateProviderConfig(provider.id, { models: updated })
      setUserModels(updated)
      onModelsChanged?.()
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleUpdateAPIFormat = async (
    nextFormat: OpenAICompatibleAPIFormat,
  ) => {
    if (provider.type !== "openai-compatible" || nextFormat === apiFormat) {
      return
    }
    const previousFormat = apiFormat
    setConfigLoading(true)
    setError(null)
    setApiFormat(nextFormat)
    try {
      await updateProviderConfig(provider.id, { apiFormat: nextFormat })
      onModelsChanged?.()
      await load()
    } catch (e) {
      setApiFormat(previousFormat)
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleSelectAll = async () => {
    if (!models) return
    const toToggle = models.filter((m) => !m.enabled)
    setModels((prev) =>
      prev ? prev.map((m) => ({ ...m, enabled: true })) : prev,
    )
    try {
      await Promise.all(
        toToggle.map((m) => toggleProviderModel(provider.id, m.id, true)),
      )
      onModelsChanged?.()
    } catch {
      await load()
    }
  }

  const handleDeselectAll = async () => {
    if (!models) return
    const toToggle = models.filter((m) => m.enabled)
    setModels((prev) =>
      prev ? prev.map((m) => ({ ...m, enabled: false })) : prev,
    )
    try {
      await Promise.all(
        toToggle.map((m) => toggleProviderModel(provider.id, m.id, false)),
      )
      onModelsChanged?.()
    } catch {
      await load()
    }
  }

  const filtered =
    models ?
      models.filter((m) => m.id.toLowerCase().includes(search.toLowerCase()))
    : []
  const enabledCount = models ? models.filter((m) => m.enabled).length : null

  return (
    <>
      <button className="btn btn-ghost btn-sm" onClick={handleOpen}>
        Models
        {provider.totalModelCount !== undefined
          && provider.totalModelCount > 0 && (
            <span
              style={{
                color: "var(--color-blue)",
                fontSize: 11,
                fontWeight: 600,
                marginLeft: 4,
              }}
            >
              {provider.enabledModelCount}/{provider.totalModelCount}
            </span>
          )}
      </button>

      {open && (
        <div
          className="dialog-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div className="dialog-box">
            <div className="dialog-header">
              <div>
                <div
                  style={{
                    fontFamily: "var(--font-display)",
                    fontWeight: 600,
                    fontSize: 15,
                    color: "var(--color-text)",
                  }}
                >
                  {provider.name} — Models
                </div>
                {enabledCount !== null && models && (
                  <div
                    style={{
                      fontSize: 12,
                      color: "var(--color-text-secondary)",
                      marginTop: 2,
                    }}
                  >
                    <span style={{ color: "var(--color-green)" }}>
                      {enabledCount}
                    </span>{" "}
                    of {models.length} enabled
                  </div>
                )}
              </div>
              <div style={{ display: "flex", gap: 8 }}>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={async () => {
                    setLoading(true)
                    setError(null)
                    try {
                      const resp = await refreshProviderModels(provider.id)
                      setModels(resp?.models ?? [])
                      onModelsChanged?.()
                    } catch (e) {
                      setError(e instanceof Error ? e.message : String(e))
                    } finally {
                      setLoading(false)
                    }
                  }}
                  disabled={loading}
                >
                  Refresh
                </button>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => setOpen(false)}
                >
                  Done
                </button>
              </div>
            </div>
            <div className="dialog-body">
              {loading && (
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    padding: "32px 0",
                    justifyContent: "center",
                    color: "var(--color-text-secondary)",
                    fontSize: 14,
                  }}
                >
                  <Spin /> Loading models…
                </div>
              )}
              {error && (
                <div
                  style={{
                    color: "var(--color-red)",
                    fontSize: 13,
                    padding: "12px 0",
                  }}
                >
                  {error}
                </div>
              )}
              {models && !loading && (
                <div
                  style={{ display: "flex", flexDirection: "column", gap: 14 }}
                >
                  {provider.type === "azure-openai" && (
                    <div
                      style={{
                        padding: "14px 16px",
                        background: "rgba(10,132,255,0.06)",
                        border: "1px solid rgba(10,132,255,0.18)",
                        borderRadius: "var(--radius-lg)",
                      }}
                    >
                      <div
                        style={{
                          fontSize: 13,
                          fontWeight: 600,
                          color: "var(--color-blue)",
                          marginBottom: 12,
                        }}
                      >
                        Deployment Management
                      </div>
                      <div
                        style={{
                          display: "flex",
                          gap: 8,
                          alignItems: "center",
                          marginBottom: 8,
                        }}
                      >
                        <input
                          className="sys-input"
                          placeholder="Add deployment name..."
                          value={newDeployment}
                          onChange={(e) => setNewDeployment(e.target.value)}
                          disabled={configLoading}
                          style={{ flex: 1, fontSize: 13 }}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") {
                              e.preventDefault()
                              handleAddDeployment()
                            }
                          }}
                        />
                        <button
                          className="btn btn-primary btn-sm"
                          onClick={handleAddDeployment}
                          disabled={configLoading || !newDeployment.trim()}
                          style={{ minWidth: 32, padding: "6px 8px" }}
                        >
                          {configLoading ?
                            <Spin size={12} />
                          : "+"}
                        </button>
                      </div>
                      <div
                        style={{
                          fontSize: 11,
                          color: "var(--color-text-tertiary)",
                        }}
                      >
                        Enter deployment names from your Azure OpenAI resource.
                        Each deployment becomes a model.
                      </div>
                    </div>
                  )}

                  {/* OpenAI-compatible user-defined model management */}
                  {provider.type === "openai-compatible" && (
                    <div
                      style={{
                        padding: "14px 16px",
                        background: "rgba(16,185,129,0.06)",
                        border: "1px solid rgba(16,185,129,0.18)",
                        borderRadius: "var(--radius-lg)",
                      }}
                    >
                      <div
                        style={{
                          fontSize: 13,
                          fontWeight: 600,
                          color: "#10b981",
                          marginBottom: 12,
                        }}
                      >
                        Model IDs
                      </div>
                      <div
                        style={{
                          display: "flex",
                          flexDirection: "column",
                          gap: 8,
                          marginBottom: 12,
                        }}
                      >
                        <div
                          style={{
                            fontSize: 12,
                            fontWeight: 600,
                            color: "#10b981",
                          }}
                        >
                          Upstream API
                        </div>
                        <OpenAICompatibleAPIFormatControl
                          value={apiFormat}
                          onChange={handleUpdateAPIFormat}
                          disabled={configLoading}
                        />
                        <div
                          style={{
                            fontSize: 11,
                            color: "var(--color-text-tertiary)",
                          }}
                        >
                          Auto uses <code>/v1/responses</code> for official
                          OpenAI when requests arrive via{" "}
                          <code>/v1/messages</code> or{" "}
                          <code>/v1/responses</code>, and{" "}
                          <code>/v1/chat/completions</code> otherwise.
                        </div>
                      </div>
                      <div
                        style={{
                          display: "flex",
                          gap: 8,
                          alignItems: "center",
                          marginBottom: 8,
                        }}
                      >
                        <input
                          className="sys-input"
                          placeholder="Add model ID (e.g. llama3, mistral)..."
                          value={newModel}
                          onChange={(e) => setNewModel(e.target.value)}
                          disabled={configLoading}
                          style={{ flex: 1, fontSize: 13 }}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") {
                              e.preventDefault()
                              handleAddUserModel()
                            }
                          }}
                        />
                        <button
                          className="btn btn-primary btn-sm"
                          onClick={handleAddUserModel}
                          disabled={configLoading || !newModel.trim()}
                          style={{ minWidth: 32, padding: "6px 8px" }}
                        >
                          {configLoading ?
                            <Spin size={12} />
                          : "+"}
                        </button>
                      </div>
                      <div
                        style={{
                          fontSize: 11,
                          color: "var(--color-text-tertiary)",
                        }}
                      >
                        Enter model IDs to use from this endpoint. These are
                        merged with any models returned by <code>/models</code>.
                      </div>
                    </div>
                  )}

                  <div
                    style={{ display: "flex", gap: 8, alignItems: "center" }}
                  >
                    <input
                      className="sys-input"
                      placeholder="Filter models…"
                      value={search}
                      onChange={(e) => setSearch(e.target.value)}
                      style={{ flex: 1 }}
                    />
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={handleSelectAll}
                    >
                      All On
                    </button>
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={handleDeselectAll}
                    >
                      All Off
                    </button>
                  </div>
                  <div
                    style={{
                      borderRadius: "var(--radius-lg)",
                      overflow: "hidden",
                      border: "1px solid var(--color-separator)",
                    }}
                  >
                    {filtered.map((m, i) => (
                      <div
                        key={m.id}
                        style={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          padding: "10px 14px",
                          borderBottom:
                            i < filtered.length - 1 ?
                              "1px solid var(--color-separator)"
                            : "none",
                          background:
                            m.enabled ? "rgba(48,209,88,0.04)" : "transparent",
                          transition: "background 0.12s",
                        }}
                      >
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 12,
                            flex: 1,
                          }}
                        >
                          <div>
                            <div
                              style={{
                                fontFamily: "var(--font-mono)",
                                fontSize: 13,
                                color:
                                  m.enabled ? "var(--color-text)" : (
                                    "var(--color-text-secondary)"
                                  ),
                              }}
                            >
                              {m.id}
                            </div>
                            {m.name && m.name !== m.id && (
                              <div
                                style={{
                                  fontSize: 11,
                                  color: "var(--color-text-tertiary)",
                                  marginTop: 1,
                                }}
                              >
                                {m.name}
                              </div>
                            )}
                          </div>
                        </div>
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 8,
                          }}
                        >
                          {provider.type === "azure-openai"
                            && deployments.has(m.id) && (
                              <button
                                className="btn btn-ghost btn-sm"
                                onClick={() => handleRemoveDeployment(m.id)}
                                disabled={configLoading}
                                style={{
                                  minWidth: 32,
                                  padding: "4px 6px",
                                  color: "var(--color-red)",
                                }}
                                title={`Remove deployment ${m.id}`}
                              >
                                {configLoading ?
                                  <Spin size={10} />
                                : "−"}
                              </button>
                            )}
                          {/* Remove model button for openai-compatible */}
                          {provider.type === "openai-compatible"
                            && userModels.includes(m.id) && (
                              <button
                                className="btn btn-ghost btn-sm"
                                onClick={() => handleRemoveUserModel(m.id)}
                                disabled={configLoading}
                                style={{
                                  minWidth: 32,
                                  padding: "4px 6px",
                                  color: "var(--color-red)",
                                }}
                                title={`Remove model ${m.id}`}
                              >
                                {configLoading ?
                                  <Spin size={10} />
                                : "−"}
                              </button>
                            )}
                          <div
                            className={`toggle-track ${m.enabled ? "on" : ""}`}
                            onClick={() => handleToggle(m)}
                          >
                            <div className="toggle-thumb" />
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  )
}

function _UsageMenuItem({ provider }: { provider: Provider }) {
  const [data, setData] = useState<UsageData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [open, setOpen] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      setData(await getProviderUsage(provider.id))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const handleOpen = () => {
    setOpen(true)
    if (data === null && !loading) load()
  }

  const getBarColor = (pct: number) =>
    pct > 90 ? "var(--color-red)"
    : pct > 75 ? "var(--color-orange)"
    : "var(--color-green)"

  return (
    <>
      <button className="btn btn-ghost btn-sm" onClick={handleOpen}>
        Usage
      </button>

      {open && (
        <div
          className="dialog-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div className="dialog-box">
            <div className="dialog-header">
              <div
                style={{
                  fontFamily: "var(--font-display)",
                  fontWeight: 600,
                  fontSize: 15,
                  color: "var(--color-text)",
                }}
              >
                {provider.name} — Usage
              </div>
              <div style={{ display: "flex", gap: 8 }}>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={load}
                  disabled={loading}
                >
                  Refresh
                </button>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => setOpen(false)}
                >
                  Done
                </button>
              </div>
            </div>
            <div className="dialog-body">
              {loading && (
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    padding: "32px 0",
                    justifyContent: "center",
                    color: "var(--color-text-secondary)",
                    fontSize: 14,
                  }}
                >
                  <Spin /> Fetching usage data…
                </div>
              )}
              {error && (
                <div
                  style={{
                    color: "var(--color-red)",
                    fontSize: 13,
                    padding: "12px 0",
                  }}
                >
                  {error}
                </div>
              )}
              {data && !loading && (
                <div
                  style={{ display: "flex", flexDirection: "column", gap: 20 }}
                >
                  {(
                    data.quota_snapshots
                    && Object.keys(data.quota_snapshots).length > 0
                  ) ?
                    <div
                      style={{
                        display: "flex",
                        flexDirection: "column",
                        gap: 18,
                      }}
                    >
                      {Object.entries(data.quota_snapshots).map(
                        ([key, value]) => {
                          const { entitlement, percent_remaining, unlimited } =
                            value
                          const remaining =
                            value.quota_remaining ?? value.remaining
                          const pctUsed =
                            unlimited ? 0 : 100 - percent_remaining
                          const used =
                            unlimited ? "N/A" : (
                              (entitlement - remaining).toLocaleString()
                            )
                          const barColor =
                            unlimited ? "var(--color-blue)" : (
                              getBarColor(pctUsed)
                            )
                          return (
                            <div key={key}>
                              <div
                                style={{
                                  display: "flex",
                                  justifyContent: "space-between",
                                  marginBottom: 8,
                                  alignItems: "baseline",
                                }}
                              >
                                <span
                                  style={{
                                    fontSize: 13,
                                    fontWeight: 500,
                                    textTransform: "capitalize",
                                    color: "var(--color-text)",
                                  }}
                                >
                                  {key.replaceAll("_", " ")}
                                </span>
                                {unlimited ?
                                  <span
                                    style={{
                                      fontSize: 11,
                                      padding: "2px 8px",
                                      background: "var(--color-blue-fill)",
                                      borderRadius: "var(--radius-pill)",
                                      color: "var(--color-blue)",
                                      fontWeight: 500,
                                    }}
                                  >
                                    Unlimited
                                  </span>
                                : <span
                                    style={{
                                      fontSize: 12,
                                      fontFamily: "var(--font-mono)",
                                      color:
                                        pctUsed > 75 ? barColor : (
                                          "var(--color-text-secondary)"
                                        ),
                                    }}
                                  >
                                    {pctUsed.toFixed(1)}% used
                                  </span>
                                }
                              </div>
                              <div className="progress-track">
                                <div
                                  className="progress-bar"
                                  style={{
                                    width: `${unlimited ? 100 : pctUsed}%`,
                                    background: barColor,
                                  }}
                                />
                              </div>
                              <div
                                style={{
                                  display: "flex",
                                  justifyContent: "space-between",
                                  marginTop: 5,
                                  fontSize: 12,
                                  color: "var(--color-text-secondary)",
                                  fontFamily: "var(--font-mono)",
                                }}
                              >
                                <span>
                                  {used} /{" "}
                                  {unlimited ?
                                    "∞"
                                  : entitlement.toLocaleString()}
                                </span>
                                <span>
                                  {unlimited ? "∞" : remaining.toLocaleString()}{" "}
                                  remaining
                                </span>
                              </div>
                            </div>
                          )
                        },
                      )}
                    </div>
                  : <div>
                      <div
                        style={{
                          fontSize: 12,
                          color: "var(--color-text-secondary)",
                          marginBottom: 10,
                        }}
                      >
                        Raw Data
                      </div>
                      <pre
                        style={{
                          fontSize: 12,
                          fontFamily: "var(--font-mono)",
                          color: "var(--color-text-secondary)",
                          background: "rgba(255,255,255,0.04)",
                          border: "1px solid var(--color-separator)",
                          borderRadius: "var(--radius-md)",
                          padding: 14,
                          overflow: "auto",
                          maxHeight: 280,
                        }}
                      >
                        {JSON.stringify(data, null, 2)}
                      </pre>
                    </div>
                  }
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  )
}

// ─── Provider Card ────────────────────────────────────────────────────────────

function ProviderCard({
  provider,
  isFlowRunning,
  isActivating,
  onActivate,
  onDeactivate,
  onDelete,
  onAuthSubmit,
  onModelsChanged,
  onRename,
  priorityIndex,
  multiProvider,
}: {
  provider: Provider
  isFlowRunning: boolean
  isActivating: boolean
  onActivate: (id: string) => void
  onDeactivate: (id: string) => void
  onDelete: (id: string) => void
  onAuthSubmit: (id: string, body: Record<string, string>) => Promise<void>
  onModelsChanged: () => void
  onRename: (id: string, fields: { name?: string; subtitle?: string }) => void
  priorityIndex: number
  multiProvider: boolean
}) {
  const confirm = useConfirm()
  const [showAuthForm, setShowAuthForm] = useState(false)
  const [editingName, setEditingName] = useState(false)
  const [nameInput, setNameInput] = useState(provider.name)
  const nameInputRef = useRef<HTMLInputElement>(null)
  const [editingSubtitle, setEditingSubtitle] = useState(false)
  const [subtitleInput, setSubtitleInput] = useState(provider.subtitle ?? "")
  const subtitleInputRef = useRef<HTMLInputElement>(null)
  const accent = PROVIDER_ACCENT[provider.type] ?? "#0a84ff"

  // Keep local input state in sync when the parent provider prop changes
  // (e.g. after optimistic update or a load() round-trip)
  useEffect(() => {
    if (!editingName) setNameInput(provider.name)
  }, [provider.name, editingName])

  useEffect(() => {
    if (!editingSubtitle) setSubtitleInput(provider.subtitle ?? "")
  }, [provider.subtitle, editingSubtitle])

  const handleAuthSubmit = async (body: Record<string, string>) => {
    await onAuthSubmit(provider.id, body)
    setShowAuthForm(false)
  }

  const startEditName = () => {
    setNameInput(provider.name)
    setEditingName(true)
    setTimeout(() => nameInputRef.current?.select(), 0)
  }

  const commitName = () => {
    const trimmed = nameInput.trim()
    if (trimmed && trimmed !== provider.name) {
      onRename(provider.id, { name: trimmed })
    }
    setEditingName(false)
  }

  const cancelEditName = () => {
    setEditingName(false)
    setNameInput(provider.name)
  }

  const startEditSubtitle = () => {
    setSubtitleInput(provider.subtitle ?? "")
    setEditingSubtitle(true)
    setTimeout(() => subtitleInputRef.current?.select(), 0)
  }

  const commitSubtitle = () => {
    const trimmed = subtitleInput.trim()
    const current = provider.subtitle ?? ""
    if (trimmed !== current) {
      onRename(provider.id, { subtitle: trimmed })
    }
    setEditingSubtitle(false)
  }

  const cancelEditSubtitle = () => {
    setEditingSubtitle(false)
    setSubtitleInput(provider.subtitle ?? "")
  }

  return (
    <div
      style={{
        background: "var(--color-bg-elevated)",
        borderRadius: "var(--radius-lg)",
        border: "1px solid",
        borderColor:
          provider.isActive ? "rgba(48,209,88,0.3)" : "var(--color-separator)",
        boxShadow:
          provider.isActive ?
            "var(--shadow-card), 0 0 0 1px rgba(48,209,88,0.15)"
          : "var(--shadow-card)",
        overflow: "hidden",
        transition: "all 0.2s var(--ease)",
      }}
    >
      {/* Colored left bar for active */}
      {provider.isActive && (
        <div
          style={{
            height: 3,
            background: `linear-gradient(to right, ${accent}, var(--color-green))`,
          }}
        />
      )}

      <div style={{ padding: "16px 18px" }}>
        {/* Header */}
        <div
          style={{
            display: "flex",
            alignItems: "flex-start",
            justifyContent: "space-between",
            marginBottom: 14,
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            {/* Icon badge */}
            <div
              style={{
                width: 42,
                height: 42,
                borderRadius: "var(--radius-md)",
                background: `${accent}18`,
                border: `1px solid ${accent}28`,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                color: accent,
                flexShrink: 0,
              }}
            >
              {PROVIDER_ICONS[provider.type] ?? (
                <span style={{ fontSize: 20 }}>◌</span>
              )}
            </div>
            <div>
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                }}
              >
                {editingName ?
                  <input
                    ref={nameInputRef}
                    value={nameInput}
                    onChange={(e) => setNameInput(e.target.value)}
                    onBlur={commitName}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        commitName()
                      } else if (e.key === "Escape") {
                        cancelEditName()
                      }
                    }}
                    style={{
                      fontFamily: "var(--font-display)",
                      fontWeight: 600,
                      fontSize: 15,
                      color: "var(--color-text)",
                      lineHeight: 1.2,
                      letterSpacing: "-0.01em",
                      background: "var(--color-bg-input, var(--color-bg))",
                      border: "1px solid var(--color-blue)",
                      borderRadius: 4,
                      padding: "1px 6px",
                      outline: "none",
                      width: 200,
                    }}
                  />
                : <span
                    style={{
                      fontFamily: "var(--font-display)",
                      fontWeight: 600,
                      fontSize: 15,
                      color: "var(--color-text)",
                      lineHeight: 1.2,
                      letterSpacing: "-0.01em",
                    }}
                  >
                    {provider.name}
                  </span>
                }
                {!editingName && (
                  <button
                    title="Rename provider"
                    onClick={startEditName}
                    style={{
                      background: "none",
                      border: "none",
                      padding: "2px 4px",
                      cursor: "pointer",
                      color: "var(--color-text-tertiary)",
                      display: "flex",
                      alignItems: "center",
                      borderRadius: 4,
                      lineHeight: 1,
                    }}
                  >
                    <svg
                      width="12"
                      height="12"
                      viewBox="0 0 16 16"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.8"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    >
                      <path d="M11.013 2.513a1.75 1.75 0 0 1 2.475 2.474L5.81 12.664l-3.257.744.744-3.257 7.716-7.638Z" />
                    </svg>
                  </button>
                )}
              </div>
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 4,
                  marginTop: 2,
                }}
              >
                {editingSubtitle ?
                  <input
                    ref={subtitleInputRef}
                    value={subtitleInput}
                    onChange={(e) => setSubtitleInput(e.target.value)}
                    onBlur={commitSubtitle}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        commitSubtitle()
                      } else if (e.key === "Escape") {
                        cancelEditSubtitle()
                      }
                    }}
                    placeholder={provider.id}
                    style={{
                      fontSize: 11,
                      color: "var(--color-text-tertiary)",
                      fontFamily: "var(--font-mono)",
                      background: "var(--color-bg-input, var(--color-bg))",
                      border: "1px solid var(--color-blue)",
                      borderRadius: 4,
                      padding: "1px 6px",
                      outline: "none",
                      width: 220,
                    }}
                  />
                : <span
                    style={{
                      fontSize: 11,
                      color: "var(--color-text-tertiary)",
                      fontFamily: "var(--font-mono)",
                    }}
                  >
                    {provider.subtitle || provider.id}
                  </span>
                }
                {!editingSubtitle && (
                  <button
                    title="Edit subtitle"
                    onClick={startEditSubtitle}
                    style={{
                      background: "none",
                      border: "none",
                      padding: "1px 3px",
                      cursor: "pointer",
                      color: "var(--color-text-tertiary)",
                      display: "flex",
                      alignItems: "center",
                      borderRadius: 4,
                      lineHeight: 1,
                      opacity: 0.6,
                    }}
                  >
                    <svg
                      width="10"
                      height="10"
                      viewBox="0 0 16 16"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.8"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    >
                      <path d="M11.013 2.513a1.75 1.75 0 0 1 2.475 2.474L5.81 12.664l-3.257.744.744-3.257 7.716-7.638Z" />
                    </svg>
                  </button>
                )}
              </div>
            </div>
          </div>

          {/* Right badges */}
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            {provider.isActive && multiProvider && priorityIndex >= 0 && (
              <div
                style={{
                  fontSize: 11,
                  fontWeight: 600,
                  padding: "3px 10px",
                  background: "var(--color-blue-fill)",
                  color: "var(--color-blue)",
                  borderRadius: "var(--radius-pill)",
                }}
              >
                #{priorityIndex + 1}
              </div>
            )}
            {provider.isActive ?
              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <div className="status-dot status-dot-active" />
                <span
                  style={{
                    fontSize: 12,
                    fontWeight: 500,
                    color: "var(--color-green)",
                  }}
                >
                  Active
                </span>
              </div>
            : <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <div className="status-dot status-dot-inactive" />
                <span
                  style={{ fontSize: 12, color: "var(--color-text-tertiary)" }}
                >
                  {provider.authStatus === "authenticated" ?
                    "Ready"
                  : "Not authorized"}
                </span>
              </div>
            }
          </div>
        </div>

        {/* Instance info */}
        <div
          style={{
            display: "flex",
            gap: 16,
            marginBottom: 12,
            fontSize: 11,
            color: "var(--color-text-tertiary)",
            fontFamily: "var(--font-mono)",
          }}
        >
          <span>
            <span style={{ color: "var(--color-text-secondary)" }}>
              instance:{" "}
            </span>
            {provider.id}
          </span>
          <span>
            <span style={{ color: "var(--color-text-secondary)" }}>
              prefix:{" "}
            </span>
            {provider.subtitle || provider.id}
          </span>
        </div>

        {/* Model progress */}
        {provider.authStatus === "authenticated"
          && provider.totalModelCount !== undefined
          && provider.totalModelCount > 0 && (
            <div style={{ marginBottom: 14 }}>
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  marginBottom: 5,
                  fontSize: 12,
                  color: "var(--color-text-secondary)",
                }}
              >
                <span>Models enabled</span>
                <span
                  style={{ fontFamily: "var(--font-mono)", fontWeight: 500 }}
                >
                  <span style={{ color: "var(--color-green)" }}>
                    {provider.enabledModelCount}
                  </span>
                  <span style={{ color: "var(--color-text-tertiary)" }}>
                    {" "}
                    / {provider.totalModelCount}
                  </span>
                </span>
              </div>
              <div className="progress-track">
                <div
                  className="progress-bar"
                  style={{
                    width: `${((provider.enabledModelCount ?? 0) / provider.totalModelCount) * 100}%`,
                    background: "var(--color-green)",
                  }}
                />
              </div>
            </div>
          )}

        {/* Actions */}
        <div
          style={{
            display: "flex",
            gap: 8,
            alignItems: "center",
          }}
        >
          {provider.isActive ?
            <button
              className="btn btn-ghost btn-sm"
              disabled={isFlowRunning || isActivating}
              onClick={() => onDeactivate(provider.id)}
            >
              {isActivating ?
                <>
                  <Spinner className="h-3 w-3" /> Working…
                </>
              : "Deactivate"}
            </button>
          : <button
              className="btn btn-green btn-sm"
              disabled={
                isFlowRunning
                || isActivating
                || provider.authStatus !== "authenticated"
              }
              onClick={() => onActivate(provider.id)}
            >
              {isActivating ?
                <>
                  <Spinner className="h-3 w-3" /> Working…
                </>
              : "Activate"}
            </button>
          }

          {/* Inline action buttons */}
          <button
            className="btn btn-ghost btn-sm"
            disabled={isFlowRunning}
            onClick={() => setShowAuthForm((v) => !v)}
          >
            {showAuthForm ? "Close" : "Authorize"}
          </button>
          <ModelsMenuItem
            provider={provider}
            onModelsChanged={onModelsChanged}
          />
          <UsageDialog provider={provider} />

          <div style={{ flex: 1 }} />
          <button
            className="btn btn-danger btn-sm"
            disabled={isFlowRunning}
            onClick={async () => {
              const ok = await confirm({
                title: "Delete provider",
                message: `Are you sure you want to delete "${provider.name}"? This cannot be undone.`,
                danger: true,
                confirmLabel: "Delete",
              })
              if (ok) onDelete(provider.id)
            }}
          >
            Delete
          </button>
        </div>
      </div>

      {/* Auth form inlined */}
      {showAuthForm && (
        <div
          style={{
            borderTop: "1px solid var(--color-separator)",
            padding: "0 18px 18px",
          }}
        >
          {provider.type === "alibaba" && (
            <AlibabaAuthForm
              onSubmit={handleAuthSubmit}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
          {provider.type === "alibaba-modelscope" && (
            <ModelScopeAuthForm
              onSubmit={handleAuthSubmit}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
          {provider.type === "github-copilot" && (
            <CopilotAuthForm
              onSubmit={handleAuthSubmit}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
          {provider.type === "codex" && (
            <CodexAuthForm
              onSubmit={handleAuthSubmit}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
          {provider.type === "azure-openai" && (
            <AzureOpenAIAuthForm
              onSubmit={handleAuthSubmit}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
          {provider.type === "antigravity" && (
            <AntigravityAuthForm
              provider={provider}
              onSuccess={() => {
                setShowAuthForm(false)
                onModelsChanged?.()
              }}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
        </div>
      )}
    </div>
  )
}

// ─── Add Provider Flow (full-page inline wizard) ─────────────────────────────

type AddFlowStep = "select" | "configure" | "authenticating"

function AddProviderFlow({
  onDone,
  onCancel,
  showToast,
}: {
  onDone: () => void
  onCancel: () => void
  showToast: (msg: string, type?: "success" | "error") => void
}) {
  const [step, setStep] = useState<AddFlowStep>("select")
  const [selectedType, setSelectedType] = useState<string | null>(null)
  const [authFlow, setAuthFlow] = useState<AuthFlow | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  // Track whether the component is still mounted so in-flight async callbacks
  // don't call state setters or onDone after unmount.
  const mountedRef = useRef(true)

  const stopPoll = useCallback(() => {
    if (pollTimer.current) {
      clearInterval(pollTimer.current)
      pollTimer.current = null
    }
  }, [])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      stopPoll()
    }
  }, [stopPoll])

  const startPoll = useCallback(() => {
    stopPoll()
    pollTimer.current = setInterval(async () => {
      try {
        const af = await getAuthStatus()
        // Guard against state updates on unmounted component.
        if (!mountedRef.current) return
        setAuthFlow(af)
        if (af?.status === "complete") {
          stopPoll()
          showToast("Provider added successfully!")
          onDone()
        } else if (af?.status === "error") {
          stopPoll()
          showToast(
            "Authentication failed: " + (af.error ?? "unknown"),
            "error",
          )
          setStep("configure")
          setAuthFlow(null)
        }
      } catch {
        /* ignore transient poll errors */
      }
    }, 2000)
  }, [onDone, showToast, stopPoll])

  const handleAuthSubmit = async (body: Record<string, string>) => {
    if (!selectedType) return
    setSubmitting(true)
    try {
      const result = await authAndCreateProvider(selectedType, body)
      if (result.requiresAuth) {
        // Use the pending_id returned by the server rather than constructing it
        // client-side, so any future backend changes don't silently break this.
        setAuthFlow({
          providerId: result.pending_id ?? selectedType + "-pending",
          status: "awaiting_user",
          userCode: result.user_code,
          instructionURL: result.verification_uri,
        })
        setStep("authenticating")
        startPoll()
      } else if (result.success) {
        showToast(
          `Provider "${result.provider?.name ?? selectedType}" added successfully!`,
        )
        onDone()
      } else {
        // Backend returned 200 with success:false for a non-OAuth path — surface the error.
        showToast(
          result.message
            ?? "Authentication failed. Please check your credentials.",
          "error",
        )
      }
    } catch (e) {
      showToast(
        "Failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setSubmitting(false)
    }
  }

  const handleCancelAuth = async () => {
    stopPoll()
    setAuthFlow(null)
    try {
      await cancelAuth()
    } catch {
      /* ignore */
    }
    setStep("configure")
  }

  // ── Step 1: Select type ──────────────────────────────────────────────────────
  if (step === "select") {
    return (
      <div>
        {/* Header */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            marginBottom: 24,
          }}
        >
          <button
            className="btn btn-ghost btn-sm"
            onClick={onCancel}
            style={{ padding: "4px 10px" }}
          >
            ← Back
          </button>
          <h2
            style={{
              fontFamily: "var(--font-display)",
              fontWeight: 600,
              fontSize: 18,
              margin: 0,
              color: "var(--color-text)",
              letterSpacing: "-0.02em",
            }}
          >
            Add Provider
          </h2>
        </div>
        <p
          style={{
            fontSize: 13,
            color: "var(--color-text-secondary)",
            marginBottom: 20,
            marginTop: 0,
          }}
        >
          Choose the provider type you want to add. You can add multiple
          accounts of the same type.
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
          {PROVIDER_TYPES.map((pt) => {
            const accent = PROVIDER_ACCENT[pt.id] ?? "#0a84ff"
            return (
              <button
                key={pt.id}
                onClick={() => {
                  setSelectedType(pt.id)
                  setStep("configure")
                }}
                className="provider-type-btn"
                style={{ "--provider-accent": accent } as React.CSSProperties}
              >
                <div
                  style={{
                    width: 38,
                    height: 38,
                    borderRadius: "var(--radius-sm)",
                    background: `${accent}18`,
                    border: `1px solid ${accent}28`,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    color: accent,
                    flexShrink: 0,
                  }}
                >
                  {PROVIDER_ICONS[pt.id] ?? (
                    <span style={{ fontSize: 18 }}>◌</span>
                  )}
                </div>
                <div>
                  <div
                    style={{
                      fontWeight: 600,
                      fontSize: 14,
                      letterSpacing: "-0.01em",
                    }}
                  >
                    {pt.name}
                  </div>
                  <div
                    style={{
                      fontSize: 12,
                      color: "var(--color-text-secondary)",
                      marginTop: 2,
                    }}
                  >
                    {pt.desc}
                  </div>
                </div>
                <span
                  style={{
                    marginLeft: "auto",
                    color: "var(--color-text-tertiary)",
                    fontSize: 16,
                  }}
                >
                  ›
                </span>
              </button>
            )
          })}
        </div>
      </div>
    )
  }

  // ── Step 2: Configure / authenticate ─────────────────────────────────────────
  if (step === "configure" && selectedType) {
    const typeName =
      PROVIDER_TYPES.find((pt) => pt.id === selectedType)?.name ?? selectedType
    const accent = PROVIDER_ACCENT[selectedType] ?? "#0a84ff"

    const authFormProps = {
      onSubmit: handleAuthSubmit,
      onCancel: () => setStep("select"),
      submitting,
    }

    return (
      <div>
        {/* Header */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            marginBottom: 24,
          }}
        >
          <button
            className="btn btn-ghost btn-sm"
            onClick={() => setStep("select")}
            style={{ padding: "4px 10px" }}
          >
            ← Back
          </button>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div
              style={{
                width: 38,
                height: 38,
                borderRadius: "var(--radius-sm)",
                background: `${accent}18`,
                border: `1px solid ${accent}28`,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                color: accent,
                flexShrink: 0,
              }}
            >
              {PROVIDER_ICONS[selectedType] ?? (
                <span style={{ fontSize: 18 }}>◌</span>
              )}
            </div>
            <div>
              <h2
                style={{
                  fontFamily: "var(--font-display)",
                  fontWeight: 600,
                  fontSize: 18,
                  margin: 0,
                  color: "var(--color-text)",
                  letterSpacing: "-0.02em",
                }}
              >
                {typeName}
              </h2>
              <p
                style={{
                  fontSize: 12,
                  color: "var(--color-text-secondary)",
                  margin: "2px 0 0",
                  marginTop: 2,
                }}
              >
                {PROVIDER_TYPES.find((pt) => pt.id === selectedType)?.desc
                  ?? ""}
              </p>
            </div>
          </div>
        </div>

        <div
          style={{
            background: "var(--color-bg-elevated)",
            borderRadius: "var(--radius-lg)",
            border: "1px solid var(--color-separator)",
            boxShadow: "var(--shadow-card)",
            padding: 24,
            display: "flex",
            flexDirection: "column",
            gap: 20,
          }}
        >
          {selectedType === "alibaba" && (
            <AddFlowAlibabaForm {...authFormProps} />
          )}
          {selectedType === "alibaba-modelscope" && (
            <AddFlowModelScopeForm {...authFormProps} />
          )}
          {selectedType === "github-copilot" && (
            <AddFlowCopilotForm {...authFormProps} />
          )}
          {selectedType === "codex" && <AddFlowCodexForm {...authFormProps} />}
          {selectedType === "antigravity" && (
            <AddFlowAntigravityForm
              onSuccess={() => {
                setStep("select")
                onDone?.()
              }}
              onCancel={() => setStep("select")}
            />
          )}
          {selectedType === "azure-openai" && (
            <AddFlowAzureForm {...authFormProps} />
          )}
          {selectedType === "google" && (
            <AddFlowGoogleForm {...authFormProps} />
          )}
          {selectedType === "kimi" && <AddFlowKimiForm {...authFormProps} />}
          {selectedType === "openai-compatible" && (
            <AddFlowOpenAICompatibleForm {...authFormProps} />
          )}
        </div>
      </div>
    )
  }

  // ── Step 3: Authenticating (OAuth device flow in progress) ───────────────────
  if (step === "authenticating") {
    return (
      <div>
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            marginBottom: 24,
          }}
        >
          <h2
            style={{
              fontFamily: "var(--font-display)",
              fontWeight: 600,
              fontSize: 18,
              margin: 0,
              color: "var(--color-text)",
              letterSpacing: "-0.02em",
            }}
          >
            Authenticating…
          </h2>
        </div>

        {authFlow && (
          <div
            style={{
              background: "rgba(255,159,10,0.08)",
              border: "1px solid rgba(255,159,10,0.25)",
              borderRadius: "var(--radius-lg)",
              padding: "18px 20px",
              display: "flex",
              flexDirection: "column",
              gap: 14,
            }}
          >
            {authFlow.userCode && (
              <div>
                <div
                  style={{
                    fontSize: 12,
                    color: "var(--color-text-secondary)",
                    marginBottom: 8,
                    fontWeight: 500,
                  }}
                >
                  Enter this code on the authorization page:
                </div>
                <div
                  style={{
                    fontFamily: "var(--font-mono)",
                    fontSize: 28,
                    fontWeight: 700,
                    letterSpacing: "0.15em",
                    color: "var(--color-orange)",
                    background: "rgba(255,159,10,0.12)",
                    border: "1px solid rgba(255,159,10,0.3)",
                    borderRadius: "var(--radius-md)",
                    padding: "10px 16px",
                    display: "inline-block",
                  }}
                >
                  {authFlow.userCode}
                </div>
              </div>
            )}
            {authFlow.instructionURL && (
              <div>
                <div
                  style={{
                    fontSize: 12,
                    color: "var(--color-text-secondary)",
                    marginBottom: 6,
                    fontWeight: 500,
                  }}
                >
                  Open this URL in your browser:
                </div>
                <a
                  href={authFlow.instructionURL}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="btn btn-sm"
                  style={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 6,
                    textDecoration: "none",
                  }}
                >
                  Open Authorization Page ↗
                </a>
              </div>
            )}
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                fontSize: 13,
                color: "var(--color-text-secondary)",
              }}
            >
              <Spin size={13} />
              Waiting for authorization…
            </div>
            <div>
              <button
                className="btn btn-sm btn-ghost"
                onClick={handleCancelAuth}
              >
                Cancel
              </button>
            </div>
          </div>
        )}
      </div>
    )
  }

  return null
}

// ── Forms used only by AddProviderFlow ────────────────────────────────────────

interface AddFlowFormProps {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
  submitting: boolean
}

function AddFlowAlibabaForm({
  onSubmit,
  onCancel,
  submitting,
}: AddFlowFormProps) {
  const [plan, setPlan] = useState("standard")
  const [region, setRegion] = useState("global")
  const [endpoint, setEndpoint] = useState("")
  const [apiKey, setApiKey] = useState("")
  const submit = async () => {
    if (!apiKey.trim()) return
    const body: Record<string, string> = {
      method: "api-key",
      plan,
      region,
      apiKey: apiKey.trim(),
    }
    if (endpoint.trim()) {
      body.endpoint = endpoint.trim()
    }
    await onSubmit(body)
  }
  return (
    <div style={addFlowPanelStyle}>
      <AddFlowHint>
        Only API key authentication is supported for Alibaba DashScope.
      </AddFlowHint>
      <FormRow label="API Mode">
        <AddFlowChoiceGroup
          value={plan}
          onChange={setPlan}
          accent="#ff9f0a"
          options={[
            {
              value: "standard",
              label: "Standard (pay-as-you-go, recommended for qwen3.6-plus)",
            },
            { value: "coding-plan", label: "Coding Plan" },
          ]}
        />
      </FormRow>
      <FormRow label="Region">
        <select
          value={region}
          onChange={(e) => setRegion(e.target.value)}
          style={addFlowControlStyle}
        >
          <option value="global">Global (dashscope-intl.aliyuncs.com)</option>
          <option value="china">China (dashscope.aliyuncs.com)</option>
        </select>
      </FormRow>
      <FormRow label="Base URL (optional)">
        <input
          type="text"
          placeholder={
            plan === "coding-plan" ?
              "https://coding-intl.dashscope.aliyuncs.com/v1"
            : "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
          }
          value={endpoint}
          onChange={(e) => setEndpoint(e.target.value)}
          style={addFlowTextInputStyle}
        />
      </FormRow>
      <FormRow label="DashScope API Key">
        <SecretInput
          placeholder={plan === "coding-plan" ? "sk-sp-…" : "sk-…"}
          value={apiKey}
          onChange={setApiKey}
          style={addFlowTextInputStyle}
        />
      </FormRow>
      <AddFlowHint>
        Standard is the right default for `qwen3.6-plus`. Use Coding Plan only
        when you have a dedicated Coding Plan key and base URL.
      </AddFlowHint>
      <div style={{ display: "flex", gap: 8, paddingTop: 4 }}>
        <button
          className="btn btn-primary btn-sm"
          onClick={submit}
          disabled={submitting}
        >
          {submitting ?
            <>
              <Spin size={13} /> Connecting…
            </>
          : "Add Provider"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

function AddFlowModelScopeForm({
  onSubmit,
  onCancel,
  submitting,
}: AddFlowFormProps) {
  const [endpoint, setEndpoint] = useState("")
  const [apiKey, setApiKey] = useState("")
  const submit = async () => {
    if (!apiKey.trim()) return
    const body: Record<string, string> = {
      method: "api-key",
      apiKey: apiKey.trim(),
    }
    if (endpoint.trim()) {
      body.endpoint = endpoint.trim()
    }
    await onSubmit(body)
  }
  return (
    <div style={addFlowPanelStyle}>
      <AddFlowHint>
        Only API key authentication is supported for ModelScope.
      </AddFlowHint>
      <FormRow label="Base URL (optional)">
        <input
          type="text"
          placeholder="https://api-inference.modelscope.cn/v1"
          value={endpoint}
          onChange={(e) => setEndpoint(e.target.value)}
          style={addFlowTextInputStyle}
        />
      </FormRow>
      <FormRow label="ModelScope API Key">
        <SecretInput
          placeholder="sk-…"
          value={apiKey}
          onChange={setApiKey}
          style={addFlowTextInputStyle}
        />
      </FormRow>
      <AddFlowHint>
        ModelScope provides access to community models like DeepSeek, Qwen, and
        more. Get your API key from modelscope.cn.
      </AddFlowHint>
      <div style={{ display: "flex", gap: 8, paddingTop: 4 }}>
        <button
          className="btn btn-primary btn-sm"
          onClick={submit}
          disabled={submitting}
        >
          {submitting ?
            <>
              <Spin size={13} /> Connecting…
            </>
          : "Add Provider"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

function AddFlowCopilotForm({
  onSubmit,
  onCancel,
  submitting,
}: AddFlowFormProps) {
  const [token, setToken] = useState("")
  const submitToken = async () => {
    if (!token.trim()) return
    await onSubmit({ method: "token", token: token.trim() })
  }
  const submitOAuth = async () => {
    await onSubmit({ method: "oauth" })
  }
  return (
    <div style={addFlowPanelStyle}>
      {/* Primary: OAuth */}
      <button
        className="btn btn-primary btn-sm"
        onClick={submitOAuth}
        disabled={submitting}
        style={{ width: "100%" }}
      >
        {submitting ?
          <>
            <Spin size={13} /> Connecting…
          </>
        : "Sign in with GitHub (OAuth)"}
      </button>
      <AddFlowHint>
        Recommended. Opens a browser window to complete the GitHub device code
        flow — no token needed.
      </AddFlowHint>

      <div className="divider-label">or use a token</div>

      {/* Secondary: Token */}
      <FormRow label="GitHub Token">
        <SecretInput
          placeholder="ghu_…"
          value={token}
          onChange={setToken}
          style={addFlowTextInputStyle}
          autoComplete="off"
        />
      </FormRow>
      <AddFlowHint>
        Generate a token from GitHub Settings → Developer settings → Personal
        access tokens.
      </AddFlowHint>
      <div style={{ display: "flex", gap: 8 }}>
        <button
          className="btn btn-outline btn-sm"
          onClick={submitToken}
          disabled={submitting}
        >
          {submitting ?
            <>
              <Spin size={13} /> Connecting…
            </>
          : "Add with Token"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

function AddFlowCodexForm({
  onSubmit,
  onCancel,
  submitting,
}: AddFlowFormProps) {
  const [apiKey, setApiKey] = useState("")

  const submit = async () => {
    if (!apiKey.trim()) return
    await onSubmit({ method: "api-key", apiKey: apiKey.trim() })
  }

  return (
    <div style={addFlowPanelStyle}>
      <AddFlowHint>
        Official Codex supports ChatGPT sign-in and API key authentication. Use
        an OpenAI API key here for programmatic access.
      </AddFlowHint>

      <FormRow label="OpenAI API Key">
        <SecretInput
          placeholder="sk-..."
          value={apiKey}
          onChange={setApiKey}
          style={addFlowTextInputStyle}
          autoComplete="off"
        />
      </FormRow>
      <div style={{ display: "flex", gap: 8 }}>
        <button
          className="btn btn-primary btn-sm"
          onClick={submit}
          disabled={submitting}
        >
          {submitting ?
            <>
              <Spin size={13} /> Connecting…
            </>
          : "Add with API Key"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

function AddFlowAntigravityForm({
  onSuccess,
  onCancel,
}: {
  onSuccess: (providerId: string) => void
  onCancel: () => void
}) {
  const [clientId, setClientId] = useState("")
  const [clientSecret, setClientSecret] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const { startOAuth } = useAntigravityOAuth(
    undefined,
    () => {
      setLoading(false)
      // provider_id unknown here; caller will refresh the list
      onSuccess("")
    },
    (msg) => {
      setLoading(false)
      setError(msg)
    },
  )

  const submit = async () => {
    if (!clientId.trim() || !clientSecret.trim()) return
    setLoading(true)
    setError(null)
    try {
      await startOAuth(clientId.trim(), clientSecret.trim())
    } catch (e) {
      setLoading(false)
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <div style={addFlowPanelStyle}>
      {error && (
        <div
          style={{ fontSize: 12, color: "var(--color-error)", marginBottom: 8 }}
        >
          {error}
        </div>
      )}
      <FormRow label="OAuth Client ID">
        <input
          type="text"
          placeholder="…apps.googleusercontent.com"
          value={clientId}
          onChange={(e) => setClientId(e.target.value)}
          style={addFlowTextInputStyle}
          autoComplete="off"
        />
      </FormRow>
      <FormRow label="OAuth Client Secret">
        <SecretInput
          placeholder="GOCSPX-…"
          value={clientSecret}
          onChange={setClientSecret}
          style={addFlowTextInputStyle}
          autoComplete="off"
        />
      </FormRow>
      <AddFlowHint>Opens a Google OAuth popup to complete sign-in.</AddFlowHint>
      <div style={{ display: "flex", gap: 8 }}>
        <button
          className="btn btn-primary btn-sm"
          onClick={submit}
          disabled={loading}
        >
          {loading ?
            <>
              <Spin size={13} /> Waiting…
            </>
          : "Start OAuth →"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={loading}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

function AddFlowAzureForm({
  onSubmit,
  onCancel,
  submitting,
}: AddFlowFormProps) {
  const [apiKey, setApiKey] = useState("")
  const [endpoint, setEndpoint] = useState("")
  const [apiVersion, setApiVersion] = useState("")
  const [mappings, setMappings] = useState<Array<AzureDeploymentMapping>>([
    { model: "", deployment: "" },
  ])

  const updateMapping = (
    index: number,
    field: keyof AzureDeploymentMapping,
    value: string,
  ) => {
    setMappings((prev) =>
      prev.map((mapping, i) =>
        i === index ? { ...mapping, [field]: value } : mapping,
      ),
    )
  }

  const addMapping = () => {
    setMappings((prev) => [...prev, { model: "", deployment: "" }])
  }

  const removeMapping = (index: number) => {
    setMappings((prev) =>
      prev.length === 1 ?
        [{ model: "", deployment: "" }]
      : prev.filter((_, i) => i !== index),
    )
  }

  const submit = async () => {
    if (!apiKey.trim()) return

    await onSubmit({
      apiKey: apiKey.trim(),
      endpoint: endpoint.trim(),
      apiVersion: apiVersion.trim() || "2024-02-01",
      deployments: serializeAzureDeploymentMappings(mappings),
    })
  }

  return (
    <div style={addFlowPanelStyle}>
      <FormRow label="Endpoint (optional)">
        <input
          type="text"
          placeholder="https://your-resource.openai.azure.com"
          value={endpoint}
          onChange={(e) => setEndpoint(e.target.value)}
          style={addFlowTextInputStyle}
          autoComplete="off"
        />
      </FormRow>
      <FormRow label="API Version (optional)">
        <input
          type="text"
          placeholder="2024-02-01"
          value={apiVersion}
          onChange={(e) => setApiVersion(e.target.value)}
          style={addFlowTextInputStyle}
          autoComplete="off"
        />
      </FormRow>
      <FormRow label="Models + Deployments (optional)">
        <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
          {mappings.map((mapping, index) => (
            <div
              key={index}
              style={{
                display: "grid",
                gridTemplateColumns: "1fr 1fr auto",
                gap: 8,
                alignItems: "center",
              }}
            >
              <input
                type="text"
                placeholder="Model name (e.g. gpt-5.4)"
                value={mapping.model}
                onChange={(e) => updateMapping(index, "model", e.target.value)}
                style={addFlowTextInputStyle}
                autoComplete="off"
              />
              <input
                type="text"
                placeholder="Deployment name"
                value={mapping.deployment}
                onChange={(e) =>
                  updateMapping(index, "deployment", e.target.value)
                }
                style={addFlowTextInputStyle}
                autoComplete="off"
              />
              <button
                className="btn btn-ghost btn-sm"
                onClick={() => removeMapping(index)}
                disabled={submitting}
                type="button"
              >
                Remove
              </button>
            </div>
          ))}
          <button
            className="btn btn-ghost btn-sm"
            onClick={addMapping}
            disabled={submitting}
            type="button"
            style={{ alignSelf: "flex-start" }}
          >
            + Add model mapping
          </button>
        </div>
      </FormRow>
      <FormRow label="API Key">
        <SecretInput
          placeholder="Enter your Azure OpenAI API key"
          value={apiKey}
          onChange={setApiKey}
          style={addFlowTextInputStyle}
          autoComplete="off"
        />
      </FormRow>
      <AddFlowHint>
        Configure app-facing model names separately from Azure deployment names.
        You can also update mappings later from the provider’s Models menu.
      </AddFlowHint>
      <div style={{ display: "flex", gap: 8 }}>
        <button
          className="btn btn-primary btn-sm"
          onClick={submit}
          disabled={submitting}
        >
          {submitting ?
            <>
              <Spin size={13} /> Connecting…
            </>
          : "Add Provider"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

function AddFlowGoogleForm({
  onSubmit,
  onCancel,
  submitting,
}: AddFlowFormProps) {
  const [apiKey, setApiKey] = useState("")
  const submit = async () => {
    await onSubmit({ method: "api-key", apiKey })
  }
  return (
    <div>
      <FormRow label="API Key">
        <SecretInput
          placeholder="Enter your Google Gemini API key"
          value={apiKey}
          onChange={setApiKey}
          style={addFlowTextInputStyle}
        />
      </FormRow>
      <AddFlowHint>
        You can obtain an API key from{" "}
        <a
          href="https://aistudio.google.com/apikey"
          target="_blank"
          rel="noopener noreferrer"
          style={{ color: "var(--color-blue)" }}
        >
          Google AI Studio
        </a>
        .
      </AddFlowHint>
      <div style={{ display: "flex", gap: 8 }}>
        <button
          className="btn btn-primary btn-sm"
          onClick={submit}
          disabled={submitting}
        >
          {submitting ?
            <>
              <Spin size={13} /> Connecting…
            </>
          : "Add Provider"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

function AddFlowKimiForm({ onSubmit, onCancel, submitting }: AddFlowFormProps) {
  const [apiKey, setApiKey] = useState("")
  const submit = async () => {
    await onSubmit({ method: "api-key", apiKey })
  }
  return (
    <div>
      <FormRow label="API Key">
        <SecretInput
          placeholder="Enter your Kimi API key"
          value={apiKey}
          onChange={setApiKey}
          style={addFlowTextInputStyle}
        />
      </FormRow>
      <AddFlowHint>
        You can obtain an API key from{" "}
        <a
          href="https://platform.moonshot.cn/console/api-keys"
          target="_blank"
          rel="noopener noreferrer"
          style={{ color: "var(--color-blue)" }}
        >
          Moonshot AI Platform
        </a>
        .
      </AddFlowHint>
      <div style={{ display: "flex", gap: 8 }}>
        <button
          className="btn btn-primary btn-sm"
          onClick={submit}
          disabled={submitting}
        >
          {submitting ?
            <>
              <Spin size={13} /> Connecting…
            </>
          : "Add Provider"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

function AddFlowOpenAICompatibleForm({
  onSubmit,
  onCancel,
  submitting,
}: AddFlowFormProps) {
  const [endpoint, setEndpoint] = useState("")
  const [apiKey, setApiKey] = useState("")
  const [models, setModels] = useState<Array<string>>([])
  const [newModel, setNewModel] = useState("")
  const [apiFormat, setApiFormat] = useState<OpenAICompatibleAPIFormat>("")

  const addModel = () => {
    const raw = newModel.trim()
    if (!raw) return
    const ids = raw
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
    if (ids.length === 0) return
    setModels((prev) => {
      const next = [...prev]
      for (const id of ids) {
        if (!next.includes(id)) next.push(id)
      }
      return next
    })
    setNewModel("")
  }

  const removeModel = (id: string) => {
    setModels((prev) => prev.filter((m) => m !== id))
  }

  const submit = async () => {
    if (!endpoint.trim()) return
    await onSubmit({
      endpoint: endpoint.trim(),
      apiKey: apiKey.trim(),
      ...(apiFormat ? { apiFormat } : {}),
      ...(models.length > 0 ? { models: JSON.stringify(models) } : {}),
    })
  }

  return (
    <div style={addFlowPanelStyle}>
      <FormRow label="Base URL">
        <input
          type="text"
          placeholder="http://localhost:11434/v1"
          value={endpoint}
          onChange={(e) => setEndpoint(e.target.value)}
          style={addFlowTextInputStyle}
        />
      </FormRow>
      <FormRow label="API Key (optional)">
        <SecretInput
          placeholder="Leave empty for open endpoints (e.g. Ollama)"
          value={apiKey}
          onChange={setApiKey}
          style={addFlowTextInputStyle}
        />
      </FormRow>
      <FormRow label="Upstream API">
        <OpenAICompatibleAPIFormatControl
          value={apiFormat}
          onChange={setApiFormat}
          disabled={submitting}
        />
      </FormRow>
      <FormRow label="Models (optional)">
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          <div style={{ display: "flex", gap: 6 }}>
            <input
              type="text"
              placeholder="e.g. llama3, mistral, phi3"
              value={newModel}
              onChange={(e) => setNewModel(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault()
                  addModel()
                }
              }}
              style={{ ...addFlowTextInputStyle, flex: 1 }}
            />
            <button
              className="btn btn-ghost btn-sm"
              type="button"
              onClick={addModel}
              disabled={!newModel.trim()}
              style={{ whiteSpace: "nowrap" }}
            >
              + Add
            </button>
          </div>
          {models.length > 0 && (
            <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
              {models.map((m) => (
                <span
                  key={m}
                  style={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 4,
                    padding: "2px 8px",
                    borderRadius: 999,
                    background: "rgba(16,185,129,0.12)",
                    border: "1px solid rgba(16,185,129,0.3)",
                    fontSize: 12,
                    fontFamily: "var(--font-mono)",
                    color: "#10b981",
                  }}
                >
                  {m}
                  <button
                    type="button"
                    onClick={() => removeModel(m)}
                    style={{
                      background: "none",
                      border: "none",
                      cursor: "pointer",
                      color: "inherit",
                      padding: 0,
                      lineHeight: 1,
                      opacity: 0.7,
                    }}
                  >
                    ×
                  </button>
                </span>
              ))}
            </div>
          )}
        </div>
      </FormRow>
      <AddFlowHint>
        Connect to any OpenAI-compatible endpoint — Ollama, vLLM, LM Studio,
        llama.cpp, or a hosted service. Auto uses Responses for official OpenAI
        when traffic comes in through <code>/v1/messages</code> or{" "}
        <code>/v1/responses</code>, and Chat Completions otherwise. Add model
        IDs upfront, or add them later from the Models panel.
      </AddFlowHint>
      <div style={{ display: "flex", gap: 8 }}>
        <button
          className="btn btn-primary btn-sm"
          onClick={submit}
          disabled={submitting || !endpoint.trim()}
        >
          {submitting ?
            <>
              <Spin size={13} /> Connecting…
            </>
          : "Add Provider"}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          onClick={onCancel}
          disabled={submitting}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

// ─── Add Provider Modal ───────────────────────────────────────────────────────

const PROVIDER_TYPES = [
  {
    id: "github-copilot",
    name: "GitHub Copilot",
    desc: "Access Copilot models via token",
  },
  {
    id: "antigravity",
    name: "Antigravity (Google)",
    desc: "Google Vertex AI via OAuth client credentials",
  },
  {
    id: "alibaba",
    name: "Alibaba DashScope",
    desc: "Qwen models via API key",
  },
  {
    id: "alibaba-modelscope",
    name: "Alibaba ModelScope",
    desc: "ModelScope models (DeepSeek, Qwen, etc.) via API key",
  },
  {
    id: "azure-openai",
    name: "Azure OpenAI",
    desc: "Azure OpenAI Service with your own deployments",
  },
  {
    id: "google",
    name: "Google Gemini",
    desc: "Google Gemini API with your API key",
  },
  {
    id: "kimi",
    name: "Kimi (Moonshot)",
    desc: "Kimi models via API key",
  },
  {
    id: "codex",
    name: "Codex",
    desc: "Official OpenAI Codex via ChatGPT or API key",
  },
  {
    id: "openai-compatible",
    name: "OpenAI-Compatible",
    desc: "Any OpenAI-compatible endpoint (Ollama, vLLM, LM Studio, etc.)",
  },
]

// ─── Collapsible Group Header ─────────────────────────────────────────────────

// ─── Providers Page ───────────────────────────────────────────────────────────

export function ProvidersPage({ showToast }: ProvidersPageProps) {
  const [providers, setProviders] = useState<Array<Provider>>([])
  const [status, setStatus] = useState<Status | null>(null)
  const [loading, setLoading] = useState(true)
  const [activating, setActivating] = useState<string | null>(null)
  const [addingProvider, setAddingProvider] = useState(false)
  const [priorities, setPriorities] = useState<Record<string, number>>({})
  const [collapsedGroups, setCollapsedGroups] = useState<
    Record<string, boolean>
  >({})
  const [showActiveOnly, setShowActiveOnly] = useState(false)
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = useCallback(async () => {
    try {
      const [p, s, pri] = await Promise.all([
        listProviders(),
        getStatus(),
        getProviderPriorities(),
      ])
      setProviders(p)
      setStatus(s)
      setPriorities(pri.priorities)
    } catch (e) {
      showToast(
        "Failed to load: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setLoading(false)
    }
  }, [showToast])

  const stopPoll = useCallback(() => {
    if (pollTimer.current) {
      clearInterval(pollTimer.current)
      pollTimer.current = null
    }
  }, [])

  const startPoll = useCallback(() => {
    stopPoll()
    pollTimer.current = setInterval(async () => {
      try {
        const af = await getAuthStatus()
        setStatus((prev) => (prev ? { ...prev, authFlow: af } : prev))
        if (af?.status === "complete") {
          stopPoll()
          showToast("Authentication complete!")
          await load()
        } else if (af?.status === "error") {
          stopPoll()
          showToast("Auth failed: " + (af.error ?? "unknown"), "error")
          await load()
        }
      } catch {
        /* ignore */
      }
    }, 2000)
  }, [load, showToast, stopPoll])

  const handleCancelAuth = useCallback(async () => {
    stopPoll()
    setStatus((prev) => (prev ? { ...prev, authFlow: null } : prev))
    try {
      await cancelAuth()
    } catch {
      /* ignore */
    }
  }, [stopPoll])

  useEffect(() => {
    void load()
    return stopPoll
  }, [load, stopPoll])

  useEffect(() => {
    const authFlow = status?.authFlow
    if (
      authFlow
      && (authFlow.status === "pending" || authFlow.status === "awaiting_user")
    ) {
      startPoll()
    }
  }, [startPoll, status])

  const handleActivate = async (id: string) => {
    setActivating(id)
    try {
      const result = await activateProvider(id)
      if (result.success) {
        showToast(`Activated ${result.provider?.name ?? id}`)
        await load()
      }
    } catch (e) {
      showToast(
        "Activate failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setActivating(null)
    }
  }

  const handleDeactivate = async (id: string) => {
    setActivating(id)
    try {
      await deactivateProvider(id)
      showToast(`Deactivated`)
      await load()
    } catch (e) {
      showToast(
        "Deactivate failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setActivating(null)
    }
  }

  const handleDelete = async (id: string) => {
    setActivating(id)
    try {
      const result = await deleteProvider(id)
      showToast(result.message || `Deleted`)
      await load()
    } catch (e) {
      showToast(
        "Delete failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setActivating(null)
    }
  }

  const handleRename = async (
    id: string,
    fields: { name?: string; subtitle?: string },
  ) => {
    // Optimistic update — reflect the change in UI immediately
    const prev = providers.find((p) => p.id === id)
    setProviders((ps) =>
      ps.map((p) =>
        p.id === id ?
          {
            ...p,
            ...(fields.name !== undefined ? { name: fields.name } : {}),
            ...(fields.subtitle !== undefined ?
              { subtitle: fields.subtitle }
            : {}),
          }
        : p,
      ),
    )
    try {
      await renameProvider(id, fields)
      // Reconcile with server — do not await; let it happen in background
      void load()
    } catch (e) {
      // Roll back on error
      if (prev) {
        setProviders((ps) => ps.map((p) => (p.id === id ? prev : p)))
      }
      showToast(
        "Rename failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    }
  }

  const handleAuthSubmit = async (id: string, body: Record<string, string>) => {
    try {
      const result = await authProvider(id, body)
      if (result.success) {
        showToast("Authentication successful")
        await load()
      } else if (result.requiresAuth) {
        showToast("Follow the auth instructions above")
        startPoll()
        await load()
      }
    } catch (e) {
      showToast(
        "Auth failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    }
  }

  const handlePrioritiesChange = async (
    newPriorities: Record<string, number>,
  ) => {
    try {
      await setProviderPriorities(newPriorities)
      setPriorities(newPriorities)
      showToast("Priorities updated")
    } catch (e) {
      showToast(
        "Priority update failed: "
          + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    }
  }

  const isFlowRunning = ["awaiting_user", "pending"].includes(
    status?.authFlow?.status ?? "",
  )

  const ALL_PROVIDER_TYPES = PROVIDER_TYPE_IDS

  const providerGroups = providers.reduce<Record<string, Array<Provider>>>(
    (g, p) => {
      if (!g[p.type]) g[p.type] = []
      g[p.type].push(p)
      return g
    },
    {},
  )

  const completeGroups: Record<string, Array<Provider>> = {}
  for (const providerType of ALL_PROVIDER_TYPES) {
    completeGroups[providerType] = providerGroups[providerType] ?? []
  }

  // Determine if all groups are collapsed/expanded
  const groupsWithProviders = Object.entries(completeGroups).filter(
    ([_, typeProviders]) => typeProviders.length > 0,
  )
  const allCollapsed = groupsWithProviders.every(
    ([providerType, _]) => collapsedGroups[providerType] ?? false,
  )

  // Toggle all groups collapsed/expanded
  const toggleAllGroups = () => {
    const newState: Record<string, boolean> = {}
    for (const [providerType, _] of groupsWithProviders) {
      newState[providerType] = !allCollapsed
    }
    setCollapsedGroups(newState)
  }

  const activeProviders = providers
    .filter((p) => p.isActive)
    .sort((a, b) => (priorities[a.id] ?? 0) - (priorities[b.id] ?? 0))
  const totalActive = activeProviders.length

  if (loading && providers.length === 0) {
    return (
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          gap: 12,
          padding: "80px 0",
          color: "var(--color-text-secondary)",
          fontSize: 14,
        }}
      >
        <Spin size={16} /> Loading providers…
      </div>
    )
  }

  // ── Add Provider flow takes over the whole page ──────────────────────────────
  if (addingProvider) {
    return (
      <AddProviderFlow
        onDone={async () => {
          setAddingProvider(false)
          await load()
        }}
        onCancel={() => setAddingProvider(false)}
        showToast={showToast}
      />
    )
  }

  return (
    <div>
      <AuthFlowBanner
        authFlow={status?.authFlow}
        providers={providers}
        onCancel={handleCancelAuth}
        Spin={Spin}
      />

      {/* Page header */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: 28,
        }}
      >
        <div
          onClick={groupsWithProviders.length > 0 ? toggleAllGroups : undefined}
          style={{
            cursor: groupsWithProviders.length > 0 ? "pointer" : "default",
            userSelect: "none",
            display: "flex",
            alignItems: "center",
            gap: 8,
          }}
        >
          <div>
            <h1
              style={{
                fontFamily: "var(--font-display)",
                fontWeight: 700,
                fontSize: 26,
                color: "var(--color-text)",
                letterSpacing: "-0.02em",
                lineHeight: 1,
              }}
            >
              Providers
            </h1>
            <p
              style={{
                fontSize: 13,
                color: "var(--color-text-secondary)",
                marginTop: 5,
              }}
            >
              {totalActive > 0 ?
                <>
                  <span
                    style={{ color: "var(--color-green)", fontWeight: 500 }}
                  >
                    {totalActive} active
                  </span>{" "}
                  · {providers.length} total instances
                </>
              : `${providers.length} instance${providers.length !== 1 ? "s" : ""} · none active`
              }
            </p>
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <button
            className={
              showActiveOnly ?
                "btn btn-primary btn-sm"
              : "btn btn-secondary btn-sm"
            }
            onClick={() => setShowActiveOnly((v) => !v)}
          >
            {showActiveOnly ? "All" : "Active only"}
          </button>
          <PriorityModal
            providers={providers}
            priorities={priorities}
            onPrioritiesChange={handlePrioritiesChange}
          />
          <button
            className="btn btn-primary btn-sm"
            disabled={isFlowRunning}
            onClick={() => setAddingProvider(true)}
          >
            Add Provider
          </button>
        </div>
      </div>

      {/* Quick stats */}
      <StatsBar providers={providers} totalActive={totalActive} />

      {/* Provider groups */}
      <div style={{ display: "flex", flexDirection: "column", gap: 32 }}>
        {Object.entries(completeGroups)
          .map(([providerType, typeProviders]) => {
            const visibleProviders =
              showActiveOnly ?
                typeProviders.filter((p) => p.isActive)
              : typeProviders
            if (visibleProviders.length === 0) return null
            return [providerType, visibleProviders] as [string, Array<Provider>]
          })
          .filter((entry): entry is [string, Array<Provider>] => entry !== null)
          .sort(([, a], [, b]) => {
            if (a.length > 0 && b.length === 0) return -1
            if (a.length === 0 && b.length > 0) return 1
            return 0
          })
          .map(([providerType, typeProviders]) => {
            const isCollapsed = collapsedGroups[providerType] ?? false
            const accent = PROVIDER_ACCENT[providerType] ?? "#0a84ff"

            return (
              <div key={providerType}>
                {/* Group header */}
                <GroupHeader
                  providerType={providerType}
                  typeProviders={typeProviders}
                  isCollapsed={isCollapsed}
                  accent={accent}
                  onToggle={() =>
                    setCollapsedGroups((prev) => ({
                      ...prev,
                      [providerType]: !prev[providerType],
                    }))
                  }
                />

                {!isCollapsed && (
                  <div
                    style={{
                      display: "flex",
                      flexDirection: "column",
                      gap: 10,
                    }}
                  >
                    {typeProviders.length > 0 ?
                      typeProviders.map((p) => (
                        <ProviderCard
                          key={p.id}
                          provider={p}
                          isFlowRunning={isFlowRunning}
                          isActivating={activating === p.id}
                          onActivate={handleActivate}
                          onDeactivate={handleDeactivate}
                          onDelete={handleDelete}
                          onAuthSubmit={handleAuthSubmit}
                          onModelsChanged={load}
                          onRename={handleRename}
                          priorityIndex={activeProviders.findIndex(
                            (x) => x.id === p.id,
                          )}
                          multiProvider={activeProviders.length >= 2}
                        />
                      ))
                    : <EmptyState
                        icon={
                          PROVIDER_ICONS[providerType] ?? (
                            <span style={{ fontSize: 20 }}>◌</span>
                          )
                        }
                        title={`No ${TYPE_NAMES[providerType]} accounts configured`}
                        description="Add an account to start using this provider type."
                        action={{
                          label: "Add Account",
                          onClick: () => setAddingProvider(true),
                        }}
                      />
                    }
                  </div>
                )}
              </div>
            )
          })}
      </div>
    </div>
  )
}
