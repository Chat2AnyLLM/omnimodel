# Harness Engineering Guide — Compliance Checklist & Implementation Plan

**Date:** May 11, 2026  
**Status:** Gap Analysis Complete — Ready for Phased Implementation  
**Repository:** OmniLLM Agent Runtime

---

## Executive Summary

Your agent runtime implements **core agentic-loop patterns** but lacks **production-grade guardrails, memory persistence, and context engineering**. This plan prioritizes fixes that have the highest impact on safety and reliability.

**Quick Stats:**
- ✅ Implemented: Basic loop, parallel tool calls, permission gates, sub-agent scaffolding
- ❌ Missing: Sandboxing, persistent memory, context budgeting, loop detection, retries, result sanitization

---

## Compliance Checklist

### ✅ ALREADY IMPLEMENTED

#### Agentic Loop
- [x] Think → Act → Observe cycle: [agent.go:45](internal/agent/agent.go#L45)
- [x] Hard max-turn limit: [agent.go:92](internal/agent/agent.go#L92)
- [x] Parallel tool execution: [tool.go:200](internal/tools/tool.go#L200)
- [x] Tool failures as messages (not crashes): [tool.go:223–248](internal/tools/tool.go#L223)

#### Tool System
- [x] Tool registry with definitions: [tool.go:115–175](internal/tools/tool.go#L115)
- [x] Tool execution with concurrent safety: [tool.go:200](internal/tools/tool.go#L200)
- [x] Tool descriptions present: [bash.go:15](internal/tools/bash.go#L15), [groups.go:1–60](internal/tools/groups.go#L1)

#### Guardrails (Partial)
- [x] Permission checker interface: [tool.go:35](internal/tools/tool.go#L35)
- [x] Human approval flow (REPL): [repl.go:180](internal/chat/repl.go#L180)
- [x] Permission denied returns error: [tool.go:248](internal/tools/tool.go#L248)
- [ ] OS-level sandboxing: **NOT IMPLEMENTED**
- [ ] Prompt-injection defenses (input sanitization): **NOT IMPLEMENTED**

#### Sub-Agents (Scaffolding)
- [x] Task store primitive: [tasks.go:33](internal/tools/tasks.go#L33)
- [x] Task lifecycle (create, list, get, output): [tasks.go:84–240](internal/tools/tasks.go#L84)
- [x] SendMessage interface: [tool.go:61](internal/tools/tool.go#L61)
- [x] AgentTool placeholder: [messaging.go:77](internal/tools/messaging.go#L77)
- [ ] Leader-worker orchestration: **NOT IMPLEMENTED**
- [ ] Session isolation for sub-agents: **NOT IMPLEMENTED**
- [ ] Git worktree coordination: **NOT IMPLEMENTED**

---

### ❌ HIGH PRIORITY — NOT IMPLEMENTED

#### 1. Sandbox (Shell Execution Isolation)
**Status:** CRITICAL  
**Guide Section:** [Sandbox](https://harness-guide.com/guide/sandbox/)

- [ ] Docker sandbox wrapper
  - [ ] Read-only root filesystem
  - [ ] Non-root user enforcement
  - [ ] Memory + CPU limits
  - [ ] Temp writable space (`--tmpfs`)
  - [ ] No network by default (`--network none`)
  - [ ] Cap drop all (`--cap-drop ALL`)
  
- [ ] Firecracker microVM support (multi-tenant)
- [ ] Network allowlist (if network needed)
- [ ] Seccomp profile (restrict syscalls)

**Current State:**  
Shell execution runs directly on host via `exec.CommandContext` in [shell.go:17–30](internal/tools/shell.go#L17).

**Why It Matters:**  
A single hallucinated `curl ... | bash` or `rm -rf /` can destroy the system. Approvals alone are insufficient.

**Implementation Scope:**
- Create `internal/sandbox/docker.go` with `DockerSandbox` struct
- Wrap `bash` and `powershell` tool execution through sandbox
- Add sandbox config to agent context
- ~200–300 LOC

---

#### 2. Persistent Memory & Context Assembly
**Status:** CRITICAL  
**Guide Section:** [Memory & Context](https://harness-guide.com/guide/memory-and-context/) + [Context Engineering](https://harness-guide.com/guide/context-engineering/)

**Part A: Persistent Memory**
- [ ] Load MEMORY.md at session startup
  - [ ] Parse from workspace root or session directory
  - [ ] Inject as system message (priority 1)
  
- [ ] Daily logs
  - [ ] Create `memory/YYYY-MM-DD.md` during session
  - [ ] Append learnings (not curated, just logged)
  - [ ] Load today's + yesterday's log at startup
  
- [ ] AGENTS.md support
  - [ ] Load from workspace root if present
  - [ ] Inject behavior rules as system message

**Part B: Context Assembly & Token Budgeting**
- [ ] Priority-based context assembly
  - Priority 0 (highest): System prompt + behavior rules
  - Priority 1: Active tool schemas only
  - Priority 2: Task instruction
  - Priority 3: Memory summary (MEMORY.md)
  - Priority 4: Relevant files (injected)
  - Priority 5: Recent conversation
  - Priority 6 (lowest): Older conversation
  
- [ ] Token budgeting
  - [ ] Estimate tokens for each section
  - [ ] Reserve 4K for response
  - [ ] Truncate/compress lower-priority content when over budget
  
- [ ] Threshold compression
  - [ ] When messages exceed 70% of budget, compress older turns into summary
  - [ ] Keep recent N turns verbatim

**Current State:**  
Runtime creates fresh `NewBufferMemory(64)` each turn in [session_runner.go:25](internal/agent/session_runner.go#L25). No persistent layer, no token accounting, no priority packing.

**Why It Matters:**  
Context is the agent's "working memory." Without persistence and budgeting, the agent forgets lessons and wastes tokens on stale messages.

**Implementation Scope:**
- Create `internal/agent/memory_manager.go` for load/save
- Extend `memory.go` with token estimation + priority packing
- Modify `session_runner.go` to load memory at startup
- ~400–500 LOC

---

#### 3. Context Budgeting in the Live Loop
**Status:** HIGH  
**Guide Section:** [Context Engineering](https://harness-guide.com/guide/context-engineering/)

- [ ] Call `Compact(ctx, summarizer)` in loop when approaching budget
- [ ] Replace older messages with summary instead of truncating
- [ ] Reassemble context each turn (priority order)

**Current State:**  
`BufferMemory.Compact()` is a no-op in [memory.go:55](internal/agent/memory.go#L55). `buildRequest()` in [agent.go:197](internal/agent/agent.go#L197) does not do token accounting or priority-based packing.

**Why It Matters:**  
A 50-turn coding session can hit 60K+ tokens; without compression, you're out of budget by turn 35.

**Implementation Scope:**
- Wire `Compact()` into the main loop in [agent.go:45–92](internal/agent/agent.go#L45)
- Implement summarization callback (optional: use cheap model or heuristic)
- ~150–200 LOC

---

### ❌ MEDIUM PRIORITY — NOT IMPLEMENTED

#### 4. Loop Detection & Exit Conditions
**Status:** MEDIUM  
**Guide Section:** [Agentic Loop](https://harness-guide.com/guide/agentic-loop/)

- [ ] Detect repeated identical tool calls
  - [ ] Track last N calls in a sliding window
  - [ ] If all N are identical, escalate or abort
  
- [ ] Token budget exceeded exit
  - [ ] Check tokens before each dispatch
  - [ ] Exit with error + checkpoint if over
  
- [ ] Consecutive errors exit
  - [ ] Count tool failures in a row
  - [ ] Abort if > 3 consecutive errors

**Current State:**  
Only max-turn check in [agent.go:92](internal/agent/agent.go#L92). No stuck-loop or budget detection.

**Why It Matters:**  
A confused model calling the same failing tool repeatedly will burn tokens and frustrate users.

**Implementation Scope:**
- Add call history to `Agent` struct
- Add detection logic before dispatch in [agent.go:45–92](internal/agent/agent.go#L45)
- ~100–150 LOC

---

#### 5. Error Handling & Retry Strategy
**Status:** MEDIUM  
**Guide Section:** [Error Handling](https://harness-guide.com/guide/error-handling/)

- [ ] Error classification
  - [ ] Transient (timeout, rate limit, 5xx)
  - [ ] Permanent (not found, permission denied, invalid input)
  - [ ] Model (bad tool call, invalid JSON)
  - [ ] Resource (out of memory, token limit)
  
- [ ] Retry with exponential backoff
  - [ ] Only transient errors
  - [ ] Base delay 1–2s, max 60s
  - [ ] Add jitter to prevent thundering herd
  
- [ ] Graceful degradation
  - [ ] Tool fallbacks (e.g., `read_file` → `cat` via shell)
  - [ ] Alternative strategies

**Current State:**  
Dispatch returns errors directly in [runtime.go:54](internal/agent/runtime.go#L54), [runtime.go:92](internal/agent/runtime.go#L92). No retry or classification layer.

**Why It Matters:**  
Transient network blips shouldn't crash the agent; permanent errors should escalate.

**Implementation Scope:**
- Create `internal/agent/error_handler.go` with classification + retry
- Wrap dispatch calls in [session_runner.go:19](internal/agent/session_runner.go#L19) and [session_runner.go:36](internal/agent/session_runner.go#L36)
- ~200–250 LOC

---

#### 6. Tool Result Sanitization & Truncation
**Status:** MEDIUM  
**Guide Section:** [Guardrails](https://harness-guide.com/guide/guardrails/) + [Context Engineering](https://harness-guide.com/guide/context-engineering/)

- [ ] Demarcate external content
  - [ ] Wrap tool results in `<tool_result>...</tool_result>` tags
  - [ ] Mark as untrusted data so model knows to be cautious
  
- [ ] Truncate large outputs
  - [ ] Cap tool results at 10K tokens
  - [ ] Append `[TRUNCATED]` marker

**Current State:**  
Tool results appended directly in [agent.go:77](internal/agent/agent.go#L77) and [agent.go:134](internal/agent/agent.go#L134) with no framing or truncation.

**Why It Matters:**  
A malicious file or web page in the tool result could prompt-inject the agent if not marked as external.

**Implementation Scope:**
- Add sanitization function to `internal/agent/sanitize.go`
- Call before appending to memory in [agent.go:77](internal/agent/agent.go#L77)
- ~80–120 LOC

---

#### 7. Dynamic Tool Loading (Skill System)
**Status:** MEDIUM  
**Guide Section:** [Tool System](https://harness-guide.com/guide/tool-system/) + [Skill System](https://harness-guide.com/guide/skill-system/)

- [ ] Load only active skill tools
  - [ ] Start with a menu of 5–10 essential tools
  - [ ] Add `load_skill(name)` tool that adds skill-specific tools
  
- [ ] SKILL.md format
  - [ ] Define each skill's tools and behavior
  - [ ] Load from workspace on demand

**Current State:**  
All tools registered upfront in [session_runner.go:21](internal/agent/session_runner.go#L21) and [session_runner.go:106](internal/agent/session_runner.go#L106). Full registry passed in every request in [agent.go:197](internal/agent/agent.go#L197).

**Why It Matters:**  
36 tools × ~100 tokens each = 3.6K tokens wasted on the schema alone. For a 128K window, that's 3% overhead per turn.

**Implementation Scope:**
- Mark tools as skill-membership in metadata
- Implement `load_skill` tool
- Filter registry by active skills in `buildRequest()`
- ~150–200 LOC

---

## Prioritized Implementation Plan

### **Phase 1: Immediate (Days 1–3) — Safety & Trust Boundaries**

#### Goal
Protect the host from command execution and add the minimal guardrails for production use.

**Tasks:**

1. **Sandbox Shell Execution** (Priority: CRITICAL)
   - [ ] Create `internal/sandbox/docker.go` with basic Docker sandbox wrapper
   - [ ] Add `DockerSandbox` config to agent initialization
   - [ ] Wrap `bash` and `powershell` tools to execute in sandbox
   - [ ] Write unit tests for sandbox execution
   - **Estimated Time:** 4–6 hours
   - **Files to Create/Modify:**
     - `internal/sandbox/docker.go` (new)
     - `internal/tools/bash.go` (modify to use sandbox)
     - `internal/tools/powershell.go` (modify to use sandbox)
     - `internal/agent/agent.go` (add sandbox config)
   - **Test Coverage:** Verify `rm -rf /` is blocked, commands return output correctly

2. **Tool Result Sanitization** (Priority: HIGH)
   - [ ] Create `internal/agent/sanitize.go` with truncation + demarcation
   - [ ] Wrap all tool results before memory append in [agent.go:77](internal/agent/agent.go#L77)
   - [ ] Add max length config (default 10K tokens)
   - **Estimated Time:** 1–2 hours
   - **Files to Create/Modify:**
     - `internal/agent/sanitize.go` (new)
     - `internal/agent/agent.go` (call sanitizer in loop)
   - **Test Coverage:** Verify large outputs are truncated and marked

---

### **Phase 2: Foundation (Days 4–7) — Memory & Context**

#### Goal
Enable the agent to learn and use context intelligently.

**Tasks:**

1. **Persistent Memory Loading** (Priority: CRITICAL)
   - [ ] Create `internal/agent/memory_manager.go`
   - [ ] Implement load functions for MEMORY.md, daily logs, AGENTS.md
   - [ ] Inject into session startup in [session_runner.go:19–30](internal/agent/session_runner.go#L19)
   - [ ] Write integration test loading from workspace
   - **Estimated Time:** 3–4 hours
   - **Files to Create/Modify:**
     - `internal/agent/memory_manager.go` (new)
     - `internal/agent/session_runner.go` (call memory_manager at startup)
   - **Test Coverage:** Verify memory files are loaded and injected as system messages

2. **Token Estimation & Priority-Based Assembly** (Priority: HIGH)
   - [ ] Add token counting to `internal/agent/memory.go`
   - [ ] Implement priority-based context assembly in a new `ContextAssembler` struct
   - [ ] Modify `buildRequest()` in [agent.go:197](internal/agent/agent.go#L197) to use assembler
   - **Estimated Time:** 4–5 hours
   - **Files to Create/Modify:**
     - `internal/agent/memory.go` (add token counting)
     - `internal/agent/context_assembler.go` (new)
     - `internal/agent/agent.go` (call assembler in buildRequest)
   - **Test Coverage:** Verify context is packed in priority order and respects budget

3. **Threshold Compression in Loop** (Priority: HIGH)
   - [ ] Implement `Compact()` for `SummaryMemory` in [memory.go:95](internal/agent/memory.go#L95)
   - [ ] Wire into main loop after dispatch in [agent.go:45–92](internal/agent/agent.go#L45)
   - [ ] Test with long conversations (50+ turns)
   - **Estimated Time:** 2–3 hours
   - **Files to Create/Modify:**
     - `internal/agent/memory.go` (implement Compact)
     - `internal/agent/agent.go` (call Compact in loop)
   - **Test Coverage:** Verify older messages are summarized when over budget

---

### **Phase 3: Robustness (Days 8–10) — Error Handling & Loop Safety**

#### Goal
Make the agent resilient to transient failures and prevent stuck loops.

**Tasks:**

1. **Error Classification & Retry** (Priority: MEDIUM)
   - [ ] Create `internal/agent/error_handler.go` with classification logic
   - [ ] Implement exponential backoff with jitter
   - [ ] Wrap dispatch in [session_runner.go:19](internal/agent/session_runner.go#L19) and [session_runner.go:36](internal/agent/session_runner.go#L36) with retry
   - **Estimated Time:** 3–4 hours
   - **Files to Create/Modify:**
     - `internal/agent/error_handler.go` (new)
     - `internal/agent/session_runner.go` (wrap dispatch with retry)
   - **Test Coverage:** Mock transient + permanent errors; verify retry behavior

2. **Loop Detection** (Priority: MEDIUM)
   - [ ] Add call history tracking to `Agent` struct
   - [ ] Detect repeated identical tool calls before executing
   - [ ] Detect token budget exceeded
   - [ ] Detect 3+ consecutive tool failures
   - **Estimated Time:** 2–3 hours
   - **Files to Create/Modify:**
     - `internal/agent/agent.go` (add detection logic)
   - **Test Coverage:** Simulate stuck loops; verify detection and early exit

---

### **Phase 4: Optimization (Days 11–13) — Skills & Efficiency**

#### Goal
Reduce token overhead and support skill-based tool loading.

**Tasks:**

1. **Dynamic Tool Loading / Skills** (Priority: MEDIUM)
   - [ ] Mark tools with skill membership in [groups.go:1–60](internal/tools/groups.go#L1)
   - [ ] Implement `load_skill` tool
   - [ ] Filter registry by active skills in `buildRequest()` in [agent.go:197](internal/agent/agent.go#L197)
   - [ ] Create SKILL.md format documentation
   - **Estimated Time:** 3–4 hours
   - **Files to Create/Modify:**
     - `internal/tools/groups.go` (add skill metadata)
     - `internal/tools/skill_loader.go` (new)
     - `internal/agent/agent.go` (filter tools by skill in buildRequest)
   - **Test Coverage:** Verify tools are loaded on-demand; schema count decreases

---

### **Phase 5: Advanced (Days 14+) — Production Completeness**

#### Goal
Full compliance with the guide for multi-tenant and long-running agents.

**Tasks:**

1. **Sub-Agent Orchestration** (Priority: MEDIUM)
   - [ ] Implement true leader-worker pattern with file-based communication
   - [ ] Add session isolation for sub-agents
   - [ ] Wire `SendMessageFn` in production paths
   - **Estimated Time:** 4–5 hours

2. **Firecracker Sandbox** (Priority: LOW)
   - [ ] Implement for multi-tenant isolation
   - **Estimated Time:** 8–10 hours

3. **Checkpoint & Resume** (Priority: MEDIUM)
   - [ ] Implement checkpoint saving every N turns
   - [ ] Add resume-from-checkpoint support in session_runner
   - **Estimated Time:** 2–3 hours

---

## Implementation Order Rationale

| Phase | Why First | Impact |
|-------|-----------|--------|
| **Phase 1** | Stop shell disasters; prevent prompt injection | Blocks production use without it |
| **Phase 2** | Agent learns and uses context efficiently | Enables long-running tasks |
| **Phase 3** | Resilience to network blips and stuck loops | Improves reliability |
| **Phase 4** | Token efficiency | Nice-to-have; reduces cost |
| **Phase 5** | Advanced production patterns | Only needed at scale |

---

## Metrics for Success

After each phase, validate:

- **Phase 1:**
  - [ ] Shell commands execute inside container
  - [ ] Host filesystem is untouched
  - [ ] Tool results are demarcated and truncated

- **Phase 2:**
  - [ ] MEMORY.md is loaded and used
  - [ ] Daily logs are created
  - [ ] Context respects token budget
  - [ ] Compression triggers at threshold

- **Phase 3:**
  - [ ] Transient errors retry with backoff
  - [ ] Stuck loops detected and logged
  - [ ] Agent completes 50+ turn conversation without OOM

- **Phase 4:**
  - [ ] Tool schema count decreases with load_skill
  - [ ] Token overhead for tools < 2% of budget

- **Phase 5:**
  - [ ] Sub-agents run isolated from parent
  - [ ] Checkpoints allow resume without replay

---

## Questions to Answer Before Starting

1. **Sandbox Engine:** Do you want Docker, Firecracker, or both?
2. **Memory Backend:** File-based, database, or in-memory with periodic flush?
3. **Summarization:** Use a fast model, heuristic, or user-provided summaries?
4. **Skill Metadata:** Hardcoded in code or loaded from SKILL.md files?
5. **Token Estimator:** Use a library like `tiktoken` or simple word-count heuristic?

---

## Success Criteria

The agent will be **Harness Guide Compliant** when:

✅ Shell execution is sandboxed  
✅ Persistent memory is loaded and updated  
✅ Context respects token budget and compression  
✅ Loop detection prevents stuck cycles  
✅ Errors are classified and retried  
✅ Tool results are sanitized and truncated  
✅ Tools are loaded dynamically (optional but recommended)  

Estimated total effort: **25–35 engineer-hours** spread over **2–3 weeks** depending on parallelization and testing depth.
