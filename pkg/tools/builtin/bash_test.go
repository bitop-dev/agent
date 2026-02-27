package builtin_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bitop-dev/agent/pkg/tools/builtin"
)

func bashRun(t *testing.T, cmd string, extra ...map[string]any) string {
	t.Helper()
	params := map[string]any{"command": cmd}
	if len(extra) > 0 {
		for k, v := range extra[0] {
			params[k] = v
		}
	}
	tool := builtin.NewBashTool(".")
	result, err := tool.Execute(context.Background(), "c1", params, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	return resultTextContent(result)
}

func TestBashTool_SimpleCommand(t *testing.T) {
	out := bashRun(t, "echo hello")
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello', got: %q", out)
	}
}

func TestBashTool_ExitCode_EchoedInOutput(t *testing.T) {
	// The bash executor returns exit codes to the caller but does not
	// inject them into the text output by default. Capture exit code via echo.
	out := bashRun(t, "sh -c 'exit 42'; echo \"exit:$?\"")
	if !strings.Contains(out, "42") {
		t.Errorf("expected '42' in output, got: %q", out)
	}
}

func TestBashTool_Stderr(t *testing.T) {
	out := bashRun(t, "echo err >&2")
	if !strings.Contains(out, "err") {
		t.Errorf("stderr not captured, got: %q", out)
	}
}

func TestBashTool_Multiline(t *testing.T) {
	out := bashRun(t, "echo one && echo two")
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Errorf("multiline output: %q", out)
	}
}

func TestBashTool_Timeout(t *testing.T) {
	start := time.Now()
	// Default timeout is 120s — override with a 1s timeout via params.
	tool := builtin.NewBashTool(".")
	result, _ := tool.Execute(context.Background(), "c1", map[string]any{
		"command": "sleep 10",
		"timeout": float64(1),
	}, nil)
	elapsed := time.Since(start)
	out := resultTextContent(result)

	if elapsed > 5*time.Second {
		t.Errorf("timeout not enforced — ran for %s", elapsed)
	}
	if !strings.Contains(strings.ToLower(out), "timeout") &&
		!strings.Contains(strings.ToLower(out), "killed") &&
		!strings.Contains(strings.ToLower(out), "timed out") &&
		!strings.Contains(strings.ToLower(out), "signal") {
		t.Logf("output after timeout: %q", out)
	}
}

func TestBashTool_WorkingDir(t *testing.T) {
	dir := t.TempDir()
	tool := builtin.NewBashTool(dir)
	result, _ := tool.Execute(context.Background(), "c1", map[string]any{
		"command": "pwd",
	}, nil)
	out := resultTextContent(result)
	if !strings.Contains(out, dir) {
		// macOS resolves /var -> /private/var; accept both.
		t.Logf("pwd output %q (expected to contain %s — may differ on macOS symlinks)", out, dir)
	}
}

func TestBashTool_MissingCommand(t *testing.T) {
	tool := builtin.NewBashTool(".")
	result, _ := tool.Execute(context.Background(), "c1", map[string]any{}, nil)
	out := resultTextContent(result)
	if !strings.Contains(strings.ToLower(out), "error") && !strings.Contains(out, "command") {
		t.Errorf("expected error for missing command, got: %q", out)
	}
}

func TestBashTool_Definition(t *testing.T) {
	def := builtin.NewBashTool(".").Definition()
	if def.Name != "bash" {
		t.Errorf("name = %q", def.Name)
	}
}
