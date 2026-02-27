// Package skills discovers and loads agent skill files.
//
// A skill is a Markdown file with YAML frontmatter that provides specialized
// instructions for specific tasks. The agent's system prompt lists all
// available skills; when a task matches the agent reads the skill file using
// the read tool to get detailed instructions.
//
// Discovery rules (mirrors pi-mono packages/coding-agent/src/core/skills.ts):
//   - Global:  ~/.config/agent/skills/  (or $XDG_CONFIG_HOME/agent/skills/)
//   - Project: {cwd}/.agent/skills/
//   - Files:   root .md files, or SKILL.md under subdirectories
//
// Frontmatter:
//
//	---
//	name: my-skill
//	description: Does something useful when X.
//	---
//
// The name must match the parent directory name (for SKILL.md files) or the
// filename (for root .md files), consist of lowercase a-z, 0-9, and hyphens,
// and not exceed 64 characters.
package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxNameLen = 64
	maxDescLen = 1024
	configDir  = ".agent"
)

// Skill is a loaded skill file.
type Skill struct {
	Name        string
	Description string
	FilePath    string // absolute path (for injecting into system prompt)
	Source      string // "user" | "project" | "path"
}

// LoadSkills discovers skills from the global agent directory and the project
// working directory. Collisions (same name) are resolved first-write-wins
// (global skills take priority over project skills).
func LoadSkills(cwd string) []Skill {
	skillMap := map[string]Skill{}

	addAll := func(skills []Skill) {
		for _, s := range skills {
			if _, exists := skillMap[s.Name]; !exists {
				skillMap[s.Name] = s
			}
		}
	}

	addAll(loadFromDir(globalSkillsDir(), "user"))
	addAll(loadFromDir(filepath.Join(cwd, configDir, "skills"), "project"))

	out := make([]Skill, 0, len(skillMap))
	for _, s := range skillMap {
		out = append(out, s)
	}
	return out
}

// LoadSkillsFromDirs loads skills from explicit directories in addition to
// the defaults. Useful for tests or CLI overrides.
func LoadSkillsFromDirs(cwd string, extra ...string) []Skill {
	all := LoadSkills(cwd)
	names := map[string]bool{}
	for _, s := range all {
		names[s.Name] = true
	}
	for _, dir := range extra {
		for _, s := range loadFromDir(dir, "path") {
			if !names[s.Name] {
				names[s.Name] = true
				all = append(all, s)
			}
		}
	}
	return all
}

// FormatSkillsForPrompt returns the <available_skills> XML block to inject
// into the system prompt, following the Agent Skills standard.
// Skills with empty descriptions are skipped.
func FormatSkillsForPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\nThe following skills provide specialized instructions for specific tasks.\n")
	sb.WriteString("Use the read tool to load a skill's file when the task matches its description.\n")
	sb.WriteString("When a skill file references a relative path, resolve it against the skill directory")
	sb.WriteString(" (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.\n")
	sb.WriteString("\n<available_skills>\n")
	for _, s := range skills {
		sb.WriteString("  <skill>\n")
		fmt.Fprintf(&sb, "    <name>%s</name>\n", escapeXML(s.Name))
		fmt.Fprintf(&sb, "    <description>%s</description>\n", escapeXML(s.Description))
		fmt.Fprintf(&sb, "    <location>%s</location>\n", escapeXML(s.FilePath))
		sb.WriteString("  </skill>\n")
	}
	sb.WriteString("</available_skills>")
	return sb.String()
}

// ---------------------------------------------------------------------------
// Internal loading
// ---------------------------------------------------------------------------

func loadFromDir(dir, source string) []Skill {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var skills []Skill
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(dir, e.Name())

		if e.IsDir() {
			// Look for SKILL.md inside the subdirectory.
			skillFile := filepath.Join(full, "SKILL.md")
			if s := loadSkillFile(skillFile, e.Name(), source); s != nil {
				skills = append(skills, *s)
			}
			continue
		}

		// Root .md files in the skills directory.
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			name := strings.TrimSuffix(e.Name(), ".md")
			if s := loadSkillFile(full, name, source); s != nil {
				skills = append(skills, *s)
			}
		}
	}
	return skills
}

// loadSkillFile parses a skill markdown file and returns a Skill, or nil on
// validation failure (errors are silently dropped, matching pi's behaviour).
func loadSkillFile(path, expectedName, source string) *Skill {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	fm := parseFrontmatter(string(data))
	name := fm["name"]
	if name == "" {
		name = expectedName
	}
	description := fm["description"]

	if description == "" || len(description) > maxDescLen {
		return nil
	}
	if !isValidName(name) || len(name) > maxNameLen {
		return nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	return &Skill{
		Name:        name,
		Description: description,
		FilePath:    abs,
		Source:      source,
	}
}

// ---------------------------------------------------------------------------
// Frontmatter parser
// ---------------------------------------------------------------------------

// parseFrontmatter extracts key: value pairs from a YAML frontmatter block
// (between --- delimiters). Only simple string values are parsed.
func parseFrontmatter(content string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(content, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// Strip surrounding quotes.
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		// Handle multi-line values (pipe/folded) by reading indented lines.
		if v == "|" || v == ">" {
			var sb strings.Builder
			sep := ""
			for i++; i < len(lines); i++ {
				if !strings.HasPrefix(lines[i], " ") && !strings.HasPrefix(lines[i], "\t") {
					i-- // put back
					break
				}
				sb.WriteString(sep)
				sb.WriteString(strings.TrimSpace(lines[i]))
				sep = " "
			}
			v = sb.String()
		}
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// Scan multi-line value from a bufio.Scanner (unused — kept for reference).
var _ = bufio.NewScanner

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func isValidName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return false
	}
	if strings.Contains(name, "--") {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// XML escaping
// ---------------------------------------------------------------------------

func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

func globalSkillsDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agent", "skills")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "agent", "skills")
}
