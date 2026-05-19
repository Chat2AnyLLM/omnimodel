#!/usr/bin/env bun
// omni-dev.ts — Comprehensive OmniLLM Development Manager
// Manages both frontend and backend services with start/stop/status operations

import consola from "consola"
import {
  appendFileSync,
  closeSync,
  existsSync,
  openSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from "node:fs"
import { homedir } from "node:os"
import { join } from "node:path"
import { parseArgs } from "node:util"

const PID_FILE = join(process.cwd(), ".omni-dev.pid")
const LOG_FILE = join(process.cwd(), ".omni-dev.log")

interface ServicePids {
  backend?: number
  frontend?: number
  host?: string
  serverPort?: string
  frontendPort?: string
}

type ProcessInfo = {
  pid: number
  cmd: string
  executablePath?: string
}

type StructuredLogPayload = Record<string, unknown>

// eslint-disable-next-line no-control-regex -- intentional: strip ANSI escape codes
const ANSI_PATTERN = /\u001b\[[0-9;]*m/g
const STRUCTURED_FIELD_ORDER = [
  "request_id",
  "api_shape",
  "model_requested",
  "model_used",
  "model",
  "provider",
  "messages",
  "tools",
  "stream",
  "stop_reason",
  "input_tokens",
  "output_tokens",
  "method",
  "path",
  "status",
  "latency_ms",
  "url",
  "admin",
  "count",
  "verbose",
] as const
const SOURCE_COLORS: Record<string, string> = {
  backend: "36",
  frontend: "32",
}
const LEVEL_COLORS: Record<string, string> = {
  FATAL: "31",
  ERROR: "31",
  WARN: "33",
  INFO: "36",
  DEBUG: "35",
  TRACE: "90",
}

// ─── Per-request-ID colors for session tracing ─────────────────────────────

const REQUEST_COLORS = ["33", "36", "35", "93", "92", "94", "96", "95", "97"]
const requestIdColorCache = new Map<string, string>()

function getRequestColor(requestId: string): string {
  let color = requestIdColorCache.get(requestId)
  if (!color) {
    color = REQUEST_COLORS[requestIdColorCache.size % REQUEST_COLORS.length]
    requestIdColorCache.set(requestId, color)
  }
  return color
}

const { values, positionals } = parseArgs({
  args: Bun.argv.slice(2),
  allowPositionals: true,
  options: {
    "server-port": { type: "string", default: "5002" },
    "frontend-port": { type: "string", default: "5080" },
    host: { type: "string", default: "127.0.0.1" },
    help: { type: "boolean", short: "h" },
    verbose: { type: "boolean", short: "v" },
    rebuild: { type: "boolean", short: "r" },
    follow: { type: "boolean", short: "f" },
  },
})

const command = positionals[0] || "help"
const serverPort = values["server-port"]
const frontendPort = values["frontend-port"]
const host = values.host
const verbose = values["verbose"]
const rebuild = values["rebuild"]
const follow = values["follow"]

function showHelp() {
  console.log(`
🚀 OmniLLM Development Manager

USAGE:
  bun run omni-dev.ts <command> [options]

COMMANDS:
  start          Start both frontend and backend services
  stop           Stop all running services
  restart        Stop and start services
  restart --rebuild    Stop, rebuild frontend and backend, then start
  status         Show service status and ports
  logs           Show recent service logs
  help           Show this help message

OPTIONS:
  --server-port <port>    Backend server port (default: 5002)
  --frontend-port <port>  Frontend dev server port (default: 5080)
  --host <host>           Host/IP to bind and display (default: 127.0.0.1)
  --verbose, -v           Enable verbose logging
  --rebuild, -r           Stop services, rebuild both frontend and backend, then start
  --follow, -f            Follow logs in real-time (like tail -f)
  --help, -h              Show help

EXAMPLES:
  # Start with default ports (backend: 5002, frontend: 5080)
  bun run omni-dev.ts start

  # Start with custom ports
  bun run omni-dev.ts start --server-port 5000 --frontend-port 5080

  # Start on a specific host/IP
  bun run omni-dev.ts start --host 127.0.0.1

  # Rebuild and restart with custom ports
  bun run omni-dev.ts restart --rebuild --server-port 5000 --frontend-port 5080

  # Check service status
  bun run omni-dev.ts status

  # Stop all services
  bun run omni-dev.ts stop

  # View recent logs
  bun run omni-dev.ts logs

  # Follow logs in real-time
  bun run omni-dev.ts logs -f

SERVICE ENDPOINTS:
  Backend API:     http://${host}:${serverPort}
  Frontend:        http://${host}:${frontendPort}
  Admin UI:        http://${host}:${frontendPort}/admin/

FEATURES:
  • 🔥 Golang backend
  • 🌐 React frontend with Vite dev server
  • 📱 Integrated admin UI
  • 🔗 Automatic API proxying
  • 🔄 Hot reload for development
  • 📊 Real-time service monitoring
  • 🛑 Graceful service shutdown

NOTES:
  • Services run in the background and persist across terminal sessions
  • PID tracking ensures clean startup/shutdown
  • Go binary is built to ~/.local/bin for clean project structure
  • Frontend automatically proxies API calls to backend
  • All services stop gracefully with proper cleanup
`)
}

function getSavedRuntimeConfig(pids: ServicePids | null) {
  return {
    host: pids?.host || host,
    serverPort: pids?.serverPort || serverPort,
    frontendPort: pids?.frontendPort || frontendPort,
  }
}

function savePids(pids: ServicePids) {
  writeFileSync(PID_FILE, JSON.stringify(pids, null, 2))
  if (verbose) consola.info(`PIDs saved to ${PID_FILE}`)
}

function loadPids(): ServicePids | null {
  if (!existsSync(PID_FILE)) return null
  try {
    const content = readFileSync(PID_FILE, "utf8")
    return JSON.parse(content) as ServicePids
  } catch {
    return null
  }
}

function clearPids() {
  if (existsSync(PID_FILE)) {
    unlinkSync(PID_FILE)
    if (verbose) consola.info("PID file cleaned up")
  }
}

function isProcessRunning(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}

function getInstalledBackendPath(): string {
  return process.platform === "win32" ?
      `${process.env.USERPROFILE}/.local/bin/omnillm.exe`
    : `${homedir()}/.local/bin/omnillm`
}

async function waitForProcessExit(
  pid: number,
  timeoutMs = 10000,
  pollMs = 200,
): Promise<boolean> {
  const deadline = Date.now() + timeoutMs

  while (Date.now() < deadline) {
    if (!isProcessRunning(pid)) {
      return true
    }
    await Bun.sleep(pollMs)
  }

  return !isProcessRunning(pid)
}

async function listProcesses(): Promise<Array<ProcessInfo>> {
  // Cross-platform process listing with minimal dependencies
  if (process.platform === "win32") {
    const proc = Bun.spawn(
      [
        "powershell",
        "-NoProfile",
        "-Command",
        "Get-CimInstance Win32_Process | Select-Object ProcessId,CommandLine,ExecutablePath | ConvertTo-Json",
      ],
      { stdout: "pipe", stderr: "pipe" },
    )

    const output = await new Response(proc.stdout).text()
    try {
      const rows = JSON.parse(output) as Array<{
        ProcessId: number
        CommandLine?: string
        ExecutablePath?: string
      }>
      return rows
        .filter((row) => typeof row.ProcessId === "number")
        .map((row) => ({
          pid: row.ProcessId,
          cmd: row.CommandLine || "",
          executablePath: row.ExecutablePath || "",
        }))
    } catch {
      return []
    }
  }

  const proc = Bun.spawn(["ps", "-eo", "pid=,command="], {
    stdout: "pipe",
    stderr: "pipe",
  })

  const output = await new Response(proc.stdout).text()
  return output
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [pidStr, ...cmdParts] = line.split(/\s+/)
      return {
        pid: Number(pidStr),
        cmd: cmdParts.join(" "),
      }
    })
    .filter((p) => !Number.isNaN(p.pid))
}

