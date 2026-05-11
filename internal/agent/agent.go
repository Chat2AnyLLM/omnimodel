package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"omnillm/internal/tools"
)

// DispatchFn sends an Anthropic /v1/messages request and returns agent-native responses.
type DispatchFn func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error)

// RunResult contains the output of a completed agent run.
type RunResult struct {
	Output   string
	Steps    int
	Messages []Message
}

// Agent orchestrates multi-step LLM interactions with tool calling.
type Agent struct {
	registry *tools.Registry
	memory   Memory
	maxSteps int
	dispatch DispatchFn
	// Summarizer is an optional callback used by compactIfNeeded to compress
	// old messages when the estimated token count exceeds the budget threshold.
	// When nil, compaction is skipped and older messages are simply dropped
	// by BufferMemory's sliding-window behaviour.
	Summarizer SummarizerFn
}

const (
	stuckToolCallWindow         = 3
	maxConsecutiveAllErrorSteps = 3
	// contextTokenBudget is the approximate model context window we target.
	// We reserve 4K for the model's response, leaving this for the prompt.
	contextTokenBudget = 28_000
	// compactThresholdRatio triggers compaction when messages exceed this
	// fraction of the context budget (70%).
	compactThresholdRatio = 0.70
)

// NewAgent creates a new Agent with the given configuration.
// If maxSteps <= 0, defaults to 10.
func NewAgent(registry *tools.Registry, memory Memory, maxSteps int, dispatch DispatchFn) *Agent {
	if maxSteps <= 0 {
		maxSteps = 10
	}
	return &Agent{
		registry: registry,
		memory:   memory,
		maxSteps: maxSteps,
		dispatch: dispatch,
	}
}

// compactIfNeeded calls memory.Compact when the estimated token usage exceeds
// the compaction threshold. It is a best-effort operation — errors are ignored
// so a failing summarizer does not abort the run.
func (a *Agent) compactIfNeeded(ctx context.Context) {
	threshold := int(float64(contextTokenBudget) * compactThresholdRatio)
	total := 0
	for _, msg := range a.memory.Messages() {
		total += estimateMessageTokens(msg)
	}
	if total >= threshold {
		_ = a.memory.Compact(ctx, a.Summarizer)
	}
}

// Run executes the agent loop synchronously, returning the full trace.
func (a *Agent) Run(ctx context.Context, sessionID string, prompt string) (*RunResult, error) {
	startStep := 0
	if cp, err := loadCheckpoint(sessionID); err == nil && cp != nil {
		// Resume from saved checkpoint: restore memory and fast-forward the loop counter.
		a.memory.Reset(cp.Messages)
		startStep = cp.Step
	} else {
		a.appendUserMessage(prompt)
	}

	var finalOutput string
	toolCallSignatures := make([]string, 0, stuckToolCallWindow)
	consecutiveAllErrorSteps := 0

	for step := startStep; step < a.maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return &RunResult{
				Output:   finalOutput,
				Steps:    step,
				Messages: a.memory.Messages(),
			}, err
		}

		a.compactIfNeeded(ctx)

		// Abort if still over token budget after compaction attempt.
		if totalBudgetTokens := a.estimateTotalTokens(); totalBudgetTokens > contextTokenBudget {
			_ = saveCheckpoint(sessionID, step, a.memory.Messages())
			return nil, fmt.Errorf("context token budget exceeded (%d > %d tokens); checkpoint saved, agent aborted", totalBudgetTokens, contextTokenBudget)
		}

		response, err := a.dispatchAndCollect(ctx, step, prompt)
		if err != nil {
			return nil, err
		}

		a.memory.Append(toAssistantMessage(response.Content))

		toolCalls := extractToolCalls(response.Content)
		if len(toolCalls) == 0 {
			finalOutput = extractTextContent(response.Content)
			clearCheckpoint(sessionID)
			return &RunResult{
				Output:   finalOutput,
				Steps:    step + 1,
				Messages: a.memory.Messages(),
			}, nil
		}

		toolCallSignatures = append(toolCallSignatures, toolCallBatchSignature(toolCalls))
		if len(toolCallSignatures) > stuckToolCallWindow {
			toolCallSignatures = toolCallSignatures[len(toolCallSignatures)-stuckToolCallWindow:]
		}
		if hasRepeatedSignatures(toolCallSignatures, stuckToolCallWindow) {
			return nil, errors.New("agent detected repeated identical tool calls and aborted")
		}

		toolResults := a.registry.ExecuteCalls(ctx, sessionID, toolCalls)
		allErrors := len(toolResults) > 0
		toolResultParts := make([]ContentBlock, 0, len(toolResults))
		for _, r := range toolResults {
			isErr := r.IsError
			if !r.IsError {
				allErrors = false
			}
			safeContent := sanitizeToolResultForModel(r.ToolName, r.Content, r.IsError)
			toolResultParts = append(toolResultParts, ContentBlock{
				Type:      "tool_result",
				ToolUseID: r.ToolCallID,
				Name:      r.ToolName,
				Content:   safeContent,
				IsError:   &isErr,
			})
		}
		if allErrors {
			consecutiveAllErrorSteps++
			if consecutiveAllErrorSteps >= maxConsecutiveAllErrorSteps {
				return nil, errors.New("agent aborted after consecutive tool-error steps")
			}
		} else {
			consecutiveAllErrorSteps = 0
		}
		a.memory.Append(toToolResultMessage(toolResultParts))

		// Checkpoint every N steps so long runs can be resumed after interruption.
		if (step+1)%checkpointEveryNSteps == 0 {
			_ = saveCheckpoint(sessionID, step+1, a.memory.Messages())
		}
	}

	return nil, errors.New("agent loop exceeded maximum steps (" + fmt.Sprint(a.maxSteps) + ")")
}

