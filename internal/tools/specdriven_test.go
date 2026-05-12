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

// ─── helpers ──────────────────────────────────────────────────────────────────

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

// ─── speckit_specify ──────────────────────────────────────────────────────────

func TestSpecKitSpecifyCreatesDirectory(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	result := execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "Allow users to sign in and out securely.",
		"title":     "User Authentication",
		"specs_dir": specsDir,
	})

	if result.IsError {
		t.Fatalf("speckit_specify failed: %s", result.Output)
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

func TestSpecKitSpecifyNumbersSequentially(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	for _, title := range []string{"Feature One", "Feature Two", "Feature Three"} {
		r := execTool(t, tools.SpecKitSpecify(), call, map[string]any{
			"feature":   "overview for " + title,
			"title":     title,
			"specs_dir": specsDir,
		})
		if r.IsError {
			t.Fatalf("speckit_specify %q failed: %s", title, r.Output)
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

func TestSpecKitSpecifyMissingFeature(t *testing.T) {
	call, _ := newSpecCtx(t)
	r := execTool(t, tools.SpecKitSpecify(), call, map[string]any{"title": "x"})
	if !r.IsError {
		t.Error("speckit_specify with empty feature should fail")
	}
}

func TestSpecKitSpecifySetsSessionState(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "Store photos.",
		"title":     "Photo Album",
		"specs_dir": specsDir,
	})

	if call.SpecState.GetSpec() == nil {
		t.Error("speckit_specify should set SpecState.currentSpec")
	}
	if call.SpecState.GetSpecDir() == "" {
		t.Error("speckit_specify should set SpecState.specDir")
	}
}

// ─── speckit_plan ─────────────────────────────────────────────────────────────

func TestSpecKitPlanCreatesPlanMd(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "login",
		"title":     "Auth",
		"specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecKitPlan(), call, map[string]any{
		"language": "Go 1.22", "framework": "Gin", "database": "SQLite",
	})
	if r.IsError {
		t.Fatalf("speckit_plan failed: %s", r.Output)
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

func TestSpecKitPlanSetsPlanState(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "login",
		"title":     "Auth",
		"specs_dir": specsDir,
	})
	execTool(t, tools.SpecKitPlan(), call, map[string]any{"language": "Go"})

	if call.SpecState.GetPlan() == nil {
		t.Error("speckit_plan should set SpecState.currentPlan")
	}
}

// ─── speckit_tasks ────────────────────────────────────────────────────────────

func TestSpecKitTasksCreatesTasksMd(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "login",
		"title":     "Auth",
		"specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecKitTasks(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("speckit_tasks failed: %s", r.Output)
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
	if !strings.Contains(s, "[P]") {
		t.Error("tasks.md should have parallelizable tasks")
	}

	stateContent, err := os.ReadFile(filepath.Join(call.SpecState.GetSpecDir(), ".speckit-state.json"))
	if err != nil {
		t.Fatalf("lifecycle metadata missing after speckit_tasks: %v", err)
	}
	if !strings.Contains(string(stateContent), "\"state\": \"in_progress\"") {
		t.Error("speckit_tasks should move lifecycle to in_progress")
	}
}

// ─── speckit lifecycle tools ──────────────────────────────────────────────────

func TestSpecKitLifecycleStatus(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "demo",
		"title":     "Lifecycle Demo",
		"specs_dir": specsDir,
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
	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "demo",
		"title":     "Complete Demo",
		"specs_dir": specsDir,
	})
	execTool(t, tools.SpecKitPlan(), call, map[string]any{"language": "Go"})
	execTool(t, tools.SpecKitTasks(), call, map[string]any{})

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
	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "demo",
		"title":     "Archive Demo",
		"specs_dir": specsDir,
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
	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "demo",
		"title":     "Archive Reject",
		"specs_dir": specsDir,
	})

	r := execTool(t, tools.SpecKitArchive(), call, map[string]any{})
	if !r.IsError {
		t.Fatal("speckit_archive should reject non-completed specs without force")
	}
}
func TestSpecKitArchiveForceWarnsForNonCompletedSpec(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")
	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "demo",
		"title":     "Archive Force",
		"specs_dir": specsDir,
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
	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "demo",
		"title":     "Missing Artifacts",
		"specs_dir": specsDir,
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
	execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "demo",
		"title":     "Conflict Demo",
		"specs_dir": specsDir,
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

// ─── Full pipeline ────────────────────────────────────────────────────────────

func TestFullSpecPipeline(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specsDir := filepath.Join(tmpDir, "specs")

	r := execTool(t, tools.SpecKitSpecify(), call, map[string]any{
		"feature":   "Handle payments.",
		"title":     "Payment Processing",
		"specs_dir": specsDir,
	})
	if r.IsError {
		t.Fatalf("specify: %s", r.Output)
	}

	r = execTool(t, tools.SpecKitPlan(), call, map[string]any{
		"language": "Go 1.22", "framework": "Gin", "database": "Postgres",
	})
	if r.IsError {
		t.Fatalf("plan: %s", r.Output)
	}

	r = execTool(t, tools.SpecKitTasks(), call, map[string]any{})
	if r.IsError {
		t.Fatalf("tasks: %s", r.Output)
	}

	// Verify artifacts exist
	for _, artifact := range []string{"spec.md", "plan.md", "tasks.md"} {
		path := filepath.Join(call.SpecState.GetSpecDir(), artifact)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist: %v", artifact, err)
		}
	}

	// Check lifecycle advanced to in_progress
	stateContent, err := os.ReadFile(filepath.Join(call.SpecState.GetSpecDir(), ".speckit-state.json"))
	if err != nil {
		t.Fatalf("lifecycle metadata missing: %v", err)
	}
	if !strings.Contains(string(stateContent), "\"state\": \"in_progress\"") {
		t.Error("expected in_progress lifecycle after generating tasks")
	}
}