function matchesProcess(cmd: string, keywords: Array<string>): boolean {
  const lower = cmd.toLowerCase()
  return keywords.every((keyword) => lower.includes(keyword.toLowerCase()))
}

async function findMatchingPids(): Promise<Array<number>> {
  const processes = await listProcesses()
  const installPath = getInstalledBackendPath()
    .toLowerCase()
    .replaceAll("/", "\\")

  // Keywords chosen to uniquely identify OmniLLM dev processes
  // Use path-separator-agnostic keywords (avoid / vs \ issues on Windows)
  const patterns: Array<Array<string>> = [
    ["omnillm", "start", "--port"], // Go binary
    ["vite.config.ts", "--port"], // Frontend Vite (matches forward or backslash paths)
  ]

  const matching = new Set<number>()

  for (const processInfo of processes) {
    if (processInfo.pid === process.pid) continue // Skip self

    const executablePath =
      processInfo.executablePath?.toLowerCase().replaceAll("/", "\\") || ""
    if (executablePath === installPath) {
      matching.add(processInfo.pid)
      continue
    }

    for (const keywords of patterns) {
      if (matchesProcess(processInfo.cmd, keywords)) {
        matching.add(processInfo.pid)
        break
      }
    }
  }

  return Array.from(matching)
}

async function checkPortAvailable(port: number): Promise<boolean> {
  for (const hostname of ["127.0.0.1", "::1"]) {
    try {
      await Bun.connect({
        hostname,
        port,
        socket: {
          open: (s) => s.end(),
          data: () => {},
          close: () => {},
          error: () => {},
        },
      })
      return false // Port is occupied
    } catch {
      // Try the next loopback address family.
    }
  }

  return true // Port is available
}

