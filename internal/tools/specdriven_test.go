package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omnillm/internal/specdriven"
	"omnillm/internal/tools"
)

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?helpers 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?

func newSpecCtx(t *testing.T) (tools.Context, string) {
	t.Helper()
	dir := t.TempDir()
	return tools.Context{SpecState: specdriven.NewSpecStore()}, dir
}

func execTool(t *testing.T, tool tools.Tool, call tools.Context, input any) tools.Result {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return tool.Execute(context.Background(), call, raw)
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?spec_init 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾

func TestSpecInitCreatesDirectory(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	result := execTool(t, tools.SpecInit(), call, map[string]any{
		"title":     "User Authentication",
		"overview":  "Allow users to sign in and out securely.",
		"specs_dir": specsDir,
	})

	if result.IsError {
		t.Fatalf("spec_init failed: %s", result.Output)
	}

	entries, err := os.ReadDir(specsDir)
	if err != nil {
		t.Fatalf("specs dir not created: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 spec dir, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Name(), "user-authentication") {
		t.Errorf("dir name %q should contain 'user-authentication'", entries[0].Name())
	}

	specFile := filepath.Join(specsDir, entries[0].Name(), "spec.md")
	content, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatalf("spec.md not created: %v", err)
	}
	if !strings.Contains(string(content), "User Authentication") {
		t.Error("spec.md missing title")
	}

	stateFile := filepath.Join(specsDir, entries[0].Name(), ".speckit-state.json")
	stateContent, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("lifecycle metadata not created: %v", err)
	}
	if !strings.Contains(string(stateContent), "\"state\": \"draft\"") {
		t.Error("lifecycle metadata should default to draft")
	}
}

func TestSpecInitNumbersSequentially(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	for _, title := range []string{"Feature One", "Feature Two", "Feature Three"} {
		r := execTool(t, tools.SpecInit(), call, map[string]any{
			"title":     title,
			"overview":  "overview",
			"specs_dir": specsDir,
		})
		if r.IsError {
			t.Fatalf("spec_init %q failed: %s", title, r.Output)
		}
	}

	entries, _ := os.ReadDir(specsDir)
	if len(entries) != 3 {
		t.Fatalf("expected 3 spec dirs, got %d", len(entries))
	}
	prefixes := []string{"001", "002", "003"}
	for i, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefixes[i]) {
			t.Errorf("entry[%d] = %q, want prefix %q", i, entry.Name(), prefixes[i])
		}
	}
}

func TestSpecInitMissingTitle(t *testing.T) {
	call, _ := newSpecCtx(t)
	r := execTool(t, tools.SpecInit(), call, map[string]any{"overview": "x"})
	if !r.IsError {
		t.Error("spec_init with empty title should fail")
	}
}

func TestSpecInitSetsSessionState(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title":     "Photo Album",
		"overview":  "Store photos.",
		"specs_dir": specsDir,
	})

	if call.SpecState.GetSpec() == nil {
		t.Error("spec_init should set SpecState.currentSpec")
	}
	if call.SpecState.GetSpecDir() == "" {
		t.Error("spec_init should set SpecState.specDir")
	}
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?spec_write 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?

func TestSpecWriteUpdatesFile(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Auth", "overview": "login", "specs_dir": specsDir,
	})
	specDir := call.SpecState.GetSpecDir()

	r := execTool(t, tools.SpecWrite(), call, map[string]any{
		"user_stories": []map[string]any{{
			"id": "US1", "title": "Login", "description": "As a user I want to log in",
			"priority": "P1", "why_priority": "MVP core",
			"scenarios": []map[string]any{{
				"title": "Happy path", "given": "valid creds", "when": "form submitted", "then": "user authenticated",
			}},
		}},
		"requirements": []map[string]any{{
			"id": "FR-001", "user_story_id": "US1", "text": "The system SHALL validate passwords",
		}},
	})
	if r.IsError {
		t.Fatalf("spec_write failed: %s", r.Output)
	}

	content, _ := os.ReadFile(filepath.Join(specDir, "spec.md"))
	s := string(content)
	if !strings.Contains(s, "US1") {
		t.Error("spec.md missing US1")
	}
	if !strings.Contains(s, "valid creds") {
		t.Error("spec.md missing scenario GIVEN")
	}
	if !strings.Contains(s, "FR-001") {
		t.Error("spec.md missing FR-001")
	}
}

