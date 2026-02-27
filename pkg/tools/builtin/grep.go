package builtin

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

const grepDefaultLimit = 100

// GrepTool searches file contents using Go's regexp engine.
// Falls back to pure-Go walk when ripgrep is not available.
type GrepTool struct {
	cwd string
}

func NewGrepTool(cwd string) *GrepTool { return &GrepTool{cwd: cwd} }

func (t *GrepTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name: "grep",
		Description: fmt.Sprintf(
			"Search file contents for a pattern. Returns matching lines with file paths and line numbers. "+
				"Respects .gitignore. Output is truncated to %d matches or %s (whichever is hit first). "+
				"Long lines are truncated to %d chars.",
			grepDefaultLimit, FormatSize(DefaultMaxBytes), GrepMaxLineLength,
		),
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"pattern":    {Type: "string", Description: "Search pattern (regex or literal string)"},
				"path":       {Type: "string", Description: "Directory or file to search (default: current directory)"},
				"glob":       {Type: "string", Description: "Filter files by glob pattern, e.g. '*.ts' or '**/*.spec.ts'"},
				"ignoreCase": {Type: "boolean", Description: "Case-insensitive search (default: false)"},
				"literal":    {Type: "boolean", Description: "Treat pattern as literal string instead of regex (default: false)"},
				"context":    {Type: "integer", Description: "Number of lines to show before and after each match (default: 0)"},
				"limit":      {Type: "integer", Description: fmt.Sprintf("Maximum number of matches to return (default: %d)", grepDefaultLimit)},
			},
			Required: []string{"pattern"},
		}),
	}
}

type grepMatch struct {
	file    string // relative path
	lineNum int    // 1-indexed
	line    string // raw line content
}

func (t *GrepTool) Execute(ctx context.Context, _ string, params map[string]any, onUpdate tools.UpdateFn) (tools.Result, error) {
	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		return tools.ErrorResult(fmt.Errorf("pattern is required")), nil
	}

	pathParam, _ := params["path"].(string)
	globParam, _ := params["glob"].(string)
	ignoreCase, _ := params["ignoreCase"].(bool)
	literal, _ := params["literal"].(bool)
	ctxLines := 0
	if v, ok := params["context"]; ok {
		switch n := v.(type) {
		case float64:
			ctxLines = int(n)
		case int:
			ctxLines = n
		}
	}
	limit := grepDefaultLimit
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

	// Compile regex
	patStr := pattern
	if literal {
		patStr = regexp.QuoteMeta(pattern)
	}
	if ignoreCase {
		patStr = "(?i)" + patStr
	}
	re, err := regexp.Compile(patStr)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("invalid pattern: %w", err)), nil
	}

	// Load gitignore rules from the search root
	gitIgnore := loadGitignore(searchRoot)

	// Check if searchRoot is a file or directory
	info, err := os.Stat(searchRoot)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("path not found: %s", searchRoot)), nil
	}

	var matches []grepMatch
	linesTruncated := false
	matchLimitReached := false

	filesSearched := 0

	if !info.IsDir() {
		// Single file search
		rel, _ := filepath.Rel(t.cwd, searchRoot)
		ms, lt, err := searchFile(ctx, searchRoot, rel, re, limit)
		if err != nil {
			return tools.ErrorResult(err), nil
		}
		matches = ms
		linesTruncated = lt
		if len(matches) >= limit {
			matchLimitReached = true
		}
	} else {
		// Walk directory
		err = filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, walkErr error) error {
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

			// Glob filter
			if globParam != "" {
				matched, _ := matchGlob(globParam, d.Name(), path, searchRoot)
				if !matched {
					return nil
				}
			}

			if !isTextFile(d.Name()) {
				return nil
			}

			if gitIgnore.matchesFile(path, searchRoot) {
				return nil
			}

			rel, _ := filepath.Rel(searchRoot, path)
			remaining := limit - len(matches)
			if remaining <= 0 {
				matchLimitReached = true
				return fmt.Errorf("limit_reached") // sentinel
			}

			ms, lt, err := searchFile(ctx, path, filepath.ToSlash(rel), re, remaining)
			if err != nil {
				return nil // skip unreadable files
			}
			matches = append(matches, ms...)
			if lt {
				linesTruncated = true
			}
			filesSearched++
			// Emit progress every 100 files.
			if onUpdate != nil && filesSearched%100 == 0 {
				onUpdate(tools.Result{
					Content: []ai.ContentBlock{ai.TextContent{
						Type: "text",
						Text: fmt.Sprintf("Searching… %d files scanned, %d matches so far", filesSearched, len(matches)),
					}},
				})
			}
			if len(matches) >= limit {
				matchLimitReached = true
				return fmt.Errorf("limit_reached")
			}
			return nil
		})
		if err != nil && err.Error() != "limit_reached" {
			return tools.ErrorResult(err), nil
		}
	}

	if len(matches) == 0 {
		return tools.TextResult("No matches found"), nil
	}

	// Format output with optional context lines
	outputLines := formatMatches(matches, ctxLines, searchRoot)
	rawOutput := strings.Join(outputLines, "\n")

	tr := TruncateHead(rawOutput, maxInt, DefaultMaxBytes)
	output := tr.Content

	// Notices
	var notices []string
	if matchLimitReached {
		notices = append(notices, fmt.Sprintf("%d matches limit reached. Use limit=%d for more, or refine pattern", limit, limit*2))
	}
	if tr.Truncated {
		notices = append(notices, fmt.Sprintf("%s limit reached", FormatSize(DefaultMaxBytes)))
	}
	if linesTruncated {
		notices = append(notices, fmt.Sprintf("Some lines truncated to %d chars. Use read tool to see full lines", GrepMaxLineLength))
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
	}

	return tools.Result{
		Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: output}},
	}, nil
}

