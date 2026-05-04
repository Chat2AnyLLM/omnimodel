package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

type grepTool struct{}

func Grep() Tool { return &grepTool{} }

func (t *grepTool) Name() string { return "grep" }

func (t *grepTool) Description() string {
	return "Search file contents for a literal string or regex. Returns matching lines with file path and line number."
}

func (t *grepTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern":          map[string]any{"type": "string", "description": "The string or regex pattern to search for."},
			"path":             map[string]any{"type": "string", "description": "Directory to search in (default: cwd)."},
			"glob":             map[string]any{"type": "string", "description": "Only search files matching this glob, e.g. \"*.go\"."},
			"case_insensitive": map[string]any{"type": "boolean", "description": "Case-insensitive search."},
			"max_results":      map[string]any{"type": "integer", "description": "Max matching lines to return (default 100)."},
		},
		"required": []string{"pattern"},
	}
}

func (t *grepTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Pattern         string `json:"pattern"`
		Path            string `json:"path"`
		Glob            string `json:"glob"`
		CaseInsensitive bool   `json:"case_insensitive"`
		MaxResults      int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Pattern == "" {
		return Result{Output: "error: pattern is required", IsError: true}
	}
	if p.MaxResults <= 0 {
		p.MaxResults = 100
	}

	base := p.Path
	if base == "" {
		base, _ = os.Getwd()
	}

	if rg, err := exec.LookPath("rg"); err == nil {
		args := []string{"--line-number", "--no-heading", "--color=never"}
		if p.CaseInsensitive {
			args = append(args, "--ignore-case")
		}
		if p.Glob != "" {
			args = append(args, "--glob", p.Glob)
		}
		args = append(args, p.Pattern, base)

		cmd := exec.CommandContext(ctx, rg, args...)
		out, err := cmd.Output()
		if err != nil {
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
				return Result{Output: "(no matches)"}
			}
			return Result{Output: "error: " + err.Error(), IsError: true}
		}
		lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
		if len(lines) > p.MaxResults {
			lines = lines[:p.MaxResults]
		}
		if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
			return Result{Output: "(no matches)"}
		}
		return Result{Output: strings.Join(lines, "\n")}
	}

	results, err := walkGrep(ctx, base, p.Pattern, p.Glob, p.CaseInsensitive, p.MaxResults)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{Output: results}
}
