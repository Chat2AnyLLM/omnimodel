package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// tool_search — search available tools by name or description.

type toolSearchTool struct{}

func ToolSearch() Tool { return &toolSearchTool{} }

func (t *toolSearchTool) Name() string { return "tool_search" }
func (t *toolSearchTool) Description() string {
	return "Search the list of available tools by name or description keyword. " +
		"Returns matching tool names and descriptions."
}
func (t *toolSearchTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Keyword to search in tool names and descriptions."},
		},
		"required": []string{"query"},
	}
}
func (t *toolSearchTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if call.Registry == nil {
		return Result{Output: "error: tool registry not available", IsError: true}
	}
	q := strings.ToLower(strings.TrimSpace(p.Query))
	tools := call.Registry.List()
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })

	var matches []string
	for _, tool := range tools {
		if q == "" ||
			strings.Contains(strings.ToLower(tool.Name()), q) ||
			strings.Contains(strings.ToLower(tool.Description()), q) {
			matches = append(matches, fmt.Sprintf("%-24s %s", tool.Name(), firstLine(tool.Description())))
		}
	}
	if len(matches) == 0 {
		return Result{Output: fmt.Sprintf("No tools match %q", p.Query)}
	}
	return Result{Output: strings.Join(matches, "\n")}
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// ─── config tool ──────────────────────────────────────────────────────────────

// ConfigStore is a simple in-memory key-value config store for a session.
type ConfigStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewConfigStore() *ConfigStore {
	return &ConfigStore{data: make(map[string]string)}
}

func (s *ConfigStore) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *ConfigStore) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

func (s *ConfigStore) All() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}
