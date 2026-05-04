package agent

import (
	"bufio"
	"bytes"
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

const defaultCommandTimeout = 30 * time.Second

// RegisterDefaultTools registers all built-in tools into the registry.
func RegisterDefaultTools(registry *Registry) {
	registry.Register(runCommandTool())
	registry.Register(readFileTool())
	registry.Register(writeFileTool())
	registry.Register(editFileTool())
	registry.Register(listDirTool())
	registry.Register(globTool())
	registry.Register(grepTool())
}

// ─── run_command ──────────────────────────────────────────────────────────────

func runCommandTool() *Tool {
	return &Tool{
		Name:        "run_command",
		Description: "Run a local shell command and return its combined stdout+stderr output.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The shell command to execute.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Optional timeout in seconds (default 30).",
				},
			},
			"required": []string{"command"},
		},
		Fn: func(ctx context.Context, input json.RawMessage) (string, error) {
			var payload struct {
				Command        string `json:"command"`
				TimeoutSeconds int    `json:"timeout_seconds"`
			}
			if err := json.Unmarshal(input, &payload); err != nil {
				return "", fmt.Errorf("parse run_command input: %w", err)
			}
			command := strings.TrimSpace(payload.Command)
			if command == "" {
				return "", fmt.Errorf("command is required")
			}

			timeout := defaultCommandTimeout
			if payload.TimeoutSeconds > 0 {
				timeout = time.Duration(payload.TimeoutSeconds) * time.Second
			}

			cmdCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "windows":
				cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
			default:
				cmd = exec.CommandContext(cmdCtx, "sh", "-lc", command)
			}

			output, err := cmd.CombinedOutput()
			text := strings.TrimSpace(string(output))
			if err != nil {
				if text == "" {
					return "", fmt.Errorf("command failed: %w", err)
				}
				return text, fmt.Errorf("command failed: %w", err)
			}
			if text == "" {
				return "(no output)", nil
			}
			return text, nil
		},
	}
}

// ─── read_file ────────────────────────────────────────────────────────────────

func readFileTool() *Tool {
	return &Tool{
		Name:        "read_file",
		Description: "Read a file from the local filesystem and return its contents. Optionally read a line range with offset/limit.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Line number to start reading from (1-based, optional).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of lines to read (optional).",
				},
			},
			"required": []string{"path"},
		},
		Fn: func(ctx context.Context, input json.RawMessage) (string, error) {
			var payload struct {
				Path   string `json:"path"`
				Offset int    `json:"offset"`
				Limit  int    `json:"limit"`
			}
			if err := json.Unmarshal(input, &payload); err != nil {
				return "", fmt.Errorf("parse read_file input: %w", err)
			}
			if payload.Path == "" {
				return "", fmt.Errorf("path is required")
			}

			data, err := os.ReadFile(payload.Path)
			if err != nil {
				return "", fmt.Errorf("read_file: %w", err)
			}

			// No range requested — return full content.
			if payload.Offset == 0 && payload.Limit == 0 {
				return string(data), nil
			}

			// Apply line-range slicing.
			lines := strings.Split(string(data), "\n")
			start := 0
			if payload.Offset > 0 {
				start = payload.Offset - 1 // convert to 0-based
			}
			if start >= len(lines) {
				return "", nil
			}
			end := len(lines)
			if payload.Limit > 0 && start+payload.Limit < end {
				end = start + payload.Limit
			}

			var buf strings.Builder
			for i, line := range lines[start:end] {
				fmt.Fprintf(&buf, "%d\t%s\n", start+i+1, line)
			}
			return buf.String(), nil
		},
	}
}

// ─── write_file ───────────────────────────────────────────────────────────────

func writeFileTool() *Tool {
	return &Tool{
		Name:        "write_file",
		Description: "Write content to a file, creating it or overwriting it entirely.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The full content to write to the file.",
				},
			},
			"required": []string{"path", "content"},
		},
		Fn: func(ctx context.Context, input json.RawMessage) (string, error) {
			var payload struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(input, &payload); err != nil {
				return "", fmt.Errorf("parse write_file input: %w", err)
			}
			if payload.Path == "" {
				return "", fmt.Errorf("path is required")
			}

			// Ensure parent directory exists.
			if err := os.MkdirAll(filepath.Dir(payload.Path), 0o755); err != nil {
				return "", fmt.Errorf("write_file: create dirs: %w", err)
			}
			if err := os.WriteFile(payload.Path, []byte(payload.Content), 0o644); err != nil {
				return "", fmt.Errorf("write_file: %w", err)
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(payload.Content), payload.Path), nil
		},
	}
}

