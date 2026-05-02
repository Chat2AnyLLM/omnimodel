/* eslint-disable @typescript-eslint/no-unnecessary-condition,@typescript-eslint/no-floating-promises,no-nested-ternary */
import { useState, useEffect, useCallback } from "react"

import {
  listVirtualModels,
  listProviders,
  getProviderModels,
  createVirtualModel,
  updateVirtualModel,
  renameVirtualModel,
  deleteVirtualModel,
  type VirtualModel,
  type VirtualModelUpstream,
  type LbStrategy,
  type Provider,
  type Model,
} from "@/api"
import { createLogger } from "@/lib/logger"
import { useConfirm } from "@/lib/useConfirm"
import {
  detectModelFamily,
  formatVirtualModelUpstreamSummary,
  resolveUpstreamProvider,
} from "@/lib/virtualmodels"

const _log = createLogger("virtualmodel-page")

interface Props {
  showToast: (msg: string, type?: "success" | "error") => void
}

const LB_STRATEGIES: Array<{ value: LbStrategy; label: string; hint: string }> =
  [
    {
      value: "round-robin",
      label: "Round-robin",
      hint: "Cycle through upstreams in order",
    },
    {
      value: "random",
      label: "Random",
      hint: "Pick a random upstream each request",
    },
    {
      value: "priority",
      label: "Priority / failover",
      hint: "Highest-priority upstream first",
    },
    {
      value: "weighted",
      label: "Weighted",
      hint: "Distribute by numeric weight",
    },
  ]

const emptyForm = (): Partial<VirtualModel> => ({
  virtual_model_id: "",
  name: "",
  description: "",
  api_shape: "openai",
  lb_strategy: "round-robin",
  enabled: true,
  upstreams: [],
})

interface UpstreamRow extends VirtualModelUpstream {
  selectedProviderId: string
}

const emptyUpstreamRow = (): UpstreamRow => ({
  model_id: "",
  weight: 1,
  priority: 0,
  selectedProviderId: "",
})

