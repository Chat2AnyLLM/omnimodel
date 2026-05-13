package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

type multiEditTool struct{}

func MultiEdit() Tool { return &multiEditTool{} }

func (t *multiEditTool) Name() string { return "multiedit" }

func (t *multiEditTool) Description() string {
	return "Apply multiple sequential edit operations (old_string -> new_string) to a single file. Use this when you need to make several related changes to one file."
}

func (t *multiEditTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{"type": "string", "description": "The absolute path to the file to modify."},
			"edits": map[string]any{
				"type":        "array",
				"description": "Array of edit operations to perform sequentially on the file.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"old_string":  map[string]any{"type": "string", "description": "The text to replace."},
						"new_string":  map[string]any{"type": "string", "description": "The text to replace it with."},
						"replace_all": map[string]any{"type": "boolean", "description": "Replace all occurrences (default false)."},
					},
					"required": []string{"old_string", "new_string"},
				},
			},
		},
		"required": []string{"file_path", "edits"},
	}
}

func (t *multiEditTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		FilePath string `json:"file_path"`
		Edits    []struct {
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		} `json:"edits"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.FilePath == "" {
		return Result{Output: "error: file_path is required", IsError: true}
	}
	if len(p.Edits) == 0 {
		return Result{Output: "error: at least one edit is required", IsError: true}
	}

	absPath := p.FilePath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(cwd(), p.FilePath)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return Result{Output: fmt.Sprintf("error: reading file: %v", err), IsError: true}
	}

	text := string(content)
	hasCRLF := strings.Contains(text, "\r\n")
	var results []string

	for i, edit := range p.Edits {
		if edit.OldString == "" {
			results = append(results, fmt.Sprintf("edit %d: skipped (empty old_string)", i+1))
			continue
		}

		directMatch := strings.Contains(text, edit.OldString)
		normalizedOld := strings.ReplaceAll(edit.OldString, "\r\n", "\n")
		normalizedNew := strings.ReplaceAll(edit.NewString, "\r\n", "\n")
		normalizedText := strings.ReplaceAll(text, "\r\n", "\n")
		normalizedMatch := strings.Contains(normalizedText, normalizedOld)

		if !directMatch && !normalizedMatch {
			log.Debug().
				Str("file", absPath).
				Int("edit_index", i+1).
				Bool("file_has_crlf", hasCRLF).
				Msg("multiedit: old_string not found")
			results = append(results, fmt.Sprintf("edit %d: error: could not find %q", i+1, edit.OldString))
			continue
		}

		if directMatch {
			if edit.ReplaceAll {
				text = strings.ReplaceAll(text, edit.OldString, edit.NewString)
			} else {
				text = strings.Replace(text, edit.OldString, edit.NewString, 1)
			}
		} else {
			log.Debug().Str("file", absPath).Int("edit_index", i+1).Msg("multiedit: matched after line-ending normalization")
			if edit.ReplaceAll {
				normalizedText = strings.ReplaceAll(normalizedText, normalizedOld, normalizedNew)
			} else {
				normalizedText = strings.Replace(normalizedText, normalizedOld, normalizedNew, 1)
			}
			if hasCRLF {
				text = strings.ReplaceAll(normalizedText, "\n", "\r\n")
			} else {
				text = normalizedText
			}
		}
		results = append(results, fmt.Sprintf("edit %d: replaced %q -> %q", i+1, edit.OldString, edit.NewString))
	}

	if err := os.WriteFile(absPath, []byte(text), 0644); err != nil {
		return Result{Output: fmt.Sprintf("error: writing file: %v", err), IsError: true}
	}

	return Result{
		Title:  fmt.Sprintf("Applied %d edit(s) to %s", len(p.Edits), filepath.Base(p.FilePath)),
		Output: strings.Join(results, "\n"),
	}
}
