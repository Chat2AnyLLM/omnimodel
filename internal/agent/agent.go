package agent

import (
	"context"
	"errors"
	"fmt"
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
}

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

// Run executes the agent loop synchronously, returning the full trace.
func (a *Agent) Run(ctx context.Context, sessionID string, prompt string) (*RunResult, error) {
	a.appendUserMessage(prompt)

	var finalOutput string

	for step := 0; step < a.maxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return &RunResult{
				Output:   finalOutput,
				Steps:    step,
				Messages: a.memory.Messages(),
			}, err
		}

		response, err := a.dispatchAndCollect(ctx, step, prompt)
		if err != nil {
			return nil, err
		}

		a.memory.Append(toAssistantMessage(response.Content))

		toolCalls := extractToolCalls(response.Content)
		if len(toolCalls) == 0 {
			finalOutput = extractTextContent(response.Content)
			return &RunResult{
				Output:   finalOutput,
				Steps:    step + 1,
				Messages: a.memory.Messages(),
			}, nil
		}

		toolResults := a.registry.ExecuteCalls(ctx, sessionID, toolCalls)
		toolResultParts := make([]ContentBlock, 0, len(toolResults))
		for _, r := range toolResults {
			isErr := r.IsError
			toolResultParts = append(toolResultParts, ContentBlock{
				Type:      "tool_result",
				ToolUseID: r.ToolCallID,
				Name:      r.ToolName,
				Content:   r.Content,
				IsError:   &isErr,
			})
		}
		a.memory.Append(toToolResultMessage(toolResultParts))
	}

	return nil, errors.New("agent loop exceeded maximum steps (" + fmt.Sprint(a.maxSteps) + ")")
}

// Stream executes the agent loop and streams events back on a channel.
func (a *Agent) Stream(ctx context.Context, sessionID string, prompt string) (<-chan Event, error) {
	events := make(chan Event, 64)

	go func() {
		defer close(events)

		a.appendUserMessage(prompt)

		for step := 0; step < a.maxSteps; step++ {
			if err := ctx.Err(); err != nil {
				events <- Event{Type: EventError, Content: err.Error()}
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
				events <- Event{Type: EventDone}
				return
			}

			for _, tc := range toolCalls {
				events <- Event{Type: EventToolCall, Tool: tc.Name, Content: tc.ID}
			}

			toolResults := a.registry.ExecuteCalls(ctx, sessionID, toolCalls)

			toolResultParts := make([]ContentBlock, 0, len(toolResults))
			for _, r := range toolResults {
				events <- Event{Type: EventToolResult, Tool: r.ToolName, Content: r.Content}
				isErr := r.IsError
				toolResultParts = append(toolResultParts, ContentBlock{
					Type:      "tool_result",
					ToolUseID: r.ToolCallID,
					Name:      r.ToolName,
					Content:   r.Content,
					IsError:   &isErr,
				})
			}
			a.memory.Append(toToolResultMessage(toolResultParts))
		}

		events <- Event{Type: EventError, Content: fmt.Sprintf("agent loop exceeded maximum steps (%d)", a.maxSteps)}
	}()

	return events, nil
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
	messages := a.memory.Messages()
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
		Messages: filtered,
		Tools:    toolDefs,
		Stream:   false,
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

func extractTextContent(content []ContentBlock) string {
	var sb strings.Builder
	for _, part := range content {
		if part.Type == "text" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}
