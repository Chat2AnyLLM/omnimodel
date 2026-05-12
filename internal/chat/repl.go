package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	agentpkg "omnillm/internal/agent"
	"omnillm/internal/specdriven"
	toolspkg "omnillm/internal/tools"

	"github.com/chzyer/readline"
)

func RunREPL(cmd CommandContext, c Client, requestedModel, existingSession string, picker ModelPickerFunc) error {
	session, err := EnsureSession(cmd, c, existingSession, requestedModel)
	if err != nil {
		return err
	}
	session.IsTTY = false
	session.Picker = picker

	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	_, _ = fmt.Fprintln(out, "Type your message and press Enter. Use ↑↓ for history, Ctrl+R to search.")
	_, _ = fmt.Fprintln(out, "Use /help for commands, /models to browse models, /mode to switch chat modes, /apishape to select the agent request shape.")
	_, _ = fmt.Fprintln(out)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:      ChatPrompt("You", session.IsTTY),
		HistoryFile: os.TempDir() + "/omnillm-chat-history",
	})
	if err != nil {
		return fmt.Errorf("init readline: %w", err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt || err == io.EOF {
				_, _ = fmt.Fprintln(out, "Goodbye.")
				break
			}
			return fmt.Errorf("read input: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "?") && strings.TrimSpace(line) == "?" {
			printHelp(cmd.OutOrStdout())
			continue
		}

		if strings.HasPrefix(line, "/") {
			result, err := handleSlashCommand(cmd, c, &session, line)
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
				continue
			}
			if result.model != "" {
				session.Model = result.model
			}
			if result.apiShape != "" {
				session.APIShape = result.apiShape
			}
			if result.agentBackend != "" {
				session.AgentBackend = result.agentBackend
			}
			if result.exit {
				_, _ = fmt.Fprintln(out, "Goodbye.")
				break
			}
			if result.handled {
				if result.agentPrompt == "" {
					continue
				}
				line = result.agentPrompt
			}
		}

		if strings.HasPrefix(line, "!") {
			command, _ := strings.CutPrefix(line, "!")
			command = strings.TrimSpace(command)
			if command == "" {
				continue
			}
			result := toolspkg.RunShellCommand(context.Background(), command, 0)
			if result.IsError {
				_, _ = fmt.Fprintf(errOut, "%s\n", result.Output)
			} else {
				_, _ = fmt.Fprintln(out, result.Output)
			}
			continue
		}

		_, _ = fmt.Fprintln(out, ChatHeader("Assistant", session.IsTTY))

		var assistantContent string
		if session.Mode == "agent" {
			sawToolActivity := false
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			eventCh, err := StreamAgentTurnWithChecker(ctx, c, session.ID, session.Model, session.AgentBackend, session.APIShape, line, makeStdioPermissionChecker(cmd, &session), 25)
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: completion: %v\n", err)
				continue
			}
			for event := range eventCh {
				switch event.Type {
				case agentpkg.EventToken:
					assistantContent += event.Content
				case agentpkg.EventToolCall:
					sawToolActivity = true
					assistantContent = ""
					_, _ = fmt.Fprintf(out, "  [tool: %s]\n", event.Tool)
				case agentpkg.EventToolResult:
					sawToolActivity = true
					_, _ = fmt.Fprintf(out, "  [result: %s]\n", event.Content)
				case agentpkg.EventError:
					err = fmt.Errorf("%s", event.Content)
				}
			}
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: completion: %v\n", err)
				continue
			}
			if assistantContent != "" {
				if saveErr := PostMessage(c, session.ID, "assistant", assistantContent); saveErr != nil {
					_, _ = fmt.Fprintf(errOut, "Error: store assistant message: %v\n", saveErr)
				}
				_, _ = fmt.Fprintln(out, assistantContent)
				_, _ = fmt.Fprintln(out)
			} else if sawToolActivity {
				_, _ = fmt.Fprintln(out, "(agent completed with no text response)")
			}
			continue
		} else {
			if err := PostMessage(c, session.ID, "user", line); err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: store message: %v\n", err)
				continue
			}

			loadedSession, messages, err := LoadSessionMessages(c, session.ID)
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
				continue
			}
			if loadedSession.Model != "" {
				session.Model = loadedSession.Model
			}

			assistantContent, err = StreamCompletion(c, session.Model, messages, out, session.IsTTY)
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: completion: %v\n", err)
				continue
			}
			if assistantContent == "" {
				_, _ = fmt.Fprintln(out, "(no response)")
				continue
			}

			if err := PostMessage(c, session.ID, "assistant", assistantContent); err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: store assistant message: %v\n", err)
			}
		}

		if assistantContent == "" {
			_, _ = fmt.Fprintln(out, "(no response)")
		}
	}

	return nil
}

