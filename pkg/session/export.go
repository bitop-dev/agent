// Package session — HTML session export.
//
// ExportHTML renders a complete session as a self-contained, shareable HTML
// file. No external dependencies: all CSS is inlined, no JavaScript required.
//
// Usage:
//
//	data, err := os.ReadFile("my-session.jsonl")
//	html, err := session.ExportHTML(data, session.ExportOptions{Title: "My Session"})
//	os.WriteFile("session.html", html, 0644)
package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
)

// ExportOptions controls HTML rendering.
type ExportOptions struct {
	// Title is used as the <title> and <h1> of the page.
	// Defaults to "Agent Session".
	Title string

	// SessionID is shown in the page header. Optional.
	SessionID string

	// CWD is shown in the page header. Optional.
	CWD string

	// Created is the session creation time. Optional.
	Created time.Time
}

// ExportHTML renders the raw JSONL bytes of a session as a self-contained
// HTML document. Returns the HTML bytes.
func ExportHTML(data []byte, opts ExportOptions) ([]byte, error) {
	msgs, err := ParseMessages(data)
	if err != nil {
		return nil, fmt.Errorf("export: parse messages: %w", err)
	}

	// Also parse the header for metadata.
	var hdr Header
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		typ, raw, err := ParseLine([]byte(line))
		if err != nil || typ != EntryTypeSession {
			continue
		}
		json.Unmarshal(raw, &hdr)
		break
	}

	if opts.Title == "" {
		opts.Title = "Agent Session"
	}
	if opts.SessionID == "" {
		opts.SessionID = hdr.ID
	}
	if opts.CWD == "" {
		opts.CWD = hdr.CWD
	}
	if opts.Created.IsZero() && hdr.Timestamp != "" {
		t, _ := time.Parse(time.RFC3339, hdr.Timestamp)
		opts.Created = t
	}

	var buf bytes.Buffer
	writeHTML(&buf, msgs, opts)
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// HTML rendering
// ---------------------------------------------------------------------------

