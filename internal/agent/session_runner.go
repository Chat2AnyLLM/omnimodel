package agent

import (
	"context"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/tools"
)

// HistoryMessage is the minimal chat-session history shape needed by the native agent runtime.
type HistoryMessage struct {
	Role    string
	Content string
}

// RunTurn runs one interactive agent turn using the native internal/agent runtime.
func RunTurn(ctx context.Context, c Client, sessionID, model, backend, prompt string, history []HistoryMessage, checker tools.PermissionChecker) (*RunResult, error) {
	registry := tools.NewRegistry()
	registerDefaultTools(registry)
	registry.SetPermissionChecker(checker)

	memory := NewBufferMemory(64)
	seedHistory(memory, history, prompt)

	ag := NewAgent(registry, memory, 10, NewChatCompletionsDispatch(c, model))
	return ag.Run(ctx, sessionID, prompt)
}

// StreamTurn runs one interactive agent turn using streaming, emitting events on the returned channel.
// The caller must drain the channel until it is closed.
func StreamTurn(ctx context.Context, c Client, sessionID, model, backend, prompt string, history []HistoryMessage, checker tools.PermissionChecker) (<-chan Event, error) {
	registry := tools.NewRegistry()
	registerDefaultTools(registry)
	registry.SetPermissionChecker(checker)

	memory := NewBufferMemory(64)
	seedHistory(memory, history, prompt)

	ag := NewAgent(registry, memory, 10, NewChatCompletionsDispatch(c, model))
	return ag.Stream(ctx, sessionID, prompt)
}

func registerDefaultTools(registry *tools.Registry) {
	m := tools.NewManager()
	tools.RegisterCoreTools(m)
	for _, tool := range m.Registry().List() {
		registry.Register(tool)
	}
}

func seedHistory(memory Memory, history []HistoryMessage, currentPrompt string) {
	trimmedPrompt := strings.TrimSpace(currentPrompt)
	for i, msg := range history {
		role := strings.TrimSpace(msg.Role)
		content := msg.Content
		if i == len(history)-1 && role == "user" && strings.TrimSpace(content) == trimmedPrompt {
			continue
		}
		switch role {
		case "assistant":
			memory.Append(cif.CIFAssistantMessage{
				Role: "assistant",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: content},
				},
			})
		case "system":
			memory.Append(cif.CIFSystemMessage{Role: "system", Content: content})
		default:
			memory.Append(cif.CIFUserMessage{
				Role: "user",
				Content: []cif.CIFContentPart{
					cif.CIFTextPart{Type: "text", Text: content},
				},
			})
		}
	}
}