export function VirtualModelPage({ showToast }: Props) {
  const confirm = useConfirm()
  const [virtualmodels, setVirtualmodels] = useState<Array<VirtualModel>>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<VirtualModel | null>(null)
  const [form, setForm] = useState<Partial<VirtualModel>>(emptyForm())
  const [upstreamRows, setUpstreamRows] = useState<Array<UpstreamRow>>([
    emptyUpstreamRow(),
  ])
  const [saving, setSaving] = useState(false)
  const [isNew, setIsNew] = useState(false)

  const [providers, setProviders] = useState<Array<Provider>>([])
  const [providerModels, setProviderModels] = useState<
    Record<string, Array<Model>>
  >({})
  const [catalogueLoading, setCatalogueLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await listVirtualModels()
      setVirtualmodels(data)
    } catch {
      showToast("Failed to load virtual models", "error")
    } finally {
      setLoading(false)
    }
  }, [showToast])

  const loadCatalogue = useCallback(async () => {
    setCatalogueLoading(true)
    try {
      const provList = await listProviders()
      setProviders(provList)
      const entries = await Promise.all(
        provList.map(async (p) => {
          try {
            const res = await getProviderModels(p.id)
            return [p.id, res.models ?? []] as [string, Array<Model>]
          } catch {
            return [p.id, []] as [string, Array<Model>]
          }
        }),
      )
      setProviderModels(Object.fromEntries(entries))
    } catch {
      showToast("Failed to load providers", "error")
    } finally {
      setCatalogueLoading(false)
    }
  }, [showToast])

  useEffect(() => {
    load()
    loadCatalogue()
  }, [load, loadCatalogue])

  const setRowProvider = (i: number, providerId: string) => {
    setUpstreamRows((prev) =>
      prev.map((r, idx) =>
        idx === i ? { ...r, selectedProviderId: providerId, model_id: "" } : r,
      ),
    )
  }

  const setRowModel = (i: number, modelId: string) => {
    setUpstreamRows((prev) =>
      prev.map((r, idx) => (idx === i ? { ...r, model_id: modelId } : r)),
    )
  }

  const setRowNum = (
    i: number,
    field: "weight" | "priority",
    value: number,
  ) => {
    setUpstreamRows((prev) =>
      prev.map((r, idx) => (idx === i ? { ...r, [field]: value } : r)),
    )
  }

  const addRow = () => setUpstreamRows((prev) => [...prev, emptyUpstreamRow()])
  const removeRow = (i: number) =>
    setUpstreamRows((prev) => prev.filter((_, idx) => idx !== i))

  const openNew = () => {
    setSelected(null)
    setIsNew(true)
    setForm(emptyForm())
    setUpstreamRows([emptyUpstreamRow()])
  }

  const openEdit = (vm: VirtualModel) => {
    setSelected(vm)
    setIsNew(false)
    setForm({ ...vm })
    const rows: Array<UpstreamRow> = (
      vm.upstreams.length > 0 ?
        vm.upstreams
      : [{ model_id: "", weight: 1, priority: 0 }]).map((u) => {
      const selectedProviderId =
        u.provider_id
        ?? providers.find((p) =>
          (providerModels[p.id] ?? []).some((m) => m.id === u.model_id),
        )?.id
        ?? ""
      return { ...u, selectedProviderId }
    })
    setUpstreamRows(rows)
  }

  const closeForm = () => {
    setSelected(null)
    setIsNew(false)
    setForm(emptyForm())
    setUpstreamRows([emptyUpstreamRow()])
  }

  const handleSave = async () => {
    if (!form.virtual_model_id?.trim()) {
      showToast("Model ID is required", "error")
      return
    }
    if (!form.name?.trim()) {
      showToast("Display name is required", "error")
      return
    }
    if (!form.lb_strategy) {
      showToast("LB strategy is required", "error")
      return
    }

    const filledRows = upstreamRows.filter((r) => r.model_id.trim())
    if (filledRows.length === 0) {
      showToast("At least one upstream model is required", "error")
      return
    }

    setSaving(true)
    try {
      const payload = {
        virtual_model_id: form.virtual_model_id,
        name: form.name,
        description: form.description ?? "",
        api_shape: form.api_shape ?? "openai",
        lb_strategy: form.lb_strategy,
        enabled: form.enabled ?? true,
        upstreams: filledRows.map((r) => ({
          provider_id: r.selectedProviderId || undefined,
          model_id: r.model_id,
          weight: r.weight ?? 1,
          priority: r.priority ?? 0,
        })),
      }
      if (isNew) {
        await createVirtualModel(payload as VirtualModel)
        showToast("Virtual model created", "success")
      } else {
        const oldID = selected?.virtual_model_id ?? form.virtual_model_id
        const newID = form.virtual_model_id
        if (oldID !== newID) {
          await renameVirtualModel(oldID, newID)
        }
        await updateVirtualModel(newID, payload as VirtualModel)
        showToast("Virtual model updated", "success")
      }
      closeForm()
      await load()
    } catch (err: unknown) {
      showToast(
        err instanceof Error ? err.message : "Failed to save virtual model",
        "error",
      )
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string) => {
    const ok = await confirm({
      title: "Delete virtual model",
      message: `Are you sure you want to delete virtual model "${id}"? This cannot be undone.`,
      danger: true,
      confirmLabel: "Delete",
    })
    if (!ok) return
    try {
      await deleteVirtualModel(id)
      showToast("Deleted", "success")
      if (selected?.virtual_model_id === id) closeForm()
      await load()
    } catch {
      showToast("Failed to delete", "error")
    }
  }

  const showWeight = form.lb_strategy === "weighted"
  const showPriority = form.lb_strategy === "priority"
  const isEditing = isNew || selected !== null

  const providerNameById = Object.fromEntries(
    providers.map((provider) => [provider.id, provider.name || provider.id]),
  )
  const upstreamResolutionContext = {
    providers,
    providerModels,
    providerNameById,
  }

  return (
    <div
      style={{ padding: 24, display: "flex", flexDirection: "column", gap: 24 }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "flex-start",
          justifyContent: "space-between",
          gap: 16,
          flexWrap: "wrap",
        }}
      >
        <div>
          <h1
            style={{
              margin: 0,
              fontFamily: "var(--font-display)",
              fontSize: 28,
              fontWeight: 700,
              color: "var(--color-text)",
              letterSpacing: "-0.03em",
            }}
          >
            Virtual Models
          </h1>
          <p
            style={{
              margin: "8px 0 0",
              color: "var(--color-text-secondary)",
              fontSize: 14,
              lineHeight: 1.5,
              maxWidth: 720,
            }}
          >
            Create stable model aliases and route requests across multiple
            upstream providers.
          </p>
        </div>
        <button className="btn btn-primary" onClick={openNew}>
          + New Virtual Model
        </button>
      </div>

      <div className={`vm-editor-grid${isEditing ? " editing" : ""}`}>
        <section className="panel" style={{ padding: 20 }}>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              marginBottom: 16,
              gap: 12,
            }}
          >
            <div>
              <div
                style={{
                  fontSize: 16,
                  fontWeight: 600,
                  color: "var(--color-text)",
                }}
              >
                Configured models
              </div>
              <div
                style={{
                  fontSize: 12,
                  color: "var(--color-text-tertiary)",
                  marginTop: 4,
                }}
              >
                {virtualmodels.length} virtual model
                {virtualmodels.length !== 1 ? "s" : ""}
              </div>
            </div>
          </div>

          {loading ?
            <div style={emptyStateStyle}>Loading virtual models…</div>
          : virtualmodels.length === 0 ?
            <div style={emptyStateStyle}>
              No virtual models yet. Click <strong>+ New Virtual Model</strong>{" "}
              to create one.
            </div>
          : <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
              {virtualmodels.map((vm) => {
                const isSelected =
                  selected?.virtual_model_id === vm.virtual_model_id
                const virtualFamily = detectModelFamily(vm.virtual_model_id)
                const primaryUpstreamFamily =
                  vm.upstreams.length > 0 ?
                    detectModelFamily(vm.upstreams[0].model_id)
                  : null
                const hasFamilyMismatch = Boolean(
                  virtualFamily
                    && primaryUpstreamFamily
                    && virtualFamily !== primaryUpstreamFamily,
                )
                const upstreamSummary = formatVirtualModelUpstreamSummary(
                  vm,
                  upstreamResolutionContext,
                )
                const showVmWeight = vm.lb_strategy === "weighted"
                const showVmPriority = vm.lb_strategy === "priority"
                return (
                  <div
                    key={vm.virtual_model_id}
                    onClick={() => openEdit(vm)}
                    className={`panel${isSelected ? " panel-active" : ""}`}
                    title={upstreamSummary || undefined}
                    style={{
                      textAlign: "left",
                      padding: 16,
                      background:
                        isSelected ?
                          "var(--color-blue-fill)"
                        : "var(--color-bg-elevated)",
                      cursor: "pointer",
                      transition: "all 0.15s var(--ease)",
                    }}
                  >
                    <div
                      style={{
                        display: "flex",
                        alignItems: "flex-start",
                        justifyContent: "space-between",
                        gap: 12,
                      }}
                    >
                      <div style={{ minWidth: 0, flex: 1 }}>
                        <div
                          style={{
                            fontFamily: "var(--font-mono)",
                            fontSize: 13,
                            fontWeight: 600,
                            color: "var(--color-text)",
                            marginBottom: 6,
                          }}
                        >
                          {vm.virtual_model_id}
                        </div>
                        <div
                          style={{
                            fontSize: 13,
                            color: "var(--color-text-secondary)",
                            marginBottom: 8,
                          }}
                        >
                          {vm.name}
                        </div>
                        <div
                          style={{
                            display: "flex",
                            flexWrap: "wrap",
                            gap: 6,
                            fontSize: 10,
                            color: "var(--color-text-tertiary)",
                            marginBottom: 8,
                          }}
                        >
                          <Badge>{vm.lb_strategy}</Badge>
                          <Badge>{vm.api_shape}</Badge>
                          <Badge>
                            {vm.upstreams.length} upstream
                            {vm.upstreams.length !== 1 ? "s" : ""}
                          </Badge>
                          <Badge tone={vm.enabled ? "green" : "neutral"}>
                            {vm.enabled ? "enabled" : "disabled"}
                          </Badge>
                          {hasFamilyMismatch && (
                            <Badge tone="amber">family mismatch</Badge>
                          )}
                        </div>
                        {hasFamilyMismatch && (
                          <div style={warningBoxStyle}>
                            Virtual model looks like{" "}
                            <strong>{virtualFamily}</strong> but primary
                            upstream routes to{" "}
                            <strong>{primaryUpstreamFamily}</strong>.
                          </div>
                        )}
                        <div
                          style={{
                            display: "flex",
                            flexDirection: "column",
                            gap: 4,
                            fontSize: 11,
                            color: "var(--color-text-secondary)",
                            marginBottom: 8,
                          }}
                        >
                          {vm.upstreams.map((upstream, index) => {
                            const { providerLabel, isLegacy } =
                              resolveUpstreamProvider(
                                upstream,
                                upstreamResolutionContext,
                              )
                            return (
                              <div
                                key={`${vm.virtual_model_id}-${index}`}
                                style={{ display: "flex", gap: 6, minWidth: 0 }}
                              >
                                <span
                                  style={{
                                    color: "var(--color-text-tertiary)",
                                    flexShrink: 0,
                                  }}
                                >
                                  {index + 1}.
                                </span>
                                <span
                                  style={{
                                    minWidth: 0,
                                    overflow: "hidden",
                                    textOverflow: "ellipsis",
                                    whiteSpace: "nowrap",
                                  }}
                                  title={`${providerLabel} · ${upstream.model_id}${showVmWeight ? ` · weight ${upstream.weight ?? 1}` : ""}${showVmPriority ? ` · priority ${upstream.priority ?? 0}` : ""}`}
                                >
                                  {providerLabel}
                                  {" · "}
                                  <span
                                    style={{ fontFamily: "var(--font-mono)" }}
                                  >
                                    {upstream.model_id}
                                  </span>
                                  {showVmWeight && (
                                    <span
                                      style={{
                                        color: "var(--color-text-tertiary)",
                                      }}
                                    >{` · w${upstream.weight ?? 1}`}</span>
                                  )}
                                  {showVmPriority && (
                                    <span
                                      style={{
                                        color: "var(--color-text-tertiary)",
                                      }}
                                    >{` · p${upstream.priority ?? 0}`}</span>
                                  )}
                                  {isLegacy && (
                                    <span style={{ marginLeft: 6 }}>
                                      <Badge tone="amber">legacy</Badge>
                                    </span>
                                  )}
                                </span>
                              </div>
                            )
                          })}
                        </div>
                        <div
                          style={{
                            display: "grid",
                            gridTemplateColumns:
                              "repeat(auto-fit, minmax(120px, 1fr))",
                            gap: 6,
                          }}
                        >
                          <MetaItem
                            label="Updated"
                            value={formatTimestamp(vm.updated_at)}
                            compact
                          />
                          <MetaItem
                            label="Description"
                            value={vm.description || "—"}
                            compact
                          />
                        </div>
                      </div>
                      <button
                        type="button"
                        className="btn btn-icon btn-icon-ghost btn-icon-danger"
                        title={`Delete ${vm.virtual_model_id}`}
                        onClick={(e) => {
                          e.stopPropagation()
                          handleDelete(vm.virtual_model_id)
                        }}
                      >
                        <svg
                          width="13"
                          height="13"
                          viewBox="0 0 14 14"
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="1.8"
                          strokeLinecap="round"
                        >
                          <path d="M2 3.5h10M5.5 3.5V2.5a.5.5 0 01.5-.5h2a.5.5 0 01.5.5v1M5.5 6v4M8.5 6v4M3 3.5l.7 7a1 1 0 001 .9h4.6a1 1 0 001-.9l.7-7" />
                        </svg>
                      </button>
                    </div>
                  </div>
                )
              })}
            </div>
          }
        </section>

        {isEditing && (
          <section className="panel" style={{ padding: 20 }}>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: 12,
                marginBottom: 20,
              }}
            >
              <div>
                <div
                  style={{
                    fontSize: 18,
                    fontWeight: 600,
                    color: "var(--color-text)",
                  }}
                >
                  {isNew ?
                    "New Virtual Model"
                  : selected?.name || selected?.virtual_model_id}
                </div>
                <div
                  style={{
                    fontSize: 12,
                    color: "var(--color-text-tertiary)",
                    marginTop: 4,
                  }}
                >
                  {isNew ?
                    "Create a new routed model alias"
                  : `Editing ${selected?.virtual_model_id}`}
                </div>
              </div>
              <button className="btn btn-ghost btn-sm" onClick={closeForm}>
                Close
              </button>
            </div>

            <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
              <div style={sectionStyle}>
                <div style={sectionTitleStyle}>Basics</div>
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))",
                    gap: 10,
                  }}
                >
                  <MetaItem
                    label="Virtual model ID"
                    value={form.virtual_model_id || "—"}
                    mono
                  />
                  <MetaItem
                    label="API shape"
                    value={form.api_shape || "openai"}
                  />
                  <MetaItem
                    label="Strategy"
                    value={form.lb_strategy || "round-robin"}
                  />
                  <MetaItem
                    label="Upstreams"
                    value={String(
                      upstreamRows.filter((row) => row.model_id.trim()).length,
                    )}
                  />
                </div>
                <div
                  style={{ display: "flex", flexDirection: "column", gap: 6 }}
                >
                  <div style={sectionTitleStyle}>Current routing</div>
                  <div style={sectionHintStyle}>
                    This virtual model currently routes to these exact upstream
                    model ids.
                  </div>
                  {(() => {
                    const summaryVirtualFamily = detectModelFamily(
                      form.virtual_model_id,
                    )
                    const summaryPrimaryFamily =
                      upstreamRows[0]?.model_id ?
                        detectModelFamily(upstreamRows[0].model_id)
                      : null
                    const summaryMismatch = Boolean(
                      summaryVirtualFamily
                        && summaryPrimaryFamily
                        && summaryVirtualFamily !== summaryPrimaryFamily,
                    )
                    return summaryMismatch ?
                        <div style={warningBoxStyle}>
                          Virtual model looks like{" "}
                          <strong>{summaryVirtualFamily}</strong> but primary
                          upstream routes to{" "}
                          <strong>{summaryPrimaryFamily}</strong>.
                        </div>
                      : null
                  })()}
                  <div
                    style={{ display: "flex", flexDirection: "column", gap: 6 }}
                  >
                    {upstreamRows
                      .filter((row) => row.model_id.trim())
                      .map((row, index) => {
                        const providerLabel =
                          row.selectedProviderId ?
                            (providerNameById[row.selectedProviderId]
                            ?? row.selectedProviderId)
                          : row.provider_id ?
                            (providerNameById[row.provider_id]
                            ?? row.provider_id)
                          : "Legacy provider not resolved"
                        const routeLabel =
                          form.lb_strategy === "priority" ?
                            index === 0 ?
                              "Primary"
                            : `Fallback ${index}`
                          : form.lb_strategy === "weighted" ?
                            `Weight ${row.weight ?? 1}`
                          : form.lb_strategy === "round-robin" ?
                            `Round-robin ${index + 1}`
                          : `Random ${index + 1}`
                        return (
                          <div key={`summary-${index}`} style={routingRowStyle}>
                            <span style={routingLabelStyle}>{routeLabel}</span>
                            <span
                              style={{
                                minWidth: 0,
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                              }}
                            >
                              {providerLabel}
                              {" · "}
                              <span style={{ fontFamily: "var(--font-mono)" }}>
                                {row.model_id}
                              </span>
                              {form.lb_strategy === "priority" && (
                                <span
                                  style={{
                                    color: "var(--color-text-tertiary)",
                                  }}
                                >{` · p${row.priority ?? 0}`}</span>
                              )}
                              {form.lb_strategy === "weighted" && (
                                <span
                                  style={{
                                    color: "var(--color-text-tertiary)",
                                  }}
                                >{` · w${row.weight ?? 1}`}</span>
                              )}
                            </span>
                          </div>
                        )
                      })}
                  </div>
                </div>
                <div style={formGridStyle}>
                  <Field
                    label="Model ID"
                    hint="Unique identifier exposed via /v1/models"
                  >
                    <input
                      className="sys-input"
                      value={form.virtual_model_id ?? ""}
                      onChange={(e) =>
                        setForm((f) => ({
                          ...f,
                          virtual_model_id: e.target.value,
                        }))
                      }
                      disabled={false}
                      placeholder="e.g. claude-mythos-5.0"
                    />
                  </Field>

                  <Field label="Display name">
                    <input
                      className="sys-input"
                      value={form.name ?? ""}
                      onChange={(e) =>
                        setForm((f) => ({ ...f, name: e.target.value }))
                      }
                      placeholder="My Virtual Model"
                    />
                  </Field>

                  <Field label="Description" hint="Optional">
                    <input
                      className="sys-input"
                      value={form.description ?? ""}
                      onChange={(e) =>
                        setForm((f) => ({ ...f, description: e.target.value }))
                      }
                      placeholder="What this virtual model does"
                    />
                  </Field>

                  <Field label="Load-balancing strategy">
                    <select
                      className="sys-select"
                      value={form.lb_strategy ?? "round-robin"}
                      onChange={(e) =>
                        setForm((f) => ({
                          ...f,
                          lb_strategy: e.target.value as LbStrategy,
                        }))
                      }
                    >
                      {LB_STRATEGIES.map((s) => (
                        <option key={s.value} value={s.value}>
                          {s.label} — {s.hint}
                        </option>
                      ))}
                    </select>
                  </Field>
                </div>

                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <label className="sys-label" style={{ marginBottom: 0 }}>
                    Enabled
                  </label>
                  <input
                    type="checkbox"
                    checked={form.enabled ?? true}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, enabled: e.target.checked }))
                    }
                    style={{ width: 16, height: 16, cursor: "pointer" }}
                  />
                </div>
              </div>

              <div style={sectionStyle}>
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: 12,
                    marginBottom: 14,
                  }}
                >
                  <div>
                    <div style={sectionTitleStyle}>Upstream models</div>
                    <div style={sectionHintStyle}>
                      Choose provider/model pairs that this virtual model can
                      route to.
                    </div>
                  </div>
                  <button className="btn btn-ghost btn-sm" onClick={addRow}>
                    + Add upstream
                  </button>
                </div>

                {catalogueLoading && (
                  <div
                    style={{
                      ...emptyStateStyle,
                      padding: 12,
                      marginBottom: 12,
                    }}
                  >
                    Loading providers…
                  </div>
                )}

                <div
                  style={{ display: "flex", flexDirection: "column", gap: 12 }}
                >
                  {upstreamRows.map((row, i) => {
                    const availableModels =
                      row.selectedProviderId ?
                        (providerModels[row.selectedProviderId] ?? [])
                      : []

                    return (
                      <div key={i} style={upstreamCardStyle}>
                        <div
                          style={upstreamGridStyle(showWeight, showPriority)}
                        >
                          <Field label="Provider">
                            <select
                              className="sys-select"
                              value={row.selectedProviderId}
                              onChange={(e) =>
                                setRowProvider(i, e.target.value)
                              }
                            >
                              <option value="">— provider —</option>
                              {providers.map((p) => (
                                <option key={p.id} value={p.id}>
                                  {p.name || p.id}
                                </option>
                              ))}
                            </select>
                          </Field>

                          <Field label="Model">
                            <select
                              className="sys-select"
                              value={row.model_id}
                              onChange={(e) => setRowModel(i, e.target.value)}
                              disabled={!row.selectedProviderId}
                            >
                              <option value="">— model —</option>
                              {availableModels.map((m) => (
                                <option key={m.id} value={m.id}>
                                  {m.name || m.id}
                                </option>
                              ))}
                            </select>
                          </Field>

                          {showWeight && (
                            <Field label="Weight">
                              <input
                                className="sys-input"
                                type="number"
                                min={1}
                                value={row.weight}
                                onChange={(e) =>
                                  setRowNum(i, "weight", Number(e.target.value))
                                }
                              />
                            </Field>
                          )}

                          {showPriority && (
                            <Field label="Priority">
                              <input
                                className="sys-input"
                                type="number"
                                min={0}
                                value={row.priority}
                                onChange={(e) =>
                                  setRowNum(
                                    i,
                                    "priority",
                                    Number(e.target.value),
                                  )
                                }
                              />
                            </Field>
                          )}
                        </div>

                        <div
                          style={{
                            display: "flex",
                            justifyContent: "flex-end",
                            marginTop: 12,
                          }}
                        >
                          <button
                            className="btn btn-ghost btn-sm"
                            onClick={() => removeRow(i)}
                            disabled={upstreamRows.length === 1}
                          >
                            Remove
                          </button>
                        </div>
                      </div>
                    )
                  })}
                </div>

                {(showWeight || showPriority) && (
                  <div style={sectionHintStyle}>
                    {showWeight && "Weight: higher value = more traffic. "}
                    {showPriority
                      && "Priority: lower number = higher priority (0 = first choice)."}
                  </div>
                )}
              </div>

              <div
                style={{
                  display: "flex",
                  gap: 10,
                  justifyContent: "flex-end",
                  flexWrap: "wrap",
                }}
              >
                {!isNew && selected && (
                  <button
                    className="btn btn-ghost"
                    onClick={() => handleDelete(selected.virtual_model_id)}
                  >
                    Delete
                  </button>
                )}
                <button className="btn btn-ghost" onClick={closeForm}>
                  Cancel
                </button>
                <button
                  className="btn btn-primary"
                  onClick={handleSave}
                  disabled={saving}
                >
                  {saving ?
                    "Saving…"
                  : isNew ?
                    "Create"
                  : "Save"}
                </button>
              </div>
            </div>
          </section>
        )}
      </div>
    </div>
  )
}

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <div>
      <label className="sys-label">
        {label}
        {hint && (
          <span
            style={{
              fontWeight: 400,
              marginLeft: 6,
              color: "var(--color-text-tertiary)",
            }}
          >
            — {hint}
          </span>
        )}
      </label>
      {children}
    </div>
  )
}

