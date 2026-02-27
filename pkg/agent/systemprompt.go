// Package agent — system prompt construction.
//
// BuildSystemPrompt assembles the full system prompt from its parts:
//   - An optional user-supplied base prompt (replaces the default)
//   - Tool list and context-aware usage guidelines
//   - Project context files (AGENTS.md / CLAUDE.md)
//   - Current date/time and working directory
//
// It mirrors pi-mono's buildSystemPrompt() in
// packages/coding-agent/src/core/system-prompt.ts.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// toolDescriptions are the one-line descriptions shown to the model.
var toolDescriptions = map[string]string{
	"read":  "Read file contents",
	"bash":  "Execute bash commands (ls, grep, find, etc.)",
	"edit":  "Make surgical edits to files (find exact text and replace)",
	"write": "Create or overwrite files",
	"grep":  "Search file contents for patterns (respects .gitignore)",
	"find":  "Find files by glob pattern (respects .gitignore)",
	"ls":    "List directory contents",
}

// SystemPromptOptions controls how the system prompt is assembled.
type SystemPromptOptions struct {
	// CustomPrompt replaces the default system prompt preamble when non-empty.
	CustomPrompt string

	// AppendPrompt is appended to the prompt (after preamble, before context).
	AppendPrompt string

	// ActiveTools is the list of tool names currently registered.
	// Only tools with known descriptions are listed.
	ActiveTools []string

	// Cwd is the working directory reported to the model.
	// Defaults to os.Getwd().
	Cwd string

	// ContextFiles are pre-loaded project context files. If nil, BuildSystemPrompt
	// calls LoadContextFiles(Cwd) automatically.
	ContextFiles []ContextFile

	// SkillsBlock is the pre-formatted <available_skills>…</available_skills> XML
	// block produced by the skills system. Empty string = no skills section.
	SkillsBlock string
}

// ContextFile holds the content of an AGENTS.md or CLAUDE.md file.
type ContextFile struct {
	Path    string
	Content string
}

// BuildSystemPrompt constructs the system prompt from the given options.
func BuildSystemPrompt(opts SystemPromptOptions) string {
	cwd := opts.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	now := time.Now()
	dateTime := fmt.Sprintf("%s, %s %d, %d at %s %s",
		now.Format("Monday"),
		now.Format("January"),
		now.Day(),
		now.Year(),
		now.Format("3:04:05 PM"),
		now.Format("MST"),
	)

	// Resolve context files if not pre-loaded.
	contextFiles := opts.ContextFiles
	if contextFiles == nil {
		contextFiles = LoadContextFiles(cwd)
	}

	var sb strings.Builder

	if opts.CustomPrompt != "" {
		sb.WriteString(opts.CustomPrompt)
		writeAppend(&sb, opts.AppendPrompt)
		writeContextFiles(&sb, contextFiles)
		writeSkills(&sb, opts.SkillsBlock, opts.ActiveTools)
		writeDateCwd(&sb, dateTime, cwd)
		return sb.String()
	}

	// --- Default preamble ---
	tools := filterKnownTools(opts.ActiveTools)
	toolsList := buildToolsList(tools)
	guidelines := buildGuidelines(tools)

	sb.WriteString("You are an expert coding assistant. You help users by reading files, executing commands, editing code, and writing new files.\n")
	sb.WriteString("\nAvailable tools:\n")
	sb.WriteString(toolsList)
	sb.WriteString("\nGuidelines:\n")
	sb.WriteString(guidelines)

	writeAppend(&sb, opts.AppendPrompt)
	writeContextFiles(&sb, contextFiles)
	writeSkills(&sb, opts.SkillsBlock, opts.ActiveTools)
	writeDateCwd(&sb, dateTime, cwd)

	return sb.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func filterKnownTools(names []string) []string {
	var out []string
	for _, n := range names {
		if _, ok := toolDescriptions[n]; ok {
			out = append(out, n)
		}
	}
	return out
}

func buildToolsList(tools []string) string {
	if len(tools) == 0 {
		return "- (none)\n"
	}
	var sb strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&sb, "- %s: %s\n", t, toolDescriptions[t])
	}
	return sb.String()
}

