package core

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ncecere/agent/pkg/tool"
)

type GlobTool struct{}

func (GlobTool) Definition() tool.Definition {
	return tool.Definition{
		ID:          "core/glob",
		Description: "Find files matching a glob pattern within the workspace",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
				"root":    map[string]any{"type": "string"},
			},
			"required": []string{"pattern"},
		},
	}
}

func (GlobTool) Run(_ context.Context, call tool.Call) (tool.Result, error) {
	pattern, err := argString(call.Arguments, "pattern")
	if err != nil {
		return tool.Result{}, err
	}
	root := "."
	if r, ok := call.Arguments["root"].(string); ok && r != "" {
		root = r
	}
	var matches []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil {
			return fmt.Errorf("invalid glob pattern: %w", err)
		}
		if !matched {
			parts := strings.Split(rel, string(filepath.Separator))
			for _, part := range parts {
				if m, _ := filepath.Match(pattern, part); m {
					matched = true
					break
				}
			}
		}
		if matched {
			matches = append(matches, rel)
		}
		return nil
	})
	if err != nil {
		return tool.Result{}, err
	}
	sort.Strings(matches)
	output := strings.Join(matches, "\n")
	return tool.Result{
		ToolID: call.ToolID,
		Output: output,
		Data:   map[string]any{"matches": matches, "count": len(matches)},
	}, nil
}
