package builtin_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bitop-dev/agent/pkg/tools/builtin"
)

func writeTool(t *testing.T, cwd, path, content string) string {
	t.Helper()
	tool := builtin.NewWriteTool(cwd)
	result, err := tool.Execute(context.Background(), "c1", map[string]any{
		"path":    path,
		"content": content,
	}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return resultTextContent(result)
}

func TestWriteTool_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	writeTool(t, dir, "new.txt", "hello world")
	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q", data)
	}
}

func TestWriteTool_Overwrites(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "exist.txt"), []byte("old"), 0644)
	writeTool(t, dir, "exist.txt", "new content")
	data, _ := os.ReadFile(filepath.Join(dir, "exist.txt"))
	if string(data) != "new content" {
		t.Errorf("content = %q", data)
	}
}

func TestWriteTool_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	writeTool(t, dir, "a/b/c/file.txt", "nested")
	data, err := os.ReadFile(filepath.Join(dir, "a", "b", "c", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nested" {
		t.Errorf("content = %q", data)
	}
}

func TestWriteTool_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "abs.txt")
	writeTool(t, "/other/cwd", abs, "absolute")
	data, _ := os.ReadFile(abs)
	if string(data) != "absolute" {
		t.Errorf("content = %q", data)
	}
}

func TestWriteTool_MissingPath(t *testing.T) {
	tool := builtin.NewWriteTool(".")
	result, _ := tool.Execute(context.Background(), "c1", map[string]any{
		"content": "text",
	}, nil)
	out := resultTextContent(result)
	if !strings.Contains(strings.ToLower(out), "error") && !strings.Contains(out, "path") {
		t.Errorf("expected error for missing path, got: %q", out)
	}
}

func TestWriteTool_Definition(t *testing.T) {
	def := builtin.NewWriteTool(".").Definition()
	if def.Name != "write" {
		t.Errorf("name = %q", def.Name)
	}
}