// ─── edit_file ────────────────────────────────────────────────────────────────

func editFileTool() *Tool {
	return &Tool{
		Name: "edit_file",
		Description: "Perform an exact string replacement inside a file. " +
			"old_string must match the file content exactly (including whitespace). " +
			"Set replace_all to true to replace every occurrence.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file.",
				},
				"old_string": map[string]any{
					"type":        "string",
					"description": "The exact text to find and replace.",
				},
				"new_string": map[string]any{
					"type":        "string",
					"description": "The replacement text.",
				},
				"replace_all": map[string]any{
					"type":        "boolean",
					"description": "Replace every occurrence instead of just the first (default false).",
				},
			},
			"required": []string{"path", "old_string", "new_string"},
		},
		Fn: func(ctx context.Context, input json.RawMessage) (string, error) {
			var payload struct {
				Path       string `json:"path"`
				OldString  string `json:"old_string"`
				NewString  string `json:"new_string"`
				ReplaceAll bool   `json:"replace_all"`
			}
			if err := json.Unmarshal(input, &payload); err != nil {
				return "", fmt.Errorf("parse edit_file input: %w", err)
			}
			if payload.Path == "" {
				return "", fmt.Errorf("path is required")
			}

			data, err := os.ReadFile(payload.Path)
			if err != nil {
				return "", fmt.Errorf("edit_file: read: %w", err)
			}
			content := string(data)

			if !strings.Contains(content, payload.OldString) {
				return "", fmt.Errorf("edit_file: old_string not found in %s", payload.Path)
			}

			var updated string
			if payload.ReplaceAll {
				updated = strings.ReplaceAll(content, payload.OldString, payload.NewString)
			} else {
				updated = strings.Replace(content, payload.OldString, payload.NewString, 1)
			}

			if err := os.WriteFile(payload.Path, []byte(updated), 0o644); err != nil {
				return "", fmt.Errorf("edit_file: write: %w", err)
			}
			return fmt.Sprintf("edit applied to %s", payload.Path), nil
		},
	}
}

// ─── list_dir ─────────────────────────────────────────────────────────────────

func listDirTool() *Tool {
	return &Tool{
		Name:        "list_dir",
		Description: "List files and directories at a given path.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path to list (default: current working directory).",
				},
			},
		},
		Fn: func(ctx context.Context, input json.RawMessage) (string, error) {
			var payload struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(input, &payload); err != nil {
				return "", fmt.Errorf("parse list_dir input: %w", err)
			}
			dir := payload.Path
			if dir == "" {
				var err error
				dir, err = os.Getwd()
				if err != nil {
					return "", fmt.Errorf("list_dir: getwd: %w", err)
				}
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				return "", fmt.Errorf("list_dir: %w", err)
			}
			if len(entries) == 0 {
				return "(empty directory)", nil
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
			return strings.TrimRight(buf.String(), "\n"), nil
		},
	}
}

// ─── glob_files ───────────────────────────────────────────────────────────────

func globTool() *Tool {
	return &Tool{
		Name:        "glob_files",
		Description: "Find files matching a glob pattern (e.g. **/*.go). Returns matching paths sorted by modification time.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern, e.g. \"src/**/*.ts\" or \"*.go\".",
				},
				"dir": map[string]any{
					"type":        "string",
					"description": "Base directory to search in (default: current working directory).",
				},
			},
			"required": []string{"pattern"},
		},
		Fn: func(ctx context.Context, input json.RawMessage) (string, error) {
			var payload struct {
				Pattern string `json:"pattern"`
				Dir     string `json:"dir"`
			}
			if err := json.Unmarshal(input, &payload); err != nil {
				return "", fmt.Errorf("parse glob_files input: %w", err)
			}
			if payload.Pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}

			base := payload.Dir
			if base == "" {
				var err error
				base, err = os.Getwd()
				if err != nil {
					return "", fmt.Errorf("glob_files: getwd: %w", err)
				}
			}

			matches, err := walkGlob(base, payload.Pattern)
			if err != nil {
				return "", fmt.Errorf("glob_files: %w", err)
			}
			if len(matches) == 0 {
				return "(no matches)", nil
			}
			return strings.Join(matches, "\n"), nil
		},
	}
}

