package core

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ncecere/agent/pkg/tool"
)

type WriteTool struct{}

func (WriteTool) Definition() tool.Definition {
	return tool.Definition{ID: "core/write", Description: "Write a file inside the local workspace"}
}

func (WriteTool) Run(_ context.Context, call tool.Call) (tool.Result, error) {
	path, err := argString(call.Arguments, "path")
	if err != nil {
		return tool.Result{}, err
	}
	content, err := argString(call.Arguments, "content")
	if err != nil {
		return tool.Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return tool.Result{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return tool.Result{}, err
	}
	return tool.Result{ToolID: call.ToolID, Output: "wrote file", Data: map[string]any{"path": path}}, nil
}
