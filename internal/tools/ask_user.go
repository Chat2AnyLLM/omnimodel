package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

type askUserTool struct{}

func AskUser() Tool { return &askUserTool{} }

func (t *askUserTool) Name() string { return "ask_user_question" }

func (t *askUserTool) Description() string {
	return "Ask the user a question and return their response. Use this when you need clarification, a decision, or additional information from the user."
}

func (t *askUserTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{"type": "string", "description": "The question to ask the user."},
			"options":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional list of predefined answer options for the user to choose from."},
		},
		"required": []string{"question"},
	}
}

func (t *askUserTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Question == "" {
		return Result{Output: "error: question is required", IsError: true}
	}

	if call.AskUser == nil {
		return Result{Output: "error: ask_user callback not available in this context", IsError: true}
	}

	answer, err := call.AskUser(ctx, p.Question, p.Options)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{
		Title:  fmt.Sprintf("Asked: %s", p.Question),
		Output: fmt.Sprintf("User answered: %s", answer),
	}
}
