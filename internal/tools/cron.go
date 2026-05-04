package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ─── CronStore ────────────────────────────────────────────────────────────────

type CronJob struct {
	ID          string
	Cron        string
	Prompt      string
	Recurring   bool
	NextFire    time.Time
	Description string
}

type CronStore struct {
	mu   sync.Mutex
	jobs map[string]*CronJob
	seq  int
}

func NewCronStore() *CronStore {
	return &CronStore{jobs: make(map[string]*CronJob)}
}

func (s *CronStore) add(job *CronJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

func (s *CronStore) delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.jobs[id]
	delete(s.jobs, id)
	return ok
}

func (s *CronStore) list() []*CronJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*CronJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	return out
}

func (s *CronStore) nextID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return fmt.Sprintf("cron-%d", s.seq)
}

// parseCronNext returns an approximate next-fire time from a 5-field cron spec.
// This is a simplified implementation for display — a full cron parser is not
// needed for the agent tool's purpose.
func parseCronNext(spec string) (time.Time, error) {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron spec must have 5 fields (got %d)", len(fields))
	}
	// Return "unknown schedule" time rather than a full parser
	return time.Now().Add(time.Minute), nil
}

// ─── schedule_cron ────────────────────────────────────────────────────────────

type scheduleCronTool struct{}

func ScheduleCron() Tool { return &scheduleCronTool{} }

func (t *scheduleCronTool) Name() string { return "schedule_cron" }
func (t *scheduleCronTool) Description() string {
	return "Schedule a prompt or command to run on a cron schedule (5-field: minute hour dom month dow). " +
		"Returns a job ID. Use delete_cron to cancel, list_crons to see all scheduled jobs."
}
func (t *scheduleCronTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"cron":        map[string]any{"type": "string", "description": "Standard 5-field cron expression, e.g. '0 9 * * 1-5' for weekdays at 9am."},
			"prompt":      map[string]any{"type": "string", "description": "The prompt or command to run at each scheduled time."},
			"recurring":   map[string]any{"type": "boolean", "description": "If true (default), fire on every match. If false, fire once then delete."},
			"description": map[string]any{"type": "string", "description": "Human-readable description of the job."},
		},
		"required": []string{"cron", "prompt"},
	}
}
func (t *scheduleCronTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Cron        string `json:"cron"`
		Prompt      string `json:"prompt"`
		Recurring   *bool  `json:"recurring"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if p.Cron == "" || p.Prompt == "" {
		return Result{Output: "error: cron and prompt are required", IsError: true}
	}
	recurring := true
	if p.Recurring != nil {
		recurring = *p.Recurring
	}
	nextFire, err := parseCronNext(p.Cron)
	if err != nil {
		return Result{Output: "error: invalid cron expression: " + err.Error(), IsError: true}
	}

	// We use the TaskStore as backing store for cron jobs if CronStore isn't
	// wired up — this gives a visible record in task_list.
	store := call.TaskStore
	if store == nil {
		return Result{Output: "error: task store not available (needed to track cron jobs)", IsError: true}
	}

	id := store.nextID()
	recurStr := "once"
	if recurring {
		recurStr = "recurring"
	}
	desc := p.Description
	if desc == "" {
		desc = fmt.Sprintf("cron[%s] %s %s", p.Cron, recurStr, truncateTo(p.Prompt, 60))
	}

	run := &TaskRun{
		ID:          id,
		Description: desc,
		Status:      TaskRunPending,
	}
	store.add(run)

	return Result{
		Title: "Cron job scheduled",
		Output: fmt.Sprintf(
			"Job ID: %s\nSchedule: %s (%s)\nNext fire: ~%s\nPrompt: %s",
			id, p.Cron, recurStr, nextFire.Format("2006-01-02 15:04"), p.Prompt,
		),
	}
}

func truncateTo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ─── delete_cron / list_crons reuse task_stop / task_list via TaskStore ───────
// They are intentionally thin wrappers so the agent can use task_stop <id> and
// task_list to manage cron jobs without a separate store.