// Stream executes the agent loop and streams events back on a channel.
func (a *Agent) Stream(ctx context.Context, sessionID string, prompt string) (<-chan Event, error) {
	events := make(chan Event, 64)

	go func() {
		defer close(events)

		startStep := 0
		if cp, err := loadCheckpoint(sessionID); err == nil && cp != nil {
			a.memory.Reset(cp.Messages)
			startStep = cp.Step
		} else {
			a.appendUserMessage(prompt)
		}

		toolCallSignatures := make([]string, 0, stuckToolCallWindow)
		consecutiveAllErrorSteps := 0

		for step := startStep; step < a.maxSteps; step++ {
			if err := ctx.Err(); err != nil {
				events <- Event{Type: EventError, Content: err.Error()}
				return
			}

			a.compactIfNeeded(ctx)

			// Abort if still over token budget after compaction attempt.
			if totalBudgetTokens := a.estimateTotalTokens(); totalBudgetTokens > contextTokenBudget {
				_ = saveCheckpoint(sessionID, step, a.memory.Messages())
				events <- Event{Type: EventError, Content: fmt.Sprintf("context token budget exceeded (%d > %d tokens); checkpoint saved, agent aborted", totalBudgetTokens, contextTokenBudget)}
				return
			}

			response, err := a.dispatchAndCollect(ctx, step, prompt)
			if err != nil {
				events <- Event{Type: EventError, Content: err.Error()}
				return
			}

			for _, part := range response.Content {
				if part.Type == "text" {
					events <- Event{Type: EventToken, Content: part.Text}
				}
			}

			a.memory.Append(toAssistantMessage(response.Content))

			toolCalls := extractToolCalls(response.Content)
			if len(toolCalls) == 0 {
				clearCheckpoint(sessionID)
				events <- Event{Type: EventDone}
				return
			}

			toolCallSignatures = append(toolCallSignatures, toolCallBatchSignature(toolCalls))
			if len(toolCallSignatures) > stuckToolCallWindow {
				toolCallSignatures = toolCallSignatures[len(toolCallSignatures)-stuckToolCallWindow:]
			}
			if hasRepeatedSignatures(toolCallSignatures, stuckToolCallWindow) {
				events <- Event{Type: EventError, Content: "agent detected repeated identical tool calls and aborted"}
				return
			}

			for _, tc := range toolCalls {
				events <- Event{Type: EventToolCall, Tool: tc.Name, Content: formatToolCallPayload(tc)}
			}

			toolResults := a.registry.ExecuteCalls(ctx, sessionID, toolCalls)
			allErrors := len(toolResults) > 0

			toolResultParts := make([]ContentBlock, 0, len(toolResults))
			for _, r := range toolResults {
				events <- Event{Type: EventToolResult, Tool: r.ToolName, Content: r.Content}
				isErr := r.IsError
				if !r.IsError {
					allErrors = false
				}
				safeContent := sanitizeToolResultForModel(r.ToolName, r.Content, r.IsError)
				toolResultParts = append(toolResultParts, ContentBlock{
					Type:      "tool_result",
					ToolUseID: r.ToolCallID,
					Name:      r.ToolName,
					Content:   safeContent,
					IsError:   &isErr,
				})
			}
			if allErrors {
				consecutiveAllErrorSteps++
				if consecutiveAllErrorSteps >= maxConsecutiveAllErrorSteps {
					events <- Event{Type: EventError, Content: "agent aborted after consecutive tool-error steps"}
					return
				}
			} else {
				consecutiveAllErrorSteps = 0
			}
			a.memory.Append(toToolResultMessage(toolResultParts))

			// Checkpoint every N steps so long runs can be resumed after interruption.
			if (step+1)%checkpointEveryNSteps == 0 {
				_ = saveCheckpoint(sessionID, step+1, a.memory.Messages())
			}
		}

		events <- Event{Type: EventError, Content: fmt.Sprintf("agent loop exceeded maximum steps (%d)", a.maxSteps)}
	}()

	return events, nil
}

// estimateTotalTokens returns the approximate token count for all messages
// currently in memory. Used to enforce the context token budget guard.
func (a *Agent) estimateTotalTokens() int {
	total := 0
	for _, msg := range a.memory.Messages() {
		total += estimateMessageTokens(msg)
	}
	return total
}

func (a *Agent) appendUserMessage(prompt string) {
	a.memory.Append(Message{
		Role:    "user",
		Content: []ContentBlock{TextBlock(prompt)},
	})
}