func TestSpecWriteRequiresSpecDir(t *testing.T) {
	call, _ := newSpecCtx(t)
	r := execTool(t, tools.SpecWrite(), call, map[string]any{})
	if !r.IsError {
		t.Error("spec_write without spec_dir and no session state should fail")
	}
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?spec_read 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾

func TestSpecReadShowsContent(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Photo Album", "overview": "manage photos", "specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecRead(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("spec_read failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Photo Album") {
		t.Error("spec_read output missing title")
	}
	if !strings.Contains(r.Output, "Lifecycle state: draft") {
		t.Error("spec_read should include lifecycle status")
	}
}

func TestSpecReadShowsArtifactStatus(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Auth", "overview": "login", "specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecRead(), call, map[string]any{})
	if !strings.Contains(r.Output, "spec.md") {
		t.Error("spec_read should show spec.md artifact status")
	}
	if !strings.Contains(r.Output, "plan.md") || !strings.Contains(r.Output, "tasks.md") {
		t.Error("spec_read should show missing plan/tasks artifact status")
	}
}

func TestSpecReadMissingDir(t *testing.T) {
	call, _ := newSpecCtx(t)
	r := execTool(t, tools.SpecRead(), call, map[string]any{})
	if !r.IsError {
		t.Error("spec_read without spec_dir should fail")
	}
}

func TestSpecReadExplicitDir(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Auth", "overview": "login", "specs_dir": specsDir,
	})
	specDir := call.SpecState.GetSpecDir()

	freshCall := tools.Context{SpecState: specdriven.NewSpecStore()}
	r := execTool(t, tools.SpecRead(), freshCall, map[string]any{"spec_dir": specDir})
	if r.IsError {
		t.Fatalf("spec_read with explicit dir failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Auth") {
		t.Error("spec_read explicit dir output missing title")
	}
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?spec_plan 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾

func TestSpecPlanCreatesPlanMd(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Auth", "overview": "login", "specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecPlan(), call, map[string]any{
		"language": "Go 1.22", "framework": "Gin", "database": "SQLite",
	})
	if r.IsError {
		t.Fatalf("spec_plan failed: %s", r.Output)
	}

	planFile := filepath.Join(call.SpecState.GetSpecDir(), "plan.md")
	content, err := os.ReadFile(planFile)
	if err != nil {
		t.Fatalf("plan.md not created: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "Go 1.22") {
		t.Error("plan.md missing language")
	}
	if !strings.Contains(s, "Phase 0: Research") {
		t.Error("plan.md missing Phase 0")
	}
	if !strings.Contains(s, "Phase 3: Implementation") {
		t.Error("plan.md missing Phase 3")
	}
}

func TestSpecPlanSetsPlanState(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Auth", "overview": "login", "specs_dir": specsDir,
	})
	execTool(t, tools.SpecPlan(), call, map[string]any{"language": "Go"})

	if call.SpecState.GetPlan() == nil {
		t.Error("spec_plan should set SpecState.currentPlan")
	}
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?spec_tasks 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?

