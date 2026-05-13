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

// slashCommandCatalog is the canonical built-in slash command list. The
// order is what /help and the empty-filter picker use.
var slashCommandCatalog = []slashCommand{
	{Name: "/help", Aliases: []string{"?"}, Summary: "show available commands"},
	{Name: "/new", TakesArgs: true, Summary: "start a new session [title]"},
	{Name: "/session", Aliases: []string{"/sessions"}, Summary: "show current session info or browse/resume sessions"},
	{Name: "/mode", TakesArgs: true, Summary: "show or switch mode (chat|agent)"},
	{Name: "/apishape", Aliases: []string{"/api-shape"}, TakesArgs: true, Summary: "show or set the agent API request shape"},
	{Name: "/permissions", Summary: "toggle autopilot (auto-approve tool calls)"},
	{Name: "/models", Summary: "open model picker to switch model"},
	{Name: "/agent", TakesArgs: true, Summary: "show or set the agent backend"},
	{Name: "/max-turns", TakesArgs: true, Summary: "show or set max agent turns (1-1000)"},
	{Name: "/specify.init", TakesArgs: true, Summary: "Spec Kit: scaffold a new spec directory [title] (offline)"},
	{Name: "/speckit.constitution", TakesArgs: true, Summary: "Spec Kit: create/update project constitution"},
	{Name: "/speckit.specify", TakesArgs: true, Summary: "Spec Kit: create/update feature spec"},
	{Name: "/speckit.clarify", TakesArgs: true, Summary: "Spec Kit: clarify open questions"},
	{Name: "/speckit.plan", TakesArgs: true, Summary: "Spec Kit: create implementation plan"},
	{Name: "/speckit.tasks", TakesArgs: true, Summary: "Spec Kit: generate tasks"},
	{Name: "/speckit.analyze", TakesArgs: true, Summary: "Spec Kit: analyze artifact consistency"},
	{Name: "/speckit.implement", TakesArgs: true, Summary: "Spec Kit: implement task plan"},
	{Name: "/speckit.taskstoissues", TakesArgs: true, Summary: "Spec Kit: convert tasks.md into GitHub issues (requires gh)"},
	{Name: "/speckit.lifecycle", TakesArgs: true, Summary: "Spec Kit: show lifecycle state and guidance"},
	{Name: "/speckit.complete", TakesArgs: true, Summary: "Spec Kit: mark a spec completed"},
	{Name: "/speckit.archive", TakesArgs: true, Summary: "Spec Kit: archive a completed spec"},
	{Name: "/speckit.checklist", TakesArgs: true, Summary: "Spec Kit: generate checklist"},
	{Name: "/speckit.status", TakesArgs: true, Summary: "Spec Kit: list specs and artifact status [dir] (offline)"},
	{Name: "/speckit.help", Summary: "Spec Kit: show workflow help and command list"},
	{Name: "/openspec:init", TakesArgs: true, Aliases: []string{"/opsx:init"}, Summary: "OpenSpec: scaffold a new change directory [name] (offline)"},
	{Name: "/openspec:propose", TakesArgs: true, Aliases: []string{"/opsx:propose"}, Summary: "OpenSpec: create a change and planning artifacts"},
	{Name: "/openspec:explore", TakesArgs: true, Aliases: []string{"/opsx:explore"}, Summary: "OpenSpec: explore ideas before a change"},
	{Name: "/openspec:apply", TakesArgs: true, Aliases: []string{"/opsx:apply"}, Summary: "OpenSpec: implement or report pending tasks"},
	{Name: "/openspec:sync", TakesArgs: true, Aliases: []string{"/opsx:sync"}, Summary: "OpenSpec: sync delta specs into main specs"},
	{Name: "/openspec:archive", TakesArgs: true, Aliases: []string{"/opsx:archive"}, Summary: "OpenSpec: archive a completed change"},
	{Name: "/openspec:new", TakesArgs: true, Aliases: []string{"/opsx:new"}, Summary: "OpenSpec: start a change scaffold"},
	{Name: "/openspec:continue", TakesArgs: true, Aliases: []string{"/opsx:continue"}, Summary: "OpenSpec: create next ready artifact"},
	{Name: "/openspec:ff", TakesArgs: true, Aliases: []string{"/opsx:ff"}, Summary: "OpenSpec: fast-forward planning artifacts"},
	{Name: "/openspec:verify", TakesArgs: true, Aliases: []string{"/opsx:verify"}, Summary: "OpenSpec: verify implementation against artifacts"},
	{Name: "/openspec:bulk-archive", TakesArgs: true, Aliases: []string{"/opsx:bulk-archive"}, Summary: "OpenSpec: archive multiple changes"},
	{Name: "/openspec:onboard", Aliases: []string{"/opsx:onboard"}, Summary: "OpenSpec: guided workflow tutorial"},
	{Name: "/openspec:help", Aliases: []string{"/opsx:help"}, Summary: "OpenSpec: show workflow help and command list"},
	{Name: "/clear", Aliases: []string{"/cls"}, Summary: "clear the screen"},
	{Name: "/quit", Aliases: []string{"/exit"}, Summary: "quit"},
}

// slashCommands returns the static catalog of built-in slash commands.
// The returned slice is a defensive copy so callers may sort/filter freely.
func slashCommands() []slashCommand {
	out := make([]slashCommand, len(slashCommandCatalog))
	copy(out, slashCommandCatalog)
	return out
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
