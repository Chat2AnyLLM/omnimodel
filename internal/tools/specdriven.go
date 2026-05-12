package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"omnillm/internal/specdriven"
)

// ─── spec_init ────────────────────────────────────────────────────────────────

type specInitTool struct{}

// SpecInit returns the spec_init tool which creates a new spec directory and
// populates an initial spec.md template.
func SpecInit() Tool { return &specInitTool{} }

func (t *specInitTool) Name() string { return "spec_init" }
func (t *specInitTool) Description() string {
	return "Initialise a new spec-driven feature. Creates a numbered spec directory " +
		"(e.g. specs/001-user-auth/) with an empty spec.md template ready for editing. " +
		"Call this first, then use spec_write to populate the spec, spec_plan to generate " +
		"plan.md, and spec_tasks to generate tasks.md."
}
func (t *specInitTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Human-readable feature title, e.g. 'User Authentication'.",
			},
			"overview": map[string]any{
				"type":        "string",
				"description": "1–3 sentence description of the feature and its purpose.",
			},
			"specs_dir": map[string]any{
				"type":        "string",
				"description": "Path to the specs root directory. Defaults to './specs'.",
			},
		},
		"required": []string{"title", "overview"},
	}
}
func (t *specInitTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Title    string `json:"title"`
		Overview string `json:"overview"`
		SpecsDir string `json:"specs_dir"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if strings.TrimSpace(p.Title) == "" {
		return Result{Output: "error: title is required", IsError: true}
	}
	specsRoot := strings.TrimSpace(p.SpecsDir)
	if specsRoot == "" {
		specsRoot = "specs"
	}

	// Determine next feature number.
	number, err := nextSpecNumber(specsRoot)
	if err != nil {
		return Result{Output: "error determining spec number: " + err.Error(), IsError: true}
	}

	slug := specdriven.Slugify(p.Title)
	spec := &specdriven.Spec{
		Number:    number,
		Slug:      slug,
		Title:     p.Title,
		Overview:  p.Overview,
		CreatedAt: specdriven.NowISO(),
	}

	dirPath := filepath.Join(specsRoot, spec.DirName())
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return Result{Output: "error creating spec dir: " + err.Error(), IsError: true}
	}

	specFile := filepath.Join(dirPath, "spec.md")
	content := specdriven.RenderSpec(spec)
	if err := os.WriteFile(specFile, []byte(content), 0o644); err != nil {
		return Result{Output: "error writing spec.md: " + err.Error(), IsError: true}
	}

	// Persist in session.
	if call.SpecState != nil {
		call.SpecState.SetSpec(spec)
		call.SpecState.SetSpecDir(dirPath)
	}

	return Result{
		Title:  fmt.Sprintf("Spec initialised: %s", spec.DirName()),
		Output: fmt.Sprintf("Created %s\n\nNext steps:\n  1. Use spec_write to add user stories, requirements, and entities.\n  2. Use spec_plan to generate plan.md.\n  3. Use spec_tasks to generate tasks.md.", specFile),
	}
}

// ─── spec_write ───────────────────────────────────────────────────────────────

type specWriteTool struct{}

// SpecWrite returns the spec_write tool which writes structured spec content.
func SpecWrite() Tool { return &specWriteTool{} }

func (t *specWriteTool) Name() string { return "spec_write" }
func (t *specWriteTool) Description() string {
	return "Write or overwrite a spec.md with structured spec-driven content. " +
		"Provide user stories (with Given-When-Then scenarios), functional requirements " +
		"(SHALL/MUST language), key entities, and edge cases. " +
		"Call spec_init first to create the directory."
}
func (t *specWriteTool) InputSchema() map[string]any {
	scenarioSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
			"given": map[string]any{"type": "string"},
			"when":  map[string]any{"type": "string"},
			"then":  map[string]any{"type": "string"},
		},
		"required": []string{"title", "given", "when", "then"},
	}
	storySchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":           map[string]any{"type": "string", "description": "e.g. US1"},
			"title":        map[string]any{"type": "string"},
			"description":  map[string]any{"type": "string"},
			"priority":     map[string]any{"type": "string", "enum": []string{"P1", "P2", "P3"}},
			"why_priority": map[string]any{"type": "string"},
			"scenarios":    map[string]any{"type": "array", "items": scenarioSchema},
		},
		"required": []string{"id", "title", "description", "priority"},
	}
	reqSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":                  map[string]any{"type": "string", "description": "e.g. FR-001"},
			"user_story_id":       map[string]any{"type": "string"},
			"text":                map[string]any{"type": "string", "description": "The system SHALL/MUST ..."},
			"needs_clarification": map[string]any{"type": "boolean"},
		},
		"required": []string{"id", "user_story_id", "text"},
	}
	entitySchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"fields":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []string{"name", "description"},
	}
	edgeSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          map[string]any{"type": "string", "description": "e.g. EC-001"},
			"description": map[string]any{"type": "string"},
			"expected":    map[string]any{"type": "string"},
		},
		"required": []string{"id", "description", "expected"},
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"spec_dir":     map[string]any{"type": "string", "description": "Path to the spec directory, e.g. specs/001-user-auth. If omitted, uses the current session spec."},
			"user_stories": map[string]any{"type": "array", "items": storySchema},
			"requirements": map[string]any{"type": "array", "items": reqSchema},
			"entities":     map[string]any{"type": "array", "items": entitySchema},
			"edge_cases":   map[string]any{"type": "array", "items": edgeSchema},
		},
	}
}
func (t *specWriteTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir     string `json:"spec_dir"`
		UserStories []struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Priority    string `json:"priority"`
			WhyPriority string `json:"why_priority"`
			Scenarios   []struct {
				Title string `json:"title"`
				Given string `json:"given"`
				When  string `json:"when"`
				Then  string `json:"then"`
			} `json:"scenarios"`
		} `json:"user_stories"`
		Requirements []struct {
			ID                 string `json:"id"`
			UserStoryID        string `json:"user_story_id"`
			Text               string `json:"text"`
			NeedsClarification bool   `json:"needs_clarification"`
		} `json:"requirements"`
		Entities []struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Fields      []string `json:"fields"`
		} `json:"entities"`
		EdgeCases []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Expected    string `json:"expected"`
		} `json:"edge_cases"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}

	specDir := p.SpecDir
	if specDir == "" && call.SpecState != nil {
		specDir = call.SpecState.GetSpecDir()
	}
	if specDir == "" {
		return Result{Output: "error: spec_dir is required (or call spec_init first)", IsError: true}
	}

	// Load existing spec.md to get header fields.
	existing := loadSpecHeader(filepath.Join(specDir, "spec.md"))

	// Build updated spec.
	spec := &specdriven.Spec{
		Number:    existing.Number,
		Slug:      existing.Slug,
		Title:     existing.Title,
		Overview:  existing.Overview,
		CreatedAt: existing.CreatedAt,
	}

	for _, us := range p.UserStories {
		story := specdriven.UserStory{
			ID:          us.ID,
			Title:       us.Title,
			Description: us.Description,
			Priority:    specdriven.Priority(us.Priority),
			WhyPriority: us.WhyPriority,
		}
		for _, sc := range us.Scenarios {
			story.Scenarios = append(story.Scenarios, specdriven.Scenario{
				Title: sc.Title,
				Given: sc.Given,
				When:  sc.When,
				Then:  sc.Then,
			})
		}
		spec.UserStories = append(spec.UserStories, story)
	}

	for _, r := range p.Requirements {
		spec.Requirements = append(spec.Requirements, specdriven.Requirement{
			ID:                 r.ID,
			UserStoryID:        r.UserStoryID,
			Text:               r.Text,
			NeedsClarification: r.NeedsClarification,
		})
	}

	for _, e := range p.Entities {
		spec.Entities = append(spec.Entities, specdriven.Entity{
			Name:        e.Name,
			Description: e.Description,
			Fields:      e.Fields,
		})
	}

	for _, ec := range p.EdgeCases {
		spec.EdgeCases = append(spec.EdgeCases, specdriven.EdgeCase{
			ID:          ec.ID,
			Description: ec.Description,
			Expected:    ec.Expected,
		})
	}

	content := specdriven.RenderSpec(spec)
	specFile := filepath.Join(specDir, "spec.md")
	if err := os.WriteFile(specFile, []byte(content), 0o644); err != nil {
		return Result{Output: "error writing spec.md: " + err.Error(), IsError: true}
	}

	if call.SpecState != nil {
		call.SpecState.SetSpec(spec)
		call.SpecState.SetSpecDir(specDir)
	}

	summary := fmt.Sprintf("spec.md updated: %d user stories, %d requirements, %d entities, %d edge cases.",
		len(spec.UserStories), len(spec.Requirements), len(spec.Entities), len(spec.EdgeCases))
	return Result{Title: "Spec written", Output: summary}
}

