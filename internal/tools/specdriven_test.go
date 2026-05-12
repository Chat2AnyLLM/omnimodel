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

// ─── helpers ─────────────────────────────────────────────────────────────────

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

// ─── spec_init ────────────────────────────────────────────────────────────────

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

	// Directory should exist
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

	// spec.md should exist and contain the title
	specFile := filepath.Join(specsDir, entries[0].Name(), "spec.md")
	content, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatalf("spec.md not created: %v", err)
	}
	if !strings.Contains(string(content), "User Authentication") {
		t.Error("spec.md missing title")
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

// ─── spec_write ───────────────────────────────────────────────────────────────

func TestSpecWriteUpdatesFile(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	// First init
	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Auth", "overview": "login", "specs_dir": specsDir,
	})
	specDir := call.SpecState.GetSpecDir()

	// Then write stories
	r := execTool(t, tools.SpecWrite(), call, map[string]any{
		"user_stories": []map[string]any{
			{
				"id": "US1", "title": "Login", "description": "As a user I want to log in",
				"priority": "P1", "why_priority": "MVP core",
				"scenarios": []map[string]any{
					{"title": "Happy path", "given": "valid creds", "when": "form submitted", "then": "user authenticated"},
				},
			},
		},
		"requirements": []map[string]any{
			{"id": "FR-001", "user_story_id": "US1", "text": "The system SHALL validate passwords"},
		},
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

// ─── spec_read ────────────────────────────────────────────────────────────────

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
}

func TestSpecReadShowsArtifactStatus(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Auth", "overview": "login", "specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecRead(), call, map[string]any{})
	// spec.md is present (✓), plan/tasks are not (○)
	if !strings.Contains(r.Output, "✓") {
		t.Error("spec_read should show ✓ for spec.md")
	}
	if !strings.Contains(r.Output, "○") {
		t.Error("spec_read should show ○ for missing plan/tasks")
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

	// Read via explicit dir on a fresh context (no session state)
	freshCall := tools.Context{SpecState: specdriven.NewSpecStore()}
	r := execTool(t, tools.SpecRead(), freshCall, map[string]any{"spec_dir": specDir})
	if r.IsError {
		t.Fatalf("spec_read with explicit dir failed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "Auth") {
		t.Error("spec_read explicit dir output missing title")
	}
}

// ─── spec_plan ────────────────────────────────────────────────────────────────

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

// ─── spec_tasks ───────────────────────────────────────────────────────────────

func TestSpecTasksCreatesTasksMd(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Auth", "overview": "login", "specs_dir": specsDir,
	})
	execTool(t, tools.SpecWrite(), call, map[string]any{
		"user_stories": []map[string]any{
			{"id": "US1", "title": "Login", "description": "user login", "priority": "P1",
				"scenarios": []map[string]any{
					{"title": "Happy path", "given": "creds", "when": "submit", "then": "logged in"},
				},
			},
		},
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
}

// ─── spec_status ──────────────────────────────────────────────────────────────

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

	// Create two specs
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
}

// ─── Full pipeline ────────────────────────────────────────────────────────────

func TestFullSpecPipeline(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	// 1. Init
	r := execTool(t, tools.SpecInit(), call, map[string]any{
		"title": "Payment Processing", "overview": "Handle payments.", "specs_dir": specsDir,
	})
	if r.IsError {
		t.Fatalf("init: %s", r.Output)
	}

	// 2. Write
	r = execTool(t, tools.SpecWrite(), call, map[string]any{
		"user_stories": []map[string]any{
			{
				"id": "US1", "title": "Make Payment", "description": "user pays", "priority": "P1",
				"scenarios": []map[string]any{
					{"title": "Card payment", "given": "valid card", "when": "checkout", "then": "charged"},
				},
			},
		},
		"requirements": []map[string]any{
			{"id": "FR-001", "user_story_id": "US1", "text": "The system SHALL process credit cards"},
		},
		"entities": []map[string]any{
			{"name": "Payment", "description": "A financial transaction", "fields": []string{"id", "amount", "status"}},
		},
		"edge_cases": []map[string]any{
			{"id": "EC-001", "description": "Declined card", "expected": "Return error to user"},
		},
	})
	if r.IsError {
		t.Fatalf("write: %s", r.Output)
	}

	// 3. Plan
	r = execTool(t, tools.SpecPlan(), call, map[string]any{
		"language": "Go 1.22", "framework": "Gin", "database": "Postgres",
	})
	if r.IsError {
		t.Fatalf("plan: %s", r.Output)
	}

	// 4. Tasks
	r = execTool(t, tools.SpecTasks(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("tasks: %s", r.Output)
	}

	// 5. Read — all three artifacts should now be present
	r = execTool(t, tools.SpecRead(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("read: %s", r.Output)
	}
	checkCount := strings.Count(r.Output, "✓")
	if checkCount < 3 {
		t.Errorf("expected at least 3 ✓ (spec, plan, tasks), got %d in:\n%s", checkCount, r.Output)
	}

	// 6. Status — spec_status should list the directory
	r = execTool(t, tools.SpecStatus(), call, map[string]any{"specs_dir": specsDir})
	if r.IsError {
		t.Fatalf("status: %s", r.Output)
	}
	if !strings.Contains(r.Output, "payment-processing") {
		t.Errorf("spec_status missing payment-processing, got:\n%s", r.Output)
	}
}

// ─── Tool interface compliance ────────────────────────────────────────────────

func TestSpecToolsHaveSchemas(t *testing.T) {
	specTools := []tools.Tool{
		tools.SpecInit(), tools.SpecWrite(), tools.SpecRead(),
		tools.SpecPlan(), tools.SpecTasks(), tools.SpecStatus(),
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

	specToolNames := []string{"spec_init", "spec_write", "spec_read", "spec_plan", "spec_tasks", "spec_status"}
	for _, name := range specToolNames {
		if r.Get(name) == nil {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestSpecSkillActivation(t *testing.T) {
	m := tools.NewManager()
	tools.RegisterCoreTools(m)
	r := m.Registry()
	tools.InitSkillMembership(r)

	// Before activation: spec tools should not appear in Definitions()
	defs := r.Definitions()
	for _, d := range defs {
		if strings.HasPrefix(d.Name, "spec_") {
			t.Errorf("spec tool %q visible before skill activation", d.Name)
		}
	}

	// After activation: spec tools should appear
	r.ActivateSkill(tools.SkillSpec)
	defs = r.Definitions()
	specCount := 0
	for _, d := range defs {
		if strings.HasPrefix(d.Name, "spec_") {
			specCount++
		}
	}
	if specCount != 6 {
		t.Errorf("expected 6 spec tools after activation, got %d", specCount)
	}
}
