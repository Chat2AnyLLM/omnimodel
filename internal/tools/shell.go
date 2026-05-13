package tools

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"omnillm/internal/sandbox"
)

const defaultTimeout = 30 * time.Second

const (
	sandboxModeEnv      = "OMNILLM_AGENT_SANDBOX_MODE"
	sandboxImageEnv     = "OMNILLM_AGENT_SANDBOX_IMAGE"
	pwshSandboxImageEnv = "OMNILLM_AGENT_POWERSHELL_SANDBOX_IMAGE"
	sandboxNetworkEnv   = "OMNILLM_AGENT_SANDBOX_NETWORK"
	sandboxAllowlistEnv = "OMNILLM_AGENT_SANDBOX_NETWORK_ALLOWLIST"
	// sandboxSeccompEnv optionally points to a custom seccomp profile JSON.
	// When unset Docker's default seccomp profile is used (recommended).
	sandboxSeccompEnv = "OMNILLM_AGENT_SANDBOX_SECCOMP"
)

var errSandboxNetworkNotAllowed = errors.New("requested sandbox network is not in allowlist")

func RunShellCommand(ctx context.Context, command string, timeoutSeconds int) Result {
	return runShellCommand(ctx, command, timeoutSeconds)
}

func runBashCommand(ctx context.Context, command string, timeoutSeconds int) Result {
	timeout := defaultTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	mode := strings.ToLower(strings.TrimSpace(os.Getenv(sandboxModeEnv)))
	if mode == "docker" {
		return runBashCommandInDocker(ctx, command, timeout)
	}
	if mode == "firecracker" {
		return runBashCommandInFirecracker(ctx, command, timeout)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return runHostBashCommand(cmdCtx, command)
}

func runPowerShellCommand(ctx context.Context, command string, timeoutSeconds int) Result {
	timeout := defaultTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	mode := strings.ToLower(strings.TrimSpace(os.Getenv(sandboxModeEnv)))
	if mode == "docker" {
		return runPowerShellCommandInDocker(ctx, command, timeout)
	}
	if mode == "firecracker" {
		return runPowerShellCommandInFirecracker(ctx, command, timeout)
	}

	return runShellCommand(ctx, command, timeoutSeconds)
}

func parseAllowlist(raw string) map[string]bool {
	out := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		out[name] = true
	}
	return out
}

func resolveDockerNetwork() (string, error) {
	network := strings.TrimSpace(os.Getenv(sandboxNetworkEnv))
	if network == "" {
		return "none", nil
	}
	allowed := parseAllowlist(strings.TrimSpace(os.Getenv(sandboxAllowlistEnv)))
	if len(allowed) > 0 && !allowed[network] {
		return "", errSandboxNetworkNotAllowed
	}
	return network, nil
}

func runBashCommandInDocker(ctx context.Context, command string, timeout time.Duration) Result {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	image := strings.TrimSpace(os.Getenv(sandboxImageEnv))
	if image == "" {
		image = "alpine:3.20"
	}

	args := []string{
		"run", "--rm",
		"--cpus", "1.0",
		"--memory", "512m",
		"--pids-limit", "100",
		"--read-only",
		"--tmpfs", "/tmp:size=100m",
		"--cap-drop", "ALL",
		"--user", "nobody:nobody",
	}
	network, err := resolveDockerNetwork()
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	args = append(args, "--network", network)
	if sp := strings.TrimSpace(os.Getenv(sandboxSeccompEnv)); sp != "" {
		args = append(args, "--security-opt", "seccomp="+sp)
	}

	if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
		args = append(args, "--workdir", "/workspace", "-v", wd+":/workspace:ro")
	}

	args = append(args, image, "sh", "-lc", command)

	dockerCmd := exec.CommandContext(cmdCtx, "docker", args...)
	output, err := dockerCmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return Result{Output: "error: docker sandbox execution failed: " + err.Error(), IsError: true}
		}
		return Result{Output: text + "\n(error: docker sandbox execution failed: " + err.Error() + ")", IsError: true}
	}
	if text == "" {
		return Result{Output: "(no output)"}
	}
	return Result{Output: text}
}

