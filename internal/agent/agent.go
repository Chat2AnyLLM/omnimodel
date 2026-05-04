package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/tools"
)

// DispatchFn wraps the existing provider dispatch. It sends a CIF request
// and returns a channel of CIF responses (supporting streaming).
type DispatchFn func(ctx context.Context, req *cif.CanonicalRequest) (<-chan *cif.CanonicalResponse, error)

// RunResult contains the output of a completed agent run.
type RunResult struct {
	Output   string
	Steps    int
	Messages []cif.CIFMessage
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

		response, err := a.dispatchAndCollect(ctx, step)
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

		toolResults := a.registry.ExecuteToolCalls(ctx, sessionID, toolCalls)
		var toolResultParts []cif.CIFContentPart
		for _, r := range toolResults {
			isErr := r.IsError
			toolResultParts = append(toolResultParts, cif.CIFToolResultPart{
				Type:       "tool_result",
				ToolCallID: r.ToolCallID,
				ToolName:   r.ToolName,
				Content:    r.Content,
				IsError:    &isErr,
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

			response, err := a.dispatchAndCollect(ctx, step)
			if err != nil {
				events <- Event{Type: EventError, Content: err.Error()}
				return
			}

			for _, part := range response.Content {
				if tp, ok := part.(cif.CIFTextPart); ok {
					events <- Event{Type: EventToken, Content: tp.Text}
				}
			}

			a.memory.Append(toAssistantMessage(response.Content))

			toolCalls := extractToolCalls(response.Content)
			if len(toolCalls) == 0 {
				events <- Event{Type: EventDone}
				return
			}

			for _, tc := range toolCalls {
				events <- Event{Type: EventToolCall, Tool: tc.ToolName, Content: tc.ToolCallID}
			}

			toolResults := a.registry.ExecuteToolCalls(ctx, sessionID, toolCalls)

			var toolResultParts []cif.CIFContentPart
			for _, r := range toolResults {
				events <- Event{Type: EventToolResult, Tool: r.ToolName, Content: r.Content}
				isErr := r.IsError
				toolResultParts = append(toolResultParts, cif.CIFToolResultPart{
					Type:       "tool_result",
					ToolCallID: r.ToolCallID,
					ToolName:   r.ToolName,
					Content:    r.Content,
					IsError:    &isErr,
				})
			}
			a.memory.Append(toToolResultMessage(toolResultParts))
		}

		events <- Event{Type: EventError, Content: fmt.Sprintf("agent loop exceeded maximum steps (%d)", a.maxSteps)}
	}()

	return events, nil
}

func (a *Agent) appendUserMessage(prompt string) {
	a.memory.Append(cif.CIFUserMessage{
		Role: "user",
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: prompt},
		},
	})
}

func toToolResultMessage(results []cif.CIFContentPart) cif.CIFUserMessage {
	return cif.CIFUserMessage{
		Role:    "user",
		Content: results,
	}
}

func toAssistantMessage(content []cif.CIFContentPart) cif.CIFAssistantMessage {
	return cif.CIFAssistantMessage{
		Role:    "assistant",
		Content: content,
	}
}

func (a *Agent) dispatchAndCollect(ctx context.Context, step int) (*cif.CanonicalResponse, error) {
	req := a.buildRequest()

	respCh, err := a.dispatch(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("dispatch error at step %d: %w", step, err)
	}

	var response *cif.CanonicalResponse
	for resp := range respCh {
		response = resp
	}
	if response == nil {
		return nil, fmt.Errorf("nil response at step %d", step)
	}

	return response, nil
}

func (a *Agent) buildRequest() *cif.CanonicalRequest {
	messages := a.memory.Messages()
	cifTools := a.registry.ToCIFTools()

	req := &cif.CanonicalRequest{
		Messages: messages,
		Tools:    cifTools,
		Stream:   false,
	}
	// Per OpenAI spec: tool_choice must only be set when tools are present.
	if len(cifTools) > 0 {
		req.ToolChoice = "auto"
	}

	return req
}

func extractToolCalls(content []cif.CIFContentPart) []cif.CIFToolCallPart {
	var calls []cif.CIFToolCallPart
	for _, part := range content {
		if tc, ok := part.(cif.CIFToolCallPart); ok {
			calls = append(calls, tc)
		}
	}
	return calls
}

func extractTextContent(content []cif.CIFContentPart) string {
	var sb strings.Builder
	for _, part := range content {
		if tp, ok := part.(cif.CIFTextPart); ok {
			sb.WriteString(tp.Text)
		}
	}
	return sb.String()
}