// ─── spec_read ────────────────────────────────────────────────────────────────

type specReadTool struct{}

// SpecRead returns the spec_read tool which reads and displays a spec.md.
func SpecRead() Tool { return &specReadTool{} }

func (t *specReadTool) Name() string { return "spec_read" }
func (t *specReadTool) Description() string {
	return "Read and display spec.md for a given spec directory. " +
		"Shows the full spec content and artifact status (which of spec/plan/tasks are present). " +
		"Use this to review the current spec before planning or implementing."
}
func (t *specReadTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"spec_dir": map[string]any{
				"type":        "string",
				"description": "Path to the spec directory. If omitted, uses the current session spec.",
			},
		},
	}
}
func (t *specReadTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir string `json:"spec_dir"`
	}
	_ = json.Unmarshal(input, &p)

	specDir := p.SpecDir
	if specDir == "" && call.SpecState != nil {
		specDir = call.SpecState.GetSpecDir()
	}
	if specDir == "" {
		return Result{Output: "error: spec_dir is required (or call spec_init first)", IsError: true}
	}

	content, err := os.ReadFile(filepath.Join(specDir, "spec.md"))
	if err != nil {
		return Result{Output: "error reading spec.md: " + err.Error(), IsError: true}
	}

	// Artifact status.
	present := map[specdriven.ArtifactKind]bool{}
	for _, kind := range []specdriven.ArtifactKind{specdriven.ArtifactSpec, specdriven.ArtifactPlan, specdriven.ArtifactTasks} {
		fname := string(kind) + ".md"
		if _, err := os.Stat(filepath.Join(specDir, fname)); err == nil {
			present[kind] = true
		}
	}

	status := specdriven.RenderStatus(specDir, present)
	output := status + "\n---\n\n" + string(content)
	return Result{Title: "spec.md", Output: output}
}

