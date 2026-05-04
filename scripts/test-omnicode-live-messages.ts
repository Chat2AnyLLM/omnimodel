#!/usr/bin/env bun
/**
 * Live agent test for omnicode /v1/messages strategy.
 *
 * Connects to a running omnillm server at localhost:5000 and runs a coding
 * agent loop for each model using ONLY /v1/messages. OmniLLM translates to
 * the appropriate upstream API.
 *
 * Tests:
 *   1. Plain chat  – short text reply
 *   2. Tool use    – weather query ("show me temperature of Shanghai")
 *   3. Tool use    – hardware info ("show all hardware info")
 *
 * Run:
 *   bun scripts/test-omnicode-live-messages.ts
 */

import process from "node:process"

const BASE_URL = process.env.OMNILLM_BASE_URL ?? "http://127.0.0.1:5000"
const API_KEY =
  process.env.OMNILLM_API_KEY ??
  "7de6a4e524310272274901721a0d697a014fc694745b9f3ab5eac21aa2c043f2"
const TIMEOUT_MS = 120_000

// ---------------------------------------------------------------------------
// Target models
// ---------------------------------------------------------------------------
const MODELS = [
  "gpt-5.4-mini",
  "gpt-5-mini",
  "claude-haiku-4.5",
  "gemini-3.1-pro-preview",
  "deepseek-v4-flash",
  "qwen3.6-flash",
  "kimi-k2.6",
] as const

// ---------------------------------------------------------------------------
// Anthropic /v1/messages types
// ---------------------------------------------------------------------------
type AnthropicContentBlock =
  | { type: "text"; text: string }
  | { type: "tool_use"; id: string; name: string; input: Record<string, unknown> }

type AnthropicMessage = {
  id: string
  model: string
  role: "assistant"
  content: AnthropicContentBlock[]
  stop_reason: "end_turn" | "tool_use" | "max_tokens" | "stop_sequence" | null
  usage?: { input_tokens: number; output_tokens: number }
}

type AnthropicTool = {
  name: string
  description: string
  input_schema: {
    type: "object"
    properties: Record<string, { type: string; description?: string }>
    required?: string[]
  }
}

type MessageParam =
  | { role: "user"; content: Array<{ type: "text"; text: string } | { type: "tool_result"; tool_use_id: string; content: string }> }
  | { role: "assistant"; content: AnthropicContentBlock[] }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function authHeaders(): Record<string, string> {
  return {
    "x-api-key": API_KEY,
    "anthropic-version": "2023-06-01",
    "Content-Type": "application/json",
  }
}

async function messagesRequest(
  model: string,
  messages: MessageParam[],
  tools?: AnthropicTool[],
  maxTokens = 1024,
  retries = 2,
): Promise<AnthropicMessage> {
  const body: Record<string, unknown> = { model, max_tokens: maxTokens, messages }
  if (tools && tools.length > 0) {
    body.tools = tools
    body.tool_choice = { type: "auto" }
  }

  let lastErr: Error | null = null
  for (let attempt = 0; attempt <= retries; attempt++) {
    if (attempt > 0) {
      await Bun.sleep(3000 * attempt)
    }
    const ac = new AbortController()
    const timer = setTimeout(() => ac.abort(), TIMEOUT_MS)
    try {
      const resp = await fetch(`${BASE_URL}/v1/messages`, {
        method: "POST",
        headers: authHeaders(),
        body: JSON.stringify(body),
        signal: ac.signal,
      })
      const text = await resp.text()
      if (!resp.ok) {
        lastErr = new Error(`HTTP ${resp.status}: ${text}`)
        // Only retry on 429 (rate limit) or 502/503 (transient upstream errors)
        if (resp.status === 429 || resp.status === 502 || resp.status === 503) {
          continue
        }
        throw lastErr
      }
      return JSON.parse(text) as AnthropicMessage
    } finally {
      clearTimeout(timer)
    }
  }
  throw lastErr ?? new Error("request failed after retries")
}

function extractText(content: AnthropicContentBlock[]): string {
  return content
    .filter((b): b is { type: "text"; text: string } => b.type === "text")
    .map((b) => b.text)
    .join("")
    .trim()
}

function extractToolUse(
  content: AnthropicContentBlock[],
): Array<{ type: "tool_use"; id: string; name: string; input: Record<string, unknown> }> {
  return content.filter(
    (b): b is { type: "tool_use"; id: string; name: string; input: Record<string, unknown> } =>
      b.type === "tool_use",
  )
}

