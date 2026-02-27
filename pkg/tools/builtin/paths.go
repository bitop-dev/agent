package builtin

import (
	"os"
	"path/filepath"
	"strings"
)

// resolvePath resolves a user-supplied path relative to cwd.
// Handles ~ expansion and absolute paths.
func resolvePath(p, cwd string) string {
	p = strings.TrimSpace(p)
	// Strip leading @ (pi convention)
	p = strings.TrimPrefix(p, "@")

	// ~ expansion
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}

	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cwd, p)
}

// normalizeToLF replaces all CRLF and lone CR with LF.
func normalizeToLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// detectLineEnding returns "\r\n" if the content uses Windows line endings,
// otherwise "\n".
func detectLineEnding(s string) string {
	crlfIdx := strings.Index(s, "\r\n")
	lfIdx := strings.Index(s, "\n")
	if lfIdx == -1 {
		return "\n"
	}
	if crlfIdx == -1 {
		return "\n"
	}
	if crlfIdx < lfIdx {
		return "\r\n"
	}
	return "\n"
}

// restoreLineEndings replaces LF with the original line ending.
func restoreLineEndings(s, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(s, "\n", "\r\n")
	}
	return s
}

// stripBOM removes a leading UTF-8 BOM if present and returns it separately.
func stripBOM(s string) (bom, text string) {
	if strings.HasPrefix(s, "\uFEFF") {
		return "\uFEFF", s[3:] // BOM is 3 bytes in UTF-8
	}
	return "", s
}
