package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/bitop-dev/agent/pkg/tool"
)

// Tool wraps an MCP tool so it satisfies the framework's tool.Tool interface.
type Tool struct {
	info   ToolInfo
	client *Client
}

func NewTool(info ToolInfo, client *Client) *Tool {
	return &Tool{info: info, client: client}
}

func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		ID:          t.info.Name,
		Description: t.info.Description,
		Schema:      t.info.InputSchema,
	}
}

func (t *Tool) Run(ctx context.Context, call tool.Call) (tool.Result, error) {
	result, err := t.client.CallTool(ctx, t.info.Name, call.Arguments)
	if err != nil {
		return tool.Result{}, err
	}
	if result.IsError {
		parts := make([]string, 0, len(result.Content))
		for _, block := range result.Content {
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return tool.Result{}, fmt.Errorf("mcp tool %s error: %s", t.info.Name, strings.Join(parts, "; "))
	}
	parts := make([]string, 0, len(result.Content))
	data := make(map[string]any)
	for i, block := range result.Content {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
		data[fmt.Sprintf("content_%d_type", i)] = block.Type
	}
	return tool.Result{
		ToolID: call.ToolID,
		Output: strings.Join(parts, "\n"),
		Data:   data,
	}, nil
}