func buildGuidelines(tools []string) string {
	has := func(name string) bool {
		for _, t := range tools {
			if t == name {
				return true
			}
		}
		return false
	}

	hasBash := has("bash")
	hasGrep := has("grep")
	hasFind := has("find")
	hasLs := has("ls")
	hasRead := has("read")
	hasEdit := has("edit")
	hasWrite := has("write")

	var lines []string

	// File exploration
	if hasBash && !hasGrep && !hasFind && !hasLs {
		lines = append(lines, "Use bash for file operations like ls, rg, find")
	} else if hasBash && (hasGrep || hasFind || hasLs) {
		lines = append(lines, "Prefer grep/find/ls tools over bash for file exploration (faster, respects .gitignore)")
	}

	if hasRead && hasEdit {
		lines = append(lines, "Use read to examine files before editing. You must use this tool instead of cat or sed.")
	}
	if hasEdit {
		lines = append(lines, "Use edit for precise changes (old text must match exactly)")
	}
	if hasWrite {
		lines = append(lines, "Use write only for new files or complete rewrites")
	}
	if hasEdit || hasWrite {
		lines = append(lines, "When summarizing your actions, output plain text directly - do NOT use cat or bash to display what you did")
	}

	lines = append(lines, "Be concise in your responses")
	lines = append(lines, "Show file paths clearly when working with files")

	var sb strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&sb, "- %s\n", l)
	}
	return sb.String()
}

func writeAppend(sb *strings.Builder, s string) {
	if s != "" {
		sb.WriteString("\n\n")
		sb.WriteString(s)
	}
}

func writeContextFiles(sb *strings.Builder, files []ContextFile) {
	if len(files) == 0 {
		return
	}
	sb.WriteString("\n\n# Project Context\n\nProject-specific instructions and guidelines:\n\n")
	for _, f := range files {
		fmt.Fprintf(sb, "## %s\n\n%s\n\n", f.Path, f.Content)
	}
}

func writeSkills(sb *strings.Builder, block string, activeTools []string) {
	if block == "" {
		return
	}
	// Only include skills if the read tool is available.
	for _, t := range activeTools {
		if t == "read" {
			sb.WriteString("\n\n")
			sb.WriteString(block)
			return
		}
	}
}

func writeDateCwd(sb *strings.Builder, dateTime, cwd string) {
	fmt.Fprintf(sb, "\nCurrent date and time: %s", dateTime)
	fmt.Fprintf(sb, "\nCurrent working directory: %s", cwd)
}

// ---------------------------------------------------------------------------
// Context file discovery
// ---------------------------------------------------------------------------

// contextFileNames are the filenames looked up in order (first match wins per dir).
var contextFileNames = []string{"AGENTS.md", "CLAUDE.md"}

// LoadContextFiles discovers and reads project context files from:
//  1. The global agent config directory (~/.config/agent/ or $XDG_CONFIG_HOME/agent/)
//  2. The working directory
//
// Returns at most one file per directory (first name that exists).
func LoadContextFiles(cwd string) []ContextFile {
	dirs := []string{globalAgentDir(), cwd}
	var files []ContextFile
	seen := map[string]bool{}

	for _, dir := range dirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		if f := readFirstContextFile(dir); f != nil {
			files = append(files, *f)
		}
	}
	return files
}

func readFirstContextFile(dir string) *ContextFile {
	for _, name := range contextFileNames {
		p := filepath.Join(dir, name)
		data, err := os.ReadFile(p)
		if err == nil {
			return &ContextFile{Path: p, Content: string(data)}
		}
	}
	return nil
}

// globalAgentDir returns the platform-appropriate config directory.
// Follows XDG on Linux/Mac; falls back to ~/.config/agent.
func globalAgentDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agent")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "agent")
}
