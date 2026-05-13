package agent

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"omnillm/internal/tools"
)

// HistoryMessage is the minimal chat-session history shape needed by the native agent runtime.
type HistoryMessage struct {
	Role    string
	Content string
}

// RunTurn runs one interactive agent turn using the native internal/agent runtime.
func RunTurn(ctx context.Context, c Client, sessionID, model, backend, apiShape, prompt string, history []HistoryMessage, checker tools.PermissionChecker, askUser func(context.Context, string, []string) (string, error), maxTurns int) (*RunResult, error) {
	osName := detectHostOS()
	sysPrompt := buildSystemPrompt(osName)

	registry := tools.NewRegistry()
	registerDefaultTools(registry)
	registry.SetPermissionChecker(checker)
	registry.SetAskUserCallback(askUser)
	orchestrator := SessionOrchestrator(sessionID, SubAgentOptions{
		Client:   c,
		Model:    model,
		Backend:  backend,
		APIShape: apiShape,
		MaxTurns: maxTurns / 2,
		Checker:  checker,
		AskUser:  askUser,
	})
	registry.SendMessageFn = orchestrator.SendMessage
	wsDir := workspaceDir()
	_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] run_start model=%s prompt=%q", sessionID, model, trimForLog(prompt, 160)))

	// Scan user prompt for injection patterns before processing.
	safePrompt, injected := SanitizeUserInput(prompt)
	if injected {
		_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] prompt_injection_detected prompt=%q", sessionID, trimForLog(prompt, 80)))
	}

	memory := NewBufferMemory(64)
	memory.Append(Message{Role: "system", Content: []ContentBlock{TextBlock(sysPrompt)}})
	if pc := LoadWorkspaceContext(wsDir); !pc.IsEmpty() {
		injectPersistentContext(memory, pc)
	}
	seedHistory(memory, history, safePrompt)

	ag := NewAgent(registry, memory, maxTurns, selectDispatch(c, model, backend, apiShape))
	log.Info().Str("session", sessionID).Str("model", model).Int("max_turns", maxTurns).Msg("agent: starting run")
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
	osName := detectHostOS()
	sysPrompt := buildSystemPrompt(osName)

	registry := tools.NewRegistry()
	registerDefaultTools(registry)
	registry.SetPermissionChecker(checker)
	registry.SetAskUserCallback(askUser)
	orchestrator := SessionOrchestrator(sessionID, SubAgentOptions{
		Client:   c,
		Model:    model,
		Backend:  backend,
		APIShape: apiShape,
		MaxTurns: maxTurns / 2,
		Checker:  checker,
		AskUser:  askUser,
	})
	registry.SendMessageFn = orchestrator.SendMessage
	wsDir := workspaceDir()
	_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] stream_start model=%s prompt=%q", sessionID, model, trimForLog(prompt, 160)))

	// Scan user prompt for injection patterns before processing.
	safePrompt, injected := SanitizeUserInput(prompt)
	if injected {
		_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] prompt_injection_detected prompt=%q", sessionID, trimForLog(prompt, 80)))
	}

	memory := NewBufferMemory(64)
	memory.Append(Message{Role: "system", Content: []ContentBlock{TextBlock(sysPrompt)}})
	if pc := LoadWorkspaceContext(wsDir); !pc.IsEmpty() {
		injectPersistentContext(memory, pc)
	}
	seedHistory(memory, history, safePrompt)

	ag := NewAgent(registry, memory, maxTurns, selectDispatch(c, model, backend, apiShape))
	log.Info().Str("session", sessionID).Str("model", model).Int("max_turns", maxTurns).Msg("agent: starting stream")
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
		base = modelOverrideDispatch(AnthropicSDKDispatch(omniLLMAPIKey(c), omniLLMAnthropicBaseURL(c)), model)
	}

	// Add transient retry behavior for interactive agent turns.
	return retryDispatch(base, 3, 500*time.Millisecond, 8*time.Second)
}

func modelOverrideDispatch(base DispatchFn, model string) DispatchFn {
	model = strings.TrimSpace(model)
	if model == "" {
		return base
	}
	return func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		request := cloneMessagesRequest(req)
		request.Model = model
		return base(ctx, request)
	}
}

