import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react"
import { useTranslation } from "react-i18next"

import {
  getMeteringByModel,
  getMeteringByProvider,
  getMeteringClients,
  getMeteringLogs,
  getMeteringModels,
  getMeteringProviders,
  getMeteringStats,
  type MeteringBreakdownItem,
  type MeteringLogsResponse,
  type MeteringQuery,
  type MeteringRecord,
  type MeteringStats,
} from "@/api"
import { SearchableSelect } from "@/components/SearchableSelect"

type DateRangePreset = "today" | "yesterday" | "last7d" | "last30d" | "custom"

type StoredDateRange = {
  preset: DateRangePreset
  since: string
  until: string
}

const METERING_DATE_RANGE_KEY = "olp-metering-date-range"

function sortItems<T>(
  items: Array<T>,
  sortKey: keyof T | null,
  sortDirection: "asc" | "desc",
) {
  if (!sortKey) return [...items]

  return [...items].sort((left, right) => {
    const leftValue = left[sortKey]
    const rightValue = right[sortKey]

    if (typeof leftValue === "number" && typeof rightValue === "number") {
      return sortDirection === "asc" ?
          leftValue - rightValue
        : rightValue - leftValue
    }

    if (typeof leftValue === "boolean" && typeof rightValue === "boolean") {
      const leftNumber = Number(leftValue)
      const rightNumber = Number(rightValue)
      return sortDirection === "asc" ?
          leftNumber - rightNumber
        : rightNumber - leftNumber
    }

    const leftText = String(leftValue ?? "")
    const rightText = String(rightValue ?? "")
    return sortDirection === "asc" ?
        leftText.localeCompare(rightText)
      : rightText.localeCompare(leftText)
  })
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}

function formatLatency(value: number) {
  return `${Math.round(value)} ms`
}

