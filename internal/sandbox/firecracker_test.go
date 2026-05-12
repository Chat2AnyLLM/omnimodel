package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunInFirecrackerRequiresRunnerEnv(t *testing.T) {
	t.Setenv(firecrackerRunnerEnv, "")
	_, err := RunInFirecracker(context.Background(), "bash", "echo hi", 2*time.Second)
	if err == nil {
		t.Fatal("expected error when firecracker runner env is missing")
	}
	if !strings.Contains(err.Error(), firecrackerRunnerEnv) {
		t.Fatalf("expected error to mention env var, got: %v", err)
	}
}
