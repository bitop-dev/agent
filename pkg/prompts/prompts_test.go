package prompts

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemplate(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSubstituteArgs_Positional(t *testing.T) {
	result := substituteArgs("Hello $1, you are $2!", []string{"Alice", "awesome"})
	if result != "Hello Alice, you are awesome!" {
		t.Errorf("got %q", result)
	}
}

func TestSubstituteArgs_AllArgs(t *testing.T) {
	result := substituteArgs("Args: $@", []string{"a", "b", "c"})
	if result != "Args: a b c" {
		t.Errorf("got %q", result)
	}
}

func TestSubstituteArgs_ARGUMENTS(t *testing.T) {
	result := substituteArgs("Args: $ARGUMENTS", []string{"x", "y"})
	if result != "Args: x y" {
		t.Errorf("got %q", result)
	}
}

func TestSubstituteArgs_Slice(t *testing.T) {
	// ${@:2} = from 2nd arg onwards
	result := substituteArgs("Rest: ${@:2}", []string{"a", "b", "c", "d"})
	if result != "Rest: b c d" {
		t.Errorf("got %q", result)
	}
}

func TestSubstituteArgs_SliceWithLength(t *testing.T) {
	// ${@:2:2} = 2 args starting from 2nd
	result := substituteArgs("Two: ${@:2:2}", []string{"a", "b", "c", "d"})
	if result != "Two: b c" {
		t.Errorf("got %q", result)
	}
}

func TestSubstituteArgs_MissingArg(t *testing.T) {
	// $2 with only 1 arg → empty
	result := substituteArgs("$1 $2", []string{"only"})
	if result != "only " {
		t.Errorf("got %q", result)
	}
}

func TestParseArgs_Quoted(t *testing.T) {
	args := parseArgs(`foo "bar baz" qux`)
	if len(args) != 3 {
		t.Fatalf("got %d args: %v", len(args), args)
	}
	if args[1] != "bar baz" {
		t.Errorf("args[1] = %q, want \"bar baz\"", args[1])
	}
}

func TestExpand_NoTemplate(t *testing.T) {
	text := "just a normal prompt"
	if got := Expand(text, nil); got != text {
		t.Errorf("Expand changed non-template text: %q", got)
	}
}

func TestExpand_UnknownTemplate(t *testing.T) {
	text := "/unknown arg1"
	if got := Expand(text, nil); got != text {
		t.Errorf("Expand should return original for unknown template, got %q", got)
	}
}

func TestExpand_MatchesTemplate(t *testing.T) {
	templates := []Template{
		{Name: "greet", Content: "Hello, $1!"},
	}
	result := Expand("/greet World", templates)
	if result != "Hello, World!" {
		t.Errorf("got %q", result)
	}
}

func TestLoadTemplates_FromDir(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "review", "---\ndescription: Review code.\n---\nPlease review $1.")

	templates := loadFromDir(dir, "test")
	if len(templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(templates))
	}
	if templates[0].Name != "review" {
		t.Errorf("name = %q", templates[0].Name)
	}
	if templates[0].Description != "Review code." {
		t.Errorf("description = %q", templates[0].Description)
	}
}
