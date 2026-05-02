package chat

import (
	"fmt"
	"io"
	"os"
	"strings"

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
	_, _ = fmt.Fprintln(out, "Use /help for commands, /models to browse models.")
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
			if result.exit {
				_, _ = fmt.Fprintln(out, "Goodbye.")
				break
			}
			if result.handled {
				continue
			}
		}

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

		_, _ = fmt.Fprintln(out, ChatHeader("Assistant", session.IsTTY))

		assistantContent, err := StreamCompletion(c, session.Model, messages, out, session.IsTTY)
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

	return nil
}

func handleSlashCommand(cmd CommandContext, c Client, session *SessionState, line string) (replCommandResult, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return replCommandResult{handled: true}, nil
	}

	switch fields[0] {
	case "/quit", "/exit":
		return replCommandResult{handled: true, exit: true}, nil
	case "/help":
		printHelp(cmd.OutOrStdout())
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
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Session: %s\n", session.ID)
		if currentModel == "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Model:   (server default)")
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Model:   %s\n", currentModel)
		}
		return replCommandResult{handled: true, model: currentModel}, nil
	default:
		return replCommandResult{}, fmt.Errorf("unknown command %q; use /help", fields[0])
	}
}

func printHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Interactive commands:")
	_, _ = fmt.Fprintln(w, "  /help              Show this help")
	_, _ = fmt.Fprintln(w, "  /session           Show current session details")
	_, _ = fmt.Fprintln(w, "  /model             Show the current model")
	_, _ = fmt.Fprintln(w, "  /model <id>        Switch to a different model")
	_, _ = fmt.Fprintln(w, "  /models            Open the model selector in a terminal")
	_, _ = fmt.Fprintln(w, "  /models <filter>   List model selectors matching a filter")
	_, _ = fmt.Fprintln(w, "  /quit, /exit       Leave the chat shell")
}
