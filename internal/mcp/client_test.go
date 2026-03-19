package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// echoServer is a minimal MCP server for testing that handles initialize and tools/list.
func startEchoServer(t *testing.T) *Client {
	t.Helper()
	// We test the client directly by creating a fake server using pipes.
	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()

	// Fake server goroutine.
	go func() {
		scanner := bufio.NewScanner(serverIn)
		encoder := json.NewEncoder(serverOut)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var req map[string]any
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				continue
			}
			id, _ := req["id"].(float64)
			method, _ := req["method"].(string)
			if method == "notifications/initialized" {
				continue
			}
			var result any
			switch method {
			case "initialize":
				result = map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"serverInfo":      map[string]any{"name": "test-server", "version": "0.1.0"},
				}
			case "tools/list":
				result = map[string]any{
					"tools": []any{
						map[string]any{
							"name":        "echo",
							"description": "Echoes the input",
							"inputSchema": map[string]any{
								"type":       "object",
								"properties": map[string]any{"message": map[string]any{"type": "string"}},
							},
						},
					},
				}
			case "tools/call":
				params, _ := req["params"].(map[string]any)
				args, _ := params["arguments"].(map[string]any)
				msg, _ := args["message"].(string)
				result = map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": fmt.Sprintf("echo: %s", msg)},
					},
					"isError": false,
				}
			}
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
		}
	}()

	// Build a client wired to our fake server.
	c := &Client{
		cmd:    &exec.Cmd{},
		stdin:  clientOut,
		stdout: bufio.NewReader(clientIn),
	}
	if err := c.initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return c
}

func TestMCPClientListTools(t *testing.T) {
	c := startEchoServer(t)
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Fatalf("unexpected tool name: %s", tools[0].Name)
	}
}

func TestMCPClientCallTool(t *testing.T) {
	c := startEchoServer(t)
	result, err := c.CallTool(context.Background(), "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	if result.Content[0].Text != "echo: hello" {
		t.Fatalf("unexpected text: %s", result.Content[0].Text)
	}
}