async function waitForPortAvailable(
  port: number,
  timeoutMs = 10000,
  pollMs = 200,
): Promise<boolean> {
  const deadline = Date.now() + timeoutMs

  while (Date.now() < deadline) {
    if (await checkPortAvailable(port)) {
      return true
    }
    await Bun.sleep(pollMs)
  }

  return checkPortAvailable(port)
}

function stripAnsi(value: string): string {
  return value.replaceAll(ANSI_PATTERN, "")
}

function normalizeSourceLabel(label: string): string {
  if (label === "go-backend" || label === "ts-backend") {
    return "backend"
  }

  return label
}

function numericValue(value: unknown): number {
  if (typeof value === "number") return value
  if (typeof value === "string") {
    const n = Number(value)
    return Number.isFinite(n) ? n : 0
  }
  return 0
}

function stringifyLogValue(value: unknown): string {
  if (typeof value === "string") {
    return value.trim()
  }

  if (typeof value === "number" || typeof value === "boolean") {
    return String(value)
  }

  if (value === null || value === undefined) {
    return ""
  }

  try {
    return JSON.stringify(value)
  } catch {
    return Object.prototype.toString.call(value)
  }
}

function formatStructuredField(
  key: string,
  value: unknown,
  requestedModel?: string,
): string | null {
  const formattedValue = stringifyLogValue(value)
  if (!formattedValue) {
    return null
  }

  switch (key) {
    case "request_id": {
      const color = getRequestColor(formattedValue)
      return `\x1b[${color}mrequest=${formattedValue}\x1b[0m`
    }
    case "api_shape": {
      return `api=${formattedValue}`
    }
    case "model_requested": {
      return `requested=${formattedValue}`
    }
    case "model_used": {
      // Omit when model_used equals model_requested (no routing occurred).
      if (formattedValue === requestedModel) {
        return null
      }
      return `used=${formattedValue}`
    }
    case "latency_ms": {
      return `latency=${formattedValue}ms`
    }
    case "input_tokens": {
      // Suppress zero token counts — they add no information.
      if (numericValue(value) === 0) {
        return null
      }
      return `input=${formattedValue}`
    }
    case "output_tokens": {
      if (numericValue(value) === 0) {
        return null
      }
      return `output=${formattedValue}`
    }
    case "message": {
      // Colorize incoming/outgoing request arrows
      if (
        formattedValue.startsWith("--> ")
        || formattedValue.startsWith("<-- ")
      ) {
        return colorizeMessageArrows(formattedValue)
      }
      return formattedValue
    }
    case "level":
    case "time": {
      return null
    }
    default: {
      return `${key}=${formattedValue}`
    }
  }
}

