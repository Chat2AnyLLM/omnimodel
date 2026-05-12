package specdriven_test

import (
	"strings"
	"sync"
	"testing"

	"omnillm/internal/specdriven"
)

// ─── Slugify ──────────────────────────────────────────────────────────────────

func TestSlugify(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"User Authentication", "user-authentication"},
		{"Photo Album App", "photo-album-app"},
		{"  Leading & Trailing  ", "leading-trailing"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"CamelCase123", "camelcase123"},
		{"already-kebab", "already-kebab"},
		{"Special!@#Characters", "special-characters"},
		{"", ""},
		// max length truncation (>48 chars)
		{"this is a very very very very very very very long title here", "this-is-a-very-very-very-very-very-very-very-lon"},
	}
	for _, tc := range cases {
		got := specdriven.Slugify(tc.input)
		if got != tc.want {
			t.Errorf("Slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSlugifyMaxLength(t *testing.T) {
	long := strings.Repeat("a", 60)
	got := specdriven.Slugify(long)
	if len(got) > 48 {
		t.Errorf("Slugify truncation: len=%d, want ≤48", len(got))
	}
}

// ─── Spec.DirName ────────────────────────────────────────────────────────────

func TestSpecDirName(t *testing.T) {
	s := &specdriven.Spec{Number: "007", Slug: "user-auth"}
	if got := s.DirName(); got != "007-user-auth" {
		t.Errorf("DirName() = %q, want %q", got, "007-user-auth")
	}
}

// ─── RenderSpec ───────────────────────────────────────────────────────────────

func TestRenderSpecTitle(t *testing.T) {
	s := &specdriven.Spec{
		Number:    "001",
		Title:     "User Auth",
		Overview:  "Allow users to sign in and out.",
		CreatedAt: "2026-05-12T00:00:00Z",
	}
	out := specdriven.RenderSpec(s)
	if !strings.Contains(out, "# Spec: 001 User Auth") {
		t.Errorf("RenderSpec missing title, got:\n%s", out)
	}
	if !strings.Contains(out, "Allow users to sign in and out.") {
		t.Errorf("RenderSpec missing overview, got:\n%s", out)
	}
	if !strings.Contains(out, "**Created**: 2026-05-12T00:00:00Z") {
		t.Errorf("RenderSpec missing created, got:\n%s", out)
	}
}

func TestRenderSpecUserStories(t *testing.T) {
	s := &specdriven.Spec{
		Number: "001",
		Title:  "User Auth",
		UserStories: []specdriven.UserStory{
			{
				ID:          "US1",
				Title:       "Login",
				Description: "As a user I want to log in",
				Priority:    specdriven.PriorityP1,
				WhyPriority: "Core MVP",
				Scenarios: []specdriven.Scenario{
					{Title: "Happy path", Given: "valid credentials", When: "user submits form", Then: "user is logged in"},
				},
			},
		},
	}
	out := specdriven.RenderSpec(s)
	if !strings.Contains(out, "### US1 – Login (P1)") {
		t.Error("RenderSpec missing user story heading")
	}
	if !strings.Contains(out, "**Why this priority**: Core MVP") {
		t.Error("RenderSpec missing why_priority")
	}
	if !strings.Contains(out, "**GIVEN** valid credentials") {
		t.Error("RenderSpec missing GIVEN")
	}
	if !strings.Contains(out, "**WHEN** user submits form") {
		t.Error("RenderSpec missing WHEN")
	}
	if !strings.Contains(out, "**THEN** user is logged in") {
		t.Error("RenderSpec missing THEN")
	}
}

func TestRenderSpecRequirements(t *testing.T) {
	s := &specdriven.Spec{
		Number: "001",
		Title:  "Auth",
		Requirements: []specdriven.Requirement{
			{ID: "FR-001", UserStoryID: "US1", Text: "The system SHALL validate credentials"},
			{ID: "FR-002", UserStoryID: "US1", Text: "The system SHALL hash passwords", NeedsClarification: true},
		},
	}
	out := specdriven.RenderSpec(s)
	if !strings.Contains(out, "**FR-001** [US1]: The system SHALL validate credentials") {
		t.Error("RenderSpec missing FR-001")
	}
	if !strings.Contains(out, "NEEDS CLARIFICATION") {
		t.Error("RenderSpec missing NEEDS CLARIFICATION marker")
	}
}

func TestRenderSpecEntities(t *testing.T) {
	s := &specdriven.Spec{
		Number: "001",
		Title:  "Auth",
		Entities: []specdriven.Entity{
			{Name: "User", Description: "An account holder", Fields: []string{"id", "email", "password_hash"}},
		},
	}
	out := specdriven.RenderSpec(s)
	if !strings.Contains(out, "### User") {
		t.Error("RenderSpec missing entity heading")
	}
	if !strings.Contains(out, "id, email, password_hash") {
		t.Error("RenderSpec missing entity fields")
	}
}

func TestRenderSpecEdgeCases(t *testing.T) {
	s := &specdriven.Spec{
		Number: "001",
		Title:  "Auth",
		EdgeCases: []specdriven.EdgeCase{
			{ID: "EC-001", Description: "Empty password", Expected: "Return 422 Unprocessable Entity"},
		},
	}
	out := specdriven.RenderSpec(s)
	if !strings.Contains(out, "**EC-001**: Empty password → Return 422 Unprocessable Entity") {
		t.Errorf("RenderSpec missing edge case, got:\n%s", out)
	}
}

func TestRenderSpecEmptyStories(t *testing.T) {
	s := &specdriven.Spec{Number: "001", Title: "Empty"}
	out := specdriven.RenderSpec(s)
	// Should not panic, should contain the heading
	if !strings.Contains(out, "# Spec: 001 Empty") {
		t.Errorf("RenderSpec empty spec missing title")
	}
}

// ─── RenderPlan ───────────────────────────────────────────────────────────────

func TestRenderPlanHeading(t *testing.T) {
	p := &specdriven.Plan{
		SpecNumber: "002",
		Title:      "Photo Album",
		CreatedAt:  "2026-05-12T00:00:00Z",
		TechCtx: specdriven.TechnicalContext{
			Language:  "Go 1.22",
			Framework: "Gin",
			Database:  "SQLite",
		},
		Phases: []specdriven.PlanPhase{
			{Phase: specdriven.PhaseResearch, Deliverable: []string{"research.md"}},
		},
	}
	out := specdriven.RenderPlan(p)
	if !strings.Contains(out, "# Plan: 002 Photo Album") {
		t.Errorf("RenderPlan missing title: %s", out)
	}
	if !strings.Contains(out, "**Language**: Go 1.22") {
		t.Error("RenderPlan missing language")
	}
	if !strings.Contains(out, "**Database**: SQLite") {
		t.Error("RenderPlan missing database")
	}
	if !strings.Contains(out, "## Phase 0: Research") {
		t.Error("RenderPlan missing phase heading")
	}
	if !strings.Contains(out, "- research.md") {
		t.Error("RenderPlan missing deliverable")
	}
}

func TestRenderPlanContracts(t *testing.T) {
	p := &specdriven.Plan{
		SpecNumber: "001",
		Title:      "Auth",
		Contracts: []specdriven.APIContract{
			{Method: "POST", Path: "/api/v1/login", Description: "Authenticate user", Response: `{"token":"string"}`},
		},
	}
	out := specdriven.RenderPlan(p)
	if !strings.Contains(out, "### POST /api/v1/login") {
		t.Error("RenderPlan missing contract heading")
	}
	if !strings.Contains(out, "Authenticate user") {
		t.Error("RenderPlan missing contract description")
	}
}

// ─── RenderTasks ─────────────────────────────────────────────────────────────

func TestRenderTasksBasic(t *testing.T) {
	groups := []specdriven.TaskGroup{
		{
			UserStoryID: "US1",
			Title:       "Login",
			Tasks: []specdriven.SpecTask{
				{ID: "T001", UserStoryID: "US1", Description: "Write tests", Status: specdriven.TaskPending},
				{ID: "T002", UserStoryID: "US1", Description: "Implement login handler", Parallelizable: true, Status: specdriven.TaskInProgress},
				{ID: "T003", UserStoryID: "US1", Description: "Add integration test", Status: specdriven.TaskDone},
			},
		},
	}
	out := specdriven.RenderTasks("001", "User Auth", groups)
	if !strings.Contains(out, "# Tasks: 001 User Auth") {
		t.Error("RenderTasks missing heading")
	}
	if !strings.Contains(out, "## US1 – Login") {
		t.Error("RenderTasks missing group heading")
	}
	if !strings.Contains(out, "[ ] **T001**") {
		t.Error("RenderTasks missing pending checkbox")
	}
	if !strings.Contains(out, "[~] **T002** [P]") {
		t.Error("RenderTasks missing in-progress + parallelizable")
	}
	if !strings.Contains(out, "[x] **T003**") {
		t.Error("RenderTasks missing done checkbox")
	}
}

func TestRenderTasksTargetPath(t *testing.T) {
	groups := []specdriven.TaskGroup{{
		UserStoryID: "US1",
		Title:       "Login",
		Tasks: []specdriven.SpecTask{
			{ID: "T001", UserStoryID: "US1", Description: "Write handler", TargetPath: "internal/auth/handler.go"},
		},
	}}
	out := specdriven.RenderTasks("001", "Auth", groups)
	if !strings.Contains(out, "— internal/auth/handler.go") {
		t.Errorf("RenderTasks missing target path, got:\n%s", out)
	}
}

// ─── ScaffoldTaskGroups ───────────────────────────────────────────────────────

func TestScaffoldTaskGroupsSetup(t *testing.T) {
	s := &specdriven.Spec{Number: "001", Title: "Auth"}
	groups := specdriven.ScaffoldTaskGroups(s)
	if len(groups) == 0 {
		t.Fatal("ScaffoldTaskGroups: expected at least setup group")
	}
	if groups[0].UserStoryID != "SETUP" {
		t.Errorf("first group should be SETUP, got %q", groups[0].UserStoryID)
	}
	if len(groups[0].Tasks) == 0 {
		t.Error("SETUP group has no tasks")
	}
}

func TestScaffoldTaskGroupsPerStory(t *testing.T) {
	s := &specdriven.Spec{
		Number: "001",
		Title:  "Auth",
		UserStories: []specdriven.UserStory{
			{ID: "US1", Title: "Login", Scenarios: []specdriven.Scenario{
				{Title: "Happy path"},
				{Title: "Wrong password"},
			}},
			{ID: "US2", Title: "Logout"},
		},
	}
	groups := specdriven.ScaffoldTaskGroups(s)
	// SETUP + 2 user stories
	if len(groups) != 3 {
		t.Errorf("expected 3 groups (SETUP+2 stories), got %d", len(groups))
	}
	us1 := groups[1]
	if us1.UserStoryID != "US1" {
		t.Errorf("group[1].UserStoryID = %q, want US1", us1.UserStoryID)
	}
	// test task + 2 scenario tasks
	if len(us1.Tasks) != 3 {
		t.Errorf("US1 should have 3 tasks (1 test + 2 scenarios), got %d", len(us1.Tasks))
	}
	// Scenario implementation tasks should be parallelizable
	if !us1.Tasks[1].Parallelizable {
		t.Error("scenario tasks should be marked parallelizable")
	}
}

func TestScaffoldTaskGroupsNoScenarios(t *testing.T) {
	s := &specdriven.Spec{
		Number:      "001",
		Title:       "Auth",
		UserStories: []specdriven.UserStory{{ID: "US1", Title: "Login"}},
	}
	groups := specdriven.ScaffoldTaskGroups(s)
	us1 := groups[1]
	// test task + 1 fallback implementation task
	if len(us1.Tasks) != 2 {
		t.Errorf("no-scenario story should have 2 tasks, got %d", len(us1.Tasks))
	}
}

func TestScaffoldTaskGroupsUniqueIDs(t *testing.T) {
	s := &specdriven.Spec{
		Number: "001",
		Title:  "Auth",
		UserStories: []specdriven.UserStory{
			{ID: "US1", Title: "Login", Scenarios: []specdriven.Scenario{{Title: "s1"}, {Title: "s2"}}},
			{ID: "US2", Title: "Logout", Scenarios: []specdriven.Scenario{{Title: "s1"}}},
		},
	}
	groups := specdriven.ScaffoldTaskGroups(s)
	seen := map[string]bool{}
	for _, g := range groups {
		for _, task := range g.Tasks {
			if seen[task.ID] {
				t.Errorf("duplicate task ID %q", task.ID)
			}
			seen[task.ID] = true
		}
	}
}

// ─── ArtifactGraph / BuildOrder ───────────────────────────────────────────────

func TestBuildOrder(t *testing.T) {
	order := specdriven.BuildOrder()
	if len(order) != 4 {
		t.Fatalf("BuildOrder: expected 4 artifacts, got %d", len(order))
	}
	// spec must come before plan, plan before tasks, tasks before code
	kinds := make([]specdriven.ArtifactKind, len(order))
	for i, a := range order {
		kinds[i] = a.Kind
	}
	wantOrder := []specdriven.ArtifactKind{
		specdriven.ArtifactSpec, specdriven.ArtifactPlan,
		specdriven.ArtifactTasks, specdriven.ArtifactCode,
	}
	for i, k := range wantOrder {
		if kinds[i] != k {
			t.Errorf("BuildOrder[%d] = %q, want %q", i, kinds[i], k)
		}
	}
}

func TestBuildOrderDependencies(t *testing.T) {
	order := specdriven.BuildOrder()
	// spec has no requirements
	if len(order[0].Requires) != 0 {
		t.Errorf("spec should have no requirements, got %v", order[0].Requires)
	}
	// plan requires spec
	if len(order[1].Requires) != 1 || order[1].Requires[0] != specdriven.ArtifactSpec {
		t.Errorf("plan should require spec, got %v", order[1].Requires)
	}
	// tasks requires plan
	if len(order[2].Requires) != 1 || order[2].Requires[0] != specdriven.ArtifactPlan {
		t.Errorf("tasks should require plan, got %v", order[2].Requires)
	}
}

// ─── RenderStatus ─────────────────────────────────────────────────────────────

func TestRenderStatusEmpty(t *testing.T) {
	out := specdriven.RenderStatus("001-auth", map[specdriven.ArtifactKind]bool{})
	if !strings.Contains(out, "001-auth") {
		t.Error("RenderStatus missing dir name")
	}
	if strings.Contains(out, "✓") {
		t.Error("RenderStatus should show no checkmarks for empty present map")
	}
	if !strings.Contains(out, "○") {
		t.Error("RenderStatus should show ○ for missing artifacts")
	}
}

func TestRenderStatusPartial(t *testing.T) {
	present := map[specdriven.ArtifactKind]bool{
		specdriven.ArtifactSpec: true,
		specdriven.ArtifactPlan: true,
	}
	out := specdriven.RenderStatus("001-auth", present)
	// spec and plan should be ✓, tasks and code ○
	lines := strings.Split(out, "\n")
	checkCount := 0
	for _, l := range lines {
		if strings.Contains(l, "✓") {
			checkCount++
		}
	}
	if checkCount != 2 {
		t.Errorf("expected 2 checkmarks, got %d in:\n%s", checkCount, out)
	}
}

// ─── SpecStore concurrency ────────────────────────────────────────────────────

func TestSpecStoreConcurrent(t *testing.T) {
	store := specdriven.NewSpecStore()
	var wg sync.WaitGroup
	spec := &specdriven.Spec{Number: "001", Title: "Auth"}
	plan := &specdriven.Plan{SpecNumber: "001", Title: "Auth"}

	for range 50 {
		wg.Add(3)
		go func() { defer wg.Done(); store.SetSpec(spec) }()
		go func() { defer wg.Done(); store.GetSpec() }()
		go func() { defer wg.Done(); store.SetPlan(plan); store.GetPlan() }()
	}
	wg.Wait()
}

func TestSpecStoreInitiallyEmpty(t *testing.T) {
	store := specdriven.NewSpecStore()
	if got := store.GetSpec(); got != nil {
		t.Errorf("new store: GetSpec() = %v, want nil", got)
	}
	if got := store.GetPlan(); got != nil {
		t.Errorf("new store: GetPlan() = %v, want nil", got)
	}
	if got := store.GetSpecDir(); got != "" {
		t.Errorf("new store: GetSpecDir() = %q, want empty", got)
	}
}

func TestSpecStoreRoundtrip(t *testing.T) {
	store := specdriven.NewSpecStore()
	spec := &specdriven.Spec{Number: "003", Slug: "photo-album", Title: "Photo Album"}
	store.SetSpec(spec)
	got := store.GetSpec()
	if got == nil || got.Number != "003" || got.Title != "Photo Album" {
		t.Errorf("SpecStore roundtrip failed: got %+v", got)
	}

	store.SetSpecDir("/tmp/specs/003-photo-album")
	if got := store.GetSpecDir(); got != "/tmp/specs/003-photo-album" {
		t.Errorf("GetSpecDir() = %q, want /tmp/specs/003-photo-album", got)
	}
}

// ─── NowISO ───────────────────────────────────────────────────────────────────

func TestNowISO(t *testing.T) {
	ts := specdriven.NowISO()
	if !strings.HasSuffix(ts, "Z") {
		t.Errorf("NowISO() = %q, want UTC Z suffix", ts)
	}
	if len(ts) != 20 { // "2006-01-02T15:04:05Z"
		t.Errorf("NowISO() = %q, unexpected length %d", ts, len(ts))
	}
}
