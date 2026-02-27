package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, dir, subdir, content string) string {
	t.Helper()
	full := filepath.Join(dir, subdir)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(full, "SKILL.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFromDir_SubdirSKILLmd(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "my-skill", "---\nname: my-skill\ndescription: Does something useful.\n---\n\n# Body")

	skills := loadFromDir(dir, "test")
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "my-skill" {
		t.Errorf("name = %q, want my-skill", skills[0].Name)
	}
	if skills[0].Description != "Does something useful." {
		t.Errorf("description = %q", skills[0].Description)
	}
}

func TestLoadFromDir_RootMd(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "go-review.md")
	os.WriteFile(p, []byte("---\nname: go-review\ndescription: Reviews Go code for correctness.\n---\n"), 0o644)

	skills := loadFromDir(dir, "test")
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "go-review" {
		t.Errorf("name = %q", skills[0].Name)
	}
}

func TestLoadFromDir_InvalidName(t *testing.T) {
	dir := t.TempDir()
	// Name with uppercase — should be rejected.
	writeSkill(t, dir, "MySkill", "---\nname: MySkill\ndescription: Uppercase.\n---\n")
	skills := loadFromDir(dir, "test")
	if len(skills) != 0 {
		t.Errorf("expected 0 skills for invalid name, got %d", len(skills))
	}
}

func TestLoadFromDir_MissingDescription(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "no-desc", "---\nname: no-desc\n---\n\nNo description.")
	skills := loadFromDir(dir, "test")
	if len(skills) != 0 {
		t.Errorf("expected 0 skills for missing description, got %d", len(skills))
	}
}

func TestFormatSkillsForPrompt(t *testing.T) {
	ss := []Skill{
		{Name: "web-search", Description: "Search the web.", FilePath: "/skills/web-search/SKILL.md"},
	}
	block := FormatSkillsForPrompt(ss)

	checks := []string{"<available_skills>", "<name>web-search</name>", "Search the web.", "/skills/web-search/SKILL.md"}
	for _, c := range checks {
		if !strings.Contains(block, c) {
			t.Errorf("missing %q in skills block", c)
		}
	}
}

func TestFormatSkillsForPrompt_Empty(t *testing.T) {
	if block := FormatSkillsForPrompt(nil); block != "" {
		t.Errorf("expected empty string for empty skills, got %q", block)
	}
}

func TestIsValidName(t *testing.T) {
	valid := []string{"my-skill", "go", "web-search-2", "a"}
	for _, n := range valid {
		if !isValidName(n) {
			t.Errorf("expected %q to be valid", n)
		}
	}

	invalid := []string{"", "MySkill", "-start", "end-", "double--hyphen", "has space"}
	for _, n := range invalid {
		if isValidName(n) {
			t.Errorf("expected %q to be invalid", n)
		}
	}
}