func runPowerShellCommandInDocker(ctx context.Context, command string, timeout time.Duration) Result {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	image := strings.TrimSpace(os.Getenv(pwshSandboxImageEnv))
	if image == "" {
		image = "mcr.microsoft.com/powershell:7.4-alpine-3.18"
	}

	args := []string{
		"run", "--rm",
		"--cpus", "1.0",
		"--memory", "512m",
		"--pids-limit", "100",
		"--read-only",
		"--tmpfs", "/tmp:size=100m",
		"--cap-drop", "ALL",
		"--user", "nobody:nobody",
	}
	network, err := resolveDockerNetwork()
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	args = append(args, "--network", network)
	if sp := strings.TrimSpace(os.Getenv(sandboxSeccompEnv)); sp != "" {
		args = append(args, "--security-opt", "seccomp="+sp)
	}

	if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
		args = append(args, "--workdir", "/workspace", "-v", wd+":/workspace:ro")
	}

	args = append(args, image, "pwsh", "-NoProfile", "-NonInteractive", "-Command", command)

	dockerCmd := exec.CommandContext(cmdCtx, "docker", args...)
	output, err := dockerCmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return Result{Output: "error: docker sandbox execution failed: " + err.Error(), IsError: true}
		}
		return Result{Output: text + "\n(error: docker sandbox execution failed: " + err.Error() + ")", IsError: true}
	}
	if text == "" {
		return Result{Output: "(no output)"}
	}
	return Result{Output: text}
}

func runBashCommandInFirecracker(ctx context.Context, command string, timeout time.Duration) Result {
	text, err := sandbox.RunInFirecracker(ctx, "bash", command, timeout)
	if err != nil {
		if strings.TrimSpace(text) == "" {
			return Result{Output: "error: " + err.Error(), IsError: true}
		}
		return Result{Output: text + "\n(error: " + err.Error() + ")", IsError: true}
	}
	if strings.TrimSpace(text) == "" {
		return Result{Output: "(no output)"}
	}
	return Result{Output: text}
}

func runPowerShellCommandInFirecracker(ctx context.Context, command string, timeout time.Duration) Result {
	text, err := sandbox.RunInFirecracker(ctx, "powershell", command, timeout)
	if err != nil {
		if strings.TrimSpace(text) == "" {
			return Result{Output: "error: " + err.Error(), IsError: true}
		}
		return Result{Output: text + "\n(error: " + err.Error() + ")", IsError: true}
	}
	if strings.TrimSpace(text) == "" {
		return Result{Output: "(no output)"}
	}
	return Result{Output: text}
}

func runShellCommand(ctx context.Context, command string, timeoutSeconds int) Result {
	timeout := defaultTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return runHostShellCommand(cmdCtx, command)
}

func runHostShellCommand(ctx context.Context, command string) Result {

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-lc", command)
	}

	output, err := cmd.CombinedOutput()
	if runtime.GOOS == "windows" && err != nil {
		text := strings.TrimSpace(string(output))
		if shouldRetryWithBashOnWindows(text) {
			return runHostBashCommand(ctx, command)
		}
	}
	return resultFromCommandOutput(output, err)
}

func runHostBashCommand(ctx context.Context, command string) Result {
	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath("bash"); err == nil {
			cmd := exec.CommandContext(ctx, path, "-lc", command)
			output, runErr := cmd.CombinedOutput()
			return resultFromCommandOutput(output, runErr)
		}
		if path, err := exec.LookPath("wsl"); err == nil {
			cmd := exec.CommandContext(ctx, path, "bash", "-lc", command)
			output, runErr := cmd.CombinedOutput()
			return resultFromCommandOutput(output, runErr)
		}
		return Result{Output: "error: bash tool requires a bash-compatible runtime on Windows (install Git Bash or WSL), or use the powershell tool", IsError: true}
	}

	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	output, err := cmd.CombinedOutput()
	return resultFromCommandOutput(output, err)
}

func resultFromCommandOutput(output []byte, err error) Result {
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

func shouldRetryWithBashOnWindows(output string) bool {
	if output == "" {
		return false
	}
	return strings.Contains(output, "The token '&&' is not a valid statement separator") ||
		strings.Contains(output, "The token '||' is not a valid statement separator")
}
