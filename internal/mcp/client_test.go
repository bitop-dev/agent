package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestMCPClientRemoteHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Test-Key"); got != "secret" {
			http.Error(w, "missing header", http.StatusUnauthorized)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method, _ := req["method"].(string)
		id, _ := req["id"].(float64)
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
			return
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "test", "version": "0.1.0"}}})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"tools": []any{map[string]any{"name": "lookup", "description": "Lookup docs", "inputSchema": map[string]any{"type": "object"}}}}})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "remote ok"}}, "isError": false}})
		default:
			http.Error(w, "unknown method", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	c, err := StartRemote(context.Background(), server.URL, map[string]string{"X-Test-Key": "secret"})
	if err != nil {
		t.Fatalf("start remote: %v", err)
	}
	defer c.Close()

	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "lookup" {
		t.Fatalf("unexpected tools: %#v", tools)
	}

	result, err := c.CallTool(context.Background(), "lookup", map[string]any{"query": "docs"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "remote ok" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestMCPClientRemoteSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method, _ := req["method"].(string)
		if method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		id, _ := req["id"].(float64)
		w.Header().Set("Content-Type", "text/event-stream")
		var payload map[string]any
		switch method {
		case "initialize":
			payload = map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "serverInfo": map[string]any{"name": "test", "version": "0.1.0"}}}
		case "tools/list":
			payload = map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"tools": []any{map[string]any{"name": "search_docs", "description": "Search docs", "inputSchema": map[string]any{"type": "object"}}}}}
		default:
			payload = map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "sse ok"}}, "isError": false}}
		}
		data, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
	}))
	defer server.Close()

	c, err := StartRemote(context.Background(), server.URL, nil)
	if err != nil {
		t.Fatalf("start remote: %v", err)
	}
	defer c.Close()

	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "search_docs" {
		t.Fatalf("unexpected tools: %#v", tools)
	}

	result, err := c.CallTool(context.Background(), "search_docs", map[string]any{"query": "mcp"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "sse ok" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
