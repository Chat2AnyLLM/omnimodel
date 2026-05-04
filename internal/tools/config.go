package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// config — read and write agent runtime configuration key-value pairs.

type configTool struct{}

func Config() Tool { return &configTool{} }

func (t *configTool) Name() string { return "config" }
func (t *configTool) Description() string {
	return "Read or write agent runtime configuration values for this session. " +
		"Use 'get' to read a key, 'set' to write a key, or 'list' to show all values."
}
func (t *configTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"get", "set", "list"},
				"description": "Operation: get a value, set a value, or list all.",
			},
			"key":   map[string]any{"type": "string", "description": "Config key (required for get/set)."},
			"value": map[string]any{"type": "string", "description": "Config value (required for set)."},
		},
		"required": []string{"action"},
	}
}
func (t *configTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Action string `json:"action"`
		Key    string `json:"key"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	store := call.ConfigStore
	if store == nil {
		return Result{Output: "error: config store not available", IsError: true}
	}
	switch p.Action {
	case "get":
		if p.Key == "" {
			return Result{Output: "error: key is required for get", IsError: true}
		}
		v, ok := store.Get(p.Key)
		if !ok {
			return Result{Output: fmt.Sprintf("(key %q not set)", p.Key)}
		}
		return Result{Output: v}
	case "set":
		if p.Key == "" {
			return Result{Output: "error: key is required for set", IsError: true}
		}
		store.Set(p.Key, p.Value)
		return Result{Output: fmt.Sprintf("Set %s = %s", p.Key, p.Value)}
	case "list":
		all := store.All()
		if len(all) == 0 {
			return Result{Output: "(no config values set)"}
		}
		var sb strings.Builder
		for k, v := range all {
			fmt.Fprintf(&sb, "%s = %s\n", k, v)
		}
		return Result{Output: strings.TrimRight(sb.String(), "\n")}
	default:
		return Result{Output: fmt.Sprintf("error: unknown action %q", p.Action), IsError: true}
	}
}
