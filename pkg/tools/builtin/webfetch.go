package builtin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// WebFetchTool fetches a URL and returns its content as clean plain text.
type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool { return &WebFetchTool{} }

func (t *WebFetchTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name: "web_fetch",
		Description: "Fetch a web page and return its content as plain text. " +
			"HTML is converted to readable text. " +
			"Output is truncated to 50 KB. Useful for reading documentation, articles, and search result pages.",
		Parameters: tools.MustSchema(tools.SimpleSchema{
			Properties: map[string]tools.Property{
				"url":      {Type: "string", Description: "The URL to fetch"},
				"max_bytes": {Type: "number", Description: "Maximum response size in bytes (default: 51200, max: 102400)"},
			},
			Required: []string{"url"},
		}),
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, _ string, params map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	rawURL, _ := params["url"].(string)
	if rawURL == "" {
		return tools.ErrorResult(fmt.Errorf("url is required")), nil
	}

	maxBytes := 51200
	if v, ok := params["max_bytes"]; ok {
		switch n := v.(type) {
		case float64:
			maxBytes = int(n)
		case int:
			maxBytes = n
		}
	}
	if maxBytes < 1024 {
		maxBytes = 1024
	}
	if maxBytes > 102400 {
		maxBytes = 102400
	}

	content, finalURL, err := fetchPage(ctx, rawURL, maxBytes)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("fetch %s: %w", rawURL, err)), nil
	}

	var sb strings.Builder
	if finalURL != rawURL {
		fmt.Fprintf(&sb, "[Redirected to: %s]\n\n", finalURL)
	}
	sb.WriteString(content)
	return tools.TextResult(sb.String()), nil
}

func fetchPage(ctx context.Context, rawURL string, maxBytes int) (content, finalURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", rawURL, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; agent-fetch/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", rawURL, err
	}
	defer resp.Body.Close()

	finalURL = resp.Request.URL.String()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", finalURL, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, int64(maxBytes)+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return "", finalURL, err
	}

	truncated := len(bodyBytes) > maxBytes
	if truncated {
		bodyBytes = bodyBytes[:maxBytes]
	}

	ct := resp.Header.Get("Content-Type")
	var text string
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		text = htmlToText(bodyBytes)
	} else {
		// Plain text, JSON, etc. — return as-is.
		text = string(bodyBytes)
	}

	if truncated {
		text = strings.TrimRight(text, "\n") +
			fmt.Sprintf("\n\n[Content truncated at %s. Refetch with a larger max_bytes if needed.]",
				FormatSize(maxBytes))
	}

	return text, finalURL, nil
}

// htmlToText converts HTML bytes to readable plain text.
// - <script>, <style>, <noscript>, <nav>, <footer>, <header> blocks are dropped
// - Block elements become newlines
// - Links rendered as text (URL)
// - Headings prefixed with #/##/###
// - Lists prefixed with • / number.
func htmlToText(data []byte) string {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		// Fallback: strip all tags.
		return stripTags(string(data))
	}

	var sb strings.Builder
	renderNode(&sb, doc, 0)
	return cleanWhitespace(sb.String())
}

// skipTags are elements whose entire subtree is suppressed.
var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true,
	"nav": true, "footer": true, "head": true,
	"svg": true, "button": true, "form": true,
	"iframe": true, "object": true, "embed": true,
}

// blockTags emit a newline before and after their content.
var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true,
	"main": true, "aside": true, "blockquote": true,
	"li": true, "dt": true, "dd": true,
	"tr": true, "td": true, "th": true,
	"pre": true, "figure": true, "figcaption": true,
}

func renderNode(sb *strings.Builder, n *html.Node, depth int) {
	if n.Type == html.TextNode {
		sb.WriteString(n.Data)
		return
	}
	if n.Type != html.ElementNode && n.Type != html.DocumentNode {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderNode(sb, c, depth)
		}
		return
	}

	tag := n.Data

	// Skip entire subtree.
	if skipTags[tag] {
		return
	}

	// Line breaks.
	if tag == "br" {
		sb.WriteByte('\n')
		return
	}
	if tag == "hr" {
		sb.WriteString("\n---\n")
		return
	}

	// Headings.
	if tag == "h1" || tag == "h2" || tag == "h3" || tag == "h4" || tag == "h5" || tag == "h6" {
		level := int(tag[1] - '0')
		sb.WriteString("\n" + strings.Repeat("#", level) + " ")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderNode(sb, c, depth+1)
		}
		sb.WriteString("\n\n")
		return
	}

	// Links — append URL in parens.
	if tag == "a" {
		href := attrVal(n, "href")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderNode(sb, c, depth+1)
		}
		if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") {
			// Only show URL if it adds information (not same as text).
			linkText := textContent(n)
			if strings.TrimSpace(linkText) != href {
				fmt.Fprintf(sb, " (%s)", href)
			}
		}
		return
	}

	// Images — use alt text.
	if tag == "img" {
		alt := attrVal(n, "alt")
		if alt != "" {
			fmt.Fprintf(sb, "[Image: %s]", alt)
		}
		return
	}

	// Ordered list items — track numbering via depth hack.
	if tag == "ol" {
		i := 1
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "li" {
				sb.WriteString("\n")
				fmt.Fprintf(sb, "%d. ", i)
				i++
				for gc := c.FirstChild; gc != nil; gc = gc.NextSibling {
					renderNode(sb, gc, depth+1)
				}
			}
		}
		sb.WriteByte('\n')
		return
	}

	// Unordered list items.
	if tag == "ul" {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "li" {
				sb.WriteString("\n• ")
				for gc := c.FirstChild; gc != nil; gc = gc.NextSibling {
					renderNode(sb, gc, depth+1)
				}
			}
		}
		sb.WriteByte('\n')
		return
	}

	// Code blocks.
	if tag == "pre" || tag == "code" {
		if tag == "pre" {
			sb.WriteString("\n```\n")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderNode(sb, c, depth+1)
		}
		if tag == "pre" {
			sb.WriteString("\n```\n")
		}
		return
	}

	// Block elements — surround with newlines.
	if blockTags[tag] {
		sb.WriteByte('\n')
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderNode(sb, c, depth+1)
		}
		sb.WriteByte('\n')
		return
	}

	// Everything else — recurse.
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderNode(sb, c, depth+1)
	}
}

// cleanWhitespace collapses runs of blank lines and trims leading/trailing space.
func cleanWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	blanks := 0
	for _, line := range lines {
		trimmed := strings.TrimFunc(line, unicode.IsSpace)
		if trimmed == "" {
			blanks++
			if blanks <= 1 {
				out = append(out, "")
			}
		} else {
			blanks = 0
			out = append(out, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// stripTags is a simple fallback that removes all HTML tags.
func stripTags(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return cleanWhitespace(sb.String())
}
