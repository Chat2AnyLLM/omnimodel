package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// checkpointEveryNSteps controls how often agent state is written to disk.
	checkpointEveryNSteps = 5

	// checkpointDirEnv overrides the default checkpoint directory.
	checkpointDirEnv = "OMNILLM_CHECKPOINT_DIR"
)

// Checkpoint holds the serializable state of an in-progress agent run.
type Checkpoint struct {
	SessionID string    `json:"session_id"`
	Step      int       `json:"step"`
	Messages  []Message `json:"messages"`
	SavedAt   time.Time `json:"saved_at"`
}

// checkpointBaseDir returns the directory that holds checkpoint files.
func checkpointBaseDir() string {
	if d := os.Getenv(checkpointDirEnv); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "omnillm-checkpoints")
}

// checkpointPath returns the canonical path for sessionID's checkpoint file.
func checkpointPath(sessionID string) string {
	return filepath.Join(checkpointBaseDir(), safeFilename(sessionID)+".json")
}

// safeFilename converts an arbitrary string into a safe filename component.
// Only [A-Za-z0-9_-] are kept; everything else becomes '_'.
func safeFilename(s string) string {
	if s == "" {
		return "default"
	}
	out := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			out[i] = c
		} else {
			out[i] = '_'
		}
	}
	return string(out)
}

// saveCheckpoint atomically writes agent state to disk (write tmp then rename).
func saveCheckpoint(sessionID string, step int, messages []Message) error {
	dir := checkpointBaseDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("checkpoint mkdir: %w", err)
	}

	cp := Checkpoint{
		SessionID: sessionID,
		Step:      step,
		Messages:  messages,
		SavedAt:   time.Now(),
	}
	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("checkpoint marshal: %w", err)
	}

	path := checkpointPath(sessionID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("checkpoint write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("checkpoint rename: %w", err)
	}
	return nil
}

// loadCheckpoint reads a previously saved checkpoint.
// Returns (nil, nil) when no checkpoint exists for sessionID.
// Silently removes corrupt checkpoint files.
func loadCheckpoint(sessionID string) (*Checkpoint, error) {
	path := checkpointPath(sessionID)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("checkpoint read: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		// Corrupt file — remove silently so next run starts fresh.
		_ = os.Remove(path)
		return nil, nil
	}
	return &cp, nil
}

// clearCheckpoint removes the checkpoint file on successful run completion.
func clearCheckpoint(sessionID string) {
	_ = os.Remove(checkpointPath(sessionID))
}
