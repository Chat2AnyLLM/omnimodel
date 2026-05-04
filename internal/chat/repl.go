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
	_, _ = fmt.Fprintln(out, "Use /help for commands, /models to browse models, /mode to switch chat modes, /agent to manage backends.")
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

		if strings.HasPrefix(line, "/") {
			result, err := handleSlashCommand(cmd, c, &session, line)
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: %v\n", err)
				continue
			}
			if result.model != "" {
				session.Model = result.model
			}
			if result.agentBackend != "" {
				session.AgentBackend = result.agentBackend
			}
			if result.exit {
				_, _ = fmt.Fprintln(out, "Goodbye.")
				break
			}
			if result.handled {
				continue
			}
		}

		_, _ = fmt.Fprintln(out, ChatHeader("Assistant", session.IsTTY))

		var assistantContent string
		if session.Mode == "agent" {
			eventCh, err := StreamAgentTurnWithChecker(context.Background(), c, session.ID, session.Model, session.AgentBackend, line, makeStdioPermissionChecker(cmd))
			if err != nil {
				_, _ = fmt.Fprintf(errOut, "Error: completion: %v\n", err)
				continue
			}
			for event := range eventCh {
				switch event.Type {
				case agentpkg.EventToken:
					assistantContent += event.Content
				case agentpkg.EventToolCall:
					_, _ = fmt.Fprintf(out, "  [tool: %s]\n", event.Tool)
				case agentpkg.EventToolResult:
					result := event.Content
					if len(result) > 200 {
						result = result[:200] + "..."
					}
					_, _ = fmt.Fprintf(out, "  [result: %s]\n", result)
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
			}
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

func makeStdioPermissionChecker(cmd CommandContext) toolspkg.PermissionChecker {
	return func(ctx context.Context, req toolspkg.PermissionRequest) (bool, error) {
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

func RunAgentTurnWithChecker(ctx context.Context, c Client, sessionID, model, backend, prompt string, checker toolspkg.PermissionChecker) (string, error) {
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

	result, err := agentpkg.RunTurn(ctx, c, sessionID, model, backend, prompt, history, checker, nil)
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

func RunAgentTurn(c Client, sessionID, model, backend, prompt string, cmd CommandContext) (string, error) {
	return RunAgentTurnWithChecker(context.Background(), c, sessionID, model, backend, prompt, makeStdioPermissionChecker(cmd))
}

// StreamAgentTurnWithChecker runs one agent turn using streaming so that tool
// call progress is delivered incrementally. Events are emitted on the returned
// channel until it is closed.  The caller is responsible for saving the final
// assistant message.
func StreamAgentTurnWithChecker(ctx context.Context, c Client, sessionID, model, backend, prompt string, checker toolspkg.PermissionChecker) (<-chan agentpkg.Event, error) {
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

	return agentpkg.StreamTurn(ctx, c, sessionID, model, backend, prompt, history, checker, nil)
}

func supportedAgentBackends() []string {
	return []string{"agent-sdk-go", "google-adk", "anthropic-sdk"}
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
	case "/help":
		printHelp(cmd.OutOrStdout())
		return replCommandResult{handled: true}, nil
	case "/mode":
		result := agentpkg.ParseCommand(line, session.Mode)
		if result.NewMode != nil {
			session.Mode = *result.NewMode
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), result.Response)
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
		if currentModel == "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Model:   (server default)")
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Model:   %s\n", currentModel)
		}
		return replCommandResult{handled: true, model: currentModel, agentBackend: currentBackend}, nil
	default:
		return replCommandResult{}, fmt.Errorf("unknown command %q; use /help", fields[0])
	}
}

func printHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Interactive commands:")
	_, _ = fmt.Fprintln(w, "  /help              Show this help")
	_, _ = fmt.Fprintln(w, "  /session           Show current session details")
	_, _ = fmt.Fprintln(w, "  /mode              Show the current chat mode")
	_, _ = fmt.Fprintln(w, "  /mode <chat|agent> Switch between chat and agent modes")
	_, _ = fmt.Fprintln(w, "  /model             Show the current model")
	_, _ = fmt.Fprintln(w, "  /model <id>        Switch to a different model")
	_, _ = fmt.Fprintln(w, "  /agent             Show the current agent backend and supported backends")
	_, _ = fmt.Fprintln(w, "  /agent <backend>   Switch agent backend (agent-sdk-go, google-adk, or anthropic-sdk)")
	_, _ = fmt.Fprintln(w, "  /models            Open the model selector in a terminal")
	_, _ = fmt.Fprintln(w, "  /models <filter>   List model selectors matching a filter")
	_, _ = fmt.Fprintln(w, "  /clear, /cls       Clear the screen")
	_, _ = fmt.Fprintln(w, "  /quit, /exit       Leave the chat shell")
}

func clearScreen(w io.Writer) {
	_, _ = fmt.Fprint(w, "\033[2J\033[H")
}
