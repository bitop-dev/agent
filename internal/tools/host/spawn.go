package host

import (
	"context"
	"fmt"

	pkghost "github.com/bitop-dev/agent/pkg/host"
	"github.com/bitop-dev/agent/pkg/tool"
)

// SpawnTool implements agent/spawn using the host capabilities API.
// It never touches core runtime internals directly.
type SpawnTool struct {
	Caps            pkghost.Capabilities
	MaxDepth        int
	MaxAgents       int
	AllowedProfiles []string
}

func (t SpawnTool) Definition() tool.Definition {
	return tool.Definition{
		ID:          "github.com/bitop-dev/agent/spawn",
		Description: "Spawn a bounded sub-agent task using an allowed profile and tool set",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task":     map[string]any{"type": "string"},
				"profile":  map[string]any{"type": "string"},
				"maxTurns": map[string]any{"type": "integer"},
			},
			"required": []string{"task"},
		},
	}
}

func (t SpawnTool) Operation() string {
	return "spawn-sub-agent"
}

func (t SpawnTool) Run(ctx context.Context, call tool.Call) (tool.Result, error) {
	task, ok := call.Arguments["task"].(string)
	if !ok || task == "" {
		return tool.Result{}, fmt.Errorf("github.com/bitop-dev/agent/spawn: task is required")
	}
	profile := ""
	if p, ok := call.Arguments["profile"].(string); ok {
		profile = p
	}
	maxTurns := 4
	if mt, ok := call.Arguments["maxTurns"].(int); ok && mt > 0 {
		maxTurns = mt
	}
	if t.MaxDepth > 0 && maxTurns > t.MaxDepth*4 {
		maxTurns = t.MaxDepth * 4
	}
	if profile != "" && !t.isAllowedProfile(profile) {
		return tool.Result{}, fmt.Errorf("github.com/bitop-dev/agent/spawn: profile %q is not in the allowed profiles list", profile)
	}
	result, err := t.Caps.SpawnSubRun(ctx, pkghost.SubRunRequest{
		Task:     task,
		Profile:  profile,
		MaxTurns: maxTurns,
	})
	if err != nil {
		return tool.Result{}, fmt.Errorf("github.com/bitop-dev/agent/spawn: %w", err)
	}
	return tool.Result{
		ToolID: call.ToolID,
		Output: result.Output,
		Data: map[string]any{
			"sessionId": result.SessionID,
			"turns":     result.Turns,
		},
	}, nil
}

func (t SpawnTool) isAllowedProfile(profile string) bool {
	if len(t.AllowedProfiles) == 0 {
		return true
	}
	for _, allowed := range t.AllowedProfiles {
		if allowed == profile {
			return true
		}
	}
	return false
}