func omniLLMAnthropicBaseURL(c Client) string {
	config, ok := c.(OmniLLMClientConfig)
	if !ok {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(config.GetBaseURL()), "/")
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

func detectHostOS() string {
	osName := strings.TrimSpace(runtime.GOOS)
	if osName == "" {
		return "linux"
	}
	return osName
}

// systemPromptCache memoizes the rendered system prompt per OS. The prompt
// is ~2KB and rendered on every agent turn; the inputs are stable per run, so
// the result is safe to cache for the lifetime of the process.
var systemPromptCache sync.Map // map[string]string

// buildSystemPrompt returns an OS-aware system prompt so the LLM generates
// shell commands appropriate for the host platform.
func buildSystemPrompt(osName string) string {
	osName = strings.TrimSpace(osName)
	if osName == "" {
		osName = detectHostOS()
	}
	if cached, ok := systemPromptCache.Load(osName); ok {
		return cached.(string)
	}
	prompt := renderSystemPrompt(osName)
	systemPromptCache.Store(osName, prompt)
	return prompt
}

func renderSystemPrompt(osName string) string {
	shellTool := "bash"
	shellSyntax := "bash/sh"
	wrongShellRule := "Do not use the powershell tool unless the OS is windows."
	if osName == "windows" {
		shellTool = "powershell"
		shellSyntax = "PowerShell"
		wrongShellRule = "Do not use the bash tool on Windows."
	}
	return fmt.Sprintf(
		"You are a software engineering agent running inside OmniCode. The current operating system is %s. "+
			"Before invoking any shell-related tool, first confirm the host OS (already provided above) and route accordingly: "+
			"on `windows`, the PowerShell tool is the FIRST CHOICE for shell commands; on `macos`/`darwin` or `linux`, the bash/shell tool is the FIRST CHOICE. "+
			"Use the %q tool to execute shell commands — all commands must use %s syntax. "+
			"%s "+
			"Never guess the OS or fall back to the wrong shell tool when one is missing — surface the missing-tool error to the user instead. "+
			"When presenting output for the OmniCode conversation UI, follow a modern Go TUI-friendly presentation style inspired by Bubble Tea, Lip Gloss, Bubbles, Glamour, and charmbracelet/x/ansi. "+
			"Prefer Markdown-first output with structured headings, bullets, fenced code blocks, and concise sections. "+
			"For workflows, tool output, task status, or summaries, use clean panel/card-style sections where helpful. "+
			"For multi-step work, present progress as streaming event blocks, such as `⠋ Running tests...` followed by `✓ Running tests...`. "+
			"When useful, organize content with short headings such as Current Task, Tool Output, Result, Next Steps, and Notes. "+
			"When information is tabular or comparative, format it as a compact, readable markdown table whenever practical. "+
			"Keep output copy/paste friendly; avoid raw ANSI spaghetti, pixel-perfect coordinate layouts, or overly decorative formatting. "+
			"For Go TUI implementation guidance, prefer Bubble Tea for the TUI framework, Lip Gloss for styling/layout, Bubbles for components, Glamour for Markdown rendering, and charmbracelet/x/ansi for ANSI helpers unless there is a specific reason not to. "+
			"Design OmniCode agent workflows around reactive state, streaming views, Markdown rendering, viewport abstractions, and async tool-event blocks. "+
			"For actionable requests, prefer using tools to inspect, verify, and change the real environment instead of describing what you would do. "+
			"Use tools when the answer depends on the repository, filesystem, git state, runtime state, command output, or any fact you can check directly. "+
			"Before making concrete claims about code or files, verify them with tools when practical. "+
			"For conceptual or advisory requests that do not require inspection, answer directly without unnecessary tool use. "+
			"When a task is multi-step, gather the needed context with tools, take the next concrete action, and then report the result briefly. "+
			"Do not merely promise tool use — actually execute the relevant tools when they are needed.",
		osName, shellTool, shellSyntax, wrongShellRule,
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
