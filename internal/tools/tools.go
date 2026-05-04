package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// ─── bash ─────────────────────────────────────────────────────────────────────

type bashTool struct{}

func Bash() Tool { return &bashTool{} }

func (t *bashTool) Name() string        { return "bash" }
func (t *bashTool) Description() string { return "Execute a shell command and return its combined stdout+stderr output." }
func (t *bashTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":           map[string]any{"type": "string", "description": "The shell command to execute."},
			"description":       map[string]any{"type": "string", "description": "Short description of what the command does."},
			"timeout_seconds":   map[string]any{"type": "integer", "description": "Optional timeout in seconds (default 30)."},
			"run_in_background": map[string]any{"type": "boolean", "description": "Run the command in the background."},
		},
		"required": []string{"command"},
	}
}

func (t *bashTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Command         string `json:"command"`
		TimeoutSeconds  int    `json:"timeout_seconds"`
		RunInBackground bool   `json:"run_in_background"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.Command = strings.TrimSpace(p.Command)
	if p.Command == "" {
		return Result{Output: "error: command is required", IsError: true}
	}

	timeout := defaultTimeout
	if p.TimeoutSeconds > 0 {
		timeout = time.Duration(p.TimeoutSeconds) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", p.Command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-lc", p.Command)
	}

	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return Result{Output: "error: " + err.Error(), IsError: true}
		}
		return Result{Output: text + "\n(error: " + err.Error() + ")", IsError: true}
	}
	if text == "" {
		return Result{Output: "(no output)"}
	}
	return Result{Output: text}
}

// ─── read ─────────────────────────────────────────────────────────────────────

type readTool struct{}

func Read() Tool { return &readTool{} }

func (t *readTool) Name() string { return "read" }
func (t *readTool) Description() string {
	return "Read a file from the local filesystem. Use offset/limit for line-range slicing (line numbers are prefixed)."
}
func (t *readTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "Absolute or relative path to the file."},
			"offset":    map[string]any{"type": "integer", "description": "Line number to start reading from (1-based)."},
			"limit":     map[string]any{"type": "integer", "description": "Maximum number of lines to read."},
		},
		"required": []string{"file_path"},
	}
}

func (t *readTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		FilePath string `json:"file_path"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.FilePath == "" {
		return Result{Output: "error: file_path is required", IsError: true}
	}

	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}

	if p.Offset == 0 && p.Limit == 0 {
		return Result{Output: string(data)}
	}

	lines := strings.Split(string(data), "\n")
	start := 0
	if p.Offset > 0 {
		start = p.Offset - 1
	}
	if start >= len(lines) {
		return Result{Output: ""}
	}
	end := len(lines)
	if p.Limit > 0 && start+p.Limit < end {
		end = start + p.Limit
	}

	var buf strings.Builder
	for i, line := range lines[start:end] {
		fmt.Fprintf(&buf, "%d\t%s\n", start+i+1, line)
	}
	return Result{Output: buf.String()}
}

// ─── write ────────────────────────────────────────────────────────────────────

type writeTool struct{}

func Write() Tool { return &writeTool{} }

func (t *writeTool) Name() string        { return "write" }
func (t *writeTool) Description() string { return "Write content to a file, creating or overwriting it. Creates parent directories as needed." }
func (t *writeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "Absolute or relative path to the file."},
			"content":   map[string]any{"type": "string", "description": "The full content to write."},
		},
		"required": []string{"file_path", "content"},
	}
}

func (t *writeTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.FilePath == "" {
		return Result{Output: "error: file_path is required", IsError: true}
	}

	if err := os.MkdirAll(filepath.Dir(p.FilePath), 0o755); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if err := os.WriteFile(p.FilePath, []byte(p.Content), 0o644); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(p.Content), p.FilePath)}
}

// ─── edit ─────────────────────────────────────────────────────────────────────

type editTool struct{}

func Edit() Tool { return &editTool{} }

func (t *editTool) Name() string { return "edit" }
func (t *editTool) Description() string {
	return "Perform an exact string replacement in a file. old_string must match exactly (including whitespace). Set replace_all to replace every occurrence."
}
func (t *editTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path":   map[string]any{"type": "string", "description": "Absolute or relative path to the file."},
			"old_string":  map[string]any{"type": "string", "description": "The exact text to find and replace."},
			"new_string":  map[string]any{"type": "string", "description": "The replacement text."},
			"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence (default false)."},
		},
		"required": []string{"file_path", "old_string", "new_string"},
	}
}

