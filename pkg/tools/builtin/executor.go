// Package builtin — pluggable executor interface for the bash tool.
//
// By default the bash tool runs commands in a local subprocess. Callers can
// supply an alternative Executor implementation to delegate execution to:
//   - A remote SSH host
//   - A Docker container
//   - A sandboxed environment (gVisor, Firecracker, etc.)
//   - A CI/CD runner
//
// Usage:
//
//	// Custom executor — run in Docker container
//	type dockerExecutor struct{ containerID string }
//
//	func (e *dockerExecutor) Exec(ctx context.Context, command, cwd string, onData func(string)) (int, error) {
//	    cmd := exec.CommandContext(ctx, "docker", "exec", "-w", cwd, e.containerID, "bash", "-c", command)
//	    // ... stream stdout/stderr → onData(chunk)
//	    return exitCode, err
//	}
//
//	tool := builtin.NewBashToolWithExecutor("/repo", &dockerExecutor{"my-container"})
package builtin

import (
	"context"
	"io"
	"os/exec"
)

// Executor abstracts the mechanism by which bash commands are run.
// Implement this interface to delegate command execution to any backend.
type Executor interface {
	// Exec runs command in the given working directory.
	// onData is called with chunks of combined stdout+stderr as they arrive;
	// it may be nil (batch mode).
	// Returns the process exit code and any execution error (distinct from
	// a non-zero exit code).
	Exec(ctx context.Context, command, cwd string, onData func(chunk string)) (exitCode int, err error)
}

// ---------------------------------------------------------------------------
// LocalExecutor — default implementation
// ---------------------------------------------------------------------------

// LocalExecutor runs commands in a local bash subprocess.
// This is the default used by NewBashTool.
type LocalExecutor struct{}

// Exec implements Executor by spawning `bash -c command` locally.
func (e *LocalExecutor) Exec(ctx context.Context, command, cwd string, onData func(chunk string)) (int, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	// Stream output to onData in a goroutine.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 32*1024)
		for {
			n, err := pr.Read(buf)
			if n > 0 && onData != nil {
				onData(string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
	}()

	cmdErr := cmd.Wait()
	pw.Close()
	<-readDone

	code := 0
	if cmdErr != nil {
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
			// Non-zero exit is not an executor error — it's a command result.
			return code, nil
		}
		return -1, cmdErr
	}
	return code, nil
}