function colorizeMessageArrows(text: string): string {
  // Match patterns like:
  //   --> POST /v1/chat/completions <-- 200
  //   --> REQUEST
  //   <-- RESPONSE
  const arrowPattern = /^(?:-->|<--).*$/

  if (arrowPattern.test(text)) {
    return text
      .replaceAll(/(-->)/g, "\x1b[33m$1\x1b[0m") // yellow outgoing
      .replaceAll(/(<--)/g, "\x1b[32m$1\x1b[0m") // green incoming
  }

  // For messages like:
  //   --> GET /api/admin/info <-- 200
  const fullPattern = /-->|<--/g
  return text.replaceAll(fullPattern, (match) => {
    return match === "-->" ? "\x1b[33m-->\x1b[0m" : "\x1b[32m<--\x1b[0m"
  })
}

function formatStructuredLogLine(
  source: string,
  payload: StructuredLogPayload,
  fallbackTimestamp: string,
  fallbackLevel: string,
): string {
  const timestamp =
    typeof payload.time === "string" && payload.time.trim() ?
      payload.time.trim()
    : fallbackTimestamp
  const level =
    typeof payload.level === "string" && payload.level.trim() ?
      payload.level.trim().toUpperCase()
    : fallbackLevel
  const message =
    typeof payload.message === "string" && payload.message.trim() ?
      colorizeMessageArrows(payload.message.trim())
    : JSON.stringify(payload)

  const segments = [`[${timestamp}]`, source, level, message]
  const seen = new Set<string>(["message"])

  const requestedModel =
    typeof payload.model_requested === "string" ?
      payload.model_requested
    : undefined

  for (const key of STRUCTURED_FIELD_ORDER) {
    const formatted = formatStructuredField(key, payload[key], requestedModel)
    if (formatted) {
      segments.push(formatted)
      seen.add(key)
    }
  }

  const remaining: Array<string> = []
  for (const [key, value] of Object.entries(payload)) {
    if (seen.has(key)) {
      continue
    }

    const formatted = formatStructuredField(key, value, requestedModel)
    if (formatted) {
      remaining.push(formatted)
    }
  }

  remaining.sort((left, right) => left.localeCompare(right))
  return segments.concat(remaining).join(" | ")
}

function inferTextLogLevel(line: string, isError: boolean): string {
  const lower = line.toLowerCase()

  if (isError || lower.includes("error") || lower.includes("failed")) {
    return "ERROR"
  }

  if (lower.includes("warn")) {
    return "WARN"
  }

  if (lower.includes("trace")) {
    return "TRACE"
  }

  if (lower.includes("debug")) {
    return "DEBUG"
  }

  return "INFO"
}

function colorizeSegment(value: string, color: string | undefined): string {
  if (!color) {
    return value
  }

  return `\x1b[${color}m${value}\x1b[0m`
}

function colorizeLogLine(line: string, source: string, level: string): string {
  const segments = line.split(" | ")
  if (segments.length < 4) {
    return line
  }

  const [timestamp, sourceSegment, levelSegment, ...rest] = segments
  return [
    timestamp,
    colorizeSegment(sourceSegment, SOURCE_COLORS[source]),
    colorizeSegment(levelSegment, LEVEL_COLORS[level]),
    ...rest,
  ].join(" | ")
}

function formatProcessLogLine(
  label: string,
  rawLine: string,
  receivedAt: string,
  isError: boolean,
): { fileEntry: string; consoleEntry: string } {
  const source = normalizeSourceLabel(label)
  const cleanLine = stripAnsi(rawLine).trim()

  let fileEntry: string
  try {
    const parsed = JSON.parse(cleanLine) as unknown
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      fileEntry = formatStructuredLogLine(
        source,
        parsed as StructuredLogPayload,
        receivedAt,
        inferTextLogLevel(cleanLine, isError),
      )

      const levelSegment = fileEntry.split(" | ")[2] ?? "INFO"
      return {
        fileEntry,
        consoleEntry: colorizeLogLine(fileEntry, source, levelSegment),
      }
    }
  } catch {
    // Plain text process output is formatted below.
  }

  const level = inferTextLogLevel(cleanLine, isError)
  fileEntry = [`[${receivedAt}]`, source, level, cleanLine].join(" | ")
  return {
    fileEntry,
    consoleEntry: colorizeLogLine(fileEntry, source, level),
  }
}

