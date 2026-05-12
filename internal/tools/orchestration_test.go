package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOrchestrateAgentsFanOut(t *testing.T) {
	tool := OrchestrateAgents()
	var calls []string
	ctx := Context{
		SendMessageFn: func(_ context.Context, to, message string) (string, error) {
			calls = append(calls, to+"::"+message)
			return fmt.Sprintf("%s done", to), nil
		},
	}

	input, _ := json.Marshal(map[string]any{
		"pattern": "fan_out",
		"tasks": []map[string]any{
			{"worker": "research", "prompt": "collect facts"},
			{"worker": "coder", "prompt": "draft patch"},
		},
	})
	res := tool.Execute(context.Background(), ctx, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if !strings.Contains(res.Output, "Pattern: fan_out") {
		t.Fatalf("missing fan_out header: %q", res.Output)
	}
}

func TestOrchestrateAgentsPipelineCarriesPreviousOutput(t *testing.T) {
	tool := OrchestrateAgents()
	received := make([]string, 0, 2)
	ctx := Context{
		SendMessageFn: func(_ context.Context, to, message string) (string, error) {
			received = append(received, to+"::"+message)
			if to == "stage1" {
				return "stage1 result", nil
			}
			return "stage2 result", nil
		},
	}

	input, _ := json.Marshal(map[string]any{
		"pattern": "pipeline",
		"tasks": []map[string]any{
			{"worker": "stage1", "prompt": "analyze"},
			{"worker": "stage2", "prompt": "implement"},
		},
	})
	res := tool.Execute(context.Background(), ctx, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if len(received) != 2 {
		t.Fatalf("expected 2 pipeline calls, got %d", len(received))
	}
	if !strings.Contains(received[1], "Previous stage output:\nstage1 result") {
		t.Fatalf("second stage did not receive prior output: %q", received[1])
	}
}

func TestOrchestrateAgentsSupervisorAddsSynthesisStep(t *testing.T) {
	tool := OrchestrateAgents()
	var supervisorInput string
	ctx := Context{
		SendMessageFn: func(_ context.Context, to, message string) (string, error) {
			if to == "supervisor" {
				supervisorInput = message
				return "final synthesis", nil
			}
			return to + " output", nil
		},
	}

	input, _ := json.Marshal(map[string]any{
		"pattern": "supervisor",
		"tasks": []map[string]any{
			{"worker": "research", "prompt": "collect facts"},
			{"worker": "coder", "prompt": "write code"},
		},
		"supervisor_worker": "supervisor",
		"supervisor_prompt": "merge worker results",
	})
	res := tool.Execute(context.Background(), ctx, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(supervisorInput, "merge worker results") {
		t.Fatalf("supervisor prompt missing custom instruction: %q", supervisorInput)
	}
	if !strings.Contains(supervisorInput, "research output") || !strings.Contains(supervisorInput, "coder output") {
		t.Fatalf("supervisor input missing worker outputs: %q", supervisorInput)
	}
	if !strings.Contains(res.Output, "final synthesis") {
		t.Fatalf("final output missing supervisor summary: %q", res.Output)
	}
}

func TestOrchestrateAgentsGeneratorEvaluatorConverges(t *testing.T) {
	tool := OrchestrateAgents()
	genCalls := 0
	evalCalls := 0
	ctx := Context{
		SendMessageFn: func(_ context.Context, to, message string) (string, error) {
			switch to {
			case "generator":
				genCalls++
				if genCalls == 1 {
					return "draft v1", nil
				}
				return "draft v2", nil
			case "evaluator":
				evalCalls++
				if evalCalls == 1 {
					return "FAIL: missing edge-case handling", nil
				}
				return "PASS: meets acceptance criteria", nil
			default:
				return "", fmt.Errorf("unexpected worker %s", to)
			}
		},
	}

	input, _ := json.Marshal(map[string]any{
		"pattern": "generator_evaluator",
		"tasks": []map[string]any{
			{"worker": "generator", "prompt": "produce implementation"},
			{"worker": "evaluator", "prompt": "evaluate result"},
		},
		"max_rounds":          3,
		"acceptance_criteria": "Code compiles and handles edge cases",
	})
	res := tool.Execute(context.Background(), ctx, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if genCalls != 2 || evalCalls != 2 {
		t.Fatalf("expected 2 gen and 2 eval calls, got gen=%d eval=%d", genCalls, evalCalls)
	}
	if !strings.Contains(res.Output, "Converged") {
		t.Fatalf("expected convergence marker, got: %q", res.Output)
	}
}

func TestOrchestrateAgentsGeneratorEvaluatorMaxRounds(t *testing.T) {
	tool := OrchestrateAgents()
	evalCalls := 0
	ctx := Context{
		SendMessageFn: func(_ context.Context, to, message string) (string, error) {
			if to == "generator" {
				return "candidate", nil
			}
			evalCalls++
			return "FAIL: needs more work", nil
		},
	}

	input, _ := json.Marshal(map[string]any{
		"pattern": "generator_evaluator",
		"tasks": []map[string]any{
			{"worker": "generator", "prompt": "produce implementation"},
			{"worker": "evaluator", "prompt": "evaluate result"},
		},
		"max_rounds": 2,
	})
	res := tool.Execute(context.Background(), ctx, input)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if evalCalls != 2 {
		t.Fatalf("expected 2 evaluator calls, got %d", evalCalls)
	}
	if !strings.Contains(res.Output, "Reached max rounds (2)") {
		t.Fatalf("expected max-round summary, got: %q", res.Output)
	}
}