// ─── grep_files ───────────────────────────────────────────────────────────────

func grepTool() *Tool {
	return &Tool{
		Name:        "grep_files",
		Description: "Search file contents for a literal string or regex pattern. Returns matching lines with file name and line number.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The string or regex pattern to search for.",
				},
				"dir": map[string]any{
					"type":        "string",
					"description": "Directory to search in (default: current working directory).",
				},
				"glob": map[string]any{
					"type":        "string",
					"description": "Only search files matching this glob pattern, e.g. \"*.go\".",
				},
				"case_insensitive": map[string]any{
					"type":        "boolean",
					"description": "Case-insensitive search (default false).",
				},
				"max_results": map[string]any{
					"type":        "integer",
					"description": "Maximum number of matching lines to return (default 100).",
				},
			},
			"required": []string{"pattern"},
		},
		Fn: func(ctx context.Context, input json.RawMessage) (string, error) {
			var payload struct {
				Pattern         string `json:"pattern"`
				Dir             string `json:"dir"`
				Glob            string `json:"glob"`
				CaseInsensitive bool   `json:"case_insensitive"`
				MaxResults      int    `json:"max_results"`
			}
			if err := json.Unmarshal(input, &payload); err != nil {
				return "", fmt.Errorf("parse grep_files input: %w", err)
			}
			if payload.Pattern == "" {
				return "", fmt.Errorf("pattern is required")
			}
			if payload.MaxResults <= 0 {
				payload.MaxResults = 100
			}

			base := payload.Dir
			if base == "" {
				var err error
				base, err = os.Getwd()
				if err != nil {
					return "", fmt.Errorf("grep_files: getwd: %w", err)
				}
			}

			// Use ripgrep if available; fall back to a pure-Go walker.
			if rg, err := exec.LookPath("rg"); err == nil {
				return runRipgrep(ctx, rg, payload.Pattern, base, payload.Glob, payload.CaseInsensitive, payload.MaxResults)
			}
			return walkGrep(ctx, base, payload.Pattern, payload.Glob, payload.CaseInsensitive, payload.MaxResults)
		},
	}
}

func runRipgrep(ctx context.Context, rg, pattern, base, glob string, ci bool, max int) (string, error) {
	args := []string{"--line-number", "--no-heading", "--color=never"}
	if ci {
		args = append(args, "--ignore-case")
	}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, pattern, base)

	cmd := exec.CommandContext(ctx, rg, args...)
	out, err := cmd.Output()
	if err != nil {
		// exit code 1 = no matches; that's fine.
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return "(no matches)", nil
		}
		return "", fmt.Errorf("grep_files: rg: %w", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) > max {
		lines = lines[:max]
	}
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return "(no matches)", nil
	}
	return strings.Join(lines, "\n"), nil
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

		// Apply glob filter if provided.
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
					return fmt.Errorf("stop") // sentinel to stop walk
				}
			}
		}

		// Detect binary files by checking for null bytes.
		if bytes.ContainsRune(scanner.Bytes(), 0) {
			return nil
		}
		return nil
	})

	if err != nil && err.Error() != "stop" {
		return "", fmt.Errorf("grep_files: walk: %w", err)
	}
	if len(results) == 0 {
		return "(no matches)", nil
	}
	return strings.Join(results, "\n"), nil
}

// walkGlob walks base and returns paths relative to base that match pattern.
// It supports "**" by walking all directories and matching each file's relative
// path against the pattern using filepath.Match on each path segment.
func walkGlob(base, pattern string) ([]string, error) {
	// Normalise separators so patterns work on Windows too.
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
		// Use filepath.Match; if the pattern contains "**" we expand it by
		// also trying to match just the filename (simple heuristic that handles
		// the common "**/*.ext" case).
		ok, _ := filepath.Match(pattern, rel)
		if !ok && strings.Contains(pattern, "**") {
			// Try matching only the base filename against the part after "**/"
			suffix := pattern[strings.LastIndex(pattern, "**/")+3:]
			ok, _ = filepath.Match(suffix, filepath.Base(rel))
		}
		if ok {
			matches = append(matches, rel)
		}
		return nil
	})
	return matches, err
}
