import { describe, expect, test } from "bun:test"

import {
  detectModelFamily,
  formatVirtualModelUpstreamSummary,
  resolveUpstreamProvider,
} from "../../frontend/src/lib/virtualmodels"

describe("virtualmodels helpers", () => {
  test("detectModelFamily recognizes common model families", () => {
    expect(detectModelFamily("claude-sonnet-4.5")).toBe("Claude")
    expect(detectModelFamily("gpt-4o")).toBe("OpenAI")
    expect(detectModelFamily("o3-mini")).toBe("OpenAI")
    expect(detectModelFamily("qwen3.6-plus")).toBe("Qwen")
    expect(detectModelFamily("gemini-2.5-pro")).toBe("Gemini")
    expect(detectModelFamily("mystery-model")).toBeNull()
  })

  test("resolveUpstreamProvider infers a single matching provider", () => {
    const result = resolveUpstreamProvider(
      { model_id: "gpt-4o", provider_id: undefined },
      {
        providers: [
          {
            id: "p1",
            type: "openai",
            name: "OpenAI",
            isActive: true,
            authStatus: "authenticated",
          },
        ],
        providerModels: {
          p1: [{ id: "gpt-4o", name: "GPT-4o", enabled: true }],
        },
        providerNameById: { p1: "OpenAI" },
      },
    )

    expect(result.providerLabel).toBe("OpenAI")
    expect(result.isLegacy).toBe(true)
  })

  test("resolveUpstreamProvider reports ambiguous legacy provider", () => {
    const result = resolveUpstreamProvider(
      { model_id: "shared-model", provider_id: undefined },
      {
        providers: [
          {
            id: "p1",
            type: "a",
            name: "A",
            isActive: true,
            authStatus: "authenticated",
          },
          {
            id: "p2",
            type: "b",
            name: "B",
            isActive: true,
            authStatus: "authenticated",
          },
        ],
        providerModels: {
          p1: [{ id: "shared-model", name: "Shared", enabled: true }],
          p2: [{ id: "shared-model", name: "Shared", enabled: true }],
        },
        providerNameById: { p1: "A", p2: "B" },
      },
    )

    expect(result.providerLabel).toBe("Ambiguous legacy provider")
    expect(result.isLegacy).toBe(true)
  })

  test("formatVirtualModelUpstreamSummary formats strategy-specific labels", () => {
    const summary = formatVirtualModelUpstreamSummary(
      {
        lb_strategy: "priority",
        upstreams: [
          { model_id: "gpt-4o", provider_id: "p1", weight: 1, priority: 0 },
          { model_id: "gpt-4.1", provider_id: "p2", weight: 1, priority: 1 },
        ],
      },
      {
        providers: [
          {
            id: "p1",
            type: "openai",
            name: "OpenAI A",
            isActive: true,
            authStatus: "authenticated",
          },
          {
            id: "p2",
            type: "openai",
            name: "OpenAI B",
            isActive: true,
            authStatus: "authenticated",
          },
        ],
        providerModels: {},
        providerNameById: { p1: "OpenAI A", p2: "OpenAI B" },
      },
    )

    expect(summary).toContain("primary: OpenAI A · gpt-4o")
    expect(summary).toContain("fallback 1: OpenAI B · gpt-4.1")
  })
})
