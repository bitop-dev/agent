package builtin

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// EditTool performs surgical find-and-replace on files.
// It normalises CRLF/smart-quotes before matching (fuzzy match), enforces
// that the search text appears exactly once, and returns a contextual diff.
type EditTool struct {
	cwd string
}

func NewEditTool(cwd string) *EditTool { return &EditTool{cwd: cwd} }

func (t *EditTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        "edit",
		Description: "Edit a file by replacing exact text. The oldText must match exactly (including whitespace). Use this for precise, surgical edits.",
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"path":    {Type: "string", Description: "Path to the file to edit (relative or absolute)"},
				"oldText": {Type: "string", Description: "Exact text to find and replace (must match exactly)"},
				"newText": {Type: "string", Description: "New text to replace the old text with"},
			},
			Required: []string{"path", "oldText", "newText"},
		}),
	}
}

// EditDetails is included in the tool result for UI / logging.
type EditDetails struct {
	Diff           string `json:"diff"`
	FirstChangedLine int  `json:"first_changed_line,omitempty"`
}

func (t *EditTool) Execute(_ context.Context, _ string, params map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	pathParam, _ := params["path"].(string)
	oldText, _ := params["oldText"].(string)
	newText, _ := params["newText"].(string)
	if pathParam == "" {
		return tools.ErrorResult(fmt.Errorf("path is required")), nil
	}

	absPath := resolvePath(pathParam, t.cwd)

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("cannot read %s: %w", pathParam, err)), nil
	}

	// Strip BOM, detect + normalise line endings
	bom, rawText := stripBOM(string(raw))
	originalEnding := detectLineEnding(rawText)
	content := normalizeToLF(rawText)
	normOld := normalizeToLF(oldText)
	normNew := normalizeToLF(newText)

	// Fuzzy find
	match := fuzzyFindText(content, normOld)
	if !match.found {
		return tools.ErrorResult(fmt.Errorf(
			"could not find the exact text in %s. The oldText must match exactly including all whitespace and newlines.",
			pathParam,
		)), nil
	}

	// Uniqueness check
	fuzzyContent := normalizeForFuzzyMatch(match.contentForReplacement)
	fuzzyOld := normalizeForFuzzyMatch(normOld)
	occurrences := strings.Count(fuzzyContent, fuzzyOld)
	if occurrences > 1 {
		return tools.ErrorResult(fmt.Errorf(
			"found %d occurrences of the text in %s. The text must be unique. Please provide more context to make it unique.",
			occurrences, pathParam,
		)), nil
	}

	// Apply replacement in the (possibly fuzzy-normalised) base
	base := match.contentForReplacement
	newContent := base[:match.index] + normNew + base[match.index+match.matchLen:]

	if base == newContent {
		return tools.ErrorResult(fmt.Errorf(
			"no changes made to %s. The replacement produced identical content.",
			pathParam,
		)), nil
	}

	// Restore line endings and BOM, then write
	final := bom + restoreLineEndings(newContent, originalEnding)
	if err := os.WriteFile(absPath, []byte(final), 0o644); err != nil {
		return tools.ErrorResult(fmt.Errorf("cannot write %s: %w", pathParam, err)), nil
	}

	diff, firstLine := generateDiff(base, match.index, normOld, normNew)

	return tools.Result{
		Content: []ai.ContentBlock{
			ai.TextContent{Type: "text", Text: fmt.Sprintf("Successfully replaced text in %s.", pathParam)},
		},
		Details: EditDetails{Diff: diff, FirstChangedLine: firstLine},
	}, nil
}

// ---------------------------------------------------------------------------
// Fuzzy matching
// ---------------------------------------------------------------------------

type matchResult struct {
	found              bool
	index              int
	matchLen           int
	contentForReplacement string
}

func fuzzyFindText(content, oldText string) matchResult {
	// Exact match first
	if idx := strings.Index(content, oldText); idx != -1 {
		return matchResult{found: true, index: idx, matchLen: len(oldText), contentForReplacement: content}
	}
	// Fuzzy: normalise both sides
	fc := normalizeForFuzzyMatch(content)
	fo := normalizeForFuzzyMatch(oldText)
	if idx := strings.Index(fc, fo); idx != -1 {
		return matchResult{found: true, index: idx, matchLen: len(fo), contentForReplacement: fc}
	}
	return matchResult{}
}

// normalizeForFuzzyMatch strips trailing whitespace per line and normalises
// smart quotes, em-dashes, and Unicode spaces to their ASCII equivalents.
func normalizeForFuzzyMatch(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRightFunc(l, unicode.IsSpace)
	}
	s = strings.Join(lines, "\n")

	// Smart single quotes → '
	s = replaceRunes(s, []rune{'\u2018', '\u2019', '\u201A', '\u201B'}, '\'')
	// Smart double quotes → "
	s = replaceRunes(s, []rune{'\u201C', '\u201D', '\u201E', '\u201F'}, '"')
	// Various dashes → -
	s = replaceRunes(s, []rune{'\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212'}, '-')
	// Unicode spaces → regular space
	s = replaceRunes(s, []rune{'\u00A0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000'}, ' ')
	return s
}

func replaceRunes(s string, from []rune, to rune) string {
	return strings.Map(func(r rune) rune {
		for _, f := range from {
			if r == f {
				return to
			}
		}
		return r
	}, s)
}

// ---------------------------------------------------------------------------
// Diff generation
// ---------------------------------------------------------------------------

// generateDiff produces a contextual unified-style diff for the single
// replacement (no LCS needed — we know exactly what changed and where).
func generateDiff(base string, matchIndex int, oldText, newText string) (diff string, firstChangedLine int) {
	allLines := strings.Split(base, "\n")
	oldLines := strings.Split(oldText, "\n")
	// Strip trailing empty element if oldText ends with \n
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	newLines := strings.Split(newText, "\n")
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	// Find which line the match starts on (0-indexed)
	startLineIdx := strings.Count(base[:matchIndex], "\n")

	totalLines := len(allLines) + len(newLines) - len(oldLines)
	lineNumWidth := len(fmt.Sprintf("%d", max(len(allLines), totalLines)))
	pad := func(n int) string {
		return fmt.Sprintf("%*d", lineNumWidth, n)
	}

	firstChangedLine = startLineIdx + 1 // 1-indexed

	var sb strings.Builder

	// Context before (up to contextLines lines)
	contextStart := max(0, startLineIdx-contextLines)
	if contextStart > 0 {
		fmt.Fprintf(&sb, " %s ...\n", strings.Repeat(" ", lineNumWidth))
	}
	for i := contextStart; i < startLineIdx && i < len(allLines); i++ {
		fmt.Fprintf(&sb, " %s %s\n", pad(i+1), allLines[i])
	}

	// Removed lines
	for i, line := range oldLines {
		fmt.Fprintf(&sb, "-%s %s\n", pad(startLineIdx+i+1), line)
	}

	// Added lines
	for i, line := range newLines {
		fmt.Fprintf(&sb, "+%s %s\n", pad(startLineIdx+i+1), line)
	}

	// Context after
	afterStart := startLineIdx + len(oldLines)
	afterEnd := min(afterStart+contextLines, len(allLines))
	for i := afterStart; i < afterEnd; i++ {
		fmt.Fprintf(&sb, " %s %s\n", pad(i+1), allLines[i])
	}
	if afterEnd < len(allLines) {
		fmt.Fprintf(&sb, " %s ...\n", strings.Repeat(" ", lineNumWidth))
	}

	return strings.TrimRight(sb.String(), "\n"), firstChangedLine
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
