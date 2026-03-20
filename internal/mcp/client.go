package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// Client speaks the MCP JSON-RPC protocol over stdio or an HTTP connection.
type Client struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	httpClient *http.Client
	endpoint   string
	headers    map[string]string
	mu         sync.Mutex
	nextID     atomic.Int64
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolInfo describes a tool exposed by the MCP server.
type ToolInfo struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// CallResult contains the output from a tool/call request.
type CallResult struct {
	Content []ContentBlock
	IsError bool
}

type ContentBlock struct {
	Type string
	Text string
}

// StartStdio spawns the given command as an MCP server and performs the
// initialize handshake. Returns a ready-to-use Client.
func StartStdio(ctx context.Context, command []string, env []string) (*Client, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("mcp: command is required")
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	if len(env) > 0 {
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start: %w", err)
	}
	c := &Client{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}
	if err := c.initialize(ctx); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("mcp: initialize: %w", err)
	}
	return c, nil
}

// StartRemote connects to an HTTP/SSE MCP endpoint and performs the initialize handshake.
func StartRemote(ctx context.Context, endpoint string, headers map[string]string) (*Client, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("mcp: endpoint is required")
	}
	client := &Client{
		httpClient: &http.Client{},
		endpoint:   endpoint,
		headers:    copyHeaders(headers),
	}
	if err := client.initialize(ctx); err != nil {
		return nil, fmt.Errorf("mcp: initialize: %w", err)
	}
	return client, nil
}

// ListTools calls tools/list and returns the available tools.
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	var result struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := c.call(ctx, "tools/list", nil, &result); err != nil {
		return nil, err
	}
	tools := make([]ToolInfo, 0, len(result.Tools))
	for _, t := range result.Tools {
		tools = append(tools, ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return tools, nil
}

// CallTool calls tools/call with the given name and arguments.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return CallResult{}, err
	}
	blocks := make([]ContentBlock, 0, len(result.Content))
	for _, b := range result.Content {
		blocks = append(blocks, ContentBlock{Type: b.Type, Text: b.Text})
	}
	return CallResult{Content: blocks, IsError: result.IsError}, nil
}

// Close stops the MCP server process.
func (c *Client) Close() error {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "agent",
			"version": "0.1.0",
		},
	}
	var result map[string]any
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return err
	}
	// Send initialized notification.
	return c.notify("notifications/initialized", nil)
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID.Add(1)
	req := request{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	if c.endpoint != "" {
		return c.callRemote(ctx, req, result)
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.stdin, "%s\n", data); err != nil {
		return fmt.Errorf("mcp write: %w", err)
	}
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return fmt.Errorf("mcp read: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var resp response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		if resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(resp.Result, result)
		}
		return nil
	}
}

func (c *Client) callRemote(ctx context.Context, req request, result any) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("mcp http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mcp http do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mcp http error: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	var rpcResp response
	if strings.Contains(contentType, "text/event-stream") {
		rpcResp, err = readSSEResponse(resp.Body, req.ID)
	} else {
		rpcResp, err = readJSONResponse(resp.Body)
	}
	if err != nil {
		return err
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("mcp error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if result != nil {
		return json.Unmarshal(rpcResp.Result, result)
	}
	return nil
}

func (c *Client) notify(method string, params any) error {
	n := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		n["params"] = params
	}
	data, err := json.Marshal(n)
	if err != nil {
		return err
	}
	if c.endpoint != "" {
		httpReq, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(data))
		if err != nil {
			return err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json, text/event-stream")
		for k, v := range c.headers {
			httpReq.Header.Set(k, v)
		}
		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("mcp http notify error: %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}
		return nil
	}
	_, err = fmt.Fprintf(c.stdin, "%s\n", data)
	return err
}

func readJSONResponse(body io.Reader) (response, error) {
	var resp response
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return response{}, fmt.Errorf("mcp decode response: %w", err)
	}
	return resp, nil
}

func readSSEResponse(body io.Reader, id int64) (response, error) {
	scanner := bufio.NewScanner(body)
	var dataLines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if len(dataLines) == 0 {
				continue
			}
			payload := strings.Join(dataLines, "\n")
			dataLines = nil
			var resp response
			if err := json.Unmarshal([]byte(payload), &resp); err != nil {
				continue
			}
			if resp.ID == id || resp.ID == 0 {
				return resp, nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return response{}, fmt.Errorf("mcp read sse: %w", err)
	}
	return response{}, io.EOF
}

func copyHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		out[k] = v
	}
	return out
}
