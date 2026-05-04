package tools

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

func runShellCommand(ctx context.Context, command string, timeoutSeconds int) Result {
	timeout := defaultTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cmdCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(cmdCtx, "sh", "-lc", command)
	}

	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return Result{Output: "error: " + err.Error(), IsError: true}
		}
		return Result{Output: text + "\n(error: " + err.Error() + ")", IsError: true}
	}
	if text == "" {
		return Result{Output: "(no output)"}
	}
	return Result{Output: text}
}
