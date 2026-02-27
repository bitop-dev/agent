package builtin

import (
	"strings"
	"testing"
)

func TestTruncateHead_NoTruncation(t *testing.T) {
	content := "line1\nline2\nline3"
	tr := TruncateHead(content, DefaultMaxLines, DefaultMaxBytes)
	if tr.Truncated {
		t.Error("expected no truncation")
	}
	if tr.Content != content {
		t.Errorf("content mismatch: %q", tr.Content)
	}
}

func TestTruncateHead_LineLimit(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	tr := TruncateHead(content, 5, DefaultMaxBytes)
	if !tr.Truncated {
		t.Error("expected truncation")
	}
	if tr.TruncatedBy != "lines" {
		t.Errorf("expected lines, got %q", tr.TruncatedBy)
	}
	if tr.OutputLines != 5 {
		t.Errorf("expected 5 output lines, got %d", tr.OutputLines)
	}
}

func TestTruncateHead_ByteLimit(t *testing.T) {
	content := strings.Repeat("a", 100)
	tr := TruncateHead(content, DefaultMaxLines, 50)
	if !tr.Truncated {
		t.Error("expected truncation")
	}
	if tr.TruncatedBy != "bytes" {
		t.Errorf("expected bytes, got %q", tr.TruncatedBy)
	}
}

func TestTruncateTail_KeepsEnd(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	tr := TruncateTail(content, 3, DefaultMaxBytes)
	if !tr.Truncated {
		t.Error("expected truncation")
	}
	if tr.OutputLines != 3 {
		t.Errorf("expected 3 output lines, got %d", tr.OutputLines)
	}
	// Should be the last 3 lines
	if tr.Content != "line\nline\nline" {
		t.Errorf("unexpected content: %q", tr.Content)
	}
}

func TestTruncateLine(t *testing.T) {
	long := strings.Repeat("a", 600)
	out, truncated := TruncateLine(long, GrepMaxLineLength)
	if !truncated {
		t.Error("expected truncation")
	}
	if !strings.HasSuffix(out, "... [truncated]") {
		t.Errorf("missing suffix: %q", out)
	}

	short := "hello"
	out2, truncated2 := TruncateLine(short, GrepMaxLineLength)
	if truncated2 {
		t.Error("did not expect truncation")
	}
	if out2 != short {
		t.Errorf("expected %q, got %q", short, out2)
	}
}

func TestFormatSize(t *testing.T) {
	cases := []struct{ in int; want string }{
		{0, "0B"}, {512, "512B"}, {1024, "1.0KB"}, {50 * 1024, "50.0KB"},
	}
	for _, c := range cases {
		got := FormatSize(c.in)
		if got != c.want {
			t.Errorf("FormatSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