func makeStdioPermissionChecker(cmd CommandContext, session *SessionState) toolspkg.PermissionChecker {
	return func(ctx context.Context, req toolspkg.PermissionRequest) (bool, error) {
		if session != nil && session.Autopilot {
			return true, nil
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", agentpkg.EncodePermissionPrompt(req.ToolName, req.Arguments))
		var ans string
		if _, err := fmt.Fscan(cmd.InOrStdin(), &ans); err != nil {
			return false, err
		}
		return strings.EqualFold(strings.TrimSpace(ans), "y"), nil
	}
}

func makeStdioAskUser(cmd CommandContext) func(context.Context, string, []string) (string, error) {
	return func(ctx context.Context, question string, options []string) (string, error) {
		if len(options) > 0 {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s (%s): ", question, strings.Join(options, "/"))
		} else {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s: ", question)
		}
		reader := bufio.NewReader(cmd.InOrStdin())
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimSpace(answer), nil
	}
}

func RunAgentTurnWithChecker(ctx context.Context, c Client, sessionID, model, backend, apiShape, prompt string, checker toolspkg.PermissionChecker, maxTurns int) (string, error) {
	if err := PostMessage(c, sessionID, "user", prompt); err != nil {
		return "", fmt.Errorf("store message: %w", err)
	}

	_, messages, err := LoadSessionMessages(c, sessionID)
	if err != nil {
		return "", fmt.Errorf("load messages: %w", err)
	}

	history := make([]agentpkg.HistoryMessage, 0, len(messages))
	for _, msg := range messages {
		history = append(history, agentpkg.HistoryMessage{Role: msg.Role, Content: msg.Content})
	}

	result, err := agentpkg.RunTurn(ctx, c, sessionID, model, backend, apiShape, prompt, history, checker, nil, maxTurns)
	if err != nil {
		return "", err
	}
	if result == nil || result.Output == "" {
		return "", nil
	}

	if err := PostMessage(c, sessionID, "assistant", result.Output); err != nil {
		return "", fmt.Errorf("store assistant message: %w", err)
	}
	return result.Output, nil
}

func RunAgentTurn(c Client, sessionID, model, backend, apiShape, prompt string, cmd CommandContext) (string, error) {
	return RunAgentTurnWithChecker(context.Background(), c, sessionID, model, backend, apiShape, prompt, makeStdioPermissionChecker(cmd, nil), 25)
}

// StreamAgentTurnWithChecker runs one agent turn using streaming so that tool
// call progress is delivered incrementally. Events are emitted on the returned
// channel until it is closed.  The caller is responsible for saving the final
// assistant message.
func StreamAgentTurnWithChecker(ctx context.Context, c Client, sessionID, model, backend, apiShape, prompt string, checker toolspkg.PermissionChecker, maxTurns int) (<-chan agentpkg.Event, error) {
	if err := PostMessage(c, sessionID, "user", prompt); err != nil {
		return nil, fmt.Errorf("store message: %w", err)
	}

	_, messages, err := LoadSessionMessages(c, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}

	history := make([]agentpkg.HistoryMessage, 0, len(messages))
	for _, msg := range messages {
		history = append(history, agentpkg.HistoryMessage{Role: msg.Role, Content: msg.Content})
	}

	return agentpkg.StreamTurn(ctx, c, sessionID, model, backend, apiShape, prompt, history, checker, nil, maxTurns)
}

func supportedAgentBackends() []string {
	return []string{DefaultAgentBackend}
}

func supportedAgentBackendsText() string {
	return strings.Join(supportedAgentBackends(), ", ")
}

func isSupportedAgentBackend(backend string) bool {
	return slices.Contains(supportedAgentBackends(), backend)
}

func handleSlashCommand(cmd CommandContext, c Client, session *SessionState, line string) (replCommandResult, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return replCommandResult{handled: true}, nil
	}

	switch fields[0] {
	case "/quit", "/exit":
		return replCommandResult{handled: true, exit: true}, nil
	case "/clear", "/cls":
		clearScreen(cmd.OutOrStdout())
		return replCommandResult{handled: true}, nil
	case "/help", "?":
		printHelp(cmd.OutOrStdout())
		return replCommandResult{handled: true}, nil
	case "/mode":
		result := agentpkg.ParseCommand(line, session.Mode)
		if result.NewMode != nil {
			session.Mode = *result.NewMode
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), result.Response)
		return replCommandResult{handled: true}, nil
	case "/permissions":
		session.Autopilot = !session.Autopilot
		status := "manual approval"
		if session.Autopilot {
			status = "autopilot (tools auto-approved)"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Permissions: %s\n", status)
		return replCommandResult{handled: true}, nil
	case "/model":
		if len(fields) == 1 {
			currentModel, err := CurrentModel(c, session.ID, session.Model)
			if err != nil {
				return replCommandResult{}, err
			}
			if currentModel == "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Current model: (server default)")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Current model: %s\n", currentModel)
			}
			return replCommandResult{handled: true, model: currentModel}, nil
		}
		newModel := fields[1]
		if err := UpdateSessionModel(c, session.ID, newModel); err != nil {
			return replCommandResult{}, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Switched model to %s\n", newModel)
		return replCommandResult{handled: true, model: newModel}, nil
	case "/apishape", "/api-shape":
		if len(fields) == 1 {
			currentShape, err := CurrentAPIShape(c, session.ID, session.APIShape)
			if err != nil {
				return replCommandResult{}, err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Current API shape: %s\n", formatAPIShape(currentShape))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Supported shapes: %s\n", supportedAPIShapesText())
			return replCommandResult{handled: true, apiShape: currentShape}, nil
		}
		newShape, ok := normalizeAPIShape(fields[1])
		if !ok || newShape == "responses" {
			return replCommandResult{}, fmt.Errorf("unknown API shape %q; supported shapes: %s", fields[1], supportedAPIShapesText())
		}
		if err := UpdateSessionAPIShape(c, session.ID, newShape); err != nil {
			return replCommandResult{}, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Switched API shape to %s\n", formatAPIShape(newShape))
		return replCommandResult{handled: true, apiShape: newShape}, nil
	case "/agent":
		if len(fields) == 1 {
			currentBackend, err := CurrentAgentBackend(c, session.ID, session.AgentBackend)
			if err != nil {
				return replCommandResult{}, err
			}
			if currentBackend == "" {
				currentBackend = supportedAgentBackends()[0]
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Current agent backend: %s\n", currentBackend)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Supported backends: %s\n", supportedAgentBackendsText())
			return replCommandResult{handled: true, agentBackend: currentBackend}, nil
		}
		newBackend := fields[1]
		if !isSupportedAgentBackend(newBackend) {
			return replCommandResult{}, fmt.Errorf("unknown agent backend %q; supported backends: %s", newBackend, supportedAgentBackendsText())
		}
		if err := UpdateSessionAgentBackend(c, session.ID, newBackend); err != nil {
			return replCommandResult{}, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Switched agent backend to %s\n", newBackend)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Supported backends: %s\n", supportedAgentBackendsText())
		return replCommandResult{handled: true, agentBackend: newBackend}, nil
	case "/models":
		models, err := ListModels(c)
		if err != nil {
			return replCommandResult{}, err
		}
		if len(models) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No models available.")
			return replCommandResult{handled: true}, nil
		}
		filter := ""
		if len(fields) > 1 {
			filter = strings.Join(fields[1:], " ")
		}
		filtered := FilterModels(models, filter)
		if session.IsTTY && filter == "" && session.Picker != nil {
			selected, err := session.Picker("Select a model", models)
			if err != nil {
				return replCommandResult{handled: true}, nil
			}
			if selected == "" {
				return replCommandResult{handled: true}, nil
			}
			if err := UpdateSessionModel(c, session.ID, selected); err != nil {
				return replCommandResult{}, err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Switched model to %s\n", selected)
			return replCommandResult{handled: true, model: selected}, nil
		}
		if len(filtered) == 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No models match %q\n", filter)
			return replCommandResult{handled: true}, nil
		}
		for _, model := range filtered {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", model.Selector, model.Name)
		}
		return replCommandResult{handled: true}, nil
	case "/session":
		currentModel, err := CurrentModel(c, session.ID, session.Model)
		if err != nil {
			return replCommandResult{}, err
		}
		currentBackend, err := CurrentAgentBackend(c, session.ID, session.AgentBackend)
		if err != nil {
			return replCommandResult{}, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Session: %s\n", session.ID)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Mode:    %s\n", session.Mode)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Agent:   %s\n", currentBackend)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "API:     %s\n", formatAPIShape(session.APIShape))
		if currentModel == "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Model:   (server default)")
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Model:   %s\n", currentModel)
		}
		return replCommandResult{handled: true, model: currentModel, agentBackend: currentBackend}, nil
	default:
		if handled, err := handleDirectSpecCommand(cmd.OutOrStdout(), fields); handled {
			return replCommandResult{handled: true}, err
		}
		if handled, agentPrompt, err := handleSpecWorkflowSlashCommand(cmd.OutOrStdout(), fields, session); handled {
			return replCommandResult{handled: true, agentPrompt: agentPrompt}, err
		}
		return replCommandResult{}, fmt.Errorf("unknown command %q; use /help", fields[0])
	}
}

func printHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Interactive commands:")
	_, _ = fmt.Fprintln(w, "  /help              Show this help")
	_, _ = fmt.Fprintln(w, "  /session           Show current session details")
	_, _ = fmt.Fprintln(w, "  /mode              Show the current chat mode")
	_, _ = fmt.Fprintln(w, "  /mode <chat|agent> Switch between chat and agent modes")
	_, _ = fmt.Fprintln(w, "  /apishape          Show the agent API request shape")
	_, _ = fmt.Fprintln(w, "  /apishape <shape>  Switch agent API shape: anthropic or openai")
	_, _ = fmt.Fprintln(w, "  /permissions       Toggle autopilot (auto-approve tool calls)")
	_, _ = fmt.Fprintln(w, "  /model             Show the current model")
	_, _ = fmt.Fprintln(w, "  /model <id>        Switch to a different model")
	_, _ = fmt.Fprintln(w, "  /agent             Show the current agent backend and supported backends")
	_, _ = fmt.Fprintln(w, "  /agent <backend>   Keep agent backend on google-adk")
	_, _ = fmt.Fprintln(w, "  /models            Open the model selector in a terminal")
	_, _ = fmt.Fprintln(w, "  /models <filter>   List model selectors matching a filter")
	_, _ = fmt.Fprintln(w, "  /specify.init      Spec Kit: scaffold a new spec directory (offline)")
	_, _ = fmt.Fprintln(w, "  /speckit.status    Spec Kit: list specs and artifact status (offline)")
	_, _ = fmt.Fprintln(w, "  /speckit.help      Spec Kit: show workflow help")
	_, _ = fmt.Fprintln(w, "  /openspec:init <name>  OpenSpec: scaffold a new change directory (offline)")
	_, _ = fmt.Fprintln(w, "  /openspec:help     OpenSpec: show workflow help")
	_, _ = fmt.Fprintln(w, "  /speckit.* /openspec:* Run a Spec Kit / OpenSpec command via the agent")
	_, _ = fmt.Fprintln(w, "  /clear, /cls       Clear the screen")
	_, _ = fmt.Fprintln(w, "  /quit, /exit       Leave the chat shell")
}