function Badge({
  children,
  tone = "neutral",
}: {
  children: React.ReactNode
  tone?: "neutral" | "green" | "amber"
}) {
  return (
    <span
      style={{
        fontSize: 11,
        padding: "2px 8px",
        borderRadius: 999,
        fontWeight: 600,
        background:
          tone === "green" ? "rgba(48,209,88,0.12)"
          : tone === "amber" ? "rgba(255,159,10,0.12)"
          : "rgba(255,255,255,0.06)",
        color:
          tone === "green" ? "var(--color-green)"
          : tone === "amber" ? "var(--color-orange)"
          : "var(--color-text-tertiary)",
        border:
          tone === "green" ? "1px solid rgba(48,209,88,0.2)"
          : tone === "amber" ? "1px solid rgba(255,159,10,0.2)"
          : "1px solid var(--color-separator)",
      }}
    >
      {children}
    </span>
  )
}

function MetaItem({
  label,
  value,
  mono = false,
  compact = false,
}: {
  label: string
  value: string
  mono?: boolean
  compact?: boolean
}) {
  return (
    <div
      style={{
        padding: compact ? "8px 10px" : "10px 12px",
        borderRadius: "var(--radius-md)",
        border: "1px solid var(--color-separator)",
        background: "rgba(255,255,255,0.03)",
      }}
    >
      <div
        style={{
          fontSize: compact ? 10 : 11,
          color: "var(--color-text-tertiary)",
          marginBottom: 4,
        }}
      >
        {label}
      </div>
      <div
        style={{
          fontSize: compact ? 11 : 13,
          color: "var(--color-text)",
          fontFamily: mono ? "var(--font-mono)" : "var(--font-text)",
          wordBreak: "break-word",
          lineHeight: 1.35,
        }}
      >
        {value}
      </div>
    </div>
  )
}

