package builtin_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bitop-dev/agent/pkg/tools/builtin"
)

func grepRun(t *testing.T, cwd string, params map[string]any) string {
	t.Helper()
	tool := builtin.NewGrepTool(cwd)
	result, err := tool.Execute(context.Background(), "c1", params, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return resultTextContent(result)
}

func setupGrepDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("func Hello() {}\nfunc World() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("var x = 42\nconst NAME = \"test\"\n"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("not a go file\n"), 0644)
	return dir
}

func TestGrepTool_FindsPattern(t *testing.T) {
	dir := setupGrepDir(t)
	out := grepRun(t, dir, map[string]any{"pattern": "func"})
	if !strings.Contains(out, "Hello") {
		t.Errorf("expected match for 'func', got: %q", out)
	}
}

func TestGrepTool_NoMatch(t *testing.T) {
	dir := setupGrepDir(t)
	out := grepRun(t, dir, map[string]any{"pattern": "XXXXNOTFOUND"})
	if strings.Contains(out, "Hello") {
		t.Errorf("unexpected match: %q", out)
	}
}

func TestGrepTool_GlobFilter(t *testing.T) {
	dir := setupGrepDir(t)
	out := grepRun(t, dir, map[string]any{
		"pattern": ".",
		"glob":    "*.go",
	})
	if strings.Contains(out, "c.txt") {
		t.Errorf("glob *.go should exclude .txt files, got: %q", out)
	}
}

func TestGrepTool_CaseInsensitive(t *testing.T) {
	dir := setupGrepDir(t)
	out := grepRun(t, dir, map[string]any{
		"pattern":    "hello",
		"ignoreCase": true,
	})
	if !strings.Contains(out, "Hello") {
		t.Errorf("case-insensitive match failed, got: %q", out)
	}
}

func TestGrepTool_CaseSensitive(t *testing.T) {
	dir := setupGrepDir(t)
	out := grepRun(t, dir, map[string]any{
		"pattern":    "hello",
		"ignoreCase": false,
	})
	// lowercase 'hello' should not match 'Hello' in case-sensitive mode.
	if strings.Contains(out, "Hello") {
		t.Errorf("case-sensitive: should not match 'Hello' for pattern 'hello', got: %q", out)
	}
}

func TestGrepTool_ContextLines(t *testing.T) {
	dir := setupGrepDir(t)
	out := grepRun(t, dir, map[string]any{
		"pattern": "Hello",
		"context": float64(1),
	})
	// With 1 context line, the next line (World) should appear.
	if !strings.Contains(out, "World") {
		t.Errorf("context lines not applied, got: %q", out)
	}
}

func TestGrepTool_SpecificFile(t *testing.T) {
	dir := setupGrepDir(t)
	out := grepRun(t, dir, map[string]any{
		"pattern": ".",
		"path":    filepath.Join(dir, "a.go"),
	})
	if strings.Contains(out, "const NAME") {
		t.Errorf("should only search a.go, got b.go content: %q", out)
	}
}

func TestGrepTool_Definition(t *testing.T) {
	def := builtin.NewGrepTool(".").Definition()
	if def.Name != "grep" {
		t.Errorf("name = %q", def.Name)
	}
}
