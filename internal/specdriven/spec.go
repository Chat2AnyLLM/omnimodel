// Package specdriven implements the spec-driven development workflow for
// OmniCode agents. It consolidates ideas from spec-kit (structured spec/plan/tasks
// markdown templates, prioritised user stories, Given-When-Then scenarios) and
// OpenSpec (artifact dependency graph, change-delta tracking).
//
// Workflow: speckit_specify -> speckit_plan -> speckit_tasks
//
// Each command corresponds to one stage of the spec-driven pipeline and is
// exposed as a loadable "spec" skill in the tools registry.
package specdriven

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ─── Priority ────────────────────────────────────────────────────────────────

// Priority classifies how critical a user story is.
type Priority string

const (
	PriorityP1 Priority = "P1" // Must-have / MVP
	PriorityP2 Priority = "P2" // Should-have
	PriorityP3 Priority = "P3" // Nice-to-have
)

// ─── Scenario ────────────────────────────────────────────────────────────────

// Scenario captures a single Given-When-Then acceptance scenario.
type Scenario struct {
	Title string
	Given string
	When  string
	Then  string
}

// ─── UserStory ────────────────────────────────────────────────────────────────

// UserStory is a deliverable slice of functionality independently testable and
// deployable. It mirrors the P1/P2/P3 structure from spec-kit.
type UserStory struct {
	ID          string     // e.g. "US1"
	Title       string     // Short title
	Description string     // One-sentence "as a user I want..."
	Priority    Priority   // P1/P2/P3
	WhyPriority string     // Rationale for priority
	Scenarios   []Scenario // Acceptance scenarios (Given/When/Then)
}

// ─── Requirement ─────────────────────────────────────────────────────────────

// Requirement expresses a specific capability using SHALL/MUST language (OpenSpec
// style), tied to a user story.
type Requirement struct {
	ID                 string // e.g. "FR-001"
	UserStoryID        string // Back-reference to UserStory.ID
	Text               string // "The system SHALL …"
	NeedsClarification bool   // From spec-kit: unresolved unknowns
}

// ─── Entity ──────────────────────────────────────────────────────────────────

// Entity is a key data concept without implementation details.
type Entity struct {
	Name        string
	Description string
	Fields      []string // High-level field names
}

// ─── EdgeCase ────────────────────────────────────────────────────────────────

// EdgeCase documents a boundary condition or error scenario.
type EdgeCase struct {
	ID          string // e.g. "EC-001"
	Description string
	Expected    string // Expected system behaviour
}

// ─── Spec ────────────────────────────────────────────────────────────────────

// Spec is the root specification artifact. It is created by spec_init/spec_write
// and persisted as spec.md in the spec directory.
type Spec struct {
	// Number is zero-padded feature index, e.g. "001".
	Number string
	// Slug is a kebab-case short name, e.g. "user-auth".
	Slug string
	// Title is the human-readable feature name.
	Title string
	// Overview is a 1–3 sentence description of the feature and its purpose.
	Overview string
	// UserStories is the ordered list of prioritised stories.
	UserStories []UserStory
	// Requirements is the derived functional requirements list.
	Requirements []Requirement
	// Entities lists the key domain entities.
	Entities []Entity
	// EdgeCases lists boundary/error conditions.
	EdgeCases []EdgeCase
	// CreatedAt is the ISO-8601 creation timestamp.
	CreatedAt string
}

// DirName returns the canonical spec directory name, e.g. "001-user-auth".
func (s *Spec) DirName() string {
	return fmt.Sprintf("%s-%s", s.Number, s.Slug)
}

// ─── SpecStore ────────────────────────────────────────────────────────────────

// SpecStore is the session-scoped state holder for spec-driven operations.
// It is stored on tools.Context and shared across all spec tool calls.
type SpecStore struct {
	mu          sync.Mutex
	currentSpec *Spec
	currentPlan *Plan
	specDir     string // Resolved path to specs/ root
}

// NewSpecStore creates a new empty store.
func NewSpecStore() *SpecStore { return &SpecStore{} }

// SetSpec stores the active spec.
func (s *SpecStore) SetSpec(spec *Spec) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentSpec = spec
}

// GetSpec returns the active spec, or nil.
func (s *SpecStore) GetSpec() *Spec {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentSpec
}

// SetPlan stores the active plan.
func (s *SpecStore) SetPlan(plan *Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentPlan = plan
}

// GetPlan returns the active plan, or nil.
func (s *SpecStore) GetPlan() *Plan {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentPlan
}

// SetSpecDir records the resolved path to the specs/ root.
func (s *SpecStore) SetSpecDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.specDir = dir
}

// GetSpecDir returns the recorded specs/ root.
func (s *SpecStore) GetSpecDir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.specDir
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// NowISO returns the current UTC time as a compact ISO-8601 string.
func NowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z")
}

// Slugify converts a free-form title into a kebab-case slug (max 48 chars).
func Slugify(title string) string {
	var sb strings.Builder
	prev := '-'
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			prev = r
		} else if prev != '-' {
			sb.WriteByte('-')
			prev = '-'
		}
	}
	slug := strings.Trim(sb.String(), "-")
	if len(slug) > 48 {
		slug = slug[:48]
		slug = strings.TrimRight(slug, "-")
	}
	return slug
}