func (t *editTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.FilePath == "" {
		return Result{Output: "error: file_path is required", IsError: true}
	}

	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	content := string(data)

	if !strings.Contains(content, p.OldString) {
		return Result{Output: fmt.Sprintf("error: old_string not found in %s", p.FilePath), IsError: true}
	}

	count := strings.Count(content, p.OldString)
	var updated string
	if p.ReplaceAll {
		updated = strings.ReplaceAll(content, p.OldString, p.NewString)
	} else {
		updated = strings.Replace(content, p.OldString, p.NewString, 1)
		count = 1
	}

	if err := os.WriteFile(p.FilePath, []byte(updated), 0o644); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{Output: fmt.Sprintf("replaced %d occurrence(s) in %s", count, p.FilePath)}
}

// ─── glob ─────────────────────────────────────────────────────────────────────

type globTool struct{}

func Glob() Tool { return &globTool{} }

func (t *globTool) Name() string        { return "glob" }
func (t *globTool) Description() string { return "Find files matching a glob pattern (e.g. **/*.go). Returns matching paths sorted by modification time." }
func (t *globTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern, e.g. \"src/**/*.ts\" or \"*.go\"."},
			"path":    map[string]any{"type": "string", "description": "Base directory to search in (default: cwd)."},
		},
		"required": []string{"pattern"},
	}
}

func (t *globTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Pattern == "" {
		return Result{Output: "error: pattern is required", IsError: true}
	}

	base := p.Path
	if base == "" {
		base, _ = os.Getwd()
	}

	matches, err := walkGlob(base, p.Pattern)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if len(matches) == 0 {
		return Result{Output: "(no matches)"}
	}
	return Result{Output: strings.Join(matches, "\n")}
}

// ─── grep ─────────────────────────────────────────────────────────────────────

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

	// Prefer ripgrep if available.
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

	// Fallback: pure-Go walker.
	results, err := walkGrep(ctx, base, p.Pattern, p.Glob, p.CaseInsensitive, p.MaxResults)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{Output: results}
}

// ─── ls ───────────────────────────────────────────────────────────────────────

type lsTool struct{}

func LS() Tool { return &lsTool{} }

func (t *lsTool) Name() string        { return "ls" }
func (t *lsTool) Description() string { return "List files and directories at a given path." }
func (t *lsTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory path to list (default: cwd)."},
		},
	}
}

func (t *lsTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}

	dir := p.Path
	if dir == "" {
		dir, _ = os.Getwd()
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if len(entries) == 0 {
		return Result{Output: "(empty directory)"}
	}

	var buf strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			fmt.Fprintf(&buf, "%s/\n", e.Name())
		} else {
			info, _ := e.Info()
			if info != nil {
				fmt.Fprintf(&buf, "%s (%d bytes)\n", e.Name(), info.Size())
			} else {
				fmt.Fprintf(&buf, "%s\n", e.Name())
			}
		}
	}
	return Result{Output: strings.TrimRight(buf.String(), "\n")}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func walkGlob(base, pattern string) ([]string, error) {
	pattern = filepath.FromSlash(pattern)
	var matches []string
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(base, path)
		if relErr != nil {
			return nil
		}
		ok, _ := filepath.Match(pattern, rel)
		if !ok && strings.Contains(pattern, "**") {
			idx := strings.LastIndex(pattern, "**/")
			if idx >= 0 {
				suffix := pattern[idx+3:]
				ok, _ = filepath.Match(suffix, filepath.Base(rel))
			}
		}
		if ok {
			matches = append(matches, rel)
		}
		return nil
	})
	return matches, err
}

func walkGrep(ctx context.Context, base, pattern, glob string, ci bool, max int) (string, error) {
	needle := pattern
	if ci {
		needle = strings.ToLower(pattern)
	}

	var results []string
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if glob != "" {
			matched, _ := filepath.Match(glob, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			haystack := line
			if ci {
				haystack = strings.ToLower(line)
			}
			if strings.Contains(haystack, needle) {
				rel, _ := filepath.Rel(base, path)
				results = append(results, fmt.Sprintf("%s:%d: %s", rel, lineNo, line))
				if len(results) >= max {
					return fmt.Errorf("stop")
				}
			}
		}
		return nil
	})

	if err != nil && err.Error() != "stop" {
		return "", err
	}
	if len(results) == 0 {
		return "(no matches)", nil
	}
	return strings.Join(results, "\n"), nil
}
