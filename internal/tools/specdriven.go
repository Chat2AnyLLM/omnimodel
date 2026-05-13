package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"omnillm/internal/specdriven"
)

// ─── Internal helpers ────────────────────────────────────────────────────────

// runSpecInit creates a new spec directory and populates an initial spec.md template.
// Input shape: {title, overview, specs_dir}
func runSpecInit(_ context.Context, call Context, input json.RawMessage) Result {
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
	number, err := specdriven.NextSpecNumber(specsRoot)
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
	if _, err := specdriven.EnsureLifecycle(dirPath, spec.CreatedAt, true); err != nil {
		return Result{Output: "error writing lifecycle metadata: " + err.Error(), IsError: true}
	}

	// Persist in session.
	if call.SpecState != nil {
		call.SpecState.SetSpec(spec)
		call.SpecState.SetSpecDir(dirPath)
	}

	return Result{
		Title:  fmt.Sprintf("Spec initialised: %s", spec.DirName()),
		Output: fmt.Sprintf("Created %s\n\nNext steps:\n  1. Use speckit_specify to add user stories, requirements, and entities.\n  2. Use speckit_plan to generate plan.md.\n  3. Use speckit_tasks to generate tasks.md.", specFile),
	}
}

// runSpecPlan generates plan.md from spec.md.
// Input shape: {spec_dir, language, framework, database, deployment, perf_goals, constraints}
func runSpecPlan(_ context.Context, call Context, input json.RawMessage) Result {
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
		return Result{Output: "error: spec_dir is required (or call speckit_specify first)", IsError: true}
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
	if _, err := specdriven.EnsureLifecycle(specDir, existing.CreatedAt, false); err != nil {
		return Result{Output: "error writing lifecycle metadata: " + err.Error(), IsError: true}
	}

	return Result{
		Title:  "Plan scaffolded",
		Output: fmt.Sprintf("Created %s\n\nNext steps:\n  1. Edit plan.md to fill in data model, API contracts, and research findings.\n  2. Use speckit_tasks to generate tasks.md.", planFile),
	}
}

// runSpecTasks generates tasks.md from spec.md.
// Input shape: {spec_dir}
func runSpecTasks(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir string `json:"spec_dir"`
	}
	_ = json.Unmarshal(input, &p)

	specDir := p.SpecDir
	if specDir == "" && call.SpecState != nil {
		specDir = call.SpecState.GetSpecDir()
	}
	if specDir == "" {
		return Result{Output: "error: spec_dir is required (or call speckit_specify first)", IsError: true}
	}

	// Prefer the full spec held in session state (has user stories + scenarios);
	// fall back to the file header parse (number/title only) when running without
	// prior speckit_specify in the same session.
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
	record, err := specdriven.ReadLifecycle(specDir)
	if err != nil {
		return Result{Output: "error reading lifecycle metadata: " + err.Error(), IsError: true}
	}
	if record.State == specdriven.LifecycleDraft {
		record.State = specdriven.LifecycleInProgress
		if err := specdriven.WriteLifecycle(specDir, record); err != nil {
			return Result{Output: "error writing lifecycle metadata: " + err.Error(), IsError: true}
		}
	}
	return Result{
		Title:  "Tasks generated",
		Output: fmt.Sprintf("Created %s with %d tasks in %d groups.\n\nNext: implement tasks in order, marking each [x] when done.", tasksFile, total, len(groups)),
	}
}

// ??? Spec Kit-compatible tools ???????????????????????????????????????????????

// Spec Kit-compatible tools.

type specKitConstitutionTool struct{}
type specKitSpecifyTool struct{}
type specKitClarifyTool struct{}
type specKitPlanTool struct{}
type specKitTasksTool struct{}
type specKitAnalyzeTool struct{}
type specKitImplementTool struct{}
type specKitTasksToIssuesTool struct{}
type specKitChecklistTool struct{}
type specKitLifecycleStatusTool struct{}
type specKitCompleteTool struct{}
type specKitArchiveTool struct{}

func SpecKitConstitution() Tool    { return &specKitConstitutionTool{} }
func SpecKitSpecify() Tool         { return &specKitSpecifyTool{} }
func SpecKitClarify() Tool         { return &specKitClarifyTool{} }
func SpecKitPlan() Tool            { return &specKitPlanTool{} }
func SpecKitTasks() Tool           { return &specKitTasksTool{} }
func SpecKitAnalyze() Tool         { return &specKitAnalyzeTool{} }
func SpecKitImplement() Tool       { return &specKitImplementTool{} }
func SpecKitTasksToIssues() Tool   { return &specKitTasksToIssuesTool{} }
func SpecKitChecklist() Tool       { return &specKitChecklistTool{} }
func SpecKitLifecycleStatus() Tool { return &specKitLifecycleStatusTool{} }
func SpecKitComplete() Tool        { return &specKitCompleteTool{} }
func SpecKitArchive() Tool         { return &specKitArchiveTool{} }

func (t *specKitConstitutionTool) Name() string { return "speckit_constitution" }
func (t *specKitConstitutionTool) Description() string {
	return "Spec Kit-compatible /speckit.constitution: create or update memory/constitution.md with project principles."
}
func (t *specKitConstitutionTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"principles": map[string]any{"type": "string"}, "constitution_path": map[string]any{"type": "string"}}}
}
func (t *specKitConstitutionTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p struct {
		Principles       string `json:"principles"`
		ConstitutionPath string `json:"constitution_path"`
	}
	_ = json.Unmarshal(input, &p)
	path := strings.TrimSpace(p.ConstitutionPath)
	if path == "" {
		path = filepath.Join("memory", "constitution.md")
	}
	principles := strings.TrimSpace(p.Principles)
	if principles == "" {
		principles = "TODO: define project principles, testing standards, UX consistency, and performance requirements."
	}
	content := fmt.Sprintf("# Project Constitution\n\n**Created**: %s\n\n## Principles\n\n%s\n", specdriven.NowISO(), principles)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{Output: "error creating constitution dir: " + err.Error(), IsError: true}
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Result{Output: "error writing constitution: " + err.Error(), IsError: true}
	}
	return Result{Title: "Constitution written", Output: fmt.Sprintf("Created %s", path)}
}

