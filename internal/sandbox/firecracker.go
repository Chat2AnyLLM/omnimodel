package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const firecrackerRunnerEnv = "OMNILLM_AGENT_FIRECRACKER_RUNNER"

// RunInFirecracker executes command using an external Firecracker runner.
// The runner binary/path is configured by OMNILLM_AGENT_FIRECRACKER_RUNNER.
//
// Expected runner contract:
//
//	runner --shell <bash|powershell> --timeout-seconds <n> --command <cmd>
func RunInFirecracker(ctx context.Context, shell, command string, timeout time.Duration) (string, error) {
	runner := strings.TrimSpace(os.Getenv(firecrackerRunnerEnv))
	if runner == "" {
		return "", fmt.Errorf("firecracker sandbox requested but %s is not set", firecrackerRunnerEnv)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"--shell", shell,
		"--timeout-seconds", fmt.Sprintf("%d", int(timeout.Seconds())),
		"--command", command,
	}
	runnerCmd := exec.CommandContext(cmdCtx, runner, args...)
	out, err := runnerCmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("firecracker sandbox execution failed: %w", err)
		}
		return text, fmt.Errorf("firecracker sandbox execution failed: %w", err)
	}
	return text, nil
}