function appendManagerLog(label: string, rawLine: string, isError = false) {
  const timestamp = new Date().toISOString()
  const { fileEntry, consoleEntry } = formatProcessLogLine(
    label,
    rawLine,
    timestamp,
    isError,
  )

  try {
    appendFileSync(LOG_FILE, `${fileEntry}\n`)
  } catch {
    // ignore log file write errors
  }

  if (verbose) {
    process[isError ? "stderr" : "stdout"].write(`${consoleEntry}\n`)
  }
}

function createLoggedProcess(
  label: string,
  options: { color: string; cmd: string; args: Array<string> },
) {
  const env = {
    ...process.env,
    GO_PORT: serverPort,
    SERVER_PORT: serverPort,
    FRONTEND_PORT: frontendPort,
    HOST: host,
  }

  const logFd = openSync(LOG_FILE, "a")
  appendManagerLog(label, `Starting ${options.cmd} ${options.args.join(" ")}`)
  const proc = Bun.spawn([options.cmd, ...options.args], {
    env,
    stdin: "ignore",
    stdout: logFd,
    stderr: logFd,
    detached: true,
  })
  closeSync(logFd)
  proc.unref()
  appendManagerLog(label, `Spawned detached process (PID: ${proc.pid})`)

  return proc
}

async function startServices() {
  const existingPids = loadPids()
  if (existingPids) {
    const backendRunning =
      existingPids.backend && isProcessRunning(existingPids.backend)
    const frontendRunning =
      existingPids.frontend && isProcessRunning(existingPids.frontend)

    if (backendRunning || frontendRunning) {
      consola.warn(
        "Services are already running. Use 'stop' first or 'restart' to restart them.",
      )
      await showStatus()
      return
    }
  }

  consola.info("🚀 Starting OmniLLM development environment...")
  consola.info(`   🔥 Backend (Go): http://${host}:${serverPort}`)
  consola.info(`   🌐 Frontend: http://${host}:${frontendPort}`)
  consola.info(`   📱 Admin UI: http://${host}:${frontendPort}/admin/`)
  if (!verbose) {
    consola.info(`   📝 Logs: ${LOG_FILE}`)
    consola.info(`   💡 Use --verbose to see real-time logs`)
  }
  console.log()

  // Check ports
  const serverAvailable = await checkPortAvailable(Number(serverPort))

  if (!serverAvailable) {
    consola.error(`❌ Port ${serverPort} is already in use by another service`)
    consola.info(`💡 Try a different port: ./omni start --server-port 5003`)
    process.exit(1)
  }

  // Note: We don't strictly check frontend port as Vite will auto-find an available one

  // Clear old logs
  if (existsSync(LOG_FILE)) {
    writeFileSync(LOG_FILE, "")
  }

  const bunExe = process.execPath

  // Build Go backend directly to ~/.local/bin
  const isWindows = process.platform === "win32"
  const installPath =
    isWindows ?
      `${process.env.USERPROFILE}/.local/bin/omnillm.exe`
    : `${homedir()}/.local/bin/omnillm`

  consola.info("🔨 Building Golang backend...")
  const goExe =
    process.platform === "win32" ? `C:\\Program Files\\Go\\bin\\go.exe` : `go`
  const buildResult = Bun.spawn(
    [goExe, "build", "-o", installPath, "main.go"],
    {
      stdout: "inherit",
      stderr: "inherit",
    },
  )
  await buildResult.exited
  if (buildResult.exitCode !== 0) {
    consola.error("❌ Failed to build Golang backend")
    process.exit(1)
  }

  consola.success("✅ Golang backend built successfully")

  const backendProc = createLoggedProcess("go-backend", {
    color: "31",
    cmd: installPath,
    args: [
      "start",
      "--port",
      serverPort,
      "--host",
      host,
      "--enable-config-edit",
    ],
  })

  // Wait a bit for backend to start
  await Bun.sleep(2000)

  const frontendProc = createLoggedProcess("frontend", {
    color: "32",
    cmd: bunExe,
    args: [
      "node_modules/vite/bin/vite.js",
      "--config",
      "frontend/vite.config.ts",
      "--port",
      frontendPort,
    ],
  })

  // Save PIDs
  savePids({
    backend: backendProc.pid,
    frontend: frontendProc.pid,
    host,
    serverPort,
    frontendPort,
  })

  if (verbose) {
    await Bun.sleep(1500)
    showLogs()
  }

  consola.success("🎉 Services started successfully!")
  consola.info("💡 Use 'bun run omni-dev.ts stop' to stop services")
}