func toToolResultMessage(results []ContentBlock) Message {
	return Message{
		Role:    "user",
		Content: results,
	}
}

func toAssistantMessage(content []ContentBlock) Message {
	return Message{
		Role:    "assistant",
		Content: content,
	}
}

func (a *Agent) dispatchAndCollect(ctx context.Context, step int, prompt string) (*MessagesResponse, error) {
	req := a.buildRequest(step, prompt)

	respCh, err := a.dispatch(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("dispatch error at step %d: %w", step, err)
	}

	var response *MessagesResponse
	for resp := range respCh {
		response = resp
	}
	if response == nil {
		return nil, fmt.Errorf("nil response at step %d", step)
	}

	return response, nil
}

func (a *Agent) buildRequest(step int, prompt string) *MessagesRequest {
	// Assemble messages within the context token budget before building the request.
	assembler := NewContextAssembler(contextTokenBudget)
	messages := assembler.Assemble(a.memory.Messages())
	toolDefs := a.registry.Definitions()

	// Extract system prompt from memory; send as SystemPrompt, not buried in history.
	var systemPrompt string
	filtered := make([]Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" {
			text := extractTextContent(msg.Content)
			if systemPrompt == "" {
				systemPrompt = text
			} else if text != "" {
				systemPrompt += "\n" + text
			}
		} else {
			filtered = append(filtered, msg)
		}
	}

	req := &MessagesRequest{
		MaxTokens: 4096,
		Messages:  filtered,
		Tools:     toolDefs,
		Stream:    false,
	}
	if systemPrompt != "" {
		req.System = []ContentBlock{TextBlock(systemPrompt)}
	}
	// Per OpenAI spec: tool_choice must only be set when tools are present.
	if len(toolDefs) > 0 {
		if step == 0 && shouldRequireInitialToolUse(prompt) {
			req.ToolChoice = "required"
		} else {
			req.ToolChoice = "auto"
		}
	}

	return req
}

func shouldRequireInitialToolUse(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return false
	}

	conversationalPhrases := []string{
		"hello", "hi", "thanks", "thank you", "what do you think", "how should we approach",
		"explain the architecture", "explain this", "why does", "what is", "can you explain",
	}
	for _, phrase := range conversationalPhrases {
		if p == phrase || strings.HasPrefix(p, phrase+" ") {
			return false
		}
	}

	actionPhrases := []string{
		"list ", "show ", "find ", "search ", "grep ", "glob ", "read ", "open ",
		"check ", "verify ", "inspect ", "look at ", "look for ", "run ", "test ",
		"trace ", "locate ", "browse ", "scan ", "print ", "cat ",
		"current directory", "working directory", "current time", "git status", "git diff",
		"environment variable", "env var", "os info", "system info", "list directory", "show directory",
		"references to", "where is", "does this exist", "is this wired", "is x wired",
	}
	for _, phrase := range actionPhrases {
		if strings.Contains(p, phrase) {
			return true
		}
	}

	for _, term := range strings.FieldsFunc(p, func(r rune) bool {
		return r < '0' || r > '9' && r < 'a' || r > 'z'
	}) {
		switch term {
		case "cpu", "memory", "disk", "process", "file", "files", "directory", "repo", "repository", "branch", "commit", "test", "tests", "log", "logs", "config", "code", "symbol", "symbols":
			return true
		}
	}

	questionMarkers := []string{"does ", "is ", "are ", "which ", "where ", "what files", "what changed"}
	for _, prefix := range questionMarkers {
		if strings.HasPrefix(p, prefix) && (strings.Contains(p, "repo") || strings.Contains(p, "file") || strings.Contains(p, "directory") || strings.Contains(p, "branch") || strings.Contains(p, "config") || strings.Contains(p, "code")) {
			return true
		}
	}

	return false
}

func extractToolCalls(content []ContentBlock) []tools.ToolCall {
	var calls []tools.ToolCall
	for _, part := range content {
		if part.Type == "tool_use" {
			calls = append(calls, tools.ToolCall{ID: part.ID, Name: part.Name, Arguments: part.Input})
		}
	}
	return calls
}

func toolCallBatchSignature(calls []tools.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	parts := make([]string, 0, len(calls))
	for _, call := range calls {
		argJSON, err := json.Marshal(call.Arguments)
		if err != nil {
			argJSON = []byte("{}")
		}
		parts = append(parts, call.Name+":"+string(argJSON))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func hasRepeatedSignatures(signatures []string, window int) bool {
	if window <= 0 || len(signatures) < window {
		return false
	}
	start := len(signatures) - window
	first := signatures[start]
	if first == "" {
		return false
	}
	for _, sig := range signatures[start+1:] {
		if sig != first {
			return false
		}
	}
	return true
}

func formatToolCallPayload(call tools.ToolCall) string {
	if command, ok := call.Arguments["command"].(string); ok && command != "" {
		return command
	}
	encoded, err := json.Marshal(call.Arguments)
	if err != nil {
		return call.ID
	}
	return string(encoded)
}

func extractTextContent(content []ContentBlock) string {
	var sb strings.Builder
	for _, part := range content {
		if part.Type == "text" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}
