package builtin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// WebSearchTool performs a web search via DuckDuckGo Lite (no API key required).
type WebSearchTool struct{}

func NewWebSearchTool() *WebSearchTool { return &WebSearchTool{} }

func (t *WebSearchTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        "web_search",
		Description: "Search the web using DuckDuckGo. Returns titles, URLs, and snippets for the top results. No API key required.",
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"query": {Type: "string", Description: "The search query"},
				"max_results": {Type: "number", Description: "Maximum number of results to return (default: 10, max: 20)"},
			},
			Required: []string{"query"},
		}),
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, _ string, params map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return tools.ErrorResult(fmt.Errorf("query is required")), nil
	}

	maxResults := 10
	if v, ok := params["max_results"]; ok {
		switch n := v.(type) {
		case float64:
			maxResults = int(n)
		case int:
			maxResults = n
		}
	}
	if maxResults < 1 {
		maxResults = 1
	}
	if maxResults > 20 {
		maxResults = 20
	}

	results, err := ddgSearch(ctx, query, maxResults)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("web search failed: %w", err)), nil
	}

	if len(results) == 0 {
		return tools.TextResult("No results found."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Search results for: %q\n\n", query)
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. **%s**\n   URL: %s\n   %s\n\n", i+1, r.title, r.url, r.snippet)
	}
	return tools.TextResult(strings.TrimRight(sb.String(), "\n")), nil
}

type searchResult struct {
	title   string
	url     string
	snippet string
}

func ddgSearch(ctx context.Context, query string, maxResults int) ([]searchResult, error) {
	reqURL := "https://lite.duckduckgo.com/lite/?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	// DDG Lite requires a browser-like User-Agent or returns a captcha page.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; agent-search/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DDG returned HTTP %d", resp.StatusCode)
	}

	return parseDDGLite(resp.Body, maxResults)
}

// parseDDGLite extracts search results from DuckDuckGo Lite HTML.
//
// DDG Lite renders results in a table. Each result row contains:
//   - an <a class="result-link"> for the title + URL
//   - a <td class="result-snippet"> for the description
func parseDDGLite(r io.Reader, maxResults int) ([]searchResult, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	var results []searchResult

	// Walk the HTML tree looking for result-link anchors.
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= maxResults {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			cls := attrVal(n, "class")
			href := attrVal(n, "href")
			if strings.Contains(cls, "result-link") && href != "" {
				title := textContent(n)
				// The snippet is in the next sibling table row's td.result-snippet.
				snippet := findNextSnippet(n)
				// Resolve DDG redirect URLs.
				resultURL := resolveURL(href)
				if resultURL != "" && title != "" {
					results = append(results, searchResult{
						title:   strings.TrimSpace(title),
						url:     resultURL,
						snippet: strings.TrimSpace(snippet),
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return results, nil
}

// findNextSnippet walks up to the containing <tr>, then finds the next <tr>
// with a .result-snippet td.
func findNextSnippet(anchor *html.Node) string {
	// Walk up to the <tr> containing this anchor.
	tr := ancestor(anchor, "tr")
	if tr == nil {
		return ""
	}
	// The snippet is typically in the next <tr>.
	for sib := tr.NextSibling; sib != nil; sib = sib.NextSibling {
		if sib.Type != html.ElementNode {
			continue
		}
		if sib.Data == "tr" {
			// Find td.result-snippet inside.
			if snip := findChild(sib, "td", "result-snippet"); snip != nil {
				return textContent(snip)
			}
			// Also check direct text in tr.
			t := textContent(sib)
			if strings.TrimSpace(t) != "" {
				return t
			}
			break
		}
	}
	return ""
}

// resolveURL unwraps DDG's redirect URLs like //duckduckgo.com/l/?uddg=...
func resolveURL(href string) string {
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	// DDG Lite redirect: extract uddg= parameter.
	if uddg := u.Query().Get("uddg"); uddg != "" {
		if decoded, err := url.QueryUnescape(uddg); err == nil {
			return decoded
		}
		return uddg
	}
	// Skip internal DDG links.
	if strings.Contains(u.Host, "duckduckgo.com") {
		return ""
	}
	return href
}

// ---------------------------------------------------------------------------
// HTML helpers
// ---------------------------------------------------------------------------

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func ancestor(n *html.Node, tag string) *html.Node {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == tag {
			return p
		}
	}
	return nil
}

func findChild(n *html.Node, tag, class string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			if class == "" || strings.Contains(attrVal(c, "class"), class) {
				return c
			}
		}
		if found := findChild(c, tag, class); found != nil {
			return found
		}
	}
	return nil
}
