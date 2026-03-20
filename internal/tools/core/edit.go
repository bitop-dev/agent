package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bitop-dev/agent/pkg/tool"
)

type EditTool struct{}

func (EditTool) Definition() tool.Definition {
	return tool.Definition{ID: "core/edit", Description: "Edit a file inside the local workspace"}
}

func (EditTool) Run(_ context.Context, call tool.Call) (tool.Result, error) {
	path, err := argString(call.Arguments, "path")
	if err != nil {
		return tool.Result{}, err
	}
	oldText, err := argString(call.Arguments, "old")
	if err != nil {
		return tool.Result{}, err
	}
	newText, err := argString(call.Arguments, "new")
	if err != nil {
		return tool.Result{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tool.Result{}, err
	}
	content := string(data)
	if !strings.Contains(content, oldText) {
		return tool.Result{}, fmt.Errorf("old text not found in %s", path)
	}
	updated := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return tool.Result{}, err
	}
	return tool.Result{ToolID: call.ToolID, Output: "edited file", Data: map[string]any{"path": path}}, nil
}
