package builtin_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/tools/builtin"
)

func readToolResult(t *testing.T, cwd string, params map[string]any) string {
	t.Helper()
	tool := builtin.NewReadTool(cwd)
	result, err := tool.Execute(context.Background(), "call1", params, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	var sb strings.Builder
	for _, b := range result.Content {
		if tc, ok := b.(ai.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestReadTool_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nline3\n"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	out := readToolResult(t, dir, map[string]any{"path": "test.txt"})
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line3") {
		t.Errorf("missing content: %q", out)
	}
}

func TestReadTool_Offset(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("A\nB\nC\nD\n"), 0644)

	out := readToolResult(t, dir, map[string]any{"path": "f.txt", "offset": float64(3)})
	if strings.Contains(out, "A") || strings.Contains(out, "B") {
		t.Errorf("offset not respected, got: %q", out)
	}
	if !strings.Contains(out, "C") {
		t.Errorf("expected line C from offset 3, got: %q", out)
	}
}

func TestReadTool_Limit(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("A\nB\nC\nD\nE\n"), 0644)

	out := readToolResult(t, dir, map[string]any{"path": "f.txt", "limit": float64(2)})
	if strings.Contains(out, "C") || strings.Contains(out, "D") {
		t.Errorf("limit not respected, got: %q", out)
	}
}

func TestReadTool_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	out := readToolResult(t, dir, map[string]any{"path": "missing.txt"})
	if !strings.Contains(strings.ToLower(out), "error") &&
		!strings.Contains(strings.ToLower(out), "no such") {
		t.Errorf("expected error message for missing file, got: %q", out)
	}
}

func TestReadTool_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "abs.txt")
	os.WriteFile(abs, []byte("absolute content\n"), 0644)

	out := readToolResult(t, "/some/other/cwd", map[string]any{"path": abs})
	if !strings.Contains(out, "absolute content") {
		t.Errorf("absolute path not resolved, got: %q", out)
	}
}

func TestReadTool_AtPrefixStripped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "at.txt"), []byte("at content\n"), 0644)

	out := readToolResult(t, dir, map[string]any{"path": "@at.txt"})
	if !strings.Contains(out, "at content") {
		t.Errorf("@ prefix not stripped, got: %q", out)
	}
}

func TestReadTool_Definition(t *testing.T) {
	def := builtin.NewReadTool(".").Definition()
	if def.Name != "read" {
		t.Errorf("name = %q", def.Name)
	}
	if def.Parameters == nil {
		t.Error("parameters schema should not be nil")
	}
}
