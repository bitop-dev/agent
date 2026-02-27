package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
)

func makeSession(t *testing.T, msgs ...ai.Message) []byte {
	t.Helper()
	dir := t.TempDir()
	sess, err := Create(dir, "/cwd")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if _, err := sess.AppendMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	path := sess.FilePath()
	sess.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func makeCompactedSession(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	sess, err := Create(dir, "/cwd")
	if err != nil {
		t.Fatal(err)
	}
	sess.AppendMessage(ai.UserMessage{
		Role:    ai.RoleUser,
		Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: "old message"}},
	})
	firstKeptID, _ := sess.AppendMessage(ai.UserMessage{
		Role:    ai.RoleUser,
		Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: "kept message"}},
	})
	sess.AppendCompaction("Summary of old stuff.", firstKeptID, 500)
	path := sess.FilePath()
	sess.Close()
	data, _ := os.ReadFile(path)
	return data
}

// ---------------------------------------------------------------------------

func TestExportHTML_Basic(t *testing.T) {
	data := makeSession(t,
		ai.UserMessage{
			Role:      ai.RoleUser,
			Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: "Hello, world!"}},
			Timestamp: time.Now().UnixMilli(),
		},
		ai.AssistantMessage{
			Role:       ai.RoleAssistant,
			Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: "Hi there!"}},
			Model:      "test-model",
			StopReason: ai.StopReasonStop,
			Usage:      ai.Usage{Input: 10, Output: 5, TotalTokens: 15},
			Timestamp:  time.Now().UnixMilli(),
		},
	)

	html, err := ExportHTML(data, ExportOptions{Title: "Test Export"})
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}

	got := string(html)
	for _, want := range []string{
		"<!DOCTYPE html>", "Test Export", "Hello, world!", "Hi there!", "test-model",
		`class="message user"`, `class="message assistant"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
}

func TestExportHTML_ToolCallAndResult(t *testing.T) {
	data := makeSession(t,
		ai.AssistantMessage{
			Role: ai.RoleAssistant,
			Content: []ai.ContentBlock{
				ai.ToolCall{Type: "tool_call", ID: "c1", Name: "bash", Arguments: map[string]any{"command": "ls"}},
			},
			StopReason: ai.StopReasonTool,
			Timestamp:  time.Now().UnixMilli(),
		},
		ai.ToolResultMessage{
			Role:       ai.RoleToolResult,
			ToolCallID: "c1",
			ToolName:   "bash",
			Content:    []ai.ContentBlock{ai.TextContent{Type: "text", Text: "file.go"}},
			Timestamp:  time.Now().UnixMilli(),
		},
	)

	html, err := ExportHTML(data, ExportOptions{})
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	got := string(html)
	for _, want := range []string{"Tool Call", "Tool Result", "bash", "file.go"} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
}

func TestExportHTML_ThinkingBlock(t *testing.T) {
	data := makeSession(t,
		ai.AssistantMessage{
			Role: ai.RoleAssistant,
			Content: []ai.ContentBlock{
				ai.ThinkingContent{Type: "thinking", Thinking: "Let me reason about this..."},
				ai.TextContent{Type: "text", Text: "The answer is 42."},
			},
			StopReason: ai.StopReasonStop,
			Timestamp:  time.Now().UnixMilli(),
		},
	)

	html, err := ExportHTML(data, ExportOptions{})
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	got := string(html)
	if !strings.Contains(got, "Thinking") {
		t.Error("HTML should show thinking block")
	}
	if !strings.Contains(got, "Let me reason about this") {
		t.Error("HTML should contain thinking text")
	}
}

func TestExportHTML_CompactionSummary(t *testing.T) {
	data := makeCompactedSession(t)

	html, err := ExportHTML(data, ExportOptions{})
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	got := string(html)
	if !strings.Contains(got, "Compaction Summary") {
		t.Error("HTML should show compaction summary block")
	}
	if !strings.Contains(got, "Summary of old stuff.") {
		t.Error("HTML should contain compaction summary text")
	}
}

func TestExportHTML_SelfContained(t *testing.T) {
	data := makeSession(t,
		ai.UserMessage{
			Role:    ai.RoleUser,
			Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: "test"}},
		},
	)

	html, err := ExportHTML(data, ExportOptions{Title: "My Test"})
	if err != nil {
		t.Fatalf("ExportHTML: %v", err)
	}
	got := string(html)

	// Must be self-contained: no external script or stylesheet links.
	if strings.Contains(got, "src=\"http") || strings.Contains(got, "href=\"http") {
		t.Error("HTML should not reference external resources")
	}
	if !strings.Contains(got, "</html>") {
		t.Error("HTML should be a complete document")
	}
}
