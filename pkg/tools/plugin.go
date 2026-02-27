package tools

// Plugin protocol — external tool processes
//
// An external plugin is a standalone executable that speaks a simple
// JSON-over-stdin/stdout protocol:
//
//  1. On startup the agent sends a single JSON line:
//       {"type":"describe"}
//     The plugin responds with its definition:
//       {"name":"...","description":"...","parameters":{...}}
//
//  2. For each tool call the agent sends:
//       {"type":"call","call_id":"...","params":{...}}
//     The plugin responds:
//       {"content":[{"type":"text","text":"..."}],"error":false}
//     or on error:
//       {"content":[{"type":"text","text":"error message"}],"error":true}
//
// Plugins are launched once and kept alive for the session.  They must
// handle concurrent calls if needed (currently the agent serialises calls
// to a single plugin process).

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"

	"github.com/nickcecere/agent/pkg/ai"
)

// pluginTool wraps a subprocess plugin as a Tool.
type pluginTool struct {
	def ai.ToolDefinition
	mu  sync.Mutex
	cmd *exec.Cmd
	enc *json.Encoder
	dec *json.Decoder
}

// pluginDescribeResponse is the response to the "describe" request.
type pluginDescribeResponse struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// pluginCallRequest is what we send for a tool call.
type pluginCallRequest struct {
	Type   string         `json:"type"`
	CallID string         `json:"call_id"`
	Params map[string]any `json:"params"`
}

// pluginCallResponse is what the plugin sends back.
type pluginCallResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error bool `json:"error"`
}

// LoadPlugin launches the executable at path, queries its definition, and
// returns a Tool that delegates calls to the subprocess.
func LoadPlugin(path string, args ...string) (Tool, error) {
	cmd := exec.Command(path, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin %s: stdin pipe: %w", path, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin %s: stdout pipe: %w", path, err)
	}
	cmd.Stderr = nil // could pipe to logger

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("plugin %s: start: %w", path, err)
	}

	enc := json.NewEncoder(stdin)
	dec := json.NewDecoder(bufio.NewReader(stdout))

	// Send describe request
	if err := enc.Encode(map[string]string{"type": "describe"}); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("plugin %s: describe request: %w", path, err)
	}

	var desc pluginDescribeResponse
	if err := dec.Decode(&desc); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("plugin %s: describe response: %w", path, err)
	}

	pt := &pluginTool{
		def: ai.ToolDefinition{
			Name:        desc.Name,
			Description: desc.Description,
			Parameters:  desc.Parameters,
		},
		cmd: cmd,
		enc: enc,
		dec: dec,
	}

	return pt, nil
}

func (pt *pluginTool) Definition() ai.ToolDefinition { return pt.def }

func (pt *pluginTool) Execute(ctx context.Context, callID string, params map[string]any, onUpdate UpdateFn) (Result, error) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	req := pluginCallRequest{
		Type:   "call",
		CallID: callID,
		Params: params,
	}
	if err := pt.enc.Encode(req); err != nil {
		return ErrorResult(err), fmt.Errorf("plugin %s: send call: %w", pt.def.Name, err)
	}

	var resp pluginCallResponse
	if err := pt.dec.Decode(&resp); err != nil {
		return ErrorResult(err), fmt.Errorf("plugin %s: read response: %w", pt.def.Name, err)
	}

	var content []ai.ContentBlock
	for _, c := range resp.Content {
		switch c.Type {
		case "text":
			content = append(content, ai.TextContent{Type: "text", Text: c.Text})
		}
	}

	if resp.Error {
		var errMsg string
		for _, c := range content {
			if tc, ok := c.(ai.TextContent); ok {
				errMsg += tc.Text
			}
		}
		return Result{Content: content}, fmt.Errorf("plugin tool error: %s", errMsg)
	}

	return Result{Content: content}, nil
}

// Close terminates the plugin subprocess. Call this on agent shutdown.
func ClosePlugin(t Tool) {
	if pt, ok := t.(*pluginTool); ok {
		pt.mu.Lock()
		defer pt.mu.Unlock()
		_ = pt.cmd.Process.Kill()
		_ = pt.cmd.Wait()
	}
}
