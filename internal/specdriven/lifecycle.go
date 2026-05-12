package specdriven

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SpecLifecycleState defines the clean lifecycle for SpecKit artifacts.
type SpecLifecycleState string

const (
	LifecycleDraft      SpecLifecycleState = "draft"
	LifecycleInProgress SpecLifecycleState = "in_progress"
	LifecycleCompleted  SpecLifecycleState = "completed"
	LifecycleArchived   SpecLifecycleState = "archived"
)

const LifecycleFilename = ".speckit-state.json"

// SpecLifecycleRecord stores lightweight repo-local lifecycle metadata.
type SpecLifecycleRecord struct {
	State       SpecLifecycleState `json:"state"`
	CreatedAt   string             `json:"created_at,omitempty"`
	UpdatedAt   string             `json:"updated_at,omitempty"`
	CompletedAt string             `json:"completed_at,omitempty"`
	ArchivedAt  string             `json:"archived_at,omitempty"`
	Notes       string             `json:"notes,omitempty"`
	FollowUps   []string           `json:"follow_ups,omitempty"`
}

func lifecyclePath(specDir string) string {
	return filepath.Join(specDir, LifecycleFilename)
}

func DefaultLifecycleRecord(createdAt string) SpecLifecycleRecord {
	ts := strings.TrimSpace(createdAt)
	if ts == "" {
		ts = NowISO()
	}
	return SpecLifecycleRecord{
		State:     LifecycleDraft,
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}

func ReadLifecycle(specDir string) (SpecLifecycleRecord, error) {
	path := lifecyclePath(specDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultLifecycleRecord(""), nil
		}
		return SpecLifecycleRecord{}, err
	}
	var record SpecLifecycleRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return SpecLifecycleRecord{}, fmt.Errorf("parse lifecycle metadata: %w", err)
	}
	if record.State == "" {
		record.State = LifecycleDraft
	}
	if record.CreatedAt == "" {
		record.CreatedAt = NowISO()
	}
	if record.UpdatedAt == "" {
		record.UpdatedAt = record.CreatedAt
	}
	return record, nil
}

func WriteLifecycle(specDir string, record SpecLifecycleRecord) error {
	if strings.TrimSpace(record.State.String()) == "" {
		record.State = LifecycleDraft
	}
	if strings.TrimSpace(record.CreatedAt) == "" {
		record.CreatedAt = NowISO()
	}
	record.UpdatedAt = NowISO()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(lifecyclePath(specDir), data, 0o644)
}

func (s SpecLifecycleState) String() string { return string(s) }

func EnsureLifecycle(specDir, createdAt string) (SpecLifecycleRecord, error) {
	record, err := ReadLifecycle(specDir)
	if err != nil {
		return SpecLifecycleRecord{}, err
	}
	if _, err := os.Stat(lifecyclePath(specDir)); os.IsNotExist(err) {
		record = DefaultLifecycleRecord(createdAt)
		if err := WriteLifecycle(specDir, record); err != nil {
			return SpecLifecycleRecord{}, err
		}
		record, err = ReadLifecycle(specDir)
		if err != nil {
			return SpecLifecycleRecord{}, err
		}
	}
	return record, nil
}

func ArtifactPresence(specDir string) map[ArtifactKind]bool {
	present := map[ArtifactKind]bool{}
	for _, kind := range []ArtifactKind{ArtifactSpec, ArtifactPlan, ArtifactTasks} {
		fname := string(kind) + ".md"
		if _, err := os.Stat(filepath.Join(specDir, fname)); err == nil {
			present[kind] = true
		}
	}
	return present
}

func RenderLifecycleGuidance(state SpecLifecycleState) string {
	switch state {
	case LifecycleDraft:
		return "Next step: refine spec.md, then create plan.md/tasks.md and mark work in progress when implementation begins."
	case LifecycleInProgress:
		return "Next step: finish implementation, validate acceptance criteria, then mark the spec completed."
	case LifecycleCompleted:
		return "Next step: keep spec.md, plan.md, and tasks.md as history; optionally archive the whole folder under specs/archive/."
	case LifecycleArchived:
		return "This spec is archived. Refer to it as historical record or restore/copy it for follow-up work."
	default:
		return "Next step: verify the lifecycle metadata and continue the standard spec workflow."
	}
}

func RenderLifecycleStatus(specDir string, present map[ArtifactKind]bool, record SpecLifecycleRecord) string {
	var sb strings.Builder
	sb.WriteString(RenderStatus(specDir, present))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Lifecycle state: %s\n", record.State))
	if record.CreatedAt != "" {
		sb.WriteString(fmt.Sprintf("Created at: %s\n", record.CreatedAt))
	}
	if record.CompletedAt != "" {
		sb.WriteString(fmt.Sprintf("Completed at: %s\n", record.CompletedAt))
	}
	if record.ArchivedAt != "" {
		sb.WriteString(fmt.Sprintf("Archived at: %s\n", record.ArchivedAt))
	}
	if strings.TrimSpace(record.Notes) != "" {
		sb.WriteString(fmt.Sprintf("Notes: %s\n", strings.TrimSpace(record.Notes)))
	}
	if len(record.FollowUps) > 0 {
		sb.WriteString("Follow-ups:\n")
		for _, item := range record.FollowUps {
			sb.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	sb.WriteString(fmt.Sprintf("Guidance: %s\n", RenderLifecycleGuidance(record.State)))
	return sb.String()
}

func UniqueArchiveDestination(specsRoot, dirName string) (string, error) {
	archiveRoot := filepath.Join(specsRoot, "archive")
	if err := os.MkdirAll(archiveRoot, 0o755); err != nil {
		return "", err
	}
	candidate := filepath.Join(archiveRoot, dirName)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	}
	for i := 2; i < 10000; i++ {
		candidate = filepath.Join(archiveRoot, fmt.Sprintf("%s-%d", dirName, i))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to find unique archive destination for %s", dirName)
}