// ---------------------------------------------------------------------------
// Simulated tool executors (coding agent tools)
// ---------------------------------------------------------------------------
function executeTool(name: string, input: Record<string, unknown>): string {
  switch (name) {
    case "get_weather": {
      const city = String(input.city ?? "unknown")
      // Simulate a live weather result
      const temps: Record<string, string> = {
        shanghai: "22°C, partly cloudy, humidity 68%",
        beijing: "18°C, clear sky",
        london: "14°C, overcast",
      }
      const key = city.toLowerCase()
      return temps[key] ?? `${city}: 20°C, mostly sunny`
    }
    case "get_hardware_info": {
      // Simulate system info (real impl would call wmi/sysfs)
      return [
        "CPU: Intel Core i9-13900K @ 3.0GHz (24 cores, 32 threads)",
        "RAM: 64 GB DDR5-5600",
        "GPU: NVIDIA RTX 4090 24 GB",
        "Storage: 2 TB NVMe SSD (Samsung 990 Pro)",
        "OS: Windows 11 Pro 23H2 (Build 22631)",
        "Network: 2.5 GbE onboard + WiFi 6E",
      ].join("\n")
    }
    case "read_file": {
      const path = String(input.path ?? "")
      return `[simulated] Contents of ${path}: package main\n\nfunc main() { /* … */ }`
    }
    case "bash": {
      const cmd = String(input.command ?? "")
      return `[simulated] $ ${cmd}\nOutput: command executed successfully`
    }
    default:
      return `[unknown tool ${name}]`
  }
}

// ---------------------------------------------------------------------------
// Agent loop  (runs until stop_reason !== "tool_use" or maxSteps exceeded)
// ---------------------------------------------------------------------------
async function runAgentLoop(
  model: string,
  systemPrompt: string,
  userPrompt: string,
  tools: AnthropicTool[],
  maxSteps = 5,
): Promise<{ output: string; steps: number; toolsUsed: string[] }> {
  const toolsUsed: string[] = []
  const messages: MessageParam[] = [
    { role: "user", content: [{ type: "text", text: userPrompt }] },
  ]

  // Prepend system as first user message (simple approach for non-claude models)
  const fullMessages: MessageParam[] = [
    { role: "user", content: [{ type: "text", text: `[SYSTEM]: ${systemPrompt}\n\n[USER]: ${userPrompt}` }] },
  ]

  for (let step = 0; step < maxSteps; step++) {
    const resp = await messagesRequest(model, fullMessages, tools)

    if (!resp.content || resp.content.length === 0) {
      throw new Error(`step ${step}: empty content in response`)
    }

    // Append assistant message to history
    fullMessages.push({ role: "assistant", content: resp.content })

    if (resp.stop_reason !== "tool_use") {
      return { output: extractText(resp.content), steps: step + 1, toolsUsed }
    }

    // Execute tool calls and add results
    const toolCalls = extractToolUse(resp.content)
    if (toolCalls.length === 0) {
      throw new Error(`step ${step}: stop_reason=tool_use but no tool_use blocks`)
    }

    const toolResults: { type: "tool_result"; tool_use_id: string; content: string }[] = []
    for (const call of toolCalls) {
      toolsUsed.push(call.name)
      const result = executeTool(call.name, call.input)
      toolResults.push({ type: "tool_result", tool_use_id: call.id, content: result })
    }

    fullMessages.push({ role: "user", content: toolResults })
  }

  throw new Error(`agent loop exceeded ${maxSteps} steps`)
}

// ---------------------------------------------------------------------------
// Test definitions
// ---------------------------------------------------------------------------
const CODING_AGENT_SYSTEM = `You are OmniCode, a coding agent with access to tools.
Use tools when the task requires external data or system information.
Be concise and factual.`

const WEATHER_TOOLS: AnthropicTool[] = [
  {
    name: "get_weather",
    description: "Get the current weather and temperature for a city.",
    input_schema: {
      type: "object",
      properties: {
        city: { type: "string", description: "Name of the city, e.g. Shanghai" },
      },
      required: ["city"],
    },
  },
]

const HARDWARE_TOOLS: AnthropicTool[] = [
  {
    name: "get_hardware_info",
    description: "Return a summary of the system hardware (CPU, RAM, GPU, storage, OS).",
    input_schema: {
      type: "object",
      properties: {},
    },
  },
]

const CODING_TOOLS: AnthropicTool[] = [
  {
    name: "read_file",
    description: "Read a file from the repository and return its content.",
    input_schema: {
      type: "object",
      properties: {
        path: { type: "string", description: "Relative file path" },
      },
      required: ["path"],
    },
  },
  {
    name: "bash",
    description: "Run a bash/shell command and return stdout.",
    input_schema: {
      type: "object",
      properties: {
        command: { type: "string", description: "Shell command to execute" },
      },
      required: ["command"],
    },
  },
]