function formatTimestamp(value?: string) {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

const warningBoxStyle: React.CSSProperties = {
  padding: "8px 10px",
  borderRadius: "var(--radius-md)",
  border: "1px solid rgba(255,159,10,0.2)",
  background: "rgba(255,159,10,0.08)",
  color: "var(--color-orange)",
  fontSize: 11,
  lineHeight: 1.4,
  marginBottom: 8,
}

const routingRowStyle: React.CSSProperties = {
  display: "flex",
  gap: 8,
  minWidth: 0,
  padding: "8px 10px",
  borderRadius: "var(--radius-md)",
  border: "1px solid var(--color-separator)",
  background: "rgba(255,255,255,0.03)",
  fontSize: 12,
  color: "var(--color-text-secondary)",
}

const routingLabelStyle: React.CSSProperties = {
  color: "var(--color-text-tertiary)",
  flexShrink: 0,
  width: 84,
  fontSize: 11,
}

const emptyStateStyle: React.CSSProperties = {
  padding: 24,
  borderRadius: "var(--radius-lg)",
  border: "1px dashed var(--color-separator)",
  textAlign: "center",
  color: "var(--color-text-tertiary)",
  fontSize: 13,
}

const sectionStyle: React.CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 14,
  padding: 16,
  borderRadius: "var(--radius-lg)",
  border: "1px solid var(--color-separator)",
  background: "rgba(255,255,255,0.03)",
}

const sectionTitleStyle: React.CSSProperties = {
  fontSize: 14,
  fontWeight: 600,
  color: "var(--color-text)",
}

const sectionHintStyle: React.CSSProperties = {
  fontSize: 12,
  color: "var(--color-text-tertiary)",
  marginTop: 4,
}

const formGridStyle: React.CSSProperties = {
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))",
  gap: 14,
}

const upstreamCardStyle: React.CSSProperties = {
  padding: 14,
  borderRadius: "var(--radius-lg)",
  border: "1px solid var(--color-separator)",
  background: "var(--color-bg-elevated)",
}

function upstreamGridStyle(
  showWeight: boolean,
  showPriority: boolean,
): React.CSSProperties {
  return {
    display: "grid",
    gridTemplateColumns: `minmax(180px, 1fr) minmax(220px, 1.2fr)${showWeight ? " 110px" : ""}${showPriority ? " 110px" : ""}`,
    gap: 12,
    alignItems: "start",
  }
}
