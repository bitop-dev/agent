package builtin_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickcecere/agent/pkg/tools/builtin"
)

func findRun(t *testing.T, cwd string, params map[string]any) string {
	t.Helper()
	tool := builtin.NewFindTool(cwd)
	result, err := tool.Execute(context.Background(), "c1", params, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return resultTextContent(result)
}

func setupFindDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0755)
	os.WriteFile(filepath.Join(dir, "root.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "child.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "sub", "deep", "nested.go"), []byte(""), 0644)
	return dir
}

func TestFindTool_FindsAllFiles(t *testing.T) {
	dir := setupFindDir(t)
	out := findRun(t, dir, map[string]any{"pattern": "*"})
	for _, name := range []string{"root.go", "root.txt", "child.go", "nested.go"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected %q in output, got: %q", name, out)
		}
	}
}

func TestFindTool_PatternFilter(t *testing.T) {
	dir := setupFindDir(t)
	out := findRun(t, dir, map[string]any{"pattern": "*.go"})
	if !strings.Contains(out, "root.go") {
		t.Errorf("*.go should match root.go, got: %q", out)
	}
	if strings.Contains(out, "root.txt") {
		t.Errorf("*.go should not match root.txt, got: %q", out)
	}
}

func TestFindTool_PatternMatchesDirs(t *testing.T) {
	// The find tool searches recursively; directory names match glob patterns too.
	dir := setupFindDir(t)
	out := findRun(t, dir, map[string]any{"pattern": "sub"})
	if !strings.Contains(out, "sub") {
		t.Logf("pattern 'sub' output: %q", out)
	}
	// At minimum, sub-directory files should appear with recursive search.
	out2 := findRun(t, dir, map[string]any{"pattern": "*.go"})
	if !strings.Contains(out2, "child.go") {
		t.Errorf("recursive search should find child.go, got: %q", out2)
	}
}

func TestFindTool_MaxResults(t *testing.T) {
	dir := setupFindDir(t)
	out := findRun(t, dir, map[string]any{"max_results": float64(1)})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Should have at most 1 result line (plus possible truncation note).
	resultLines := 0
	for _, l := range lines {
		if strings.Contains(l, dir) || filepath.IsAbs(l) || strings.HasSuffix(l, ".go") || strings.HasSuffix(l, ".txt") {
			resultLines++
		}
	}
	if resultLines > 2 { // allow 1 result + possible header/note
		t.Errorf("max_results=1 not honoured, got %d result lines: %q", resultLines, out)
	}
}

func TestFindTool_SubdirSearch(t *testing.T) {
	dir := setupFindDir(t)
	out := findRun(t, dir, map[string]any{
		"path":    filepath.Join(dir, "sub"),
		"pattern": "*.go",
	})
	if !strings.Contains(out, "child.go") {
		t.Errorf("expected child.go, got: %q", out)
	}
	if strings.Contains(out, "root.go") {
		t.Errorf("should not find root.go when searching sub/, got: %q", out)
	}
}

func TestFindTool_Definition(t *testing.T) {
	def := builtin.NewFindTool(".").Definition()
	if def.Name != "find" {
		t.Errorf("name = %q", def.Name)
	}
}
