package agent

import "strings"

// CommandResult holds the result of parsing a slash command.
type CommandResult struct {
	IsCommand bool
	Response  string  // system message to display
	NewMode   *string // non-nil if mode changed
}

// ParseCommand parses agent-related slash commands from user input.
// Returns a CommandResult indicating if the input was a recognized command.
func ParseCommand(input string, currentMode string) CommandResult {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/mode") {
		return CommandResult{IsCommand: false}
	}

	fields := strings.Fields(input)
	if len(fields) == 1 {
		// /mode with no argument — show current mode
		return CommandResult{
			IsCommand: true,
			Response:  "Current mode: " + currentMode,
		}
	}

	arg := strings.ToLower(fields[1])
	switch arg {
	case "agent":
		mode := "agent"
		return CommandResult{
			IsCommand: true,
			Response:  "Agent mode enabled",
			NewMode:   &mode,
		}
	case "chat":
		mode := "chat"
		return CommandResult{
			IsCommand: true,
			Response:  "Chat mode disabled",
			NewMode:   &mode,
		}
	default:
		return CommandResult{
			IsCommand: true,
			Response:  "Unknown mode: " + arg + ". Available modes: chat, agent",
		}
	}
}