// ─── spec_plan ────────────────────────────────────────────────────────────────

type specPlanTool struct{}

// SpecPlan returns the spec_plan tool which scaffolds plan.md from spec.md.
func SpecPlan() Tool { return &specPlanTool{} }

func (t *specPlanTool) Name() string { return "spec_plan" }
func (t *specPlanTool) Description() string {
	return "Generate a plan.md scaffold from the current spec. " +
		"Provide the technical context (language, framework, database) and the plan is " +
		"created with standard phases: Phase 0 Research, Phase 1 Design, Phase 2 Setup, " +
		"Phase 3 Implementation. Requires spec.md to exist. " +
		"Edit plan.md to add data model, API contracts, and phase details."
}
func (t *specPlanTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"spec_dir": map[string]any{
				"type":        "string",
				"description": "Path to the spec directory. If omitted, uses the current session spec.",
			},
			"language":    map[string]any{"type": "string", "description": "e.g. 'Go 1.22'"},
			"framework":   map[string]any{"type": "string", "description": "e.g. 'Gin', 'Echo', 'net/http'"},
			"database":    map[string]any{"type": "string", "description": "e.g. 'SQLite', 'Postgres'"},
			"deployment":  map[string]any{"type": "string", "description": "e.g. 'Docker', 'bare metal'"},
			"perf_goals":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"constraints": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
}
func (t *specPlanTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir     string   `json:"spec_dir"`
		Language    string   `json:"language"`
		Framework   string   `json:"framework"`
		Database    string   `json:"database"`
		Deployment  string   `json:"deployment"`
		PerfGoals   []string `json:"perf_goals"`
		Constraints []string `json:"constraints"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}

	specDir := p.SpecDir
	if specDir == "" && call.SpecState != nil {
		specDir = call.SpecState.GetSpecDir()
	}
	if specDir == "" {
		return Result{Output: "error: spec_dir is required (or call spec_init first)", IsError: true}
	}

	existing := loadSpecHeader(filepath.Join(specDir, "spec.md"))

	plan := &specdriven.Plan{
		SpecNumber: existing.Number,
		SpecSlug:   existing.Slug,
		Title:      existing.Title,
		TechCtx: specdriven.TechnicalContext{
			Language:     p.Language,
			Framework:    p.Framework,
			Database:     p.Database,
			Deployment:   p.Deployment,
			PerformGoals: p.PerfGoals,
			Constraints:  p.Constraints,
		},
		Phases: []specdriven.PlanPhase{
			{Phase: specdriven.PhaseResearch, Deliverable: []string{"research.md — resolves all NEEDS CLARIFICATION items from spec.md"}, Notes: ""},
			{Phase: specdriven.PhaseDesign, Deliverable: []string{"data-model.md", "contracts/rest-api.md", "quickstart.md"}, Notes: ""},
			{Phase: specdriven.PhaseSetup, Deliverable: []string{"Project skeleton", "Dependency manifest", "CI configuration"}, Notes: ""},
			{Phase: specdriven.PhaseImplement, Deliverable: []string{"Source code per user story", "Tests per acceptance scenario"}, Notes: ""},
		},
		DataModel: existing.Entities,
		CreatedAt: specdriven.NowISO(),
	}

	content := specdriven.RenderPlan(plan)
	planFile := filepath.Join(specDir, "plan.md")
	if err := os.WriteFile(planFile, []byte(content), 0o644); err != nil {
		return Result{Output: "error writing plan.md: " + err.Error(), IsError: true}
	}

	if call.SpecState != nil {
		call.SpecState.SetPlan(plan)
	}

	return Result{
		Title:  "Plan scaffolded",
		Output: fmt.Sprintf("Created %s\n\nNext steps:\n  1. Edit plan.md to fill in data model, API contracts, and research findings.\n  2. Use spec_tasks to generate tasks.md.", planFile),
	}
}

// ─── spec_tasks ───────────────────────────────────────────────────────────────

type specTasksTool struct{}

// SpecTasks returns the spec_tasks tool which generates tasks.md from the spec.
func SpecTasks() Tool { return &specTasksTool{} }

func (t *specTasksTool) Name() string { return "spec_tasks" }
func (t *specTasksTool) Description() string {
	return "Generate tasks.md from spec.md (and optionally plan.md). " +
		"Produces an atomic task breakdown grouped by user story, with parallelizable tasks " +
		"marked [P]. Format: '[ ] T001 [P] [US1] Description — src/path'. " +
		"Requires spec.md (and ideally plan.md) to exist."
}
func (t *specTasksTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"spec_dir": map[string]any{
				"type":        "string",
				"description": "Path to the spec directory. If omitted, uses the current session spec.",
			},
		},
	}
}
func (t *specTasksTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir string `json:"spec_dir"`
	}
	_ = json.Unmarshal(input, &p)

	specDir := p.SpecDir
	if specDir == "" && call.SpecState != nil {
		specDir = call.SpecState.GetSpecDir()
	}
	if specDir == "" {
		return Result{Output: "error: spec_dir is required (or call spec_init first)", IsError: true}
	}

	// Prefer the full spec held in session state (has user stories + scenarios);
	// fall back to the file header parse (number/title only) when running without
	// prior spec_init/spec_write in the same session.
	var spec *specdriven.Spec
	if call.SpecState != nil {
		spec = call.SpecState.GetSpec()
	}
	if spec == nil {
		spec = loadSpecHeader(filepath.Join(specDir, "spec.md"))
	}
	groups := specdriven.ScaffoldTaskGroups(spec)

	content := specdriven.RenderTasks(spec.Number, spec.Title, groups)
	tasksFile := filepath.Join(specDir, "tasks.md")
	if err := os.WriteFile(tasksFile, []byte(content), 0o644); err != nil {
		return Result{Output: "error writing tasks.md: " + err.Error(), IsError: true}
	}

	total := 0
	for _, g := range groups {
		total += len(g.Tasks)
	}
	return Result{
		Title:  "Tasks generated",
		Output: fmt.Sprintf("Created %s with %d tasks in %d groups.\n\nNext: implement tasks in order, marking each [x] when done.", tasksFile, total, len(groups)),
	}
}

// ─── spec_status ──────────────────────────────────────────────────────────────

type specStatusTool struct{}

// SpecStatus returns the spec_status tool which shows all specs and their status.
func SpecStatus() Tool { return &specStatusTool{} }

func (t *specStatusTool) Name() string { return "spec_status" }
func (t *specStatusTool) Description() string {
	return "Show the status of all specs in the specs directory. " +
		"Lists each spec with which artifacts (spec.md, plan.md, tasks.md) are present."
}
func (t *specStatusTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"specs_dir": map[string]any{
				"type":        "string",
				"description": "Path to the specs root directory. Defaults to './specs'.",
			},
		},
	}
}
func (t *specStatusTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p struct {
		SpecsDir string `json:"specs_dir"`
	}
	_ = json.Unmarshal(input, &p)
	specsRoot := p.SpecsDir
	if specsRoot == "" {
		specsRoot = "specs"
	}

	entries, err := os.ReadDir(specsRoot)
	if err != nil {
		return Result{Output: fmt.Sprintf("Cannot read %s: %v", specsRoot, err), IsError: true}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Specs in %s:\n\n", specsRoot))
	found := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(specsRoot, entry.Name())
		present := map[specdriven.ArtifactKind]bool{}
		for _, kind := range []specdriven.ArtifactKind{specdriven.ArtifactSpec, specdriven.ArtifactPlan, specdriven.ArtifactTasks} {
			fname := string(kind) + ".md"
			if _, err := os.Stat(filepath.Join(dirPath, fname)); err == nil {
				present[kind] = true
			}
		}
		sb.WriteString(specdriven.RenderStatus(entry.Name(), present))
		sb.WriteString("\n")
		found++
	}
	if found == 0 {
		sb.WriteString("No spec directories found. Use spec_init to create one.\n")
	}
	return Result{Title: "Spec status", Output: strings.TrimRight(sb.String(), "\n")}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// nextSpecNumber scans the specs directory and returns the next zero-padded number.
func nextSpecNumber(specsRoot string) (string, error) {
	entries, err := os.ReadDir(specsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "001", nil
		}
		return "", err
	}
	max := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) >= 3 {
			var n int
			_, err := fmt.Sscanf(name[:3], "%d", &n)
			if err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("%03d", max+1), nil
}

// loadSpecHeader reads a spec.md and extracts the minimal header fields
// (Number, Slug, Title, Overview, CreatedAt, Entities) without full parsing.
// Returns a zero-value Spec with safe defaults if the file cannot be read.
func loadSpecHeader(specFile string) *specdriven.Spec {
	content, err := os.ReadFile(specFile)
	if err != nil {
		return &specdriven.Spec{Number: "001", CreatedAt: specdriven.NowISO()}
	}

	spec := &specdriven.Spec{}
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "# Spec: ") {
			parts := strings.SplitN(strings.TrimPrefix(line, "# Spec: "), " ", 2)
			if len(parts) == 2 {
				spec.Number = parts[0]
				spec.Title = parts[1]
				spec.Slug = specdriven.Slugify(spec.Title)
			}
		} else if strings.HasPrefix(line, "**Created**: ") {
			spec.CreatedAt = strings.TrimPrefix(line, "**Created**: ")
		} else if line == "## Overview" && i+2 < len(lines) {
			spec.Overview = strings.TrimSpace(lines[i+2])
		}
	}
	if spec.CreatedAt == "" {
		spec.CreatedAt = specdriven.NowISO()
	}
	return spec
}