func writeHTML(buf *bytes.Buffer, msgs []ai.Message, opts ExportOptions) {
	title := html.EscapeString(opts.Title)

	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>` + title + `</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0f0f0f;color:#e0e0e0;line-height:1.6;padding:24px}
.page{max-width:900px;margin:0 auto}
.header{border-bottom:1px solid #333;padding-bottom:16px;margin-bottom:24px}
.header h1{font-size:1.4rem;color:#fff;margin-bottom:6px}
.header .meta{font-size:0.8rem;color:#666;display:flex;gap:16px;flex-wrap:wrap}
.message{margin-bottom:20px;border-radius:8px;overflow:hidden}
.msg-header{padding:8px 14px;font-size:0.75rem;font-weight:600;letter-spacing:0.05em;text-transform:uppercase}
.msg-body{padding:14px;white-space:pre-wrap;word-break:break-word;font-size:0.9rem}
.user .msg-header{background:#1a2a1a;color:#6abf69}
.user .msg-body{background:#111811}
.assistant .msg-header{background:#1a1a2e;color:#7b9dd4}
.assistant .msg-body{background:#111120}
.tool-call .msg-header{background:#2a1a0a;color:#d4a76b}
.tool-call .msg-body{background:#1a100a;font-family:monospace;font-size:0.85rem}
.tool-result .msg-header{background:#1a1a1a;color:#888}
.tool-result .msg-body{background:#111;font-family:monospace;font-size:0.85rem}
.thinking .msg-header{background:#1e1a2e;color:#a78bd4}
.thinking .msg-body{background:#12101a;font-style:italic;color:#999;font-size:0.85rem}
.summary .msg-header{background:#2e2010;color:#c8963e}
.summary .msg-body{background:#1a1208}
code{background:#222;border:1px solid #333;border-radius:3px;padding:2px 5px;font-family:monospace;font-size:0.85em}
pre{background:#1a1a1a;border:1px solid #333;border-radius:6px;padding:12px;overflow-x:auto;font-family:monospace;font-size:0.85rem}
.error{color:#e05c5c}
.meta-badge{background:#222;border-radius:4px;padding:2px 8px}
</style>
</head>
<body>
<div class="page">
<div class="header">
  <h1>` + title + `</h1>
  <div class="meta">`)

	if opts.SessionID != "" {
		fmt.Fprintf(buf, `<span class="meta-badge">session: %s</span>`, html.EscapeString(shortID(opts.SessionID)))
	}
	if opts.CWD != "" {
		fmt.Fprintf(buf, `<span class="meta-badge">cwd: %s</span>`, html.EscapeString(opts.CWD))
	}
	if !opts.Created.IsZero() {
		fmt.Fprintf(buf, `<span class="meta-badge">%s</span>`, opts.Created.Format("2006-01-02 15:04 MST"))
	}
	fmt.Fprintf(buf, `<span class="meta-badge">%d messages</span>`, len(msgs))

	buf.WriteString(`
  </div>
</div>
<div class="messages">
`)

	for _, m := range msgs {
		renderMessage(buf, m)
	}

	buf.WriteString(`
</div>
</div>
</body>
</html>`)
}

func renderMessage(buf *bytes.Buffer, m ai.Message) {
	switch msg := m.(type) {
	case ai.UserMessage:
		// Check if this is a compaction summary.
		if len(msg.Content) == 1 {
			if tc, ok := msg.Content[0].(ai.TextContent); ok {
				if strings.HasPrefix(tc.Text, "The conversation history before this point was compacted") {
					renderSummaryBlock(buf, tc.Text)
					return
				}
			}
		}
		buf.WriteString(`<div class="message user">`)
		buf.WriteString(`<div class="msg-header">User</div>`)
		buf.WriteString(`<div class="msg-body">`)
		for _, b := range msg.Content {
			renderContentBlock(buf, b)
		}
		buf.WriteString(`</div></div>` + "\n")

	case ai.AssistantMessage:
		buf.WriteString(`<div class="message assistant">`)
		label := "Assistant"
		if msg.Model != "" {
			label += " · " + html.EscapeString(msg.Model)
		}
		buf.WriteString(`<div class="msg-header">` + label + `</div>`)
		buf.WriteString(`<div class="msg-body">`)

		// Separate thinking blocks from content.
		var hasContent bool
		for _, b := range msg.Content {
			switch bc := b.(type) {
			case ai.ThinkingContent:
				renderThinkingBlock(buf, bc.Thinking)
			case ai.ToolCall:
				renderToolCallBlock(buf, bc)
			default:
				renderContentBlock(buf, b)
				hasContent = true
			}
		}
		_ = hasContent

		// Show usage if present.
		if msg.Usage.TotalTokens > 0 {
			fmt.Fprintf(buf,
				`<div style="margin-top:8px;font-size:0.75rem;color:#555">in=%d out=%d cache_read=%d cache_write=%d</div>`,
				msg.Usage.Input, msg.Usage.Output, msg.Usage.CacheRead, msg.Usage.CacheWrite,
			)
		}
		if msg.StopReason == ai.StopReasonError {
			fmt.Fprintf(buf, `<div class="error">Error: %s</div>`, html.EscapeString(msg.ErrorMessage))
		}
		buf.WriteString(`</div></div>` + "\n")

	case ai.ToolResultMessage:
		buf.WriteString(`<div class="message tool-result">`)
		label := "Tool Result"
		if msg.ToolName != "" {
			label = "Tool Result · " + html.EscapeString(msg.ToolName)
		}
		if msg.IsError {
			label += " (error)"
		}
		buf.WriteString(`<div class="msg-header">` + label + `</div>`)
		buf.WriteString(`<div class="msg-body">`)
		for _, b := range msg.Content {
			renderContentBlock(buf, b)
		}
		buf.WriteString(`</div></div>` + "\n")
	}
}

func renderContentBlock(buf *bytes.Buffer, b ai.ContentBlock) {
	switch bc := b.(type) {
	case ai.TextContent:
		buf.WriteString(html.EscapeString(bc.Text))
	case ai.ImageContent:
		// Inline base64 image.
		mime := bc.MIMEType
		if mime == "" {
			mime = "image/png"
		}
		fmt.Fprintf(buf, `<img src="data:%s;base64,%s" style="max-width:100%%;border-radius:4px;margin:4px 0" alt="image">`,
			html.EscapeString(mime), html.EscapeString(bc.Data))
	}
}

func renderToolCallBlock(buf *bytes.Buffer, tc ai.ToolCall) {
	args, _ := json.MarshalIndent(tc.Arguments, "", "  ")
	buf.WriteString(`<div class="message tool-call" style="margin:8px 0">`)
	fmt.Fprintf(buf, `<div class="msg-header">Tool Call · %s</div>`, html.EscapeString(tc.Name))
	fmt.Fprintf(buf, `<div class="msg-body"><pre>%s</pre></div>`, html.EscapeString(string(args)))
	buf.WriteString(`</div>`)
}

func renderThinkingBlock(buf *bytes.Buffer, thinking string) {
	buf.WriteString(`<div class="message thinking" style="margin:8px 0">`)
	buf.WriteString(`<div class="msg-header">Thinking</div>`)
	fmt.Fprintf(buf, `<div class="msg-body">%s</div>`, html.EscapeString(truncateStr(thinking, 2000)))
	buf.WriteString(`</div>`)
}

func renderSummaryBlock(buf *bytes.Buffer, text string) {
	buf.WriteString(`<div class="message summary">`)
	buf.WriteString(`<div class="msg-header">⟳ Compaction Summary</div>`)
	fmt.Fprintf(buf, `<div class="msg-body">%s</div>`, html.EscapeString(text))
	buf.WriteString(`</div>` + "\n")
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
