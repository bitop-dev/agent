package builtin

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// BashTool executes shell commands and streams their output.
// Output is tail-truncated to DefaultMaxLines / DefaultMaxBytes; the full
// output is saved to a temp file when it exceeds that limit.
type BashTool struct {
	cwd      string
	executor Executor
}

// NewBashTool creates a BashTool that runs commands locally.
func NewBashTool(cwd string) *BashTool {
	return &BashTool{cwd: cwd, executor: &LocalExecutor{}}
}

// NewBashToolWithExecutor creates a BashTool that delegates execution to exec.
// Use this to run commands in Docker, SSH, sandboxes, etc.
func NewBashToolWithExecutor(cwd string, exec Executor) *BashTool {
	if exec == nil {
		exec = &LocalExecutor{}
	}
	return &BashTool{cwd: cwd, executor: exec}
}

func (t *BashTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name: "bash",
		Description: fmt.Sprintf(
			"Execute a bash command in the current working directory. Returns stdout and stderr. "+
				"Output is truncated to last %d lines or %s (whichever is hit first). "+
				"If truncated, full output is saved to a temp file. "+
				"Optionally provide a timeout in seconds.",
			DefaultMaxLines, FormatSize(DefaultMaxBytes),
		),
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"command": {Type: "string", Description: "Bash command to execute"},
				"timeout": {Type: "number", Description: "Timeout in seconds (optional, no default timeout)"},
			},
			Required: []string{"command"},
		}),
	}
}

func (t *BashTool) Execute(ctx context.Context, _ string, params map[string]any, onUpdate tools.UpdateFn) (tools.Result, error) {
	command, _ := params["command"].(string)
	if command == "" {
		return tools.ErrorResult(fmt.Errorf("command is required")), nil
	}

	var timeoutSecs float64
	if v, ok := params["timeout"]; ok {
		switch n := v.(type) {
		case float64:
			timeoutSecs = n
		case int:
			timeoutSecs = float64(n)
		}
	}

	if timeoutSecs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSecs*float64(time.Second)))
		defer cancel()
	}

	return t.run(ctx, command, timeoutSecs, onUpdate)
}

func (t *BashTool) run(ctx context.Context, command string, timeoutSecs float64, onUpdate tools.UpdateFn) (tools.Result, error) {
	// Rolling buffer state (shared between the executor's onData callback and
	// the main goroutine, protected by mu)
	var mu sync.Mutex
	var chunks [][]byte // rolling window of recent data
	var chunksBytes int
	var totalBytes int
	var tempFile *os.File
	var tempPath string

	const maxChunksBytes = DefaultMaxBytes * 2

	onData := func(chunk string) {
		data := []byte(chunk)
		mu.Lock()
		totalBytes += len(data)

		// Open temp file once we exceed the limit
		if totalBytes > DefaultMaxBytes && tempFile == nil {
			if tf, terr := os.CreateTemp("", "agent-bash-*.log"); terr == nil {
				tempFile = tf
				tempPath = tf.Name()
				for _, c := range chunks {
					tf.Write(c)
				}
			}
		}
		if tempFile != nil {
			tempFile.Write(data)
		}

		chunks = append(chunks, data)
		chunksBytes += len(data)
		// Trim old chunks
		for chunksBytes > maxChunksBytes && len(chunks) > 1 {
			chunksBytes -= len(chunks[0])
			chunks = chunks[1:]
		}

		if onUpdate != nil {
			combined := combineChunks(chunks)
			tr := TruncateTail(string(combined), DefaultMaxLines, DefaultMaxBytes)
			mu.Unlock()
			onUpdate(tools.Result{
				Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: tr.Content}},
			})
		} else {
			mu.Unlock()
		}
	}

	_, execErr := t.executor.Exec(ctx, command, t.cwd, onData)

	if tempFile != nil {
		tempFile.Close()
	}

	mu.Lock()
	combined := combineChunks(chunks)
	tp := tempPath
	tb := totalBytes
	mu.Unlock()

	fullOutput := string(combined)
	tr := TruncateTail(fullOutput, DefaultMaxLines, DefaultMaxBytes)

	timedOut := ctx.Err() == context.DeadlineExceeded
	aborted := ctx.Err() == context.Canceled

	outputText := tr.Content
	if outputText == "" {
		outputText = "(no output)"
	}

	// Append truncation notice
	if tr.Truncated {
		startLine := tr.TotalLines - tr.OutputLines + 1
		endLine := tr.TotalLines
		if tr.LastLinePartial {
			lastLineSize := FormatSize(len([]byte(fullOutput[len(fullOutput)-len(tr.Content):])))
			outputText += fmt.Sprintf(
				"\n\n[Showing last %s of line %d (line is %s). Full output: %s]",
				FormatSize(tr.OutputBytes), endLine, lastLineSize, tp,
			)
		} else if tr.TruncatedBy == "lines" {
			outputText += fmt.Sprintf(
				"\n\n[Showing lines %d-%d of %d. Full output: %s]",
				startLine, endLine, tr.TotalLines, tp,
			)
		} else {
			outputText += fmt.Sprintf(
				"\n\n[Showing lines %d-%d of %d (%s limit). Full output: %s]",
				startLine, endLine, tr.TotalLines, FormatSize(DefaultMaxBytes), tp,
			)
		}
	} else if tb > DefaultMaxBytes && tp != "" {
		outputText += fmt.Sprintf("\n\n[Full output: %s]", tp)
	}

	// Append error notices
	switch {
	case aborted:
		if outputText != "(no output)" {
			outputText += "\n\n"
		} else {
			outputText = ""
		}
		outputText += "Command aborted"
		return tools.TextResult(outputText), fmt.Errorf("command aborted")

	case timedOut:
		if outputText != "(no output)" {
			outputText += "\n\n"
		} else {
			outputText = ""
		}
		outputText += fmt.Sprintf("Command timed out after %.0f seconds", timeoutSecs)
		return tools.TextResult(outputText), fmt.Errorf("command timed out")

	case execErr != nil:
		outputText += fmt.Sprintf("\n\nCommand failed: %v", execErr)
		return tools.TextResult(outputText), fmt.Errorf("%s", outputText)
	}

	return tools.TextResult(outputText), nil
}

func combineChunks(chunks [][]byte) []byte {
	total := 0
	for _, c := range chunks {
		total += len(c)
	}
	out := make([]byte, 0, total)
	for _, c := range chunks {
		out = append(out, c...)
	}
	return out
}
