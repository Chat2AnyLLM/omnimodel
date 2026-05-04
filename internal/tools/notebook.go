package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// notebook_edit — read and edit Jupyter notebook cells (.ipynb files).

type notebookEditTool struct{}

func NotebookEdit() Tool { return &notebookEditTool{} }

func (t *notebookEditTool) Name() string { return "notebook_edit" }
func (t *notebookEditTool) Description() string {
	return "Read or edit cells in a Jupyter notebook (.ipynb) file. " +
		"Supports reading all cells, replacing a cell's source, inserting a new cell, or deleting a cell."
}
func (t *notebookEditTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":      map[string]any{"type": "string", "description": "Absolute or relative path to the .ipynb file."},
			"edit_mode": map[string]any{"type": "string", "enum": []string{"read", "replace", "insert", "delete"}, "description": "Operation: read all cells, replace cell source, insert new cell after cell_index, or delete a cell."},
			"cell_index": map[string]any{"type": "integer", "description": "0-based cell index. Required for replace, insert (inserts after this index), and delete."},
			"cell_type":  map[string]any{"type": "string", "enum": []string{"code", "markdown"}, "description": "Cell type for insert (default: code)."},
			"new_source": map[string]any{"type": "string", "description": "New source content for replace or insert operations."},
		},
		"required": []string{"path", "edit_mode"},
	}
}

// notebookCell is a simplified in-memory representation.
type notebookCell struct {
	CellType string      `json:"cell_type"`
	Source   interface{} `json:"source"` // may be string or []string in JSON
	Metadata interface{} `json:"metadata,omitempty"`
	Outputs  interface{} `json:"outputs,omitempty"`
}

type notebook struct {
	NBFormat      int                    `json:"nbformat"`
	NBFormatMinor int                    `json:"nbformat_minor"`
	Metadata      map[string]interface{} `json:"metadata"`
	Cells         []notebookCell         `json:"cells"`
}

func (t *notebookEditTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Path      string `json:"path"`
		EditMode  string `json:"edit_mode"`
		CellIndex *int   `json:"cell_index"`
		CellType  string `json:"cell_type"`
		NewSource string `json:"new_source"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Path == "" {
		return Result{Output: "error: path is required", IsError: true}
	}

	raw, err := os.ReadFile(p.Path)
	if err != nil {
		return Result{Output: "error: read file: " + err.Error(), IsError: true}
	}
	var nb notebook
	if err := json.Unmarshal(raw, &nb); err != nil {
		return Result{Output: "error: parse notebook: " + err.Error(), IsError: true}
	}

	switch p.EditMode {
	case "read", "":
		var sb strings.Builder
		for i, cell := range nb.Cells {
			src := cellSourceString(cell.Source)
			sb.WriteString(fmt.Sprintf("[%d] %s\n%s\n\n", i, cell.CellType, src))
		}
		return Result{Output: strings.TrimRight(sb.String(), "\n")}

	case "replace":
		if p.CellIndex == nil {
			return Result{Output: "error: cell_index is required for replace", IsError: true}
		}
		idx := *p.CellIndex
		if idx < 0 || idx >= len(nb.Cells) {
			return Result{Output: fmt.Sprintf("error: cell_index %d out of range (0-%d)", idx, len(nb.Cells)-1), IsError: true}
		}
		nb.Cells[idx].Source = p.NewSource
		return writeNotebook(p.Path, &nb, fmt.Sprintf("Cell %d replaced.", idx))

	case "insert":
		insertAfter := -1
		if p.CellIndex != nil {
			insertAfter = *p.CellIndex
		}
		ct := p.CellType
		if ct == "" {
			ct = "code"
		}
		newCell := notebookCell{
			CellType: ct,
			Source:   p.NewSource,
			Metadata: map[string]interface{}{},
		}
		if ct == "code" {
			newCell.Outputs = []interface{}{}
		}
		pos := insertAfter + 1
		if pos > len(nb.Cells) {
			pos = len(nb.Cells)
		}
		nb.Cells = append(nb.Cells[:pos], append([]notebookCell{newCell}, nb.Cells[pos:]...)...)
		return writeNotebook(p.Path, &nb, fmt.Sprintf("Cell inserted at index %d.", pos))

	case "delete":
		if p.CellIndex == nil {
			return Result{Output: "error: cell_index is required for delete", IsError: true}
		}
		idx := *p.CellIndex
		if idx < 0 || idx >= len(nb.Cells) {
			return Result{Output: fmt.Sprintf("error: cell_index %d out of range", idx), IsError: true}
		}
		nb.Cells = append(nb.Cells[:idx], nb.Cells[idx+1:]...)
		return writeNotebook(p.Path, &nb, fmt.Sprintf("Cell %d deleted.", idx))

	default:
		return Result{Output: fmt.Sprintf("error: unknown edit_mode %q", p.EditMode), IsError: true}
	}
}

func writeNotebook(path string, nb *notebook, msg string) Result {
	out, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return Result{Output: "error: marshal notebook: " + err.Error(), IsError: true}
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return Result{Output: "error: write file: " + err.Error(), IsError: true}
	}
	return Result{Title: "Notebook edited", Output: msg}
}

func cellSourceString(src interface{}) string {
	switch v := src.(type) {
	case string:
		return v
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, s := range v {
			if str, ok := s.(string); ok {
				parts = append(parts, str)
			}
		}
		return strings.Join(parts, "")
	default:
		return fmt.Sprintf("%v", src)
	}
}
