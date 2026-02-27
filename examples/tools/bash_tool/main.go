// bash_tool is an example external tool plugin that executes shell commands.
//
// It implements the agent plugin protocol:
//   - Reads JSON from stdin, one object per line.
//   - Writes JSON to stdout, one object per line.
//
// Protocol messages:
//   in:  {"type":"describe"}
//   out: {"name":"bash","description":"...","parameters":{...}}
//
//   in:  {"type":"call","call_id":"...","params":{"command":"ls -la"}}
//   out: {"content":[{"type":"text","text":"..."}],"error":false}
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

var definition = map[string]any{
	"name":        "bash",
	"description": "Execute a bash command and return its stdout/stderr. Use for file operations, running scripts, checking system state, etc.",
	"parameters": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The bash command to execute.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Max seconds to wait (default 30).",
			},
		},
		"required": []string{"command"},
	},
}

type inMsg struct {
	Type   string         `json:"type"`
	CallID string         `json:"call_id"`
	Params map[string]any `json:"params"`
}

type outMsg struct {
	Content []map[string]string `json:"content"`
	Error   bool                `json:"error"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var msg inMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			writeError(enc, "invalid JSON: "+err.Error())
			continue
		}

		switch msg.Type {
		case "describe":
			enc.Encode(definition)

		case "call":
			command, _ := msg.Params["command"].(string)
			if command == "" {
				writeError(enc, "missing required param: command")
				continue
			}

			timeout := 30 * time.Second
			if t, ok := msg.Params["timeout_seconds"].(float64); ok && t > 0 {
				timeout = time.Duration(t) * time.Second
			}

			output, isErr := runCommand(command, timeout)
			out := outMsg{
				Content: []map[string]string{{"type": "text", "text": output}},
				Error:   isErr,
			}
			enc.Encode(out)

		default:
			writeError(enc, "unknown message type: "+msg.Type)
		}
	}
}

func runCommand(command string, timeout time.Duration) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("command timed out after %v\n%s", timeout, output), true
	}
	if err != nil {
		return fmt.Sprintf("exit error: %v\n%s", err, output), true
	}
	if output == "" {
		output = "(no output)"
	}
	return output, false
}

func writeError(enc *json.Encoder, msg string) {
	enc.Encode(outMsg{
		Content: []map[string]string{{"type": "text", "text": msg}},
		Error:   true,
	})
}
