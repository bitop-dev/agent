// Package prompts loads and expands prompt templates.
//
// A prompt template is a Markdown file in a prompts directory. Typing
// /template-name arg1 arg2 in the REPL expands the template, substituting
// argument placeholders before sending the text to the agent.
//
// Discovery (mirrors pi-mono packages/coding-agent/src/core/prompt-templates.ts):
//   - Global:  ~/.config/agent/prompts/
//   - Project: {cwd}/.agent/prompts/
//
// Template files may have optional YAML frontmatter with a "description" field.
//
// Placeholder substitution:
//
//	$1, $2, …    positional arguments
//	$@           all arguments joined with spaces
//	$ARGUMENTS   same as $@
//	${@:N}       arguments from Nth onwards (1-indexed)
//	${@:N:L}     L arguments starting at Nth
package prompts

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const configDir = ".agent"

// Template is a loaded prompt template.
type Template struct {
	Name        string
	Description string
	Content     string // body after frontmatter
	Source      string // "user" | "project" | "path"
	FilePath    string
}

// LoadTemplates discovers templates from the global and project prompts dirs.
func LoadTemplates(cwd string) []Template {
	var all []Template
	seen := map[string]bool{}

	add := func(ts []Template) {
		for _, t := range ts {
			if !seen[t.Name] {
				seen[t.Name] = true
				all = append(all, t)
			}
		}
	}

	add(loadFromDir(globalPromptsDir(), "user"))
	add(loadFromDir(filepath.Join(cwd, configDir, "prompts"), "project"))
	return all
}

// Expand checks whether text begins with /template-name and, if so, expands
// the template with the remaining tokens as arguments.
// Returns the original text unchanged if no matching template is found.
func Expand(text string, templates []Template) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}

	spaceIdx := strings.IndexByte(text, ' ')
	var name, argsStr string
	if spaceIdx == -1 {
		name = text[1:]
	} else {
		name = text[1:spaceIdx]
		argsStr = text[spaceIdx+1:]
	}

	for _, t := range templates {
		if t.Name == name {
			args := parseArgs(argsStr)
			return substituteArgs(t.Content, args)
		}
	}

	return text
}

// ---------------------------------------------------------------------------
// Argument parsing
// ---------------------------------------------------------------------------

// parseArgs splits a string into arguments respecting single and double
// quoted strings (bash-style).
func parseArgs(s string) []string {
	var args []string
	var cur strings.Builder
	var inQuote rune

	for _, c := range s {
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			} else {
				cur.WriteRune(c)
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case ' ', '\t':
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

// slicePattern matches ${@:N} and ${@:N:L}
var slicePattern = regexp.MustCompile(`\$\{@:(\d+)(?::(\d+))?\}`)

// substituteArgs replaces placeholders in content with the given arguments.
func substituteArgs(content string, args []string) string {
	// $1, $2, … positional (do first so $10 isn't confused for $1 + "0")
	result := regexp.MustCompile(`\$(\d+)`).ReplaceAllStringFunc(content, func(m string) string {
		idx, _ := strconv.Atoi(m[1:])
		if idx < 1 || idx > len(args) {
			return ""
		}
		return args[idx-1]
	})

	// ${@:start} and ${@:start:length}
	result = slicePattern.ReplaceAllStringFunc(result, func(m string) string {
		sub := slicePattern.FindStringSubmatch(m)
		start, _ := strconv.Atoi(sub[1])
		if start < 1 {
			start = 1
		}
		start-- // convert to 0-indexed
		if start >= len(args) {
			return ""
		}
		if sub[2] != "" {
			length, _ := strconv.Atoi(sub[2])
			end := start + length
			if end > len(args) {
				end = len(args)
			}
			return strings.Join(args[start:end], " ")
		}
		return strings.Join(args[start:], " ")
	})

	allArgs := strings.Join(args, " ")
	result = strings.ReplaceAll(result, "$ARGUMENTS", allArgs)
	result = strings.ReplaceAll(result, "$@", allArgs)
	return result
}

// ---------------------------------------------------------------------------
// File loading
// ---------------------------------------------------------------------------

func loadFromDir(dir, source string) []Template {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var templates []Template
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		desc, body := splitFrontmatter(string(data))
		if desc == "" {
			// Use first non-empty line, truncated to 60 chars.
			for _, line := range strings.SplitN(body, "\n", 10) {
				if t := strings.TrimSpace(line); t != "" {
					if len(t) > 60 {
						t = t[:57] + "..."
					}
					desc = t
					break
				}
			}
		}

		abs, _ := filepath.Abs(path)
		templates = append(templates, Template{
			Name:        name,
			Description: desc,
			Content:     body,
			Source:      source,
			FilePath:    abs,
		})
	}
	return templates
}

// splitFrontmatter splits a markdown file into its frontmatter description
// and body. Returns ("", content) if there is no frontmatter.
func splitFrontmatter(content string) (description, body string) {
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return "", content
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
		k, v, ok := strings.Cut(lines[i], ":")
		if ok && strings.TrimSpace(k) == "description" {
			description = strings.TrimSpace(v)
		}
	}
	if end == -1 {
		return "", content
	}
	return description, strings.Join(lines[end+1:], "\n")
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

func globalPromptsDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agent", "prompts")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "agent", "prompts")
}