async function killPid(pid: number): Promise<boolean> {
  if (process.platform === "win32") {
    // /F = force, /T = kill entire process tree (catches Vite's child processes)
    const result = Bun.spawn(["taskkill", "/F", "/T", "/PID", String(pid)], {
      stdout: "pipe",
      stderr: "pipe",
    })
    await result.exited
    return result.exitCode === 0
  }
  try {
    // Try SIGTERM first, fall back to SIGKILL
    process.kill(pid, "SIGTERM")
    return true
  } catch {
    try {
      process.kill(pid, "SIGKILL")
      return true
    } catch {
      return false
    }
  }
}

async function stopServices() {
  const pids = loadPids()
  const runtime = getSavedRuntimeConfig(pids)
  consola.info("🛑 Stopping services...")

  const killed = new Set<number>()

  // First stop tracked processes
  if (pids?.backend && isProcessRunning(pids.backend)) {
    const ok = await killPid(pids.backend)
    if (ok) {
      const exited = await waitForProcessExit(pids.backend)
      const portReleased =
        runtime.serverPort ?
          await waitForPortAvailable(Number(runtime.serverPort))
        : true

      if (exited && portReleased) {
        killed.add(pids.backend)
        consola.success(`✅ Backend stopped (PID: ${pids.backend})`)
      } else {
        consola.warn(
          `⚠️  Backend stop is still draining (PID: ${pids.backend})`,
        )
      }
    } else {
      consola.warn(`⚠️  Could not stop backend (PID: ${pids.backend})`)
    }
  }

  if (pids?.frontend && isProcessRunning(pids.frontend)) {
    const ok = await killPid(pids.frontend)
    if (ok) {
      const exited = await waitForProcessExit(pids.frontend)
      const portReleased =
        runtime.frontendPort ?
          await waitForPortAvailable(Number(runtime.frontendPort))
        : true

      if (exited && portReleased) {
        killed.add(pids.frontend)
        consola.success(`✅ Frontend stopped (PID: ${pids.frontend})`)
      } else {
        consola.warn(
          `⚠️  Frontend stop is still draining (PID: ${pids.frontend})`,
        )
      }
    } else {
      consola.warn(`⚠️  Could not stop frontend (PID: ${pids.frontend})`)
    }
  }

  // Then find any stray OmniLLM processes by command signature
  const matchingPids = await findMatchingPids()
  const extraPids = matchingPids.filter((pid) => !killed.has(pid))

  for (const pid of extraPids) {
    const ok = await killPid(pid)
    if (ok) {
      killed.add(pid)
      consola.success(`✅ Stopped stray process (PID: ${pid})`)
    } else {
      consola.warn(`⚠️  Could not stop stray process (PID: ${pid})`)
    }
  }

  clearPids()

  if (killed.size === 0) {
    consola.info("ℹ️  No running services found")
  } else {
    consola.success(
      `🎉 Stopped ${killed.size} process${killed.size > 1 ? "es" : ""}`,
    )
  }
}

