package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// WriteTool writes (or overwrites) a file, auto-creating parent directories.
type WriteTool struct {
	cwd string
}

func NewWriteTool(cwd string) *WriteTool { return &WriteTool{cwd: cwd} }

func (t *WriteTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        "write",
		Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"path":    {Type: "string", Description: "Path to the file to write (relative or absolute)"},
				"content": {Type: "string", Description: "Content to write to the file"},
			},
			Required: []string{"path", "content"},
		}),
	}
}

func (t *WriteTool) Execute(_ context.Context, _ string, params map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	pathParam, _ := params["path"].(string)
	content, _ := params["content"].(string)
	if pathParam == "" {
		return tools.ErrorResult(fmt.Errorf("path is required")), nil
	}

	absPath := resolvePath(pathParam, t.cwd)

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return tools.ErrorResult(fmt.Errorf("cannot create directories for %s: %w", pathParam, err)), nil
	}

	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return tools.ErrorResult(fmt.Errorf("cannot write %s: %w", pathParam, err)), nil
	}

	return tools.Result{
		Content: []ai.ContentBlock{
			ai.TextContent{
				Type: "text",
				Text: fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), pathParam),
			},
		},
	}, nil
}