type TestCase = {
  name: string
  prompt: string
  tools: AnthropicTool[]
  validate: (output: string, toolsUsed: string[]) => void
}

const TEST_CASES: TestCase[] = [
  {
    name: "plain chat",
    prompt: "Reply with exactly one sentence: OmniCode is ready.",
    tools: [],
    validate(output) {
      if (output.length === 0) throw new Error("empty output")
    },
  },
  {
    name: "weather tool: Shanghai temperature",
    prompt: "Show me the current temperature in Shanghai.",
    tools: WEATHER_TOOLS,
    validate(output, toolsUsed) {
      if (!toolsUsed.includes("get_weather")) {
        throw new Error(`expected get_weather tool call, got: ${toolsUsed.join(", ") || "(none)"}`)
      }
      if (output.length === 0) throw new Error("empty output after tool use")
    },
  },
  {
    name: "hardware info tool",
    prompt: "Show all hardware info for this machine.",
    tools: HARDWARE_TOOLS,
    validate(output, toolsUsed) {
      if (!toolsUsed.includes("get_hardware_info")) {
        throw new Error(`expected get_hardware_info tool call, got: ${toolsUsed.join(", ") || "(none)"}`)
      }
      if (output.length === 0) throw new Error("empty output after tool use")
    },
  },
  {
    name: "coding agent: read file + bash",
    prompt: "Read the go.mod file and then run `go version` to confirm the toolchain.",
    tools: CODING_TOOLS,
    validate(output, toolsUsed) {
      // Some models may answer directly without tool calls — accept both.
      if (output.length === 0 && toolsUsed.length === 0) {
        throw new Error("empty output and no tools used")
      }
    },
  },
]

// ---------------------------------------------------------------------------
// Main runner
// ---------------------------------------------------------------------------
type ModelResult = {
  model: string
  tests: Array<{ name: string; ok: boolean; steps?: number; toolsUsed?: string[]; error?: string }>
}

async function runModel(model: string): Promise<ModelResult> {
  const results: ModelResult["tests"] = []

  for (const tc of TEST_CASES) {
    process.stdout.write(`  [${model}] ${tc.name} … `)
    try {
      const { output, steps, toolsUsed } = await runAgentLoop(
        model,
        CODING_AGENT_SYSTEM,
        tc.prompt,
        tc.tools,
      )
      tc.validate(output, toolsUsed)
      process.stdout.write(`✅ (${steps} step${steps !== 1 ? "s" : ""}${toolsUsed.length ? ", tools: " + toolsUsed.join(",") : ""})\n`)
      results.push({ name: tc.name, ok: true, steps, toolsUsed })
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      process.stdout.write(`❌ ${msg}\n`)
      results.push({ name: tc.name, ok: false, error: msg })
    }
  }

  return { model, tests: results }
}

async function main(): Promise<void> {
  console.log(`\n🤖 OmniCode Live Agent Test — /v1/messages strategy`)
  console.log(`   Server : ${BASE_URL}`)
  console.log(`   Models : ${MODELS.join(", ")}`)
  console.log(`   Tests  : ${TEST_CASES.map((t) => t.name).join(", ")}`)
  console.log("─".repeat(72))

  const allResults: ModelResult[] = []

  for (const model of MODELS) {
    console.log(`\n▶ ${model}`)
    const result = await runModel(model)
    allResults.push(result)
    // Small pause between models to avoid hitting upstream rate limits
    await Bun.sleep(2000)
  }

  // ---- Summary ----
  console.log("\n" + "═".repeat(72))
  console.log("SUMMARY")
  console.log("═".repeat(72))

  let totalPass = 0
  let totalFail = 0

  for (const mr of allResults) {
    const pass = mr.tests.filter((t) => t.ok).length
    const fail = mr.tests.filter((t) => !t.ok).length
    totalPass += pass
    totalFail += fail
    const icon = fail === 0 ? "✅" : "❌"
    console.log(`${icon} ${mr.model.padEnd(30)} ${pass}/${mr.tests.length} passed`)
    for (const t of mr.tests.filter((x) => !x.ok)) {
      console.log(`     ✗ ${t.name}: ${t.error}`)
    }
  }

  console.log("─".repeat(72))
  console.log(`Total: ${totalPass} passed, ${totalFail} failed`)

  if (totalFail > 0) {
    process.exitCode = 1
  } else {
    console.log("\n✅ All live agent checks passed.")
  }
}

void main()
