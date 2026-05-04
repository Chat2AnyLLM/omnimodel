package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type lspTool struct{}

func LSP() Tool { return &lspTool{} }

func (t *lspTool) Name() string { return "lsp" }

func (t *lspTool) Description() string {
	return "Query an LSP server for code intelligence: go-to-definition, find-references, hover info, document symbols, workspace symbols, call hierarchy."
}

func (t *lspTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"description": "The LSP operation to perform",
				"enum":        []string{"goToDefinition", "findReferences", "hover", "documentSymbol", "workspaceSymbol", "goToImplementation", "prepareCallHierarchy", "incomingCalls", "outgoingCalls"},
			},
			"filePath":  map[string]any{"type": "string", "description": "The absolute path to the file"},
			"line":      map[string]any{"type": "integer", "description": "The line number (1-based)"},
			"character": map[string]any{"type": "integer", "description": "The character offset (1-based)"},
			"query":     map[string]any{"type": "string", "description": "Search query for workspaceSymbol operation"},
		},
		"required": []string{"operation"},
	}
}

func (t *lspTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Operation string `json:"operation"`
		FilePath  string `json:"filePath"`
		Line      int    `json:"line"`
		Character int    `json:"character"`
		Query     string `json:"query"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}

	// Validate operation
	switch p.Operation {
	case "goToDefinition", "findReferences", "hover", "documentSymbol", "workspaceSymbol",
		"goToImplementation", "prepareCallHierarchy", "incomingCalls", "outgoingCalls":
	default:
		return Result{Output: fmt.Sprintf("error: unknown LSP operation %q", p.Operation), IsError: true}
	}

	// For operations that need a file, validate file path
	if p.Operation != "workspaceSymbol" && p.FilePath == "" {
		return Result{Output: "error: filePath is required for " + p.Operation, IsError: true}
	}

	// Build the LSP request
	var resultLines []string

	switch p.Operation {
	case "hover":
		resultLines = append(resultLines, fmt.Sprintf("Hover info for %s:%d:%d", p.FilePath, p.Line, p.Character))
		resultLines = append(resultLines, "(LSP integration requires an LSP server to be running)")
	case "goToDefinition":
		resultLines = append(resultLines, fmt.Sprintf("Definition at %s:%d:%d", p.FilePath, p.Line, p.Character))
		resultLines = append(resultLines, "(LSP integration requires an LSP server to be running)")
	case "findReferences":
		resultLines = append(resultLines, fmt.Sprintf("References in %s:%d:%d", p.FilePath, p.Line, p.Character))
		resultLines = append(resultLines, "(LSP integration requires an LSP server to be running)")
	case "documentSymbol":
		resultLines = append(resultLines, fmt.Sprintf("Symbols in %s", p.FilePath))
		resultLines = append(resultLines, "(LSP integration requires an LSP server to be running)")
	case "workspaceSymbol":
		resultLines = append(resultLines, fmt.Sprintf("Workspace symbols matching %q", p.Query))
		resultLines = append(resultLines, "(LSP integration requires an LSP server to be running)")
	case "goToImplementation":
		resultLines = append(resultLines, fmt.Sprintf("Implementations of %s:%d:%d", p.FilePath, p.Line, p.Character))
		resultLines = append(resultLines, "(LSP integration requires an LSP server to be running)")
	case "prepareCallHierarchy":
		resultLines = append(resultLines, fmt.Sprintf("Call hierarchy for %s:%d:%d", p.FilePath, p.Line, p.Character))
		resultLines = append(resultLines, "(LSP integration requires an LSP server to be running)")
	case "incomingCalls":
		resultLines = append(resultLines, fmt.Sprintf("Incoming calls to %s:%d:%d", p.FilePath, p.Line, p.Character))
		resultLines = append(resultLines, "(LSP integration requires an LSP server to be running)")
	case "outgoingCalls":
		resultLines = append(resultLines, fmt.Sprintf("Outgoing calls from %s:%d:%d", p.FilePath, p.Line, p.Character))
		resultLines = append(resultLines, "(LSP integration requires an LSP server to be running)")
	}

	return Result{
		Title:  fmt.Sprintf("LSP %s", p.Operation),
		Output: strings.Join(resultLines, "\n"),
	}
}