func clearScreen(w io.Writer) {
	_, _ = fmt.Fprint(w, "\033[2J\033[H")
}

func specWorkflowAgentPrompt(cmdSlash, toolName, arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return fmt.Sprintf("load the spec skill and run %s for %s", toolName, cmdSlash)
	}
	return fmt.Sprintf("load the spec skill and run %s for %s with this intent: %s", toolName, cmdSlash, arg)
}

// handleDirectSpecCommand intercepts the offline-only spec scaffold commands
// (/specify.init, /opsx:init, /speckit.status) before agent routing. Returns
// (handled, err). Unhandled commands fall through to agent routing.
func handleDirectSpecCommand(w io.Writer, fields []string) (bool, error) {
	if len(fields) == 0 {
		return false, nil
	}
	name := strings.ToLower(fields[0])
	arg := strings.TrimSpace(strings.Join(fields[1:], " "))

	switch name {
	case "/specify.init":
		if arg == "" {
			_, _ = fmt.Fprintln(w, "Usage: /specify.init <title>")
			_, _ = fmt.Fprintln(w, "Example: /specify.init User Authentication")
			return true, nil
		}
		if err := specREPLInit(w, arg); err != nil {
			_, _ = fmt.Fprintf(w, "Error: %v\n", err)
		}
		return true, nil
	case "/openspec:init", "/opsx:init":
		if arg == "" {
			_, _ = fmt.Fprintln(w, "Usage: /openspec:init <change-name>")
			_, _ = fmt.Fprintln(w, "Example: /openspec:init add-user-auth")
			return true, nil
		}
		if err := openSpecREPLInit(w, arg); err != nil {
			_, _ = fmt.Fprintf(w, "Error: %v\n", err)
		}
		return true, nil
	case "/speckit.status":
		specsDir := "specs"
		if arg != "" {
			specsDir = arg
		}
		if err := specREPLStatus(w, specsDir); err != nil {
			_, _ = fmt.Fprintf(w, "Error: %v\n", err)
		}
		return true, nil
	case "/speckit.help":
		_, _ = fmt.Fprint(w, specKitHelpMarkdown())
		return true, nil
	case "/openspec:help", "/opsx:help":
		_, _ = fmt.Fprint(w, openSpecHelpMarkdown())
		return true, nil
	}
	return false, nil
}

