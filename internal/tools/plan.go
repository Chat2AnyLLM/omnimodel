package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ─── PlanState ────────────────────────────────────────────────────────────────

// PlanState tracks whether the agent is currently in plan mode.
type PlanState struct {
	mu     sync.Mutex
	active bool
	plan   string
}

func NewPlanState() *PlanState { return &PlanState{} }

func (s *PlanState) Enter(plan string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = true
	s.plan = plan
}

func (s *PlanState) Exit() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan := s.plan
	s.active = false
	s.plan = ""
	return plan
}

func (s *PlanState) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// ─── enter_plan_mode ──────────────────────────────────────────────────────────

type enterPlanModeTool struct{}

func EnterPlanMode() Tool { return &enterPlanModeTool{} }

func (t *enterPlanModeTool) Name() string { return "enter_plan_mode" }
func (t *enterPlanModeTool) Description() string {
	return "Enter plan mode: write out a structured implementation plan before making any changes. " +
		"Use this to think through a complex task and present the plan to the user for approval. " +
		"Call exit_plan_mode when the plan is ready and you want to proceed with execution."
}
func (t *enterPlanModeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan": map[string]any{
				"type":        "string",
				"description": "The implementation plan you intend to follow. Be specific: list files to create/modify, steps to take, and trade-offs considered.",
			},
		},
		"required": []string{"plan"},
	}
}
func (t *enterPlanModeTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Plan == "" {
		return Result{Output: "error: plan is required", IsError: true}
	}
	ps := call.PlanState
	if ps == nil {
		return Result{Output: "error: plan state not available", IsError: true}
	}
	if ps.IsActive() {
		return Result{Output: "error: already in plan mode — call exit_plan_mode first", IsError: true}
	}
	ps.Enter(p.Plan)
	return Result{
		Title:  "Plan mode entered",
		Output: fmt.Sprintf("Plan mode activated. Current plan:\n\n%s\n\nCall exit_plan_mode to proceed with execution.", p.Plan),
	}
}

// ─── exit_plan_mode ───────────────────────────────────────────────────────────

type exitPlanModeTool struct{}

func ExitPlanMode() Tool { return &exitPlanModeTool{} }

func (t *exitPlanModeTool) Name() string { return "exit_plan_mode" }
func (t *exitPlanModeTool) Description() string {
	return "Exit plan mode and proceed with executing the plan. " +
		"Only call this after the plan has been approved or you are ready to act."
}
func (t *exitPlanModeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"approved": map[string]any{
				"type":        "boolean",
				"description": "Whether the plan was explicitly approved. Defaults to true.",
			},
		},
	}
}
func (t *exitPlanModeTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Approved *bool `json:"approved"`
	}
	_ = json.Unmarshal(input, &p)
	ps := call.PlanState
	if ps == nil {
		return Result{Output: "error: plan state not available", IsError: true}
	}
	if !ps.IsActive() {
		return Result{Output: "Not in plan mode."}
	}
	plan := ps.Exit()
	approved := p.Approved == nil || *p.Approved
	if !approved {
		return Result{
			Title:  "Plan mode exited (not approved)",
			Output: "Plan mode exited. The plan was not approved — no changes will be made.",
		}
	}
	return Result{
		Title:  "Plan mode exited — proceeding with execution",
		Output: fmt.Sprintf("Plan mode exited. Executing plan:\n\n%s", plan),
	}
}
