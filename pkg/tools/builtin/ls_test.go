package builtin_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickcecere/agent/pkg/tools/builtin"
)

func lsRun(t *testing.T, cwd string, params map[string]any) string {
	t.Helper()
	tool := builtin.NewLsTool(cwd)
	result, err := tool.Execute(context.Background(), "c1", params, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return resultTextContent(result)
}

func TestLsTool_ListsFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "alpha.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "beta.txt"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	out := lsRun(t, dir, map[string]any{})
	for _, name := range []string{"alpha.go", "beta.txt", "subdir"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected %q in ls output, got: %q", name, out)
		}
	}
}

func TestLsTool_IncludesDotfiles(t *testing.T) {
	// ls includes dotfiles by default (documented behaviour).
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "visible"), []byte("x"), 0644)

	out := lsRun(t, dir, map[string]any{})
	if !strings.Contains(out, ".hidden") {
		t.Errorf("ls should include dotfiles by default, got: %q", out)
	}
	if !strings.Contains(out, "visible") {
		t.Errorf("visible file should appear, got: %q", out)
	}
}

func TestLsTool_SpecificPath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "inner.go"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "outer.go"), []byte("x"), 0644)

	out := lsRun(t, dir, map[string]any{"path": sub})
	if !strings.Contains(out, "inner.go") {
		t.Errorf("expected inner.go from sub dir, got: %q", out)
	}
	if strings.Contains(out, "outer.go") {
		t.Errorf("should not show outer.go when listing sub/, got: %q", out)
	}
}

func TestLsTool_MissingDir(t *testing.T) {
	out := lsRun(t, t.TempDir(), map[string]any{"path": "/definitely/does/not/exist"})
	if !strings.Contains(strings.ToLower(out), "error") &&
		!strings.Contains(strings.ToLower(out), "no such") {
		t.Errorf("expected error for missing dir, got: %q", out)
	}
}

func TestLsTool_Definition(t *testing.T) {
	def := builtin.NewLsTool(".").Definition()
	if def.Name != "ls" {
		t.Errorf("name = %q", def.Name)
	}
}
