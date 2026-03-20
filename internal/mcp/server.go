package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Server implements an MCP tool server over stdio. It exposes agent profiles
// as callable MCP tools so external clients (Claude Desktop, Cursor, etc.)
// can invoke agents.
type Server struct {
	name    string
	version string
	tools   []ServerTool
	writer  io.Writer
}

// ServerTool describes a tool the MCP server exposes.
type ServerTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]any         `json:"inputSchema"`
	Handler     func(ctx context.Context, arguments map[string]any) (string, error)
}

// NewServer creates an MCP server that will respond to JSON-RPC requests on stdio.
func NewServer(name, version string, tools []ServerTool) *Server {
	return &Server{name: name, version: version, tools: tools}
}

// Serve reads JSON-RPC requests from reader and writes responses to writer.
// Blocks until the reader is closed or context is cancelled.
func (s *Server) Serve(ctx context.Context, reader io.Reader, writer io.Writer) error {
	s.writer = writer
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      *json.Number   `json:"id,omitempty"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		// Notifications (no ID) — just acknowledge silently.
		if req.ID == nil {
			continue
		}

		id, _ := req.ID.Int64()

		switch req.Method {
		case "initialize":
			s.respond(id, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{"listChanged": false},
				},
				"serverInfo": map[string]any{
					"name":    s.name,
					"version": s.version,
				},
			})

		case "tools/list":
			toolDefs := make([]map[string]any, 0, len(s.tools))
			for _, t := range s.tools {
				toolDefs = append(toolDefs, map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"inputSchema": t.InputSchema,
				})
			}
			s.respond(id, map[string]any{"tools": toolDefs})

		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				s.respondError(id, -32602, "invalid params")
				continue
			}
			var handler func(context.Context, map[string]any) (string, error)
			for _, t := range s.tools {
				if t.Name == params.Name {
					handler = t.Handler
					break
				}
			}
			if handler == nil {
				s.respondError(id, -32601, fmt.Sprintf("tool %q not found", params.Name))
				continue
			}
			output, err := handler(ctx, params.Arguments)
			if err != nil {
				s.respond(id, map[string]any{
					"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("error: %v", err)}},
					"isError": true,
				})
				continue
			}
			s.respond(id, map[string]any{
				"content": []map[string]any{{"type": "text", "text": output}},
				"isError": false,
			})

		default:
			s.respondError(id, -32601, fmt.Sprintf("method %q not supported", req.Method))
		}
	}
	return scanner.Err()
}

func (s *Server) respond(id int64, result any) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(s.writer, "%s\n", data)
}

func (s *Server) respondError(id int64, code int, message string) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(s.writer, "%s\n", data)
}