// ─── Tool interface compliance ────────────────────────────────────────────────

func TestSpecToolsHaveSchemas(t *testing.T) {
	specTools := []tools.Tool{
		tools.SpecKitSpecify(), tools.SpecKitPlan(), tools.SpecKitTasks(),
		tools.SpecKitLifecycleStatus(), tools.SpecKitComplete(), tools.SpecKitArchive(),
		tools.SpecKitConstitution(), tools.SpecKitClarify(), tools.SpecKitAnalyze(),
		tools.SpecKitImplement(), tools.SpecKitChecklist(),
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

	specToolNames := []string{
		"speckit_constitution", "speckit_specify", "speckit_clarify", "speckit_plan", "speckit_tasks",
		"speckit_analyze", "speckit_implement", "speckit_taskstoissues", "speckit_checklist", "speckit_lifecycle_status",
		"speckit_complete", "speckit_archive",
		"openspec_propose", "openspec_explore", "openspec_new", "openspec_continue", "openspec_ff",
		"openspec_apply", "openspec_verify", "openspec_sync", "openspec_archive", "openspec_bulk_archive",
		"openspec_onboard",
	}
	for _, name := range specToolNames {
		if r.Get(name) == nil {
			t.Errorf("tool %q not registered", name)
		}
	}
}
// ─── speckit_taskstoissues ────────────────────────────────────────────────────

// TestSpecKitTasksToIssuesDryRunParsesAndSkipsDone exercises the parser and
// dry-run path. We don't assert on which GitHub repo is detected (it depends
// on the current git remote, which may inherit from the parent repo), but we
// do assert that the output reflects the parsed tasks: T001 + T002 are queued
// for issue creation while T003 (state "[x]") is skipped. This guards the
// parseTasksMarkdown helper and the include_done default. If the host has no
// git or no GitHub remote at all, the tool errors instead — also acceptable.
func TestSpecKitTasksToIssuesDryRunRequiresGitHubRemote(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specDir := filepath.Join(tmpDir, "specs", "001-demo")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tasks := "# Tasks: 001 Demo\n\n## SETUP\n\n- [ ] **T001** First task\n- [~] **T002** [P] Second task\n- [x] **T003** Third task done\n"
	if err := os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte(tasks), 0o644); err != nil {
		t.Fatalf("write tasks.md: %v", err)
	}

	result := execTool(t, tools.SpecKitTasksToIssues(), call, map[string]any{
		"spec_dir": specDir,
		"dry_run":  true,
	})
	if result.IsError {
		if !strings.Contains(result.Output, "GitHub") && !strings.Contains(result.Output, "git remote") {
			t.Fatalf("error path: expected GitHub/git-remote error, got %q", result.Output)
		}
		return
	}
	for _, want := range []string{"Dry run: true", "[T001] First task", "[T002] Second task", "Created: 2", "Skipped (already done): 1", "Total parsed: 3"} {
		if !strings.Contains(result.Output, want) {
			t.Errorf("dry-run output missing %q\n---\n%s", want, result.Output)
		}
	}
	if strings.Contains(result.Output, "[T003]") {
		t.Errorf("done task should be skipped by default; got:\n%s", result.Output)
	}
}

func TestSpecKitTasksToIssuesMissingTasksFile(t *testing.T) {
	call, tmpDir := newSpecCtx(t)
	specDir := filepath.Join(tmpDir, "specs", "001-demo")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	result := execTool(t, tools.SpecKitTasksToIssues(), call, map[string]any{
		"spec_dir": specDir,
		"dry_run":  true,
	})
	if !result.IsError {
		t.Fatalf("expected error for missing tasks.md, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "tasks.md") {
		t.Fatalf("expected tasks.md error, got %q", result.Output)
	}
}
