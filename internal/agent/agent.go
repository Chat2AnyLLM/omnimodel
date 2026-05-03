package agent

import (
	"context"
	"errors"
	"fmt"
	"omnillm/internal/cif"
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
	registry *Registry
	memory   Memory
	maxSteps int
	dispatch DispatchFn
}

// NewAgent creates a new Agent with the given configuration.
// If maxSteps <= 0, defaults to 10.
func NewAgent(registry *Registry, memory Memory, maxSteps int, dispatch DispatchFn) *Agent {
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
	userMsg := cif.CIFUserMessage{
		Role: "user",
		Content: []cif.CIFContentPart{
			cif.CIFTextPart{Type: "text", Text: prompt},
		},
	}
	a.memory.Append(userMsg)

	var finalOutput string

	for step := 0; step < a.maxSteps; step++ {
		select {
		case <-ctx.Done():
			return &RunResult{
				Output:   finalOutput,
				Steps:    step,
				Messages: a.memory.Messages(),
			}, ctx.Err()
		default:
		}

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

		assistantMsg := cif.CIFAssistantMessage{
			Role:    "assistant",
			Content: response.Content,
		}
		a.memory.Append(assistantMsg)

		toolCalls := extractToolCalls(response.Content)
		if len(toolCalls) == 0 || response.StopReason != cif.StopReasonToolUse {
			finalOutput = extractTextContent(response.Content)
			return &RunResult{
				Output:   finalOutput,
				Steps:    step + 1,
				Messages: a.memory.Messages(),
			}, nil
		}

		results := a.registry.ExecuteToolCalls(ctx, sessionID, toolCalls)

		var resultParts []cif.CIFContentPart
		for _, r := range results {
			isErr := r.IsError
			resultParts = append(resultParts, cif.CIFToolResultPart{
				Type:       "tool_result",
				ToolCallID: r.ToolCallID,
				ToolName:   r.ToolName,
				Content:    r.Content,
				IsError:    &isErr,
			})
		}
		toolResultMsg := cif.CIFUserMessage{
			Role:    "user",
			Content: resultParts,
		}
		a.memory.Append(toolResultMsg)
	}

	return nil, errors.New("agent loop exceeded maximum steps (" + fmt.Sprint(a.maxSteps) + ")")
}

// Stream executes the agent loop and streams events back on a channel.
func (a *Agent) Stream(ctx context.Context, sessionID string, prompt string) (<-chan Event, error) {
	events := make(chan Event, 64)

	go func() {
		defer close(events)

		userMsg := cif.CIFUserMessage{
			Role: "user",
			Content: []cif.CIFContentPart{
				cif.CIFTextPart{Type: "text", Text: prompt},
			},
		}
		a.memory.Append(userMsg)

		for step := 0; step < a.maxSteps; step++ {
			select {
			case <-ctx.Done():
				events <- Event{Type: EventError, Content: ctx.Err().Error()}
				return
			default:
			}

			req := a.buildRequest()

			respCh, err := a.dispatch(ctx, req)
			if err != nil {
				events <- Event{Type: EventError, Content: err.Error()}
				return
			}

			var response *cif.CanonicalResponse
			for resp := range respCh {
				response = resp
				for _, part := range resp.Content {
					if tp, ok := part.(cif.CIFTextPart); ok {
						events <- Event{Type: EventToken, Content: tp.Text}
					}
				}
			}
			if response == nil {
				events <- Event{Type: EventError, Content: "nil response from provider"}
				return
			}

			assistantMsg := cif.CIFAssistantMessage{
				Role:    "assistant",
				Content: response.Content,
			}
			a.memory.Append(assistantMsg)

			toolCalls := extractToolCalls(response.Content)
			if len(toolCalls) == 0 || response.StopReason != cif.StopReasonToolUse {
				events <- Event{Type: EventDone}
				return
			}

			for _, tc := range toolCalls {
				events <- Event{Type: EventToolCall, Tool: tc.ToolName, Content: tc.ToolCallID}
			}

			results := a.registry.ExecuteToolCalls(ctx, sessionID, toolCalls)

			var resultParts []cif.CIFContentPart
			for _, r := range results {
				events <- Event{Type: EventToolResult, Tool: r.ToolName, Content: r.Content}
				isErr := r.IsError
				resultParts = append(resultParts, cif.CIFToolResultPart{
					Type:       "tool_result",
					ToolCallID: r.ToolCallID,
					ToolName:   r.ToolName,
					Content:    r.Content,
					IsError:    &isErr,
				})
			}
			toolResultMsg := cif.CIFUserMessage{
				Role:    "user",
				Content: resultParts,
			}
			a.memory.Append(toolResultMsg)
		}

		events <- Event{Type: EventError, Content: fmt.Sprintf("agent loop exceeded maximum steps (%d)", a.maxSteps)}
	}()

	return events, nil
}

func (a *Agent) buildRequest() *cif.CanonicalRequest {
	messages := a.memory.Messages()
	tools := a.registry.ToCIFTools()

	req := &cif.CanonicalRequest{
		Messages:   messages,
		Tools:      tools,
		ToolChoice: "auto",
		Stream:     false,
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
	var text string
	for _, part := range content {
		if tp, ok := part.(cif.CIFTextPart); ok {
			text += tp.Text
		}
	}
	return text
}
