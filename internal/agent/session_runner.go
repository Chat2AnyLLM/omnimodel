package agent

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"omnillm/internal/tools"
)

// HistoryMessage is the minimal chat-session history shape needed by the native agent runtime.
type HistoryMessage struct {
	Role    string
	Content string
}

// RunTurn runs one interactive agent turn using the native internal/agent runtime.
func RunTurn(ctx context.Context, c Client, sessionID, model, backend, apiShape, prompt string, history []HistoryMessage, checker tools.PermissionChecker, askUser func(context.Context, string, []string) (string, error), maxTurns int) (*RunResult, error) {
	registry := tools.NewRegistry()
	registerDefaultTools(registry)
	registry.SetPermissionChecker(checker)
	registry.SetAskUserCallback(askUser)
	registry.SendMessageFn = MakeSubAgentFn(SubAgentOptions{
		Client:   c,
		Model:    model,
		Backend:  backend,
		APIShape: apiShape,
		MaxTurns: maxTurns / 2,
		Checker:  checker,
		AskUser:  askUser,
	})
	wsDir := workspaceDir()
	_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] run_start model=%s prompt=%q", sessionID, model, trimForLog(prompt, 160)))

	// Scan user prompt for injection patterns before processing.
	safePrompt, injected := SanitizeUserInput(prompt)
	if injected {
		_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] prompt_injection_detected prompt=%q", sessionID, trimForLog(prompt, 80)))
	}

	memory := NewBufferMemory(64)
	sysPrompt := buildSystemPrompt()
	memory.Append(Message{Role: "system", Content: []ContentBlock{TextBlock(sysPrompt)}})
	if pc := LoadWorkspaceContext(wsDir); !pc.IsEmpty() {
		injectPersistentContext(memory, pc)
	}
	seedHistory(memory, history, safePrompt)

	ag := NewAgent(registry, memory, maxTurns, selectDispatch(c, model, backend, apiShape))
	result, err := ag.Run(ctx, sessionID, safePrompt)
	if err != nil {
		_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] run_error model=%s err=%q", sessionID, model, trimForLog(err.Error(), 200)))
		return nil, err
	}
	if result != nil {
		_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] run_done model=%s steps=%d output=%q", sessionID, model, result.Steps, trimForLog(result.Output, 200)))
	}
	return result, nil
}

// StreamTurn runs one interactive agent turn using streaming, emitting events on the returned channel.
// The caller must drain the channel until it is closed.
func StreamTurn(ctx context.Context, c Client, sessionID, model, backend, apiShape, prompt string, history []HistoryMessage, checker tools.PermissionChecker, askUser func(context.Context, string, []string) (string, error), maxTurns int) (<-chan Event, error) {
	registry := tools.NewRegistry()
	registerDefaultTools(registry)
	registry.SetPermissionChecker(checker)
	registry.SetAskUserCallback(askUser)
	registry.SendMessageFn = MakeSubAgentFn(SubAgentOptions{
		Client:   c,
		Model:    model,
		Backend:  backend,
		APIShape: apiShape,
		MaxTurns: maxTurns / 2,
		Checker:  checker,
		AskUser:  askUser,
	})
	wsDir := workspaceDir()
	_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] stream_start model=%s prompt=%q", sessionID, model, trimForLog(prompt, 160)))

	// Scan user prompt for injection patterns before processing.
	safePrompt, injected := SanitizeUserInput(prompt)
	if injected {
		_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] prompt_injection_detected prompt=%q", sessionID, trimForLog(prompt, 80)))
	}

	memory := NewBufferMemory(64)
	sysPrompt := buildSystemPrompt()
	memory.Append(Message{Role: "system", Content: []ContentBlock{TextBlock(sysPrompt)}})
	if pc := LoadWorkspaceContext(wsDir); !pc.IsEmpty() {
		injectPersistentContext(memory, pc)
	}
	seedHistory(memory, history, safePrompt)

	ag := NewAgent(registry, memory, maxTurns, selectDispatch(c, model, backend, apiShape))
	events, err := ag.Stream(ctx, sessionID, safePrompt)
	if err != nil {
		_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] stream_error model=%s err=%q", sessionID, model, trimForLog(err.Error(), 200)))
		return nil, err
	}
	wrapped := make(chan Event, 64)
	go func() {
		defer close(wrapped)
		for ev := range events {
			wrapped <- ev
			switch ev.Type {
			case EventDone:
				_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] stream_done model=%s", sessionID, model))
			case EventError:
				_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] stream_error model=%s err=%q", sessionID, model, trimForLog(ev.Content, 200)))
			}
		}
	}()
	return wrapped, nil
}

func trimForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func selectDispatch(c Client, model, _, apiShape string) DispatchFn {
	var base DispatchFn
	switch normalizeAPIShape(apiShape) {
	case "openai":
		base = OpenAISDKDispatch(omniLLMAPIKey(c), omniLLMOpenAIBaseURL(c), model)
	default:
		base = NewDispatch(c, model, DefaultAPIShape)
	}

	// Add transient retry behavior for interactive agent turns.
	return retryDispatch(base, 3, 500*time.Millisecond, 8*time.Second)
}

func omniLLMOpenAIBaseURL(c Client) string {
	config, ok := c.(OmniLLMClientConfig)
	if !ok {
		return ""
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.GetBaseURL()), "/")
	if baseURL == "" {
		return ""
	}
	return baseURL + "/v1"
}

func omniLLMAPIKey(c Client) string {
	config, ok := c.(OmniLLMClientConfig)
	if !ok {
		return ""
	}
	return strings.TrimSpace(config.GetAPIKey())
}

// buildSystemPrompt returns an OS-aware system prompt so the LLM generates
// shell commands appropriate for the host platform.
func buildSystemPrompt() string {
	osName := runtime.GOOS
	shellTool := "bash"
	shellSyntax := "bash/sh"
	if osName == "windows" {
		shellTool = "powershell"
		shellSyntax = "PowerShell"
	}
	return fmt.Sprintf(
		"You are a software engineering agent running inside OmniCode. The current operating system is %s. "+
			"Use the %q tool to execute shell commands — all commands must use %s syntax. "+
			"When presenting output for the OmniCode conversation UI, prefer structured sections so content reads cleanly in separate blocks. "+
			"When useful, organize content with short headings such as User message, Assistant thinking, and Assistant response. "+
			"When information is tabular or comparative, format it as a compact, readable markdown table whenever practical. "+
			"For actionable requests, prefer using tools to inspect, verify, and change the real environment instead of describing what you would do. "+
			"Use tools when the answer depends on the repository, filesystem, git state, runtime state, command output, or any fact you can check directly. "+
			"Before making concrete claims about code or files, verify them with tools when practical. "+
			"For conceptual or advisory requests that do not require inspection, answer directly without unnecessary tool use. "+
			"When a task is multi-step, gather the needed context with tools, take the next concrete action, and then report the result briefly. "+
			"Do not merely promise tool use — actually execute the relevant tools when they are needed.",
		osName, shellTool, shellSyntax,
	)
}

func registerDefaultTools(registry *tools.Registry) {
	m := tools.NewManager()
	tools.RegisterCoreTools(m)
	for _, tool := range m.Registry().List() {
		registry.Register(tool)
	}
	tools.InitRegistryStores(registry)
	tools.InitSkillMembership(registry)
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
			memory.Append(Message{Role: "assistant", Content: []ContentBlock{TextBlock(content)}})
		case "system":
			memory.Append(Message{Role: "system", Content: []ContentBlock{TextBlock(content)}})
		default:
			memory.Append(Message{Role: "user", Content: []ContentBlock{TextBlock(content)}})
		}
	}
}
