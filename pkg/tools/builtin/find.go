package builtin

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

const findDefaultLimit = 1000

// FindTool searches for files matching a glob pattern.
// Pure-Go implementation; respects .gitignore (basic) and skips .git / node_modules.
type FindTool struct {
	cwd string
}

func NewFindTool(cwd string) *FindTool { return &FindTool{cwd: cwd} }

func (t *FindTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name: "find",
		Description: fmt.Sprintf(
			"Search for files by glob pattern. Returns matching file paths relative to the search directory. "+
				"Respects .gitignore. Output is truncated to %d results or %s (whichever is hit first).",
			findDefaultLimit, FormatSize(DefaultMaxBytes),
		),
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"pattern": {Type: "string", Description: "Glob pattern to match files, e.g. '*.ts', '**/*.json', or 'src/**/*.spec.ts'"},
				"path":    {Type: "string", Description: "Directory to search in (default: current directory)"},
				"limit":   {Type: "integer", Description: fmt.Sprintf("Maximum number of results (default: %d)", findDefaultLimit)},
			},
			Required: []string{"pattern"},
		}),
	}
}

func (t *FindTool) Execute(ctx context.Context, _ string, params map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return tools.ErrorResult(fmt.Errorf("pattern is required")), nil
	}

	pathParam, _ := params["path"].(string)
	limit := findDefaultLimit
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

	searchRoot := resolvePath(pathParam, t.cwd)
	if pathParam == "" {
		searchRoot = t.cwd
	}

	info, err := os.Stat(searchRoot)
	if err != nil || !info.IsDir() {
		return tools.ErrorResult(fmt.Errorf("path not found or not a directory: %s", searchRoot)), nil
	}

	gitIgnore := loadGitignore(searchRoot)

	var results []string
	limitReached := false

	walkErr := filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || ctx.Err() != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".hg" || name == ".svn" {
				return filepath.SkipDir
			}
			if gitIgnore.matchesDir(path, searchRoot) {
				return filepath.SkipDir
			}
			return nil
		}

		if gitIgnore.matchesFile(path, searchRoot) {
			return nil
		}

		rel, _ := filepath.Rel(searchRoot, path)
		relSlash := filepath.ToSlash(rel)

		matched, _ := matchGlob(pattern, d.Name(), path, searchRoot)
		if !matched {
			return nil
		}

		results = append(results, relSlash)
		if len(results) >= limit {
			limitReached = true
			return fmt.Errorf("limit_reached")
		}
		return nil
	})
	if walkErr != nil && walkErr.Error() != "limit_reached" {
		return tools.ErrorResult(walkErr), nil
	}

	if len(results) == 0 {
		return tools.TextResult("No files found matching pattern"), nil
	}

	rawOutput := strings.Join(results, "\n")
	tr := TruncateHead(rawOutput, maxInt, DefaultMaxBytes)
	output := tr.Content

	var notices []string
	if limitReached {
		notices = append(notices, fmt.Sprintf("%d results limit reached. Use limit=%d for more, or refine pattern", limit, limit*2))
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
