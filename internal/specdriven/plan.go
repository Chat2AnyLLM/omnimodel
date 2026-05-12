package specdriven

import (
	"fmt"
	"strings"
)

// ─── Phase ───────────────────────────────────────────────────────────────────

// Phase classifies a plan phase (mirrors spec-kit's Phase 0/1/2/3 pattern).
type Phase string

const (
	PhaseResearch  Phase = "Phase 0: Research"       // Resolve NEEDS CLARIFICATION items
	PhaseDesign    Phase = "Phase 1: Design"         // Data model, contracts, quickstart
	PhaseSetup     Phase = "Phase 2: Setup"          // Project structure, dependencies
	PhaseImplement Phase = "Phase 3: Implementation" // User story implementation
)

// ─── TechnicalContext ─────────────────────────────────────────────────────────

// TechnicalContext documents the implementation environment.
type TechnicalContext struct {
	Language     string   // Primary language, e.g. "Go 1.22"
	Framework    string   // e.g. "Gin", "Echo"
	Database     string   // e.g. "SQLite", "Postgres"
	Deployment   string   // e.g. "Docker", "Kubernetes"
	PerformGoals []string // e.g. ["p99 < 100ms", "1k RPS"]
	Constraints  []string // e.g. ["no external auth service"]
	Dependencies []string // Key libraries to use
}

// ─── PlanPhase ────────────────────────────────────────────────────────────────

// PlanPhase groups deliverables for one phase.
type PlanPhase struct {
	Phase       Phase
	Deliverable []string // Files / documents produced
	Notes       string
}

// ─── APIContract ─────────────────────────────────────────────────────────────

// APIContract documents a REST endpoint or event contract.
type APIContract struct {
	Method      string // GET/POST/PUT/DELETE/event
	Path        string // e.g. "/api/v1/users"
	Description string
	RequestBody string // JSON schema or prose
	Response    string // JSON schema or prose
}

// ─── Plan ────────────────────────────────────────────────────────────────────

// Plan is generated from a Spec and drives task extraction. Persisted as plan.md.
type Plan struct {
	SpecNumber string
	SpecSlug   string
	Title      string
	TechCtx    TechnicalContext
	Phases     []PlanPhase
	DataModel  []Entity      // Derived from Spec.Entities + research
	Contracts  []APIContract // REST/event contracts
	CreatedAt  string
}

// ─── Markdown generation ──────────────────────────────────────────────────────

// RenderSpec renders a Spec to markdown (spec.md content).
func RenderSpec(s *Spec) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Spec: %s %s\n\n", s.Number, s.Title))
	sb.WriteString(fmt.Sprintf("**Created**: %s\n\n", s.CreatedAt))
	sb.WriteString("## Overview\n\n")
	sb.WriteString(s.Overview)
	sb.WriteString("\n\n")

	sb.WriteString("## User Stories\n\n")
	for _, us := range s.UserStories {
		sb.WriteString(fmt.Sprintf("### %s – %s (%s)\n\n", us.ID, us.Title, us.Priority))
		sb.WriteString(us.Description)
		sb.WriteString("\n\n")
		if us.WhyPriority != "" {
			sb.WriteString(fmt.Sprintf("**Why this priority**: %s\n\n", us.WhyPriority))
		}
		if len(us.Scenarios) > 0 {
			sb.WriteString("**Acceptance Scenarios**:\n\n")
			for _, sc := range us.Scenarios {
				sb.WriteString(fmt.Sprintf("#### %s\n\n", sc.Title))
				sb.WriteString(fmt.Sprintf("- **GIVEN** %s\n", sc.Given))
				sb.WriteString(fmt.Sprintf("- **WHEN** %s\n", sc.When))
				sb.WriteString(fmt.Sprintf("- **THEN** %s\n\n", sc.Then))
			}
		}
	}

	if len(s.Requirements) > 0 {
		sb.WriteString("## Functional Requirements\n\n")
		for _, r := range s.Requirements {
			clarification := ""
			if r.NeedsClarification {
				clarification = " <!-- NEEDS CLARIFICATION -->"
			}
			sb.WriteString(fmt.Sprintf("- **%s** [%s]: %s%s\n", r.ID, r.UserStoryID, r.Text, clarification))
		}
		sb.WriteString("\n")
	}

	if len(s.Entities) > 0 {
		sb.WriteString("## Key Entities\n\n")
		for _, e := range s.Entities {
			sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", e.Name, e.Description))
			if len(e.Fields) > 0 {
				sb.WriteString("Fields: " + strings.Join(e.Fields, ", ") + "\n\n")
			}
		}
	}

	if len(s.EdgeCases) > 0 {
		sb.WriteString("## Edge Cases\n\n")
		for _, ec := range s.EdgeCases {
			sb.WriteString(fmt.Sprintf("- **%s**: %s → %s\n", ec.ID, ec.Description, ec.Expected))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// RenderPlan renders a Plan to markdown (plan.md content).
func RenderPlan(p *Plan) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Plan: %s %s\n\n", p.SpecNumber, p.Title))
	sb.WriteString(fmt.Sprintf("**Created**: %s\n\n", p.CreatedAt))

	sb.WriteString("## Technical Context\n\n")
	tc := p.TechCtx
	if tc.Language != "" {
		sb.WriteString(fmt.Sprintf("- **Language**: %s\n", tc.Language))
	}
	if tc.Framework != "" {
		sb.WriteString(fmt.Sprintf("- **Framework**: %s\n", tc.Framework))
	}
	if tc.Database != "" {
		sb.WriteString(fmt.Sprintf("- **Database**: %s\n", tc.Database))
	}
	if tc.Deployment != "" {
		sb.WriteString(fmt.Sprintf("- **Deployment**: %s\n", tc.Deployment))
	}
	for _, g := range tc.PerformGoals {
		sb.WriteString(fmt.Sprintf("- **Perf**: %s\n", g))
	}
	for _, c := range tc.Constraints {
		sb.WriteString(fmt.Sprintf("- **Constraint**: %s\n", c))
	}
	sb.WriteString("\n")

	for _, ph := range p.Phases {
		sb.WriteString(fmt.Sprintf("## %s\n\n", ph.Phase))
		for _, d := range ph.Deliverable {
			sb.WriteString(fmt.Sprintf("- %s\n", d))
		}
		if ph.Notes != "" {
			sb.WriteString("\n")
			sb.WriteString(ph.Notes)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(p.DataModel) > 0 {
		sb.WriteString("## Data Model\n\n")
		for _, e := range p.DataModel {
			sb.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", e.Name, e.Description))
			if len(e.Fields) > 0 {
				sb.WriteString("Fields: " + strings.Join(e.Fields, ", ") + "\n\n")
			}
		}
	}

	if len(p.Contracts) > 0 {
		sb.WriteString("## API Contracts\n\n")
		for _, c := range p.Contracts {
			sb.WriteString(fmt.Sprintf("### %s %s\n\n%s\n\n", c.Method, c.Path, c.Description))
			if c.RequestBody != "" {
				sb.WriteString(fmt.Sprintf("**Request**: %s\n\n", c.RequestBody))
			}
			if c.Response != "" {
				sb.WriteString(fmt.Sprintf("**Response**: %s\n\n", c.Response))
			}
		}
	}

	return sb.String()
}
