package agent

import (
	"context"
	"fmt"
	"runtime"
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
func RunTurn(ctx context.Context, c Client, sessionID, model, backend, prompt string, history []HistoryMessage, checker tools.PermissionChecker, askUser func(context.Context, string, []string) (string, error)) (*RunResult, error) {
	registry := tools.NewRegistry()
	registerDefaultTools(registry)
	registry.SetPermissionChecker(checker)
	registry.SetAskUserCallback(askUser)

	memory := NewBufferMemory(64)
	sysPrompt := buildSystemPrompt()
	memory.Append(cif.CIFSystemMessage{Role: "system", Content: sysPrompt})
	seedHistory(memory, history, prompt)

	ag := NewAgent(registry, memory, 10, selectDispatch(c, model, backend))
	return ag.Run(ctx, sessionID, prompt)
}

// StreamTurn runs one interactive agent turn using streaming, emitting events on the returned channel.
// The caller must drain the channel until it is closed.
func StreamTurn(ctx context.Context, c Client, sessionID, model, backend, prompt string, history []HistoryMessage, checker tools.PermissionChecker, askUser func(context.Context, string, []string) (string, error)) (<-chan Event, error) {
	registry := tools.NewRegistry()
	registerDefaultTools(registry)
	registry.SetPermissionChecker(checker)
	registry.SetAskUserCallback(askUser)

	memory := NewBufferMemory(64)
	sysPrompt := buildSystemPrompt()
	memory.Append(cif.CIFSystemMessage{Role: "system", Content: sysPrompt})
	seedHistory(memory, history, prompt)

	ag := NewAgent(registry, memory, 10, selectDispatch(c, model, backend))
	return ag.Stream(ctx, sessionID, prompt)
}

// selectDispatch picks the right DispatchFn based on the requested backend.
//
// All three backends — "agent-sdk-go", "google-adk", and "anthropic-sdk" —
// route through OmniLLM's local /v1/messages proxy.  OmniLLM translates the
// request to the appropriate upstream format for each provider.
//
// AnthropicSDKDispatch (direct SDK connection) is available as an explicit
// opt-in via code; it is not activated by the backend selector.
func selectDispatch(c Client, model, _ string) DispatchFn {
	return NewChatCompletionsDispatch(c, model)
}

// buildSystemPrompt returns an OS-aware system prompt so the LLM generates
// shell commands appropriate for the host platform.
func buildSystemPrompt() string {
	os := runtime.GOOS
	shell := "bash/sh"
	if os == "windows" {
		shell = "PowerShell"
	}
	return fmt.Sprintf(
		"You are a helpful agent. The current operating system is %s. "+
			"When writing shell commands for the bash tool, always use %s syntax. "+
			"IMPORTANT: Use the available tools to accomplish the task. Do not just describe what you will do — actually execute the tool calls.",
		os, shell,
	)
}

func registerDefaultTools(registry *tools.Registry) {
	m := tools.NewManager()
	tools.RegisterCoreTools(m)
	for _, tool := range m.Registry().List() {
		registry.Register(tool)
	}
	tools.InitRegistryStores(registry)
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
