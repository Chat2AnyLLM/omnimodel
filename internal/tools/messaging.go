package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// send_message — send a message to another running agent or sub-process.

type sendMessageTool struct{}

func SendMessage() Tool { return &sendMessageTool{} }

func (t *sendMessageTool) Name() string { return "send_message" }
func (t *sendMessageTool) Description() string {
	return "Send a message to another agent, sub-agent, or named process and return its response. " +
		"Used for multi-agent coordination: orchestrate parallel agents, relay instructions, " +
		"or pass results between agents."
}
func (t *sendMessageTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"to":      map[string]any{"type": "string", "description": "Target agent or process name/ID."},
			"message": map[string]any{"type": "string", "description": "The message or instruction to send."},
		},
		"required": []string{"to", "message"},
	}
}
func (t *sendMessageTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		To      string `json:"to"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.To == "" || p.Message == "" {
		return Result{Output: "error: to and message are required", IsError: true}
	}
	if call.SendMessageFn == nil {
		return Result{Output: "error: send_message not configured in this environment", IsError: true}
	}
	resp, err := call.SendMessageFn(ctx, p.To, p.Message)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{
		Title:  fmt.Sprintf("Message sent to %s", p.To),
		Output: resp,
	}
}

// ─── agent_tool — spawn a named sub-agent to handle a sub-task ───────────────

type agentTool struct{}

func AgentTool() Tool { return &agentTool{} }

func (t *agentTool) Name() string { return "agent" }
func (t *agentTool) Description() string {
	return "Spawn a sub-agent to handle a complex, multi-step sub-task in isolation. " +
		"The sub-agent gets its own tool context, runs to completion, and returns its output. " +
		"Use this to parallelize work or delegate specialised tasks."
}
func (t *agentTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"description": map[string]any{"type": "string", "description": "Short description of the sub-agent's task (used as its name/label)."},
			"prompt":      map[string]any{"type": "string", "description": "The full instruction/task for the sub-agent."},
		},
		"required": []string{"prompt"},
	}
}
func (t *agentTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Prompt == "" {
		return Result{Output: "error: prompt is required", IsError: true}
	}
	// Delegate through SendMessageFn if wired (orchestrator handles the routing).
	if call.SendMessageFn != nil {
		target := p.Description
		if target == "" {
			target = "sub-agent"
		}
		resp, err := call.SendMessageFn(ctx, target, p.Prompt)
		if err != nil {
			return Result{Output: "error: " + err.Error(), IsError: true}
		}
		return Result{Title: fmt.Sprintf("Sub-agent (%s) completed", target), Output: resp}
	}
	// Fallback: run as a background shell task if no messaging backend is configured.
	store := call.TaskStore
	if store == nil {
		return Result{
			Output:  "error: agent tool requires either a send_message backend or a task store",
			IsError: true,
		}
	}
	id := store.nextID()
	desc := p.Description
	if desc == "" {
		desc = fmt.Sprintf("sub-agent: %s", truncateTo(p.Prompt, 60))
	}
	run := &TaskRun{
		ID:          id,
		Description: desc,
		Status:      TaskRunPending,
		Output:      fmt.Sprintf("Sub-agent queued. Prompt: %s", p.Prompt),
	}
	store.add(run)
	return Result{
		Title:  fmt.Sprintf("Sub-agent queued: %s", id),
		Output: fmt.Sprintf("Sub-agent task %s created. Prompt:\n%s\n\nUse task_output %s to retrieve results.", id, p.Prompt, id),
	}
}
