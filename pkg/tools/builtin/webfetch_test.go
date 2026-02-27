package builtin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// resultText extracts the concatenated text from a Result's content blocks.
func resultText(r tools.Result) string {
	var sb strings.Builder
	for _, b := range r.Content {
		if tc, ok := b.(ai.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// htmlToText
// ---------------------------------------------------------------------------

func TestHtmlToText_HeadingsAndParagraphs(t *testing.T) {
	input := `<html><body>
<h1>Main Title</h1>
<h2>Sub Section</h2>
<p>This is a paragraph with <strong>bold</strong> text.</p>
<p>Second paragraph.</p>
</body></html>`

	got := htmlToText([]byte(input))
	if !strings.Contains(got, "# Main Title") {
		t.Errorf("missing h1: %q", got)
	}
	if !strings.Contains(got, "## Sub Section") {
		t.Errorf("missing h2: %q", got)
	}
	if !strings.Contains(got, "paragraph") {
		t.Errorf("missing paragraph text: %q", got)
	}
}

func TestHtmlToText_SkipsScriptStyle(t *testing.T) {
	input := `<html><head><style>body{color:red}</style></head>
<body>
<script>alert("xss")</script>
<p>Real content here.</p>
</body></html>`

	got := htmlToText([]byte(input))
	if strings.Contains(got, "alert") {
		t.Error("should strip <script> content")
	}
	if strings.Contains(got, "color:red") {
		t.Error("should strip <style> content")
	}
	if !strings.Contains(got, "Real content here") {
		t.Error("should keep paragraph content")
	}
}

func TestHtmlToText_Links(t *testing.T) {
	input := `<p><a href="https://golang.org">Go website</a></p>`
	got := htmlToText([]byte(input))
	if !strings.Contains(got, "Go website") {
		t.Errorf("should contain link text: %q", got)
	}
	if !strings.Contains(got, "golang.org") {
		t.Errorf("should contain link URL: %q", got)
	}
}

func TestHtmlToText_Lists(t *testing.T) {
	input := `<ul>
<li>Item one</li>
<li>Item two</li>
</ul>
<ol>
<li>First</li>
<li>Second</li>
</ol>`

	got := htmlToText([]byte(input))
	if !strings.Contains(got, "• Item one") {
		t.Errorf("unordered list: %q", got)
	}
	if !strings.Contains(got, "1. First") {
		t.Errorf("ordered list: %q", got)
	}
}

func TestHtmlToText_CodeBlocks(t *testing.T) {
	input := `<pre><code>func main() {
    fmt.Println("hello")
}</code></pre>`

	got := htmlToText([]byte(input))
	if !strings.Contains(got, "```") {
		t.Errorf("code block should have fences: %q", got)
	}
	if !strings.Contains(got, "func main()") {
		t.Errorf("code content missing: %q", got)
	}
}

func TestCleanWhitespace(t *testing.T) {
	input := "  line one  \n\n\n\n  line two  \n\n\n\nline three"
	got := cleanWhitespace(input)
	if strings.Contains(got, "\n\n\n") {
		t.Errorf("too many consecutive blank lines: %q", got)
	}
	if !strings.Contains(got, "line one") {
		t.Error("should contain content")
	}
}

// ---------------------------------------------------------------------------
// WebFetchTool.Execute
// ---------------------------------------------------------------------------

func TestWebFetchTool_Execute_HTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1>Test Page</h1><p>Hello from the server.</p></body></html>`))
	}))
	defer srv.Close()

	result, err := NewWebFetchTool().Execute(context.Background(), "", map[string]any{"url": srv.URL}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("result should have content blocks")
	}
	text := resultText(result)
	if strings.Contains(text, "error:") {
		t.Errorf("unexpected error in result: %q", text)
	}
	if !strings.Contains(text, "Test Page") {
		t.Errorf("result should contain page heading, got: %q", text)
	}
}

func TestWebFetchTool_Execute_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	}))
	defer srv.Close()

	result, _ := NewWebFetchTool().Execute(context.Background(), "", map[string]any{"url": srv.URL}, nil)
	text := resultText(result)
	if !strings.Contains(text, "error") {
		t.Errorf("404 should return error text, got: %q", text)
	}
}

func TestWebFetchTool_Execute_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("just plain text content"))
	}))
	defer srv.Close()

	result, _ := NewWebFetchTool().Execute(context.Background(), "", map[string]any{"url": srv.URL}, nil)
	text := resultText(result)
	if !strings.Contains(text, "plain text content") {
		t.Errorf("plain text result = %q", text)
	}
}

func TestWebFetchTool_Execute_Truncation(t *testing.T) {
	big := strings.Repeat("x", 2000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(big))
	}))
	defer srv.Close()

	// Request only 1024 bytes.
	result, _ := NewWebFetchTool().Execute(context.Background(), "", map[string]any{
		"url":       srv.URL,
		"max_bytes": float64(1024),
	}, nil)
	text := resultText(result)
	if !strings.Contains(text, "truncated") {
		t.Errorf("should mention truncation: %q", text)
	}
}