async function showStatus() {
  const pids = loadPids()
  const runtime = getSavedRuntimeConfig(pids)

  consola.info("📊 OmniLLM Service Status")
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

  if (!pids) {
    consola.info("📭 No services are currently tracked")
    console.log()
    consola.info("💡 Use 'bun run omni-dev.ts start' to start services")
    return
  }

  // Backend status
  const backendRunning = pids.backend && isProcessRunning(pids.backend)
  const backendPortBusy = !(await checkPortAvailable(
    Number(runtime.serverPort),
  ))

  console.log(`🔥 Backend (Go):`)
  console.log(`   Status: ${backendRunning ? "🟢 Running" : "🔴 Stopped"}`)
  console.log(`   PID: ${pids.backend || "N/A"}`)
  console.log(
    `   Port: ${runtime.serverPort} ${backendPortBusy ? "(🔒 Busy)" : "(🔓 Free)"}`,
  )
  console.log(`   URL: http://${runtime.host}:${runtime.serverPort}`)

  // Frontend status
  const frontendRunning = pids.frontend && isProcessRunning(pids.frontend)
  const frontendPortBusy = !(await checkPortAvailable(
    Number(runtime.frontendPort),
  ))

  console.log(`🌐 Frontend:`)
  console.log(`   Status: ${frontendRunning ? "🟢 Running" : "🔴 Stopped"}`)
  console.log(`   PID: ${pids.frontend || "N/A"}`)
  console.log(
    `   Port: ${runtime.frontendPort} ${frontendPortBusy ? "(🔒 Busy)" : "(🔓 Free)"}`,
  )
  console.log(`   URL: http://${runtime.host}:${runtime.frontendPort}`)
  console.log(
    `   Admin UI: http://${runtime.host}:${runtime.frontendPort}/admin/`,
  )

  console.log()

  if (backendRunning && frontendRunning) {
    consola.success("🎉 All services are running!")
  } else if (!backendRunning && !frontendRunning) {
    consola.info("💤 All services are stopped")
    consola.info("💡 Use 'bun run omni-dev.ts start' to start them")
  } else {
    consola.warn("⚠️  Some services are not running")
    consola.info("💡 Use 'bun run omni-dev.ts restart' to restart all")
  }
}

function showLogs({ follow = false } = {}) {
  if (!existsSync(LOG_FILE)) {
    consola.info("📭 No logs available yet")
    consola.info("💡 Start services first to generate logs")
    return
  }

  if (!follow) {
    try {
      const logs = readFileSync(LOG_FILE, "utf8")
      const lines = logs.split("\n").filter((line) => line.trim())
      const recentLines = lines.slice(-50) // Show last 50 lines

      consola.info("📋 Recent Service Logs (last 50 lines):")
      console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
      console.log(recentLines.join("\n"))
      console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
      consola.info(`💡 Full logs: ${LOG_FILE}`)
    } catch {
      consola.error("❌ Could not read log file")
    }
    return
  }

  // ── Follow mode: tail last 50 lines then stream new ones ──────────────────
  const logs = readFileSync(LOG_FILE, "utf8")
  const allLines = logs.split("\n").filter((line) => line.trim())
  const tailLines = allLines.slice(-50)

  consola.info("📋 Streaming logs (last 50 lines, then live)…")
  consola.info("💡 Press Ctrl+C to stop")
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

  // Print recent lines with colorization
  for (const line of tailLines) {
    printLogLine(line)
  }
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

  // Track the number of lines we've already processed
  let processedLineCount = allLines.length

  // Use a polling approach: read file periodically and detect new lines
  const pollInterval = 500 // ms
  let stopped = false

  const poll = () => {
    if (stopped) return
    try {
      const content = readFileSync(LOG_FILE, "utf8")
      const currentLines = content.split("\n").filter((line) => line.trim())

      // Only process new lines since our last check
      if (currentLines.length > processedLineCount) {
        const newLines = currentLines.slice(processedLineCount)
        processedLineCount = currentLines.length

        for (const line of newLines) {
          printLogLine(line)
        }
      }
    } catch {
      // File may be truncated or inaccessible momentarily
    }
    setTimeout(poll, pollInterval)
  }

  poll()

  // Handle graceful shutdown
  const onSignal = () => {
    stopped = true
    consola.info("\n🛑 Stopping log stream")
    process.exit(0)
  }

  process.on("SIGINT", onSignal)
  process.on("SIGTERM", onSignal)
}