func (t *specKitSpecifyTool) Name() string { return "speckit_specify" }
func (t *specKitSpecifyTool) Description() string {
	return "Spec Kit-compatible /speckit.specify: create a numbered spec directory and spec.md from a natural-language feature description."
}
func (t *specKitSpecifyTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"feature": map[string]any{"type": "string"}, "title": map[string]any{"type": "string"}, "specs_dir": map[string]any{"type": "string"}}, "required": []string{"feature"}}
}
func (t *specKitSpecifyTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Feature  string `json:"feature"`
		Title    string `json:"title"`
		SpecsDir string `json:"specs_dir"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	feature := strings.TrimSpace(p.Feature)
	if feature == "" {
		return Result{Output: "error: feature is required", IsError: true}
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = deriveTitle(feature)
	}
	overview := feature
	if len(overview) > 600 {
		overview = overview[:600] + "..."
	}
	payload, _ := json.Marshal(map[string]any{"title": title, "overview": overview, "specs_dir": p.SpecsDir})
	return runSpecInit(ctx, call, payload)
}

func (t *specKitClarifyTool) Name() string { return "speckit_clarify" }
func (t *specKitClarifyTool) Description() string {
	return "Spec Kit-compatible /speckit.clarify: append clarification answers or prompts to the current spec.md."
}
func (t *specKitClarifyTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"spec_dir": map[string]any{"type": "string"}, "clarifications": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
}
func (t *specKitClarifyTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir        string   `json:"spec_dir"`
		Clarifications []string `json:"clarifications"`
	}
	_ = json.Unmarshal(input, &p)
	specDir := resolveSpecDir(call, p.SpecDir)
	if specDir == "" {
		return missingSpecDirResult("speckit_specify")
	}
	specFile := filepath.Join(specDir, "spec.md")
	if _, err := os.Stat(specFile); err != nil {
		return Result{Output: "error reading spec.md: " + err.Error(), IsError: true}
	}
	items := p.Clarifications
	if len(items) == 0 {
		items = []string{"TODO: identify and answer up to 5 targeted clarification questions."}
	}
	var sb strings.Builder
	sb.WriteString("\n## Clarifications\n\n")
	sb.WriteString(fmt.Sprintf("**Updated**: %s\n\n", specdriven.NowISO()))
	for _, c := range items {
		if strings.TrimSpace(c) != "" {
			sb.WriteString("- " + strings.TrimSpace(c) + "\n")
		}
	}
	f, err := os.OpenFile(specFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Result{Output: "error opening spec.md: " + err.Error(), IsError: true}
	}
	defer f.Close()
	if _, err := f.WriteString(sb.String()); err != nil {
		return Result{Output: "error updating spec.md: " + err.Error(), IsError: true}
	}
	return Result{Title: "Clarifications recorded", Output: fmt.Sprintf("Updated %s", specFile)}
}

func (t *specKitPlanTool) Name() string { return "speckit_plan" }
func (t *specKitPlanTool) Description() string {
	return "Spec Kit-compatible /speckit.plan: generate plan.md from spec.md."
}
func (t *specKitPlanTool) InputSchema() map[string]any {
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
func (t *specKitPlanTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	return runSpecPlan(ctx, call, input)
}
func (t *specKitTasksTool) Name() string { return "speckit_tasks" }
func (t *specKitTasksTool) Description() string {
	return "Spec Kit-compatible /speckit.tasks: generate tasks.md from spec.md and plan.md."
}
func (t *specKitTasksTool) InputSchema() map[string]any {
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
func (t *specKitTasksTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	return runSpecTasks(ctx, call, input)
}

func (t *specKitAnalyzeTool) Name() string { return "speckit_analyze" }
func (t *specKitAnalyzeTool) Description() string {
	return "Spec Kit-compatible /speckit.analyze: inspect spec.md, plan.md, and tasks.md for missing artifacts."
}
func (t *specKitAnalyzeTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"spec_dir": map[string]any{"type": "string"}}}
}
func (t *specKitAnalyzeTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir string `json:"spec_dir"`
	}
	_ = json.Unmarshal(input, &p)
	specDir := resolveSpecDir(call, p.SpecDir)
	if specDir == "" {
		return missingSpecDirResult("speckit_specify")
	}
	present := artifactPresence(specDir)
	status := specdriven.RenderStatus(specDir, present)
	issues := []string{}
	if !present[specdriven.ArtifactSpec] {
		issues = append(issues, "missing spec.md")
	}
	if !present[specdriven.ArtifactPlan] {
		issues = append(issues, "missing plan.md")
	}
	if !present[specdriven.ArtifactTasks] {
		issues = append(issues, "missing tasks.md")
	}
	if len(issues) == 0 {
		issues = append(issues, "no missing core artifacts detected")
	}
	content := status + "\n## Findings\n\n- " + strings.Join(issues, "\n- ") + "\n"
	path := filepath.Join(specDir, "analysis.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Result{Output: "error writing analysis.md: " + err.Error(), IsError: true}
	}
	return Result{Title: "Spec Kit analysis", Output: fmt.Sprintf("Created %s\n\n%s", path, strings.TrimSpace(content))}
}

func (t *specKitImplementTool) Name() string { return "speckit_implement" }
func (t *specKitImplementTool) Description() string {
	return "Spec Kit-compatible /speckit.implement: report tasks ready for implementation."
}
func (t *specKitImplementTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"spec_dir": map[string]any{"type": "string"}, "dry_run": map[string]any{"type": "boolean"}}}
}
func (t *specKitImplementTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir string `json:"spec_dir"`
		DryRun  bool   `json:"dry_run"`
	}
	_ = json.Unmarshal(input, &p)
	specDir := resolveSpecDir(call, p.SpecDir)
	if specDir == "" {
		return missingSpecDirResult("speckit_specify")
	}
	tasksFile := filepath.Join(specDir, "tasks.md")
	content, err := os.ReadFile(tasksFile)
	if err != nil {
		return Result{Output: "error reading tasks.md: " + err.Error() + "; run speckit_tasks first", IsError: true}
	}
	pending := strings.Count(string(content), "- [ ]")
	return Result{Title: "Implementation readiness", Output: fmt.Sprintf("%s contains %d pending tasks. Execute tasks in dependency order, updating checkboxes as work completes.", tasksFile, pending)}
}

// taskstoissuesTask is one parsed line from tasks.md.
type taskstoissuesTask struct {
	ID    string // e.g. "T001"
	Body  string // task description after the ID
	State string // " ", "~", "x"
}

// parseTasksMarkdown extracts task lines like "- [ ] **T001** description"
// from a tasks.md file. The format is what RenderTasks() emits.
func parseTasksMarkdown(content string) []taskstoissuesTask {
	var out []taskstoissuesTask
	for _, line := range strings.Split(content, "\n") {
		s := strings.TrimSpace(line)
		if !strings.HasPrefix(s, "- [") || len(s) < 6 {
			continue
		}
		state := string(s[3])
		rest := strings.TrimSpace(s[5:])
		// Expect: **TXXX** description ...  OR  **TXXX** [P] description
		id := ""
		body := rest
		if strings.HasPrefix(rest, "**") {
			if end := strings.Index(rest[2:], "**"); end > 0 {
				id = rest[2 : 2+end]
				body = strings.TrimSpace(rest[2+end+2:])
				body = strings.TrimPrefix(body, "[P]")
				body = strings.TrimSpace(body)
			}
		}
		if id == "" {
			continue
		}
		out = append(out, taskstoissuesTask{ID: id, Body: body, State: state})
	}
	return out
}

// detectGitHubRepo reads `git remote get-url <remote>` and returns
// "owner/repo" if the remote points at github.com, or an error otherwise.
// It avoids any network call.
func detectGitHubRepo(remote string) (string, error) {
	if remote == "" {
		remote = "origin"
	}
	cmd := exec.Command("git", "remote", "get-url", remote)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git remote get-url %s: %w", remote, err)
	}
	url := strings.TrimSpace(string(output))
	// Accept https://github.com/owner/repo(.git) and git@github.com:owner/repo(.git)
	url = strings.TrimSuffix(url, ".git")
	switch {
	case strings.HasPrefix(url, "https://github.com/"):
		path := strings.TrimPrefix(url, "https://github.com/")
		if parts := strings.SplitN(path, "/", 3); len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return parts[0] + "/" + parts[1], nil
		}
	case strings.HasPrefix(url, "git@github.com:"):
		path := strings.TrimPrefix(url, "git@github.com:")
		if parts := strings.SplitN(path, "/", 3); len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return parts[0] + "/" + parts[1], nil
		}
	}
	return "", fmt.Errorf("remote %q is not a GitHub URL: %s", remote, url)
}

func (t *specKitTasksToIssuesTool) Name() string { return "speckit_taskstoissues" }
func (t *specKitTasksToIssuesTool) Description() string {
	return "Spec Kit-compatible /speckit.taskstoissues: convert tasks.md items into GitHub issues via the `gh` CLI. Validates the git remote points at github.com, then runs `gh issue create` per pending task. Use dry_run=true to preview without creating issues."
}
func (t *specKitTasksToIssuesTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"spec_dir":     map[string]any{"type": "string", "description": "Spec directory containing tasks.md. Defaults to the active spec dir."},
			"remote":       map[string]any{"type": "string", "description": "Git remote to inspect (default: origin)."},
			"labels":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Labels to apply to each issue."},
			"assignees":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "GitHub usernames to assign to each issue."},
			"milestone":    map[string]any{"type": "string", "description": "Milestone title to attach to each issue."},
			"include_done": map[string]any{"type": "boolean", "description": "Also create issues for tasks already marked [x] (default: false)."},
			"dry_run":      map[string]any{"type": "boolean", "description": "If true, log the issues that would be created but do not run `gh`."},
		},
	}
}
func (t *specKitTasksToIssuesTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir     string   `json:"spec_dir"`
		Remote      string   `json:"remote"`
		Labels      []string `json:"labels"`
		Assignees   []string `json:"assignees"`
		Milestone   string   `json:"milestone"`
		IncludeDone bool     `json:"include_done"`
		DryRun      bool     `json:"dry_run"`
	}
	_ = json.Unmarshal(input, &p)

	specDir := resolveSpecDir(call, p.SpecDir)
	if specDir == "" {
		return missingSpecDirResult("speckit_specify")
	}
	tasksFile := filepath.Join(specDir, "tasks.md")
	content, err := os.ReadFile(tasksFile)
	if err != nil {
		return Result{Output: "error reading tasks.md: " + err.Error() + "; run speckit_tasks first", IsError: true}
	}

	repo, err := detectGitHubRepo(p.Remote)
	if err != nil {
		return Result{Output: "error: " + err.Error() + "; /speckit.taskstoissues requires a GitHub remote", IsError: true}
	}

	tasks := parseTasksMarkdown(string(content))
	if len(tasks) == 0 {
		return Result{Output: "no tasks found in " + tasksFile + "; nothing to do", IsError: true}
	}

	var summary strings.Builder
	fmt.Fprintf(&summary, "Repository: %s\nTasks file: %s\nDry run: %v\n\n", repo, tasksFile, p.DryRun)

	created := 0
	skipped := 0
	for _, task := range tasks {
		if !p.IncludeDone && task.State == "x" {
			skipped++
			continue
		}
		title := fmt.Sprintf("[%s] %s", task.ID, task.Body)
		if len(title) > 200 {
			title = title[:200]
		}
		body := fmt.Sprintf("From `%s`.\n\nTask `%s` — state: `%s`\n\n%s", tasksFile, task.ID, task.State, task.Body)

		if p.DryRun {
			fmt.Fprintf(&summary, "[dry-run] would create: %s\n", title)
			created++
			continue
		}

		args := []string{"issue", "create", "--repo", repo, "--title", title, "--body", body}
		for _, lbl := range p.Labels {
			args = append(args, "--label", lbl)
		}
		for _, a := range p.Assignees {
			args = append(args, "--assignee", a)
		}
		if strings.TrimSpace(p.Milestone) != "" {
			args = append(args, "--milestone", p.Milestone)
		}
		cmd := exec.CommandContext(ctx, "gh", args...)
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			fmt.Fprintf(&summary, "[error] %s: %v\n%s\n", task.ID, runErr, strings.TrimSpace(string(out)))
			continue
		}
		fmt.Fprintf(&summary, "[ok] %s -> %s\n", task.ID, strings.TrimSpace(string(out)))
		created++
	}

	fmt.Fprintf(&summary, "\nCreated: %d  Skipped (already done): %d  Total parsed: %d", created, skipped, len(tasks))
	title := "Spec Kit tasks → GitHub issues"
	if p.DryRun {
		title = "Spec Kit tasks → GitHub issues (dry run)"
	}
	return Result{Title: title, Output: summary.String()}
}

func (t *specKitChecklistTool) Name() string { return "speckit_checklist" }
func (t *specKitChecklistTool) Description() string {
	return "Spec Kit-compatible /speckit.checklist: generate a checklist for validating requirements quality."
}
func (t *specKitChecklistTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"spec_dir": map[string]any{"type": "string"}, "purpose": map[string]any{"type": "string"}, "items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
}
func (t *specKitChecklistTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir string   `json:"spec_dir"`
		Purpose string   `json:"purpose"`
		Items   []string `json:"items"`
	}
	_ = json.Unmarshal(input, &p)
	specDir := resolveSpecDir(call, p.SpecDir)
	if specDir == "" {
		return missingSpecDirResult("speckit_specify")
	}
	purpose := strings.TrimSpace(p.Purpose)
	if purpose == "" {
		purpose = "Requirements Quality"
	}
	items := p.Items
	if len(items) == 0 {
		items = []string{"Requirements are testable", "Acceptance scenarios are unambiguous", "Edge cases are documented", "No implementation details leak into the spec"}
	}
	dir := filepath.Join(specDir, "checklists")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{Output: "error creating checklists dir: " + err.Error(), IsError: true}
	}
	file := filepath.Join(dir, specdriven.Slugify(purpose)+".md")
	var sb strings.Builder
	sb.WriteString("# Checklist: " + purpose + "\n\n")
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			sb.WriteString("- [ ] " + strings.TrimSpace(item) + "\n")
		}
	}
	if err := os.WriteFile(file, []byte(sb.String()), 0o644); err != nil {
		return Result{Output: "error writing checklist: " + err.Error(), IsError: true}
	}
	return Result{Title: "Checklist written", Output: fmt.Sprintf("Created %s", file)}
}

func (t *specKitLifecycleStatusTool) Name() string { return "speckit_lifecycle_status" }
func (t *specKitLifecycleStatusTool) Description() string {
	return "Show the lifecycle state, artifact summary, and recommended next step for a Spec Kit folder."
}
func (t *specKitLifecycleStatusTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"spec_dir": map[string]any{"type": "string"}}}
}
func (t *specKitLifecycleStatusTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir string `json:"spec_dir"`
	}
	_ = json.Unmarshal(input, &p)
	specDir := resolveSpecDir(call, p.SpecDir)
	if specDir == "" {
		return missingSpecDirResult("speckit_specify")
	}
	record, err := specdriven.EnsureLifecycle(specDir, "", false)
	if err != nil {
		return Result{Output: "error reading lifecycle metadata: " + err.Error(), IsError: true}
	}
	return Result{Title: "Spec lifecycle status", Output: specdriven.RenderLifecycleStatus(specDir, specdriven.ArtifactPresence(specDir), record)}
}

func (t *specKitCompleteTool) Name() string { return "speckit_complete" }
func (t *specKitCompleteTool) Description() string {
	return "Mark a Spec Kit folder completed while preserving spec.md, plan.md, and tasks.md; optionally record notes and follow-ups."
}
func (t *specKitCompleteTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"spec_dir": map[string]any{"type": "string"}, "notes": map[string]any{"type": "string"}, "follow_ups": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}}
}
func (t *specKitCompleteTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir   string   `json:"spec_dir"`
		Notes     string   `json:"notes"`
		FollowUps []string `json:"follow_ups"`
	}
	_ = json.Unmarshal(input, &p)
	specDir := resolveSpecDir(call, p.SpecDir)
	if specDir == "" {
		return missingSpecDirResult("speckit_specify")
	}
	record, err := specdriven.EnsureLifecycle(specDir, "", true)
	if err != nil {
		return Result{Output: "error reading lifecycle metadata: " + err.Error(), IsError: true}
	}
	now := specdriven.NowISO()
	record.State = specdriven.LifecycleCompleted
	record.CompletedAt = now
	if strings.TrimSpace(p.Notes) != "" {
		record.Notes = strings.TrimSpace(p.Notes)
	}
	if len(p.FollowUps) > 0 {
		record.FollowUps = p.FollowUps
	}
	if err := specdriven.WriteLifecycle(specDir, record); err != nil {
		return Result{Output: "error writing lifecycle metadata: " + err.Error(), IsError: true}
	}
	record, _ = specdriven.ReadLifecycle(specDir)
	return Result{Title: "Spec completed", Output: specdriven.RenderLifecycleStatus(specDir, specdriven.ArtifactPresence(specDir), record)}
}

func (t *specKitArchiveTool) Name() string { return "speckit_archive" }
func (t *specKitArchiveTool) Description() string {
	return "Archive a completed Spec Kit folder under specs/archive/ without destructive overwrites."
}
func (t *specKitArchiveTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"spec_dir": map[string]any{"type": "string"}, "force": map[string]any{"type": "boolean"}}}
}
func (t *specKitArchiveTool) Execute(_ context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		SpecDir string `json:"spec_dir"`
		Force   bool   `json:"force"`
	}
	_ = json.Unmarshal(input, &p)
	specDir := resolveSpecDir(call, p.SpecDir)
	if specDir == "" {
		return missingSpecDirResult("speckit_specify")
	}
	record, err := specdriven.EnsureLifecycle(specDir, "", true)
	if err != nil {
		return Result{Output: "error reading lifecycle metadata: " + err.Error(), IsError: true}
	}
	warning := ""
	if record.State != specdriven.LifecycleCompleted && !p.Force {
		return Result{Output: fmt.Sprintf("error: spec lifecycle is %s; mark it completed first or set force=true", record.State), IsError: true}
	}
	if record.State != specdriven.LifecycleCompleted && p.Force {
		warning = fmt.Sprintf("Warning: archiving spec from %s state due to force=true.\n\n", record.State)
	}
	specsRoot := filepath.Dir(specDir)
	dest, err := specdriven.UniqueArchiveDestination(specsRoot, filepath.Base(specDir))
	if err != nil {
		return Result{Output: "error choosing archive destination: " + err.Error(), IsError: true}
	}
	record.State = specdriven.LifecycleArchived
	record.ArchivedAt = specdriven.NowISO()
	if err := specdriven.WriteLifecycle(specDir, record); err != nil {
		return Result{Output: "error writing lifecycle metadata: " + err.Error(), IsError: true}
	}
	if err := os.Rename(specDir, dest); err != nil {
		return Result{Output: "error archiving spec: " + err.Error(), IsError: true}
	}
	if call.SpecState != nil {
		call.SpecState.SetSpecDir(dest)
	}
	archivedRecord, err := specdriven.ReadLifecycle(dest)
	if err != nil {
		return Result{Output: "error reading archived lifecycle metadata: " + err.Error(), IsError: true}
	}
	return Result{Title: "Spec archived", Output: warning + fmt.Sprintf("Archived to %s\n\n%s", dest, specdriven.RenderLifecycleStatus(dest, specdriven.ArtifactPresence(dest), archivedRecord))}
}

func resolveSpecDir(call Context, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if call.SpecState != nil {
		return call.SpecState.GetSpecDir()
	}
	return ""
}
func missingSpecDirResult(first string) Result {
	return Result{Output: "error: spec_dir is required (or call " + first + " first)", IsError: true}
}
func artifactPresence(specDir string) map[specdriven.ArtifactKind]bool {
	present := map[specdriven.ArtifactKind]bool{}
	for _, kind := range []specdriven.ArtifactKind{specdriven.ArtifactSpec, specdriven.ArtifactPlan, specdriven.ArtifactTasks} {
		if _, err := os.Stat(filepath.Join(specDir, string(kind)+".md")); err == nil {
			present[kind] = true
		}
	}
	return present
}
func deriveTitle(feature string) string {
	fields := strings.Fields(feature)
	if len(fields) > 6 {
		fields = fields[:6]
	}
	return strings.Trim(strings.Join(fields, " "), " .,:;!?\t\n\r")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// nextSpecNumber moved to specdriven.NextSpecNumber.

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

// ─── OpenSpec-compatible tools ────────────────────────────────────────────────

type openSpecProposeTool struct{}
type openSpecExploreTool struct{}
type openSpecNewTool struct{}
type openSpecContinueTool struct{}
type openSpecFFTool struct{}
type openSpecApplyTool struct{}
type openSpecVerifyTool struct{}
type openSpecSyncTool struct{}
type openSpecArchiveTool struct{}
type openSpecBulkArchiveTool struct{}
type openSpecOnboardTool struct{}

func OpenSpecPropose() Tool     { return &openSpecProposeTool{} }
func OpenSpecExplore() Tool     { return &openSpecExploreTool{} }
func OpenSpecNew() Tool         { return &openSpecNewTool{} }
func OpenSpecContinue() Tool    { return &openSpecContinueTool{} }
func OpenSpecFF() Tool          { return &openSpecFFTool{} }
func OpenSpecApply() Tool       { return &openSpecApplyTool{} }
func OpenSpecVerify() Tool      { return &openSpecVerifyTool{} }
func OpenSpecSync() Tool        { return &openSpecSyncTool{} }
func OpenSpecArchive() Tool     { return &openSpecArchiveTool{} }
func OpenSpecBulkArchive() Tool { return &openSpecBulkArchiveTool{} }
func OpenSpecOnboard() Tool     { return &openSpecOnboardTool{} }

func (t *openSpecProposeTool) Name() string { return "openspec_propose" }
func (t *openSpecProposeTool) Description() string {
	return "Propose a new change with all artifacts generated in one step. Use when the user wants to quickly describe what they want to build and get a complete proposal with design, specs, and tasks ready for implementation."
}
func (t *openSpecProposeTool) InputSchema() map[string]any { return openSpecChangeSchema(true) }
func (t *openSpecProposeTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p openSpecChangeInput
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	change, err := ensureOpenSpecChange(p, true)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	created, err := createOpenSpecArtifacts(change, []string{"proposal", "specs", "design", "tasks"}, p, false)
	if err != nil {
		return Result{Output: "error creating artifacts: " + err.Error(), IsError: true}
	}
	return Result{Title: "OpenSpec proposal created", Output: openSpecFillInstructions(created, change, strings.TrimSpace(p.Description))}
}

func (t *openSpecExploreTool) Name() string { return "openspec_explore" }
func (t *openSpecExploreTool) Description() string {
	return "Enter explore mode — a thinking partner for exploring ideas, investigating problems, and clarifying requirements. Use when the user wants to think through something before or during a change."
}
func (t *openSpecExploreTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"topic": map[string]any{"type": "string"}, "notes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "openspec_dir": map[string]any{"type": "string"}}}
}
func (t *openSpecExploreTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p struct {
		Topic       string   `json:"topic"`
		Notes       []string `json:"notes"`
		OpenSpecDir string   `json:"openspec_dir"`
	}
	_ = json.Unmarshal(input, &p)
	root := strings.TrimSpace(p.OpenSpecDir)
	if root == "" {
		root = "openspec"
	}
	dir := filepath.Join(root, "explorations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{Output: "error creating explorations dir: " + err.Error(), IsError: true}
	}
	topic := strings.TrimSpace(p.Topic)
	if topic == "" {
		topic = "exploration"
	}
	file := filepath.Join(dir, specdriven.Slugify(topic)+".md")
	var sb strings.Builder
	sb.WriteString("# Exploration: " + topic + "\n\n")
	sb.WriteString("**Created**: " + specdriven.NowISO() + "\n\n")
	sb.WriteString("## Notes\n\n")
	if len(p.Notes) == 0 {
		sb.WriteString("- TODO: capture findings, options, and open questions.\n")
	} else {
		for _, note := range p.Notes {
			if strings.TrimSpace(note) != "" {
				sb.WriteString("- " + strings.TrimSpace(note) + "\n")
			}
		}
	}
	if err := os.WriteFile(file, []byte(sb.String()), 0o644); err != nil {
		return Result{Output: "error writing exploration: " + err.Error(), IsError: true}
	}
	return Result{Title: "OpenSpec exploration", Output: fmt.Sprintf("Created %s\n\nWhen ready, run openspec_propose or /opsx:propose.", file)}
}

func (t *openSpecNewTool) Name() string { return "openspec_new" }
func (t *openSpecNewTool) Description() string {
	return "Start a new OpenSpec change using the artifact workflow. Use when the user wants to create a new feature, fix, or modification with a structured step-by-step approach."
}
func (t *openSpecNewTool) InputSchema() map[string]any { return openSpecChangeSchema(false) }
func (t *openSpecNewTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p openSpecChangeInput
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	change, err := ensureOpenSpecChange(p, false)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	return Result{Title: "OpenSpec change created", Output: fmt.Sprintf("Created %s\nSchema: %s\n\nReady to create: proposal\nUse openspec_continue or openspec_ff.", change.dir, change.schema)}
}

func (t *openSpecContinueTool) Name() string { return "openspec_continue" }
func (t *openSpecContinueTool) Description() string {
	return "Continue working on an OpenSpec change by creating the next artifact. Use when the user wants to progress their change, create the next artifact, or continue their workflow."
}
func (t *openSpecContinueTool) InputSchema() map[string]any { return openSpecChangeSchema(false) }
func (t *openSpecContinueTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p openSpecChangeInput
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	change, err := resolveOpenSpecChange(p)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	for _, artifact := range specdriven.OpenSpecArtifacts() {
		path := filepath.Join(change.dir, filepath.FromSlash(artifact.Filename))
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if !openSpecDepsPresent(change.dir, artifact) {
			continue
		}
		created, err := createOpenSpecArtifacts(change, []string{artifact.ID}, p, false)
		if err != nil {
			return Result{Output: "error creating artifact: " + err.Error(), IsError: true}
		}
		return Result{Title: "OpenSpec artifact created", Output: openSpecFillSingleArtifact(strings.Join(created, ", "), artifact.ID, change, strings.TrimSpace(p.Description))}
	}
	return Result{Title: "OpenSpec change complete", Output: fmt.Sprintf("All planning artifacts are present for %s. Run openspec_apply or /opsx:apply.", change.name)}
}

func (t *openSpecFFTool) Name() string { return "openspec_ff" }
func (t *openSpecFFTool) Description() string {
	return "Fast-forward through OpenSpec artifact creation. Use when the user wants to quickly create all artifacts needed for implementation without stepping through each one individually."
}
func (t *openSpecFFTool) InputSchema() map[string]any { return openSpecChangeSchema(false) }
func (t *openSpecFFTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p openSpecChangeInput
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	change, err := ensureOpenSpecChange(p, false)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	created, err := createOpenSpecArtifacts(change, []string{"proposal", "specs", "design", "tasks"}, p, false)
	if err != nil {
		return Result{Output: "error creating artifacts: " + err.Error(), IsError: true}
	}
	return Result{Title: "OpenSpec fast-forward complete", Output: openSpecFillInstructions(created, change, strings.TrimSpace(p.Description))}
}

func (t *openSpecApplyTool) Name() string { return "openspec_apply" }
func (t *openSpecApplyTool) Description() string {
	return "Implement tasks from an OpenSpec change. Use when the user wants to start implementing, continue implementation, or work through tasks."
}
func (t *openSpecApplyTool) InputSchema() map[string]any { return openSpecChangeSchema(false) }
func (t *openSpecApplyTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p openSpecChangeInput
	_ = json.Unmarshal(input, &p)
	change, err := resolveOpenSpecChange(p)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	tasksFile := filepath.Join(change.dir, "tasks.md")
	content, err := os.ReadFile(tasksFile)
	if err != nil {
		return Result{Output: "error reading tasks.md: " + err.Error() + "; run openspec_propose, openspec_ff, or openspec_continue first", IsError: true}
	}
	text := string(content)
	pending := strings.Count(text, "- [ ]")
	done := strings.Count(text, "- [x]")
	return Result{Title: "OpenSpec apply readiness", Output: fmt.Sprintf("%s contains %d pending tasks and %d completed tasks. Implement pending tasks in order, update checkboxes, then run openspec_verify.", tasksFile, pending, done)}
}

func (t *openSpecVerifyTool) Name() string { return "openspec_verify" }
func (t *openSpecVerifyTool) Description() string {
	return "Verify implementation matches change artifacts. Use when the user wants to validate that implementation is complete, correct, and coherent before archiving."
}
func (t *openSpecVerifyTool) InputSchema() map[string]any { return openSpecChangeSchema(false) }
func (t *openSpecVerifyTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p openSpecChangeInput
	_ = json.Unmarshal(input, &p)
	change, err := resolveOpenSpecChange(p)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	missing := missingOpenSpecArtifacts(change.dir)
	tasksDone, tasksPending := openSpecTaskCounts(filepath.Join(change.dir, "tasks.md"))
	var sb strings.Builder
	sb.WriteString("# Verification: " + change.name + "\n\n")
	sb.WriteString("**Created**: " + specdriven.NowISO() + "\n\n")
	sb.WriteString("## Completeness\n\n")
	if len(missing) == 0 {
		sb.WriteString("- All required planning artifacts are present.\n")
	} else {
		sb.WriteString("- Missing artifacts: " + strings.Join(missing, ", ") + "\n")
	}
	sb.WriteString(fmt.Sprintf("- Tasks complete: %d\n- Tasks pending: %d\n", tasksDone, tasksPending))
	sb.WriteString("\n## Recommendations\n\n- Inspect implementation evidence before archiving.\n- Resolve any pending tasks or document why they are deferred.\n")
	file := filepath.Join(change.dir, "verification.md")
	if err := os.WriteFile(file, []byte(sb.String()), 0o644); err != nil {
		return Result{Output: "error writing verification.md: " + err.Error(), IsError: true}
	}
	return Result{Title: "OpenSpec verification", Output: fmt.Sprintf("Created %s\nMissing artifacts: %d\nPending tasks: %d", file, len(missing), tasksPending)}
}

func (t *openSpecSyncTool) Name() string { return "openspec_sync" }
func (t *openSpecSyncTool) Description() string {
	return "Sync delta specs from a change to main specs. Use when the user wants to update main specs with changes from a delta spec, without archiving the change."
}
func (t *openSpecSyncTool) InputSchema() map[string]any { return openSpecChangeSchema(false) }
func (t *openSpecSyncTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p openSpecChangeInput
	_ = json.Unmarshal(input, &p)
	change, err := resolveOpenSpecChange(p)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	srcRoot := filepath.Join(change.dir, "specs")
	if _, err := os.Stat(srcRoot); err != nil {
		return Result{Output: "error reading delta specs: " + err.Error(), IsError: true}
	}
	dstRoot := filepath.Join(change.root, "specs")
	copied := []string{}
	err = filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstRoot, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, content, 0o644); err != nil {
			return err
		}
		copied = append(copied, dst)
		return nil
	})
	if err != nil {
		return Result{Output: "error syncing specs: " + err.Error(), IsError: true}
	}
	return Result{Title: "OpenSpec sync", Output: fmt.Sprintf("Synced %d spec file(s):\n- %s", len(copied), strings.Join(copied, "\n- "))}
}

func (t *openSpecArchiveTool) Name() string { return "openspec_archive" }
func (t *openSpecArchiveTool) Description() string {
	return "Archive a completed change. Use when the user wants to finalize and archive a change after implementation is complete."
}
func (t *openSpecArchiveTool) InputSchema() map[string]any { return openSpecChangeSchema(false) }
func (t *openSpecArchiveTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p openSpecChangeInput
	_ = json.Unmarshal(input, &p)
	change, err := resolveOpenSpecChange(p)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	dst, err := uniqueOpenSpecArchiveDir(change.root, change.name)
	if err != nil {
		return Result{Output: "error preparing archive dir: " + err.Error(), IsError: true}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Result{Output: "error creating archive root: " + err.Error(), IsError: true}
	}
	if err := os.Rename(change.dir, dst); err != nil {
		return Result{Output: "error archiving change: " + err.Error(), IsError: true}
	}
	return Result{Title: "OpenSpec archived", Output: fmt.Sprintf("Archived %s to %s", change.name, dst)}
}

func (t *openSpecBulkArchiveTool) Name() string { return "openspec_bulk_archive" }
func (t *openSpecBulkArchiveTool) Description() string {
	return "Archive multiple completed changes at once. Use when archiving several parallel changes."
}
func (t *openSpecBulkArchiveTool) InputSchema() map[string]any {
	schema := openSpecChangeSchema(false)
	schema["properties"].(map[string]any)["changes"] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	return schema
}
func (t *openSpecBulkArchiveTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p struct {
		OpenSpecDir string   `json:"openspec_dir"`
		Changes     []string `json:"changes"`
	}
	_ = json.Unmarshal(input, &p)
	root := strings.TrimSpace(p.OpenSpecDir)
	if root == "" {
		root = "openspec"
	}
	changes := p.Changes
	if len(changes) == 0 {
		entries, err := os.ReadDir(filepath.Join(root, "changes"))
		if err != nil {
			return Result{Output: "error listing changes: " + err.Error(), IsError: true}
		}
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() != "archive" {
				changes = append(changes, entry.Name())
			}
		}
		sort.Strings(changes)
	}
	archived := []string{}
	for _, name := range changes {
		change := openSpecChange{name: specdriven.Slugify(name), root: root, dir: filepath.Join(root, "changes", specdriven.Slugify(name))}
		if _, err := os.Stat(change.dir); err != nil {
			continue
		}
		dst, err := uniqueOpenSpecArchiveDir(root, change.name)
		if err != nil {
			return Result{Output: "error preparing archive dir: " + err.Error(), IsError: true}
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return Result{Output: "error creating archive root: " + err.Error(), IsError: true}
		}
		if err := os.Rename(change.dir, dst); err != nil {
			return Result{Output: "error archiving " + change.name + ": " + err.Error(), IsError: true}
		}
		archived = append(archived, dst)
	}
	return Result{Title: "OpenSpec bulk archive", Output: fmt.Sprintf("Archived %d change(s):\n- %s", len(archived), strings.Join(archived, "\n- "))}
}

func (t *openSpecOnboardTool) Name() string { return "openspec_onboard" }
func (t *openSpecOnboardTool) Description() string {
	return "Guided onboarding for OpenSpec — walk through a complete workflow cycle with narration and real codebase work."
}
func (t *openSpecOnboardTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"openspec_dir": map[string]any{"type": "string"}}}
}
func (t *openSpecOnboardTool) Execute(_ context.Context, _ Context, input json.RawMessage) Result {
	var p struct {
		OpenSpecDir string `json:"openspec_dir"`
	}
	_ = json.Unmarshal(input, &p)
	root := strings.TrimSpace(p.OpenSpecDir)
	if root == "" {
		root = "openspec"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Result{Output: "error creating openspec dir: " + err.Error(), IsError: true}
	}
	file := filepath.Join(root, "onboarding.md")
	content := "# OpenSpec Onboarding\n\n- [ ] Explore the codebase\n- [ ] Run openspec_new for a small change\n- [ ] Create proposal, specs, design, and tasks\n- [ ] Apply tasks\n- [ ] Verify implementation\n- [ ] Archive the change\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return Result{Output: "error writing onboarding: " + err.Error(), IsError: true}
	}
	return Result{Title: "OpenSpec onboarding", Output: fmt.Sprintf("Created %s", file)}
}

type openSpecChangeInput struct {
	ChangeName  string `json:"change_name"`
	Description string `json:"description"`
	Schema      string `json:"schema"`
	OpenSpecDir string `json:"openspec_dir"`
	Area        string `json:"area"`
}

type openSpecChange struct {
	name   string
	root   string
	dir    string
	schema string
}

func openSpecChangeSchema(requireDescription bool) map[string]any {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"change_name":  map[string]any{"type": "string", "description": "Kebab-case change name. Derived from description when omitted."},
			"description":  map[string]any{"type": "string", "description": "Plain-language change description."},
			"schema":       map[string]any{"type": "string", "description": "OpenSpec workflow schema. Defaults to spec-driven."},
			"openspec_dir": map[string]any{"type": "string", "description": "OpenSpec root directory. Defaults to ./openspec."},
			"area":         map[string]any{"type": "string", "description": "Delta spec area. Defaults to general."},
		},
	}
	if requireDescription {
		schema["required"] = []string{"description"}
	}
	return schema
}

func ensureOpenSpecChange(p openSpecChangeInput, allowDescriptionName bool) (openSpecChange, error) {
	change, err := buildOpenSpecChange(p, allowDescriptionName)
	if err != nil {
		return change, err
	}
	if err := os.MkdirAll(change.dir, 0o755); err != nil {
		return change, err
	}
	meta := filepath.Join(change.dir, ".openspec.yaml")
	if _, err := os.Stat(meta); os.IsNotExist(err) {
		content := fmt.Sprintf("schema: %s\ncreated: %s\nchange: %s\n", change.schema, specdriven.NowISO(), change.name)
		if err := os.WriteFile(meta, []byte(content), 0o644); err != nil {
			return change, err
		}
	}
	return change, nil
}

func resolveOpenSpecChange(p openSpecChangeInput) (openSpecChange, error) {
	change, err := buildOpenSpecChange(p, true)
	if err != nil {
		return change, err
	}
	if _, err := os.Stat(change.dir); err != nil {
		return change, fmt.Errorf("change %q not found at %s", change.name, change.dir)
	}
	return change, nil
}

func buildOpenSpecChange(p openSpecChangeInput, allowDescriptionName bool) (openSpecChange, error) {
	root := strings.TrimSpace(p.OpenSpecDir)
	if root == "" {
		root = "openspec"
	}
	name := specdriven.Slugify(p.ChangeName)
	if name == "" && allowDescriptionName {
		name = specdriven.Slugify(deriveTitle(p.Description))
	}
	if name == "" {
		return openSpecChange{}, fmt.Errorf("change_name is required")
	}
	schema := strings.TrimSpace(p.Schema)
	if schema == "" {
		schema = "spec-driven"
	}
	return openSpecChange{name: name, root: root, dir: filepath.Join(root, "changes", name), schema: schema}, nil
}

func createOpenSpecArtifacts(change openSpecChange, ids []string, p openSpecChangeInput, overwrite bool) ([]string, error) {
	created := []string{}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	for _, artifact := range specdriven.OpenSpecArtifacts() {
		if !wanted[artifact.ID] {
			continue
		}
		path := filepath.Join(change.dir, filepath.FromSlash(artifact.Filename))
		if !overwrite {
			if _, err := os.Stat(path); err == nil {
				created = append(created, path+" (exists)")
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return created, err
		}
		content := renderOpenSpecArtifact(change, artifact.ID, p)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return created, err
		}
		created = append(created, path)
	}
	return created, nil
}

func openSpecFillInstructions(created []string, change openSpecChange, description string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Scaffolded change directory: %s\n\nCreated artifacts:\n", change.dir))
	for _, f := range created {
		sb.WriteString(fmt.Sprintf("- %s\n", f))
	}
	sb.WriteString("\nIMPORTANT: These artifacts contain placeholder content. You MUST now fill them in with real, substantive content.\n\n")
	sb.WriteString("Follow these steps:\n\n")
	sb.WriteString("1. Read the codebase to understand the change scope — look at relevant source files, tests, and existing patterns.\n")
	sb.WriteString("2. Read and update proposal.md: Write a real motivation (Why), list specific user-visible and technical changes (What Changes), and identify concrete impact (affected files, specs, components).\n")
	sb.WriteString("3. Read and update specs/general/spec.md: Write real ADDED/MODIFIED/REMOVED requirements using SHALL language, with specific GIVEN/WHEN/THEN acceptance scenarios based on the actual change behavior.\n")
	sb.WriteString("4. Read and update design.md: Document real context, design decisions with trade-offs considered, and risks with mitigations.\n")
	sb.WriteString("5. Read and update tasks.md: Replace the generic tasks with specific, actionable implementation steps derived from the proposal, specs, and design. Each task should reference specific files or functions.\n\n")
	sb.WriteString("Write the updated content to each file. Do NOT leave TODO placeholders.\n\n")
	if description != "" {
		sb.WriteString(fmt.Sprintf("User's intent: %s\n", description))
	}
	sb.WriteString(fmt.Sprintf("Change name: %s\n", change.name))
	return sb.String()
}

func openSpecFillSingleArtifact(path string, artifactID string, change openSpecChange, description string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Created artifact: %s\n\n", path))
	sb.WriteString("IMPORTANT: This artifact contains placeholder content. You MUST now fill it in with real, substantive content.\n\n")
	switch artifactID {
	case "proposal":
		sb.WriteString("Read the codebase to understand the change scope, then update proposal.md with: real motivation (Why), specific user-visible and technical changes (What Changes), and concrete impact analysis.\n")
	case "specs":
		sb.WriteString("Read the codebase and the proposal, then update the spec with: real ADDED/MODIFIED/REMOVED requirements using SHALL language, and specific GIVEN/WHEN/THEN acceptance scenarios.\n")
	case "design":
		sb.WriteString("Read the codebase and the proposal, then update design.md with: real context, design decisions with trade-offs, and risks with mitigations.\n")
	case "tasks":
		sb.WriteString("Read the proposal, specs, and design, then update tasks.md with: specific, actionable implementation steps that reference concrete files and functions.\n")
	}
	sb.WriteString("\nDo NOT leave TODO placeholders. Write the updated content to the file.\n\n")
	if description != "" {
		sb.WriteString(fmt.Sprintf("User's intent: %s\n", description))
	}
	sb.WriteString(fmt.Sprintf("Change name: %s\n", change.name))
	sb.WriteString("\nAfter filling in this artifact, run openspec_continue or /openspec:continue for the next one.\n")
	return sb.String()
}

func renderOpenSpecArtifact(change openSpecChange, id string, p openSpecChangeInput) string {
	description := strings.TrimSpace(p.Description)
	if description == "" {
		description = "TODO: describe the requested change."
	}
	area := strings.TrimSpace(p.Area)
	if area == "" {
		area = "general"
	}
	switch id {
	case "proposal":
		return fmt.Sprintf("# Proposal: %s\n\n## Why\n\n%s\n\n## What Changes\n\n- TODO: summarize user-visible and technical changes.\n\n## Impact\n\n- Specs: %s\n- Code: TODO\n", change.name, description, area)
	case "specs":
		return fmt.Sprintf("# Spec Delta: %s\n\n## ADDED Requirements\n\n### Requirement: %s\n\nThe system SHALL support this change.\n\n#### Scenario: Happy path\n\n- **GIVEN** the change is implemented\n- **WHEN** the user exercises the new behavior\n- **THEN** the expected outcome occurs\n\n## MODIFIED Requirements\n\nNone.\n\n## REMOVED Requirements\n\nNone.\n", area, description)
	case "design":
		return fmt.Sprintf("# Design: %s\n\n## Context\n\n%s\n\n## Decisions\n\n- TODO: document design decisions and trade-offs.\n\n## Risks\n\n- TODO: document risks and mitigations.\n", change.name, description)
	case "tasks":
		return fmt.Sprintf("# Tasks: %s\n\n- [ ] 1.1 Review proposal, specs, and design\n- [ ] 1.2 Implement the change\n- [ ] 1.3 Add or update tests\n- [ ] 1.4 Run validation\n- [ ] 1.5 Update documentation if needed\n", change.name)
	default:
		return ""
	}
}

func openSpecDepsPresent(changeDir string, artifact specdriven.OpenSpecArtifact) bool {
	byID := map[string]specdriven.OpenSpecArtifact{}
	for _, a := range specdriven.OpenSpecArtifacts() {
		byID[a.ID] = a
	}
	for _, req := range artifact.Requires {
		dep, ok := byID[req]
		if !ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(changeDir, filepath.FromSlash(dep.Filename))); err != nil {
			return false
		}
	}
	return true
}

func missingOpenSpecArtifacts(changeDir string) []string {
	missing := []string{}
	for _, artifact := range specdriven.OpenSpecArtifacts() {
		if _, err := os.Stat(filepath.Join(changeDir, filepath.FromSlash(artifact.Filename))); err != nil {
			missing = append(missing, artifact.Filename)
		}
	}
	return missing
}

func openSpecTaskCounts(tasksFile string) (int, int) {
	content, err := os.ReadFile(tasksFile)
	if err != nil {
		return 0, 0
	}
	text := string(content)
	return strings.Count(text, "- [x]"), strings.Count(text, "- [ ]")
}

func uniqueOpenSpecArchiveDir(root, changeName string) (string, error) {
	base := filepath.Join(root, "changes", "archive", specdriven.NowISO()[:10]+"-"+changeName)
	candidate := base
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}
