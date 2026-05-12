package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const multiAgentRootDir = ".omnicode/multi-agent"

var orchestratorStore = struct {
	mu       sync.Mutex
	sessions map[string]*LeaderWorkerOrchestrator
}{
	sessions: make(map[string]*LeaderWorkerOrchestrator),
}

type LeaderWorkerOrchestrator struct {
	mu        sync.Mutex
	sessionID string
	opts      SubAgentOptions
	rootDir   string
	workers   map[string]*leaderWorker
	reqSeq    uint64
}

type leaderWorker struct {
	mu        sync.Mutex
	name      string
	sessionID string
	history   []HistoryMessage
}

type mailboxMessage struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Worker    string `json:"worker"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	Error     string `json:"error,omitempty"`
}

func SessionOrchestrator(sessionID string, opts SubAgentOptions) *LeaderWorkerOrchestrator {
	normalizedID := sanitizeSubAgentID(strings.TrimSpace(sessionID))
	if normalizedID == "" {
		normalizedID = "session"
	}
	orchestratorStore.mu.Lock()
	defer orchestratorStore.mu.Unlock()
	if existing := orchestratorStore.sessions[normalizedID]; existing != nil {
		existing.setOptions(opts)
		return existing
	}
	orch := &LeaderWorkerOrchestrator{
		sessionID: normalizedID,
		opts:      opts,
		rootDir:   filepath.Join(workspaceDir(), multiAgentRootDir, "sessions", normalizedID),
		workers:   make(map[string]*leaderWorker),
	}
	orchestratorStore.sessions[normalizedID] = orch
	return orch
}

func (o *LeaderWorkerOrchestrator) setOptions(opts SubAgentOptions) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.opts = opts
}

func (o *LeaderWorkerOrchestrator) SendMessage(ctx context.Context, to, message string) (string, error) {
	target := sanitizeSubAgentID(strings.TrimSpace(to))
	if target == "" {
		target = "worker"
	}
	body := strings.TrimSpace(message)
	if body == "" {
		return "", fmt.Errorf("message is required")
	}

	worker := o.getOrCreateWorker(target)
	reqID := fmt.Sprintf("%s-%d", target, atomic.AddUint64(&o.reqSeq, 1))
	req := mailboxMessage{
		ID:        reqID,
		SessionID: o.sessionID,
		Worker:    target,
		Body:      body,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := o.writeMailboxMessage(target, "inbox", reqID, req); err != nil {
		return "", fmt.Errorf("write worker inbox: %w", err)
	}

	worker.mu.Lock()
	defer worker.mu.Unlock()

	opts := o.currentOptions()
	result, err := runIsolatedSubAgentTurn(ctx, opts, worker.sessionID, body, worker.history)
	if err != nil {
		resp := mailboxMessage{
			ID:        reqID,
			SessionID: o.sessionID,
			Worker:    target,
			Body:      "",
			Error:     err.Error(),
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		_ = o.writeMailboxMessage(target, "outbox", reqID, resp)
		return "", fmt.Errorf("worker %q failed: %w", target, err)
	}

	output := strings.TrimSpace(result.Output)
	worker.history = append(worker.history,
		HistoryMessage{Role: "user", Content: body},
		HistoryMessage{Role: "assistant", Content: output},
	)

	resp := mailboxMessage{
		ID:        reqID,
		SessionID: o.sessionID,
		Worker:    target,
		Body:      output,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := o.writeMailboxMessage(target, "outbox", reqID, resp); err != nil {
		return "", fmt.Errorf("write worker outbox: %w", err)
	}

	wsDir := workspaceDir()
	_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] orchestrator_message worker=%q id=%s", o.sessionID, target, reqID))
	return output, nil
}

func (o *LeaderWorkerOrchestrator) currentOptions() SubAgentOptions {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.opts
}

func (o *LeaderWorkerOrchestrator) getOrCreateWorker(name string) *leaderWorker {
	o.mu.Lock()
	defer o.mu.Unlock()
	if existing := o.workers[name]; existing != nil {
		return existing
	}
	worker := &leaderWorker{
		name:      name,
		sessionID: fmt.Sprintf("%s-worker-%s", o.sessionID, name),
		history:   make([]HistoryMessage, 0, 8),
	}
	o.workers[name] = worker
	return worker
}

func (o *LeaderWorkerOrchestrator) writeMailboxMessage(workerName, box, id string, payload mailboxMessage) error {
	if box != "inbox" && box != "outbox" {
		return fmt.Errorf("unsupported mailbox: %s", box)
	}
	base := filepath.Join(o.rootDir, "workers", workerName, box)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	filePath := filepath.Join(base, id+".json")
	return os.WriteFile(filePath, append(data, '\n'), 0o600)
}
