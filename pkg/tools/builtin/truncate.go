// Package builtin provides the standard set of agent tools: read, bash, edit,
// write, grep, find, and ls — ported from pi-mono's coding-agent.
package builtin

import "fmt"

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	DefaultMaxLines   = 2000
	DefaultMaxBytes   = 50 * 1024 // 50 KB
	GrepMaxLineLength = 500
	contextLines      = 4 // lines of context shown around edits / grep matches
)

// ---------------------------------------------------------------------------
// TruncationResult
// ---------------------------------------------------------------------------

// TruncationResult describes what happened during a truncation operation.
type TruncationResult struct {
	Content               string
	Truncated             bool
	TruncatedBy           string // "lines" | "bytes" | ""
	TotalLines            int
	TotalBytes            int
	OutputLines           int
	OutputBytes           int
	LastLinePartial       bool
	FirstLineExceedsLimit bool
	MaxLines              int
	MaxBytes              int
}

// ---------------------------------------------------------------------------
// FormatSize
// ---------------------------------------------------------------------------

// FormatSize formats a byte count as a human-readable string.
func FormatSize(bytes int) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

// ---------------------------------------------------------------------------
// TruncateHead — keep first N lines/bytes (read, grep, find, ls)
// ---------------------------------------------------------------------------

// TruncateHead keeps the beginning of content up to maxLines or maxBytes.
// It never returns a partial line (except when the very first line exceeds
// the byte limit, in which case it returns empty content and sets
// FirstLineExceedsLimit).
func TruncateHead(content string, maxLines, maxBytes int) TruncationResult {
	lines := splitLines(content)
	totalLines := len(lines)
	totalBytes := len(content) // byte count (ASCII-safe; full UTF-8 via len([]byte(s)))

	// Recompute with actual UTF-8 byte count
	totalBytes = len([]byte(content))

	// No truncation needed?
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:     content,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
			MaxLines:    maxLines,
			MaxBytes:    maxBytes,
		}
	}

	// First line alone exceeds byte limit?
	if len(lines) > 0 && len([]byte(lines[0])) > maxBytes {
		return TruncationResult{
			Content:               "",
			Truncated:             true,
			TruncatedBy:           "bytes",
			TotalLines:            totalLines,
			TotalBytes:            totalBytes,
			FirstLineExceedsLimit: true,
			MaxLines:              maxLines,
			MaxBytes:              maxBytes,
		}
	}

	out := make([]string, 0, min(maxLines, totalLines))
	outBytes := 0
	truncatedBy := "lines"

	for i, line := range lines {
		if i >= maxLines {
			break
		}
		lineBytes := len([]byte(line))
		sep := 0
		if i > 0 {
			sep = 1 // newline separator
		}
		if outBytes+lineBytes+sep > maxBytes {
			truncatedBy = "bytes"
			break
		}
		out = append(out, line)
		outBytes += lineBytes + sep
	}

	// If we stopped due to reaching maxLines but bytes were fine, mark as lines
	if len(out) >= maxLines && outBytes <= maxBytes {
		truncatedBy = "lines"
	}

	joined := joinLines(out)
	return TruncationResult{
		Content:     joined,
		Truncated:   true,
		TruncatedBy: truncatedBy,
		TotalLines:  totalLines,
		TotalBytes:  totalBytes,
		OutputLines: len(out),
		OutputBytes: len([]byte(joined)),
		MaxLines:    maxLines,
		MaxBytes:    maxBytes,
	}
}

// ---------------------------------------------------------------------------
// TruncateTail — keep last N lines/bytes (bash)
// ---------------------------------------------------------------------------

// TruncateTail keeps the end of content up to maxLines or maxBytes.
// When a single line at the very end exceeds maxBytes, it returns a
// partial last line and sets LastLinePartial.
func TruncateTail(content string, maxLines, maxBytes int) TruncationResult {
	lines := splitLines(content)
	totalLines := len(lines)
	totalBytes := len([]byte(content))

	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:     content,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
			MaxLines:    maxLines,
			MaxBytes:    maxBytes,
		}
	}

	out := make([]string, 0, min(maxLines, totalLines))
	outBytes := 0
	truncatedBy := "lines"
	lastLinePartial := false

	for i := len(lines) - 1; i >= 0 && len(out) < maxLines; i-- {
		line := lines[i]
		lineBytes := len([]byte(line))
		sep := 0
		if len(out) > 0 {
			sep = 1
		}
		if outBytes+lineBytes+sep > maxBytes {
			truncatedBy = "bytes"
			// Edge case: not a single line added yet and this line alone exceeds the budget
			if len(out) == 0 {
				partial := truncateBytesFromEnd(line, maxBytes)
				out = append([]string{partial}, out...)
				outBytes = len([]byte(partial))
				lastLinePartial = true
			}
			break
		}
		out = append([]string{line}, out...)
		outBytes += lineBytes + sep
	}

	if len(out) >= maxLines && outBytes <= maxBytes {
		truncatedBy = "lines"
	}

	joined := joinLines(out)
	return TruncationResult{
		Content:         joined,
		Truncated:       true,
		TruncatedBy:     truncatedBy,
		TotalLines:      totalLines,
		TotalBytes:      totalBytes,
		OutputLines:     len(out),
		OutputBytes:     len([]byte(joined)),
		LastLinePartial: lastLinePartial,
		MaxLines:        maxLines,
		MaxBytes:        maxBytes,
	}
}

// ---------------------------------------------------------------------------
// TruncateLine — single-line truncation for grep
// ---------------------------------------------------------------------------

// TruncateLine truncates a single line to maxChars, appending "... [truncated]".
func TruncateLine(line string, maxChars int) (text string, wasTruncated bool) {
	runes := []rune(line)
	if len(runes) <= maxChars {
		return line, false
	}
	return string(runes[:maxChars]) + "... [truncated]", true
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	// Split on \n; keep empty final element to mirror JS behaviour
	out := make([]string, 0, 64)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	total := 0
	for i, l := range lines {
		total += len(l)
		if i < len(lines)-1 {
			total++
		}
	}
	buf := make([]byte, 0, total)
	for i, l := range lines {
		buf = append(buf, l...)
		if i < len(lines)-1 {
			buf = append(buf, '\n')
		}
	}
	return string(buf)
}

// truncateBytesFromEnd returns the last maxBytes UTF-8 bytes of s, starting
// at a valid rune boundary.
func truncateBytesFromEnd(s string, maxBytes int) string {
	b := []byte(s)
	if len(b) <= maxBytes {
		return s
	}
	start := len(b) - maxBytes
	// advance to a valid UTF-8 rune start
	for start < len(b) && (b[start]&0xc0) == 0x80 {
		start++
	}
	return string(b[start:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
