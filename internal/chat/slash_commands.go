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
		{Name: "/specify.init", TakesArgs: true, Summary: "Spec Kit: scaffold a new spec directory [title] (offline)"},
		{Name: "/speckit.constitution", TakesArgs: true, Summary: "Spec Kit: create/update project constitution"},
		{Name: "/speckit.specify", TakesArgs: true, Summary: "Spec Kit: create/update feature spec"},
		{Name: "/speckit.clarify", TakesArgs: true, Summary: "Spec Kit: clarify open questions"},
		{Name: "/speckit.plan", TakesArgs: true, Summary: "Spec Kit: create implementation plan"},
		{Name: "/speckit.tasks", TakesArgs: true, Summary: "Spec Kit: generate tasks"},
		{Name: "/speckit.analyze", TakesArgs: true, Summary: "Spec Kit: analyze artifact consistency"},
		{Name: "/speckit.implement", TakesArgs: true, Summary: "Spec Kit: implement task plan"},
		{Name: "/speckit.lifecycle", TakesArgs: true, Summary: "Spec Kit: show lifecycle state and guidance"},
		{Name: "/speckit.complete", TakesArgs: true, Summary: "Spec Kit: mark a spec completed"},
		{Name: "/speckit.archive", TakesArgs: true, Summary: "Spec Kit: archive a completed spec"},
		{Name: "/speckit.checklist", TakesArgs: true, Summary: "Spec Kit: generate checklist"},
		{Name: "/speckit.status", TakesArgs: true, Summary: "Spec Kit: list specs and artifact status [dir] (offline)"},
		{Name: "/opsx:init", TakesArgs: true, Summary: "OpenSpec: scaffold a new change directory [name] (offline)"},
		{Name: "/opsx:propose", TakesArgs: true, Summary: "OpenSpec: create a change and planning artifacts"},
		{Name: "/opsx:explore", TakesArgs: true, Summary: "OpenSpec: explore ideas before a change"},
		{Name: "/opsx:apply", TakesArgs: true, Summary: "OpenSpec: implement or report pending tasks"},
		{Name: "/opsx:sync", TakesArgs: true, Summary: "OpenSpec: sync delta specs into main specs"},
		{Name: "/opsx:archive", TakesArgs: true, Summary: "OpenSpec: archive a completed change"},
		{Name: "/opsx:new", TakesArgs: true, Summary: "OpenSpec: start a change scaffold"},
		{Name: "/opsx:continue", TakesArgs: true, Summary: "OpenSpec: create next ready artifact"},
		{Name: "/opsx:ff", TakesArgs: true, Summary: "OpenSpec: fast-forward planning artifacts"},
		{Name: "/opsx:verify", TakesArgs: true, Summary: "OpenSpec: verify implementation against artifacts"},
		{Name: "/opsx:bulk-archive", TakesArgs: true, Summary: "OpenSpec: archive multiple changes"},
		{Name: "/opsx:onboard", Summary: "OpenSpec: guided workflow tutorial"},
		{Name: "/openspec:proposal", TakesArgs: true, Summary: "OpenSpec legacy: create all artifacts"},
		{Name: "/openspec:apply", TakesArgs: true, Summary: "OpenSpec legacy: apply change"},
		{Name: "/openspec:archive", TakesArgs: true, Summary: "OpenSpec legacy: archive change"},
		{Name: "/clear", Aliases: []string{"/cls"}, Summary: "clear the screen"},
		{Name: "/quit", Aliases: []string{"/exit"}, Summary: "quit"},
	}
}

// fuzzySlashFilter ranks the catalog against filter using the same
// scoring shape as fuzzyScore in tui.go: prefix > substring > subsequence.
// Aliases are considered. The leading "/" in filter is ignored so users
// who type "mo" get the same results as "/mo".
func fuzzySlashFilter(all []slashCommand, filter string) []slashCommand {
	raw := strings.ToLower(filter)
	q := strings.TrimSpace(raw)
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
			queries := []string{q}
			if strings.Contains(q, " ") {
				queries = append(queries, strings.Join(strings.Fields(q), ""))
			}
			for _, query := range queries {
				if score, ok := fuzzyScore(text, query); ok {
					matched = true
					if score > best {
						best = score
					}
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
	return b.String()
}
