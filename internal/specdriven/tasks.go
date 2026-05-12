package specdriven

import (
	"fmt"
	"strings"
)

// ─── Task ────────────────────────────────────────────────────────────────────

// TaskStatus mirrors the spec-kit checkbox convention.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"     // [ ]
	TaskInProgress TaskStatus = "in_progress" // [~]
	TaskDone       TaskStatus = "done"        // [x]
)

// SpecTask is an atomic unit of work extracted from a plan. Persisted as a
// line in tasks.md. Inspired by spec-kit task format:
//
//	[x] T001 [P] [US1] Description — src/path/file.go
type SpecTask struct {
	ID             string // e.g. "T001"
	UserStoryID    string // e.g. "US1"
	Description    string
	TargetPath     string // Source file to create/modify (optional)
	Parallelizable bool   // true → marked [P]
	Status         TaskStatus
	Phase          Phase // Which plan phase this belongs to
}

// TaskGroup groups tasks by user story for independent delivery.
type TaskGroup struct {
	UserStoryID string
	Title       string
	Tasks       []SpecTask
}

// ─── Markdown generation ──────────────────────────────────────────────────────

// RenderTasks renders the task list to markdown (tasks.md content).
func RenderTasks(specNumber, title string, groups []TaskGroup) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Tasks: %s %s\n\n", specNumber, title))
	sb.WriteString("Legend: `[ ]` pending · `[~]` in progress · `[x]` done · `[P]` parallelizable\n\n")

	for _, g := range groups {
		sb.WriteString(fmt.Sprintf("## %s – %s\n\n", g.UserStoryID, g.Title))
		for _, t := range g.Tasks {
			check := "[ ]"
			switch t.Status {
			case TaskInProgress:
				check = "[~]"
			case TaskDone:
				check = "[x]"
			}
			parallel := ""
			if t.Parallelizable {
				parallel = " [P]"
			}
			path := ""
			if t.TargetPath != "" {
				path = " — " + t.TargetPath
			}
			sb.WriteString(fmt.Sprintf("- %s **%s**%s [%s] %s%s\n",
				check, t.ID, parallel, t.UserStoryID, t.Description, path))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// ─── Scaffold helpers ────────────────────────────────────────────────────────

// ScaffoldTaskGroups builds an initial task breakdown from a Spec, providing a
// starting template that the agent can enrich. This mirrors spec-kit's phases:
//
//	Phase 2 (Setup) → foundational infrastructure tasks
//	Phase 3+ (Stories) → one group per user story
func ScaffoldTaskGroups(s *Spec) []TaskGroup {
	var groups []TaskGroup
	counter := 1

	// Phase 2: Setup group
	setupGroup := TaskGroup{
		UserStoryID: "SETUP",
		Title:       "Project Setup & Dependencies",
		Tasks: []SpecTask{
			{ID: nextTaskID(&counter), UserStoryID: "SETUP", Description: "Initialise project structure and dependencies", Phase: PhaseSetup},
			{ID: nextTaskID(&counter), UserStoryID: "SETUP", Description: "Add required libraries and verify build", Phase: PhaseSetup},
		},
	}
	groups = append(groups, setupGroup)

	// Phase 3+: One group per user story
	for _, us := range s.UserStories {
		var tasks []SpecTask
		// Test task first (TDD style)
		tasks = append(tasks, SpecTask{
			ID:          nextTaskID(&counter),
			UserStoryID: us.ID,
			Description: fmt.Sprintf("Write acceptance tests for %s", us.Title),
			Phase:       PhaseImplement,
		})
		// Implementation task per scenario
		for _, sc := range us.Scenarios {
			tasks = append(tasks, SpecTask{
				ID:             nextTaskID(&counter),
				UserStoryID:    us.ID,
				Description:    fmt.Sprintf("Implement: %s", sc.Title),
				Parallelizable: true,
				Phase:          PhaseImplement,
			})
		}
		// Fallback if no scenarios
		if len(us.Scenarios) == 0 {
			tasks = append(tasks, SpecTask{
				ID:          nextTaskID(&counter),
				UserStoryID: us.ID,
				Description: fmt.Sprintf("Implement %s", us.Title),
				Phase:       PhaseImplement,
			})
		}
		groups = append(groups, TaskGroup{
			UserStoryID: us.ID,
			Title:       us.Title,
			Tasks:       tasks,
		})
	}
	return groups
}

func nextTaskID(n *int) string {
	id := fmt.Sprintf("T%03d", *n)
	(*n)++
	return id
}

// ─── Artifact graph ──────────────────────────────────────────────────────────

// ArtifactKind names a spec-driven artifact. Inspired by OpenSpec's ArtifactGraph.
type ArtifactKind string

const (
	ArtifactSpec  ArtifactKind = "spec"
	ArtifactPlan  ArtifactKind = "plan"
	ArtifactTasks ArtifactKind = "tasks"
	ArtifactCode  ArtifactKind = "code"
)

// ArtifactStatus records whether an artifact file is present.
type ArtifactStatus struct {
	Kind     ArtifactKind
	Filename string // e.g. "spec.md"
	Requires []ArtifactKind
	Present  bool
}

// BuildOrder returns the artifacts in dependency order.
func BuildOrder() []ArtifactStatus {
	return []ArtifactStatus{
		{Kind: ArtifactSpec, Filename: "spec.md", Requires: nil},
		{Kind: ArtifactPlan, Filename: "plan.md", Requires: []ArtifactKind{ArtifactSpec}},
		{Kind: ArtifactTasks, Filename: "tasks.md", Requires: []ArtifactKind{ArtifactPlan}},
		{Kind: ArtifactCode, Filename: "(implementation)", Requires: []ArtifactKind{ArtifactTasks}},
	}
}

// RenderStatus builds a human-readable status board for a spec directory.
func RenderStatus(specDir string, present map[ArtifactKind]bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Spec: %s\n", specDir))
	sb.WriteString("Artifacts:\n")
	for _, a := range BuildOrder() {
		icon := "○"
		status := "missing"
		if present[a.Kind] {
			icon = "✓"
			status = "present"
		}
		sb.WriteString(fmt.Sprintf("  %s %-5s %-17s %s\n", icon, a.Kind, a.Filename, status))
	}
	return sb.String()
}
