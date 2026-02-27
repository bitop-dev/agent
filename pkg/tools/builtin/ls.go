package builtin

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/tools"
)

const lsDefaultLimit = 500

// LsTool lists directory contents — sorted alphabetically, with "/" suffix for
// subdirectories, including dotfiles.
type LsTool struct {
	cwd string
}

func NewLsTool(cwd string) *LsTool { return &LsTool{cwd: cwd} }

func (t *LsTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name: "ls",
		Description: fmt.Sprintf(
			"List directory contents. Returns entries sorted alphabetically, with '/' suffix for directories. "+
				"Includes dotfiles. Output is truncated to %d entries or %s (whichever is hit first).",
			lsDefaultLimit, FormatSize(DefaultMaxBytes),
		),
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"path":  {Type: "string", Description: "Directory to list (default: current directory)"},
				"limit": {Type: "integer", Description: fmt.Sprintf("Maximum number of entries to return (default: %d)", lsDefaultLimit)},
			},
		}),
	}
}

func (t *LsTool) Execute(_ context.Context, _ string, params map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	pathParam, _ := params["path"].(string)
	limit := lsDefaultLimit
	if v, ok := params["limit"]; ok {
		switch n := v.(type) {
		case float64:
			if int(n) > 0 {
				limit = int(n)
			}
		case int:
			if n > 0 {
				limit = n
			}
		}
	}

	dirPath := resolvePath(pathParam, t.cwd)
	if pathParam == "" {
		dirPath = t.cwd
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("path not found: %s", pathParam)), nil
	}
	if !info.IsDir() {
		return tools.ErrorResult(fmt.Errorf("not a directory: %s", pathParam)), nil
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("cannot read directory: %w", err)), nil
	}

	// Sort case-insensitively (mirrors JS localeCompare behaviour)
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	var results []string
	limitReached := false

	for _, e := range entries {
		if len(results) >= limit {
			limitReached = true
			break
		}
		name := e.Name()
		if e.IsDir() {
			name += "/"
		} else if e.Type()&os.ModeSymlink != 0 {
			// Resolve symlink to check if it points to a directory
			if target, err := os.Stat(dirPath + "/" + name); err == nil && target.IsDir() {
				name += "/"
			}
		}
		results = append(results, name)
	}

	if len(results) == 0 {
		return tools.TextResult("(empty directory)"), nil
	}

	rawOutput := strings.Join(results, "\n")
	tr := TruncateHead(rawOutput, maxInt, DefaultMaxBytes)
	output := tr.Content

	var notices []string
	if limitReached {
		notices = append(notices, fmt.Sprintf("%d entries limit reached. Use limit=%d for more", limit, limit*2))
	}
	if tr.Truncated {
		notices = append(notices, fmt.Sprintf("%s limit reached", FormatSize(DefaultMaxBytes)))
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
	}

	return tools.Result{
		Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: output}},
	}, nil
}