function formatDateTime(value: string) {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function isoInputValue(date: Date) {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  const hours = String(date.getHours()).padStart(2, "0")
  const minutes = String(date.getMinutes()).padStart(2, "0")
  return `${year}-${month}-${day}T${hours}:${minutes}`
}

function startOfToday() {
  const value = new Date()
  value.setHours(0, 0, 0, 0)
  return value
}

function startOfYesterday() {
  const value = startOfToday()
  value.setDate(value.getDate() - 1)
  return value
}

function getPresetRange(preset: Exclude<DateRangePreset, "custom">): {
  since: string
  until: string
} {
  switch (preset) {
    case "today": {
      return { since: isoInputValue(startOfToday()), until: "" }
    }
    case "yesterday": {
      const since = startOfYesterday()
      const until = startOfToday()
      return { since: isoInputValue(since), until: isoInputValue(until) }
    }
    case "last7d": {
      const since = startOfToday()
      since.setDate(since.getDate() - 6)
      return { since: isoInputValue(since), until: "" }
    }
    case "last30d": {
      const since = startOfToday()
      since.setDate(since.getDate() - 29)
      return { since: isoInputValue(since), until: "" }
    }
    default: {
      return { since: isoInputValue(startOfToday()), until: "" }
    }
  }
}

function loadStoredDateRange(): StoredDateRange {
  const fallback = { preset: "today" as const, ...getPresetRange("today") }

  try {
    const raw = localStorage.getItem(METERING_DATE_RANGE_KEY)
    if (!raw) return fallback

    const parsed = JSON.parse(raw) as Partial<StoredDateRange>
    if (
      parsed.preset !== "today"
      && parsed.preset !== "yesterday"
      && parsed.preset !== "last7d"
      && parsed.preset !== "last30d"
      && parsed.preset !== "custom"
    ) {
      return fallback
    }

    return {
      preset: parsed.preset,
      since: typeof parsed.since === "string" ? parsed.since : fallback.since,
      until: typeof parsed.until === "string" ? parsed.until : fallback.until,
    }
  } catch {
    return fallback
  }
}

function saveStoredDateRange(value: StoredDateRange) {
  try {
    localStorage.setItem(METERING_DATE_RANGE_KEY, JSON.stringify(value))
  } catch {
    // ignore
  }
}

function Card({
  children,
  style,
}: {
  children: React.ReactNode
  style?: CSSProperties
}) {
  return (
    <section
      style={{
        background: "var(--color-bg-elevated)",
        borderRadius: "var(--radius-lg)",
        border: "1px solid var(--color-separator)",
        boxShadow: "var(--shadow-card)",
        overflow: "hidden",
        minWidth: 0,
        ...style,
      }}
    >
      {children}
    </section>
  )
}

function StatCard({
  label,
  value,
  accent,
  subtext,
}: {
  label: string
  value: string
  accent: string
  subtext?: string
}) {
  return (
    <Card
      style={{
        padding: 24,
        borderTop: "none",
        borderLeft: `3px solid ${accent}`,
        background: `linear-gradient(135deg, var(--color-bg-elevated) 0%, color-mix(in srgb, ${accent} 3%, var(--color-bg-elevated) 97%))`,
        transition: "all 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)",
        cursor: "default",
        position: "relative",
        overflow: "hidden",
      }}
    >
      <div
        style={{
          fontSize: 11,
          fontWeight: 700,
          letterSpacing: "0.08em",
          textTransform: "uppercase",
          color: "var(--color-text-tertiary)",
          marginBottom: 16,
        }}
      >
        {label}
      </div>
      <div
        style={{
          fontSize: 36,
          fontWeight: 700,
          color: accent,
          fontFamily: "var(--font-mono)",
          letterSpacing: "-0.02em",
          marginBottom: 12,
          lineHeight: 1,
        }}
      >
        {value}
      </div>
      {subtext && (
        <div
          style={{
            fontSize: 12,
            color: "var(--color-text-secondary)",
            lineHeight: 1.5,
          }}
        >
          {subtext}
        </div>
      )}
    </Card>
  )
}

function BreakdownTable({
  title,
  keyLabel,
  items,
  valueKey,
}: {
  title: string
  keyLabel: string
  items: Array<MeteringBreakdownItem> | null | undefined
  valueKey: "model_id" | "provider_id" | "client"
}) {
  const { t } = useTranslation("metering")
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(5)
  const [sortKey, setSortKey] = useState<
    | "requests"
    | "input_tokens"
    | "output_tokens"
    | "total_tokens"
    | "avg_latency_ms"
    | "model_id"
    | "provider_id"
    | "client"
  >("total_tokens")
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc")
  const safeItems = items ?? []

  const sortedItems = useMemo(
    () => sortItems(safeItems, sortKey, sortDirection),
    [safeItems, sortDirection, sortKey],
  )
  const totalPages = Math.max(1, Math.ceil(sortedItems.length / pageSize))

  useEffect(() => {
    setPage(1)
  }, [items, pageSize, sortDirection, sortKey])

  const pageItems = sortedItems.slice((page - 1) * pageSize, page * pageSize)

  const handleSort = (
    nextKey:
      | "requests"
      | "input_tokens"
      | "output_tokens"
      | "total_tokens"
      | "avg_latency_ms"
      | "model_id"
      | "provider_id"
      | "client",
  ) => {
    if (sortKey === nextKey) {
      setSortDirection((current) => (current === "asc" ? "desc" : "asc"))
      return
    }
    setSortKey(nextKey)
    let direction: "asc" | "desc" = "asc"
    if (nextKey !== valueKey) {
      direction = nextKey === "total_tokens" ? "desc" : "asc"
    }
    setSortDirection(direction)
  }

  const renderSortIndicator = (
    key:
      | "requests"
      | "input_tokens"
      | "output_tokens"
      | "total_tokens"
      | "avg_latency_ms"
      | "model_id"
      | "provider_id"
      | "client",
  ) => {
    if (sortKey !== key) return null
    return sortDirection === "asc" ? " ▲" : " ▼"
  }

  const groupedHeaders: Array<{
    key:
      | "requests"
      | "input_tokens"
      | "output_tokens"
      | "total_tokens"
      | "avg_latency_ms"
    label: string
  }> = [
    { key: "requests", label: t("table.requests") },
    { key: "input_tokens", label: t("table.input") },
    { key: "output_tokens", label: t("table.output") },
    { key: "total_tokens", label: t("table.total") },
    { key: "avg_latency_ms", label: t("table.avgLatency") },
  ]

  return (
    <Card>
      <div
        style={{
          padding: "14px 16px",
          borderBottom: "1px solid var(--color-separator)",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          gap: 12,
          flexWrap: "wrap",
        }}
      >
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: 10,
            minWidth: 220,
            flex: "1 1 260px",
          }}
        >
          <h2
            style={{
              margin: 0,
              fontSize: 15,
              fontWeight: 700,
              color: "var(--color-text)",
            }}
          >
            {title}
          </h2>
        </div>
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            flexWrap: "wrap",
            justifyContent: "flex-end",
          }}
        >
          <label
            style={{
              display: "flex",
              flexDirection: "row",
              alignItems: "center",
              gap: 6,
              fontSize: 12,
              color: "var(--color-text-secondary)",
              whiteSpace: "nowrap",
            }}
          >
            {t("filters.rowsPerPage")}
            <select
              className="sys-select"
              style={{ fontSize: 12, padding: "2px 6px" }}
              value={pageSize}
              onChange={(e) => setPageSize(Number(e.target.value))}
            >
              <option value={5}>5</option>
              <option value={10}>10</option>
              <option value={15}>15</option>
            </select>
          </label>
          <span
            style={{
              fontSize: 11,
              color: "var(--color-text-tertiary)",
              fontFamily: "var(--font-mono)",
            }}
          >
            {t("pagination.rows", { count: sortedItems.length })}
          </span>
        </div>
      </div>
      <div style={{ overflowX: "auto" }}>
        <table
          style={{ width: "100%", minWidth: 720, borderCollapse: "collapse" }}
        >
          <thead>
            <tr style={{ background: "rgba(255,255,255,0.02)" }}>
              <th
                onClick={() => handleSort(valueKey)}
                style={{
                  ...groupedHeaderCellStyle,
                  minWidth: 180,
                  cursor: "pointer",
                }}
              >
                {keyLabel}
                {renderSortIndicator(valueKey)}
              </th>
              {groupedHeaders.map((header) => (
                <th
                  key={header.key}
                  onClick={() => handleSort(header.key)}
                  style={{
                    ...groupedHeaderCellStyle,
                    minWidth: 100,
                    cursor: "pointer",
                  }}
                >
                  {header.label}
                  {renderSortIndicator(header.key)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {sortedItems.length === 0 && (
              <tr>
                <td
                  colSpan={6}
                  style={{
                    padding: "24px 14px",
                    color: "var(--color-text-secondary)",
                    textAlign: "center",
                  }}
                >
                  {t("table.noData")}
                </td>
              </tr>
            )}
            {pageItems.map((item, index) => (
              <tr
                key={`${item[valueKey] ?? "unknown"}-${index}`}
                style={{ transition: "background-color 0.15s ease" }}
                onMouseEnter={(e) => {
                  const el = e.currentTarget
                  el.style.backgroundColor = "rgba(255,255,255,0.02)"
                }}
                onMouseLeave={(e) => {
                  const el = e.currentTarget
                  el.style.backgroundColor = "transparent"
                }}
              >
                <td style={groupedKeyCellStyle}>{item[valueKey] ?? "—"}</td>
                <td style={cellStyle}>{formatNumber(item.requests)}</td>
                <td style={cellStyle}>{formatNumber(item.input_tokens)}</td>
                <td style={cellStyle}>{formatNumber(item.output_tokens)}</td>
                <td style={cellStyle}>{formatNumber(item.total_tokens)}</td>
                <td style={cellStyle}>{formatLatency(item.avg_latency_ms)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {totalPages > 1 && (
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            gap: 12,
            padding: "10px 16px",
            borderTop: "1px solid var(--color-separator)",
          }}
        >
          <div style={{ fontSize: 12, color: "var(--color-text-secondary)" }}>
            {t("pagination.page", { current: page, total: totalPages })}
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <button
              className="btn btn-ghost btn-sm"
              disabled={page <= 1}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
            >
              {t("pagination.previous")}
            </button>
            <button
              className="btn btn-ghost btn-sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              {t("pagination.next")}
            </button>
          </div>
        </div>
      )}
    </Card>
  )
}

const groupedHeaderCellStyle: CSSProperties = {
  textAlign: "left",
  padding: "12px 14px",
  fontSize: 11,
  textTransform: "uppercase",
  letterSpacing: "0.06em",
  color: "var(--color-text-tertiary)",
  borderBottom: "1px solid var(--color-separator)",
  whiteSpace: "nowrap",
  fontWeight: 600,
  transition: "background-color 0.2s ease, color 0.2s ease",
}

const groupedKeyCellStyle: CSSProperties = {
  padding: "12px 14px",
  borderBottom: "1px solid var(--color-separator)",
  color: "var(--color-text)",
  fontWeight: 600,
  minWidth: 180,
}

const cellStyle: CSSProperties = {
  padding: "12px 14px",
  borderBottom: "1px solid var(--color-separator)",
  color: "var(--color-text-secondary)",
  fontFamily: "var(--font-mono)",
  fontSize: 12,
  transition: "background-color 0.15s ease",
}

const pageStyles = `
  @keyframes pulse {
    0%, 100% { opacity: 1 }
    50% { opacity: 0.5 }
  }

  table tbody tr:hover {
    background-color: rgba(255, 255, 255, 0.02);
  }

  table th {
    transition: background-color 0.2s ease, color 0.2s ease;
  }

  table th:hover {
    background-color: rgba(255, 255, 255, 0.03);
    color: var(--color-text);
  }
`

export function MeteringPage({
  showToast,
}: {
  showToast: (msg: string, type?: "success" | "error") => void
}) {
  const { t } = useTranslation("metering")
  const initialDateRange = useMemo(loadStoredDateRange, [])

  const [selectedRangePreset, setSelectedRangePreset] =
    useState<DateRangePreset>(initialDateRange.preset)
  const [since, setSince] = useState(initialDateRange.since)
  const [until, setUntil] = useState(initialDateRange.until)
  const [modelId, setModelId] = useState("")
  const [providerId, setProviderId] = useState("")
  const [client, setClient] = useState("")
  const [apiShape, setAPIShape] = useState("")
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(50)
  const [maxRows, setMaxRows] = useState(500)
  const [reloadKey, setReloadKey] = useState(0)
  const [autoRefreshInterval, setAutoRefreshInterval] = useState<number | null>(
    null,
  )
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const [loading, setLoading] = useState(true)
  const [stats, setStats] = useState<MeteringStats | null>(null)
  const [logs, setLogs] = useState<MeteringLogsResponse | null>(null)
  const [byModel, setByModel] = useState<Array<MeteringBreakdownItem>>([])
  const [byProvider, setByProvider] = useState<Array<MeteringBreakdownItem>>([])
  const [logSortKey, setLogSortKey] =
    useState<keyof MeteringRecord>("created_at")
  const [logSortDirection, setLogSortDirection] = useState<"asc" | "desc">(
    "desc",
  )
  const [modelOptions, setModelOptions] = useState<Array<string>>([])
  const [providerOptions, setProviderOptions] = useState<Array<string>>([])
  const [clientOptions, setClientOptions] = useState<Array<string>>([])

  const query = useMemo<MeteringQuery>(
    () => ({
      since: since ? new Date(since).toISOString() : undefined,
      until: until ? new Date(until).toISOString() : undefined,
      model_id: modelId || undefined,
      provider_id: providerId || undefined,
      client: client || undefined,
      api_shape: apiShape || undefined,
      page,
      page_size: pageSize,
    }),
    [
      apiShape,
      client,
      modelId,
      page,
      pageSize,
      providerId,
      reloadKey,
      since,
      until,
    ],
  )

  const modelOptionsQuery = useMemo<MeteringQuery>(
    () => ({
      since: since ? new Date(since).toISOString() : undefined,
      until: until ? new Date(until).toISOString() : undefined,
      provider_id: providerId || undefined,
      client: client || undefined,
      api_shape: apiShape || undefined,
    }),
    [apiShape, client, providerId, reloadKey, since, until],
  )

  const providerOptionsQuery = useMemo<MeteringQuery>(
    () => ({
      since: since ? new Date(since).toISOString() : undefined,
      until: until ? new Date(until).toISOString() : undefined,
      model_id: modelId || undefined,
      client: client || undefined,
      api_shape: apiShape || undefined,
    }),
    [apiShape, client, modelId, reloadKey, since, until],
  )

  const clientOptionsQuery = useMemo<MeteringQuery>(
    () => ({
      since: since ? new Date(since).toISOString() : undefined,
      until: until ? new Date(until).toISOString() : undefined,
      model_id: modelId || undefined,
      provider_id: providerId || undefined,
      api_shape: apiShape || undefined,
    }),
    [apiShape, modelId, providerId, reloadKey, since, until],
  )

  const stopAutoRefresh = () => {
    if (pollTimer.current) {
      clearInterval(pollTimer.current)
      pollTimer.current = null
    }
  }

  const startAutoRefresh = (intervalSeconds: number) => {
    stopAutoRefresh()
    pollTimer.current = setInterval(() => {
      setPage(1)
      setReloadKey((value) => value + 1)
    }, intervalSeconds * 1000)
  }

  const handleAutoRefreshChange = (intervalSeconds: number | null) => {
    setAutoRefreshInterval(intervalSeconds)
    if (intervalSeconds) {
      startAutoRefresh(intervalSeconds)
    } else {
      stopAutoRefresh()
    }
  }

  const applyDateRange = (
    preset: DateRangePreset,
    nextSince: string,
    nextUntil: string,
  ) => {
    setSelectedRangePreset(preset)
    setSince(nextSince)
    setUntil(nextUntil)
    setPage(1)
  }

  const applyPreset = (preset: Exclude<DateRangePreset, "custom">) => {
    const range = getPresetRange(preset)
    applyDateRange(preset, range.since, range.until)
  }

  useEffect(() => {
    saveStoredDateRange({
      preset: selectedRangePreset,
      since,
      until,
    })
  }, [selectedRangePreset, since, until])

  useEffect(() => {
    let cancelled = false
    setLoading(true)

    Promise.all([
      getMeteringStats(query),
      getMeteringLogs(query),
      getMeteringByModel(query),
      getMeteringByProvider(query),
      getMeteringModels(modelOptionsQuery),
      getMeteringProviders(providerOptionsQuery),
      getMeteringClients(clientOptionsQuery),
    ])
      .then(
        ([
          nextStats,
          nextLogs,
          nextByModel,
          nextByProvider,
          nextModels,
          nextProviders,
          nextClients,
        ]) => {
          if (cancelled) return
          setStats(nextStats)
          setLogs({ ...nextLogs, items: nextLogs.items ?? [] })
          setByModel(nextByModel.items ?? [])
          setByProvider(nextByProvider.items ?? [])
          setModelOptions(nextModels.items ?? [])
          setProviderOptions(nextProviders.items ?? [])
          setClientOptions(nextClients.items ?? [])
        },
      )
      .catch((e: unknown) => {
        if (cancelled) return
        showToast(
          "Failed to load metering data: "
            + (e instanceof Error ? e.message : String(e)),
          "error",
        )
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
      stopAutoRefresh()
    }
  }, [
    clientOptionsQuery,
    modelOptionsQuery,
    providerOptionsQuery,
    query,
    showToast,
  ])

  const totalPages =
    logs ? Math.max(1, Math.ceil(logs.total / logs.page_size)) : 1
  const logSortOptions: Array<{ key: keyof MeteringRecord; label: string }> = [
    { key: "created_at", label: t("table.time") },
    { key: "model_id", label: t("table.model") },
    { key: "provider_id", label: t("table.provider") },
    { key: "client", label: t("table.client") },
    { key: "api_shape", label: t("table.shape") },
    { key: "input_tokens", label: t("table.input") },
    { key: "output_tokens", label: t("table.output") },
    { key: "total_tokens", label: t("table.total") },
    { key: "latency_ms", label: t("table.latency") },
    { key: "status_code", label: t("table.status") },
    { key: "is_stream", label: t("table.stream") },
  ]

  const handleLogSort = (nextKey: keyof MeteringRecord) => {
    if (logSortKey === nextKey) {
      setLogSortDirection((current) => (current === "asc" ? "desc" : "asc"))
      return
    }
    setLogSortKey(nextKey)
    setLogSortDirection(nextKey === "created_at" ? "desc" : "asc")
  }

  const renderLogSortIndicator = (key: keyof MeteringRecord) => {
    if (logSortKey !== key) return null
    return logSortDirection === "asc" ? " ▲" : " ▼"
  }

  const sortedLogs = useMemo(
    () => sortItems(logs?.items ?? [], logSortKey, logSortDirection),
    [logSortDirection, logSortKey, logs?.items],
  )
  const logItems = sortedLogs.slice(0, maxRows || undefined)

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
      <style>{pageStyles}</style>
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          gap: 20,
          flexWrap: "wrap",
        }}
      >
        <div>
          <h1
            style={{
              margin: 0,
              fontFamily: "var(--font-display)",
              fontWeight: 700,
              fontSize: 28,
              letterSpacing: "-0.02em",
              color: "var(--color-text)",
            }}
          >
            {t("title")}
          </h1>
          <p
            style={{
              margin: "8px 0 0",
              fontSize: 14,
              color: "var(--color-text-secondary)",
              maxWidth: 720,
            }}
          >
            {t("description")}
          </p>
        </div>
        <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
          <button
            className="btn btn-ghost btn-sm"
            onClick={() => {
              setPage(1)
              setReloadKey((value) => value + 1)
            }}
            disabled={loading}
            title="Refresh metering data"
          >
            {t("refresh")}
          </button>
          <div
            style={{
              width: "1px",
              height: "24px",
              background: "var(--color-separator)",
            }}
          />
          <label
            style={{
              display: "flex",
              flexDirection: "row",
              alignItems: "center",
              gap: 8,
              fontSize: 12,
              color: "var(--color-text-secondary)",
              whiteSpace: "nowrap",
            }}
          >
            <select
              className="sys-select"
              style={{ fontSize: 12, padding: "4px 6px" }}
              value={autoRefreshInterval ?? ""}
              onChange={(e) => {
                const interval =
                  e.target.value ? Number.parseInt(e.target.value, 10) : null
                handleAutoRefreshChange(interval)
              }}
              title="Auto-refresh interval"
            >
              <option value="">Off</option>
              <option value="5">5s</option>
              <option value="10">10s</option>
              <option value="15">15s</option>
              <option value="30">30s</option>
            </select>
            {autoRefreshInterval && (
              <div
                style={{
                  width: 6,
                  height: 6,
                  borderRadius: "50%",
                  background: "var(--color-blue)",
                  animation: "pulse 2s infinite",
                }}
              />
            )}
          </label>
        </div>
      </div>

      <Card style={{ padding: 18, overflow: "visible" }}>
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: 14,
          }}
        >
          <div style={fieldStyle}>
            <span style={labelStyle}>{t("filters.quickRange")}</span>
            <div
              style={{
                display: "flex",
                gap: 8,
                flexWrap: "wrap",
              }}
            >
              {(["today", "yesterday", "last7d", "last30d"] as const).map(
                (preset) => {
                  const active = selectedRangePreset === preset
                  return (
                    <button
                      key={preset}
                      type="button"
                      className="btn btn-ghost btn-sm"
                      onClick={() => applyPreset(preset)}
                      style={{
                        borderColor:
                          active ? "var(--color-primary)" : undefined,
                        color: active ? "var(--color-text)" : undefined,
                        background:
                          active ?
                            "color-mix(in srgb, var(--color-primary) 16%, transparent)"
                          : undefined,
                      }}
                    >
                      {t(`filters.${preset}`)}
                    </button>
                  )
                },
              )}
              {selectedRangePreset === "custom" && (
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  style={{
                    borderColor: "var(--color-primary)",
                    color: "var(--color-text)",
                    background:
                      "color-mix(in srgb, var(--color-primary) 16%, transparent)",
                  }}
                >
                  {t("filters.custom")}
                </button>
              )}
            </div>
          </div>
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))",
              gap: 12,
            }}
          >
            <label style={fieldStyle}>
              <span style={labelStyle}>{t("filters.since")}</span>
              <input
                className="sys-input"
                type="datetime-local"
                value={since}
                onChange={(e) => {
                  applyDateRange("custom", e.target.value, until)
                }}
              />
            </label>
            <label style={fieldStyle}>
              <span style={labelStyle}>{t("filters.until")}</span>
              <input
                className="sys-input"
                type="datetime-local"
                value={until}
                onChange={(e) => {
                  applyDateRange("custom", since, e.target.value)
                }}
              />
            </label>
            <label style={fieldStyle}>
              <span style={labelStyle}>{t("filters.model")}</span>
              <SearchableSelect
                options={modelOptions}
                value={modelId}
                onChange={(v) => {
                  setModelId(v)
                  setPage(1)
                }}
                placeholder={t("filters.allModels")}
              />
            </label>
            <label style={fieldStyle}>
              <span style={labelStyle}>{t("filters.provider")}</span>
              <SearchableSelect
                options={providerOptions}
                value={providerId}
                onChange={(v) => {
                  setProviderId(v)
                  setPage(1)
                }}
                placeholder={t("filters.allProviders")}
              />
            </label>
            <label style={fieldStyle}>
              <span style={labelStyle}>{t("table.client")}</span>
              <SearchableSelect
                options={clientOptions}
                value={client}
                onChange={(v) => {
                  setClient(v)
                  setPage(1)
                }}
                placeholder={t("filters.allClients")}
              />
            </label>
            <label style={fieldStyle}>
              <span style={labelStyle}>{t("filters.apiShape")}</span>
              <select
                className="sys-select"
                value={apiShape}
                onChange={(e) => {
                  setAPIShape(e.target.value)
                  setPage(1)
                }}
              >
                <option value="">{t("filters.all")}</option>
                <option value="openai">openai</option>
                <option value="anthropic">anthropic</option>
                <option value="responses">responses</option>
              </select>
            </label>
          </div>
        </div>
      </Card>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(210px, 1fr))",
          gap: 14,
        }}
      >
        <StatCard
          label={t("stats.requests")}
          value={formatNumber(stats?.total_requests ?? 0)}
          accent="var(--color-blue)"
          subtext={t("stats.requestsSubtext")}
        />
        <StatCard
          label={t("stats.totalTokens")}
          value={formatNumber(stats?.total_tokens ?? 0)}
          accent="var(--color-green)"
          subtext={`${formatNumber(stats?.total_input_tokens ?? 0)} in / ${formatNumber(stats?.total_output_tokens ?? 0)} out`}
        />
        <StatCard
          label={t("stats.avgLatency")}
          value={formatLatency(stats?.avg_latency_ms ?? 0)}
          accent="var(--color-orange)"
          subtext={t("stats.avgLatencySubtext")}
        />
        <StatCard
          label={t("stats.errors")}
          value={formatNumber(stats?.error_count ?? 0)}
          accent="var(--color-red)"
          subtext={t("stats.errorsSubtext")}
        />
      </div>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))",
          gap: 16,
        }}
      >
        <BreakdownTable
          title={t("byModel")}
          keyLabel={t("table.model")}
          items={byModel}
          valueKey="model_id"
        />
        <BreakdownTable
          title={t("byProvider")}
          keyLabel={t("table.provider")}
          items={byProvider}
          valueKey="provider_id"
        />
      </div>

      <Card>
        <div
          style={{
            padding: "14px 16px",
            borderBottom: "1px solid var(--color-separator)",
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            gap: 12,
          }}
        >
          <div>
            <h2
              style={{
                margin: 0,
                fontSize: 15,
                fontWeight: 700,
                color: "var(--color-text)",
              }}
            >
              {t("requestLog")}
            </h2>
            <p
              style={{
                margin: "6px 0 0",
                fontSize: 12,
                color: "var(--color-text-secondary)",
              }}
            >
              {t("requestLogDescription")}
            </p>
          </div>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 12,
              flexWrap: "wrap",
            }}
          >
            <label
              style={{
                display: "flex",
                flexDirection: "row",
                alignItems: "center",
                gap: 6,
                fontSize: 12,
                color: "var(--color-text-secondary)",
                whiteSpace: "nowrap",
              }}
            >
              {t("filters.rowsPerPage")}
              <select
                className="sys-select"
                style={{ fontSize: 12, padding: "2px 6px" }}
                value={pageSize}
                onChange={(e) => {
                  setPageSize(Number(e.target.value))
                  setPage(1)
                }}
              >
                <option value={10}>10</option>
                <option value={20}>20</option>
                <option value={50}>50</option>
              </select>
            </label>
            <label
              style={{
                display: "flex",
                flexDirection: "row",
                alignItems: "center",
                gap: 6,
                fontSize: 12,
                color: "var(--color-text-secondary)",
                whiteSpace: "nowrap",
              }}
            >
              {t("filters.maxRows")}
              <select
                className="sys-select"
                style={{ fontSize: 12, padding: "2px 6px" }}
                value={maxRows}
                onChange={(e) => {
                  setMaxRows(Number(e.target.value))
                  setPage(1)
                }}
              >
                <option value={500}>500</option>
                <option value={1000}>1000</option>
                <option value={2000}>2000</option>
                <option value={0}>All</option>
              </select>
            </label>
            <div
              style={{
                fontSize: 11,
                color: "var(--color-text-tertiary)",
                fontFamily: "var(--font-mono)",
              }}
            >
              {loading ?
                t("pagination.loading")
              : `${logs?.total ?? 0} ${t("pagination.rows", { count: logs?.total ?? 0 })}`
              }
            </div>
          </div>
        </div>
        <div style={{ overflowX: "auto" }}>
          <table
            style={{
              width: "100%",
              minWidth: 1280,
              borderCollapse: "collapse",
            }}
          >
            <thead>
              <tr style={{ background: "rgba(255,255,255,0.02)" }}>
                {logSortOptions.map((option) => (
                  <th
                    key={option.key}
                    onClick={() => handleLogSort(option.key)}
                    style={{
                      ...groupedHeaderCellStyle,
                      minWidth: (() => {
                        switch (option.key) {
                          case "created_at": {
                            return 170
                          }
                          case "model_id": {
                            return 170
                          }
                          case "provider_id": {
                            return 210
                          }
                          case "client": {
                            return 160
                          }
                          default: {
                            return 100
                          }
                        }
                      })(),
                      cursor: "pointer",
                    }}
                  >
                    {option.label}
                    {renderLogSortIndicator(option.key)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {!loading && logItems.length === 0 && (
                <tr>
                  <td
                    colSpan={11}
                    style={{
                      padding: "28px 16px",
                      textAlign: "center",
                      color: "var(--color-text-secondary)",
                    }}
                  >
                    {t("table.noRecords")}
                  </td>
                </tr>
              )}
              {logItems.map((row: MeteringRecord) => (
                <tr
                  key={row.id}
                  style={{ transition: "background-color 0.15s ease" }}
                  onMouseEnter={(e) => {
                    const el = e.currentTarget
                    el.style.backgroundColor = "rgba(255,255,255,0.02)"
                  }}
                  onMouseLeave={(e) => {
                    const el = e.currentTarget
                    el.style.backgroundColor = "transparent"
                  }}
                >
                  <td style={{ ...cellStyle, minWidth: 170 }}>
                    {formatDateTime(row.created_at)}
                  </td>
                  <td style={{ ...cellStyle, minWidth: 170 }}>
                    {row.model_id}
                  </td>
                  <td style={{ ...cellStyle, minWidth: 210 }}>
                    {row.provider_id}
                  </td>
                  <td style={{ ...cellStyle, minWidth: 160 }}>
                    {row.client || "—"}
                  </td>
                  <td style={cellStyle}>{row.api_shape}</td>
                  <td style={cellStyle}>{formatNumber(row.input_tokens)}</td>
                  <td style={cellStyle}>{formatNumber(row.output_tokens)}</td>
                  <td style={cellStyle}>{formatNumber(row.total_tokens)}</td>
                  <td style={cellStyle}>{formatLatency(row.latency_ms)}</td>
                  <td
                    style={{
                      ...cellStyle,
                      color:
                        row.status_code >= 400 ?
                          "var(--color-red)"
                        : "var(--color-green)",
                    }}
                  >
                    {row.status_code}
                  </td>
                  <td style={cellStyle}>{row.is_stream ? "yes" : "no"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            gap: 12,
            padding: "12px 16px",
            borderTop: "1px solid var(--color-separator)",
            flexWrap: "wrap",
          }}
        >
          <div style={{ fontSize: 12, color: "var(--color-text-secondary)" }}>
            {t("pagination.page", {
              current: logs?.page ?? page,
              total: totalPages,
            })}
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <button
              className="btn btn-ghost btn-sm"
              disabled={page <= 1 || loading}
              onClick={() => setPage((value) => Math.max(1, value - 1))}
            >
              {t("pagination.previous")}
            </button>
            <button
              className="btn btn-ghost btn-sm"
              disabled={page >= totalPages || loading}
              onClick={() => setPage((value) => value + 1)}
            >
              {t("pagination.next")}
            </button>
          </div>
        </div>
      </Card>
    </div>
  )
}

const fieldStyle: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 6,
}

const labelStyle: CSSProperties = {
  fontSize: 11,
  fontWeight: 700,
  textTransform: "uppercase",
  letterSpacing: "0.06em",
  color: "var(--color-text-tertiary)",
}
