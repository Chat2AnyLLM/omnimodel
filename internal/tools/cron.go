package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
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

// parseCronNext returns the next-fire time from a cron spec.
func parseCronNext(spec string) (time.Time, error) {
	schedule, err := parseCronSpec(spec)
	if err != nil {
		return time.Time{}, err
	}
	next := schedule.Next(time.Now())
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("cron expression has no next run time")
	}
	return next, nil
}

func parseCronSpec(spec string) (cron.Schedule, error) {
	parser := cron.NewParser(
		cron.SecondOptional |
			cron.Minute |
			cron.Hour |
			cron.Dom |
			cron.Month |
			cron.Dow |
			cron.Descriptor,
	)
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return nil, fmt.Errorf("cron expression is required")
	}
	return parser.Parse(trimmed)
}

// ─── schedule_cron ────────────────────────────────────────────────────────────

type scheduleCronTool struct{}

func ScheduleCron() Tool { return &scheduleCronTool{} }

func (t *scheduleCronTool) Name() string { return "schedule_cron" }
func (t *scheduleCronTool) Description() string {
	return "Schedule a prompt or command to run on a cron schedule (5/6-field or descriptor like '@every 30s'). " +
		"Returns a job ID and runs in the background. Use task_stop to cancel and task_output to inspect results."
}
func (t *scheduleCronTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"cron":        map[string]any{"type": "string", "description": "Cron expression (5/6-field or descriptor, e.g. '@every 30s')."},
			"prompt":      map[string]any{"type": "string", "description": "The prompt or command to run at each scheduled time."},
			"recurring":   map[string]any{"type": "boolean", "description": "If true (default), fire on every match. If false, fire once then delete."},
			"target":      map[string]any{"type": "string", "description": "Optional worker target. Defaults to 'scheduler-worker'."},
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
		Target      string `json:"target"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.Cron = strings.TrimSpace(p.Cron)
	p.Prompt = strings.TrimSpace(p.Prompt)
	p.Target = strings.TrimSpace(p.Target)
	if p.Cron == "" || p.Prompt == "" {
		return Result{Output: "error: cron and prompt are required", IsError: true}
	}
	recurring := true
	if p.Recurring != nil {
		recurring = *p.Recurring
	}
	schedule, err := parseCronSpec(p.Cron)
	if err != nil {
		return Result{Output: "error: invalid cron expression: " + err.Error(), IsError: true}
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
		Status:      TaskRunRunning,
		Output:      fmt.Sprintf("Scheduled cron job. Next fire: %s", nextFire.Format(time.RFC3339)),
	}
	taskCtx, cancel := context.WithCancel(context.Background())
	run.cancel = cancel
	store.add(run)

	target := p.Target
	if target == "" {
		target = "scheduler-worker"
	}

	go func() {
		for {
			next := schedule.Next(time.Now())
			if next.IsZero() {
				setTaskRunStatus(store, run, TaskRunFailed)
				appendTaskRunOutput(store, run, "cron schedule produced no next fire time")
				return
			}

			wait := time.Until(next)
			if wait < 0 {
				wait = 0
			}
			timer := time.NewTimer(wait)
			select {
			case <-taskCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}

			output, dispatchErr := dispatchScheduledPrompt(taskCtx, run, p.Prompt, target, call.SendMessageFn)
			if dispatchErr != nil {
				appendTaskRunOutput(store, run, fmt.Sprintf("[%s] dispatch error: %v", time.Now().Format(time.RFC3339), dispatchErr))
				if !recurring {
					setTaskRunStatus(store, run, TaskRunFailed)
					return
				}
			} else {
				appendTaskRunOutput(store, run, fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), output))
			}

			if !recurring {
				setTaskRunStatus(store, run, TaskRunCompleted)
				return
			}
		}
	}()

	return Result{
		Title: "Cron job scheduled",
		Output: fmt.Sprintf(
			"Job ID: %s\nSchedule: %s (%s)\nNext fire: %s\nTarget: %s\nPrompt: %s",
			id, p.Cron, recurStr, nextFire.Format(time.RFC3339), target, p.Prompt,
		),
	}
}

func truncateTo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ─── schedule_heartbeat ─────────────────────────────────────────────────────

type scheduleHeartbeatTool struct{}

func ScheduleHeartbeat() Tool { return &scheduleHeartbeatTool{} }

func (t *scheduleHeartbeatTool) Name() string { return "schedule_heartbeat" }
func (t *scheduleHeartbeatTool) Description() string {
	return "Schedule a heartbeat automation entry for periodic checks or reminders. " +
		"This creates a managed task entry that can be tracked via task_list/task_get/task_stop."
}
func (t *scheduleHeartbeatTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"interval_seconds": map[string]any{"type": "integer", "description": "Heartbeat interval in seconds."},
			"prompt":           map[string]any{"type": "string", "description": "Prompt/instruction to execute each heartbeat."},
			"target":           map[string]any{"type": "string", "description": "Optional worker/target label for routing."},
		},
		"required": []string{"interval_seconds", "prompt"},
	}
}
func (t *scheduleHeartbeatTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		IntervalSeconds int    `json:"interval_seconds"`
		Prompt          string `json:"prompt"`
		Target          string `json:"target"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.Prompt = strings.TrimSpace(p.Prompt)
	p.Target = strings.TrimSpace(p.Target)
	if p.IntervalSeconds <= 0 {
		return Result{Output: "error: interval_seconds must be > 0", IsError: true}
	}
	if p.Prompt == "" {
		return Result{Output: "error: prompt is required", IsError: true}
	}
	if call.TaskStore == nil {
		return Result{Output: "error: task store not available", IsError: true}
	}

	id := call.TaskStore.nextID()
	desc := fmt.Sprintf("heartbeat every %ds", p.IntervalSeconds)
	if p.Target != "" {
		desc += " -> " + p.Target
	}
	target := p.Target
	if target == "" {
		target = "heartbeat-worker"
	}
	run := &TaskRun{
		ID:          id,
		Description: desc,
		Status:      TaskRunRunning,
		Output:      fmt.Sprintf("Heartbeat scheduled: every %d seconds\nTarget: %s\nPrompt: %s", p.IntervalSeconds, target, p.Prompt),
	}
	taskCtx, cancel := context.WithCancel(context.Background())
	run.cancel = cancel
	call.TaskStore.add(run)

	go func() {
		ticker := time.NewTicker(time.Duration(p.IntervalSeconds) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-taskCtx.Done():
				return
			case <-ticker.C:
				output, dispatchErr := dispatchScheduledPrompt(taskCtx, run, p.Prompt, target, call.SendMessageFn)
				if dispatchErr != nil {
					appendTaskRunOutput(call.TaskStore, run, fmt.Sprintf("[%s] dispatch error: %v", time.Now().Format(time.RFC3339), dispatchErr))
					continue
				}
				appendTaskRunOutput(call.TaskStore, run, fmt.Sprintf("[%s] %s", time.Now().Format(time.RFC3339), output))
			}
		}
	}()

	return Result{
		Title:  "Heartbeat scheduled",
		Output: fmt.Sprintf("Job ID: %s\nInterval: %ds\nTarget: %s\nPrompt: %s", id, p.IntervalSeconds, target, p.Prompt),
	}
}