function printLogLine(rawLine: string) {
  const trimmed = rawLine.trim()
  if (!trimmed) return

  // If the line is already pipe-formatted, just colorize it directly.
  // Check this before trying JSON parse — a pipe line starts with [timestamp].
  const pipeSegments = trimmed.split(" | ")
  if (pipeSegments.length >= 4 && /^\[.+\]$/.test(pipeSegments[0])) {
    const level = pipeSegments[2] ?? "INFO"
    const clean = stripAnsi(trimmed)
    process.stdout.write(
      `${colorizeLogLine(clean, pipeSegments[1] ?? "backend", level)}\n`,
    )
    return
  }

  // Otherwise try to parse as JSON from zerolog.
  try {
    const parsed = JSON.parse(trimmed) as unknown
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      const payload = { ...parsed } as StructuredLogPayload

      // ANSI escapes in the message field are stored as JSON unicode
      // (e.g. \u001b[32m). After JSON.parse they become actual escape codes
      // which corrupts terminal output — strip them.
      if (typeof payload.message === "string") {
        payload.message = stripAnsi(payload.message)
      }

      const formatted = formatStructuredLogLine(
        "backend",
        payload,
        new Date().toISOString(),
        inferTextLogLevel(trimmed, false),
      )
      const levelSeg = formatted.split(" | ")[2] ?? "INFO"
      process.stdout.write(
        `${colorizeLogLine(formatted, "backend", levelSeg)}\n`,
      )
      return
    }
  } catch {
    // Not JSON — fall through to plain-text formatting.
  }

  // Plain text — wrap with timestamp/source/level.
  const timestamp = new Date().toISOString()
  const clean = stripAnsi(trimmed)
  const level = inferTextLogLevel(clean, false)
  const entry = [`[${timestamp}]`, "backend", level, clean].join(" | ")
  process.stdout.write(`${colorizeLogLine(entry, "backend", level)}\n`)
}

// Main command handling
if (values.help || command === "help") {
  showHelp()
} else
  switch (command) {
    case "start": {
      await startServices()

      break
    }
    case "stop": {
      await stopServices()

      break
    }
    case "restart": {
      await stopServices()
      await Bun.sleep(2000)
      if (rebuild) {
        // Rebuild Go backend directly to ~/.local/bin
        const isWindows = process.platform === "win32"
        const installPath =
          isWindows ?
            `${process.env.USERPROFILE}/.local/bin/omnillm.exe`
          : `${homedir()}/.local/bin/omnillm`
        consola.info("🔨 Rebuilding Golang backend...")
        const goExe =
          process.platform === "win32" ?
            `C:\\Program Files\\Go\\bin\\go.exe`
          : `go`
        const backendBuild = Bun.spawn(
          [goExe, "build", "-o", installPath, "main.go"],
          {
            stdout: "inherit",
            stderr: "inherit",
          },
        )
        await backendBuild.exited
        if (backendBuild.exitCode !== 0) {
          consola.error("❌ Failed to rebuild Golang backend")
          process.exit(1)
        }

        consola.success("✅ Golang backend rebuilt successfully")
        // Rebuild frontend
        consola.info("🔨 Rebuilding frontend...")
        const frontendBuild = Bun.spawn([process.execPath, "run", "build"], {
          stdout: "inherit",
          stderr: "inherit",
        })
        await frontendBuild.exited
        if (frontendBuild.exitCode !== 0) {
          consola.error("❌ Failed to rebuild frontend")
          process.exit(1)
        }
        consola.success("✅ Frontend rebuilt successfully")
      }
      await startServices()

      break
    }
    case "status": {
      await showStatus()

      break
    }
    case "logs": {
      showLogs({ follow })

      break
    }
    default: {
      consola.error(`❌ Unknown command: ${command}`)
      console.log()
      showHelp()
      process.exit(1)
    }
  }

// Graceful shutdown handling
process.on("SIGINT", async () => {
  consola.info("\n🛑 Received interrupt signal...")
  await stopServices()
  process.exit(0)
})

process.on("SIGTERM", async () => {
  consola.info("\n🛑 Received termination signal...")
  await stopServices()
  process.exit(0)
})
