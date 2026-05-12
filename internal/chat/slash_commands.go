package chat

import (
	"fmt"
	"sort"
	"strings"
)

// slashCommand describes one built-in TUI slash command.
type slashCommand struct {
	Name      string   // canonical name including leading "/"
	Aliases   []string // optional aliases (e.g. "?" for "/help")
	Summary   string   // one-line description shown in picker and /help
	TakesArgs bool     // true when the command accepts arguments
}

// slashCommands returns the static catalog of built-in slash commands.
// The order is the order in which they are presented in /help and as
// the initial picker order before any filter is applied.
func slashCommands() []slashCommand {
	return []slashCommand{
		{Name: "/help", Aliases: []string{"?"}, Summary: "show available commands"},
		{Name: "/new", TakesArgs: true, Summary: "start a new session [title]"},
		{Name: "/sessions", Summary: "browse and resume a previous session"},
		{Name: "/session", Summary: "show current session info"},
		{Name: "/mode", TakesArgs: true, Summary: "show or switch mode (chat|agent)"},
		{Name: "/apishape", Aliases: []string{"/api-shape"}, TakesArgs: true, Summary: "show or set the agent API request shape"},
		{Name: "/permissions", Summary: "toggle autopilot (auto-approve tool calls)"},
		{Name: "/model", TakesArgs: true, Summary: "show or switch model"},
		{Name: "/agent", TakesArgs: true, Summary: "show or set the agent backend"},
		{Name: "/max-turns", TakesArgs: true, Summary: "show or set max agent turns (1-100)"},
		{Name: "/models", Summary: "open model picker"},
		{Name: "/spec", TakesArgs: true, Summary: "spec-driven workflow: mode, init, status, help"},
		{Name: "/clear", Aliases: []string{"/cls"}, Summary: "clear the screen"},
		{Name: "/quit", Aliases: []string{"/exit"}, Summary: "quit"},
	}
}

// fuzzySlashFilter ranks the catalog against filter using the same
// scoring shape as fuzzyScore in tui.go: prefix > substring > subsequence.
// Aliases are considered. The leading "/" in filter is ignored so users
// who type "mo" get the same results as "/mo".
func fuzzySlashFilter(all []slashCommand, filter string) []slashCommand {
	q := strings.TrimSpace(strings.ToLower(filter))
	q = strings.TrimPrefix(q, "/")
	if q == "" {
		out := make([]slashCommand, len(all))
		copy(out, all)
		return out
	}

	type match struct {
		cmd   slashCommand
		score int
		order int
	}

	var matches []match
	for i, c := range all {
		best := 0
		matched := false
		candidates := append([]string{c.Name}, c.Aliases...)
		for _, name := range candidates {
			text := strings.TrimPrefix(strings.ToLower(name), "/")
			if score, ok := fuzzyScore(text, q); ok {
				matched = true
				if score > best {
					best = score
				}
			}
		}
		if matched {
			matches = append(matches, match{cmd: c, score: best, order: i})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].order < matches[j].order
	})

	out := make([]slashCommand, len(matches))
	for i, m := range matches {
		out[i] = m.cmd
	}
	return out
}

// renderSlashHelp produces the markdown body for the /help command.
func renderSlashHelp(cmds []slashCommand) string {
	var b strings.Builder
	b.WriteString("**Commands:**\n\n")
	for _, c := range cmds {
		names := c.Name
		if len(c.Aliases) > 0 {
			names = fmt.Sprintf("%s (%s)", c.Name, strings.Join(c.Aliases, ", "))
		}
		b.WriteString(fmt.Sprintf("- `%s` — %s\n", names, c.Summary))
	}
	b.WriteString("\n**Keyboard shortcuts:**\n\n")
	b.WriteString("- `/` — open the command picker; type to filter, ↑↓ to navigate, Enter to select, Esc to close\n")
	b.WriteString("- `Shift+Tab` — toggle autopilot (auto-approve tool calls)\n")
	b.WriteString("- `↑`/`↓` — focus expandable tool results (when input is empty)\n")
	b.WriteString("- `Space` — expand/collapse the focused tool result\n")
	b.WriteString("- `Esc` — cancel current running job\n")
	return b.String()
}