func TestSpecTasksCreatesTasksMd(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Auth", "overview": "login", "specs_dir": specsDir,
	})
	execTool(t, tools.SpecWrite(), call, map[string]any{
		"user_stories": []map[string]any{{
			"id": "US1", "title": "Login", "description": "user login", "priority": "P1",
			"scenarios": []map[string]any{{
				"title": "Happy path", "given": "creds", "when": "submit", "then": "logged in",
			}},
		}},
	})

	r := execTool(t, tools.SpecTasks(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("spec_tasks failed: %s", r.Output)
	}

	tasksFile := filepath.Join(call.SpecState.GetSpecDir(), "tasks.md")
	content, err := os.ReadFile(tasksFile)
	if err != nil {
		t.Fatalf("tasks.md not created: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "SETUP") {
		t.Error("tasks.md missing SETUP group")
	}
	if !strings.Contains(s, "US1") {
		t.Error("tasks.md missing US1 group")
	}
	if !strings.Contains(s, "[P]") {
		t.Error("tasks.md should have parallelizable tasks")
	}

	stateContent, err := os.ReadFile(filepath.Join(call.SpecState.GetSpecDir(), ".speckit-state.json"))
	if err != nil {
		t.Fatalf("lifecycle metadata missing after spec_tasks: %v", err)
	}
	if !strings.Contains(string(stateContent), "\"state\": \"in_progress\"") {
		t.Error("spec_tasks should move lifecycle to in_progress")
	}
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?spec_status 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾

func TestSpecStatusEmptyDir(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	_ = os.MkdirAll(specsDir, 0o755)

	r := execTool(t, tools.SpecStatus(), call, map[string]any{"specs_dir": specsDir})
	if r.IsError {
		t.Fatalf("spec_status empty dir failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "No spec directories") {
		t.Errorf("spec_status on empty dir should say no specs, got: %s", r.Output)
	}
}

func TestSpecStatusNonexistentDir(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	r := execTool(t, tools.SpecStatus(), call, map[string]any{"specs_dir": filepath.Join(tmpDir, "nonexistent")})
	if !r.IsError {
		t.Error("spec_status on missing dir should fail")
	}
}

func TestSpecStatusShowsArtifacts(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Feature A", "overview": "a", "specs_dir": specsDir,
	})
	call2 := tools.Context{SpecState: specdriven.NewSpecStore()}
	execTool(t, tools.SpecInit(), call2, map[string]any{
		"title": "Feature B", "overview": "b", "specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecStatus(), tools.Context{SpecState: specdriven.NewSpecStore()}, map[string]any{"specs_dir": specsDir})
	if r.IsError {
		t.Fatalf("spec_status failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "feature-a") {
		t.Errorf("spec_status missing feature-a, got: %s", r.Output)
	}
	if !strings.Contains(r.Output, "feature-b") {
		t.Errorf("spec_status missing feature-b, got: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Lifecycle state: draft") {
		t.Error("spec_status should include lifecycle states")
	}
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?speckit lifecycle tools 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?

func TestSpecKitLifecycleStatus(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Lifecycle Demo", "overview": "demo", "specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecKitLifecycleStatus(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("speckit_lifecycle_status failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Lifecycle state: draft") {
		t.Error("expected draft lifecycle state")
	}
}

func TestSpecKitCompletePreservesArtifactsAndAddsNotes(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Complete Demo", "overview": "demo", "specs_dir": specsDir,
	})
	execTool(t, tools.SpecPlan(), call, map[string]any{"language": "Go"})
	execTool(t, tools.SpecTasks(), call, map[string]any{})

	r := execTool(t, tools.SpecKitComplete(), call, map[string]any{
		"notes":      "Implemented and verified.",
		"follow_ups": []string{"consider cleanup"},
	})
	if r.IsError {
		t.Fatalf("speckit_complete failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Lifecycle state: completed") {
		t.Error("expected completed lifecycle state")
	}
	for _, name := range []string{"spec.md", "plan.md", "tasks.md"} {
		if _, err := os.Stat(filepath.Join(call.SpecState.GetSpecDir(), name)); err != nil {
			t.Fatalf("expected %s to remain after completion: %v", name, err)
		}
	}
}

func TestSpecKitArchiveMovesCompletedSpec(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Archive Demo", "overview": "demo", "specs_dir": specsDir,
	})
	execTool(t, tools.SpecKitComplete(), call, map[string]any{})
	originalDir := call.SpecState.GetSpecDir()

	r := execTool(t, tools.SpecKitArchive(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("speckit_archive failed: %s", r.Output)
	}
	if _, err := os.Stat(originalDir); !os.IsNotExist(err) {
		t.Fatalf("original dir should be moved, stat err=%v", err)
	}
	if !strings.Contains(call.SpecState.GetSpecDir(), filepath.Join("specs", "archive")) && !strings.Contains(call.SpecState.GetSpecDir(), "archive") {
		t.Error("session spec dir should update to archive location")
	}
	if !strings.Contains(r.Output, "Lifecycle state: archived") {
		t.Error("expected archived lifecycle state")
	}
}

func TestSpecKitArchiveRejectsNonCompletedWithoutForce(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Archive Reject", "overview": "demo", "specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecKitArchive(), call, map[string]any{})
	if !r.IsError {
		t.Fatal("speckit_archive should reject non-completed specs without force")
	}
}
func TestSpecKitArchiveForceWarnsForNonCompletedSpec(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Archive Force", "overview": "demo", "specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecKitArchive(), call, map[string]any{"force": true})
	if r.IsError {
		t.Fatalf("speckit_archive force should archive non-completed spec: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Warning: archiving spec from draft state") {
		t.Errorf("expected force warning, got: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Lifecycle state: archived") {
		t.Errorf("expected archived lifecycle state, got: %s", r.Output)
	}
}

func TestSpecKitCompleteHandlesMissingPlanAndTasks(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Missing Artifacts", "overview": "demo", "specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecKitComplete(), call, map[string]any{"notes": "completed with deferred artifacts"})
	if r.IsError {
		t.Fatalf("speckit_complete should tolerate missing plan/tasks: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Lifecycle state: completed") {
		t.Errorf("expected completed lifecycle state, got: %s", r.Output)
	}
	if !strings.Contains(r.Output, "plan.md") || !strings.Contains(r.Output, "tasks.md") {
		t.Errorf("expected missing artifact summary for plan/tasks, got: %s", r.Output)
	}
}

func TestSpecKitArchiveUsesUniqueDestination(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Conflict Demo", "overview": "demo", "specs_dir": specsDir,
	})
	execTool(t, tools.SpecKitComplete(), call, map[string]any{})
	base := filepath.Base(call.SpecState.GetSpecDir())
	conflictDir := filepath.Join(specsDir, "archive", base)
	if err := os.MkdirAll(conflictDir, 0o755); err != nil {
		t.Fatalf("create conflict dir: %v", err)
	}

	r := execTool(t, tools.SpecKitArchive(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("speckit_archive with conflict failed: %s", r.Output)
	}
	if call.SpecState.GetSpecDir() == conflictDir {
		t.Error("archive should choose a unique destination when conflict exists")
	}
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?Full pipeline 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾

func TestFullSpecPipeline(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	r := execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Payment Processing", "overview": "Handle payments.", "specs_dir": specsDir,
	})
	if r.IsError {
		t.Fatalf("init: %s", r.Output)
	}

	r = execTool(t, tools.SpecWrite(), call, map[string]any{
		"user_stories": []map[string]any{{
			"id": "US1", "title": "Make Payment", "description": "user pays", "priority": "P1",
			"scenarios": []map[string]any{{
				"title": "Card payment", "given": "valid card", "when": "checkout", "then": "charged",
			}},
		}},
		"requirements": []map[string]any{{
			"id": "FR-001", "user_story_id": "US1", "text": "The system SHALL process credit cards",
		}},
		"entities": []map[string]any{{
			"name": "Payment", "description": "A financial transaction", "fields": []string{"id", "amount", "status"},
		}},
		"edge_cases": []map[string]any{{
			"id": "EC-001", "description": "Declined card", "expected": "Return error to user",
		}},
	})
	if r.IsError {
		t.Fatalf("write: %s", r.Output)
	}

	r = execTool(t, tools.SpecPlan(), call, map[string]any{
		"language": "Go 1.22", "framework": "Gin", "database": "Postgres",
	})
	if r.IsError {
		t.Fatalf("plan: %s", r.Output)
	}

	r = execTool(t, tools.SpecTasks(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("tasks: %s", r.Output)
	}

	r = execTool(t, tools.SpecRead(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("read: %s", r.Output)
	}
	for _, artifact := range []string{"spec.md", "plan.md", "tasks.md"} {
		if !strings.Contains(r.Output, artifact) {
			t.Errorf("expected artifact %s in spec_read output:\n%s", artifact, r.Output)
		}
	}

	r = execTool(t, tools.SpecStatus(), call, map[string]any{"specs_dir": specsDir})
	if r.IsError {
		t.Fatalf("status: %s", r.Output)
	}
	if !strings.Contains(r.Output, "payment-processing") {
		t.Errorf("spec_status missing payment-processing, got:\n%s", r.Output)
	}
	if !strings.Contains(r.Output, "Lifecycle state: in_progress") {
		t.Error("expected in_progress lifecycle after generating tasks")
	}
}

// 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋?Tool interface compliance 闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾闁冲厜鍋撻柍鍏夊亾

func TestSpecToolsHaveSchemas(t *testing.T) {
	specTools := []tools.Tool{
		tools.SpecInit(), tools.SpecWrite(), tools.SpecRead(),
		tools.SpecPlan(), tools.SpecTasks(), tools.SpecStatus(),
		tools.SpecKitLifecycleStatus(), tools.SpecKitComplete(), tools.SpecKitArchive(),
	}
	for _, tool := range specTools {
		if tool.Name() == "" {
			t.Errorf("tool has empty Name()")
		}
		if tool.Description() == "" {
			t.Errorf("%s has empty Description()", tool.Name())
		}
		schema := tool.InputSchema()
		if schema == nil {
			t.Errorf("%s has nil InputSchema()", tool.Name())
		}
		if schema["type"] != "object" {
			t.Errorf("%s InputSchema type = %q, want 'object'", tool.Name(), schema["type"])
		}
	}
}

func TestSpecToolsRegistered(t *testing.T) {
	m := tools.NewManager()
	tools.RegisterCoreTools(m)
	r := m.Registry()
	tools.InitSkillMembership(r)

	specToolNames := []string{"spec_init", "spec_write", "spec_read", "spec_plan", "spec_tasks", "spec_status", "speckit_lifecycle_status", "speckit_complete", "speckit_archive"}
	for _, name := range specToolNames {
		if r.Get(name) == nil {
			t.Errorf("tool %q not registered", name)
		}
	}
}
