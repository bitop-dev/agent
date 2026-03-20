package core

import (
	"context"
	"os/exec"

	"github.com/bitop-dev/agent/pkg/tool"
)

type BashTool struct{}

func (BashTool) Definition() tool.Definition {
	return tool.Definition{ID: "core/bash", Description: "Run a shell command subject to policy and approval"}
}

func (BashTool) Run(ctx context.Context, call tool.Call) (tool.Result, error) {
	command, err := argString(call.Arguments, "command")
	if err != nil {
		return tool.Result{}, err
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	output, err := cmd.CombinedOutput()
	return tool.Result{ToolID: call.ToolID, Output: string(output)}, err
}
