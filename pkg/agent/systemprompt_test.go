package agent

import (
	"strings"
	"testing"
)

func TestBuildSystemPrompt_DefaultTools(t *testing.T) {
	prompt := BuildSystemPrompt(SystemPromptOptions{
		ActiveTools: []string{"read", "bash", "edit", "write"},
		Cwd:         "/tmp/project",
	})

	checks := []string{
		"read: Read file contents",
		"edit: Make surgical edits",
		"Use read to examine files before editing",
		"Use edit for precise changes",
		"Use write only for new files",
		"Current working directory: /tmp/project",
	}
	for _, want := range checks {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	// Should not mention grep/find guidelines when they're not active
	if strings.Contains(prompt, "Prefer grep/find") {
		t.Error("prompt should not mention grep/find when those tools are not active")
	}
}

func TestBuildSystemPrompt_AllTools(t *testing.T) {
	prompt := BuildSystemPrompt(SystemPromptOptions{
		ActiveTools: []string{"read", "bash", "edit", "write", "grep", "find", "ls"},
		Cwd:         "/tmp",
	})

	if !strings.Contains(prompt, "Prefer grep/find/ls tools over bash") {
		t.Error("expected grep/find/ls guideline when all tools active")
	}
}

func TestBuildSystemPrompt_CustomPrompt(t *testing.T) {
	prompt := BuildSystemPrompt(SystemPromptOptions{
		CustomPrompt: "You are a pirate.",
		ActiveTools:  []string{"read"},
		Cwd:          "/arr",
	})

	if !strings.Contains(prompt, "You are a pirate.") {
		t.Error("expected custom prompt in output")
	}
	// Default preamble should not appear.
	if strings.Contains(prompt, "expert coding assistant") {
		t.Error("default preamble should not appear when CustomPrompt is set")
	}
	// Date/cwd should still be injected.
	if !strings.Contains(prompt, "Current working directory: /arr") {
		t.Error("expected cwd even with custom prompt")
	}
}

func TestBuildSystemPrompt_DateTimePresent(t *testing.T) {
	prompt := BuildSystemPrompt(SystemPromptOptions{Cwd: "/tmp"})
	if !strings.Contains(prompt, "Current date and time:") {
		t.Error("expected date/time in prompt")
	}
}

func TestBuildSystemPrompt_ContextFiles(t *testing.T) {
	prompt := BuildSystemPrompt(SystemPromptOptions{
		Cwd: "/tmp",
		ContextFiles: []ContextFile{
			{Path: "/tmp/AGENTS.md", Content: "Do not delete prod."},
		},
	})
	if !strings.Contains(prompt, "Project Context") {
		t.Error("expected Project Context section")
	}
	if !strings.Contains(prompt, "Do not delete prod.") {
		t.Error("expected context file content")
	}
}

func TestBuildSystemPrompt_SkillsRequireReadTool(t *testing.T) {
	skillsBlock := "<available_skills>...</available_skills>"

	// With read tool — skills should appear.
	withRead := BuildSystemPrompt(SystemPromptOptions{
		ActiveTools: []string{"read", "bash"},
		Cwd:         "/tmp",
		SkillsBlock: skillsBlock,
	})
	if !strings.Contains(withRead, skillsBlock) {
		t.Error("expected skills block when read tool is active")
	}

	// Without read tool — skills should be suppressed.
	withoutRead := BuildSystemPrompt(SystemPromptOptions{
		ActiveTools: []string{"bash"},
		Cwd:         "/tmp",
		SkillsBlock: skillsBlock,
	})
	if strings.Contains(withoutRead, skillsBlock) {
		t.Error("skills block should be suppressed when read tool is not active")
	}
}