// searchFile searches a single file for the pattern.
func searchFile(ctx context.Context, absPath, relPath string, re *regexp.Regexp, limit int) ([]grepMatch, bool, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	var matches []grepMatch
	linesTruncated := false
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	lineNum := 0

	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			truncated, wasTruncated := TruncateLine(line, GrepMaxLineLength)
			if wasTruncated {
				linesTruncated = true
			}
			matches = append(matches, grepMatch{file: relPath, lineNum: lineNum, line: truncated})
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches, linesTruncated, scanner.Err()
}

// formatMatches renders matches as "file:line: content" lines, with optional
// context (leading "-" separator for non-match context lines).
func formatMatches(matches []grepMatch, ctxLines int, searchRoot string) []string {
	if ctxLines <= 0 {
		out := make([]string, 0, len(matches))
		for _, m := range matches {
			out = append(out, fmt.Sprintf("%s:%d: %s", m.file, m.lineNum, m.line))
		}
		return out
	}

	// For context lines we need to re-read the files (cache per file)
	fileLines := map[string][]string{}
	getLines := func(absPath string) []string {
		if l, ok := fileLines[absPath]; ok {
			return l
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			fileLines[absPath] = nil
			return nil
		}
		lines := strings.Split(normalizeToLF(string(data)), "\n")
		fileLines[absPath] = lines
		return lines
	}

	var out []string
	for _, m := range matches {
		absPath := filepath.Join(searchRoot, filepath.FromSlash(m.file))
		lines := getLines(absPath)
		start := max(0, m.lineNum-1-ctxLines)
		end := min(len(lines), m.lineNum+ctxLines)
		for i := start; i < end; i++ {
			lineText, _ := TruncateLine(lines[i], GrepMaxLineLength)
			if i+1 == m.lineNum {
				out = append(out, fmt.Sprintf("%s:%d: %s", m.file, i+1, lineText))
			} else {
				out = append(out, fmt.Sprintf("%s-%d- %s", m.file, i+1, lineText))
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Gitignore (basic)
// ---------------------------------------------------------------------------

type gitIgnoreRules struct {
	patterns []string
}

func loadGitignore(root string) gitIgnoreRules {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return gitIgnoreRules{}
	}
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		patterns = append(patterns, line)
	}
	return gitIgnoreRules{patterns: patterns}
}

func (g gitIgnoreRules) matchesDir(absPath, root string) bool {
	rel, _ := filepath.Rel(root, absPath)
	name := filepath.Base(absPath)
	for _, p := range g.patterns {
		clean := strings.TrimSuffix(p, "/")
		if matched, _ := filepath.Match(clean, name); matched {
			return true
		}
		if matched, _ := filepath.Match(clean, filepath.ToSlash(rel)); matched {
			return true
		}
	}
	return false
}

func (g gitIgnoreRules) matchesFile(absPath, root string) bool {
	rel, _ := filepath.Rel(root, absPath)
	name := filepath.Base(absPath)
	for _, p := range g.patterns {
		if strings.HasSuffix(p, "/") {
			continue // directory-only rule
		}
		if matched, _ := filepath.Match(p, name); matched {
			return true
		}
		if matched, _ := filepath.Match(p, filepath.ToSlash(rel)); matched {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Glob and text-file helpers
// ---------------------------------------------------------------------------

// matchGlob matches a file against a glob pattern, supporting ** via path walk.
func matchGlob(pattern, name, absPath, root string) (bool, error) {
	// Simple patterns (no **) — match against filename
	if !strings.Contains(pattern, "**") {
		return filepath.Match(pattern, name)
	}
	// ** patterns — match against full relative path
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return false, err
	}
	return doubleStarMatch(pattern, filepath.ToSlash(rel))
}

// doubleStarMatch implements basic ** glob matching.
func doubleStarMatch(pattern, path string) (bool, error) {
	// Normalise ** to a sentinel, then do segment-level matching
	parts := strings.Split(pattern, "**")
	if len(parts) == 1 {
		return filepath.Match(pattern, path)
	}
	// Simple: check prefix matches parts[0] and suffix matches parts[len-1]
	// This handles the most common cases: **/*.ts, src/**/*.go
	prefix := parts[0]
	suffix := parts[len(parts)-1]

	if prefix != "" {
		if !strings.HasPrefix(path, prefix) {
			return false, nil
		}
		path = path[len(prefix):]
	}
	if suffix != "" {
		if !strings.HasSuffix(path, suffix) {
			m, _ := filepath.Match(suffix, filepath.Base(path))
			return m, nil
		}
	}
	return true, nil
}

// isTextFile returns false for well-known binary file extensions.
func isTextFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	binary := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
		".ico": true, ".svg": true, ".pdf": true, ".zip": true, ".tar": true,
		".gz": true, ".bz2": true, ".xz": true, ".7z": true, ".rar": true,
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".wasm": true, ".bin": true, ".db": true, ".sqlite": true,
		".mp3": true, ".mp4": true, ".mov": true, ".avi": true,
	}
	return !binary[ext]
}

const maxInt = int(^uint(0) >> 1)
