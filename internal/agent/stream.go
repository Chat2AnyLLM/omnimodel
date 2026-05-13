package agent

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// EventType represents the type of agent streaming event.
type EventType string

const (
	EventToken        EventType = "token"
	EventToolCall     EventType = "tool_call"
	EventToolResult   EventType = "tool_result"
	EventTurnProgress EventType = "turn_progress"
	EventDone         EventType = "done"
	EventError        EventType = "error"
)

// Event represents a single agent streaming event.
type Event struct {
	Type     EventType
	Content  string
	Tool     string
	Turn     int
	MaxTurns int
}

// SerializeToSSE converts an agent Event to OpenAI-compatible SSE format.
// EventToken maps to a normal content delta chunk.
// EventToolCall/EventToolResult map to content with metadata.
// EventDone maps to the [DONE] sentinel.
// EventError maps to an error chunk.
func SerializeToSSE(e Event, streamID string) []byte {
	if streamID == "" {
		streamID = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}

	switch e.Type {
	case EventToken:
		content := e.Content
		chunk := map[string]any{
			"id":      streamID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"content": content,
					},
				},
			},
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			log.Error().Err(err).Msg("agent: failed to marshal token event")
			return nil
		}
		return []byte(fmt.Sprintf("data: %s\n\n", data))

	case EventToolCall:
		chunk := map[string]any{
			"id":      streamID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"content": fmt.Sprintf("[tool_call: %s] %s", e.Tool, e.Content),
					},
				},
			},
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			log.Error().Err(err).Msg("agent: failed to marshal tool_call event")
			return nil
		}
		return []byte(fmt.Sprintf("data: %s\n\n", data))

	case EventToolResult:
		chunk := map[string]any{
			"id":      streamID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"content": fmt.Sprintf("[tool_result: %s] %s", e.Tool, e.Content),
					},
				},
			},
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			log.Error().Err(err).Msg("agent: failed to marshal tool_result event")
			return nil
		}
		return []byte(fmt.Sprintf("data: %s\n\n", data))

	case EventDone:
		return []byte("data: [DONE]\n\n")

	case EventError:
		chunk := map[string]any{
			"error": map[string]any{
				"message": e.Content,
				"type":    "agent_error",
			},
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			log.Error().Err(err).Msg("agent: failed to marshal error event")
			return nil
		}
		return []byte(fmt.Sprintf("data: %s\n\n", data))

	default:
		return nil
	}
}