func handleSpecWorkflowSlashCommand(w io.Writer, fields []string, session *SessionState) (bool, string, error) {
	if len(fields) == 0 {
		return false, "", nil
	}
	name := strings.ToLower(fields[0])
	// Backwards compat: accept the deprecated /opsx:* aliases by rewriting
	// them to their canonical /openspec:* counterparts before lookup.
	if strings.HasPrefix(name, "/opsx:") {
		name = "/openspec:" + strings.TrimPrefix(name, "/opsx:")
	}
	arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(strings.Join(fields, " "), fields[0])), " "))

	for _, cmd := range specdriven.SpecKitCommands() {
		if name == cmd.Slash {
			if session != nil {
				session.SpecMode = "spec-kit"
				session.Mode = "agent"
			}
			prompt := specWorkflowAgentPrompt(cmd.Slash, cmd.Tool, arg)
			_, _ = fmt.Fprintf(w, "Spec mode: spec-kit. Switched to agent mode.\n")
			_, _ = fmt.Fprintf(w, "Mapped %s -> %s\n", cmd.Slash, cmd.Tool)
			_, _ = fmt.Fprintf(w, "Running agent workflow: %s\n\n", prompt)
			return true, prompt, nil
		}
	}

	for _, cmd := range specdriven.OpenSpecCommands() {
		if name == cmd.Slash {
			if session != nil {
				session.SpecMode = "openspec"
				session.Mode = "agent"
			}
			prompt := specWorkflowAgentPrompt(cmd.Slash, cmd.Tool, arg)
			_, _ = fmt.Fprintf(w, "Spec mode: openspec. Switched to agent mode.\n")
			_, _ = fmt.Fprintf(w, "Mapped %s -> %s\n", cmd.Slash, cmd.Tool)
			_, _ = fmt.Fprintf(w, "Running agent workflow: %s\n\n", prompt)
			return true, prompt, nil
		}
	}

	return false, "", nil
}

// handleSpecCommand removed: the /spec, /spec mode, /spec init, /spec status,
// and /spec help shortcuts were retired in favour of /specify.init,
// /opsx:init, /speckit.status, and the existing /speckit.* and /opsx:*
// command families that route through the agent.

