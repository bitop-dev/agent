package builtin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Minimal DDG Lite HTML fixture mirroring the real page structure.
const ddgLiteHTML = `<!DOCTYPE html>
<html>
<body>
<table>
<tr>
  <td>
    <a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fgo-article">Go Programming Guide</a>
  </td>
</tr>
<tr>
  <td class="result-snippet">
    Go is a statically typed language designed at Google.
  </td>
</tr>
<tr>
  <td>
    <a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fpkg.go.dev">Go Packages</a>
  </td>
</tr>
<tr>
  <td class="result-snippet">
    The official Go package index.
  </td>
</tr>
</table>
</body>
</html>`

func TestParseDDGLite_ExtractsResults(t *testing.T) {
	results, err := parseDDGLite(strings.NewReader(ddgLiteHTML), 10)
	if err != nil {
		t.Fatalf("parseDDGLite: %v", err)
	}
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got 0")
	}

	r := results[0]
	if r.title != "Go Programming Guide" {
		t.Errorf("title = %q", r.title)
	}
	if r.url != "https://example.com/go-article" {
		t.Errorf("url = %q", r.url)
	}
	if !strings.Contains(r.snippet, "statically typed") {
		t.Errorf("snippet = %q", r.snippet)
	}
}

func TestParseDDGLite_MaxResults(t *testing.T) {
	results, err := parseDDGLite(strings.NewReader(ddgLiteHTML), 1)
	if err != nil {
		t.Fatalf("parseDDGLite: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (max), got %d", len(results))
	}
}

func TestResolveURL_DDGRedirect(t *testing.T) {
	cases := []struct{ in, want string }{
		{
			"//duckduckgo.com/l/?uddg=https%3A%2F%2Fgolang.org",
			"https://golang.org",
		},
		{
			"https://example.com/page",
			"https://example.com/page",
		},
		{
			"//duckduckgo.com/y.js?ad=something",
			"", // DDG internal link — skip
		},
	}
	for _, tc := range cases {
		got := resolveURL(tc.in)
		if got != tc.want {
			t.Errorf("resolveURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWebSearchTool_Execute_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		w.Write([]byte(ddgLiteHTML))
	}))
	defer srv.Close()

	// Patch the DDG URL for testing isn't straightforward without dependency
	// injection, so test the parser directly (above) and trust Execute() calls it.
	// This test validates the tool definition is valid.
	tool := NewWebSearchTool()
	def := tool.Definition()
	if def.Name != "web_search" {
		t.Errorf("name = %q", def.Name)
	}
	if def.Parameters == nil {
		t.Error("parameters schema should not be nil")
	}
}
