import { describe, expect, test } from "bun:test"

import type { Model, Provider, VirtualModel } from "../../../frontend/src/api"

import { formatVirtualModelUpstreamSummary } from "../../../frontend/src/lib/virtualmodels"

const providers: Array<Provider> = [
  {
    id: "provider-a",
    type: "openai",
    name: "Provider A",
    isActive: true,
    authStatus: "authenticated",
  },
  {
    id: "provider-b",
    type: "anthropic",
    name: "Provider B",
    isActive: true,
    authStatus: "authenticated",
  },
  {
    id: "provider-c",
    type: "openai",
    name: "Provider C",
    isActive: true,
    authStatus: "authenticated",
  },
]

const providerModels: Record<string, Array<Model>> = {
  "provider-a": [
    { id: "gpt-4.1", name: "GPT-4.1", enabled: true },
    { id: "shared-model", name: "Shared Model", enabled: true },
  ],
  "provider-b": [
    { id: "claude-3.7", name: "Claude 3.7", enabled: true },
    { id: "shared-model", name: "Shared Model", enabled: true },
  ],
  "provider-c": [{ id: "shared-model", name: "Shared Model", enabled: true }],
}

const providerNameById: Record<string, string> = Object.fromEntries(
  providers.map((provider) => [provider.id, provider.name]),
)
const upstreamResolutionContext = {
  providers,
  providerModels,
  providerNameById,
}

describe("formatVirtualModelUpstreamSummary", () => {
  test("formats weighted upstreams without throwing", () => {
    const vm: VirtualModel = {
      virtual_model_id: "gpt-router",
      name: "GPT Router",
      description: "",
      api_shape: "openai",
      lb_strategy: "weighted",
      enabled: true,
      upstreams: [
        {
          provider_id: "provider-a",
          model_id: "gpt-4.1",
          weight: 4,
          priority: 0,
        },
        {
          provider_id: "provider-b",
          model_id: "claude-3.7",
          weight: 1,
          priority: 1,
        },
      ],
    }

    const summary = formatVirtualModelUpstreamSummary(
      vm,
      upstreamResolutionContext,
    )

    expect(summary).toBe(
      "weight 4: Provider A · gpt-4.1\nweight 1: Provider B · claude-3.7",
    )
  })

  test("marks legacy upstreams as ambiguous when multiple providers expose the same model", () => {
    const vm: VirtualModel = {
      virtual_model_id: "legacy-router",
      name: "Legacy Router",
      description: "",
      api_shape: "openai",
      lb_strategy: "random",
      enabled: true,
      upstreams: [
        {
          model_id: "shared-model",
          weight: 1,
          priority: 0,
        },
      ],
    }

    const summary = formatVirtualModelUpstreamSummary(
      vm,
      upstreamResolutionContext,
    )

    expect(summary).toBe("random 1: Ambiguous legacy provider · shared-model")
  })
})