// ─── trigger_event ──────────────────────────────────────────────────────────

type triggerEventTool struct{}

func TriggerEvent() Tool { return &triggerEventTool{} }

func (t *triggerEventTool) Name() string { return "trigger_event" }
func (t *triggerEventTool) Description() string {
	return "Trigger an automation event and optionally route it to a named worker. " +
		"Useful for harness-native event-driven workflows."
}
func (t *triggerEventTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"event_type": map[string]any{"type": "string", "description": "Event type name."},
			"payload":    map[string]any{"type": "string", "description": "Event payload or summary."},
			"target":     map[string]any{"type": "string", "description": "Optional worker target."},
		},
		"required": []string{"event_type"},
	}
}
func (t *triggerEventTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		EventType string `json:"event_type"`
		Payload   string `json:"payload"`
		Target    string `json:"target"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	p.EventType = strings.TrimSpace(p.EventType)
	p.Payload = strings.TrimSpace(p.Payload)
	p.Target = strings.TrimSpace(p.Target)
	if p.EventType == "" {
		return Result{Output: "error: event_type is required", IsError: true}
	}

	message := fmt.Sprintf("Event: %s", p.EventType)
	if p.Payload != "" {
		message += "\nPayload: " + p.Payload
	}

	if call.SendMessageFn != nil {
		target := p.Target
		if target == "" {
			target = "event-handler"
		}
		resp, err := call.SendMessageFn(ctx, target, message)
		if err != nil {
			return Result{Output: "error: failed to dispatch event: " + err.Error(), IsError: true}
		}
		return Result{Title: "Event triggered", Output: fmt.Sprintf("Target: %s\nResponse:\n%s", target, strings.TrimSpace(resp))}
	}

	if call.TaskStore != nil {
		id := call.TaskStore.nextID()
		call.TaskStore.add(&TaskRun{
			ID:          id,
			Description: "event " + p.EventType,
			Status:      TaskRunPending,
			Output:      message,
		})
		return Result{Title: "Event queued", Output: fmt.Sprintf("Event %s queued as task %s", p.EventType, id)}
	}

	return Result{Output: fmt.Sprintf("Event %s accepted (no dispatcher configured)", p.EventType)}
}

func dispatchScheduledPrompt(
	ctx context.Context,
	run *TaskRun,
	prompt,
	target string,
	sendMessageFn func(context.Context, string, string) (string, error),
) (string, error) {
	if sendMessageFn == nil {
		return fmt.Sprintf("scheduled tick fired for %q (no dispatcher configured)", run.ID), nil
	}
	resp, err := sendMessageFn(ctx, target, prompt)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(resp)
	if trimmed == "" {
		return fmt.Sprintf("scheduled tick fired for %q", run.ID), nil
	}
	return trimmed, nil
}

func appendTaskRunOutput(store *TaskStore, run *TaskRun, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if run.Output == "" {
		run.Output = line
		return
	}
	run.Output += "\n" + line
}

func setTaskRunStatus(store *TaskStore, run *TaskRun, status TaskRunStatus) {
	store.mu.Lock()
	defer store.mu.Unlock()
	run.Status = status
}

func defaultLabel(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// ─── delete_cron / list_crons reuse task_stop / task_list via TaskStore ───────
// They are intentionally thin wrappers so the agent can use task_stop <id> and
// task_list to manage cron jobs without a separate store.
